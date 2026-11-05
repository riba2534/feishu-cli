package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

// runCondFormatCmd 与 runSheetCmdForTest 相同，但 flag 按顺序设置（--formula/--range 可重复）。
func runCondFormatCmd(t *testing.T, c *cobra.Command, args []string, flags [][2]string) (string, error) {
	t.Helper()
	resetCondFormatFlags(c)
	t.Cleanup(func() { resetCondFormatFlags(c) })
	for _, kv := range flags {
		if err := c.Flags().Set(kv[0], kv[1]); err != nil {
			t.Fatalf("设置 --%s=%s 失败: %v", kv[0], kv[1], err)
		}
	}
	var runErr error
	out := captureStdoutConcurrent(t, func() { runErr = c.RunE(c, args) })
	return out, runErr
}

// resetCondFormatFlags 复位 flag；StringArray 的 DefValue 是 "[]"，Set 会追加，需用 Replace 清空。
func resetCondFormatFlags(c *cobra.Command) {
	resetFlagValues(c.Flags())
}

func condFormatOf(t *testing.T, body []byte) (string, map[string]any) {
	t.Helper()
	items, _ := decodeBody(t, body)["sheet_condition_formats"].([]any)
	if len(items) != 1 {
		t.Fatalf("sheet_condition_formats = %s", body)
	}
	item := items[0].(map[string]any)
	return item["sheet_id"].(string), item["condition_format"].(map[string]any)
}

// TestSheetCondFormatCreate_Body create 请求体对齐公开文档：子表名换算为 sheetId，范围补前缀，文本比较值加引号。
func TestSheetCondFormatCreate_Body(t *testing.T) {
	m := newSheetMockServer(t)
	m.respond = func(w http.ResponseWriter, r *http.Request, body []byte) bool {
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"responses":[{"sheet_id":"s2","cf_id":"cf1","res_code":0,"res_msg":"success"}]}}`)
		return true
	}
	out, err := runCondFormatCmd(t, sheetCondFormatCreateCmd, []string{"shtTok", "数据"}, [][2]string{
		{"range", "C2:C10,数据!A2:B"}, {"rule-type", "cellis"}, {"operator", "between"},
		{"formula", "1"}, {"formula", "abc"}, {"bold", "true"}, {"back-color", "#D9F5D6"},
	})
	if err != nil {
		t.Fatal(err)
	}
	reqs := m.requests()
	if len(reqs) != 1 || reqs[0].Method != http.MethodPost || !strings.HasSuffix(reqs[0].Path, "/condition_formats/batch_create") {
		t.Fatalf("请求 = %+v", reqs)
	}
	sheetID, cf := condFormatOf(t, reqs[0].Body)
	if sheetID != "s2" || cf["rule_type"] != "cellIs" {
		t.Errorf("sheet_id/rule_type = %s/%v", sheetID, cf["rule_type"])
	}
	ranges, _ := json.Marshal(cf["ranges"])
	if string(ranges) != `["s2!C2:C10","s2!A2:B"]` {
		t.Errorf("ranges = %s", ranges)
	}
	attrs, _ := json.Marshal(cf["attrs"])
	if string(attrs) != `[{"formula":["1","\"abc\""],"operator":"between"}]` {
		t.Errorf("attrs = %s", attrs)
	}
	style, _ := json.Marshal(cf["style"])
	if string(style) != `{"back_color":"#d9f5d6","font":{"bold":true}}` {
		t.Errorf("style = %s", style)
	}
	if !strings.Contains(out, "cf1") {
		t.Errorf("输出缺少 cf_id: %q", out)
	}
}

// TestSheetCondFormatCreate_ResCodeFails 批量接口顶层 code=0 但逐条 res_code≠0 时必须报错。
func TestSheetCondFormatCreate_ResCodeFails(t *testing.T) {
	m := newSheetMockServer(t)
	m.respond = func(w http.ResponseWriter, r *http.Request, body []byte) bool {
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"responses":[{"sheet_id":"s1","cf_id":"","res_code":90232,"res_msg":"style is empty"}]}}`)
		return true
	}
	_, err := runCondFormatCmd(t, sheetCondFormatCreateCmd, []string{"shtTok", "s1"}, [][2]string{
		{"range", "A1:A3"}, {"rule-type", "uniqueValues"}, {"italic", "true"},
	})
	if err == nil || !strings.Contains(err.Error(), "90232") {
		t.Fatalf("应报告逐条失败，得到 %v", err)
	}
}

// TestSheetCondFormatUpdate_DryRunOffline update --dry-run 不联网，cf_id 写入请求体。
func TestSheetCondFormatUpdate_DryRunOffline(t *testing.T) {
	m := newSheetMockServer(t)
	out, err := runCondFormatCmd(t, sheetCondFormatUpdateCmd, []string{"shtTok", "s1", "cfX"}, [][2]string{
		{"range", "A:A"}, {"rule-type", "containsText"}, {"text", "紧急"}, {"operator", "beginswith"},
		{"fore-color", "#f54a45"}, {"dry-run", "true"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.requests()) != 0 {
		t.Fatalf("dry-run 不应发请求: %+v", m.requests())
	}
	for _, want := range []string{`batch_update"`, `"cf_id": "cfX"`, `"s1!A:A"`, `"operator": "beginsWith"`, `"text": "紧急"`} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run 输出缺少 %s:\n%s", want, out)
		}
	}
}

// TestSheetCondFormat_UsageErrors 参数错误一律 exit 2（clierr.Usage），且在联网之前拦截。
func TestSheetCondFormat_UsageErrors(t *testing.T) {
	cases := []struct {
		name  string
		flags [][2]string
		want  string
	}{
		{"缺 --range", [][2]string{{"rule-type", "duplicateValues"}, {"bold", "true"}}, "--range"},
		{"缺 --rule-type", [][2]string{{"range", "A1"}, {"bold", "true"}}, "--rule-type"},
		{"未知规则", [][2]string{{"range", "A1"}, {"rule-type", "dataBar"}, {"bold", "true"}}, "不支持"},
		{"between 只给 1 个值", [][2]string{{"range", "A1"}, {"rule-type", "cellIs"}, {"operator", "between"}, {"formula", "1"}, {"bold", "true"}}, "2 个 --formula"},
		{"cellIs 缺 operator", [][2]string{{"range", "A1"}, {"rule-type", "cellIs"}, {"formula", "1"}, {"bold", "true"}}, "--operator"},
		{"containsText 缺 text", [][2]string{{"range", "A1"}, {"rule-type", "containsText"}, {"bold", "true"}}, "--text"},
		{"无参数规则带 --text", [][2]string{{"range", "A1"}, {"rule-type", "duplicateValues"}, {"text", "x"}, {"bold", "true"}}, "不使用 --text"},
		{"last7Days 被拒", [][2]string{{"range", "A1"}, {"rule-type", "timePeriod"}, {"time-period", "last7Days"}, {"bold", "true"}}, "last7Days"},
		{"缺样式", [][2]string{{"range", "A1"}, {"rule-type", "uniqueValues"}}, "样式"},
		{"颜色非法", [][2]string{{"range", "A1"}, {"rule-type", "uniqueValues"}, {"back-color", "red"}}, "#RRGGBB"},
		{"文字装饰越界", [][2]string{{"range", "A1"}, {"rule-type", "uniqueValues"}, {"text-decoration", "4"}}, "--text-decoration"},
		{"范围非法", [][2]string{{"range", "A1:B2:C3"}, {"rule-type", "uniqueValues"}, {"bold", "true"}}, "A1 写法"},
		{"--data 与规则 flag 互斥", [][2]string{{"data", `{"ranges":["A1"],"rule_type":"uniqueValues","style":{"back_color":"#ffffff"}}`}, {"bold", "true"}}, "不能同时使用"},
		{"--data 缺 style", [][2]string{{"data", `{"ranges":["A1"],"rule_type":"uniqueValues"}`}}, "缺少 style"},
		{"--data 未知字段", [][2]string{{"data", `{"ranges":["A1"],"rule_type":"uniqueValues","colour":1}`}}, "--data 必须是"},
		{"非法输出格式", [][2]string{{"output", "yaml"}}, "输出格式"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newSheetMockServer(t)
			_, err := runCondFormatCmd(t, sheetCondFormatCreateCmd, []string{"shtTok", "s1"}, c.flags)
			if err == nil || exitCodeFor(err) != clierr.ExitUsage || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v (exit %d)，want 用法错误含 %q", err, exitCodeFor(err), c.want)
			}
			if len(m.requests()) != 0 {
				t.Errorf("参数错误不应发请求: %+v", m.requests())
			}
		})
	}
}

// TestSheetCondFormatCreate_RangeOtherSheet 范围前缀指向另一个子表时报用法错误、不发写请求。
func TestSheetCondFormatCreate_RangeOtherSheet(t *testing.T) {
	m := newSheetMockServer(t)
	_, err := runCondFormatCmd(t, sheetCondFormatCreateCmd, []string{"shtTok", "s1"}, [][2]string{
		{"range", "数据!A1:A3"}, {"rule-type", "uniqueValues"}, {"bold", "true"},
	})
	if err == nil || exitCodeFor(err) != clierr.ExitUsage || !strings.Contains(err.Error(), "同一子表") {
		t.Fatalf("err = %v", err)
	}
	if len(m.requests()) != 0 {
		t.Errorf("不应发写请求: %+v", m.requests())
	}
}

// TestSheetCondFormatDelete_ConfirmGate 非交互且无 --yes 时 exit 10、不发删除请求；--yes 后发送 batch_delete。
func TestSheetCondFormatDelete_ConfirmGate(t *testing.T) {
	origInteractive := confirmIsInteractive
	confirmIsInteractive = func() bool { return false }
	t.Cleanup(func() { confirmIsInteractive = origInteractive })
	origYes := assumeYes
	assumeYes = false
	t.Cleanup(func() { assumeYes = origYes })

	m := newSheetMockServer(t)
	_, err := runCondFormatCmd(t, sheetCondFormatDeleteCmd, []string{"shtTok", "Sheet1", "cf1", "cf2", "cf1"}, nil)
	if err == nil || exitCodeFor(err) != clierr.ExitConfirmationRequired {
		t.Fatalf("应 exit 10，得到 %v (exit %d)", err, exitCodeFor(err))
	}
	if len(m.requests()) != 0 {
		t.Fatalf("未确认不应发请求: %+v", m.requests())
	}

	assumeYes = true
	m.respond = func(w http.ResponseWriter, r *http.Request, body []byte) bool {
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"responses":[{"sheet_id":"s1","cf_id":"cf1","res_code":0},{"sheet_id":"s1","cf_id":"cf2","res_code":0}]}}`)
		return true
	}
	if _, err := runCondFormatCmd(t, sheetCondFormatDeleteCmd, []string{"shtTok", "Sheet1", "cf1", "cf2", "cf1"}, nil); err != nil {
		t.Fatal(err)
	}
	reqs := m.requests()
	if len(reqs) != 1 || reqs[0].Method != http.MethodDelete || !strings.HasSuffix(reqs[0].Path, "/condition_formats/batch_delete") {
		t.Fatalf("请求 = %+v", reqs)
	}
	ids, _ := json.Marshal(decodeBody(t, reqs[0].Body)["sheet_cf_ids"])
	if string(ids) != `[{"cf_id":"cf1","sheet_id":"s1"},{"cf_id":"cf2","sheet_id":"s1"}]` {
		t.Errorf("sheet_cf_ids = %s（应去重并换算子表名）", ids)
	}
}

// TestSheetCondFormatDelete_DryRunAndLimit dry-run 不联网；超过 10 条为用法错误。
func TestSheetCondFormatDelete_DryRunAndLimit(t *testing.T) {
	m := newSheetMockServer(t)
	out, err := runCondFormatCmd(t, sheetCondFormatDeleteCmd, []string{"shtTok", "s1", "cf1"}, [][2]string{{"dry-run", "true"}})
	if err != nil || !strings.Contains(out, "batch_delete") || len(m.requests()) != 0 {
		t.Fatalf("dry-run: err=%v reqs=%d out=%s", err, len(m.requests()), out)
	}
	args := []string{"shtTok", "s1"}
	for i := 0; i < 11; i++ {
		args = append(args, "cf"+string(rune('a'+i)))
	}
	_, err = runCondFormatCmd(t, sheetCondFormatDeleteCmd, args, nil)
	if exitCodeFor(err) != clierr.ExitUsage {
		t.Fatalf("超过 10 条应 exit 2，得到 %v", err)
	}
}

// TestSheetCondFormatList_AllSheets 不指定子表时按全部子表查询，sheet_ids 逗号拼接。
func TestSheetCondFormatList_AllSheets(t *testing.T) {
	m := newSheetMockServer(t)
	m.respond = func(w http.ResponseWriter, r *http.Request, body []byte) bool {
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"sheet_condition_formats":[{"sheet_id":"s1","condition_format":{"cf_id":"cf1","ranges":["s1!A1:A3"],"rule_type":"uniqueValues","style":{"back_color":"#ffffff"}}}]}}`)
		return true
	}
	out, err := runCondFormatCmd(t, sheetCondFormatListCmd, []string{"shtTok"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	reqs := m.requests()
	if len(reqs) != 1 || reqs[0].Method != http.MethodGet || reqs[0].Query != "sheet_ids=s1%2Cs2" {
		t.Fatalf("请求 = %+v", reqs)
	}
	if !strings.Contains(out, "cf_id=cf1") || !strings.Contains(out, "规则=uniqueValues") {
		t.Errorf("输出 = %q", out)
	}
}

// TestSheetCondFormatUpdate_ConfirmGate 更新为整条覆盖：非交互且无 --yes 时 exit 10、不发写请求。
func TestSheetCondFormatUpdate_ConfirmGate(t *testing.T) {
	origInteractive := confirmIsInteractive
	confirmIsInteractive = func() bool { return false }
	t.Cleanup(func() { confirmIsInteractive = origInteractive })
	origYes := assumeYes
	assumeYes = false
	t.Cleanup(func() { assumeYes = origYes })

	m := newSheetMockServer(t)
	_, err := runCondFormatCmd(t, sheetCondFormatUpdateCmd, []string{"shtTok", "s1", "cf1"}, [][2]string{
		{"range", "A1:A3"}, {"rule-type", "uniqueValues"}, {"bold", "true"},
	})
	if err == nil || exitCodeFor(err) != clierr.ExitConfirmationRequired {
		t.Fatalf("应 exit 10，得到 %v", err)
	}
	for _, r := range m.requests() {
		if strings.Contains(r.Path, "batch_update") {
			t.Fatalf("未确认不应发 batch_update: %+v", r)
		}
	}
}
