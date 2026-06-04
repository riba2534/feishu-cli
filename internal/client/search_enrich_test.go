package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

func TestExtractMessageText(t *testing.T) {
	cases := []struct {
		msgType, content, want string
	}{
		{"text", `{"text":"hello world"}`, "hello world"},
		{"post", `{"title":"标题","content":[[{"tag":"text","text":"正文1"},{"tag":"a","text":"链接"}]]}`, "标题 正文1 链接"},
		{"image", `{"image_key":"img_x"}`, "[image]"},
		{"file", `{"file_key":"f"}`, "[file]"},
	}
	for _, c := range cases {
		msg := &larkim.Message{MsgType: strPtr(c.msgType), Body: &larkim.MessageBody{Content: strPtr(c.content)}}
		if got := ExtractMessageText(msg); got != c.want {
			t.Errorf("ExtractMessageText(%s) = %q, want %q", c.msgType, got, c.want)
		}
	}
	// 语言包裹 post
	wrapped := &larkim.Message{MsgType: strPtr("post"), Body: &larkim.MessageBody{Content: strPtr(`{"zh_cn":{"title":"T","content":[[{"tag":"text","text":"中文"}]]}}`)}}
	if got := ExtractMessageText(wrapped); !strings.Contains(got, "中文") {
		t.Errorf("语言包裹 post 提取失败: %q", got)
	}
	// nil 安全
	if got := ExtractMessageText(nil); got != "" {
		t.Errorf("nil 应返回空, 得 %q", got)
	}
}

func TestFormatMsgTime(t *testing.T) {
	if got := formatMsgTime(""); got != "" {
		t.Errorf("空应返回空, 得 %q", got)
	}
	if got := formatMsgTime("notanumber"); got != "notanumber" {
		t.Errorf("非法应返回原值, 得 %q", got)
	}
	// 合法毫秒戳应被格式化（含 '-' 与 ':'）
	got := formatMsgTime("1700000000000")
	if !strings.Contains(got, "-") || !strings.Contains(got, ":") {
		t.Errorf("合法戳应格式化, 得 %q", got)
	}
}

func TestEnrichMessages(t *testing.T) {
	msgs := []*larkim.Message{
		{
			MessageId:  strPtr("om_1"),
			MsgType:    strPtr("text"),
			ChatId:     strPtr("oc_1"),
			CreateTime: strPtr("1700000000000"),
			Sender:     &larkim.Sender{Id: strPtr("ou_a"), SenderType: strPtr("user")},
			Body:       &larkim.MessageBody{Content: strPtr(`{"text":"hi"}`)},
		},
		nil, // 应被跳过
	}
	senderNames := map[string]string{"ou_a": "Alice"}
	chatNames := map[string]string{"oc_1": "测试群"}
	got := EnrichMessages(msgs, senderNames, chatNames)
	if len(got) != 1 {
		t.Fatalf("应得 1 条（nil 跳过），得 %d", len(got))
	}
	e := got[0]
	if e.MessageID != "om_1" || e.ChatName != "测试群" || e.SenderName != "Alice" || e.Text != "hi" {
		t.Errorf("enrich 结果不对: %+v", e)
	}
}

func TestSearchMessagesEnrichedIntegration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/search/v2/message":
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"items":["om_1"],"has_more":false,"page_token":""}}`)
		case r.URL.Path == "/open-apis/im/v1/messages/om_1":
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"items":[{
				"message_id":"om_1","msg_type":"text","create_time":"1700000000000","chat_id":"oc_1",
				"sender":{"id":"ou_sender","sender_type":"user","id_type":"open_id"},
				"mentions":[{"id":"ou_sender","name":"发送者A","key":"@_user_1"}],
				"body":{"content":"{\"text\":\"hello world\"}"}
			}]}}`)
		case r.URL.Path == "/open-apis/im/v1/chats/oc_1":
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"name":"测试群","chat_id":"oc_1"}}`)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer srv.Close()
	setupTestConfig(t, srv.URL)

	enriched, res, err := SearchMessagesEnriched(SearchMessagesOptions{Query: "hello", PageSize: 10}, "u-test-token", "user")
	if err != nil {
		t.Fatalf("SearchMessagesEnriched error: %v", err)
	}
	if res == nil || res.HasMore {
		t.Errorf("分页结果不对: %+v", res)
	}
	if len(enriched) != 1 {
		t.Fatalf("应得 1 条 enriched，得 %d", len(enriched))
	}
	e := enriched[0]
	if e.MessageID != "om_1" {
		t.Errorf("message_id = %q", e.MessageID)
	}
	if e.Text != "hello world" {
		t.Errorf("text = %q, want 'hello world'", e.Text)
	}
	if e.SenderName != "发送者A" {
		t.Errorf("sender_name = %q, want 发送者A", e.SenderName)
	}
	if e.ChatName != "测试群" {
		t.Errorf("chat_name = %q, want 测试群", e.ChatName)
	}
	if e.ChatID != "oc_1" {
		t.Errorf("chat_id = %q", e.ChatID)
	}
}

func TestSearchMessagesEnrichedEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"items":[],"has_more":false,"page_token":""}}`)
	}))
	defer srv.Close()
	setupTestConfig(t, srv.URL)

	enriched, res, err := SearchMessagesEnriched(SearchMessagesOptions{Query: "none"}, "u-test", "user")
	if err != nil {
		t.Fatalf("空结果不应报错: %v", err)
	}
	if len(enriched) != 0 || res == nil {
		t.Errorf("空结果处理不对: enriched=%d res=%v", len(enriched), res)
	}
}

// TestSearchMessagesEnrichedBestEffort 验证 BUG#7 修复：--enrich 用 best-effort 批量取详情，
// 个别消息 GetMessage 失败（撤回/退群/无可见性）时跳过该条、不让整页富化归零，且保留其余成功项的顺序。
func TestSearchMessagesEnrichedBestEffort(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/open-apis/search/v2/message":
			// 三条结果，中间一条不可读
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"items":["om_ok1","om_bad","om_ok2"],"has_more":false,"page_token":""}}`)
		case "/open-apis/im/v1/messages/om_ok1":
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"items":[{"message_id":"om_ok1","msg_type":"text","create_time":"1700000000000","chat_id":"oc_1","body":{"content":"{\"text\":\"first\"}"}}]}}`)
		case "/open-apis/im/v1/messages/om_bad":
			// 模拟不可读消息（撤回/退群/无可见性）→ GetMessage 返回 error
			_, _ = io.WriteString(w, `{"code":230002,"msg":"message not found"}`)
		case "/open-apis/im/v1/messages/om_ok2":
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"items":[{"message_id":"om_ok2","msg_type":"text","create_time":"1700000001000","chat_id":"oc_1","body":{"content":"{\"text\":\"third\"}"}}]}}`)
		case "/open-apis/im/v1/chats/oc_1":
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"name":"群","chat_id":"oc_1"}}`)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer srv.Close()
	setupTestConfig(t, srv.URL)

	enriched, res, err := SearchMessagesEnriched(SearchMessagesOptions{Query: "x", PageSize: 10}, "u-test", "user")
	// best-effort：个别消息失败不应让整批报错（修复前严格版会整页归零）
	if err != nil {
		t.Fatalf("best-effort 下个别消息失败不应整批报错，得: %v", err)
	}
	if res == nil {
		t.Fatal("res 不应为 nil")
	}
	// 失败的 om_bad 被跳过，保留 2 条成功的（保持搜索返回顺序）
	if len(enriched) != 2 {
		t.Fatalf("应保留 2 条成功 enriched（om_bad 跳过），得 %d", len(enriched))
	}
	if enriched[0].MessageID != "om_ok1" || enriched[1].MessageID != "om_ok2" {
		t.Errorf("顺序/内容不对: %s, %s", enriched[0].MessageID, enriched[1].MessageID)
	}
	if enriched[0].Text != "first" || enriched[1].Text != "third" {
		t.Errorf("text 不对: %q, %q", enriched[0].Text, enriched[1].Text)
	}
}
