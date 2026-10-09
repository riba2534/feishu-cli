package auth

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
)

func TestDefaultTATEndpoint(t *testing.T) {
	if got := DefaultTATEndpoint(""); got != "https://accounts.feishu.cn/oauth/v3/token" {
		t.Fatalf("feishu endpoint = %s", got)
	}
	if got := DefaultTATEndpoint("https://open.larksuite.com"); got != "https://accounts.larksuite.com/oauth/v3/token" {
		t.Fatalf("lark endpoint = %s", got)
	}
}

func TestDefaultTATEndpoint_LoopbackOverrideOnly(t *testing.T) {
	t.Setenv("FEISHU_TAT_ENDPOINT", "https://evil.example/oauth/v3/token")
	if got := DefaultTATEndpoint(""); got != "https://accounts.feishu.cn/oauth/v3/token" {
		t.Fatalf("远端覆盖必须忽略，得到 %s", got)
	}
	t.Setenv("FEISHU_TAT_ENDPOINT", "ftp://127.0.0.1:21/oauth/v3/token")
	if got := DefaultTATEndpoint(""); got != "https://accounts.feishu.cn/oauth/v3/token" {
		t.Fatalf("非 http(s) loopback 覆盖必须忽略，得到 %s", got)
	}
	t.Setenv("FEISHU_TAT_ENDPOINT", "http://127.0.0.1:9/oauth/v3/token")
	if got := DefaultTATEndpoint(""); got != "http://127.0.0.1:9/oauth/v3/token" {
		t.Fatalf("loopback 覆盖应生效，得到 %s", got)
	}
}

func TestFetchTenantAccessToken_ClientCredentialsContract(t *testing.T) {
	var (
		gotMethod string
		gotCT     string
		gotBody   string
		gotHost   string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotCT = r.Header.Get("Content-Type")
		gotHost = r.Host
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"access_token":"t-test-token","token_type":"Bearer","expires_in":7200}`))
	}))
	defer srv.Close()

	orig := TATEndpointFunc
	TATEndpointFunc = func(string) string { return srv.URL }
	t.Cleanup(func() { TATEndpointFunc = orig })

	token, err := FetchTenantAccessToken("cli_app", "secret_x", "https://open.feishu.cn")
	if err != nil {
		t.Fatalf("FetchTenantAccessToken: %v", err)
	}
	if token != "t-test-token" {
		t.Fatalf("token = %q", token)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %s", gotMethod)
	}
	if gotCT != "application/x-www-form-urlencoded" {
		t.Fatalf("Content-Type = %q", gotCT)
	}
	for _, want := range []string{"grant_type=client_credentials", "client_id=cli_app", "client_secret=secret_x"} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("body missing %q: %s", want, gotBody)
		}
	}
	_ = gotHost
}

func TestFetchTenantAccessToken_HTTPAndBusinessErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"oauth invalid_client", 400, `{"error":"invalid_client","error_description":"The client secret is invalid.","code":20002}`, "invalid_client"},
		{"legacy code", 200, `{"code":99991663,"msg":"app secret invalid"}`, "99991663"},
		{"http 500", 500, `{"error":"server_error"}`, "暂时失败"},
		{"missing token", 200, `{"code":0}`, "access_token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)
			orig := TATEndpointFunc
			TATEndpointFunc = func(string) string { return srv.URL }
			t.Cleanup(func() { TATEndpointFunc = orig })

			_, err := FetchTenantAccessToken("cli_app", "secret_x", "https://open.feishu.cn")
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q 应包含 %q", err.Error(), tc.want)
			}
			if strings.Contains(err.Error(), "secret_x") {
				t.Fatalf("错误信息泄漏 secret: %v", err)
			}
		})
	}
}

func TestFetchTenantAccessToken_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"code":0,"access_token":"t-late"}`))
	}))
	defer srv.Close()
	orig := TATEndpointFunc
	TATEndpointFunc = func(string) string { return srv.URL }
	t.Cleanup(func() { TATEndpointFunc = orig })

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := fetchTenantAccessToken(ctx, config.NewHTTPClient(20*time.Millisecond), "cli_app", "secret_x", "https://open.feishu.cn")
	if err == nil {
		t.Fatal("超时应返回错误")
	}
}

func TestFetchTenantAccessToken_RejectsInvalidExpiresIn(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"missing", `{"code":0,"access_token":"t-x"}`},
		{"zero", `{"code":0,"access_token":"t-x","expires_in":0}`},
		{"negative", `{"code":0,"access_token":"t-x","expires_in":-1}`},
		{"too large", `{"code":0,"access_token":"t-x","expires_in":99999999}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)
			orig := TATEndpointFunc
			TATEndpointFunc = func(string) string { return srv.URL }
			t.Cleanup(func() { TATEndpointFunc = orig })
			_, err := FetchTenantAccessToken("cli_app", "secret_x", "https://open.feishu.cn")
			if err == nil {
				t.Fatal("expected expires_in validation error")
			}
		})
	}
}

func TestFetchTenantAccessToken_RedactsSecretsAndRejectsOversize(t *testing.T) {
	t.Run("redact", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"access_token=u-leak client_secret=s-leak"}`))
		}))
		t.Cleanup(srv.Close)
		orig := TATEndpointFunc
		TATEndpointFunc = func(string) string { return srv.URL }
		t.Cleanup(func() { TATEndpointFunc = orig })
		_, err := FetchTenantAccessToken("cli_app", "secret_x", "https://open.feishu.cn")
		if err == nil {
			t.Fatal("expected error")
		}
		if strings.Contains(err.Error(), "u-leak") || strings.Contains(err.Error(), "s-leak") {
			t.Fatalf("错误预览泄漏 secret: %v", err)
		}
		if !strings.Contains(err.Error(), "[REDACTED]") {
			t.Fatalf("应脱敏: %v", err)
		}
	})
	t.Run("oversize", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(bytes.Repeat([]byte("x"), maxAuthResponseBytes+1))
		}))
		t.Cleanup(srv.Close)
		orig := TATEndpointFunc
		TATEndpointFunc = func(string) string { return srv.URL }
		t.Cleanup(func() { TATEndpointFunc = orig })
		_, err := FetchTenantAccessToken("cli_app", "secret_x", "https://open.feishu.cn")
		if err == nil || !strings.Contains(err.Error(), "1MiB") {
			t.Fatalf("超过 1MiB 必须报错: %v", err)
		}
	})
}

// App 凭证被确定性拒绝（invalid_client，如 20002 secret 错误、20048 应用不存在）→ 鉴权类（退出码 3），
// 文本保持不变；5xx / 临时错误不算凭证被拒。
func TestFetchTenantAccessToken_CredentialRejected(t *testing.T) {
	cases := []struct {
		status   int
		body     string
		rejected bool
	}{
		{400, `{"code":20002,"error":"invalid_client","error_description":"The client secret is invalid."}`, true},
		{400, `{"code":20048,"error":"invalid_client","error_description":"The specified app does not exist."}`, true},
		{400, `{"code":99991543,"msg":"app secret invalid"}`, true},
		{503, `{"error":"server_error","error_description":"busy"}`, false},
		{400, `{"code":12345,"msg":"other"}`, false},
	}
	for _, c := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(c.status)
			_, _ = w.Write([]byte(c.body))
		}))
		orig := TATEndpointFunc
		TATEndpointFunc = func(string) string { return srv.URL }
		_, err := FetchTenantAccessToken("cli_a", "bad", "")
		TATEndpointFunc = orig
		srv.Close()
		if err == nil {
			t.Fatalf("%s: 应失败", c.body)
		}
		if IsCredentialRejected(err) != c.rejected {
			t.Fatalf("%s: IsCredentialRejected=%v want %v (%v)", c.body, !c.rejected, c.rejected, err)
		}
		if c.rejected && !clierr.HasKind(err, clierr.KindAuth) {
			t.Fatalf("%s: 凭证被拒应打鉴权标签", c.body)
		}
		if c.rejected && !strings.Contains(err.Error(), "获取 tenant_access_token 失败") {
			t.Fatalf("错误文本应保持不变: %v", err)
		}
	}
}

func TestFetchTenantAccessToken_RetriesOnceOn429(t *testing.T) {
	origMax, origDef := tatMaxRetryAfter, tatDefaultRetryAfter
	tatMaxRetryAfter, tatDefaultRetryAfter = 50*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { tatMaxRetryAfter, tatDefaultRetryAfter = origMax, origDef })

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "1") // 1s，被上限截到 50ms
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"code":99991400,"msg":"rate limited"}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"access_token":"t-after-429","expires_in":7200}`))
	}))
	defer srv.Close()
	orig := TATEndpointFunc
	TATEndpointFunc = func(string) string { return srv.URL }
	defer func() { TATEndpointFunc = orig }()

	start := time.Now()
	tok, err := FetchTenantAccessToken("cli_a", "s", "")
	if err != nil || tok != "t-after-429" {
		t.Fatalf("429 后应重试一次成功: %q %v", tok, err)
	}
	if calls != 2 {
		t.Fatalf("应请求 2 次，实际 %d", calls)
	}
	if time.Since(start) < 40*time.Millisecond {
		t.Fatal("应按 Retry-After（截断后）等待")
	}

	// 连续 429：只重试一次后报错
	calls = 0
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"code":99991400,"msg":"rate limited"}`))
	}))
	defer srv2.Close()
	TATEndpointFunc = func(string) string { return srv2.URL }
	if _, err := FetchTenantAccessToken("cli_a", "s", ""); err == nil || calls != 2 {
		t.Fatalf("连续 429 应重试一次后失败: calls=%d err=%v", calls, err)
	}
}

func TestParseRetryAfter(t *testing.T) {
	origMax, origDef := tatMaxRetryAfter, tatDefaultRetryAfter
	tatMaxRetryAfter, tatDefaultRetryAfter = 5*time.Second, time.Second
	t.Cleanup(func() { tatMaxRetryAfter, tatDefaultRetryAfter = origMax, origDef })
	cases := map[string]time.Duration{"": time.Second, "2": 2 * time.Second, "60": 5 * time.Second, "abc": time.Second, "0": time.Second}
	for in, want := range cases {
		if got := parseRetryAfter(in); got != want {
			t.Errorf("parseRetryAfter(%q)=%s want %s", in, got, want)
		}
	}
}

func TestLoadToken_CorruptFileHintsRelogin(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/token.json"
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadTokenFrom(path)
	if err == nil || !strings.Contains(err.Error(), "解析 token 文件失败") || !strings.Contains(err.Error(), "auth login") {
		t.Fatalf("损坏的 token.json 应提示重新登录: %v", err)
	}
}
