// 部分实现改编自 larksuite/cli（MIT License, Copyright (c) 2026 Lark Technologies Pte. Ltd.）

package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"

	// 远程图片按声明的 Content-Type 校验真实格式，注册全部受支持的解码器（只读文件头）
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// 文档写入中 <img href="https://..."/> 的远程图片下载（对齐官方 local_doc_resources 的 remote image 流程）：
//   - 只接受绝对 http(s) URL，不能带 userinfo；拒绝 localhost / 内网 / 链路本地等受限地址（DNS 解析后逐个校验）；
//   - 独立的受控下载客户端：目标是用户给的公网地址，不走 Open API host 白名单，也绝不携带飞书凭证；
//     直连时在建连阶段再校验一次对端 IP（防 DNS rebinding），每次重定向重新校验，最多 5 次，禁止 HTTPS→HTTP 降级；
//   - Content-Type 必须是 BMP / GIF / JPEG / PNG / TIFF / WebP，内容必须与声明的格式一致；上限 20MiB，空响应报错。

// RemoteImageMaxBytes 是远程图片的大小上限（20MiB）。
const RemoteImageMaxBytes = int64(20 * 1024 * 1024)

const remoteImageMaxRedirects = 5

// RemoteImageContentTypes 是支持的图片 Content-Type 及上传时使用的扩展名。
var RemoteImageContentTypes = map[string]string{
	"image/bmp":  ".bmp",
	"image/gif":  ".gif",
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/tiff": ".tiff",
	"image/webp": ".webp",
}

// RemoteImage 是下载到内存的远程图片。
type RemoteImage struct {
	Content  []byte
	FileName string // image.<ext>
	Width    int
	Height   int
}

// RemoteImageError 是远程图片下载失败；Retryable 表示网络抖动、429、5xx 等可重试的失败。
type RemoteImageError struct {
	Msg       string
	Retryable bool
	Err       error
}

func (e *RemoteImageError) Error() string {
	if e.Err != nil {
		return e.Msg + ": " + e.Err.Error()
	}
	return e.Msg
}

func (e *RemoteImageError) Unwrap() error { return e.Err }

// IsRetryableRemoteImageError 判断远程图片下载错误是否值得重试。
func IsRetryableRemoteImageError(err error) bool {
	var re *RemoteImageError
	return errors.As(err, &re) && re.Retryable
}

// errRemoteImageBlocked 标记本地安全策略拒绝（受限地址、重定向违规），这类错误重试无意义。
var errRemoteImageBlocked = errors.New("远程图片地址被安全策略拒绝")

// remoteImageLookupIP 与 remoteImageAllowLoopback 供测试替换（httptest 地址是 loopback）。
var (
	remoteImageLookupIP      = net.DefaultResolver.LookupIP
	remoteImageAllowLoopback = false
)

// ParseRemoteImageURL 离线校验 href：绝对 http(s) URL、有 host、不带 userinfo。返回规范化后的 URL。
func ParseRemoteImageURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	u, err := url.Parse(rawURL)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || strings.TrimSpace(u.Hostname()) == "" || u.User != nil {
		return "", errors.New("href 必须是不带用户名密码的绝对 http(s) URL")
	}
	return u.String(), nil
}

// ValidateRemoteImageURL 校验 href 并解析 DNS：任一解析结果落在受限网段即拒绝（写入文档前调用，失败即用法错误）。
func ValidateRemoteImageURL(ctx context.Context, rawURL string) error {
	if _, err := ParseRemoteImageURL(rawURL); err != nil {
		return err
	}
	u, _ := url.Parse(rawURL)
	_, err := resolveRemoteImageHost(ctx, u.Hostname())
	return err
}

func resolveRemoteImageHost(ctx context.Context, rawHost string) ([]net.IP, error) {
	host := strings.TrimSpace(strings.ToLower(rawHost))
	if host == "" {
		return nil, errors.New("URL 缺少 host")
	}
	if !remoteImageAllowLoopback && (host == "localhost" || strings.HasSuffix(host, ".localhost")) {
		return nil, errors.New("不允许指向本机或内网地址")
	}
	if ip := net.ParseIP(host); ip != nil {
		if isRestrictedRemoteImageIP(ip) {
			return nil, errors.New("不允许指向本机或内网地址")
		}
		return []net.IP{ip}, nil
	}
	ips, err := remoteImageLookupIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil, fmt.Errorf("无法解析主机 %s", host)
	}
	for _, ip := range ips {
		if isRestrictedRemoteImageIP(ip) {
			return nil, errors.New("不允许指向本机或内网地址")
		}
	}
	return ips, nil
}

// isRestrictedRemoteImageIP 判断 IP 是否属于本机、内网、链路本地、CGNAT、保留等受限网段（同官方 isRestrictedDownloadIP）。
func isRestrictedRemoteImageIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if remoteImageAllowLoopback && ip.IsLoopback() {
		return false
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		switch {
		case v4[0] == 0, v4[0] >= 240:
			return true
		case v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127: // RFC6598 CGNAT
			return true
		case v4[0] == 198 && (v4[1] == 18 || v4[1] == 19): // RFC2544 benchmarking
			return true
		}
		return false
	}
	return ip.To16() == nil
}

// remoteImageHTTPClient 返回下载远程图片的受控客户端（不带任何凭证头）。
func remoteImageHTTPClient() *http.Client {
	base, _ := http.DefaultTransport.(*http.Transport)
	var transport *http.Transport
	if base != nil {
		transport = base.Clone()
	} else {
		transport = &http.Transport{Proxy: http.ProxyFromEnvironment}
	}
	proxies := envProxyAddrs()
	dialer := &net.Dialer{
		Control: func(_, address string, _ syscall.RawConn) error { return checkRemoteImageDialAddr(address) },
	}
	plain := &net.Dialer{}
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		// 经环境变量代理时连接的是代理本身（可能在内网），目标地址已在请求前与每次重定向时按 DNS 校验
		if proxies[strings.ToLower(addr)] {
			return plain.DialContext(ctx, network, addr)
		}
		return dialer.DialContext(ctx, network, addr)
	}
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= remoteImageMaxRedirects {
				return fmt.Errorf("%w：重定向次数超过 %d 次", errRemoteImageBlocked, remoteImageMaxRedirects)
			}
			if prev := via[len(via)-1]; strings.EqualFold(prev.URL.Scheme, "https") && strings.EqualFold(req.URL.Scheme, "http") {
				return fmt.Errorf("%w：不允许从 HTTPS 重定向到 HTTP", errRemoteImageBlocked)
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("%w：重定向目标只能是 http(s)", errRemoteImageBlocked)
			}
			if req.URL.User != nil {
				return fmt.Errorf("%w：重定向目标不能带用户名密码", errRemoteImageBlocked)
			}
			if _, err := resolveRemoteImageHost(req.Context(), req.URL.Hostname()); err != nil {
				return fmt.Errorf("%w：重定向目标 %s 不允许（%v）", errRemoteImageBlocked, req.URL.Hostname(), err)
			}
			return nil
		},
	}
}

// checkRemoteImageDialAddr 在建连时校验对端 IP（address 为已解析的 ip:port），防 DNS rebinding。
func checkRemoteImageDialAddr(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	if isRestrictedRemoteImageIP(net.ParseIP(host)) {
		return fmt.Errorf("%w：%s 属于本机或内网地址", errRemoteImageBlocked, host)
	}
	return nil
}

// envProxyAddrs 返回环境变量中配置的代理地址（host:port，小写）。
func envProxyAddrs() map[string]bool {
	out := map[string]bool{}
	for _, key := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy"} {
		v := strings.TrimSpace(os.Getenv(key))
		if v == "" {
			continue
		}
		if !strings.Contains(v, "://") {
			v = "http://" + v
		}
		u, err := url.Parse(v)
		if err != nil || u.Hostname() == "" {
			continue
		}
		port := u.Port()
		if port == "" {
			port = map[string]string{"https": "443", "socks5": "1080"}[u.Scheme]
			if port == "" {
				port = "80"
			}
		}
		out[strings.ToLower(net.JoinHostPort(u.Hostname(), port))] = true
	}
	return out
}

// DownloadRemoteImage 下载远程图片到内存并校验类型、大小与真实格式。
func DownloadRemoteImage(ctx context.Context, rawURL string) (*RemoteImage, error) {
	if err := ValidateRemoteImageURL(ctx, rawURL); err != nil {
		return nil, &RemoteImageError{Msg: "远程图片地址不允许", Err: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, &RemoteImageError{Msg: "构造远程图片请求失败", Err: err}
	}
	resp, err := remoteImageHTTPClient().Do(req)
	if err != nil {
		return nil, &RemoteImageError{Msg: "下载远程图片失败", Retryable: ctx.Err() == nil && !errors.Is(err, errRemoteImageBlocked), Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &RemoteImageError{
			Msg:       fmt.Sprintf("下载远程图片失败: HTTP %d", resp.StatusCode),
			Retryable: resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500,
		}
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, &RemoteImageError{Msg: "远程图片响应的 Content-Type 无效", Err: err}
	}
	mediaType = strings.ToLower(mediaType)
	ext, ok := RemoteImageContentTypes[mediaType]
	if !ok {
		return nil, &RemoteImageError{Msg: fmt.Sprintf("远程图片 Content-Type %q 不受支持（只支持 BMP / GIF / JPEG / PNG / TIFF / WebP）", mediaType)}
	}
	if resp.ContentLength > RemoteImageMaxBytes {
		return nil, &RemoteImageError{Msg: "远程图片超过 20MiB 上限"}
	}
	content, err := io.ReadAll(io.LimitReader(resp.Body, RemoteImageMaxBytes+1))
	if err != nil {
		return nil, &RemoteImageError{Msg: "读取远程图片失败", Retryable: ctx.Err() == nil, Err: err}
	}
	if len(content) == 0 {
		return nil, &RemoteImageError{Msg: "远程图片响应为空"}
	}
	if int64(len(content)) > RemoteImageMaxBytes {
		return nil, &RemoteImageError{Msg: "远程图片超过 20MiB 上限"}
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, &RemoteImageError{Msg: fmt.Sprintf("远程图片内容不是有效的 %s 图片", mediaType), Err: err}
	}
	if format != strings.TrimPrefix(mediaType, "image/") {
		return nil, &RemoteImageError{Msg: fmt.Sprintf("远程图片声明为 %s，实际内容是 %s", mediaType, format)}
	}
	return &RemoteImage{Content: content, FileName: "image" + ext, Width: cfg.Width, Height: cfg.Height}, nil
}
