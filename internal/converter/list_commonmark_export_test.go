package converter

import (
	"strings"
	"testing"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
)

func listText(content string) *larkdocx.Text {
	return &larkdocx.Text{Elements: []*larkdocx.TextElement{{
		TextRun: &larkdocx.TextRun{Content: strPtr(content)},
	}}}
}

func TestListExportUsesCumulativeCommonMarkMarkerWidth(t *testing.T) {
	done := false
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("ordered-10"),
			BlockType: intPtr(int(BlockTypeOrdered)),
			Ordered: &larkdocx.Text{
				Elements: listText("ordered 10").Elements,
				Style:    &larkdocx.TextStyle{Sequence: strPtr("10")},
			},
			Children: []string{"bullet"},
		},
		{
			BlockId:   strPtr("bullet"),
			BlockType: intPtr(int(BlockTypeBullet)),
			Bullet:    listText("bullet"),
			Children:  []string{"todo"},
		},
		{
			BlockId:   strPtr("todo"),
			BlockType: intPtr(int(BlockTypeTodo)),
			Todo: &larkdocx.Text{
				Elements: listText("todo").Elements,
				Style:    &larkdocx.TextStyle{Done: &done},
			},
			Children: []string{"ordered-1"},
		},
		{
			BlockId:   strPtr("ordered-1"),
			BlockType: intPtr(int(BlockTypeOrdered)),
			Ordered: &larkdocx.Text{
				Elements: listText("ordered 1").Elements,
				Style:    &larkdocx.TextStyle{Sequence: strPtr("1")},
			},
			Children: []string{"leaf"},
		},
		{
			BlockId:   strPtr("leaf"),
			BlockType: intPtr(int(BlockTypeBullet)),
			Bullet:    listText("leaf"),
		},
	}

	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	markdown, err := conv.Convert()
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	want := strings.Join([]string{
		"10. ordered 10",
		"    - bullet",
		"      - [ ] todo",
		"        1. ordered 1",
		"           - leaf",
		"",
	}, "\n")
	if markdown != want {
		t.Fatalf("CommonMark 缩进异常:\n--- got ---\n%s--- want ---\n%s", markdown, want)
	}

	// 用项目自身的 Goldmark/GFM 导入器重新解析，验证层级没有被扁平化。
	importer := NewMarkdownToBlock([]byte(markdown), ConvertOptions{}, "")
	result, err := importer.ConvertWithTableData()
	if err != nil {
		t.Fatalf("重新解析导出 Markdown 失败: %v", err)
	}
	wantTypes := []BlockType{BlockTypeOrdered, BlockTypeBullet, BlockTypeTodo, BlockTypeOrdered, BlockTypeBullet}
	nodes := result.BlockNodes
	for depth, wantType := range wantTypes {
		if len(nodes) != 1 || nodes[0] == nil || nodes[0].Block == nil || nodes[0].Block.BlockType == nil {
			t.Fatalf("depth=%d 的列表层级异常: %#v", depth, nodes)
		}
		if got := BlockType(*nodes[0].Block.BlockType); got != wantType {
			t.Fatalf("depth=%d block type=%s，期望 %s", depth, BlockTypeName(got), BlockTypeName(wantType))
		}
		nodes = nodes[0].Children
	}
	if len(nodes) != 0 {
		t.Fatalf("叶子列表不应再有子节点: %#v", nodes)
	}
}

func TestListExportMixedMarkerWidths(t *testing.T) {
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("bullet-root"),
			BlockType: intPtr(int(BlockTypeBullet)),
			Bullet:    listText("root"),
			Children:  []string{"ordered-12"},
		},
		{
			BlockId:   strPtr("ordered-12"),
			BlockType: intPtr(int(BlockTypeOrdered)),
			Ordered: &larkdocx.Text{
				Elements: listText("child").Elements,
				Style:    &larkdocx.TextStyle{Sequence: strPtr("12")},
			},
			Children: []string{"leaf"},
		},
		{
			BlockId:   strPtr("leaf"),
			BlockType: intPtr(int(BlockTypeBullet)),
			Bullet:    listText("leaf"),
		},
	}
	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	markdown, err := conv.Convert()
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	want := "- root\n  12. child\n      - leaf\n"
	if markdown != want {
		t.Fatalf("混合列表 marker 累计宽度异常:\n--- got ---\n%s--- want ---\n%s", markdown, want)
	}
}
