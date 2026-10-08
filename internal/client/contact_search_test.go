package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestSearchUsersV3BodyAndProjection(t *testing.T) {
	var gotPath, gotQuery, gotAuth string
	var gotBody map[string]any
	business := func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery, gotAuth = r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"has_more":true,"page_token":"pt2","notice":"n","items":[
			{"id":"ou_a","display_info":"<h>张三</h>\n研发部\n[3 天前聊过]","meta_data":{"i18n_names":{"en_us":"San Zhang","zh_cn":"张三"},"enterprise_mail_address":"san@example.com","chat_id":"oc_p2p","is_registered":true}},
			{"id":"ou_b","display_info":"<h>张三</h>","meta_data":{"i18n_names":{"en_us":"Zhang"},"is_cross_tenant":true}}
		]}}`)
	}
	_, cleanup := stubFeishuServer(t, tenantRouteHandler(t, business))
	defer cleanup()

	res, err := SearchUsersV3(SearchUsersV3Options{Query: "张三", HasChatted: true, ExcludeExternalUsers: true, UserIDs: []string{"ou_a"}, PageSize: 10}, "u-test")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/open-apis/contact/v3/users/search" || !strings.Contains(gotQuery, "page_size=10") || gotAuth != "Bearer u-test" {
		t.Fatalf("请求不符: %s?%s auth=%s", gotPath, gotQuery, gotAuth)
	}
	filter, _ := gotBody["filter"].(map[string]any)
	if gotBody["query"] != "张三" || filter["has_contact"] != true || filter["exclude_outer_contact"] != true || filter["is_resigned"] != nil {
		t.Fatalf("请求体不符: %v", gotBody)
	}
	a, b := res.Users[0], res.Users[1]
	if a.Name != "张三" || a.Department != "研发部" || a.ChatRecencyHint != "3 天前聊过" || !a.HasChatted || a.P2PChatID != "oc_p2p" {
		t.Fatalf("投影不符: %+v", a)
	}
	if b.Name != "Zhang" || !b.IsCrossTenant || b.HasChatted || b.Department != "" {
		t.Fatalf("投影不符: %+v", b)
	}
	if !res.HasMore || res.PageToken != "pt2" || res.Notice != "n" {
		t.Fatalf("分页信息不符: %+v", res)
	}
	if _, err := SearchUsersV3(SearchUsersV3Options{Query: "x"}, ""); err == nil {
		t.Fatal("无 User Token 应报错")
	}
}

func TestSearchBotsParsesDisplayInfo(t *testing.T) {
	var gotBody map[string]any
	business := func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"has_more":false,"items":[{"id":"ou_bot","display_info":"告警<h>助手</h> &amp; 值班\n负责推送告警","meta_data":{"chat_id":"oc_b","enable_join_group":true,"is_agent":true,"tenant_id":"1"}}]}}`)
	}
	_, cleanup := stubFeishuServer(t, tenantRouteHandler(t, business))
	defer cleanup()
	res, err := SearchBots(SearchBotsOptions{Query: "助手", ChatIDs: []string{"oc_1"}, HasChatted: true}, "u-test")
	if err != nil {
		t.Fatal(err)
	}
	filter, _ := gotBody["filter"].(map[string]any)
	if gotBody["query"] != "助手" || filter["has_chatter"] != true || fmt.Sprint(filter["chat_ids"]) != "[oc_1]" {
		t.Fatalf("请求体不符: %v", gotBody)
	}
	bot := res.Bots[0]
	if bot.Name != "告警助手 & 值班" || bot.Description != "负责推送告警" || !bot.IsAgent || bot.MatchSegments[0] != "助手" {
		t.Fatalf("解析不符: %+v", bot)
	}
}

func TestGetUserBasicInfo(t *testing.T) {
	var gotQuery string
	business := func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"users":[{"user_id":"on_x","name":"张三","i18n_name":{"en_us":"San"}}]}}`)
	}
	_, cleanup := stubFeishuServer(t, tenantRouteHandler(t, business))
	defer cleanup()
	info, err := GetUserBasicInfo("on_x", "union_id", "u-test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotQuery, "user_id_type=union_id") || info.UnionID != "on_x" || info.Name != "张三" || info.EnName != "San" || info.OpenID != "" {
		t.Fatalf("query=%s info=%+v", gotQuery, info)
	}
}
