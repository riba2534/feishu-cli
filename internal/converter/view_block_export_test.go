package converter

import (
	"strings"
	"testing"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
)

// TestViewBlockIsTransparentContainer 视图块（type=33，附件外层容器）只展开子块：
// 不再输出"不支持的块类型"注释，附件也不会在顶层重复导出。
func TestViewBlockIsTransparentContainer(t *testing.T) {
	blocks := []*larkdocx.Block{
		{BlockId: strPtr("doc"), BlockType: intPtr(int(BlockTypePage)), Page: &larkdocx.Text{}, Children: []string{"p1", "view1"}},
		{BlockId: strPtr("p1"), BlockType: intPtr(int(BlockTypeText)), Text: &larkdocx.Text{Elements: []*larkdocx.TextElement{{TextRun: &larkdocx.TextRun{Content: strPtr("正文")}}}}},
		{BlockId: strPtr("view1"), BlockType: intPtr(int(BlockTypeView)), View: &larkdocx.View{}, Children: []string{"file1"}},
		{BlockId: strPtr("file1"), BlockType: intPtr(int(BlockTypeFile)), File: &larkdocx.File{Token: strPtr("fileTok"), Name: strPtr("a.txt")}},
	}
	got, err := NewBlockToMarkdown(blocks, ConvertOptions{}).Convert()
	if err != nil {
		t.Fatalf("Convert 失败: %v", err)
	}
	if strings.Contains(got, "不支持的块类型") {
		t.Fatalf("视图块不应输出不支持注释:\n%s", got)
	}
	if n := strings.Count(got, `<file token="fileTok"`); n != 1 {
		t.Fatalf("附件应恰好导出 1 次，实际 %d 次:\n%s", n, got)
	}
}
