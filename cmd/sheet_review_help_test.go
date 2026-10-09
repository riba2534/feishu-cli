package cmd

import (
	"strings"
	"testing"
)

// TestSheetFilterTypeHelpListsMultiValue filter create 与 filter-view condition create/update 的
// --filter-type 说明列出 multiValue（代码本就原样透传）。
func TestSheetFilterTypeHelpListsMultiValue(t *testing.T) {
	for name, usage := range map[string]string{
		"sheet filter create":                      sheetFilterCreateCmd.Flags().Lookup("filter-type").Usage,
		"sheet filter-view condition create":       sheetFilterViewConditionCreateCmd.Flags().Lookup("filter-type").Usage,
		"sheet filter-view condition update":       sheetFilterViewConditionUpdateCmd.Flags().Lookup("filter-type").Usage,
		"sheet filter create（Long）":                sheetFilterCreateCmd.Long,
		"sheet filter-view condition create（Long）": sheetFilterViewConditionCreateCmd.Long,
	} {
		if !strings.Contains(usage, "multiValue") {
			t.Errorf("%s 的 --filter-type 说明应列出 multiValue: %q", name, usage)
		}
	}
}

// TestSheetDropdownSetSpreadsheetTokenAlias dropdown set 与 get/update/delete 一致接受 --spreadsheet-token。
func TestSheetDropdownSetSpreadsheetTokenAlias(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	m := newSheetMockServer(t)

	if _, err := runSheetCmdForTest(t, sheetDropdownSetCmd, nil, map[string]string{
		"spreadsheet-token": "shtFpTest", "range": "s1!A1:A3", "options": "待办,完成",
	}); err != nil {
		t.Fatalf("--spreadsheet-token 应可用: %v", err)
	}
	reqs := m.requests()
	if len(reqs) != 1 || reqs[0].Method != "POST" || reqs[0].Path != "/open-apis/sheets/v2/spreadsheets/shtFpTest/dataValidation" {
		t.Fatalf("请求 = %+v", reqs)
	}

	// 新旧名同时指定不同值：报错且不发请求
	if _, err := runSheetCmdForTest(t, sheetDropdownSetCmd, nil, map[string]string{
		"token": "shtA", "spreadsheet-token": "shtB", "range": "s1!A1:A3", "options": "x",
	}); err == nil || !strings.Contains(err.Error(), "--spreadsheet-token") {
		t.Fatalf("--token 与 --spreadsheet-token 不同值应报错，实际: %v", err)
	}
	if len(m.requests()) != 1 {
		t.Fatalf("冲突时不应发请求: %+v", m.requests())
	}
}
