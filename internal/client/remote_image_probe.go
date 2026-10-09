package client

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"
)

// 远程图片可用性探测（doc script parse 的资源预检，对齐官方 docs +script 的 probeRemoteDocImageDownload）：
//   - 只发一次 Range: bytes=0-0 的 GET，不缓存图片内容；
//   - SSRF 防护：URL 必须是不带凭据的 http(s)，主机不能是 localhost / 内网 / 保留地址（DNS 解析后逐个校验），
//     每次重定向重新校验，最多 5 次；未配置代理时在建立 TCP 连接前再拦一次受限 IP（防 DNS rebinding）；
//   - 失败按 RemoteImageProbeKind 分类，调用方据此给出稳定的诊断码。

// RemoteImageMaxBytes 是远程图片的大小上限（与写入时 CLI 下载上传的上限一致：20MiB）。
const RemoteImageMaxBytes = int64(20 * 1024 * 1024)

const (
	remoteImageProbeTimeout      = 20 * time.Second
	remoteImageProbeMaxRedirects = 5
)

// RemoteImageProbeKind 是远程图片探测失败的分类。
type RemoteImageProbeKind string

const (
	RemoteImageSourceDisallowed RemoteImageProbeKind = "source_disallowed" // URL 非法或指向本地 / 内网
	RemoteImageUnavailable      RemoteImageProbeKind = "unavailable"       // 网络错误或非 2xx
	RemoteImageFormat           RemoteImageProbeKind = "format"            // Content-Type 不是支持的图片
	RemoteImageTooLarge         RemoteImageProbeKind = "too_large"         // 超过 20MiB
)

// RemoteImageProbeError 是远程图片探测失败的结构化错误；Error() 只返回原因（不含图片序号）。
type RemoteImageProbeError struct {
	Kind   RemoteImageProbeKind
	Reason string
	Err    error
}

func (e *RemoteImageProbeError) Error() string { return e.Reason }
func (e *RemoteImageProbeError) Unwrap() error { return e.Err }

// remoteImageAllowedTypes 是飞书文档图片接受的 Content-Type（BMP / GIF / JPEG / PNG / TIFF / WebP）。
var remoteImageAllowedTypes = map[string]bool{
	"image/bmp": true, "image/gif": true, "image/jpeg": true,
	"image/png": true, "image/tiff": true, "image/webp": true,
}

// 测试注入点：httptest 地址是 loopback，会被 SSRF 防护拒绝。
var (
	remoteImageLookupIP = func(ctx context.Context, host string) ([]net.IP, error) {
		return net.DefaultResolver.LookupIP(ctx, "ip", host)
	}
	remoteImageProbeDo = func(c *http.Client, req *http.Request) (*http.Response, error) {
		return c.Do(req)
	}
)

// ValidateRemoteImageSource 校验远程图片 URL 可被安全访问（scheme / 凭据 / 主机解析后的 IP）。
func ValidateRemoteImageSource(ctx context.Context, rawURL string) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u == nil {
		return &RemoteImageProbeError{Kind: RemoteImageSourceDisallowed, Reason: "URL 非法", Err: err}
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return &RemoteImageProbeError{Kind: RemoteImageSourceDisallowed, Reason: "只支持 http/https 图片 URL"}
	}
	if u.User != nil {
		return &RemoteImageProbeError{Kind: RemoteImageSourceDisallowed, Reason: "URL 不能包含用户名或密码"}
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		return &RemoteImageProbeError{Kind: RemoteImageSourceDisallowed, Reason: "URL 缺少主机名"}
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return &RemoteImageProbeError{Kind: RemoteImageSourceDisallowed, Reason: "不允许访问本地/内网地址"}
	}
	if ip := net.ParseIP(host); ip != nil {
		if isRestrictedRemoteImageIP(ip) {
			return &RemoteImageProbeError{Kind: RemoteImageSourceDisallowed, Reason: "不允许访问本地/内网地址"}
		}
		return nil
	}
	ips, err := remoteImageLookupIP(ctx, host)
	if err != nil || len(ips) == 0 {
		return &RemoteImageProbeError{Kind: RemoteImageSourceDisallowed, Reason: "无法解析图片 URL 的主机名", Err: err}
	}
	for _, ip := range ips {
		if isRestrictedRemoteImageIP(ip) {
			return &RemoteImageProbeError{Kind: RemoteImageSourceDisallowed, Reason: "不允许访问本地/内网地址"}
		}
	}
	return nil
}

// ProbeRemoteImage 用 Range: bytes=0-0 的 GET 探测远程图片是否可下载、类型与大小是否可接受。
// Content-Type 缺失时视为通过（与官方一致）；失败返回 *RemoteImageProbeError。
func ProbeRemoteImage(ctx context.Context, rawURL string) error {
	if ctx == nil {
		ctx = Context()
	}
	ctx, cancel := context.WithTimeout(ctx, remoteImageProbeTimeout)
	defer cancel()
	if err := ValidateRemoteImageSource(ctx, rawURL); err != nil {
		return err
	}
	httpClient, closeIdle := newRemoteImageProbeClient(ctx)
	defer closeIdle()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(rawURL), nil)
	if err != nil {
		return &RemoteImageProbeError{Kind: RemoteImageSourceDisallowed, Reason: "URL 非法", Err: err}
	}
	req.Header.Set("Range", "bytes=0-0")
	resp, err := remoteImageProbeDo(httpClient, req)
	if err != nil {
		var probeErr *RemoteImageProbeError
		if errors.As(err, &probeErr) {
			return probeErr
		}
		return &RemoteImageProbeError{Kind: RemoteImageUnavailable, Reason: "可用性探测失败: " + err.Error(), Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &RemoteImageProbeError{Kind: RemoteImageUnavailable, Reason: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	if resp.ContentLength > RemoteImageMaxBytes {
		return &RemoteImageProbeError{Kind: RemoteImageTooLarge, Reason: "超过 20MiB 上限"}
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		return nil
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return &RemoteImageProbeError{Kind: RemoteImageFormat, Reason: "响应的 Content-Type 非法", Err: err}
	}
	if !remoteImageAllowedTypes[strings.ToLower(mediaType)] {
		return &RemoteImageProbeError{Kind: RemoteImageFormat, Reason: fmt.Sprintf("响应的 Content-Type %q 不是支持的图片类型", mediaType)}
	}
	return nil
}

// newRemoteImageProbeClient 返回探测用的公网客户端（不携带任何凭证）与释放连接的函数。
func newRemoteImageProbeClient(ctx context.Context) (*http.Client, func()) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if !remoteImageProxyConfigured() {
		// 未走代理时在 TCP 握手前拦截受限 IP；走代理时连接目标是代理本身，由上面的 DNS 预校验兜底
		dialer := &net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
			Control: func(_, address string, _ syscall.RawConn) error {
				host, _, err := net.SplitHostPort(address)
				if err != nil {
					return err
				}
				if ip := net.ParseIP(host); ip != nil && isRestrictedRemoteImageIP(ip) {
					return &RemoteImageProbeError{Kind: RemoteImageSourceDisallowed, Reason: "不允许访问本地/内网地址"}
				}
				return nil
			},
		}
		transport.DialContext = dialer.DialContext
	}
	httpClient := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= remoteImageProbeMaxRedirects {
				return fmt.Errorf("重定向次数超过 %d", remoteImageProbeMaxRedirects)
			}
			if len(via) > 0 && via[0].URL.Scheme == "https" && req.URL.Scheme == "http" {
				return &RemoteImageProbeError{Kind: RemoteImageSourceDisallowed, Reason: "重定向不允许从 HTTPS 降级到 HTTP"}
			}
			return ValidateRemoteImageSource(ctx, req.URL.String())
		},
	}
	return httpClient, transport.CloseIdleConnections
}

func remoteImageProxyConfigured() bool {
	for _, key := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy"} {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return true
		}
	}
	return false
}

// isRestrictedRemoteImageIP 判断 IP 是否属于本地、内网、链路本地、CGNAT、基准测试或保留地址段。
func isRestrictedRemoteImageIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if isBlockedIP(ip) || ip.IsInterfaceLocalMulticast() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		switch {
		case v4[0] == 0: // RFC 1122 "this network"
			return true
		case v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127: // RFC 6598 CGNAT
			return true
		case v4[0] == 198 && (v4[1] == 18 || v4[1] == 19): // RFC 2544 基准测试
			return true
		case v4[0] >= 240: // 保留与广播
			return true
		}
		return false
	}
	// IPv6 唯一本地地址 fc00::/7 已由 IsPrivate 覆盖；额外拦截 IPv4 映射之外的文档前缀 2001:db8::/32
	return len(ip) == net.IPv6len && ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x0d && ip[3] == 0xb8
}
