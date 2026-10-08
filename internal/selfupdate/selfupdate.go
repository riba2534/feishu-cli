// Package selfupdate 实现 feishu-cli 的版本检查与自更新。
//
// 版本查询沿用 install.sh 的思路：优先请求 github.com/<repo>/releases/latest 的 302 跳转，
// 从 Location 提取 tag（走网页路由，不消耗 api.github.com 配额）；失败再回退 GitHub API。
// 安装包命名、内部结构与 checksums.txt 遵循仓库 CLAUDE.md 的发布规范：
//
//	feishu-cli_{version}_{os}-{arch}.tar.gz → feishu-cli_{version}_{os}-{arch}/feishu-cli[.exe]
//
// 本包只在用户显式运行 update 时联网，不做任何后台检查。
package selfupdate

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultRepo 是发布 release 的 GitHub 仓库。
	DefaultRepo = "riba2534/feishu-cli"
	// MaxArchiveBytes 限制下载的安装包大小，防止异常响应占满磁盘。
	MaxArchiveBytes = 300 << 20
	// MaxBinaryBytes 限制解压出的二进制大小。
	MaxBinaryBytes    = 500 << 20
	maxChecksumsBytes = 1 << 20
)

// SupportedPlatforms 是 release 提供安装包的平台（os/arch）。
var SupportedPlatforms = []string{"linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64", "windows/amd64"}

var tagPattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$`)

// Client 访问 GitHub release。
type Client struct {
	HTTP    *http.Client
	WebBase string // 默认 https://github.com
	APIBase string // 默认 https://api.github.com
	Repo    string
	Token   string // 仅用于 API 回退（GITHUB_TOKEN），不会发往下载地址
	// UserAgent 随请求发送；GitHub API 要求非空 UA。
	UserAgent string
}

// New 返回默认客户端（遵循 HTTP(S)_PROXY / NO_PROXY）。
func New() *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 10 * time.Minute},
		WebBase:   "https://github.com",
		APIBase:   "https://api.github.com",
		Repo:      DefaultRepo,
		Token:     strings.TrimSpace(os.Getenv("GITHUB_TOKEN")),
		UserAgent: "feishu-cli-update",
	}
}

func (c *Client) newRequest(ctx context.Context, method, rawURL string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	return req, nil
}

// VersionQueryTimeout 是单次版本查询（302 或 API）的超时。
var VersionQueryTimeout = 20 * time.Second

// LatestVersion 返回最新 release 的 tag。
func (c *Client) LatestVersion(ctx context.Context) (string, error) {
	redirectCtx, cancel := context.WithTimeout(ctx, VersionQueryTimeout)
	tag, redirectErr := c.latestViaRedirect(redirectCtx)
	cancel()
	if redirectErr == nil {
		return tag, nil
	}
	apiCtx, cancelAPI := context.WithTimeout(ctx, VersionQueryTimeout)
	defer cancelAPI()
	tag, apiErr := c.latestViaAPI(apiCtx)
	if apiErr == nil {
		return tag, nil
	}
	return "", fmt.Errorf("获取最新版本失败（302 跳转: %v；GitHub API: %v）。请检查网络/代理（HTTPS_PROXY），API 限流时可设置 GITHUB_TOKEN", redirectErr, apiErr)
}

func (c *Client) latestViaRedirect(ctx context.Context) (string, error) {
	req, err := c.newRequest(ctx, http.MethodHead, fmt.Sprintf("%s/%s/releases/latest", c.WebBase, c.Repo))
	if err != nil {
		return "", err
	}
	client := *c.HTTP
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		return "", fmt.Errorf("期望 302 跳转，实际 HTTP %d", resp.StatusCode)
	}
	return tagFromLocation(resp.Header.Get("Location"))
}

func tagFromLocation(location string) (string, error) {
	idx := strings.LastIndex(location, "/releases/tag/")
	if idx < 0 {
		return "", fmt.Errorf("跳转地址中没有 tag: %q", location)
	}
	raw := location[idx+len("/releases/tag/"):]
	if cut := strings.IndexAny(raw, "?#/"); cut >= 0 {
		raw = raw[:cut]
	}
	tag, err := url.PathUnescape(raw)
	if err != nil {
		return "", fmt.Errorf("解析 tag 失败: %w", err)
	}
	return validateTag(tag)
}

func (c *Client) latestViaAPI(ctx context.Context) (string, error) {
	req, err := c.newRequest(ctx, http.MethodGet, fmt.Sprintf("%s/repos/%s/releases/latest", c.APIBase, c.Repo))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return "", fmt.Errorf("HTTP %d（可能触发未认证 API 限流 60 次/小时）", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}
	return validateTag(payload.TagName)
}

func validateTag(tag string) (string, error) {
	tag = strings.TrimSpace(tag)
	if !tagPattern.MatchString(tag) {
		return "", fmt.Errorf("版本号格式异常: %q（期望形如 v1.2.3）", tag)
	}
	return tag, nil
}

// AssetName 返回指定平台的安装包文件名。
func AssetName(version, goos, goarch string) (string, error) {
	platform := goos + "/" + goarch
	for _, p := range SupportedPlatforms {
		if p == platform {
			return fmt.Sprintf("feishu-cli_%s_%s-%s.tar.gz", version, goos, goarch), nil
		}
	}
	return "", fmt.Errorf("release 未提供 %s 平台的安装包（支持: %s），请从源码构建: go install github.com/%s@latest", platform, strings.Join(SupportedPlatforms, ", "), DefaultRepo)
}

// BinaryName 返回安装包内的二进制文件名。
func BinaryName(goos string) string {
	if goos == "windows" {
		return "feishu-cli.exe"
	}
	return "feishu-cli"
}

// DownloadURL 返回 release 资产下载地址。
func (c *Client) DownloadURL(version, asset string) string {
	return fmt.Sprintf("%s/%s/releases/download/%s/%s", c.WebBase, c.Repo, url.PathEscape(version), url.PathEscape(asset))
}

// ErrNotFound 表示 release 资产不存在（HTTP 404）。
var ErrNotFound = errors.New("资源不存在")

// Fetch 下载 url 的内容写入 w，超过 limit 字节报错。
func (c *Client) Fetch(ctx context.Context, rawURL string, w io.Writer, limit int64) (int64, error) {
	req, err := c.newRequest(ctx, http.MethodGet, rawURL)
	if err != nil {
		return 0, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, fmt.Errorf("下载 %s 失败: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return 0, fmt.Errorf("下载 %s 失败: %w (HTTP 404)", rawURL, ErrNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("下载 %s 失败: HTTP %d", rawURL, resp.StatusCode)
	}
	n, err := io.Copy(w, io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return n, fmt.Errorf("下载 %s 中断: %w", rawURL, err)
	}
	if n > limit {
		return n, fmt.Errorf("下载 %s 超过大小上限 %d 字节", rawURL, limit)
	}
	return n, nil
}

// FetchChecksums 下载并解析 checksums.txt。
func (c *Client) FetchChecksums(ctx context.Context, version string) (map[string]string, error) {
	var buf bytes.Buffer
	if _, err := c.Fetch(ctx, c.DownloadURL(version, "checksums.txt"), &buf, maxChecksumsBytes); err != nil {
		return nil, err
	}
	sums := ParseChecksums(buf.Bytes())
	if len(sums) == 0 {
		return nil, errors.New("checksums.txt 为空或格式无法识别")
	}
	return sums, nil
}

// ParseChecksums 解析 sha256sum 输出格式（"<hash>  <file>" 或 "<hash> *<file>"）。
func ParseChecksums(data []byte) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 || len(fields[0]) != 64 {
			continue
		}
		if _, err := hex.DecodeString(fields[0]); err != nil {
			continue
		}
		out[strings.TrimPrefix(fields[1], "*")] = strings.ToLower(fields[0])
	}
	return out
}

// FileSHA256 计算文件 sha256。
func FileSHA256(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ExtractBinary 从 tar.gz 中找出名为 binaryName 的普通文件（位于根或一级目录下）写入 w。
// 拒绝绝对路径、.. 路径与超过 MaxBinaryBytes 的条目。
func ExtractBinary(archivePath, binaryName string, w io.Writer) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("安装包不是合法的 gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("读取安装包失败: %w", err)
		}
		name := strings.TrimPrefix(hdr.Name, "./")
		if hdr.Typeflag != tar.TypeReg || path.IsAbs(name) || strings.Contains(name, "..") {
			continue
		}
		parts := strings.Split(strings.Trim(name, "/"), "/")
		if len(parts) > 2 || parts[len(parts)-1] != binaryName {
			continue
		}
		if hdr.Size > MaxBinaryBytes {
			return fmt.Errorf("安装包中的 %s 超过大小上限", binaryName)
		}
		if _, err := io.Copy(w, io.LimitReader(tr, MaxBinaryBytes)); err != nil {
			return fmt.Errorf("解压 %s 失败: %w", binaryName, err)
		}
		return nil
	}
	return fmt.Errorf("安装包中没有找到 %s", binaryName)
}

// Version 是解析后的版本号。Dev 表示 git describe 形式的开发构建（基于该 tag 之后的提交或含未提交改动）。
type Version struct {
	Major, Minor, Patch int
	Pre                 string
	Dev                 bool
	Raw                 string
}

var (
	versionPattern  = regexp.MustCompile(`^v?([0-9]+)\.([0-9]+)\.([0-9]+)(?:-([0-9A-Za-z.-]+))?$`)
	describePattern = regexp.MustCompile(`^(?:[0-9]+-g[0-9a-f]+)?(?:-?dirty)?$`)
	findVersion     = regexp.MustCompile(`v?[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?`)
)

// ParseVersion 解析 v1.2.3 / 1.2.3 / v1.2.3-rc1 / v1.2.3-5-gabc123(-dirty)。
func ParseVersion(s string) (Version, bool) {
	s = strings.TrimSpace(s)
	m := versionPattern.FindStringSubmatch(s)
	if m == nil {
		return Version{}, false
	}
	v := Version{Raw: s}
	v.Major, _ = strconv.Atoi(m[1])
	v.Minor, _ = strconv.Atoi(m[2])
	v.Patch, _ = strconv.Atoi(m[3])
	if suffix := m[4]; suffix != "" {
		if describePattern.MatchString(suffix) {
			v.Dev = true
		} else {
			v.Pre = suffix
		}
	}
	return v, true
}

// ExtractVersion 从 `feishu-cli --version` 输出中提取版本号。
func ExtractVersion(output string) string {
	return findVersion.FindString(output)
}

// Compare 比较两个版本：开发构建 > 同号正式版 > 同号预发布版。
func Compare(a, b Version) int {
	for _, d := range [][2]int{{a.Major, b.Major}, {a.Minor, b.Minor}, {a.Patch, b.Patch}} {
		if d[0] != d[1] {
			if d[0] < d[1] {
				return -1
			}
			return 1
		}
	}
	rank := func(v Version) int {
		switch {
		case v.Dev:
			return 2
		case v.Pre == "":
			return 1
		default:
			return 0
		}
	}
	if ra, rb := rank(a), rank(b); ra != rb {
		if ra < rb {
			return -1
		}
		return 1
	}
	return strings.Compare(a.Pre, b.Pre)
}

// SameVersion 判断两个版本号是否指同一个 tag（忽略 v 前缀）。
func SameVersion(a, b string) bool {
	return strings.TrimPrefix(strings.TrimSpace(a), "v") == strings.TrimPrefix(strings.TrimSpace(b), "v")
}

// BinaryVersion 运行二进制的 --version 并提取版本号。
func BinaryVersion(ctx context.Context, binary string) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, binary, "--version").CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		return "", text, fmt.Errorf("运行 %s --version 失败: %w（输出: %s）", binary, err, text)
	}
	v := ExtractVersion(text)
	if v == "" {
		return "", text, fmt.Errorf("无法从 %s --version 输出中识别版本号: %q", binary, text)
	}
	return v, text, nil
}

// ReplaceExecutable 用 newPath（必须与 target 在同一目录）原子替换 target。
// Windows 无法覆盖运行中的 exe：先把旧文件改名为 .old 再放入新文件，失败时回滚。
func ReplaceExecutable(target, newPath, goos string) (string, error) {
	if filepath.Dir(target) != filepath.Dir(newPath) {
		return "", errors.New("新二进制必须与目标位于同一目录才能原子替换")
	}
	if goos != "windows" {
		return "", os.Rename(newPath, target)
	}
	old := target + ".old"
	_ = os.Remove(old)
	if err := os.Rename(target, old); err != nil {
		return "", err
	}
	if err := os.Rename(newPath, target); err != nil {
		_ = os.Rename(old, target)
		return "", err
	}
	return old, nil
}
