package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func resetCmdFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			_ = sv.Replace(nil)
		} else {
			_ = f.Value.Set(f.DefValue)
		}
		f.Changed = false
	})
}

func TestParseMailRuleConditionAndAction(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"from:contains:boss@example.com", `{"input":"boss@example.com","operator":1,"type":1}`},
		{"Subject:Starts_With:[周报]", `{"input":"[周报]","operator":3,"type":6}`},
		{"has_attachment", `{"type":16}`},
		{"to_or_cc:contains_self", `{"operator":7,"type":4}`},
		{"sender:eq:a@example.com", `{"input":"a@example.com","operator":5,"type":1}`},
	}
	for _, tc := range cases {
		got, err := parseMailRuleCondition(tc.in)
		if err != nil {
			t.Errorf("%s: %v", tc.in, err)
			continue
		}
		b, _ := json.Marshal(got)
		if string(b) != tc.want {
			t.Errorf("%s => %s, want %s", tc.in, b, tc.want)
		}
	}
	for _, bad := range []string{"unknown:contains:x", "from", "from:contains", "from:bogus:x", "has_attachment:contains:x", "to:contains_self:x"} {
		if _, err := parseMailRuleCondition(bad); err == nil {
			t.Errorf("条件 %q 应报错", bad)
		}
	}
	acts := map[string]string{
		"mark_read":                  `{"type":3}`,
		"flag":                       `{"type":9}`,
		"move_folder:folder_id=7001": `{"input":"7001","type":11}`,
		"move_folder:7002":           `{"input":"7002","type":11}`,
	}
	for in, want := range acts {
		got, err := parseMailRuleAction(in)
		if err != nil {
			t.Errorf("%s: %v", in, err)
			continue
		}
		b, _ := json.Marshal(got)
		if string(b) != want {
			t.Errorf("%s => %s, want %s", in, b, want)
		}
	}
	for _, bad := range []string{"forward", "move_folder", "mark_read:x=1", "move_folder:other=1"} {
		if _, err := parseMailRuleAction(bad); err == nil {
			t.Errorf("动作 %q 应报错", bad)
		}
	}
}

func TestParseMailRuleJSONItems(t *testing.T) {
	items, err := parseMailRuleJSONItems(`["from:contains:a@example.com", {"field":"subject","operator":"contains","value":"周报"}, {"type":13}]`, true)
	if err != nil || len(items) != 3 || items[1]["type"] != 6 || items[2]["type"] != 13 {
		t.Fatalf("items=%v err=%v", items, err)
	}
	acts, err := parseMailRuleJSONItems(`[{"kind":"move_folder","folder_id":"9"},{"type":3}]`, false)
	if err != nil || len(acts) != 2 || acts[0]["input"] != "9" {
		t.Fatalf("acts=%v err=%v", acts, err)
	}
}

func TestDescribeMailRule(t *testing.T) {
	rule := normalizeMailRuleBody(map[string]any{
		"name": "r", "is_enable": true, "ignore_the_rest_of_rules": true,
		"condition": map[string]any{"match_type": 2, "items": []any{map[string]any{"type": 1, "operator": 1, "input": "a@example.com"}, map[string]any{"type": 16}}},
		"action":    map[string]any{"items": []any{map[string]any{"type": 11, "input": "7001"}, map[string]any{"type": 99}}},
	})
	got := describeMailRule(rule)
	want := "满足任一条件：发件人 包含 a@example.com；带附件 → 移动到文件夹(7001)、未知动作(99)（命中后不再执行后续规则）"
	if got != want {
		t.Errorf("describe =\n%s\nwant\n%s", got, want)
	}
}

func TestMailRuleCreate_DryRunAndRealBody(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"rule":{"id":"r_new"}}}`)
	}))
	defer srv.Close()
	setupMailAttendanceCmdTestConfig(t, srv.URL)
	cmd := mailRuleCreateCmd
	resetCmdFlags(cmd)
	defer resetCmdFlags(cmd)
	_ = cmd.Flags().Set("user-access-token", "u-test")
	_ = cmd.Flags().Set("name", "老板邮件")
	_ = cmd.Flags().Set("condition", "from:contains:boss@example.com")
	_ = cmd.Flags().Set("action", "star")
	_ = cmd.Flags().Set("action", "move_folder:folder_id=7001")
	_ = cmd.Flags().Set("match", "any")
	_ = cmd.Flags().Set("dry-run", "true")

	var err error
	out := captureMailStdout(t, func() { err = cmd.RunE(cmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	if gotBody != nil {
		t.Fatal("dry-run 不应发请求")
	}
	if !strings.Contains(out, `"method": "POST"`) || !strings.Contains(out, `"match_type": 2`) || !strings.Contains(out, "添加旗标") {
		t.Errorf("dry-run 输出不符: %s", out)
	}

	_ = cmd.Flags().Set("dry-run", "false")
	captureMailStdout(t, func() { err = cmd.RunE(cmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(gotBody)
	for _, want := range []string{`"name":"老板邮件"`, `"is_enable":true`, `"ignore_the_rest_of_rules":false`, `{"input":"boss@example.com","operator":1,"type":1}`, `{"input":"7001","type":11}`, `{"type":9}`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("请求体缺少 %s: %s", want, b)
		}
	}
}

func mailRulesMockServer(t *testing.T, rules []map[string]any, captured *[]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		*captured = append(*captured, r.Method+" "+r.URL.Path+" "+string(raw))
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "msg": "ok", "data": map[string]any{"items": rules}})
			return
		}
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{}}`)
	}))
}

func sampleMailRules() []map[string]any {
	mk := func(id, name string) map[string]any {
		return map[string]any{"id": id, "name": name, "is_enable": true, "ignore_the_rest_of_rules": false,
			"condition": map[string]any{"match_type": 1, "items": []any{map[string]any{"type": 6, "operator": 1, "input": name}}},
			"action":    map[string]any{"items": []any{map[string]any{"type": 3}}}}
	}
	return []map[string]any{mk("r1", "一"), mk("r2", "二"), mk("r3", "三")}
}

// TestMailRuleUpdate_PreservesUnspecifiedFields 只改名称时，条件/动作/开关保持原值整体写回。
func TestMailRuleUpdate_PreservesUnspecifiedFields(t *testing.T) {
	var reqs []string
	srv := mailRulesMockServer(t, sampleMailRules(), &reqs)
	defer srv.Close()
	setupMailAttendanceCmdTestConfig(t, srv.URL)
	cmd := mailRuleUpdateCmd
	resetCmdFlags(cmd)
	defer resetCmdFlags(cmd)
	_ = cmd.Flags().Set("user-access-token", "u-test")
	_ = cmd.Flags().Set("rule-id", "r2")
	_ = cmd.Flags().Set("name", "改名")
	_ = cmd.Flags().Set("disable", "true")
	var err error
	captureMailStdout(t, func() { err = cmd.RunE(cmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) != 2 || !strings.HasPrefix(reqs[1], "PUT /open-apis/mail/v1/user_mailboxes/me/rules/r2 ") {
		t.Fatalf("请求序列不符: %v", reqs)
	}
	put := reqs[1]
	for _, want := range []string{`"name":"改名"`, `"is_enable":false`, `"input":"二"`, `"action":{"items":[{"type":3}]}`} {
		if !strings.Contains(put, want) {
			t.Errorf("PUT 体缺少 %s: %s", want, put)
		}
	}

	// 未指定任何修改 → 用法错误、不发请求
	reqs = nil
	resetCmdFlags(cmd)
	_ = cmd.Flags().Set("rule-id", "r2")
	if err := cmd.RunE(cmd, nil); err == nil || len(reqs) != 0 {
		t.Errorf("无修改项应报用法错误且不发请求: err=%v reqs=%v", err, reqs)
	}
}

func TestMailRuleToggleDeleteReorder(t *testing.T) {
	var reqs []string
	srv := mailRulesMockServer(t, sampleMailRules(), &reqs)
	defer srv.Close()
	setupMailAttendanceCmdTestConfig(t, srv.URL)

	// disable：只改 is_enable
	resetCmdFlags(mailRuleDisableCmd)
	_ = mailRuleDisableCmd.Flags().Set("user-access-token", "u-test")
	_ = mailRuleDisableCmd.Flags().Set("rule-id", "r1")
	var err error
	captureMailStdout(t, func() { err = mailRuleDisableCmd.RunE(mailRuleDisableCmd, nil) })
	if err != nil || len(reqs) != 2 || !strings.Contains(reqs[1], `"is_enable":false`) || !strings.Contains(reqs[1], `"input":"一"`) {
		t.Fatalf("rule-disable 不符: err=%v reqs=%v", err, reqs)
	}

	// delete：dry-run 不发请求；非交互未确认 → 需确认错误，不发 DELETE
	reqs = nil
	resetCmdFlags(mailRuleDeleteCmd)
	defer resetCmdFlags(mailRuleDeleteCmd)
	_ = mailRuleDeleteCmd.Flags().Set("user-access-token", "u-test")
	_ = mailRuleDeleteCmd.Flags().Set("rule-id", "r3")
	_ = mailRuleDeleteCmd.Flags().Set("dry-run", "true")
	out := captureMailStdout(t, func() { err = mailRuleDeleteCmd.RunE(mailRuleDeleteCmd, nil) })
	if err != nil || len(reqs) != 0 || !strings.Contains(out, `"DELETE"`) {
		t.Fatalf("rule-delete --dry-run 不符: err=%v reqs=%v out=%s", err, reqs, out)
	}
	_ = mailRuleDeleteCmd.Flags().Set("dry-run", "false")
	oldInteractive := confirmIsInteractive
	confirmIsInteractive = func() bool { return false }
	defer func() { confirmIsInteractive = oldInteractive }()
	err = mailRuleDeleteCmd.RunE(mailRuleDeleteCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "确认") {
		t.Fatalf("非交互未确认应拒绝: %v", err)
	}
	for _, r := range reqs {
		if strings.HasPrefix(r, "DELETE") {
			t.Fatal("未确认不应发 DELETE")
		}
	}

	// reorder：r3 移到最前
	reqs = nil
	resetCmdFlags(mailRuleReorderCmd)
	defer resetCmdFlags(mailRuleReorderCmd)
	_ = mailRuleReorderCmd.Flags().Set("user-access-token", "u-test")
	_ = mailRuleReorderCmd.Flags().Set("move-rule-id", "r3")
	_ = mailRuleReorderCmd.Flags().Set("to-top", "true")
	captureMailStdout(t, func() { err = mailRuleReorderCmd.RunE(mailRuleReorderCmd, nil) })
	if err != nil || len(reqs) != 2 || !strings.Contains(reqs[1], `{"rule_ids":["r3","r1","r2"]}`) {
		t.Fatalf("rule-reorder 不符: err=%v reqs=%v", err, reqs)
	}
	// 完整列表缺规则 → 报错
	resetCmdFlags(mailRuleReorderCmd)
	_ = mailRuleReorderCmd.Flags().Set("user-access-token", "u-test")
	_ = mailRuleReorderCmd.Flags().Set("rule-ids", "r1,r2")
	if err := mailRuleReorderCmd.RunE(mailRuleReorderCmd, nil); err == nil {
		t.Error("--rule-ids 缺少现有规则应报错")
	}
}

func TestMailThreadModifyAndTrash(t *testing.T) {
	var reqs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		reqs = append(reqs, r.URL.Path+" "+string(raw))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{}}`)
	}))
	defer srv.Close()
	setupMailAttendanceCmdTestConfig(t, srv.URL)

	ids := make([]string, 25)
	for i := range ids {
		ids[i] = fmt.Sprintf("t%02d", i)
	}
	cmd := mailThreadModifyCmd
	resetCmdFlags(cmd)
	defer resetCmdFlags(cmd)
	_ = cmd.Flags().Set("user-access-token", "u-test")
	_ = cmd.Flags().Set("thread-ids", strings.Join(ids, ","))
	_ = cmd.Flags().Set("add-label-ids", "important")
	_ = cmd.Flags().Set("folder-id", "archive")
	var err error
	captureMailStdout(t, func() { err = cmd.RunE(cmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) != 2 || !strings.HasPrefix(reqs[0], "/open-apis/mail/v1/user_mailboxes/me/threads/batch_modify ") ||
		!strings.Contains(reqs[0], `"add_label_ids":["IMPORTANT"]`) || !strings.Contains(reqs[0], `"add_folder":"ARCHIVED"`) ||
		!strings.Contains(reqs[1], `"thread_ids":["t20","t21","t22","t23","t24"]`) {
		t.Errorf("thread-modify 请求不符: %v", reqs)
	}

	// thread-trash：dry-run 不发请求；非交互未确认被拒
	reqs = nil
	tc := mailThreadTrashCmd
	resetCmdFlags(tc)
	defer resetCmdFlags(tc)
	_ = tc.Flags().Set("user-access-token", "u-test")
	_ = tc.Flags().Set("thread-ids", "t1,t2")
	_ = tc.Flags().Set("dry-run", "true")
	out := captureMailStdout(t, func() { err = tc.RunE(tc, nil) })
	if err != nil || len(reqs) != 0 || !strings.Contains(out, "threads/batch_trash") {
		t.Fatalf("thread-trash --dry-run 不符: err=%v reqs=%v", err, reqs)
	}
	_ = tc.Flags().Set("dry-run", "false")
	oldInteractive := confirmIsInteractive
	confirmIsInteractive = func() bool { return false }
	defer func() { confirmIsInteractive = oldInteractive }()
	if err := tc.RunE(tc, nil); err == nil || len(reqs) != 0 {
		t.Fatalf("非交互未确认应拒绝且不发请求: err=%v reqs=%v", err, reqs)
	}
}
