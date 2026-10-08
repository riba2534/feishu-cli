package client

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestWikiRawCalls_BusinessCodeOnHTTP400 回归：飞书业务错误常随 HTTP 400/403 下发。
// 过去这些调用点先判 StatusCode != 200 就返回 "HTTP 400, body: ..."，*APIError 与按业务码分支的提示
// （如 wiki delete 的 131011 审批提示）永远走不到；改为先解析信封后，必须能从错误链取到业务码。
func TestWikiRawCalls_BusinessCodeOnHTTP400(t *testing.T) {
	cases := []struct {
		name string
		call func() error
	}{
		{"DeleteWikiSpace", func() error { _, err := DeleteWikiSpace("sp-1", "u-token"); return err }},
		{"GetWikiDeleteSpaceTask", func() error { _, err := GetWikiDeleteSpaceTask("task-1", "u-token"); return err }},
		{"DeleteWikiNode", func() error { _, err := DeleteWikiNode("sp-1", "wikNode", "wiki", true, "u-token"); return err }},
		{"GetWikiDeleteNodeTask", func() error { _, err := GetWikiDeleteNodeTask("task-1", "u-token"); return err }},
		{"MoveWikiNodeToDrive", func() error { _, err := MoveWikiNodeToDrive("wikNode", "fld", "u-token"); return err }},
		{"GetMoveWikiToDriveTask", func() error { _, err := GetMoveWikiToDriveTask("task-1", "u-token"); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = fmt.Fprint(w, `{"code":131011,"msg":"need approval","error":{"log_id":"LOGID123"}}`)
			}))
			defer server.Close()
			setupTestConfig(t, server.URL)

			err := tc.call()
			if err == nil {
				t.Fatal("HTTP 400 + 业务码必须返回错误")
			}
			apiErr, ok := AsAPIError(err)
			if !ok {
				t.Fatalf("错误链中必须能取到 *APIError（先解析业务码再看 HTTP 状态），实际: %v", err)
			}
			if apiErr.Code != 131011 {
				t.Fatalf("业务码 = %d, 期望 131011", apiErr.Code)
			}
			if !HasAPICode(err, 131011) {
				t.Fatalf("HasAPICode 必须命中 131011: %v", err)
			}
			if strings.Contains(err.Error(), "HTTP 400, body") {
				t.Fatalf("不应再退化为 HTTP 状态文本: %v", err)
			}
		})
	}
}

// TestWikiRawCalls_NonEnvelope5xx 非飞书信封的 5xx 仍要报 HTTP 状态，不能被当成成功。
func TestWikiRawCalls_NonEnvelope5xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gateway boom", http.StatusBadGateway)
	}))
	defer server.Close()
	setupTestConfig(t, server.URL)

	if _, err := DeleteWikiSpace("sp-1", "u-token"); err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("非信封 502 必须报 HTTP 状态，得到: %v", err)
	}
}

// TestAddWikiSpaceMember_NotificationQuery 验证 need_notification 由调用方决定（不再写死 true）。
func TestAddWikiSpaceMember_NotificationQuery(t *testing.T) {
	for _, notify := range []bool{true, false} {
		t.Run(fmt.Sprintf("notify=%v", notify), func(t *testing.T) {
			var gotQuery string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost && r.URL.Path == "/open-apis/wiki/v2/spaces/sp-1/members" {
					gotQuery = r.URL.RawQuery
					_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"member":{"member_type":"openid","member_id":"ou_x","member_role":"member"}}}`)
					return
				}
				http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
			}))
			defer server.Close()
			setupTestConfig(t, server.URL)

			if err := AddWikiSpaceMemberWithOptions("sp-1", "openid", "ou_x", "member", notify, "u-token"); err != nil {
				t.Fatalf("添加成员失败: %v", err)
			}
			want := fmt.Sprintf("need_notification=%v", notify)
			if gotQuery != want {
				t.Fatalf("query = %q, 期望 %q", gotQuery, want)
			}
		})
	}
}
