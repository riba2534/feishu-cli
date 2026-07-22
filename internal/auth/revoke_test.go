package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// withRevokeServer 启动一个 httptest server 作为吊销端点，并临时替换 revokeEndpointFunc。
func withRevokeServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	orig := revokeEndpointFunc
	revokeEndpointFunc = func(string) string { return srv.URL }
	t.Cleanup(func() {
		revokeEndpointFunc = orig
		srv.Close()
	})
	return srv
}

func TestRevokeToken_SuccessSendsForm(t *testing.T) {
	var gotForm map[string]string
	withRevokeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("解析表单失败: %v", err)
		}
		gotForm = map[string]string{
			"client_id":       r.PostFormValue("client_id"),
			"client_secret":   r.PostFormValue("client_secret"),
			"token":           r.PostFormValue("token"),
			"token_type_hint": r.PostFormValue("token_type_hint"),
		}
		_, _ = w.Write([]byte(`{"code":0,"msg":"success"}`))
	})

	if err := RevokeToken("cli_app", "secret", "https://open.feishu.cn", "r-token", "refresh_token"); err != nil {
		t.Fatalf("期望成功，得到错误: %v", err)
	}
	if gotForm["client_id"] != "cli_app" || gotForm["client_secret"] != "secret" ||
		gotForm["token"] != "r-token" || gotForm["token_type_hint"] != "refresh_token" {
		t.Errorf("表单参数不匹配: %+v", gotForm)
	}
}

func TestRevokeToken_EmptyBodyIsSuccess(t *testing.T) {
	withRevokeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK) // 空响应体
	})
	if err := RevokeToken("cli_app", "secret", "", "a-token", "access_token"); err != nil {
		t.Fatalf("空响应体应视为成功，得到错误: %v", err)
	}
}

func TestRevokeToken_BusinessErrorCode(t *testing.T) {
	withRevokeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":20050,"msg":"invalid token"}`))
	})
	if err := RevokeToken("cli_app", "secret", "", "bad", ""); err == nil {
		t.Fatal("业务 code!=0 应返回错误")
	}
}

func TestRevokeToken_OAuthErrorField(t *testing.T) {
	withRevokeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":"invalid_client","error_description":"bad secret"}`))
	})
	if err := RevokeToken("cli_app", "secret", "", "bad", ""); err == nil {
		t.Fatal("OAuth error 字段应返回错误")
	}
}

func TestRevokeToken_HTTPStatusError(t *testing.T) {
	withRevokeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_request"}`))
	})
	if err := RevokeToken("cli_app", "secret", "", "bad", ""); err == nil {
		t.Fatal("HTTP 4xx 应返回错误")
	}
}

func TestRevokeToken_EmptyTokenRejected(t *testing.T) {
	// 空 token 应在发起请求前直接报错，不触达服务端
	if err := RevokeToken("cli_app", "secret", "", "", ""); err == nil {
		t.Fatal("空 token 应返回错误")
	}
}

func TestRevokeStoredToken_PrefersRefreshToken(t *testing.T) {
	var gotToken, gotHint string
	withRevokeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotToken = r.PostFormValue("token")
		gotHint = r.PostFormValue("token_type_hint")
		_, _ = w.Write([]byte(`{"code":0}`))
	})
	store := &TokenStore{AccessToken: "a-tok", RefreshToken: "r-tok"}
	if err := RevokeStoredToken(store, "cli_app", "secret", ""); err != nil {
		t.Fatalf("期望成功: %v", err)
	}
	if gotToken != "r-tok" || gotHint != "refresh_token" {
		t.Errorf("应优先吊销 refresh_token，实际 token=%q hint=%q", gotToken, gotHint)
	}
}

func TestRevokeStoredToken_FallbackToAccessToken(t *testing.T) {
	var gotToken, gotHint string
	withRevokeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotToken = r.PostFormValue("token")
		gotHint = r.PostFormValue("token_type_hint")
		_, _ = w.Write([]byte(`{"code":0}`))
	})
	store := &TokenStore{AccessToken: "a-tok"} // 无 refresh_token
	if err := RevokeStoredToken(store, "cli_app", "secret", ""); err != nil {
		t.Fatalf("期望成功: %v", err)
	}
	if gotToken != "a-tok" || gotHint != "access_token" {
		t.Errorf("无 refresh_token 时应吊销 access_token，实际 token=%q hint=%q", gotToken, gotHint)
	}
}

func TestRevokeStoredToken_NoTokenNoCall(t *testing.T) {
	called := false
	withRevokeServer(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	if err := RevokeStoredToken(&TokenStore{}, "cli_app", "secret", ""); err != nil {
		t.Fatalf("无可吊销 token 应返回 nil，得到: %v", err)
	}
	if err := RevokeStoredToken(nil, "cli_app", "secret", ""); err != nil {
		t.Fatalf("nil store 应返回 nil，得到: %v", err)
	}
	if called {
		t.Error("没有可吊销的 token 时不应发起请求")
	}
}

func TestAccountsBaseFor(t *testing.T) {
	if got := accountsBaseFor("https://open.feishu.cn"); got != "https://accounts.feishu.cn" {
		t.Errorf("feishu 域应解析为 accounts.feishu.cn，实际 %q", got)
	}
	if got := accountsBaseFor("https://open.larksuite.com"); got != "https://accounts.larksuite.com" {
		t.Errorf("larksuite 域应解析为 accounts.larksuite.com，实际 %q", got)
	}
	if got := accountsBaseFor(""); got != "https://accounts.feishu.cn" {
		t.Errorf("空 baseURL 应回退 feishu，实际 %q", got)
	}
}
