package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

const testSearchNotice = "The query is too long and has been truncated to the first 50 characters for search."

// TestSearchNoticeIsSurfaced 服务端在查询词超长时通过 data.notice 提示已截断；
// 消息搜索与群搜索都要透出（旧版丢弃，用户不知道结果只匹配了前 50 字）。
func TestSearchNoticeIsSurfaced(t *testing.T) {
	business := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/open-apis/im/v1/messages/search":
			_, _ = fmt.Fprintf(w, `{"code":0,"data":{"items":[{"meta_data":{"message_id":"om_1"}}],"has_more":false,"notice":%q}}`, testSearchNotice)
		case "/open-apis/im/v2/chats/search":
			_, _ = fmt.Fprintf(w, `{"code":0,"data":{"items":[],"has_more":false,"notice":%q}}`, testSearchNotice)
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
		}
	}
	_, cleanup := stubFeishuServer(t, tenantRouteHandler(t, business))
	defer cleanup()

	long := strings.Repeat("超长查询词", 12)
	msgRes, err := SearchMessages(SearchMessagesOptions{Query: long}, "u-test")
	if err != nil {
		t.Fatalf("SearchMessages() error = %v", err)
	}
	if msgRes.Notice != testSearchNotice {
		t.Fatalf("消息搜索 notice = %q", msgRes.Notice)
	}
	data, _ := json.Marshal(msgRes)
	if !strings.Contains(string(data), `"notice":`) || !strings.Contains(string(data), `"MessageIDs"`) {
		t.Fatalf("JSON 应保留旧字段并新增 notice: %s", data)
	}

	chatRes, err := SearchChats(SearchChatsOptions{Query: long}, "u-test")
	if err != nil {
		t.Fatalf("SearchChats() error = %v", err)
	}
	if chatRes.Notice != testSearchNotice {
		t.Fatalf("群搜索 notice = %q", chatRes.Notice)
	}

	// 无 notice 时 JSON 不出现该字段（omitempty，保持旧输出不变）
	empty, _ := json.Marshal(&SearchMessagesResult{MessageIDs: []string{"om_x"}})
	if strings.Contains(string(empty), "notice") {
		t.Fatalf("无 notice 时不应输出字段: %s", empty)
	}
}
