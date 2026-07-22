package client

import (
	"encoding/json"
	"testing"
)

func textCell(s string) []*CellElement {
	return []*CellElement{{Type: "text", Text: &TextElement{Text: s}}}
}

func numCell(s string) []*CellElement {
	return []*CellElement{{Type: "value", Value: &ValueElement{Value: s}}}
}

func dateCell(s string) []*CellElement {
	return []*CellElement{{Type: "date_time", DateTime: &DateTimeElement{DateTime: s}}}
}

func TestNormalizeCellElements(t *testing.T) {
	cases := []struct {
		name  string
		elems []*CellElement
		kind  string
		want  string
	}{
		{"纯文本", textCell("hello"), "text", "hello"},
		{"数字", numCell("1200.50"), "number", "1200.50"},
		{"日期", dateCell("2026/01/15"), "date", "2026/01/15"},
		{"空文本", textCell(""), "empty", ""},
		{"nil 元素", nil, "empty", ""},
		{"公式数字", []*CellElement{{Type: "formula", Formula: &FormulaElement{Formula: "=SUM(A1)", FormulaValue: "42"}}}, "number", "42"},
		{"公式文本", []*CellElement{{Type: "formula", Formula: &FormulaElement{Formula: "=A1", FormulaValue: "abc"}}}, "text", "abc"},
		{"链接", []*CellElement{{Type: "link", Link: &LinkElement{Text: "官网", Link: "https://example.com"}}}, "text", "官网"},
	}
	for _, c := range cases {
		sc := normalizeCellElements(c.elems)
		if sc.kind != c.kind {
			t.Errorf("%s: kind = %s, want %s", c.name, sc.kind, c.kind)
			continue
		}
		got := sc.text
		if sc.kind == "number" {
			got = sc.num.String()
		}
		if c.kind != "empty" && got != c.want {
			t.Errorf("%s: value = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestInferColumnDtype(t *testing.T) {
	rows := [][]cellScalar{
		{{kind: "text", text: "A001"}, {kind: "number", num: "1200.5"}, {kind: "date", text: "2026/01/15"}, {kind: "text", text: "TRUE"}, {kind: "number", num: "1"}},
		{{kind: "text", text: "A002"}, {kind: "number", num: "980"}, {kind: "date", text: "2026/02/20"}, {kind: "text", text: "FALSE"}, {kind: "text", text: "x"}},
		{{kind: "empty"}, {kind: "empty"}, {kind: "empty"}, {kind: "empty"}, {kind: "empty"}},
	}
	wants := []string{"string", "float64", "datetime64[ns]", "bool", "object"}
	for j, want := range wants {
		if got := inferColumnDtype(rows, j); got != want {
			t.Errorf("col %d dtype = %s, want %s", j, got, want)
		}
	}
}

func TestScalarToJSONAndDateNormalize(t *testing.T) {
	if v := scalarToJSON(cellScalar{kind: "date", text: "2026/01/15"}, "datetime64[ns]"); v != "2026-01-15" {
		t.Errorf("日期归一 = %v, want 2026-01-15", v)
	}
	if v := scalarToJSON(cellScalar{kind: "text", text: "TRUE"}, "bool"); v != true {
		t.Errorf("bool 转换 = %v, want true", v)
	}
	if v := scalarToJSON(cellScalar{kind: "empty"}, "float64"); v != nil {
		t.Errorf("空值 = %v, want nil", v)
	}
	if v := scalarToJSON(cellScalar{kind: "number", num: json.Number("980")}, "float64"); v != json.Number("980") {
		t.Errorf("数字保精度 = %v", v)
	}
	if got := normalizeDateString("2026/01/15 10:30"); got != "2026-01-15T10:30" {
		t.Errorf("datetime 归一 = %s", got)
	}
}

func TestTrimEmptyEdgesAndColumnLetter(t *testing.T) {
	rows := [][]cellScalar{
		{{kind: "text", text: "a"}, {kind: "empty"}, {kind: "empty"}},
		{{kind: "text", text: "b"}, {kind: "number", num: "1"}, {kind: "empty"}},
		{{kind: "empty"}, {kind: "empty"}, {kind: "empty"}},
	}
	out := trimEmptyEdges(rows, 3)
	if len(out) != 2 || len(out[0]) != 2 {
		t.Errorf("trim 后应为 2 行 2 列，实际 %d 行 %d 列", len(out), len(out[0]))
	}
	for idx, want := range map[int]string{0: "A", 25: "Z", 26: "AA", 51: "AZ", 52: "BA"} {
		if got := ColumnIndexToLetter(idx); got != want {
			t.Errorf("ColumnIndexToLetter(%d) = %s, want %s", idx, got, want)
		}
	}
}

func TestDedupeColumnNames(t *testing.T) {
	got := dedupeColumnNames([]string{"备注", "分数", "备注", "备注", "备注_2"})
	want := []string{"备注", "分数", "备注_2", "备注_3", "备注_2_2"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dedupe[%d] = %s, want %s（完整: %v）", i, got[i], want[i], got)
		}
	}
	// 去重后必须全局唯一（round-trip 回 table-put 的硬前提）
	seen := map[string]bool{}
	for _, n := range got {
		if seen[n] {
			t.Fatalf("去重后仍有重复列名: %s", n)
		}
		seen[n] = true
	}
}
