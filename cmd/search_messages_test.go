package cmd

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/output"
)

// TestSearchMessagesFlags 验证 flag 解析向后兼容：有 --enrich、无 --ids-only。
func TestSearchMessagesFlags(t *testing.T) {
	if f := searchMessagesCmd.Flags().Lookup("enrich"); f == nil {
		t.Fatal("缺少 --enrich flag")
	}
	if def := searchMessagesCmd.Flags().Lookup("enrich").DefValue; def != "false" {
		t.Errorf("--enrich 默认应为 false（opt-in），实际 %q", def)
	}
	if f := searchMessagesCmd.Flags().Lookup("ids-only"); f != nil {
		t.Error("--ids-only 已移除，不应再注册")
	}
	if f := searchMessagesCmd.Flags().Lookup("as"); f == nil || f.DefValue != "auto" {
		t.Fatal("缺少 --as auto")
	}
	if f := searchMessagesCmd.Flags().Lookup("is-at-me"); f == nil {
		t.Fatal("缺少 --is-at-me")
	}
	if f := searchMessagesCmd.Flags().Lookup("exclude-from-type"); f == nil {
		t.Fatal("缺少 --exclude-from-type")
	}
	uid := searchMessagesCmd.Flags().Lookup("user-id-type")
	if uid == nil || !uid.Hidden {
		t.Fatal("--user-id-type 应隐藏：current endpoint 忽略它")
	}
}

// TestSearchMessagesHelpMentionsEnrich 验证 Use/Long 反映 --enrich opt-in 且不再提 --ids-only。
func TestSearchMessagesHelpMentionsEnrich(t *testing.T) {
	long := searchMessagesCmd.Long
	if !strings.Contains(long, "--enrich") {
		t.Error("Long 帮助应提及 --enrich")
	}
	if strings.Contains(long, "--ids-only") {
		t.Error("Long 帮助不应再提及 --ids-only")
	}
	if !strings.Contains(long, "MessageIDs") {
		t.Error("Long 帮助应说明默认 -o json 返回 {MessageIDs,...} 旧 schema")
	}
}

// TestSearchMessagesDefaultJSONSchema 验证默认（非 enrich）路径渲染的是旧 schema
// {MessageIDs,PageToken,HasMore}（向后兼容），而非 snake_case 或 enriched 数组。
func TestSearchMessagesDefaultJSONSchema(t *testing.T) {
	o, err := output.NewOptions(output.FormatJSON, "")
	if err != nil {
		t.Fatalf("NewOptions 失败: %v", err)
	}
	res := &client.SearchMessagesResult{
		MessageIDs: []string{"om_xxx", "om_yyy"},
		PageToken:  "tok",
		HasMore:    true,
	}
	got, err := output.RenderString(o, res)
	if err != nil {
		t.Fatalf("RenderString 失败: %v", err)
	}
	// 旧 schema：Go 字段名大驼峰键
	for _, key := range []string{`"MessageIDs"`, `"PageToken"`, `"HasMore"`} {
		if !strings.Contains(got, key) {
			t.Errorf("默认 JSON 输出缺少旧 schema 键 %s，实际:\n%s", key, got)
		}
	}
	// 不应出现 enriched 数组特有字段或 snake_case
	for _, bad := range []string{`"message_ids"`, `"message_id"`, `"sender_name"`} {
		if strings.Contains(got, bad) {
			t.Errorf("默认 JSON 输出不应含 %s（那是 enrich/旧 map 形态），实际:\n%s", bad, got)
		}
	}
}

// TestCollectMessageIDsKeepsFirstPageNotice notice 通常只在首页下发，--page-all 翻页后仍需保留并透出。
func TestCollectMessageIDsKeepsFirstPageNotice(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	cleanup := stubCmdFeishuServer(t, tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page_token") == "" {
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"items":[{"meta_data":{"message_id":"om_1"}}],"has_more":true,"page_token":"p2","notice":"truncated to 50"}}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"items":[{"meta_data":{"message_id":"om_2"}}],"has_more":false}}`)
	}))
	defer cleanup()

	ids, last, err := collectMessageIDs(client.SearchMessagesOptions{Query: "x"}, "", true, 5)
	if err != nil {
		t.Fatalf("collectMessageIDs() error = %v", err)
	}
	if strings.Join(ids, ",") != "om_1,om_2" {
		t.Fatalf("ids = %v", ids)
	}
	if last.Notice != "truncated to 50" {
		t.Fatalf("翻页后 notice 丢失: %+v", last)
	}
	var buf bytes.Buffer
	printSearchNotice(&buf, last.Notice)
	if !strings.Contains(buf.String(), "truncated to 50") {
		t.Fatalf("stderr 提示缺失: %q", buf.String())
	}
}
