package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

// 网关模式：tenant token 经 base_url 的 Open API 端点换取，不使用 Accounts 端点，也不要求 app_secret。
func TestFetchTenantAccessToken_GatewayUsesBaseURL(t *testing.T) {
	t.Setenv("FEISHU_GATEWAY", "1")
	var (
		gotPath string
		gotCT   string
		gotBody map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_, _ = w.Write([]byte(`{"code":0,"msg":"ok","tenant_access_token":"t-gateway","expire":7200}`))
	}))
	defer srv.Close()

	orig := TATEndpointFunc
	TATEndpointFunc = func(string) string {
		t.Fatal("网关模式不应解析 Accounts 端点")
		return ""
	}
	t.Cleanup(func() { TATEndpointFunc = orig })

	tok, err := FetchTenantAccessTokenResult(context.Background(), "cli_app", "", srv.URL)
	if err != nil {
		t.Fatalf("网关模式换取 tenant token: %v", err)
	}
	if tok.AccessToken != "t-gateway" || tok.ExpiresIn != 7200 {
		t.Fatalf("token = %+v", tok)
	}
	if gotPath != "/open-apis/auth/v3/tenant_access_token/internal" {
		t.Fatalf("path = %s", gotPath)
	}
	if gotCT != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", gotCT)
	}
	if gotBody["app_id"] != "cli_app" {
		t.Fatalf("body 应含 app_id: %v", gotBody)
	}
	if _, ok := gotBody["app_secret"]; ok {
		t.Fatalf("未配置 app_secret 时不应发送该字段: %v", gotBody)
	}
}

func TestFetchTenantAccessToken_GatewayRefusesOfficialBase(t *testing.T) {
	t.Setenv("FEISHU_GATEWAY", "1")
	for _, base := range []string{"", "https://open.feishu.cn"} {
		if _, err := FetchTenantAccessTokenResult(context.Background(), "cli_app", "", base); err == nil {
			t.Fatalf("网关模式下 base_url=%q 应拒绝", base)
		}
	}
}

func TestFetchTenantAccessToken_GatewayErrors(t *testing.T) {
	t.Setenv("FEISHU_GATEWAY", "1")
	cases := []struct {
		status   int
		body     string
		rejected bool
	}{
		{200, `{"code":10014,"msg":"app secret invalid"}`, true},
		{400, `{"code":99991543,"msg":"app_id or app_secret is invalid"}`, true},
		{200, `{"code":99991400,"msg":"rate limited"}`, false},
		{502, `bad gateway`, false},
		{200, `{"code":0,"msg":"ok"}`, false},
		{200, `{"code":0,"tenant_access_token":"t","expire":0}`, false},
	}
	for _, c := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(c.status)
			_, _ = w.Write([]byte(c.body))
		}))
		_, err := FetchTenantAccessTokenResult(context.Background(), "cli_app", "", srv.URL)
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
	}
}
