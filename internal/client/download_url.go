package client

import (
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/v2/internal/safefile"
)

// DownloadOptions 预签名 URL 下载选项
type DownloadOptions struct {
	OutputDir string        // 输出目录；为空时使用当前工作目录
	Filename  string        // 强制指定文件名；为空时从响应头解析
	Overwrite bool          // 是否覆盖已存在文件
	Timeout   time.Duration // 整个下载的超时；<=0 时不设置（由调用方用 ctx 控制）
	// UniqueName 可选：文件名确定后、覆盖检查前调用，返回实际使用的文件名。
	// 批量下载用它在同一批次内去重（如 a.mp4 → a-2.mp4），避免后一个文件覆盖本批次已写出的文件。
	UniqueName func(filename string) string
}

// DownloadResult 下载结果
type DownloadResult struct {
	SavedPath string
	Size      int64
	Filename  string
}

// 飞书预签名 URL 下载最大重定向次数
const downloadMaxRedirects = 5

// DownloadFromPresignedURL 从预签名 URL 下载文件到磁盘
// 特性：
//   - SSRF 防护：拒绝内网 IP / localhost / file://
//   - 重定向防护：最多 5 次，拒绝 HTTPS → HTTP 降级，每次重定向重新校验 host
//   - 文件名解析优先级：opts.Filename > Content-Disposition > Content-Type 扩展 > defaultFallbackName
//   - 请求 context 派生自进程根 context（Ctrl-C 可中断）；opts.Timeout > 0 时为总时长上限
//   - 空闲超时 + 分片级有界重试 + 断点续传；同目录临时文件 + rename，失败不留半截文件
func DownloadFromPresignedURL(presignedURL string, defaultFallbackName string, opts DownloadOptions) (*DownloadResult, error) {
	if err := validateDownloadURL(presignedURL); err != nil {
		return nil, err
	}
	// 输出目录在发起下载前拒绝敏感目录（最终文件另由 safefile.AtomicWriteFrom 兜底校验）
	if opts.OutputDir != "" {
		if err := validatePath(opts.OutputDir); err != nil {
			return nil, err
		}
	}

	httpClient := publicDownloadHTTPClient(func(req *http.Request, via []*http.Request) error {
		if len(via) >= downloadMaxRedirects {
			return fmt.Errorf("下载重定向次数超过 %d", downloadMaxRedirects)
		}
		return validateDownloadURL(req.URL.String())
	})

	ctx, cancel := downloadContext(opts.Timeout)
	defer cancel()
	stream, err := openDownloadStream(ctx, downloadStreamSpec{
		Action:     "下载",
		URL:        presignedURL,
		HTTPClient: httpClient,
	})
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	// 决定文件名
	filename := opts.Filename
	if filename == "" {
		filename = resolveFilenameFromResponse(&http.Response{Header: stream.Header()}, defaultFallbackName)
	}
	filename = sanitizeFilename(filename)
	if filename == "" {
		filename = sanitizeFilename(defaultFallbackName)
	}
	if filename == "" {
		filename = "download"
	}
	if opts.UniqueName != nil {
		if alt := sanitizeFilename(opts.UniqueName(filename)); alt != "" {
			filename = alt
		}
	}

	// 输出目录
	outputDir := opts.OutputDir
	if outputDir == "" {
		outputDir = "."
	}
	if err := safefile.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建输出目录失败: %w", err)
	}

	outputPath := filepath.Join(outputDir, filename)

	// 覆盖检查
	if _, statErr := os.Stat(outputPath); statErr == nil && !opts.Overwrite {
		return nil, fmt.Errorf("文件已存在: %s（使用 --overwrite 覆盖）", outputPath)
	}

	size, err := safefile.AtomicWriteFrom(outputPath, stream, 0o644)
	if err != nil {
		if streamErr := stream.Err(); streamErr != nil {
			return nil, streamErr
		}
		return nil, fmt.Errorf("写入下载文件失败: %w", err)
	}

	return &DownloadResult{
		SavedPath: outputPath,
		Size:      size,
		Filename:  filename,
	}, nil
}

// publicDownloadHTTPClient 返回下载预签名/公开 URL（非 Open API 主机、不携带凭证）用的客户端：
//   - 复用进程级默认 Transport 连接池；不设墙钟超时，超时与取消由请求 context 控制；
//   - 不走 Open API host 白名单（目标本就是对象存储/CDN 主机），SSRF 校验由调用方的 checkRedirect 负责；
//   - checkRedirect 为空时使用 publicRedirectPolicy：最多 10 次、拒绝 HTTPS→HTTP 降级、跨源剥离凭证头。
//
// 禁止用它发送带 Authorization 的 Open API 请求——那类请求必须用 rawHTTPClient()。
func publicDownloadHTTPClient(checkRedirect func(*http.Request, []*http.Request) error) *http.Client {
	if checkRedirect == nil {
		checkRedirect = publicRedirectPolicy
	}
	return &http.Client{
		Transport: http.DefaultTransport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 0 && via[0].URL.Scheme == "https" && req.URL.Scheme == "http" {
				return errors.New("下载重定向不允许从 HTTPS 降级到 HTTP")
			}
			if len(via) > 0 && req.URL.Host != via[len(via)-1].URL.Host {
				req.Header.Del("Authorization")
				req.Header.Del("Cookie")
			}
			return checkRedirect(req, via)
		},
	}
}

func publicRedirectPolicy(_ *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("下载重定向次数超过 10")
	}
	return nil
}

// validateDownloadURL 校验下载 URL 避免 SSRF
// 拒绝非 http/https scheme、本地回环、内网 IP、链路本地等
func validateDownloadURL(rawURL string) error {
	if rawURL == "" {
		return errors.New("下载 URL 为空")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("解析下载 URL 失败: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("下载 URL scheme 非法: %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("下载 URL 缺少 host")
	}

	// 基于主机名的黑名单
	lowered := strings.ToLower(host)
	if lowered == "localhost" || strings.HasSuffix(lowered, ".localhost") {
		return errors.New("下载 URL 不允许指向 localhost")
	}

	// 尝试解析 IP（host 可能本身就是 IP 字面量）
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return fmt.Errorf("下载 URL 指向受限 IP: %s", ip.String())
		}
	}
	return nil
}

// isBlockedIP 判断 IP 是否属于内网/本地/链路本地等受限段
func isBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsPrivate() {
		return true
	}
	return false
}

// resolveFilenameFromResponse 从响应头解析文件名
// 优先级：Content-Disposition filename > Content-Type 扩展推导 > defaultFallback
func resolveFilenameFromResponse(resp *http.Response, defaultFallback string) string {
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if _, params, err := mime.ParseMediaType(cd); err == nil {
			// RFC 5987 兼容：优先 filename*，再 filename
			if v, ok := params["filename*"]; ok && v != "" {
				if decoded, ok := decodeRFC5987(v); ok && decoded != "" {
					return decoded
				}
			}
			if v, ok := params["filename"]; ok && v != "" {
				return v
			}
		}
	}

	// 从 Content-Type 推导扩展名
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		mediaType, _, err := mime.ParseMediaType(ct)
		if err == nil {
			ext := extFromMediaType(mediaType)
			if ext != "" {
				return defaultFallback + ext
			}
		}
	}

	return defaultFallback + ".media"
}

// extFromMediaType 根据 MIME 推导文件扩展名，优先用常见映射，再回退到 mime.ExtensionsByType
func extFromMediaType(mediaType string) string {
	switch strings.ToLower(mediaType) {
	case "video/mp4":
		return ".mp4"
	case "video/quicktime":
		return ".mov"
	case "video/webm":
		return ".webm"
	case "audio/mp4", "audio/x-m4a":
		return ".m4a"
	case "audio/mpeg":
		return ".mp3"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "audio/ogg":
		return ".ogg"
	case "application/pdf":
		return ".pdf"
	case "application/zip":
		return ".zip"
	case "text/plain":
		return ".txt"
	}
	if exts, err := mime.ExtensionsByType(mediaType); err == nil && len(exts) > 0 {
		return exts[0]
	}
	return ""
}

// sanitizeFilename 清洗文件名，剥除路径分隔符和危险字符
func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	name = filepath.Base(name)
	if name == "." || name == ".." || name == "/" || name == `\` {
		return ""
	}
	// 替换不安全字符
	replaced := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', '\x00':
			return '_'
		}
		return r
	}, name)
	// 长度兜底
	if len(replaced) > 200 {
		replaced = replaced[:200]
	}
	return replaced
}

// decodeRFC5987 解码 RFC 5987 格式的 filename*（UTF-8”<encoded>）
func decodeRFC5987(v string) (string, bool) {
	// 形如 UTF-8''%E4%B8%AD%E6%96%87.mp4
	_, encoded, ok := strings.Cut(v, "''")
	if !ok {
		return "", false
	}
	decoded, err := url.QueryUnescape(encoded)
	if err != nil {
		return "", false
	}
	return decoded, true
}
