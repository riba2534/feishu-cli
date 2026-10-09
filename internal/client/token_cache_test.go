package client

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// TestTokenCacheNotSharedAcrossClientRebuild 同一 app_id 在配置变更（base_url 变化）后，
// 重建的 client 必须重新换取 tenant token，不能复用旧配置换到的 token。
// SDK 默认的全局缓存只按 app_id 区分，会把旧服务器的 token 带到新服务器上。
func TestTokenCacheNotSharedAcrossClientRebuild(t *testing.T) {
	newServer := func(token string, gotAuth *string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
				_, _ = fmt.Fprintf(w, `{"code":0,"tenant_access_token":%q,"expire":7200}`, token)
				return
			}
			*gotAuth = r.Header.Get("Authorization")
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"app":{"app_id":"test_app_id","scopes":[]}}}`)
		}
	}

	var auth1, auth2 string
	_, cleanup1 := stubFeishuServer(t, newServer("t-server-one", &auth1))
	if _, err := GetApplicationScopes("test_app_id"); err != nil {
		t.Fatal(err)
	}
	cleanup1()
	if auth1 != "Bearer t-server-one" {
		t.Fatalf("第一个服务器应使用其签发的 token，得到 %q", auth1)
	}

	_, cleanup2 := stubFeishuServer(t, newServer("t-server-two", &auth2))
	defer cleanup2()
	if _, err := GetApplicationScopes("test_app_id"); err != nil {
		t.Fatal(err)
	}
	if auth2 != "Bearer t-server-two" {
		t.Fatalf("配置变更后应重新换取 token，得到 %q（复用了旧配置的 token）", auth2)
	}
}
