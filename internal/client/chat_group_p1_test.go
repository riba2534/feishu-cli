package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestBuildChatSearchBody(t *testing.T) {
	body := buildChatSearchBody(SearchChatsOptions{
		Query:     "a-b",
		MemberIDs: []string{"ou_1"},
		ChatModes: []string{"topic", "group", "topic"},
		Sort:      "member_count",
	})
	data, _ := json.Marshal(body)
	got := string(data)
	for _, want := range []string{`"query":"\"a-b\""`, `"member_ids":["ou_1"]`, `"chat_modes":["thread","default"]`, `"sorter":"member_count_desc"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("请求体缺少 %s: %s", want, got)
		}
	}
	if b := buildChatSearchBody(SearchChatsOptions{MemberIDs: []string{"ou_1"}}); b["query"] != nil {
		t.Fatalf("只按成员搜索时不应带 query: %v", b)
	}
}

// TestSearchChatsUsesSearchAPIWithMemberFilterOnly 只有 --member-ids 没有 query 时也走搜索接口（旧版回退到列表接口）。
func TestSearchChatsUsesSearchAPIWithMemberFilterOnly(t *testing.T) {
	var gotPath string
	business := func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"items":[{"meta_data":{"chat_id":"oc_1","name":"群","chat_mode":"THREAD"}}],"has_more":false}}`)
	}
	_, cleanup := stubFeishuServer(t, tenantRouteHandler(t, business))
	defer cleanup()
	res, err := SearchChats(SearchChatsOptions{MemberIDs: []string{"ou_1"}}, "u-test")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/open-apis/im/v2/chats/search" || len(res.Items) != 1 || res.Items[0].ChatMode != "THREAD" {
		t.Fatalf("path=%s items=%+v", gotPath, res.Items)
	}
}

func TestListChatsWithOptionsTypesAndFields(t *testing.T) {
	var gotQuery string
	business := func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"items":[{"chat_id":"oc_p2p","name":"张三","chat_mode":"p2p","p2p_target_id":"ou_z","p2p_target_type":"user"}],"has_more":false}}`)
	}
	_, cleanup := stubFeishuServer(t, tenantRouteHandler(t, business))
	defer cleanup()
	res, err := ListChatsWithOptions(ListChatsOptions{Types: "p2p,group", SortType: "ByActiveTimeDesc", PageSize: 10}, "u-test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotQuery, "types=p2p%2Cgroup") || !strings.Contains(gotQuery, "sort_type=ByActiveTimeDesc") {
		t.Fatalf("query = %s", gotQuery)
	}
	if res.Items[0].ChatMode != "p2p" || res.Items[0].P2PTargetID != "ou_z" {
		t.Fatalf("单聊字段丢失: %+v", res.Items[0])
	}
}

func TestBatchGetMuteStatusBatchesAndUnknown(t *testing.T) {
	var batches []int
	business := func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ChatIDs []string `json:"chat_ids"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		batches = append(batches, len(body.ChatIDs))
		var items []string
		for _, id := range body.ChatIDs {
			if id == "oc_unknown" {
				continue
			}
			items = append(items, fmt.Sprintf(`{"chat_id":%q,"is_muted":%v}`, id, strings.HasSuffix(id, "_m")))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"code":0,"data":{"items":[%s],"invalid_id_list":[]}}`, strings.Join(items, ","))
	}
	_, cleanup := stubFeishuServer(t, tenantRouteHandler(t, business))
	defer cleanup()

	ids := []string{"oc_unknown", "oc_1_m", "oc_1_m"}
	for i := 0; i < 120; i++ {
		ids = append(ids, fmt.Sprintf("oc_n%d", i))
	}
	muted, unknown, err := BatchGetMuteStatus(ids, "u-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 2 || batches[0] != 100 || batches[1] != 22 {
		t.Fatalf("应按 100 一批去重后分批，got %v", batches)
	}
	if !muted["oc_1_m"] || muted["oc_n1"] || len(unknown) != 1 || unknown[0] != "oc_unknown" {
		t.Fatalf("muted=%v unknown=%v", muted["oc_1_m"], unknown)
	}
	if _, _, err := BatchGetMuteStatus(ids, ""); err == nil {
		t.Fatal("Bot 身份应报错（免打扰是个人设置）")
	}
}

func TestCreateChatWithOptionsBody(t *testing.T) {
	var body map[string]any
	var query string
	business := func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"chat_id":"oc_new","name":"fp","chat_mode":"topic","chat_type":"private"}}`)
	}
	_, cleanup := stubFeishuServer(t, tenantRouteHandler(t, business))
	defer cleanup()
	created, err := CreateChatWithOptions(CreateChatOptions{Name: "fp", ChatMode: "topic", BotIDs: []string{"cli_x"}, UserIDs: []string{"ou_1"}})
	if err != nil {
		t.Fatal(err)
	}
	if body["chat_mode"] != "topic" || fmt.Sprint(body["bot_id_list"]) != "[cli_x]" || !strings.Contains(query, "user_id_type=open_id") {
		t.Fatalf("body=%v query=%s", body, query)
	}
	if created.ChatID != "oc_new" || created.ChatMode != "topic" {
		t.Fatalf("created=%+v", created)
	}
}
