package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/viper"
)

func stubCmdFeishuServer(t *testing.T, handler http.HandlerFunc) func() {
	t.Helper()
	srv := httptest.NewServer(handler)

	viper.Reset()
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

	return srv.Close
}

func TestListMessagesViaSearchKeepsMergeForwardSubMessages(t *testing.T) {
	const (
		chatID    = "oc_test_chat"
		rootID    = "om_root"
		subID     = "om_sub_card"
		userToken = "u-test-token"
	)

	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/search/v2/message":
			_, _ = fmt.Fprintf(w, `{"code":0,"msg":"success","data":{"items":["%s"],"has_more":false,"page_token":""}}`, rootID)
		case r.URL.Path == "/open-apis/im/v1/messages/"+rootID:
			got := r.URL.Query().Get("card_msg_content_type")
			if got != client.CardMsgContentTypeUser && got != client.CardMsgContentTypeRaw {
				t.Errorf("card_msg_content_type = %q, want %q or %q", got, client.CardMsgContentTypeUser, client.CardMsgContentTypeRaw)
			}
			_, _ = fmt.Fprintf(w, `{"code":0,"msg":"success","data":{"items":[
				{"message_id":"%s","msg_type":"merge_forward","body":{"content":"placeholder"}},
				{"message_id":"%s","msg_type":"interactive","upper_message_id":"%s","body":{"content":"{\"body\":{\"elements\":[{\"tag\":\"plain_text\",\"content\":\"fallback 子卡片\"}]}}"}}
			]}}`, rootID, subID, rootID)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	})
	defer cleanup()

	result, err := listMessagesViaSearch(chatID, 10, "", userToken, client.CardMsgContentTypeUser)
	if err != nil {
		t.Fatalf("listMessagesViaSearch 返回错误: %v", err)
	}
	if len(result.Items) != 1 || client.StringVal(result.Items[0].MessageId) != rootID {
		t.Fatalf("Items = %#v, want root message", result.Items)
	}
	subs := result.MergeForwardSubMessages[rootID]
	if len(subs) != 1 || client.StringVal(subs[0].MessageId) != subID {
		t.Fatalf("MergeForwardSubMessages[%s] = %#v, want sub message %s", rootID, subs, subID)
	}
	if texts := client.ExtractCardTexts(subs[0]); len(texts) != 1 || !strings.Contains(texts[0], "fallback 子卡片") {
		t.Fatalf("sub card texts = %#v", texts)
	}
}
