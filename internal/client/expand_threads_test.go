package client

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// stringPtr 测试用的 string 字面量 → *string 辅助。
func stringPtr(s string) *string { return &s }

// TestExpandThreadReplies_Basic 验证：
//  1. 带 thread_id 的根消息会触发拉取
//  2. 同一 thread_id 只调一次 API（去重）
//  3. 根消息本身（thread API 会一起返回）被过滤掉
//  4. 结果填到 result.ThreadReplies
func TestExpandThreadReplies_Basic(t *testing.T) {
	const userToken = "u-test"
	const rootA = "om_root_a"
	const rootB = "om_root_b"
	const tid1 = "omt_thread_one"
	const tid2 = "omt_thread_two"

	var (
		mu         sync.Mutex
		calledTIDs []string
	)

	handler := func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("container_id_type") != "thread" {
			t.Errorf("期望 container_id_type=thread, got %q", q.Get("container_id_type"))
		}
		if q.Get("sort_type") != "ByCreateTimeAsc" {
			t.Errorf("期望 ByCreateTimeAsc, got %q", q.Get("sort_type"))
		}
		tid := q.Get("container_id")
		mu.Lock()
		calledTIDs = append(calledTIDs, tid)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch tid {
		case tid1:
			// 含根消息 + 2 条回复（根消息应被过滤）
			_, _ = fmt.Fprintf(w, `{"code":0,"msg":"ok","data":{"items":[
				{"message_id":"%s","msg_type":"text","thread_id":"%s","body":{"content":"{\"text\":\"root_a\"}"}},
				{"message_id":"om_reply_1","msg_type":"text","thread_id":"%s","body":{"content":"{\"text\":\"r1\"}"}},
				{"message_id":"om_reply_2","msg_type":"text","thread_id":"%s","body":{"content":"{\"text\":\"r2\"}"}}
			],"has_more":false,"page_token":""}}`, rootA, tid1, tid1, tid1)
		case tid2:
			_, _ = fmt.Fprintf(w, `{"code":0,"msg":"ok","data":{"items":[
				{"message_id":"om_reply_3","msg_type":"text","thread_id":"%s","body":{"content":"{\"text\":\"r3\"}"}}
			],"has_more":true,"page_token":"next_tok"}}`, tid2)
		default:
			t.Errorf("意外的 thread_id 请求: %q", tid)
		}
	}

	_, cleanup := stubFeishuServer(t, handler)
	defer cleanup()

	result := &ListMessagesResult{
		Items: []*larkim.Message{
			{MessageId: stringPtr(rootA), ThreadId: stringPtr(tid1), MsgType: stringPtr("text")},
			{MessageId: stringPtr(rootB), ThreadId: stringPtr(tid2), MsgType: stringPtr("text")},
			// 同 tid1 第二次出现，不应再次触发 API
			{MessageId: stringPtr("om_dup"), ThreadId: stringPtr(tid1), MsgType: stringPtr("text")},
			// 不带 thread_id 的普通消息
			{MessageId: stringPtr("om_plain"), MsgType: stringPtr("text")},
		},
	}

	ExpandThreadReplies(result, userToken, 50, 500)

	// 验证去重：tid1 + tid2 各调一次
	mu.Lock()
	seenTIDs := map[string]int{}
	for _, t := range calledTIDs {
		seenTIDs[t]++
	}
	mu.Unlock()
	if seenTIDs[tid1] != 1 || seenTIDs[tid2] != 1 {
		t.Errorf("期望每个 thread 调一次, 实际 %v", seenTIDs)
	}

	// 验证根消息被过滤掉
	if len(result.ThreadReplies[tid1]) != 2 {
		t.Errorf("tid1 期望 2 条回复（根消息已过滤）, got %d", len(result.ThreadReplies[tid1]))
	}
	for _, r := range result.ThreadReplies[tid1] {
		if StringVal(r.MessageId) == rootA {
			t.Errorf("根消息 %s 未被过滤", rootA)
		}
	}
	if len(result.ThreadReplies[tid2]) != 1 {
		t.Errorf("tid2 期望 1 条回复, got %d", len(result.ThreadReplies[tid2]))
	}

	// 验证 has_more 透传
	if !result.ThreadHasMore[tid2] {
		t.Errorf("tid2 应有 has_more=true")
	}
	if result.ThreadHasMore[tid1] {
		t.Errorf("tid1 不应 has_more")
	}
}

// TestExpandThreadReplies_TotalLimit 验证 totalLimit 触发后提前停止。
func TestExpandThreadReplies_TotalLimit(t *testing.T) {
	var mu sync.Mutex
	var callCount int

	business := func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()
		// 每个 thread 返回 5 条回复
		tid := r.URL.Query().Get("container_id")
		var items []string
		for i := 0; i < 5; i++ {
			items = append(items, fmt.Sprintf(
				`{"message_id":"om_r_%s_%d","msg_type":"text","thread_id":"%s"}`,
				tid, i, tid))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"code":0,"msg":"ok","data":{"items":[%s],"has_more":false,"page_token":""}}`,
			strings.Join(items, ","))
	}

	// 用 tenantRouteHandler 包一层：tenant_access_token 取 token 请求由它处理，
	// 不会被误算入业务 callCount。
	_, cleanup := stubFeishuServer(t, tenantRouteHandler(t, business))
	defer cleanup()

	// 3 个不同 thread，每个返回 5 条；totalLimit=7 应在第 2 个 thread 后停止
	result := &ListMessagesResult{
		Items: []*larkim.Message{
			{MessageId: stringPtr("om_1"), ThreadId: stringPtr("omt_a")},
			{MessageId: stringPtr("om_2"), ThreadId: stringPtr("omt_b")},
			{MessageId: stringPtr("om_3"), ThreadId: stringPtr("omt_c")},
		},
	}
	ExpandThreadReplies(result, "", 5, 7)

	mu.Lock()
	got := callCount
	mu.Unlock()
	if got != 2 {
		t.Errorf("totalLimit=7 期望调 2 次 (5+5≥7 即停), 实际 %d", got)
	}
	if len(result.ThreadReplies) != 2 {
		t.Errorf("期望填了 2 个 thread, got %d", len(result.ThreadReplies))
	}
}

// TestExpandThreadReplies_EdgeCases 覆盖空输入 / nil 消息 / 无 thread_id 全跳过。
func TestExpandThreadReplies_EdgeCases(t *testing.T) {
	// nil result 不 panic
	ExpandThreadReplies(nil, "", 0, 0)

	// 空 Items 不 panic
	r1 := &ListMessagesResult{}
	ExpandThreadReplies(r1, "", 0, 0)

	// 所有消息都没 thread_id：不应触发任何 API
	handler := func(w http.ResponseWriter, _ *http.Request) {
		t.Errorf("不应触发 thread API")
		_, _ = w.Write([]byte(`{}`))
	}
	_, cleanup := stubFeishuServer(t, handler)
	defer cleanup()

	r2 := &ListMessagesResult{
		Items: []*larkim.Message{
			{MessageId: stringPtr("om_1"), MsgType: stringPtr("text")},
			{MessageId: stringPtr("om_2"), MsgType: stringPtr("text")},
		},
	}
	ExpandThreadReplies(r2, "u-test", 50, 500)
	if len(r2.ThreadReplies) != 0 {
		t.Errorf("期望无 ThreadReplies, got %d", len(r2.ThreadReplies))
	}
}

// 静默 unused import
var _ = url.QueryEscape
