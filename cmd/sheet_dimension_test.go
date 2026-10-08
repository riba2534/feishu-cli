package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

// dimBody 取请求体中的 dimension 对象。
func dimBody(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	d, _ := decodeBody(t, raw)["dimension"].(map[string]any)
	if d == nil {
		t.Fatalf("请求体缺少 dimension: %s", raw)
	}
	return d
}

func assertDim(t *testing.T, d map[string]any, major string, start, end int) {
	t.Helper()
	if d["majorDimension"] != major || d["startIndex"] != float64(start) || d["endIndex"] != float64(end) {
		t.Errorf("dimension = %v, want %s %d..%d", d, major, start, end)
	}
}

// TestSheetDeleteRows_IndexConversion delete-rows 的 --start（0 起始）/--end（不含）须换算成接口的 1 起始、两端包含。
// 旧实现原样透传：--start 2 实际删除第 2、3 两行（接口实测 startIndex=2,endIndex=3 删 2 行）。
func TestSheetDeleteRows_IndexConversion(t *testing.T) {
	cases := []struct {
		name       string
		flags      map[string]string
		start, end int
		outContain string
	}{
		{"只传 --start 删除 1 行", map[string]string{"start": "2"}, 3, 3, "第 3 行"},
		{"--start/--end 0 起始不含 end", map[string]string{"start": "2", "end": "5"}, 3, 5, "第 3 到 5 行"},
		{"--start 0 删除第 1 行", map[string]string{"start": "0"}, 1, 1, "第 1 行"},
		{"--range 1 起始两端包含", map[string]string{"range": "3:5"}, 3, 5, "共 3 行"},
		{"--range 可带同一子表前缀", map[string]string{"range": "s1!7"}, 7, 7, "第 7 行"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newSheetMockServer(t)
			out, err := runSheetCmdForTest(t, sheetDeleteRowsCmd, []string{"shtTok", "s1"}, c.flags)
			if err != nil {
				t.Fatal(err)
			}
			reqs := m.requests()
			if len(reqs) != 1 || reqs[0].Method != http.MethodDelete || !strings.HasSuffix(reqs[0].Path, "/dimension_range") {
				t.Fatalf("请求 = %+v", reqs)
			}
			assertDim(t, dimBody(t, reqs[0].Body), "ROWS", c.start, c.end)
			if !strings.Contains(out, c.outContain) {
				t.Errorf("输出 %q 不含 %q", out, c.outContain)
			}
		})
	}
}

// TestSheetDeleteCols_IndexConversion delete-cols 同样换算口径，--range 用列字母。
func TestSheetDeleteCols_IndexConversion(t *testing.T) {
	cases := []struct {
		flags      map[string]string
		start, end int
	}{
		{map[string]string{"start": "1", "end": "4"}, 2, 4},
		{map[string]string{"start": "0"}, 1, 1},
		{map[string]string{"range": "B:D"}, 2, 4},
		{map[string]string{"range": "aa"}, 27, 27},
	}
	for _, c := range cases {
		m := newSheetMockServer(t)
		out, err := runSheetCmdForTest(t, sheetDeleteColsCmd, []string{"shtTok", "s1"}, c.flags)
		if err != nil {
			t.Fatal(err)
		}
		reqs := m.requests()
		if len(reqs) != 1 {
			t.Fatalf("请求数 = %d", len(reqs))
		}
		assertDim(t, dimBody(t, reqs[0].Body), "COLUMNS", c.start, c.end)
		if !strings.Contains(out, "列") {
			t.Errorf("输出 = %q", out)
		}
	}
}

// TestSheetDeleteDimension_ValidationAndDryRun 参数校验在联网前完成；--dry-run 不发请求并展示真实删除范围。
func TestSheetDeleteDimension_ValidationAndDryRun(t *testing.T) {
	m := newSheetMockServer(t)
	bad := []map[string]string{
		{},                             // 未指定范围
		{"range": "3:5", "start": "1"}, // 两种写法混用
		{"range": "B:D"},               // delete-rows 传列区间
		{"start": "-1"},                // 负数
		{"start": "5", "end": "3"},     // end <= start
		{"range": "other!3:5"},         // 前缀与 sheet_id 不一致
		{"range": "0:2"},               // 行号从 1 开始
	}
	for _, flags := range bad {
		_, err := runSheetCmdForTest(t, sheetDeleteRowsCmd, []string{"shtTok", "s1"}, flags)
		if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
			t.Errorf("flags %v 应返回用法错误，得到 %v", flags, err)
		}
		resetSheetCmdFlags(sheetDeleteRowsCmd)
	}

	out, err := runSheetCmdForTest(t, sheetDeleteRowsCmd, []string{"https://xxx.feishu.cn/sheets/shtTok", "s1"}, map[string]string{"start": "2", "end": "5", "dry-run": "true"})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.requests()) != 0 {
		t.Fatalf("参数错误或 dry-run 不应发出请求: %+v", m.requests())
	}
	var plan map[string]any
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("dry-run 输出不是 JSON: %v\n%s", err, out)
	}
	if plan["dry_run"] != true || plan["range"] != "3:5" || plan["count"] != float64(3) || plan["spreadsheet_token"] != "shtTok" {
		t.Errorf("dry-run 计划 = %v", plan)
	}
	step := plan["api"].([]any)[0].(map[string]any)
	assertDim(t, step["body"].(map[string]any)["dimension"].(map[string]any), "ROWS", 3, 5)

	out, err = runSheetCmdForTest(t, sheetDeleteSheetCmd, []string{"shtTok", "s2"}, map[string]string{"dry-run": "true"})
	if err != nil || !strings.Contains(out, `"deleteSheet"`) || len(m.requests()) != 0 {
		t.Errorf("delete-sheet --dry-run 输出 = %s, err=%v, reqs=%d", out, err, len(m.requests()))
	}
}

// TestSheetInsertDimension_Conversion 插入接口为 0 起始、不含 end：--range "3:4"（新行位置）→ startIndex 2, endIndex 4。
func TestSheetInsertDimension_Conversion(t *testing.T) {
	cases := []struct {
		flags      map[string]string
		major      string
		start, end int
	}{
		{map[string]string{"range": "3:4"}, "ROWS", 2, 4},
		{map[string]string{"start": "2", "end": "4"}, "ROWS", 2, 4},
		{map[string]string{"start": "0"}, "ROWS", 0, 1},
	}
	for _, c := range cases {
		m := newSheetMockServer(t)
		if _, err := runSheetCmdForTest(t, sheetInsertRowsCmd, []string{"shtTok", "s1"}, c.flags); err != nil {
			t.Fatal(err)
		}
		reqs := m.requests()
		if len(reqs) != 1 || !strings.HasSuffix(reqs[0].Path, "/insert_dimension_range") {
			t.Fatalf("请求 = %+v", reqs)
		}
		body := decodeBody(t, reqs[0].Body)
		assertDim(t, body["dimension"].(map[string]any), c.major, c.start, c.end)
	}
}

// TestSheetProtect_IndexConversion protect 示例「--start 0 --end 5 保护前 5 行」换算为接口的 1..5；
// 旧实现原样透传 0..5（实测 startIndex=0 报 90202）。
func TestSheetProtect_IndexConversion(t *testing.T) {
	cases := []struct {
		flags      map[string]string
		major      string
		start, end int
		query      string
	}{
		{map[string]string{"dimension": "ROWS", "start": "0", "end": "5"}, "ROWS", 1, 5, ""},
		{map[string]string{"dimension": "COLUMNS", "end": "3"}, "COLUMNS", 1, 3, ""},
		{map[string]string{"range": "A:C", "users": "ou_a, ou_b"}, "COLUMNS", 1, 3, "user_id_type=open_id"},
		{map[string]string{"range": "2:4", "users": "on_a", "user-id-type": "union_id"}, "ROWS", 2, 4, "user_id_type=union_id"},
	}
	for _, c := range cases {
		m := newSheetMockServer(t)
		m.respond = func(w http.ResponseWriter, r *http.Request, body []byte) bool {
			_, _ = w.Write([]byte(`{"code":0,"msg":"ok","data":{"addProtectedDimension":[{"protectId":"p1"}]}}`))
			return true
		}
		if _, err := runSheetCmdForTest(t, sheetProtectCmd, []string{"shtTok", "s1"}, c.flags); err != nil {
			t.Fatal(err)
		}
		reqs := m.requests()
		if len(reqs) != 1 {
			t.Fatalf("请求数 = %d", len(reqs))
		}
		if reqs[0].Query != c.query {
			t.Errorf("query = %q, want %q", reqs[0].Query, c.query)
		}
		item := decodeBody(t, reqs[0].Body)["addProtectedDimension"].([]any)[0].(map[string]any)
		assertDim(t, item["dimension"].(map[string]any), c.major, c.start, c.end)
		if c.flags["users"] != "" {
			if users, _ := item["users"].([]any); len(users) == 0 {
				t.Errorf("users 未发送: %v", item)
			}
		}
		if _, has := item["editors"]; has {
			t.Errorf("不应发送 editors: %v", item)
		}
	}

	for _, flags := range []map[string]string{
		{},                             // 未指定范围
		{"range": "1:2", "start": "0"}, // 混用
		{"range": "1:2", "dimension": "COLUMNS"},
		{"start": "3", "end": "3"}, // end 不大于 start
		{"range": "A:B", "users": "ou_a", "user-id-type": "user_id"},
	} {
		newSheetMockServer(t)
		if _, err := runSheetCmdForTest(t, sheetProtectCmd, []string{"shtTok", "s1"}, flags); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
			t.Errorf("flags %v 应返回用法错误，得到 %v", flags, err)
		}
		resetSheetCmdFlags(sheetProtectCmd)
	}
}
