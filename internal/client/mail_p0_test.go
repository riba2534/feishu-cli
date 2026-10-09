package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCallMailAPI_BusinessErrorOnHTTP400 飞书邮箱业务错误随 HTTP 400 下发时，必须解析出业务码与 log_id，
// 而不是返回 "HTTP 400, body: ..."（否则 HasAPICode 分支与诊断都失效）。
func TestCallMailAPI_BusinessErrorOnHTTP400(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"code":4038,"msg":"folder_id is invalid","error":{"log_id":"LOGID123"}}`)
	}))
	defer srv.Close()
	setupTestConfig(t, srv.URL)

	_, err := ListMailMessages(ListMailMessagesParams{FolderID: "inbox"}, "u-test-token")
	if err == nil {
		t.Fatal("expected error")
	}
	if !HasAPICode(err, 4038) {
		t.Errorf("HasAPICode(err, 4038) = false, err = %v", err)
	}
	apiErr, ok := AsAPIError(err)
	if !ok || apiErr.Code != 4038 {
		t.Fatalf("应为 *APIError(code=4038)，得到 %T %v", err, err)
	}
	if strings.Contains(err.Error(), "HTTP 400, body") {
		t.Errorf("不应退化为 HTTP 状态错误: %v", err)
	}
	if strings.Contains(err.Error(), "page_size=") {
		t.Errorf("错误信息不应带 query 参数: %v", err)
	}
}

// TestGetMailDraftRaw_DraftMessageRawShape 实测 format=raw 响应是 data.draft.message.raw；
// 旧实现只读 data.raw / data.draft.raw，永远拿到空串。
func TestGetMailDraftRaw_DraftMessageRawShape(t *testing.T) {
	cases := map[string]string{
		"draft.message.raw": `{"code":0,"msg":"ok","data":{"draft":{"id":"d1","message":{"message_id":"d1","raw":"RAW1"}}}}`,
		"draft.raw":         `{"code":0,"msg":"ok","data":{"draft":{"raw":"RAW1"}}}`,
		"raw":               `{"code":0,"msg":"ok","data":{"raw":"RAW1"}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			var gotQuery string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotQuery = r.URL.RawQuery
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			}))
			defer srv.Close()
			setupTestConfig(t, srv.URL)
			raw, err := GetMailDraftRaw("me", "d1", "u-test-token")
			if err != nil || raw != "RAW1" {
				t.Fatalf("raw=%q err=%v", raw, err)
			}
			if gotQuery != "format=raw" {
				t.Errorf("query = %q", gotQuery)
			}
		})
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"draft":{"id":"d1"}}}`)
	}))
	defer srv.Close()
	setupTestConfig(t, srv.URL)
	if _, err := GetMailDraftRaw("me", "d1", "u-test-token"); err == nil {
		t.Error("响应中没有 raw 时应报错，而不是返回空串")
	}
}
