package client

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// P0-6：业务码随 HTTP 400 下发时，必须先解析业务码，让按业务码分支的专门提示可达。
func TestDriveClients_HTTP400BusinessCodeParsed(t *testing.T) {
	cases := []struct {
		name     string
		code     int
		call     func() error
		wantHint string
	}{
		{"secure-label set 1063013", 1063013, func() error { return SetSecureLabel("doxcn1", "docx", "lbl1", "u-x") }, "审批"},
		{"secure-label set 1063002", 1063002, func() error { return SetSecureLabel("doxcn1", "docx", "lbl1", "u-x") }, "无权修改"},
		{"secure-label list 1063002", 1063002, func() error {
			_, _, _, err := ListSecureLabels(10, "", "", "u-x")
			return err
		}, "无权查询"},
		{"version revert", 1061002, func() error { return RevertFileVersion("boxcn1", "123", "u-x") }, ""},
		{"root folder meta", 1061004, func() error { _, err := GetRootFolderToken("u-x"); return err }, ""},
		{"export task create", 1069902, func() error {
			_, err := CreateExportTaskEx("doxcn1", "docx", "pdf", "", false, "u-x")
			return err
		}, ""},
		{"meta title", 1061004, func() error { _, err := FetchDocMetaTitle("doxcn1", "docx", "u-x"); return err }, ""},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = fmt.Fprintf(w, `{"code":%d,"msg":"biz error","log_id":"lg-400"}`, c.code)
			})
			defer cleanup()
			err := c.call()
			if err == nil {
				t.Fatal("want error")
			}
			if !HasAPICode(err, c.code) {
				t.Fatalf("业务码 %d 未保留: %v", c.code, err)
			}
			if _, ok := AsAPIError(err); !ok {
				t.Fatalf("应返回 *APIError: %v", err)
			}
			if strings.Contains(err.Error(), "HTTP 400, body") {
				t.Fatalf("不应先按 HTTP 状态短路: %v", err)
			}
			if c.wantHint != "" && !strings.Contains(err.Error(), c.wantHint) {
				t.Fatalf("专门提示不可达: %v", err)
			}
		})
	}
}
