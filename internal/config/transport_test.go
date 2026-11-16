package config

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCheckBaseURL_OfficialAndLoopback(t *testing.T) {
	t.Setenv(envAllowCustomBaseURL, "")
	t.Setenv(envAllowInsecureHTTP, "")
	resetConfig()

	cases := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{"empty", "", false},
		{"feishu", OfficialFeishuOpen, false},
		{"feishu slash", "https://open.feishu.cn/", false},
		{"lark", OfficialLarkOpen, false},
		{"loopback http", "http://127.0.0.1:1234", false},
		{"localhost http", "http://localhost:8080", false},
		{"ipv6 loopback", "http://[::1]:9", false},
		{"loopback ftp", "ftp://127.0.0.1:21", true},
		{"custom https", "https://private.example.com", true},
		{"remote http", "http://evil.example.com", true},
		{"official http downgrade", "http://open.feishu.cn", true},
		{"lookalike suffix", "https://custom.feishu.cn", true},
		{"userinfo", "https://user:pass@open.feishu.cn", true},
		{"non https scheme", "ftp://open.feishu.cn", true},
		{"official nondefault port", "https://open.feishu.cn:8443", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckBaseURL(tc.raw)
			if tc.wantErr && err == nil {
				t.Fatalf("CheckBaseURL(%q) 应失败", tc.raw)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("CheckBaseURL(%q) 应成功: %v", tc.raw, err)
			}
		})
	}
}

func TestCheckBaseURL_CustomHTTPSOptIn(t *testing.T) {
	resetConfig()
	t.Setenv(envAllowCustomBaseURL, "1")
	if err := CheckBaseURL("https://private.example.com"); err != nil {
		t.Fatalf("opt-in 后自定义 HTTPS 应允许: %v", err)
	}
}

func TestCheckBaseURL_RemoteHTTPOptIn(t *testing.T) {
	resetConfig()
	t.Setenv(envAllowCustomBaseURL, "1")
	t.Setenv(envAllowInsecureHTTP, "true")
	if err := CheckBaseURL("http://private.example.com"); err != nil {
		t.Fatalf("双重 opt-in 后远端 HTTP 应允许: %v", err)
	}
}

func TestResolveAccountsBase_DoesNotExfiltrateToCustomHost(t *testing.T) {
	resetConfig()
	t.Setenv(envAllowCustomBaseURL, "")
	got := ResolveAccountsBase("https://open.evil.example")
	if got != OfficialFeishuAccounts {
		t.Fatalf("未 opt-in 时自定义 open.X 不得映射到 accounts.X，得到 %q", got)
	}

	t.Setenv(envAllowCustomBaseURL, "1")
	got = ResolveAccountsBase("https://open.evil.example")
	if got != "https://accounts.evil.example" {
		t.Fatalf("opt-in 后应映射 accounts.evil.example，得到 %q", got)
	}

	if got := ResolveAccountsBase("https://open.larksuite.com"); got != OfficialLarkAccounts {
		t.Fatalf("lark 品牌应解析到 %s，得到 %q", OfficialLarkAccounts, got)
	}
}

func TestRedirectPolicy_StripsAuthorizationOnCrossOrigin(t *testing.T) {
	resetConfig()
	src, _ := url.Parse("https://open.feishu.cn/open-apis/x")
	dst, _ := url.Parse("https://accounts.feishu.cn/oauth/v3/token")
	req, _ := http.NewRequest(http.MethodGet, dst.String(), nil)
	req.Header.Set("Authorization", "Bearer u-secret-token")
	via := []*http.Request{{URL: src, Method: http.MethodGet, Header: req.Header.Clone()}}
	if err := RedirectPolicy(req, via); err != nil {
		t.Fatalf("官方跨源 GET 应允许: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("跨源重定向后仍携带 Authorization")
	}
}

func TestRedirectPolicy_RejectsHTTPSToHTTP(t *testing.T) {
	resetConfig()
	t.Setenv(envAllowInsecureHTTP, "")
	src, _ := url.Parse("https://open.feishu.cn/a")
	dst, _ := url.Parse("http://127.0.0.1/a")
	req, _ := http.NewRequest(http.MethodGet, dst.String(), nil)
	req.Header.Set("Authorization", "Bearer u-secret-token")
	via := []*http.Request{{URL: src, Method: http.MethodGet}}
	err := RedirectPolicy(req, via)
	if err == nil {
		t.Fatal("HTTPS→HTTP 默认应拒绝")
	}
	if strings.Contains(err.Error(), "u-secret-token") {
		t.Fatalf("错误信息泄漏 token: %v", err)
	}
	if req.Header.Get("Authorization") != "" {
		t.Fatal("降级重定向仍应剥离 Authorization")
	}
}

func TestRedirectPolicy_RejectsCrossOriginPOSTBody(t *testing.T) {
	resetConfig()
	t.Setenv(envAllowCrossOriginRedirect, "")
	src, _ := url.Parse("https://accounts.feishu.cn/oauth/v3/token")
	dst, _ := url.Parse("https://open.feishu.cn/steal")
	req, _ := http.NewRequest(http.MethodPost, dst.String(), strings.NewReader("client_secret=super-secret"))
	req.Header.Set("Authorization", "Basic abc")
	orig, _ := http.NewRequest(http.MethodPost, src.String(), strings.NewReader("client_secret=super-secret"))
	err := RedirectPolicy(req, []*http.Request{orig})
	if err == nil {
		t.Fatal("带 body 的跨源 307 风格重定向默认应拒绝")
	}
	if strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("错误信息泄漏 secret: %v", err)
	}
	if req.Header.Get("Authorization") != "" {
		t.Fatal("拒绝前仍应剥离 Authorization")
	}
}

func TestCheckRequestURL_RequiresHTTPSchemeEvenOnLoopback(t *testing.T) {
	resetConfig()
	u, _ := url.Parse("ftp://127.0.0.1/secret")
	if err := CheckRequestURL(u); err == nil {
		t.Fatal("loopback 也必须先校验 http/https scheme")
	}
}

type internalAPITestTransport struct {
	called bool
}

func (transport *internalAPITestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.called = true
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: request}, nil
}

func TestCheckRequestURL_InternalAPIHosts(test *testing.T) {
	resetConfig()
	test.Setenv(envAllowCustomBaseURL, "")
	test.Setenv(envAllowInsecureHTTP, "")

	cases := []struct {
		name    string
		prefix  string
		suffix  string
		wantErr bool
	}{
		{"https", "https://", "", false},
		{"https default port", "https://", ":443", false},
		{"http", "http://", "", true},
		{"http port 443", "http://", ":443", true},
		{"https port 80", "https://", ":80", true},
		{"https nondefault port", "https://", ":8443", true},
		{"https padded port", "https://", ":0443", true},
		{"malicious suffix", "https://", ".evil.example", true},
		{"subdomain", "https://sub.", "", true},
		{"trailing dot", "https://", ".", true},
		{"userinfo", "https://user:pass@", "", true},
		{"ftp", "ftp://", "", true},
	}
	for _, host := range []string{"internal-api-lark-api.feishu.cn", "internal-api-lark-api.larksuite.com"} {
		for _, testCase := range cases {
			test.Run(host+"/"+testCase.name, func(subtest *testing.T) {
				raw := testCase.prefix + host + testCase.suffix
				request, err := http.NewRequest(http.MethodGet, raw+"/im/v1/preview", nil)
				if err != nil {
					subtest.Fatalf("创建测试请求失败: %v", err)
				}
				if err := CheckRequestURL(request.URL); (err != nil) != testCase.wantErr {
					subtest.Fatalf("CheckRequestURL(%q) = %v，wantErr = %v", raw, err, testCase.wantErr)
				}
				if err := CheckBaseURL(raw); err == nil {
					subtest.Fatalf("内部 API host 不得作为 Open API base_url: %q", raw)
				}

				httpClient := NewHTTPClient(time.Second)
				transport, ok := httpClient.Transport.(*policyTransport)
				if !ok {
					subtest.Fatalf("HTTP 客户端缺少白名单策略: %T", httpClient.Transport)
				}
				base := &internalAPITestTransport{}
				transport.base = base
				response, err := httpClient.Do(request)
				if response != nil {
					response.Body.Close()
				}
				if (err != nil) != testCase.wantErr {
					subtest.Fatalf("NewHTTPClient.Do(%q) = %v，wantErr = %v", raw, err, testCase.wantErr)
				}
				if base.called != !testCase.wantErr {
					subtest.Fatalf("底层 transport 调用 = %v，wantErr = %v", base.called, testCase.wantErr)
				}
			})
		}
	}
}

func TestNewHTTPClient_TimeoutAndLoopback(t *testing.T) {
	c := NewHTTPClient(1500 * time.Millisecond)
	if c.Timeout != 1500*time.Millisecond {
		t.Fatalf("Timeout = %v", c.Timeout)
	}
	if c.CheckRedirect == nil {
		t.Fatal("CheckRedirect 未设置")
	}
	req, _ := http.NewRequest(http.MethodGet, "https://evil.example/x", nil)
	_, err := c.Transport.RoundTrip(req)
	if err == nil {
		t.Fatal("未 opt-in 的自定义 host 不应发出请求")
	}
}
