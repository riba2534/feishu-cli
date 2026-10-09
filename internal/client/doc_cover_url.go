package client

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/runctx"
)

// 封面 --url 下载的安全边界（与官方 docs +resource-update --url 一致）：
// 仅 HTTPS、拒绝 userinfo、拒绝解析到内网/回环/链路本地等地址、最多 3 次跳转（逐跳重新校验）、
// 只接受图片 Content-Type、响应体最大 20MiB。
const (
	CoverURLMaxBytes     = int64(20 * 1024 * 1024)
	coverURLMaxRedirects = 3
	coverURLTimeout      = 30 * time.Second
	coverURLDefaultName  = "cover"
)

// CoverURLSafetyNote 供 --dry-run 展示的安全边界说明。
const CoverURLSafetyNote = "仅 HTTPS；拒绝解析到内网/回环/链路本地地址；最多 3 次跳转；只接受图片 Content-Type；最大 20MiB"

var coverURLAllowedContentTypes = map[string]string{
	"image/bmp":  ".bmp",
	"image/gif":  ".gif",
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/tiff": ".tiff",
	"image/webp": ".webp",
}

// 测试注入点：DNS 解析、受限 IP 判定、TLS 配置（httptest 自签证书）。
var (
	coverURLLookupIP = func(ctx context.Context, host string) ([]net.IP, error) {
		return net.DefaultResolver.LookupIP(ctx, "ip", host)
	}
	coverURLUnsafeIP              = isUnsafeCoverIP
	coverURLTLSConfig *tls.Config = nil
)

// coverURLPolicyError 表示 --url 被安全策略拒绝（用法错误，退出码 2）。
type coverURLPolicyError struct{ msg string }

func (e *coverURLPolicyError) Error() string { return e.msg }

func coverURLPolicyf(format string, a ...any) error {
	return &coverURLPolicyError{msg: fmt.Sprintf(format, a...)}
}

// RemoteImage 是从 URL 下载到内存的图片。
type RemoteImage struct {
	Data        []byte
	FileName    string
	ContentType string
}

// ValidateCoverImageURL 离线校验 --url 语法（https、无 userinfo、host 非空），不做 DNS 解析，可在 --dry-run 前调用。
func ValidateCoverImageURL(raw string) error {
	_, err := parseCoverURLSyntax(raw)
	if err != nil {
		return clierr.Usage(err)
	}
	return nil
}

func parseCoverURLSyntax(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, coverURLPolicyf("--url 无效: %v", err)
	}
	if u.Scheme != "https" {
		return nil, coverURLPolicyf("--url 只支持 https:// 链接，得到 %q", raw)
	}
	if u.User != nil {
		return nil, coverURLPolicyf("--url 不能包含用户名或密码（userinfo）")
	}
	if u.Hostname() == "" {
		return nil, coverURLPolicyf("--url 缺少主机名")
	}
	return u, nil
}

// validateCoverURLHost 解析 host 并拒绝任何落在受限网段的地址（任一解析结果受限即拒绝）。
func validateCoverURLHost(ctx context.Context, host string) error {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return coverURLPolicyf("--url 不能指向本机或内网地址: %s", host)
	}
	if ip := net.ParseIP(host); ip != nil {
		if coverURLUnsafeIP(ip) {
			return coverURLPolicyf("--url 不能指向本机或内网地址: %s", ip)
		}
		return nil
	}
	ips, err := coverURLLookupIP(ctx, host)
	if err != nil {
		return clierr.Network(fmt.Errorf("解析 --url 主机 %s 失败: %w", host, err))
	}
	if len(ips) == 0 {
		return coverURLPolicyf("解析 --url 主机 %s 失败: 没有地址", host)
	}
	for _, ip := range ips {
		if coverURLUnsafeIP(ip) {
			return coverURLPolicyf("--url 主机 %s 解析到本机或内网地址 %s，已拒绝", host, ip)
		}
	}
	return nil
}

// isUnsafeCoverIP 判断 IP 是否属于回环、未指定、组播、链路本地、私有、CGNAT、基准测试或保留网段。
func isUnsafeCoverIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		switch {
		case v4[0] == 0: // 0.0.0.0/8
			return true
		case v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127: // 100.64.0.0/10 CGNAT
			return true
		case v4[0] == 198 && (v4[1] == 18 || v4[1] == 19): // 198.18.0.0/15 基准测试
			return true
		case v4[0] >= 240: // 240.0.0.0/4 保留与广播
			return true
		}
	}
	return false
}

// FetchCoverImageURL 下载 HTTPS 图片到内存（用于设置文档封面），返回内容、建议文件名与 Content-Type。
//
// 除请求前与每次跳转前的 DNS 校验外，直连时在拨号阶段再检查实际连接的 IP（防 DNS rebinding）；
// 经环境变量代理时由代理解析目标，此时依赖请求前与每次跳转前的校验。
func FetchCoverImageURL(raw string) (*RemoteImage, error) {
	u, err := parseCoverURLSyntax(raw)
	if err != nil {
		return nil, clierr.Usage(err)
	}
	ctx, cancel := context.WithTimeout(runctx.Root(), coverURLTimeout)
	defer cancel()
	if err := validateCoverURLHost(ctx, u.Hostname()); err != nil {
		return nil, classifyCoverURLError(err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, clierr.Usagef("--url 无效: %v", err)
	}
	req.Header.Set("Accept", "image/*")
	httpClient := newCoverURLHTTPClient()
	defer httpClient.CloseIdleConnections()
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, classifyCoverURLError(fmt.Errorf("下载封面图片失败: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		statusErr := fmt.Errorf("下载封面图片失败: HTTP %d", resp.StatusCode)
		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusRequestTimeout {
			return nil, clierr.Network(statusErr)
		}
		return nil, statusErr
	}

	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || mediaType == "" {
		return nil, clierr.Usagef("封面 URL 的响应缺少图片 Content-Type，无法确认是图片")
	}
	mediaType = strings.ToLower(mediaType)
	ext, ok := coverURLAllowedContentTypes[mediaType]
	if !ok {
		return nil, clierr.Usagef("封面 URL 的 Content-Type %q 不受支持，只接受 image/png、image/jpeg、image/gif、image/webp、image/bmp、image/tiff", mediaType)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, CoverURLMaxBytes+1))
	if err != nil {
		return nil, classifyCoverURLError(fmt.Errorf("读取封面图片失败: %w", err))
	}
	if int64(len(data)) > CoverURLMaxBytes {
		return nil, clierr.Usagef("封面 URL 的图片超过 20MiB 上限，请先下载压缩后用 --file 上传")
	}
	if len(data) == 0 {
		return nil, clierr.Usagef("封面 URL 返回了空内容")
	}
	finalURL := u
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL
	}
	return &RemoteImage{Data: data, FileName: coverURLFileName(finalURL, ext), ContentType: mediaType}, nil
}

// classifyCoverURLError：安全策略拒绝 → 用法错误（退出码 2）；已分类错误原样返回；其余传输错误 → 网络错误（退出码 4）。
func classifyCoverURLError(err error) error {
	var policy *coverURLPolicyError
	if errors.As(err, &policy) {
		return clierr.Usage(err)
	}
	if len(clierr.Kinds(err)) > 0 {
		return err
	}
	return clierr.Network(err)
}

// coverURLFileName 取 URL 路径最后一段作为文件名，缺扩展名时按 Content-Type 补齐。
func coverURLFileName(u *url.URL, ext string) string {
	base := path.Base(u.EscapedPath())
	if unescaped, err := url.PathUnescape(base); err == nil {
		base = unescaped
	}
	base = sanitizeFilename(base)
	if base == "" {
		return coverURLDefaultName + ext
	}
	if filepath.Ext(base) == "" {
		base += ext
	}
	return base
}

// newCoverURLHTTPClient 构造带 SSRF 防护的外部图片下载客户端（不携带任何飞书凭证）。
func newCoverURLHTTPClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control:   coverURLDialControl(envProxyDialAddrs()),
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: coverURLTimeout,
		ForceAttemptHTTP2:     true,
	}
	if coverURLTLSConfig != nil {
		transport.TLSClientConfig = coverURLTLSConfig.Clone()
	}
	return &http.Client{
		Timeout:   coverURLTimeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= coverURLMaxRedirects {
				return coverURLPolicyf("封面 URL 重定向超过 %d 次", coverURLMaxRedirects)
			}
			if prev := via[len(via)-1]; strings.EqualFold(prev.URL.Scheme, "https") && strings.EqualFold(req.URL.Scheme, "http") {
				return coverURLPolicyf("封面 URL 不允许从 HTTPS 重定向到 HTTP")
			}
			u, err := parseCoverURLSyntax(req.URL.String())
			if err != nil {
				return err
			}
			return validateCoverURLHost(req.Context(), u.Hostname())
		},
	}
}

// coverURLDialControl 在建立 TCP 连接前检查实际要连接的 IP：受限地址一律拒绝，
// 防止请求前校验与拨号之间 DNS 结果被替换（rebinding）。拨号到环境变量代理本身时放行。
func coverURLDialControl(proxyAddrs map[string]struct{}) func(network, address string, c syscall.RawConn) error {
	return func(network, address string, _ syscall.RawConn) error {
		if _, ok := proxyAddrs[address]; ok {
			return nil
		}
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		if ip := net.ParseIP(host); ip == nil || coverURLUnsafeIP(ip) {
			return coverURLPolicyf("--url 实际连接的地址 %s 属于本机或内网网段，已拒绝", host)
		}
		return nil
	}
}

// envProxyDialAddrs 返回环境变量代理（HTTPS_PROXY / HTTP_PROXY 及小写形式）解析后的 ip:port 集合。
func envProxyDialAddrs() map[string]struct{} {
	addrs := make(map[string]struct{})
	for _, key := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		val := strings.TrimSpace(os.Getenv(key))
		if val == "" {
			continue
		}
		if !strings.Contains(val, "://") {
			val = "http://" + val
		}
		pu, err := url.Parse(val)
		if err != nil || pu.Hostname() == "" {
			continue
		}
		port := pu.Port()
		if port == "" {
			switch strings.ToLower(pu.Scheme) {
			case "https":
				port = "443"
			case "socks5", "socks5h":
				port = "1080"
			default:
				port = "80"
			}
		}
		if ip := net.ParseIP(pu.Hostname()); ip != nil {
			addrs[net.JoinHostPort(ip.String(), port)] = struct{}{}
			continue
		}
		ips, err := net.LookupIP(pu.Hostname())
		if err != nil {
			continue
		}
		for _, ip := range ips {
			addrs[net.JoinHostPort(ip.String(), port)] = struct{}{}
		}
	}
	return addrs
}
