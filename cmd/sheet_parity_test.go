package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestSheetWriteCommands_DryRun 删除 / 覆盖类 sheet 写命令的 --dry-run 只打印计划，不发任何请求（含子表列表）。
func TestSheetWriteCommands_DryRun(t *testing.T) {
	cases := []struct {
		name  string
		cmd   *cobra.Command
		args  []string
		flags map[string]string
		want  []string
	}{
		{"write", sheetWriteCmd, []string{"shtTok", "s1!A1"}, map[string]string{"data": `[["a",1]]`, "output": "json"},
			[]string{`"PUT"`, "/spreadsheets/shtTok/values", `"rows": 1`}},
		{"clear", sheetClearCmd, []string{"shtTok", "s1", "A1:B2"}, nil,
			[]string{"/sheets/s1/values/batch_clear", `"A1:B2"`}},
		{"unprotect", sheetUnprotectCmd, []string{"shtTok", "p1", "p2"}, nil,
			[]string{"protected_range_batch_del", `"protectIds"`}},
		{"filter delete", sheetFilterDeleteCmd, []string{"shtTok", "s1"}, nil,
			[]string{`"DELETE"`, "/sheets/s1/filter"}},
		{"image delete", sheetImageDeleteCmd, []string{"shtTok", "s1", "fi1"}, nil,
			[]string{"/sheets/s1/float_images/fi1"}},
		{"dropdown delete", sheetDropdownDeleteCmd, nil, map[string]string{"token": "shtTok", "ranges": "s1!A1:A9,s1!B1:B9"},
			[]string{"/dataValidation", `"dataValidationRanges"`, `"s1!B1:B9"`}},
		{"filter-view delete", sheetFilterViewDeleteCmd, nil, map[string]string{"token": "shtTok", "sheet-id": "s1", "filter-view-id": "fv12345678"},
			[]string{"/sheets/s1/filter_views/fv12345678"}},
		{"filter-view condition delete", sheetFilterViewConditionDeleteCmd, nil, map[string]string{"token": "shtTok", "sheet-id": "s1", "filter-view-id": "fv12345678", "condition-id": "E"},
			[]string{"/filter_views/fv12345678/conditions/E"}},
		{"table-put", sheetTablePutCmd, []string{"shtTok", "s1"}, map[string]string{"sheets": `{"sheets":[{"columns":["x"],"data":[[1],[2]],"dtypes":{"x":"int64"}}]}`, "output": "json"},
			[]string{`"range": "s1!A1:A3"`, "/sheets/s1/values/batch_update", `"data_rows": 2`}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newSheetMockServer(t)
			flags := map[string]string{"dry-run": "true"}
			for k, v := range c.flags {
				flags[k] = v
			}
			out, err := runSheetCmdForTest(t, c.cmd, c.args, flags)
			if err != nil {
				t.Fatal(err)
			}
			if len(m.requests()) != 0 {
				t.Fatalf("dry-run 不应发请求: %+v", m.requests())
			}
			if !strings.Contains(out, `"dry_run": true`) {
				t.Errorf("应输出 dry-run 计划: %s", out)
			}
			for _, w := range c.want {
				if !strings.Contains(out, w) {
					t.Errorf("dry-run 输出缺少 %s:\n%s", w, out)
				}
			}
		})
	}
}

// TestSheetParamErrors_ExitUsage sheet 参数错误为用法错误（exit 2），文本不变，且不发请求。
func TestSheetParamErrors_ExitUsage(t *testing.T) {
	cases := []struct {
		name  string
		cmd   *cobra.Command
		args  []string
		flags map[string]string
		want  string
	}{
		{"dropdown set 缺 range", sheetDropdownSetCmd, nil, map[string]string{"token": "shtTok"}, "--token/--spreadsheet-token、--range 均为必填项"},
		{"dropdown update 选项冲突", sheetDropdownUpdateCmd, nil, map[string]string{"token": "shtTok", "sheet-id": "s1", "ranges": "A1", "options": "a", "options-json": `["b"]`}, "不能同时使用"},
		{"filter-view create 缺参", sheetFilterViewCreateCmd, nil, map[string]string{"token": "shtTok"}, "--token、--sheet-id、--range 均为必填项"},
		{"filter-view update 无字段", sheetFilterViewUpdateCmd, nil, map[string]string{"token": "shtTok", "sheet-id": "s1", "filter-view-id": "fv12345678"}, "至少需要指定一个"},
		{"condition create expected 非法", sheetFilterViewConditionCreateCmd, nil, map[string]string{"token": "shtTok", "sheet-id": "s1", "filter-view-id": "fv12345678", "condition-id": "E", "expected": "x"}, "--expected 必须是字符串数组"},
		{"batch-set-style data 非法", sheetBatchSetStyleCmd, []string{"shtTok"}, map[string]string{"data": "{"}, "--data 必须是"},
		{"image update 无字段", sheetImageUpdateCmd, []string{"shtTok", "s1", "fi1"}, nil, "至少需要指定一个待更新字段"},
		{"image write-batch 缺 manifest", sheetImageWriteBatchCmd, []string{"shtTok", "s1"}, nil, "--manifest 为必填项"},
		{"export 格式非法", sheetExportCmd, []string{"shtTok"}, map[string]string{"format": "pdf"}, "不支持的导出格式"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newSheetMockServer(t)
			_, err := runSheetCmdForTest(t, c.cmd, c.args, c.flags)
			if err == nil || exitCodeFor(err) != 2 || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v (exit %d)，want exit 2 含 %q", err, exitCodeFor(err), c.want)
			}
			if len(m.requests()) != 0 {
				t.Errorf("参数错误不应发请求: %+v", m.requests())
			}
		})
	}
}

// TestSheetImportMD_LocalErrorsBeforeValidate 本地表格内容错误先于配置校验报出（无凭据时也是 exit 2）。
func TestSheetImportMD_LocalErrorsBeforeValidate(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "t.md")
	if err := os.WriteFile(md, []byte("| a | b |\n|---|---|\n| 1 | 2 |\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	validated := false
	deps := defaultSheetImportMDDeps()
	deps.validate = func() error { validated = true; return errors.New("缺少 app_id") }
	resetSheetCmdFlags(sheetImportMDCmd)
	t.Cleanup(func() { resetSheetCmdFlags(sheetImportMDCmd) })
	_ = sheetImportMDCmd.Flags().Set("table-index", "3")
	err := runSheetImportMD(sheetImportMDCmd, []string{md}, deps)
	if err == nil || exitCodeFor(err) != 2 || !strings.Contains(err.Error(), "--table-index 3 超出范围") {
		t.Fatalf("err = %v (exit %d)", err, exitCodeFor(err))
	}
	if validated {
		t.Error("本地参数错误应在 config.Validate 之前返回")
	}
}
