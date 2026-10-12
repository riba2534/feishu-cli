package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

// 官方 Open API / Accounts 原点。默认传输只允许这些 HTTPS host。
const (
	OfficialFeishuOpen     = "https://open.feishu.cn"
	OfficialLarkOpen       = "https://open.larksuite.com"
	OfficialFeishuAccounts = "https://accounts.feishu.cn"
	OfficialLarkAccounts   = "https://accounts.larksuite.com"
	OAuthTokenV3Path       = "/oauth/v3/token"
	// TenantTokenInternalPath 是 Open API 自建应用 tenant_access_token 端点（SDK 自取 token 走它）。
	// 网关模式下所有 tenant token 都经 base_url 的这个端点换取。
	TenantTokenInternalPath = "/open-apis/auth/v3/tenant_access_token/internal"
)

const (
	envAllowCustomBaseURL       = "FEISHU_ALLOW_CUSTOM_BASE_URL"
	envAllowInsecureHTTP        = "FEISHU_ALLOW_INSECURE_HTTP"
	envAllowCrossOriginRedirect = "FEISHU_ALLOW_CROSS_ORIGIN_REDIRECT"
	envGateway                  = "FEISHU_GATEWAY"
)

// Brand 标识飞书国内站或 Lark 国际站。
type Brand string

const (
	BrandFeishu Brand = "feishu"
	BrandLark   Brand = "lark"
)

var officialOpenHosts = map[string]struct{}{
	"open.feishu.cn":     {},
	"open.larksuite.com": {},
}

var officialAccountsHosts = map[string]struct{}{
	"accounts.feishu.cn":     {},
	"accounts.larksuite.com": {},
}

func envTruthyValue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func envOrConfigBool(envKey string, cfgVal bool) bool {
	if v, ok := os.LookupEnv(envKey); ok {
		return envTruthyValue(v)
	}
	return cfgVal
}

func configAllowCustomBaseURL() bool {
	if cfg == nil {
		return false
	}
	return cfg.AllowCustomBaseURL
}

func configAllowInsecureHTTP() bool {
	if cfg == nil {
		return false
	}
	return cfg.AllowInsecureHTTP
}

func configAllowCrossOriginRedirect() bool {
	if cfg == nil {
		return false
	}
	return cfg.AllowCrossOriginRedirect
}

func configGateway() bool {
	if cfg == nil {
		return false
	}
	return cfg.Gateway
}

// GatewayMode 是否启用网关模式（配置 gateway: true 或 FEISHU_GATEWAY=1）。
// 网关模式下 base_url 指向一个代持 App 凭证的网关：本地不需要 app_secret，所有 token
// 都经 base_url 换取，且不会直连官方 Open API / Accounts host。
func GatewayMode() bool {
	return envOrConfigBool(envGateway, configGateway())
}

// AllowCustomBaseURL 是否允许非官方远端 host（配置或环境变量显式 opt-in）。
func AllowCustomBaseURL() bool {
	return envOrConfigBool(envAllowCustomBaseURL, configAllowCustomBaseURL())
}

// AllowInsecureHTTP 是否允许非 loopback 的 HTTP 或 HTTPS→HTTP 降级。
func AllowInsecureHTTP() bool {
	return envOrConfigBool(envAllowInsecureHTTP, configAllowInsecureHTTP())
}

// AllowCrossOriginRedirect 是否允许带 body 的跨源重定向（默认拒绝，防止 App Secret 外送）。
func AllowCrossOriginRedirect() bool {
	return envOrConfigBool(envAllowCrossOriginRedirect, configAllowCrossOriginRedirect())
}

// ParseBrand 从 Open API base URL 解析品牌；无法识别时默认为飞书国内站。
func ParseBrand(baseURL string) Brand {
	if strings.Contains(strings.ToLower(baseURL), "larksuite.com") {
		return BrandLark
	}
	return BrandFeishu
}

// OfficialOpenBase 返回该品牌的官方 Open API 原点。
func OfficialOpenBase(brand Brand) string {
	if brand == BrandLark {
		return OfficialLarkOpen
	}
	return OfficialFeishuOpen
}

// OfficialAccountsBase 返回该品牌的官方 Accounts 原点。
func OfficialAccountsBase(brand Brand) string {
	if brand == BrandLark {
		return OfficialLarkAccounts
	}
	return OfficialFeishuAccounts
}

func hostnameOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// IsOfficialOpenHost 精确匹配官方 Open API host，不做后缀匹配。
func IsOfficialOpenHost(host string) bool {
	_, ok := officialOpenHosts[strings.ToLower(host)]
	return ok
}

// IsOfficialAccountsHost 精确匹配官方 Accounts host。
func IsOfficialAccountsHost(host string) bool {
	_, ok := officialAccountsHosts[strings.ToLower(host)]
	return ok
}

// IsLoopbackHost 判断是否为本机回环地址（开发/测试 httptest 用）。
func IsLoopbackHost(host string) bool {
	h := strings.TrimSpace(host)
	if h == "" {
		return false
	}
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func officialHTTPSPortOK(u *url.URL) bool {
	if u == nil || !strings.EqualFold(u.Scheme, "https") {
		return false
	}
	port := u.Port()
	return port == "" || port == "443"
}

func parseAbsoluteURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("URL 为空")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("解析 URL 失败: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("URL 必须包含 scheme 与 host: %s", raw)
	}
	if u.User != nil {
		return nil, fmt.Errorf("拒绝带 userinfo 的 URL（可能泄漏凭证）")
	}
	return u, nil
}

func rejectRemoteHTTP(u *url.URL) error {
	if strings.EqualFold(u.Scheme, "http") && !IsLoopbackHost(u.Hostname()) && !AllowInsecureHTTP() {
		return fmt.Errorf("拒绝非 loopback 的 HTTP 端点 %s。默认只允许官方 HTTPS；明文 HTTP 仅用于本机开发/测试。若确需远端 HTTP，请设置 FEISHU_ALLOW_INSECURE_HTTP=1 与 FEISHU_ALLOW_CUSTOM_BASE_URL=1", u.Host)
	}
	return nil
}

func rejectCustomHost(u *url.URL, official bool) error {
	if official {
		return nil
	}
	if IsLoopbackHost(u.Hostname()) {
		return nil
	}
	if !AllowCustomBaseURL() {
		return fmt.Errorf("拒绝自定义远端 host %q。默认只允许官方 HTTPS（open.feishu.cn / open.larksuite.com）。若确需私有化或测试域名，请设置 FEISHU_ALLOW_CUSTOM_BASE_URL=1（或配置 allow_custom_base_url: true）", u.Hostname())
	}
	return nil
}

var errGatewayNeedsBaseURL = errors.New("网关模式（gateway: true / FEISHU_GATEWAY=1）需要把 base_url 指向网关（loopback，或配合 allow_custom_base_url 的自定义 host），不能是官方地址")

// CheckBaseURL 校验 Open API base_url：官方 HTTPS、loopback，或显式 opt-in 的自定义/明文端点。
func CheckBaseURL(raw string) error {
	raw = strings.TrimSpace(strings.TrimRight(raw, "/"))
	if raw == "" || raw == OfficialFeishuOpen || raw == OfficialLarkOpen {
		if GatewayMode() {
			return errGatewayNeedsBaseURL
		}
		return nil
	}
	u, err := parseAbsoluteURL(raw)
	if err != nil {
		return fmt.Errorf("base_url 无效: %w", err)
	}
	if err := rejectRemoteHTTP(u); err != nil {
		return err
	}
	host := u.Hostname()
	if GatewayMode() && (IsOfficialOpenHost(host) || IsOfficialAccountsHost(host)) {
		return errGatewayNeedsBaseURL
	}
	official := IsOfficialOpenHost(host) && officialHTTPSPortOK(u)
	if official {
		return nil
	}
	if strings.EqualFold(u.Scheme, "https") && IsOfficialOpenHost(host) && !officialHTTPSPortOK(u) {
		return fmt.Errorf("官方 host %s 只允许 HTTPS 默认端口，当前为 %s", host, u.Host)
	}
	if !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("base_url 只支持 http/https，得到 %s", u.Scheme)
	}
	return rejectCustomHost(u, false)
}

// CheckRequestURL 校验实际发出的请求 URL（Open API、Accounts、loopback 或已 opt-in 的自定义端点）。
func CheckRequestURL(u *url.URL) error {
	if u == nil {
		return fmt.Errorf("请求 URL 为空")
	}
	if u.User != nil {
		return fmt.Errorf("拒绝带 userinfo 的请求 URL（可能泄漏凭证）")
	}
	if u.Scheme == "" || u.Hostname() == "" {
		return fmt.Errorf("请求 URL 必须包含 scheme 与 host")
	}
	if !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("请求 URL 只支持 http/https，得到 %s", u.Scheme)
	}
	if err := rejectRemoteHTTP(u); err != nil {
		return err
	}
	host := u.Hostname()
	if IsLoopbackHost(host) {
		return nil
	}
	if GatewayMode() && (IsOfficialOpenHost(host) || IsOfficialAccountsHost(host)) {
		return fmt.Errorf("网关模式下拒绝直连官方 host %s：所有请求只经 base_url 指向的网关发出", host)
	}
	if (IsOfficialOpenHost(host) || IsOfficialAccountsHost(host)) && officialHTTPSPortOK(u) {
		return nil
	}
	return rejectCustomHost(u, false)
}

// ResolveAccountsBase 从 Open API base_url 得到 Accounts 原点。
// 默认只返回官方 Accounts；自定义 open.X → accounts.X 仅在 opt-in 后生效，避免把 App Secret 送到攻击者域名。
func ResolveAccountsBase(openBaseURL string) string {
	brand := ParseBrand(openBaseURL)
	official := OfficialAccountsBase(brand)
	raw := strings.TrimSpace(strings.TrimRight(openBaseURL, "/"))
	if raw == "" || raw == OfficialFeishuOpen || raw == OfficialLarkOpen {
		return official
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return official
	}
	host := strings.ToLower(u.Hostname())
	if IsOfficialAccountsHost(host) && officialHTTPSPortOK(u) {
		return strings.TrimRight(u.Scheme+"://"+u.Host, "/")
	}
	if IsOfficialOpenHost(host) {
		return official
	}
	if !AllowCustomBaseURL() {
		return official
	}
	if strings.HasPrefix(host, "open.") {
		cloned := *u
		cloned.Host = "accounts." + strings.TrimPrefix(host, "open.")
		if port := u.Port(); port != "" {
			cloned.Host = cloned.Hostname() + ":" + port
		}
		cloned.Path = ""
		cloned.RawQuery = ""
		cloned.Fragment = ""
		return strings.TrimRight(cloned.String(), "/")
	}
	return official
}

// ResolveOpenBase 归一化 Open API 原点：空值走官方默认；自定义需已通过 CheckBaseURL。
func ResolveOpenBase(openBaseURL string) string {
	raw := strings.TrimSpace(strings.TrimRight(openBaseURL, "/"))
	if raw == "" {
		return OfficialFeishuOpen
	}
	return raw
}
