package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/output"
	"github.com/spf13/cobra"
)

// 客户端侧尺寸上限（用 var 便于单测调小覆盖拦截路径）。
//
//	maxAppsRawBytes     —— tar+gzip 进入内存前的「未压缩」总大小上限，防解压炸弹/OOM。
//	maxAppsTarballBytes —— 打包后 tar.gz 上限，对齐 OAPI「本期接口上限 20MB」约束。
var (
	maxAppsRawBytes     int64 = 200 * 1024 * 1024
	maxAppsTarballBytes int64 = 20 * 1024 * 1024
)

// maxAppsSensitiveListInError 控制校验错误里最多内联列出多少个凭证文件命中。
const maxAppsSensitiveListInError = 5

var appsHTMLPublishCmd = &cobra.Command{
	Use:   "html-publish",
	Short: "把 HTML 文件/目录打包发布到妙搭应用，返回访问 URL（一键部署）",
	Long: `把 --path（单个 HTML 文件或整个目录）打包成 tar.gz，单次 multipart POST 上传并发布，
返回可访问的应用 URL。

要求:
  - 目录形态：根目录下必须有 index.html（妙搭以它作为应用入口）
  - 单文件形态：文件名必须就是 index.html
  - 未压缩总大小 ≤ 200MB；打包后 tar.gz ≤ 20MB
  - 默认拦截凭证文件（.env / .npmrc / .netrc / .git-credentials / .aws/credentials /
    .docker/config.json / .kube/config），用 --allow-sensitive 显式放行

权限: User Access Token + spark:app:write

示例:
  feishu-cli apps html-publish --app-id app_xxx --path ./index.html
  feishu-cli apps html-publish --app-id app_xxx --path ./dist
  feishu-cli apps html-publish --app-id app_xxx --path ./dist --dry-run   # 只看打包清单`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		appID := strings.TrimSpace(flagString(cmd, "app-id"))
		if appID == "" {
			return fmt.Errorf("--app-id 不能为空")
		}
		pathArg := strings.TrimSpace(flagString(cmd, "path"))
		if pathArg == "" {
			return fmt.Errorf("--path 不能为空")
		}
		allowSensitive, _ := cmd.Flags().GetBool("allow-sensitive")
		dry, _ := cmd.Flags().GetBool("dry-run")

		candidates, walkErr := appsWalkCandidates(pathArg)

		// 凭证文件拦截：dry-run 和实跑共用同一道闸门（与官方 lark-cli 语义一致，
		// 命中且未加 --allow-sensitive 时两条路径都非零退出）。walk 失败时跳过，
		// 交给下面的分支用各自更丰富的报错呈现。
		if walkErr == nil && !allowSensitive {
			var hits []string
			for _, c := range candidates {
				if appsIsSensitiveCandidate(pathArg, c) {
					hits = append(hits, c.RelPath)
				}
			}
			if len(hits) > 0 {
				return appsSensitiveError(hits)
			}
		}

		if dry {
			return appsHTMLPublishDryRun(appID, pathArg, candidates, walkErr, allowSensitive)
		}

		if walkErr != nil {
			return fmt.Errorf("扫描 --path %s 失败: %w", pathArg, walkErr)
		}
		if err := appsEnsureIndexHTML(candidates); err != nil {
			return err
		}

		var rawTotal int64
		for _, c := range candidates {
			rawTotal += c.Size
		}
		if rawTotal > maxAppsRawBytes {
			return fmt.Errorf("--path 未压缩总大小 %d 字节超过 %d 字节上限（tar+gzip 进内存前拦截，避免 OOM）；精简 --path 内容或选更小的子目录", rawTotal, maxAppsRawBytes)
		}

		tarball, err := appsBuildTarball(candidates)
		if err != nil {
			return fmt.Errorf("打包失败: %w", err)
		}
		if int64(len(tarball)) > maxAppsTarballBytes {
			return fmt.Errorf("打包后 tar.gz 大小 %d 字节超过 %d 字节上限；精简 --path 目录（去掉无关大文件/压缩资源）后重试，本期接口上限 20MB", len(tarball), maxAppsTarballBytes)
		}

		token, err := requireUserToken(cmd, "apps html-publish")
		if err != nil {
			return err
		}
		data, err := client.SparkHTMLPublish(appID, tarball, token)
		if err != nil {
			return err
		}
		return renderAppsResult(cmd, data)
	},
}

// appsHTMLPublishDryRun 打印打包清单预览（文件列表/总大小/缺 index.html 提示/放行的凭证文件）。
func appsHTMLPublishDryRun(appID, pathArg string, candidates []appsCandidate, walkErr error, allowSensitive bool) error {
	o, err := output.NewOptions(output.FormatJSON, "")
	if err != nil {
		return err
	}
	m := map[string]any{
		"method":       "POST",
		"endpoint":     appsAppPath(appID, "/upload_and_release_html_code"),
		"content_type": "multipart/form-data",
		"dry_run":      true,
	}
	if walkErr != nil {
		m["path_error"] = walkErr.Error()
		return output.Render(o, m)
	}
	// 缺 index.html 在 dry-run 里以字段呈现（仍 0 退出，符合 dry-run「预览」语义）。
	if err := appsEnsureIndexHTML(candidates); err != nil {
		m["validation_error"] = err.Error()
	}
	var total int64
	names := make([]string, 0, len(candidates))
	for _, c := range candidates {
		total += c.Size
		names = append(names, c.RelPath)
	}
	m["file_count"] = len(candidates)
	m["total_size_bytes"] = total
	m["files"] = names
	if allowSensitive {
		var waived []string
		for _, c := range candidates {
			if appsIsSensitiveCandidate(pathArg, c) {
				waived = append(waived, c.RelPath)
			}
		}
		if len(waived) > 0 {
			m["sensitive_waived"] = waived
			m["sensitive_waived_summary"] = fmt.Sprintf("%d 个凭证文件因 --allow-sensitive 被放行", len(waived))
		}
	}
	return output.Render(o, m)
}

// appsCandidate 是待打包的一个文件条目。
type appsCandidate struct {
	RelPath string // tar 内的相对路径（forward-slash）
	AbsPath string // 磁盘绝对/相对路径
	Size    int64
}

// appsWalkCandidates 遍历 rootPath，返回每个 regular file。单文件形态返回一条
// （RelPath = basename）；目录形态用 filepath.WalkDir 收集所有 regular file
// （symlink/device/pipe/socket 跳过）。
func appsWalkCandidates(rootPath string) ([]appsCandidate, error) {
	stat, err := os.Stat(rootPath)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", rootPath, err)
	}
	if !stat.IsDir() {
		return []appsCandidate{{
			RelPath: filepath.Base(rootPath),
			AbsPath: rootPath,
			Size:    stat.Size(),
		}}, nil
	}

	var out []appsCandidate
	err = filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		// 只接受 regular file —— symlink 不跟随（避免 loop + 越界引用）。
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(rootPath, path)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)
		if appsIsUnsafeRelPath(relSlash) {
			return fmt.Errorf("遍历产生了不安全的相对路径 %q（%s）", relSlash, path)
		}
		out = append(out, appsCandidate{RelPath: relSlash, AbsPath: path, Size: info.Size()})
		return nil
	})
	return out, err
}

// appsIsUnsafeRelPath 判断一个 forward-slash 相对路径是否含越界/危险成分：
// 绝对路径前缀、.. 作为完整路径成分、或内嵌空字节。组件级判断，不会对
// 合法文件名里恰好含 ".." 子串（如 archive.tar..bak）误报。
func appsIsUnsafeRelPath(rel string) bool {
	return strings.HasPrefix(rel, "/") ||
		rel == ".." ||
		strings.HasPrefix(rel, "../") ||
		strings.Contains(rel, "/../") ||
		strings.HasSuffix(rel, "/..") ||
		strings.ContainsRune(rel, 0)
}

// appsEnsureIndexHTML 要求 candidates 里必须含 index.html（妙搭以它作为应用入口）。
func appsEnsureIndexHTML(candidates []appsCandidate) error {
	for _, c := range candidates {
		if c.RelPath == "index.html" {
			return nil
		}
	}
	return fmt.Errorf("--path 中缺少 index.html；妙搭以 index.html 作为应用入口（目录形态把首页放根目录命名 index.html，单文件形态把文件命名为 index.html）")
}

// appsBuildTarball 把 candidates 打包成内存中的 tar.gz。
func appsBuildTarball(candidates []appsCandidate) ([]byte, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("没有可打包的文件")
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, c := range candidates {
		if err := appsWriteTarEntry(tw, c); err != nil {
			_ = tw.Close()
			_ = gz.Close()
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		_ = gz.Close()
		return nil, fmt.Errorf("tar close: %w", err)
	}
	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("gzip close: %w", err)
	}
	return buf.Bytes(), nil
}

func appsWriteTarEntry(tw *tar.Writer, c appsCandidate) error {
	if appsIsUnsafeRelPath(c.RelPath) {
		return fmt.Errorf("非法 tar 条目名 %q", c.RelPath)
	}
	src, err := os.Open(c.AbsPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", c.AbsPath, err)
	}
	defer src.Close()

	hdr := &tar.Header{
		Name:     c.RelPath,
		Size:     c.Size,
		Mode:     0o644,
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("write header %s: %w", c.RelPath, err)
	}
	if _, err := io.Copy(tw, src); err != nil {
		return fmt.Errorf("copy %s: %w", c.RelPath, err)
	}
	return nil
}

// appsSensitiveError 构造凭证文件拦截错误（命中且未加 --allow-sensitive）。
func appsSensitiveError(hits []string) error {
	var sample string
	if len(hits) <= maxAppsSensitiveListInError {
		sample = strings.Join(hits, ", ")
	} else {
		sample = strings.Join(hits[:maxAppsSensitiveListInError], ", ") +
			fmt.Sprintf("（还有 %d 个）", len(hits)-maxAppsSensitiveListInError)
	}
	return fmt.Errorf("--path 含 %d 个不应发布的凭证文件: %s\n从发布内容里移除这些文件，或确实要发布时加 --allow-sensitive", len(hits), sample)
}

func init() {
	appsCmd.AddCommand(appsHTMLPublishCmd)
	appsHTMLPublishCmd.Flags().String("app-id", "", "妙搭应用 ID（必填）")
	appsHTMLPublishCmd.Flags().String("path", "", "HTML 文件或目录路径（必填）")
	appsHTMLPublishCmd.Flags().Bool("allow-sensitive", false, "跳过凭证文件扫描（放行 .env / .npmrc / .aws/credentials 等）")
	addAppsWriteFlags(appsHTMLPublishCmd)
}
