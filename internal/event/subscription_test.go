package event

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
)

func planBodies(t *testing.T, key string, raw ...string) []SubscriptionCall {
	t.Helper()
	def := mustLookup(t, key)
	params, err := ParseParams([]KeyDefinition{def}, raw)
	if err != nil {
		t.Fatalf("ParseParams(%v): %v", raw, err)
	}
	calls, err := SubscriptionPlan(def, params)
	if err != nil {
		t.Fatalf("SubscriptionPlan: %v", err)
	}
	return calls
}

func bodyJSON(t *testing.T, body any) string {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// 审批不传 --param 仍注册两种类型（与旧版一致）；--param 只注册指定类型。
func TestApprovalSubscriptionTypeParam(t *testing.T) {
	calls := planBodies(t, "approval.instance.status_changed_v4")
	if len(calls) != 2 || bodyJSON(t, calls[0].Body) != `{"subscription_type":"INVOLVED_APPROVAL"}` ||
		bodyJSON(t, calls[1].Body) != `{"subscription_type":"MANAGED_APPROVAL"}` {
		t.Fatalf("默认应注册两种类型，got %+v", calls)
	}
	calls = planBodies(t, "approval.task.status_changed_v4", "subscription_type=MANAGED_APPROVAL")
	if len(calls) != 1 || bodyJSON(t, calls[0].Body) != `{"subscription_type":"MANAGED_APPROVAL"}` {
		t.Fatalf("--param 应只注册 MANAGED，got %+v", calls)
	}
	// 重复传入 / 逗号分隔均可，按声明顺序去重
	calls = planBodies(t, "approval.task.status_changed_v4", "subscription_type=MANAGED_APPROVAL", "subscription_type=INVOLVED_APPROVAL,MANAGED_APPROVAL")
	if len(calls) != 2 || calls[0].Label != "subscription_type=INVOLVED_APPROVAL" {
		t.Fatalf("多值应去重并按声明顺序，got %+v", calls)
	}
}

func TestParseParamsValidation(t *testing.T) {
	approval := mustLookup(t, "approval.instance.status_changed_v4")
	board := mustLookup(t, "board.whiteboard.updated_v1")
	im := mustLookup(t, "im.message.receive_v1")
	cases := []struct {
		name string
		defs []KeyDefinition
		raw  []string
		want string
	}{
		{"格式错误", []KeyDefinition{approval}, []string{"subscription_type"}, "key=value"},
		{"未知参数", []KeyDefinition{approval}, []string{"x=1"}, "未知参数"},
		{"key 无参数", []KeyDefinition{im}, []string{"x=1"}, "不接受任何参数"},
		{"枚举非法", []KeyDefinition{approval}, []string{"subscription_type=BAD"}, "无效"},
		{"空值", []KeyDefinition{approval}, []string{"subscription_type="}, "不能为空"},
		{"必填缺失", []KeyDefinition{board}, nil, "whiteboard_id"},
		{"非 Multi 重复", []KeyDefinition{board}, []string{"whiteboard_id=a", "whiteboard_id=b"}, "只能指定一次"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseParams(c.defs, c.raw)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error containing %q, got %v", c.want, err)
			}
		})
	}
	params, err := ParseParams([]KeyDefinition{mustLookup(t, "mail.user_mailbox.event.message_received_v1")}, nil)
	if err != nil || params["mailbox"] != "me" {
		t.Fatalf("mailbox 默认应为 me，got %v %v", params, err)
	}
}

func TestNewKeySubscriptionPlans(t *testing.T) {
	// 画板：路径段转义，退出注销，body event_type
	calls := planBodies(t, "board.whiteboard.updated_v1", "whiteboard_id=wb/1?x")
	if len(calls) != 1 || calls[0].Path != "/open-apis/board/v1/whiteboards/wb%2F1%3Fx/subscribe" ||
		calls[0].UnsubscribePath != "/open-apis/board/v1/whiteboards/wb%2F1%3Fx/unsubscribe" ||
		bodyJSON(t, calls[0].Body) != `{"event_type":"board.whiteboard.updated_v1"}` {
		t.Fatalf("画板计划错误: %+v", calls)
	}
	// 任务：无 body，不注销
	calls = planBodies(t, "task.task.update_user_access_v2")
	if len(calls) != 1 || calls[0].Body != nil || calls[0].UnsubscribePath != "" ||
		calls[0].Path != "/open-apis/task/v2/task_v2/task_subscription?user_id_type=open_id" {
		t.Fatalf("任务计划错误: %+v", calls)
	}
	// 云文档评论：event_type body，不注销
	calls = planBodies(t, "drive.notice.comment_add_v1")
	if len(calls) != 1 || calls[0].UnsubscribePath != "" || calls[0].Path != "/open-apis/drive/v1/user/subscription" ||
		bodyJSON(t, calls[0].Body) != `{"event_type":"drive.notice.comment_add_v1"}` {
		t.Fatalf("评论计划错误: %+v", calls)
	}
	// 妙记：与 VC 同模型
	calls = planBodies(t, "minutes.minute.generated_v1")
	if len(calls) != 1 || calls[0].Path != "/open-apis/minutes/v1/minutes/subscription" ||
		calls[0].UnsubscribePath != "/open-apis/minutes/v1/minutes/unsubscription" ||
		bodyJSON(t, calls[0].Body) != `{"event_type":"minutes.minute.generated_v1"}` {
		t.Fatalf("妙记计划错误: %+v", calls)
	}
	// 邮箱：int event_type，路径带 mailbox
	calls = planBodies(t, "mail.user_mailbox.event.message_received_v1", "mailbox=user@example.com")
	if len(calls) != 1 || calls[0].Path != "/open-apis/mail/v1/user_mailboxes/user@example.com/event/subscribe" ||
		bodyJSON(t, calls[0].Body) != `{"event_type":1}` ||
		calls[0].UnsubscribePath != "/open-apis/mail/v1/user_mailboxes/user@example.com/event/unsubscribe" {
		t.Fatalf("邮箱计划错误: %+v", calls)
	}
}

func TestNewKeysNotInCommonAckList(t *testing.T) {
	newKeys := []string{"drive.notice.comment_add_v1", "task.task.update_user_access_v2", "minutes.minute.generated_v1",
		"board.whiteboard.updated_v1", "mail.user_mailbox.event.message_received_v1"}
	for _, k := range newKeys {
		mustLookup(t, k)
		if slices.Contains(commonAckEventTypes, k) {
			t.Errorf("%s 已注册为 EventKey，应从 commonAckEventTypes 移出", k)
		}
		if !slices.Contains(ackOnlyEventTypes(), k) {
			t.Errorf("%s 仍应在 ACK 集合内（来自 keyRegistry）", k)
		}
	}
}

func TestIdentityRules(t *testing.T) {
	vc := mustLookup(t, "vc.note.generated_v1")
	drive := mustLookup(t, "drive.notice.comment_add_v1")
	im := mustLookup(t, "im.message.receive_v1")
	contact := mustLookup(t, "contact.user.created_v3")
	if err := CheckIdentity([]KeyDefinition{vc}, "bot"); err == nil {
		t.Fatal("VC 不支持 bot，应报错")
	}
	if err := CheckIdentity([]KeyDefinition{drive}, "bot"); err != nil {
		t.Fatalf("评论支持 bot: %v", err)
	}
	if err := CheckIdentity([]KeyDefinition{contact}, "user"); err != nil {
		t.Fatalf("未声明身份且无订阅的 key 不限制: %v", err)
	}
	// 订阅 key 未声明 AuthTypes 时默认 [user]
	sub := KeyDefinition{Key: "x", SubscribePath: "/s"}
	if got := sub.IdentityTypes(); len(got) != 1 || got[0] != "user" {
		t.Fatalf("默认 AuthTypes 应为 [user]，got %v", got)
	}
	cases := []struct {
		defs    []KeyDefinition
		hasUser bool
		want    string
	}{
		{[]KeyDefinition{im}, true, ""},
		{[]KeyDefinition{vc}, false, "user"},
		{[]KeyDefinition{vc}, true, "user"},
		{[]KeyDefinition{drive}, true, "user"},
		{[]KeyDefinition{drive}, false, "bot"},
		{[]KeyDefinition{drive, vc}, false, "user"},
	}
	for i, c := range cases {
		if got := ResolveAutoIdentity(c.defs, c.hasUser); got != c.want {
			t.Errorf("case %d: got %q want %q", i, got, c.want)
		}
	}
}

// --as bot 时订阅请求用 App Token；任务订阅不带 body。
func TestRegisterSubscriptionsAsBotNoBody(t *testing.T) {
	var mu sync.Mutex
	var gotAuth, gotBody, gotCT, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		gotQuery = r.URL.RawQuery
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Write([]byte(`{"code":0,"msg":"ok"}`))
	}))
	defer srv.Close()
	r := NewRuntime(ConsumeOptions{
		BaseURL:           srv.URL,
		UserAccessToken:   "u-should-not-use",
		SubscribeAsBot:    true,
		TenantAccessToken: "t-bot",
		ErrOut:            io.Discard,
	})
	if err := r.registerSubscriptions(context.Background(), mustLookup(t, "task.task.update_user_access_v2")); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if gotAuth != "Bearer t-bot" {
		t.Errorf("Authorization = %q, want Bearer t-bot", gotAuth)
	}
	if gotBody != "" || gotCT != "" {
		t.Errorf("任务订阅不应带 body，got body=%q ct=%q", gotBody, gotCT)
	}
	if gotQuery != "user_id_type=open_id" {
		t.Errorf("query = %q", gotQuery)
	}

	r2 := NewRuntime(ConsumeOptions{BaseURL: srv.URL, SubscribeAsBot: true, ErrOut: io.Discard})
	if err := r2.registerSubscriptions(context.Background(), mustLookup(t, "task.task.update_user_access_v2")); err == nil ||
		!strings.Contains(err.Error(), "App Token") {
		t.Fatalf("缺 App Token 应报错，got %v", err)
	}
}

// 画板事件按 --param whiteboard_id 过滤：其他画板的事件不输出。
// 确认是唯一长连接（AckUnsubscribed）时 ACK 丢弃；否则返回错误让服务端重投给订阅该画板的连接。
func TestBuildDispatcherAppliesMatch(t *testing.T) {
	payload := func(id, wb string) []byte {
		return []byte(`{"schema":"2.0","header":{"event_id":"` + id + `","event_type":"board.whiteboard.updated_v1","create_time":"1"},"event":{"whiteboard_id":"` + wb + `"}}`)
	}
	for _, ackUnsub := range []bool{true, false} {
		var out concurrentBuffer
		r := NewRuntime(ConsumeOptions{
			EventKey:        "board.whiteboard.updated_v1",
			Params:          map[string]string{"whiteboard_id": "wb_a"},
			Out:             &out,
			ErrOut:          io.Discard,
			AckUnsubscribed: ackUnsub,
		})
		dis := r.buildDispatcher([]KeyDefinition{mustLookup(t, "board.whiteboard.updated_v1")})
		_, err := dis.Do(context.Background(), payload("ev1", "wb_b"))
		if ackUnsub && err != nil {
			t.Fatalf("唯一连接时其他画板事件应 ACK: %v", err)
		}
		if !ackUnsub && err == nil {
			t.Fatal("存在其他连接时其他画板事件不得 ACK（应返回错误交由服务端重投）")
		}
		if _, err := dis.Do(context.Background(), payload("ev2", "wb_a")); err != nil {
			t.Fatal(err)
		}
		got := out.String()
		if strings.Contains(got, "wb_b") || !strings.Contains(got, "wb_a") {
			t.Fatalf("应只输出 wb_a 的事件，got %q", got)
		}
	}
}

// 订阅范围包含订阅身份与该 key 声明的参数；默认身份无参数时为空（兼容旧 bus.json）。
func TestSubscriptionScope(t *testing.T) {
	board := mustLookup(t, "board.whiteboard.updated_v1")
	a := NewRuntime(ConsumeOptions{Params: map[string]string{"whiteboard_id": "wb_a", "unrelated": "x"}})
	b := NewRuntime(ConsumeOptions{Params: map[string]string{"whiteboard_id": "wb_b"}, SubscribeAsBot: true})
	if got := a.subscriptionScope(board); got != "whiteboard_id=wb_a" {
		t.Fatalf("scope = %q", got)
	}
	if got := b.subscriptionScope(board); got != "as=bot;whiteboard_id=wb_b" {
		t.Fatalf("scope = %q", got)
	}
	vc := mustLookup(t, "vc.note.generated_v1")
	if got := NewRuntime(ConsumeOptions{}).subscriptionScope(vc); got != "" {
		t.Fatalf("默认身份无参数应为空 scope，got %q", got)
	}
}

// --as 只约束需要服务端订阅的 key：IM 消息（仅长连接）与 user-only 订阅 key 组合时 --as user 合法。
func TestCheckIdentitySkipsKeysWithoutSubscription(t *testing.T) {
	defs := []KeyDefinition{mustLookup(t, "im.message.receive_v1"), mustLookup(t, "approval.instance.status_changed_v4")}
	if err := CheckIdentity(defs, "user"); err != nil {
		t.Fatalf("IM + 审批 --as user 应合法: %v", err)
	}
	if err := CheckIdentity([]KeyDefinition{mustLookup(t, "approval.instance.status_changed_v4")}, "bot"); err == nil {
		t.Fatal("审批只支持 user，--as bot 应报错")
	}
}
