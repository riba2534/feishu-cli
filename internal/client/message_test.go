package client

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/riba2534/feishu-cli/internal/config"
)

// stubFeishuServer 起一个本地 httptest server 替代真实飞书 OAPI；
// 通过 base_url 配置注入到 cli。
func stubFeishuServer(t *testing.T, handler http.HandlerFunc) (string, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)

	resetClient()
	resetConfig()

	tmpDir := t.TempDir()
	configFile := tmpDir + "/config.yaml"
	content := fmt.Sprintf(`app_id: "test_app_id"
app_secret: "test_app_secret"
base_url: "%s"
`, srv.URL)
	if err := os.WriteFile(configFile, []byte(content), 0o600); err != nil {
		t.Fatalf("写测试配置失败: %v", err)
	}
	if err := config.Init(configFile); err != nil {
		t.Fatalf("初始化测试配置失败: %v", err)
	}

	return srv.URL, srv.Close
}

func TestGetMessageWithUserToken_CardContentType(t *testing.T) {
	tests := []struct {
		name             string
		cardContentType  string
		expectedQueryArg string
	}{
		{"空 → 不传 query", "", ""},
		{"user_card_content", CardMsgContentTypeUser, "user_card_content"},
		{"raw_card_content", CardMsgContentTypeRaw, "raw_card_content"},
	}

	const messageID = "om_x100b512ca9a404b8b2432e156aa8895"
	const userToken = "u-test-token"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedQuery url.Values
			var capturedAuth string
			handler := func(w http.ResponseWriter, r *http.Request) {
				capturedQuery = r.URL.Query()
				capturedAuth = r.Header.Get("Authorization")
				expectedPath := "/open-apis/im/v1/messages/" + messageID
				if r.URL.Path != expectedPath {
					t.Errorf("path 不符: got %q, want %q", r.URL.Path, expectedPath)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"code":0,"msg":"success","data":{"items":[{"message_id":"%s","msg_type":"interactive","body":{"content":"{}"}}]}}`, messageID)
			}

			_, cleanup := stubFeishuServer(t, handler)
			defer cleanup()

			result, err := getMessageWithUserToken(messageID, userToken, tt.cardContentType)
			if err != nil {
				t.Fatalf("getMessageWithUserToken 返回错误: %v", err)
			}
			if result == nil || result.Message == nil {
				t.Fatalf("返回结果为空")
			}
			if got := capturedQuery.Get("card_msg_content_type"); got != tt.expectedQueryArg {
				t.Errorf("card_msg_content_type query: got %q, want %q", got, tt.expectedQueryArg)
			}
			if capturedAuth != "Bearer "+userToken {
				t.Errorf("Authorization header: got %q, want %q", capturedAuth, "Bearer "+userToken)
			}
		})
	}
}

func TestListMessagesWithUserToken_CardContentType(t *testing.T) {
	const containerID = "oc_test_chat"
	const userToken = "u-test-token"

	var capturedQuery url.Values
	handler := func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"items":[],"has_more":false,"page_token":""}}`)
	}

	_, cleanup := stubFeishuServer(t, handler)
	defer cleanup()

	opts := ListMessagesOptions{
		ContainerIDType: "chat",
		PageSize:        10,
		CardContentType: CardMsgContentTypeUser,
	}
	if _, err := ListMessages(containerID, opts, userToken); err != nil {
		t.Fatalf("ListMessages 返回错误: %v", err)
	}

	if got := capturedQuery.Get("card_msg_content_type"); got != CardMsgContentTypeUser {
		t.Errorf("card_msg_content_type query: got %q, want %q", got, CardMsgContentTypeUser)
	}
	if got := capturedQuery.Get("container_id"); got != containerID {
		t.Errorf("container_id query: got %q, want %q", got, containerID)
	}
	if got := capturedQuery.Get("container_id_type"); got != "chat" {
		t.Errorf("container_id_type query: got %q, want %q", got, "chat")
	}
}

func TestGetMessageWithUserToken_ApiErrorPropagated(t *testing.T) {
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":230002,"msg":"permission denied","data":{}}`)
	}

	_, cleanup := stubFeishuServer(t, handler)
	defer cleanup()

	_, err := getMessageWithUserToken("om_xxx", "u-test", CardMsgContentTypeUser)
	if err == nil {
		t.Fatal("API 返回 code != 0 应返回错误")
	}
}
