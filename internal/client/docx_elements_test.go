package client

import (
	"testing"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
)

// TestBuildElementsJSONKeepsEquation 表格单元格经 update_text_elements 填充时，行内公式不能被丢弃
// （此前只序列化 text_run，单元格里的 $...$ 无法导入为公式）。
func TestBuildElementsJSONKeepsEquation(t *testing.T) {
	text, formula := "前缀 ", "x^2"
	got := buildElementsJSON([]*larkdocx.TextElement{
		{TextRun: &larkdocx.TextRun{Content: &text}},
		{Equation: &larkdocx.Equation{Content: &formula}},
	})
	if len(got) != 2 {
		t.Fatalf("expected 2 elements, got %#v", got)
	}
	eq, ok := got[1]["equation"].(map[string]any)
	if !ok || eq["content"] != "x^2" {
		t.Fatalf("equation element lost: %#v", got[1])
	}
}
