package cmd

import (
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
