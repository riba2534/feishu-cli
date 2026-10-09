package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// TestListChatMembersUsesMembersListEndpoint 验证群成员列表走 /members/list（能拿到 bots），
// users 与旧字段 items 一致且补齐 member_id_type，bots / truncations / *_total 透出。
func TestListChatMembersUsesMembersListEndpoint(t *testing.T) {
	var gotPath, gotQuery, gotAuth string
	business := func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{
			"users":[{"member_id":"ou_user_1","name":"张三","tenant_key":"t1"}],
			"bots":[{"member_id":"ou_bot_1","name":"告警机器人","app_id":"cli_bot_1","tenant_key":"t1"}],
			"truncations":[{"member_type":"user","limit":500}],
			"user_total":800,"bot_total":1,"has_more":true,"page_token":"next_tok"}}`)
	}
	_, cleanup := stubFeishuServer(t, tenantRouteHandler(t, business))
	defer cleanup()

	res, err := ListChatMembersV2("oc_test", ChatMembersListOptions{MemberTypes: "user,bot", PageSize: 50, PageToken: "tok0"}, "")
	if err != nil {
		t.Fatalf("ListChatMembersV2() error = %v", err)
	}
	if gotPath != "/open-apis/im/v1/chats/oc_test/members/list" {
		t.Fatalf("path = %q，应走 /members/list", gotPath)
	}
	for _, want := range []string{"member_id_type=open_id", "member_types=user%2Cbot", "page_size=50", "page_token=tok0"} {
		if !containsQuery(gotQuery, want) {
			t.Fatalf("query 缺少 %s: %s", want, gotQuery)
		}
	}
	if gotAuth != "Bearer t-fake" {
		t.Fatalf("空 User Token 应走 Bot 身份，Authorization = %q", gotAuth)
	}
	if len(res.Items) != 1 || res.Items[0].MemberID != "ou_user_1" || res.Items[0].MemberIDType != "open_id" {
		t.Fatalf("items = %+v", res.Items)
	}
	if len(res.Bots) != 1 || res.Bots[0].AppID != "cli_bot_1" || res.Bots[0].Name != "告警机器人" {
		t.Fatalf("bots = %+v", res.Bots)
	}
	if len(res.Truncations) != 1 || res.UserTotal == nil || *res.UserTotal != 800 {
		t.Fatalf("truncations/user_total 丢失: %+v", res)
	}
	if !res.HasMore || res.PageToken != "next_tok" {
		t.Fatalf("分页信息丢失: %+v", res)
	}

	data, _ := json.Marshal(res)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	for _, k := range []string{"items", "users", "bots", "truncations", "has_more", "page_token", "user_total", "bot_total"} {
		if _, ok := out[k]; !ok {
			t.Fatalf("JSON 输出缺少字段 %s: %s", k, data)
		}
	}
}

// TestListChatMembersBusinessErrorKeepsCode 外部群 232033 等业务错误随 HTTP 400 下发时保留业务码。
func TestListChatMembersBusinessErrorKeepsCode(t *testing.T) {
	business := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"code":232033,"msg":"The operator or invited bots does NOT have the authority to manage external chats"}`)
	}
	_, cleanup := stubFeishuServer(t, tenantRouteHandler(t, business))
	defer cleanup()

	_, err := ListChatMembers("oc_ext", "open_id", 0, "", "u-test")
	if !HasAPICode(err, 232033) {
		t.Fatalf("期望保留业务码 232033，got %v", err)
	}
}

func TestListChatMembersResultAppendPageKeepsLastPageSignals(t *testing.T) {
	total1, total2 := 3, 5
	all := &ListChatMembersResult{}
	all.AppendPage(&ListChatMembersResult{
		Items:       []*ChatMemberInfo{{MemberID: "ou_1"}},
		Bots:        []*ChatBotMemberInfo{{MemberID: "ou_b1"}},
		Truncations: []map[string]any{},
		UserTotal:   &total1, HasMore: true, PageToken: "p2",
	})
	all.AppendPage(&ListChatMembersResult{
		Items:       []*ChatMemberInfo{{MemberID: "ou_2"}},
		Truncations: []map[string]any{{"member_type": "user", "limit": 2}},
		UserTotal:   &total2, HasMore: false,
	})
	if len(all.Items) != 2 || len(all.Users) != 2 || len(all.Bots) != 1 {
		t.Fatalf("合并后成员数量不对: %+v", all)
	}
	if len(all.Truncations) != 1 || *all.UserTotal != 5 || all.HasMore || all.PageToken != "" {
		t.Fatalf("应取最后一页的 truncations/total/has_more: %+v", all)
	}
}

func containsQuery(rawQuery, kv string) bool {
	for _, part := range splitQuery(rawQuery) {
		if part == kv {
			return true
		}
	}
	return false
}

func splitQuery(q string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(q); i++ {
		if i == len(q) || q[i] == '&' {
			out = append(out, q[start:i])
			start = i + 1
		}
	}
	return out
}
