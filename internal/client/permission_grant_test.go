package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGrantFullAccessToOpenID_RequestShape(t *testing.T) {
	cases := []struct {
		name         string
		resourceType string
		wantPermType string
	}{
		{"docx 不带 perm_type", "docx", ""},
		{"wiki 授予容器权限", "wiki", "container"},
		{"base 别名规范化为 bitable", "base", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath, gotQueryType, gotNotify, gotAuth string
			var gotBody map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
					writeTenantToken(w)
					return
				}
				gotPath = r.URL.Path
				gotQueryType = r.URL.Query().Get("type")
				gotNotify = r.URL.Query().Get("need_notification")
				gotAuth = r.Header.Get("Authorization")
				_ = json.NewDecoder(r.Body).Decode(&gotBody)
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{}}`)
			}))
			defer server.Close()
			setupTestConfig(t, server.URL)

			if err := GrantFullAccessToOpenID("TokA", tc.resourceType, "ou_user"); err != nil {
				t.Fatalf("GrantFullAccessToOpenID 失败: %v", err)
			}
			if gotPath != "/open-apis/drive/v1/permissions/TokA/members" {
				t.Fatalf("path = %q", gotPath)
			}
			if gotQueryType != NormalizeResourceType(tc.resourceType) || gotNotify != "false" {
				t.Fatalf("query type=%q need_notification=%q", gotQueryType, gotNotify)
			}
			if gotAuth != "Bearer t-test" {
				t.Fatalf("授权必须以 Bot（Tenant）身份发起，Authorization=%q", gotAuth)
			}
			if gotBody["member_type"] != "openid" || gotBody["member_id"] != "ou_user" || gotBody["perm"] != "full_access" || gotBody["type"] != "user" {
				t.Fatalf("body 不符: %v", gotBody)
			}
			permType, has := gotBody["perm_type"]
			if tc.wantPermType == "" && has {
				t.Fatalf("非 wiki 资源不应携带 perm_type: %v", gotBody)
			}
			if tc.wantPermType != "" && permType != tc.wantPermType {
				t.Fatalf("perm_type = %v, want %s", permType, tc.wantPermType)
			}
		})
	}
}

func TestGrantFullAccessToOpenID_Errors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			writeTenantToken(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"code":1063002,"msg":"permission denied","error":{"log_id":"LG"}}`)
	}))
	defer server.Close()
	setupTestConfig(t, server.URL)

	err := GrantFullAccessToOpenID("TokA", "docx", "ou_user")
	var grantErr *PermissionGrantError
	if !errors.As(err, &grantErr) || grantErr.Code != 1063002 || grantErr.LogID != "LG" || !HasAPICode(err, 1063002) {
		t.Fatalf("应返回 code=1063002 的 PermissionGrantError，得到 %T %v", err, err)
	}
	if err := GrantFullAccessToOpenID("a/b", "docx", "ou_user"); err == nil {
		t.Fatal("非法 token 应在本地拒绝")
	}
	if err := GrantFullAccessToOpenID("TokA", "docx", ""); err == nil {
		t.Fatal("空 open_id 应在本地拒绝")
	}
}
