package client

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// MailInlineImageRef 描述 body 中一个本地图片引用的扫描结果
// RawSrc:     原 HTML 中 <img src="..."> 里的字面值
// LocalPath:  解析后的绝对路径
// CID:        生成的唯一 CID（不含 "<>" 包裹，不含 "cid:" 前缀）
// FileToken:  上传到 drive 后返回的 file_token（用于附件回写 EML body part）
// FileName:   原始文件名
// Bytes:      文件内容（构造 multipart/related 时使用）
// MIME:       内容类型（image/png 等，按扩展名兜底为 application/octet-stream）
type MailInlineImageRef struct {
	RawSrc    string
	LocalPath string
	CID       string
	FileToken string
	FileName  string
	Bytes     []byte
	MIME      string
}

// imgSrcRegexp 抓 <img ...src="...">，捕获 src 的字面值
// 不区分大小写，兼容 <IMG SRC='...'>，单/双引号皆可
var imgSrcRegexp = regexp.MustCompile(`(?i)<img\b[^>]*\bsrc\s*=\s*(?:"([^"]+)"|'([^']+)')[^>]*>`)

// inlineURISchemeRegexp 检测带 scheme 的 URL（已经是 cid:/http(s):/data: 不再扫描）
var inlineURISchemeRegexp = regexp.MustCompile(`(?i)^[a-z][a-z0-9+.\-]*:`)

// isWindowsDrivePath 检测 Windows 驱动器路径（如 C:\file.png / d:/x），避免被 scheme 正则误判为 URI scheme
func isWindowsDrivePath(s string) bool {
	if len(s) < 2 || s[1] != ':' {
		return false
	}
	c := s[0]
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

// ScanInlineImagePaths 扫描 HTML body 中的 <img src="local-path">，仅返回本地路径
// 已经是 cid:/http:/https:/data: 等 scheme 的会跳过
// 同一文件路径只返回一次（去重）
func ScanInlineImagePaths(body string) []string {
	matches := imgSrcRegexp.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	for _, m := range matches {
		var src string
		if len(m) >= 2 && m[1] != "" {
			src = m[1]
		} else if len(m) >= 3 && m[2] != "" {
			src = m[2]
		}
		src = strings.TrimSpace(src)
		if src == "" {
			continue
		}
		// 协议无关 URL（//cdn.com/x.png）跳过
		if strings.HasPrefix(src, "//") {
			continue
		}
		// 有 scheme（http:/https:/data:/cid: ...）跳过；
		// 但 Windows 驱动器路径（C:\x.png）形似 scheme，先放行视为本地路径
		if !isWindowsDrivePath(src) && inlineURISchemeRegexp.MatchString(src) {
			continue
		}
		if seen[src] {
			continue
		}
		seen[src] = true
		out = append(out, src)
	}
	return out
}

// ReplaceInlineImageSrc 把 body 中所有匹配 rawSrc 的 <img src="rawSrc"> 替换为 cid:cid
// 使用倒序替换以保持索引稳定
func ReplaceInlineImageSrc(body string, refs []MailInlineImageRef) string {
	if len(refs) == 0 {
		return body
	}
	matches := imgSrcRegexp.FindAllStringSubmatchIndex(body, -1)
	if len(matches) == 0 {
		return body
	}
	srcToCID := make(map[string]string, len(refs))
	for _, r := range refs {
		srcToCID[r.RawSrc] = r.CID
	}
	// 倒序，避免 index 漂移
	out := body
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		// m[2]:m[3] 为双引号 src；m[4]:m[5] 为单引号 src
		var s, e int
		if m[2] >= 0 {
			s, e = m[2], m[3]
		} else if m[4] >= 0 {
			s, e = m[4], m[5]
		} else {
			continue
		}
		srcVal := strings.TrimSpace(out[s:e])
		cid, ok := srcToCID[srcVal]
		if !ok {
			continue
		}
		out = out[:s] + "cid:" + cid + out[e:]
	}
	return out
}

// GenerateMailCID 生成一个 20-hex CID（与 lark-cli 风格一致）
func GenerateMailCID() (string, error) {
	var b [10]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("生成 CID 失败: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// guessMIMEByExt 按扩展名兜底 MIME（避免 multipart/related 缺 Content-Type）
func guessMIMEByExt(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".svg":
		return "image/svg+xml"
	default:
		return "application/octet-stream"
	}
}

// LoadInlineImageBytes 读取本地文件并填充 FileName/Bytes/MIME
// 用于 EML builder 构造 multipart/related part；调用方可选择只用 FileToken
// 而不读盘（如已经走 drive 上传完毕、只需 cid 替换）
//
// 安全：拒绝路径遍历（`..` 片段）+ 限制 abs 必须落在 cwd 或 home 子树内，
// 防止恶意 HTML `<img src="../../.ssh/id_rsa">` 把敏感文件作为附件外发。
func LoadInlineImageBytes(ref *MailInlineImageRef) error {
	if err := validateInlineImagePath(ref.LocalPath); err != nil {
		return err
	}
	abs, err := filepath.Abs(ref.LocalPath)
	if err != nil {
		return fmt.Errorf("解析路径失败 %s: %w", ref.LocalPath, err)
	}
	if err := assertPathInSafeRoots(abs); err != nil {
		return err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return fmt.Errorf("读取本地图片失败 %s: %w", abs, err)
	}
	ref.Bytes = data
	if ref.FileName == "" {
		ref.FileName = filepath.Base(abs)
	}
	if ref.MIME == "" {
		ref.MIME = guessMIMEByExt(ref.FileName)
	}
	return nil
}

// validateInlineImagePath 拒绝任何包含 ".." 片段的路径（防路径遍历）。
// 注意：直接拒绝 `..` 比 `filepath.Clean` 后比较更严，避免 symlink 绕过。
func validateInlineImagePath(p string) error {
	if p == "" {
		return fmt.Errorf("内嵌图片路径为空")
	}
	// 兼容 Windows 路径，统一替换分隔符再切分
	norm := strings.ReplaceAll(p, "\\", "/")
	for _, seg := range strings.Split(norm, "/") {
		if seg == ".." {
			return fmt.Errorf("内嵌图片路径不允许包含 `..`（防路径遍历）: %s", p)
		}
	}
	return nil
}

// assertPathInSafeRoots 要求 abs 必须落在当前工作目录或 user home 子树内。
// 在没有任一根可解析时（罕见）放行，避免误伤；但 `..` 已在前一步拦截。
func assertPathInSafeRoots(abs string) error {
	roots := make([]string, 0, 2)
	if cwd, err := os.Getwd(); err == nil && cwd != "" {
		if r, err2 := filepath.Abs(cwd); err2 == nil {
			roots = append(roots, r)
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if r, err2 := filepath.Abs(home); err2 == nil {
			roots = append(roots, r)
		}
	}
	if len(roots) == 0 {
		return nil
	}
	for _, r := range roots {
		// 用 filepath.Rel 兼容跨平台分隔符；rel 不含 ".." 即在子树内
		rel, err := filepath.Rel(r, abs)
		if err != nil {
			continue
		}
		if rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)) {
			return nil
		}
	}
	return fmt.Errorf("内嵌图片路径必须落在当前目录或 home 子树内: %s", abs)
}
