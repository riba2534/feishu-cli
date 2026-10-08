package client

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQueryDriveToken_Success(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantType string
		wantTok  string
		wantWiki bool
		status   int
	}{
		{"普通 docx", `{"code":0,"data":{"is_wiki_token":false,"obj_token":"DocA","obj_type":"docx","status":0}}`, "docx", "DocA", false, 0},
		{"wiki 自动解包", `{"code":0,"data":{"is_wiki_token":true,"obj_token":"ShtB","obj_type":"sheet","status":0}}`, "sheet", "ShtB", true, 0},
		{"回收站节点", `{"code":0,"data":{"is_wiki_token":true,"obj_token":"FileC","obj_type":"file","status":1}}`, "file", "FileC", true, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath, gotToken string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
					writeTenantToken(w)
					return
				}
				gotPath, gotToken = r.URL.Path, r.URL.Query().Get("token")
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			setupTestConfig(t, server.URL)

			info, err := QueryDriveToken("InTok", "") // Bot 身份
			if err != nil {
				t.Fatalf("QueryDriveToken 失败: %v", err)
			}
			if gotPath != DriveQueryByTokenPath || gotToken != "InTok" {
				t.Fatalf("请求 path=%q token=%q", gotPath, gotToken)
			}
			if info.ObjType != tc.wantType || info.ObjToken != tc.wantTok || info.IsWikiToken != tc.wantWiki || info.Status != tc.status {
				t.Fatalf("结果不符: %+v", info)
			}
		})
	}
}

func TestQueryDriveToken_Errors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
		code   int
	}{
		{"981002 无效", http.StatusBadRequest, `{"code":981002,"msg":"invalid token.","error":{"log_id":"L1"}}`, "格式无效", 981002},
		{"981003 不存在", http.StatusNotFound, `{"code":981003,"msg":"not found."}`, "资源不存在", 981003},
		{"981004 无权限", http.StatusForbidden, `{"code":981004,"msg":"forbidden."}`, "无权访问", 981004},
		{"数据不完整", http.StatusOK, `{"code":0,"data":{"obj_token":"","obj_type":"docx"}}`, "不完整", 0},
		{"非法 obj_token", http.StatusOK, `{"code":0,"data":{"obj_token":"../x","obj_type":"docx"}}`, "非法的 obj_token", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			setupTestConfig(t, server.URL)

			_, err := QueryDriveToken("InTok", "u-token")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("期望包含 %q 的错误，得到 %v", tc.want, err)
			}
			if tc.code != 0 {
				var qe *DriveTokenQueryError
				if !errors.As(err, &qe) || qe.Code != tc.code || !HasAPICode(err, tc.code) {
					t.Fatalf("应返回 code=%d 的 DriveTokenQueryError，得到 %T %v", tc.code, err, err)
				}
			}
		})
	}
}
