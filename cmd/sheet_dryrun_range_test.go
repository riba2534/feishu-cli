package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/client"
)

// TestPreviewSheetRange_MatchesQualifyRange 离线预览可确定（pending=false）时，结果必须与执行时 qualifyRange 一致；
// 需要子表列表才能确定的情况必须标记 pending，不能静默展示与执行不同的范围。
func TestPreviewSheetRange_MatchesQualifyRange(t *testing.T) {
	two := []*client.SheetInfo{{SheetID: "s1", Title: "Sheet1"}, {SheetID: "s2", Title: "数据"}}
	cases := []struct {
		name                       string
		rng, sheetID, sheetNm, url string
		want                       string
		wantPending                bool
	}{
		{name: "前缀等于 --sheet-id", rng: "s2!A1:B2", sheetID: "s2", want: "s2!A1:B2"},
		{name: "无前缀 + --sheet-id", rng: "A1:B2", sheetID: "s2", want: "s2!A1:B2"},
		{name: "无前缀 + URL ?sheet=", rng: "A1:B2", url: "s2", want: "s2!A1:B2"},
		{name: "--sheet-id 优先于 URL", rng: "A1:B2", sheetID: "s1", url: "s2", want: "s1!A1:B2"},
		{name: "zsh 转义 + --sheet-id 前缀", rng: `s1\!A1`, sheetID: "s1", want: "s1!A1"},
		{name: "整个子表（已知 sheetId）", rng: "s1", sheetID: "s1", want: "s1"},
		{name: "子表名前缀", rng: "数据!A1", want: "数据!A1", wantPending: true},
		{name: "sheetId 前缀但未给 --sheet-id", rng: "s2!A1", want: "s2!A1", wantPending: true},
		{name: "无前缀 + --sheet-name", rng: "A1:B2", sheetNm: "数据", want: "A1:B2", wantPending: true},
		{name: "--sheet-name 优先于 URL", rng: "A1:B2", sheetNm: "数据", url: "s1", want: "A1:B2", wantPending: true},
		{name: "无前缀且无任何子表信息", rng: "A1:B2", want: "A1:B2", wantPending: true},
		{name: "整个子表（子表名）", rng: "数据", want: "数据", wantPending: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, pending, err := previewSheetRange(c.rng, c.sheetID, c.sheetNm, c.url)
			if err != nil || got != c.want || pending != c.wantPending {
				t.Fatalf("previewSheetRange(%q) = (%q, %v, %v), want (%q, %v)", c.rng, got, pending, err, c.want, c.wantPending)
			}
			if pending {
				return
			}
			stubSheetList(t, two, nil)
			target := &sheetTarget{Token: "shtTok", URLSheetID: c.url}
			real, err := target.qualifyRange(c.rng, c.sheetID, c.sheetNm)
			if err != nil || real != got {
				t.Fatalf("预览 %q 与执行 qualifyRange %q (%v) 不一致", got, real, err)
			}
		})
	}
	for _, bad := range []string{"", "  ", "!A1", "s1!"} {
		if _, _, err := previewSheetRange(bad, "", "", ""); err == nil || exitCodeFor(err) != 2 {
			t.Errorf("previewSheetRange(%q) 应为用法错误，得到 %v", bad, err)
		}
	}
}

// dryRunRequestBody 解析 dry-run JSON 计划中第一个请求步骤。
func dryRunFirstStep(t *testing.T, out string) map[string]any {
	t.Helper()
	var plan struct {
		API []map[string]any `json:"api"`
	}
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("dry-run 输出不是 JSON: %v\n%s", err, out)
	}
	if len(plan.API) == 0 {
		t.Fatalf("dry-run 计划缺少请求步骤:\n%s", out)
	}
	return plan.API[0]
}

// TestSheetWrite_DryRunRangeMatchesRealRequest 预览中的范围与真实请求一致；执行时才能确定的前缀在 desc 中注明。
func TestSheetWrite_DryRunRangeMatchesRealRequest(t *testing.T) {
	cases := []struct {
		name        string
		token       string
		flags       map[string]string
		wantRange   string
		wantPending bool
	}{
		{"--sheet-id", "shtTok", map[string]string{"sheet-id": "s2"}, "s2!A1:B1", false},
		{"URL ?sheet=", "https://example.feishu.cn/sheets/shtTok?sheet=s2", nil, "s2!A1:B1", false},
		{"--sheet-name", "shtTok", map[string]string{"sheet-name": "数据"}, "A1:B1", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newSheetMockServer(t)
			flags := map[string]string{"data": `[["a","b"]]`, "output": "json", "dry-run": "true"}
			for k, v := range c.flags {
				flags[k] = v
			}
			out, err := runSheetCmdForTest(t, sheetWriteCmd, []string{c.token, "A1:B1"}, flags)
			if err != nil {
				t.Fatal(err)
			}
			if len(m.requests()) != 0 {
				t.Fatalf("dry-run 不应发请求: %+v", m.requests())
			}
			step := dryRunFirstStep(t, out)
			body, _ := json.Marshal(step["body"])
			if !strings.Contains(string(body), fmt.Sprintf(`"range":%q`, c.wantRange)) {
				t.Errorf("预览请求体范围应为 %q: %s", c.wantRange, body)
			}
			desc, _ := step["desc"].(string)
			if c.wantPending != (desc == sheetRangePendingDesc) {
				t.Errorf("desc = %q, wantPending=%v", desc, c.wantPending)
			}

			// 真实执行（非 dry-run）发送的范围：可离线确定的情况必须与预览完全一致
			if c.wantPending {
				return
			}
			delete(flags, "dry-run")
			if _, err := runSheetCmdForTest(t, sheetWriteCmd, []string{c.token, "A1:B1"}, flags); err != nil {
				t.Fatal(err)
			}
			reqs := m.requests()
			if len(reqs) != 1 || !strings.Contains(string(reqs[0].Body), fmt.Sprintf(`"range":%q`, c.wantRange)) {
				t.Fatalf("真实请求范围应为 %q: %+v", c.wantRange, reqs)
			}
		})
	}
}

// TestSheetWrite_RangeSmallerThanData_UsageBeforeNetwork 显式范围小于数据（未超分批上限）时本地报用法错误，
// --dry-run 与真实执行一致，且不发任何请求（含子表列表）。服务端对此返回 90202（行、列均实测）。
func TestSheetWrite_RangeSmallerThanData_UsageBeforeNetwork(t *testing.T) {
	cases := []struct {
		name, rng, data, want string
	}{
		{"列不足", "s1!H1:I1", `[["a","b","c"]]`, "只有 2 列，小于数据的 3 列"},
		{"行不足", "s1!H1:I1", `[["a","b"],["c","d"]]`, "只有 1 行，小于数据的 2 行"},
		{"子表名前缀也校验", "Sheet1!A1:A1", `[["a","b"]]`, "只有 1 列"},
	}
	for _, c := range cases {
		for _, dry := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/dry=%v", c.name, dry), func(t *testing.T) {
				m := newSheetMockServer(t)
				calls := stubSheetList(t, []*client.SheetInfo{{SheetID: "s1", Title: "Sheet1"}}, nil)
				flags := map[string]string{"data": c.data}
				if dry {
					flags["dry-run"] = "true"
				}
				_, err := runSheetCmdForTest(t, sheetWriteCmd, []string{"shtTok", c.rng}, flags)
				if err == nil || exitCodeFor(err) != 2 || !strings.Contains(err.Error(), c.want) {
					t.Fatalf("err = %v (exit %d)，want exit 2 含 %q", err, exitCodeFor(err), c.want)
				}
				if len(m.requests()) != 0 || *calls != 0 {
					t.Errorf("应在首个网络请求前报错: reqs=%+v list=%d", m.requests(), *calls)
				}
			})
		}
	}

	// 范围足够、整列开放端、单个锚点单元格不受影响
	for _, ok := range []struct{ rng, data string }{
		{"s1!H1:J2", `[["a","b"]]`},
		{"s1!H:I", `[["a","b"],["c","d"]]`},
		{"s1!H1", `[["a","b","c"],["d","e","f"]]`},
	} {
		newSheetMockServer(t)
		if _, err := runSheetCmdForTest(t, sheetWriteCmd, []string{"shtTok", ok.rng}, map[string]string{"data": ok.data, "dry-run": "true"}); err != nil {
			t.Errorf("范围 %s 不应报错: %v", ok.rng, err)
		}
	}
}

// TestSheetDropdownDelete_MaxRanges 超过 100 个范围在本地报用法错误（服务端实测 90204 max size limit = 100）。
func TestSheetDropdownDelete_MaxRanges(t *testing.T) {
	build := func(n int) string {
		parts := make([]string, n)
		for i := range parts {
			parts[i] = fmt.Sprintf("s1!A%d:A%d", i+1, i+1)
		}
		return strings.Join(parts, ",")
	}
	for _, dry := range []bool{true, false} {
		m := newSheetMockServer(t)
		flags := map[string]string{"token": "shtTok", "ranges": build(101)}
		if dry {
			flags["dry-run"] = "true"
		}
		_, err := runSheetCmdForTest(t, sheetDropdownDeleteCmd, nil, flags)
		if err == nil || exitCodeFor(err) != 2 || !strings.Contains(err.Error(), "--ranges 最多 100 个范围，当前 101 个") {
			t.Fatalf("dry=%v err = %v (exit %d)", dry, err, exitCodeFor(err))
		}
		if len(m.requests()) != 0 {
			t.Errorf("dry=%v 参数错误不应发请求: %+v", dry, m.requests())
		}
	}
	newSheetMockServer(t)
	out, err := runSheetCmdForTest(t, sheetDropdownDeleteCmd, nil, map[string]string{"token": "shtTok", "ranges": build(100), "dry-run": "true"})
	if err != nil || !strings.Contains(out, `"s1!A100:A100"`) {
		t.Fatalf("100 个范围应允许: %v\n%s", err, out)
	}
}

// TestSheetDropdownDelete_DryRunRangeMatchesRealRequest URL ?sheet= 补前缀在预览中体现，与真实请求一致；
// 依赖唯一子表补全的情况在 desc 中注明。
func TestSheetDropdownDelete_DryRunRangeMatchesRealRequest(t *testing.T) {
	m := newSheetMockServer(t)
	flags := map[string]string{"token": "https://example.feishu.cn/sheets/shtTok?sheet=s2", "ranges": "A1:A9", "dry-run": "true"}
	out, err := runSheetCmdForTest(t, sheetDropdownDeleteCmd, nil, flags)
	if err != nil {
		t.Fatal(err)
	}
	step := dryRunFirstStep(t, out)
	body, _ := json.Marshal(step["body"])
	if !strings.Contains(string(body), `"range":"s2!A1:A9"`) || step["desc"] != nil {
		t.Fatalf("预览应补好 URL 子表前缀且无 pending 说明: %s desc=%v", body, step["desc"])
	}
	delete(flags, "dry-run")
	if _, err := runSheetCmdForTest(t, sheetDropdownDeleteCmd, nil, flags); err != nil {
		t.Fatal(err)
	}
	if reqs := m.requests(); len(reqs) != 1 || !strings.Contains(string(reqs[0].Body), `"range":"s2!A1:A9"`) {
		t.Fatalf("真实请求范围应为 s2!A1:A9: %+v", reqs)
	}

	newSheetMockServer(t)
	out, err = runSheetCmdForTest(t, sheetDropdownDeleteCmd, nil, map[string]string{"token": "shtTok", "ranges": "A1:A9", "dry-run": "true"})
	if err != nil {
		t.Fatal(err)
	}
	if step := dryRunFirstStep(t, out); step["desc"] != sheetRangePendingDesc {
		t.Errorf("唯一子表补全应在 desc 注明执行时确定: %v", step["desc"])
	}
}

// TestSheetDangerHelpMentionsGate cond-format update 的 help 写明确认门禁；board delete 的 help 写明无门禁、先 --dry-run。
func TestSheetDangerHelpMentionsGate(t *testing.T) {
	for _, w := range []string{"确认门禁", "--yes", "exit 10"} {
		if !strings.Contains(sheetCondFormatUpdateCmd.Long, w) {
			t.Errorf("cond-format update --help 缺少 %q", w)
		}
	}
	for _, w := range []string{"没有确认门禁", "--dry-run"} {
		if !strings.Contains(boardDeleteCmd.Long, w) {
			t.Errorf("board delete --help 缺少 %q", w)
		}
	}
}
