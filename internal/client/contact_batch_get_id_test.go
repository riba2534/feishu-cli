package client

import (
	"fmt"
	"net/http"
	"testing"
)

// TestBatchGetUserIDFillsEachIDTypeSeparately 旧版只按 open_id 查询却把结果写进 user_id 字段；
// 现在 open_id / user_id / union_id 分别查询并填入各自字段，user_id 无权限时留空。
func TestBatchGetUserIDFillsEachIDTypeSeparately(t *testing.T) {
	for _, userIDForbidden := range []bool{false, true} {
		name := "全部可查"
		if userIDForbidden {
			name = "user_id 无权限"
		}
		t.Run(name, func(t *testing.T) {
			var types []string
			business := func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/open-apis/contact/v3/users/batch_get_id" {
					http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
					return
				}
				idType := r.URL.Query().Get("user_id_type")
				types = append(types, idType)
				w.Header().Set("Content-Type", "application/json")
				id := map[string]string{"open_id": "ou_alice", "user_id": "u_alice", "union_id": "on_alice"}[idType]
				if idType == "user_id" && userIDForbidden {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = fmt.Fprint(w, `{"code":41050,"msg":"no user authority"}`)
					return
				}
				// 服务端对邮箱大小写不敏感，回显时可能与请求不同
				_, _ = fmt.Fprintf(w, `{"code":0,"msg":"success","data":{"user_list":[{"email":"Alice@Example.com","user_id":"%s"},{"mobile":"+8613800000000"}]}}`, id)
			}
			_, cleanup := stubFeishuServer(t, tenantRouteHandler(t, business))
			defer cleanup()

			got, err := BatchGetUserID([]string{"alice@example.com"}, []string{"+8613800000000"})
			if err != nil {
				t.Fatalf("BatchGetUserID() error = %v", err)
			}
			if len(types) != 3 || types[0] != "open_id" {
				t.Fatalf("应依次按 open_id/user_id/union_id 查询，got %v", types)
			}
			if len(got) != 2 {
				t.Fatalf("结果条数 = %d, want 2", len(got))
			}
			alice := got[0]
			if alice.OpenID != "ou_alice" || alice.UnionID != "on_alice" {
				t.Fatalf("open_id/union_id 不符: %+v", alice)
			}
			wantUserID := "u_alice"
			if userIDForbidden {
				wantUserID = ""
			}
			if alice.UserID != wantUserID {
				t.Fatalf("user_id = %q, want %q（绝不能再是 open_id）", alice.UserID, wantUserID)
			}
			if got[1].OpenID != "" || got[1].Mobile != "+8613800000000" {
				t.Fatalf("未匹配到的手机号应保留且 ID 为空: %+v", got[1])
			}
		})
	}
}
