package output

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func mustRender(t *testing.T, o *Options, data any) string {
	t.Helper()
	s, err := RenderString(o, data)
	if err != nil {
		t.Fatalf("RenderString error: %v", err)
	}
	return s
}

func TestRenderJSON(t *testing.T) {
	data := map[string]any{"name": "测试", "n": 3}
	got := mustRender(t, &Options{Format: FormatJSON}, data)
	if !strings.Contains(got, `"name": "测试"`) {
		t.Errorf("json 输出缺 name 字段: %q", got)
	}
	// 中文不应被转义成 \uXXXX
	if strings.Contains(got, `\u`) {
		t.Errorf("中文被 HTML/unicode 转义: %q", got)
	}
}

func TestRenderNDJSONArrayExpansion(t *testing.T) {
	data := []any{
		map[string]any{"id": "a"},
		map[string]any{"id": "b"},
	}
	got := mustRender(t, &Options{Format: FormatNDJSON}, data)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("ndjson 应按元素拆 2 行，得 %d: %q", len(lines), got)
	}
	if !strings.Contains(lines[0], `"id":"a"`) || !strings.Contains(lines[1], `"id":"b"`) {
		t.Errorf("ndjson 行内容不对: %q", got)
	}
}

func TestRenderTableHeterogeneousAndCJK(t *testing.T) {
	data := []any{
		map[string]any{"名称": "群A", "id": "oc_1"},
		map[string]any{"id": "oc_2", "extra": "x"}, // 缺 名称、多 extra
	}
	got := mustRender(t, &Options{Format: FormatTable}, data)
	// 列并集应含三列
	for _, col := range []string{"名称", "id", "extra"} {
		if !strings.Contains(got, col) {
			t.Errorf("table 缺列 %q: %q", col, got)
		}
	}
	if !strings.Contains(got, "群A") || !strings.Contains(got, "oc_2") {
		t.Errorf("table 缺单元格内容: %q", got)
	}
	// 含分隔线
	if !strings.Contains(got, "---") {
		t.Errorf("table 缺分隔线: %q", got)
	}
}

func TestRenderCSV(t *testing.T) {
	data := []any{
		map[string]any{"id": "a", "v": 1},
		map[string]any{"id": "b", "v": 2},
	}
	got := mustRender(t, &Options{Format: FormatCSV}, data)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 3 { // header + 2 行
		t.Fatalf("csv 应 3 行，得 %d: %q", len(lines), got)
	}
	if lines[0] != "id,v" {
		t.Errorf("csv 表头不对: %q", lines[0])
	}
}

func TestRenderJQFilter(t *testing.T) {
	data := map[string]any{
		"items": []any{
			map[string]any{"id": "x", "active": true},
			map[string]any{"id": "y", "active": false},
		},
	}
	// 取 active==true 的 id
	got := mustRender(t, &Options{Format: FormatJSON, JQ: `.items[] | select(.active) | .id`}, data)
	if !strings.Contains(got, `"x"`) {
		t.Errorf("jq 应输出 x: %q", got)
	}
	if strings.Contains(got, `"y"`) {
		t.Errorf("jq 不应输出 y（active=false）: %q", got)
	}
}

func TestRenderJQMultiOutputNDJSON(t *testing.T) {
	data := map[string]any{"items": []any{
		map[string]any{"id": "a"}, map[string]any{"id": "b"},
	}}
	got := mustRender(t, &Options{Format: FormatNDJSON, JQ: `.items[]`}, data)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("jq 多输出 ndjson 应 2 行，得 %d: %q", len(lines), got)
	}
}

func TestRenderJQToTable(t *testing.T) {
	data := map[string]any{"items": []any{
		map[string]any{"id": "a", "n": 1},
		map[string]any{"id": "b", "n": 2},
	}}
	got := mustRender(t, &Options{Format: FormatTable, JQ: `.items`}, data)
	if !strings.Contains(got, "id") || !strings.Contains(got, "a") || !strings.Contains(got, "b") {
		t.Errorf("jq→table 内容不对: %q", got)
	}
}

func TestRenderBigIntPrecision(t *testing.T) {
	// 大整数（消息 ID 类）不应被 float 科学计数法破坏
	data := map[string]any{"id": int64(7030776512726958083)}
	got := mustRender(t, &Options{Format: FormatJSON}, data)
	if !strings.Contains(got, "7030776512726958083") {
		t.Errorf("大整数精度丢失: %q", got)
	}
}

func TestRenderBigIntThroughJQ(t *testing.T) {
	// 飞书 19 位 message_id 经 --jq 路径必须保精度（回归：旧 toJQInput 会降级 float64 截断）
	data := map[string]any{"data": map[string]any{"items": []any{
		map[string]any{"message_id": int64(7030776512726958083)},
	}}}
	got := mustRender(t, &Options{Format: FormatJSON, JQ: `.data.items[].message_id`}, data)
	if !strings.Contains(got, "7030776512726958083") {
		t.Errorf("jq 路径大整数精度丢失: %q", got)
	}
	// 不应出现科学计数法或被截断的 ...080
	if strings.Contains(got, "e+18") || strings.Contains(got, "7030776512726958080") {
		t.Errorf("jq 路径大整数被降级: %q", got)
	}
}

func TestRenderBigIntJQToTableCSV(t *testing.T) {
	data := []any{map[string]any{"id": int64(7030776512726958083), "name": "x"}}
	for _, f := range []string{FormatTable, FormatCSV} {
		got := mustRender(t, &Options{Format: f, JQ: `.`}, data)
		if !strings.Contains(got, "7030776512726958083") {
			t.Errorf("format=%s jq 路径大整数丢精度: %q", f, got)
		}
	}
}

func TestParseOptionsDefaults(t *testing.T) {
	cmd := &cobra.Command{Use: "x", Run: func(*cobra.Command, []string) {}}
	AddOutputFlags(cmd)
	AddPaginationFlags(cmd)
	AddDryRunFlag(cmd)
	o, err := ParseOptions(cmd)
	if err != nil {
		t.Fatalf("ParseOptions error: %v", err)
	}
	if o.Format != FormatJSON {
		t.Errorf("默认 format 应 json，得 %q", o.Format)
	}
	if o.DryRun || o.PageAll {
		t.Errorf("默认 dry-run/page-all 应 false")
	}
}

func TestParseOptionsInvalidFormat(t *testing.T) {
	cmd := &cobra.Command{Use: "x", Run: func(*cobra.Command, []string) {}}
	AddOutputFlags(cmd)
	_ = cmd.Flags().Set("format", "yaml")
	if _, err := ParseOptions(cmd); err == nil {
		t.Errorf("非法 format 应报错")
	}
}

func TestParseOptionsPagination(t *testing.T) {
	cmd := &cobra.Command{Use: "x", Run: func(*cobra.Command, []string) {}}
	AddPaginationFlags(cmd)
	_ = cmd.Flags().Set("page-all", "true")
	_ = cmd.Flags().Set("page-size", "50")
	_ = cmd.Flags().Set("page-limit", "5")
	o, err := ParseOptions(cmd)
	if err != nil {
		t.Fatalf("ParseOptions error: %v", err)
	}
	if !o.PageAll || o.PageSize != 50 || o.PageLimit != 5 {
		t.Errorf("分页解析不对: %+v", o)
	}
}

func TestAddOutputFlagsNoCollisionWithExistingOutput(t *testing.T) {
	cmd := &cobra.Command{Use: "x", Run: func(*cobra.Command, []string) {}}
	cmd.Flags().StringP("output", "o", "", "preexisting")
	// 不应 panic / 重复注册
	AddOutputFlags(cmd)
	if cmd.Flags().Lookup("format") == nil {
		t.Errorf("format 仍应被注册")
	}
}
