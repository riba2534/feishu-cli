package client

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestGetApplicationScopes_ParsesAndSplitsByTokenType(t *testing.T) {
	var gotPath, gotLang, gotAuth string
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			_, _ = fmt.Fprint(w, `{"code":0,"tenant_access_token":"t-app-info","expire":7200}`)
			return
		}
		gotPath, gotLang, gotAuth = r.URL.Path, r.URL.Query().Get("lang"), r.Header.Get("Authorization")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"app":{"app_id":"test_app_id","app_name":"示例应用","scopes":[
			{"scope":"search:docs:read","token_types":["user"],"description":"搜索文档","level":1},
			{"scope":"im:message:send_as_bot","token_types":["tenant"]},
			{"scope":"drive:drive","token_types":["user","tenant"]},
			{"scope":"drive:drive","token_types":["user","tenant"]},
			{"scope":"","token_types":["user"]}
		]}}}`)
	}
	_, cleanup := stubFeishuServer(t, handler)
	defer cleanup()

	app, err := GetApplicationScopes("test_app_id")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/open-apis/application/v6/applications/test_app_id" || gotLang != "zh_cn" || gotAuth != "Bearer t-app-info" {
		t.Fatalf("请求不正确: path=%s lang=%s auth=%s", gotPath, gotLang, gotAuth)
	}
	if app.AppName != "示例应用" {
		t.Fatalf("app_name = %q", app.AppName)
	}
	if got := strings.Join(app.ScopesFor("user"), " "); got != "drive:drive search:docs:read" {
		t.Fatalf("user scopes = %q", got)
	}
	if got := strings.Join(app.ScopesFor("tenant"), " "); got != "drive:drive im:message:send_as_bot" {
		t.Fatalf("tenant scopes = %q", got)
	}
}

// 业务错误随 HTTP 400 下发时也要解析出业务码（不能先按 HTTP 状态短路）。
func TestGetApplicationScopes_BusinessErrorOn400(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			_, _ = fmt.Fprint(w, `{"code":0,"tenant_access_token":"t-x","expire":7200}`)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"code":99991672,"msg":"Access denied.","error":{"log_id":"LOGAPP","permission_violations":[{"subject":"application:application:self_manage"}]}}`)
	}
	_, cleanup := stubFeishuServer(t, handler)
	defer cleanup()

	_, err := GetApplicationScopes("test_app_id")
	if err == nil || !HasAPICode(err, 99991672) {
		t.Fatalf("应解析出业务码 99991672: %v", err)
	}
	if apiErr, ok := AsAPIError(err); !ok || apiErr.LogID != "LOGAPP" {
		t.Fatalf("应保留 log_id: %v", err)
	}
}
