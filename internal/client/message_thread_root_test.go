package client

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// TestListMessagesOnlyThreadRootQueryParam 验证 only_thread_root_messages 只在
// chat 容器且显式开启时下发；User Token（raw HTTP）与 Tenant（SDK raw request）两条路径一致。
func TestListMessagesOnlyThreadRootQueryParam(t *testing.T) {
	tests := []struct {
		name          string
		userToken     string
		containerType string
		onlyRoot      bool
		want          string
	}{
		{"user+chat+开启", "u-test", "chat", true, "true"},
		{"tenant+chat+开启", "", "chat", true, "true"},
		{"user+默认容器类型+开启", "u-test", "", true, "true"},
		{"user+chat+关闭", "u-test", "chat", false, ""},
		{"tenant+thread+开启（thread 容器不下发）", "", "thread", true, ""},
		{"user+thread+开启（thread 容器不下发）", "u-test", "thread", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured url.Values
			business := func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/open-apis/im/v1/messages" {
					http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
					return
				}
				captured = r.URL.Query()
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"items":[],"has_more":false}}`)
			}
			_, cleanup := stubFeishuServer(t, tenantRouteHandler(t, business))
			defer cleanup()

			_, err := ListMessages("oc_test", ListMessagesOptions{
				ContainerIDType:        tt.containerType,
				OnlyThreadRootMessages: tt.onlyRoot,
			}, tt.userToken)
			if err != nil {
				t.Fatalf("ListMessages() error = %v", err)
			}
			if got := captured.Get("only_thread_root_messages"); got != tt.want {
				t.Fatalf("only_thread_root_messages = %q, want %q（query=%s）", got, tt.want, captured.Encode())
			}
			if captured.Get("with_sender_name") != "true" {
				t.Fatalf("with_sender_name 丢失: %s", captured.Encode())
			}
		})
	}
}

// TestListMessagesBusinessErrorOnHTTP400 验证业务错误随 HTTP 400 下发时，
// 先解析信封拿到业务码（HasAPICode 可用、带 log_id），而不是只报 "HTTP 400"。
func TestListMessagesBusinessErrorOnHTTP400(t *testing.T) {
	for _, userToken := range []string{"u-test", ""} {
		name := "tenant"
		if userToken != "" {
			name = "user"
		}
		t.Run(name, func(t *testing.T) {
			business := func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = fmt.Fprint(w, `{"code":230002,"msg":"Bot/User can NOT be out of the chat.","error":{"log_id":"log_test_123"}}`)
			}
			_, cleanup := stubFeishuServer(t, tenantRouteHandler(t, business))
			defer cleanup()

			_, err := ListMessages("oc_test", ListMessagesOptions{ContainerIDType: "chat"}, userToken)
			if err == nil {
				t.Fatal("期望返回错误")
			}
			if !HasAPICode(err, 230002) {
				t.Fatalf("期望可按业务码 230002 判定，got %v", err)
			}
			if !strings.Contains(err.Error(), "log_test_123") {
				t.Fatalf("错误应带 log_id，got %v", err)
			}
		})
	}
}
