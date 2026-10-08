package config

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/apidiag"
)

const defaultRedirectLimit = 10

// NewHTTPClient 返回带超时与重定向凭证策略的 HTTP 客户端。
// 调用方仍须用 LimitReader 限制响应体大小。
func NewHTTPClient(timeout time.Duration) *http.Client {
	return NewHTTPClientWithTransport(nil, timeout)
}

// NewHTTPClientWithTransport 使用指定底层 Transport；timeout=0 表示不设客户端墙钟超时，改由请求 context 控制。
func NewHTTPClientWithTransport(base http.RoundTripper, timeout time.Duration) *http.Client {
	if base == nil {
		if t, ok := http.DefaultTransport.(*http.Transport); ok {
			base = t.Clone()
		} else {
			base = http.DefaultTransport
		}
	}
	return &http.Client{
		Transport:     &policyTransport{base: base},
		Timeout:       timeout,
		CheckRedirect: RedirectPolicy,
	}
}

type policyTransport struct {
	base http.RoundTripper
}

func (t *policyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("请求 URL 为空")
	}
	if err := CheckRequestURL(req.URL); err != nil {
		return nil, err
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err == nil {
		// 旁路记录飞书错误信封里的 log_id / 缺失 scope / 字段校验，供根命令打印诊断
		apidiag.ObserveResponse(resp)
	}
	return resp, err
}

// RedirectPolicy 拒绝默认的 HTTPS→HTTP 与带 body 跨源重定向，并在跨源时剥离 Authorization。
func RedirectPolicy(req *http.Request, via []*http.Request) error {
	if len(via) >= defaultRedirectLimit {
		return fmt.Errorf("重定向超过 %d 次", defaultRedirectLimit)
	}
	if req == nil || req.URL == nil || len(via) == 0 || via[0] == nil || via[0].URL == nil {
		return nil
	}
	prev := via[len(via)-1]
	if prev == nil || prev.URL == nil {
		return nil
	}

	downgrade := strings.EqualFold(prev.URL.Scheme, "https") && strings.EqualFold(req.URL.Scheme, "http")
	crossOrigin := !sameOrigin(prev.URL, req.URL)

	if crossOrigin || downgrade {
		// net/http 在调用 CheckRedirect 前已复制 header，必须在这里剥掉凭证。
		stripCredentialHeaders(req)
	}

	if downgrade && !AllowInsecureHTTP() {
		return fmt.Errorf("拒绝 HTTPS 降级到 HTTP 的重定向（%s → %s）。若确需明文传输，请设置 FEISHU_ALLOW_INSECURE_HTTP=1", originName(prev.URL), originName(req.URL))
	}

	if crossOrigin && redirectKeepsBody(req, via[0]) && !AllowCrossOriginRedirect() {
		return fmt.Errorf("拒绝带 body 的跨源重定向（%s → %s），以免把 App Secret 等凭证发到其他主机。确需时设置 FEISHU_ALLOW_CROSS_ORIGIN_REDIRECT=1", originName(prev.URL), originName(req.URL))
	}

	if err := CheckRequestURL(req.URL); err != nil {
		return fmt.Errorf("重定向目标不被允许: %w", err)
	}
	return nil
}

func stripCredentialHeaders(req *http.Request) {
	if req == nil {
		return
	}
	req.Header.Del("Authorization")
	req.Header.Del("Proxy-Authorization")
	req.Header.Del("Cookie")
	req.Header.Del("Set-Cookie")
}

func redirectKeepsBody(req *http.Request, original *http.Request) bool {
	if req == nil {
		return false
	}
	if req.ContentLength > 0 {
		return true
	}
	if req.Body != nil && req.Body != http.NoBody {
		return true
	}
	if original == nil {
		return false
	}
	switch original.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return req.Method == original.Method
	default:
		return false
	}
}

func sameOrigin(left, right *url.URL) bool {
	if left == nil || right == nil {
		return false
	}
	return strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(left.Hostname(), right.Hostname()) &&
		originPort(left) == originPort(right)
}

func originPort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	switch strings.ToLower(u.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func originName(u *url.URL) string {
	if u == nil {
		return ""
	}
	return strings.ToLower(u.Scheme) + "://" + u.Host
}
