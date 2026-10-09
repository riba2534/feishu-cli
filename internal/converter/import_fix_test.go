package converter

import (
	"strings"
	"testing"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
)

func convertForTest(t *testing.T, md string, opts ConvertOptions) *ConvertResult {
	t.Helper()
	result, err := NewMarkdownToBlock([]byte(md), opts, "").ConvertWithTableData()
	if err != nil {
		t.Fatalf("ConvertWithTableData() error = %v", err)
	}
	return result
}

func textOf(elements []*larkdocx.TextElement) string {
	var sb strings.Builder
	for _, e := range elements {
		if e == nil {
			continue
		}
		if e.TextRun != nil && e.TextRun.Content != nil {
			sb.WriteString(*e.TextRun.Content)
		}
		if e.Equation != nil && e.Equation.Content != nil {
			sb.WriteString("$" + *e.Equation.Content + "$")
		}
	}
	return sb.String()
}

func blockText(b *larkdocx.Block) string {
	switch {
	case b == nil:
		return ""
	case b.Text != nil:
		return textOf(b.Text.Elements)
	case b.Bullet != nil:
		return textOf(b.Bullet.Elements)
	case b.Ordered != nil:
		return textOf(b.Ordered.Elements)
	case b.Todo != nil:
		return textOf(b.Todo.Elements)
	case b.Code != nil:
		return textOf(b.Code.Elements)
	}
	return ""
}

// ===== 缺陷 1：带 token 的 <image> / feishu://media/ 不能原样建块 =====

// TestImportImageTokenBuildsEmptyBlockWithReuseRef 带 token 的图片：建块请求不能带 token（服务端 1770001），
// 转换器建空 Image 块并登记素材复用 MediaRef（保留原显示宽高/对齐）；关闭上传时降级为占位文本。
func TestImportImageTokenBuildsEmptyBlockWithReuseRef(t *testing.T) {
	for _, md := range []string{
		`<image token="imgTok" width="120" height="80" align="center"/>` + "\n",
		"![图](feishu://media/imgTok)\n",
	} {
		result := convertForTest(t, md, ConvertOptions{UploadImages: true})
		if len(result.BlockNodes) != 1 {
			t.Fatalf("%q: expected 1 block, got %d", md, len(result.BlockNodes))
		}
		block := result.BlockNodes[0].Block
		if block.Image == nil {
			t.Fatalf("%q: expected Image block, got %#v", md, block)
		}
		if block.Image.Token != nil {
			t.Fatalf("%q: 建块请求不能带图片 token（1770001），got %q", md, *block.Image.Token)
		}
		ref := result.MediaRefs[block]
		if ref == nil || ref.Kind != MediaKindImage || ref.Token != "imgTok" || ref.UploadSource() != "feishu://media/imgTok" {
			t.Fatalf("%q: expected image reuse MediaRef, got %#v", md, ref)
		}
		if result.ImageStats.Total != 1 || result.ImageStats.Skipped != 0 {
			t.Fatalf("%q: ImageStats = %#v", md, result.ImageStats)
		}
	}

	tagged := convertForTest(t, `<image token="imgTok" width="120" height="80" align="center"/>`+"\n", ConvertOptions{UploadImages: true})
	if ref := tagged.MediaRefs[tagged.BlockNodes[0].Block]; ref.Width != 120 || ref.Height != 80 || ref.Align != 2 {
		t.Fatalf("应保留原显示宽高与对齐: %#v", ref)
	}

	off := convertForTest(t, `<image token="imgTok"/>`+"\n", ConvertOptions{})
	if b := off.BlockNodes[0].Block; b.Text == nil || !strings.Contains(blockText(b), "feishu://media/imgTok") || off.ImageStats.Skipped != 1 {
		t.Fatalf("关闭上传时应降级为占位文本: %#v stats=%#v", b, off.ImageStats)
	}
}

// TestCellInlineImageTokenCollectedForEmbedding doc export 在表格单元格里输出行内 <image token/>，
// 导入时应收集为 feishu://media/<token> 交给单元格嵌入（素材复用），而不是被静默丢弃。
func TestCellInlineImageTokenCollectedForEmbedding(t *testing.T) {
	md := "| 图 | 说明 |\n| --- | --- |\n| <image token=\"cellTok\" width=\"60\" height=\"60\"/> | 文字 |\n"
	result := convertForTest(t, md, ConvertOptions{UploadImages: true, EmbedTableImages: true})
	if len(result.TableDatas) != 1 {
		t.Fatalf("expected 1 table, got %d", len(result.TableDatas))
	}
	td := result.TableDatas[0]
	if len(td.CellImages) != 4 || len(td.CellImages[2]) != 1 || td.CellImages[2][0] != "feishu://media/cellTok" {
		t.Fatalf("单元格 token 图片应进入 CellImages: %#v", td.CellImages)
	}

	plain := convertForTest(t, md, ConvertOptions{})
	if got := textOf(plain.TableDatas[0].CellElements[2]); !strings.Contains(got, "feishu://media/cellTok") {
		t.Fatalf("不嵌入时应降级为占位文本而不是丢弃: %q", got)
	}
}

// ===== 缺陷 2：嵌套引用 =====

// TestNestedBlockquoteFlattened 飞书不允许 QuoteContainer/Callout 作为 QuoteContainer 的子块（1770030），
// 嵌套引用要扁平化进外层引用，内容不能丢。
func TestNestedBlockquoteFlattened(t *testing.T) {
	md := "> 外层第一行\n> > 内层内容\n>\n> 外层第二段\n\n> 外层\n> > [!NOTE]\n> > 内层提示\n"
	result := convertForTest(t, md, ConvertOptions{})
	if len(result.BlockNodes) != 2 {
		t.Fatalf("expected 2 quotes, got %d", len(result.BlockNodes))
	}
	var texts []string
	for _, quote := range result.BlockNodes {
		if quote.Block.QuoteContainer == nil {
			t.Fatalf("expected QuoteContainer, got %#v", quote.Block)
		}
		for _, child := range quote.Children {
			bt := BlockType(*child.Block.BlockType)
			if bt == BlockTypeQuoteContainer || bt == BlockTypeCallout {
				t.Fatalf("QuoteContainer 下不能再有 %s（服务端 1770030）", BlockTypeName(bt))
			}
			texts = append(texts, blockText(child.Block))
		}
	}
	joined := strings.Join(texts, "|")
	for _, want := range []string{"外层第一行", "内层内容", "外层第二段", "外层", "内层提示"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("扁平化后缺少 %q: %s", want, joined)
		}
	}
}

// ===== 缺陷 3：分栏 =====

// TestGridMultilineAndColumnCount 列内含空行/多个块的 <grid>（doc export 的输出形态）要整体解析，
// 列数按 <column> 实际数量（2-5）建，不能把列内容泄漏到分栏外或丢弃多出的列。
func TestGridMultilineAndColumnCount(t *testing.T) {
	md := "<grid cols=\"2\">\n<column>\n左栏段落\n\n- 左栏列表\n</column>\n<column>\n\n</column>\n<column>\n第三列\n</column>\n</grid>\n\n尾段\n"
	result := convertForTest(t, md, ConvertOptions{})
	if len(result.BlockNodes) != 2 {
		t.Fatalf("expected grid + 尾段, got %d nodes", len(result.BlockNodes))
	}
	grid := result.BlockNodes[0]
	if grid.Block.Grid == nil || *grid.Block.Grid.ColumnSize != 3 || len(grid.Children) != 3 {
		t.Fatalf("expected 3-column grid, got %#v children=%d", grid.Block.Grid, len(grid.Children))
	}
	left := grid.Children[0].Children
	if len(left) != 2 || blockText(left[0].Block) != "左栏段落" || left[1].Block.Bullet == nil {
		t.Fatalf("左栏应含段落+列表: %#v", left)
	}
	if got := blockText(grid.Children[2].Children[0].Block); got != "第三列" {
		t.Fatalf("第三列内容 = %q", got)
	}
	if blockText(result.BlockNodes[1].Block) != "尾段" {
		t.Fatalf("分栏后的内容应保持在分栏外: %#v", result.BlockNodes[1].Block)
	}

	// 6 列超过服务端上限 5：多出的列并入最后一列；cols="1" 低于下限 2
	six := "<grid>\n<column>\n1\n</column>\n<column>\n2\n</column>\n<column>\n3\n</column>\n<column>\n4\n</column>\n<column>\n5\n</column>\n<column>\n6\n</column>\n</grid>\n"
	g := convertForTest(t, six, ConvertOptions{}).BlockNodes[0]
	if *g.Block.Grid.ColumnSize != 5 || len(g.Children) != 5 || len(g.Children[4].Children) != 2 {
		t.Fatalf("6 列应收敛为 5 列且第 6 列并入最后一列: size=%d cols=%d", *g.Block.Grid.ColumnSize, len(g.Children))
	}
	one := convertForTest(t, "<grid cols=\"1\">\n<column>\nA\n</column>\n</grid>\n", ConvertOptions{}).BlockNodes[0]
	if *one.Block.Grid.ColumnSize != 2 {
		t.Fatalf("cols=1 应提升到服务端下限 2, got %d", *one.Block.Grid.ColumnSize)
	}
}

// TestGridNestedTableDataRegistered 分栏列里的表格也要能被导入层找到填充数据（TableDataByBlock）。
func TestGridNestedTableDataRegistered(t *testing.T) {
	md := "<grid cols=\"2\">\n<column>\n| a | b |\n| --- | --- |\n| 1 | 2 |\n</column>\n<column>\n右\n</column>\n</grid>\n"
	result := convertForTest(t, md, ConvertOptions{})
	table := result.BlockNodes[0].Children[0].Children[0]
	if table.Block.Table == nil {
		t.Fatalf("expected table in first column, got %#v", table.Block)
	}
	if td := result.TableDataByBlock[table.Block]; td == nil || td.Cols != 2 {
		t.Fatalf("分栏内表格缺少填充数据: %#v", td)
	}
}

// ===== 缺陷 6：列表项多段落 =====

// TestListItemMultiParagraphKeepsSeparateChildren 列表项的第二段起要作为该项的子块保留，不能并进首段。
func TestListItemMultiParagraphKeepsSeparateChildren(t *testing.T) {
	md := "- 首段\n\n  第二段\n\n  第三段\n- 下一项\n\n1. 有序首段\n\n   有序第二段\n   1. 嵌套\n\n- [x] 待办首段\n\n  待办第二段\n"
	result := convertForTest(t, md, ConvertOptions{})
	if len(result.BlockNodes) != 4 {
		t.Fatalf("expected 4 top-level items, got %d", len(result.BlockNodes))
	}
	bullet := result.BlockNodes[0]
	if got := blockText(bullet.Block); got != "首段" {
		t.Fatalf("列表项正文只应是首段, got %q", got)
	}
	if len(bullet.Children) != 2 || blockText(bullet.Children[0].Block) != "第二段" || blockText(bullet.Children[1].Block) != "第三段" {
		t.Fatalf("后续段落应为独立子块: %#v", bullet.Children)
	}
	ordered := result.BlockNodes[2]
	if blockText(ordered.Block) != "有序首段" || len(ordered.Children) != 2 ||
		blockText(ordered.Children[0].Block) != "有序第二段" || ordered.Children[1].Block.Ordered == nil {
		t.Fatalf("有序列表项结构异常: %q %#v", blockText(ordered.Block), ordered.Children)
	}
	todo := result.BlockNodes[3]
	if todo.Block.Todo == nil || blockText(todo.Block) != "待办首段" || len(todo.Children) != 1 || blockText(todo.Children[0].Block) != "待办第二段" {
		t.Fatalf("待办项结构异常: %q %#v", blockText(todo.Block), todo.Children)
	}
}

// TestListChildBlocksExportIndentedAndRoundtrip 列表项的非列表子块（段落、代码块）导出时缩进到正文列并空行分隔，
// 再导入后仍是同一列表项的子块（此前段落会被惰性续行并回首段、代码块跳出列表）。
func TestListChildBlocksExportIndentedAndRoundtrip(t *testing.T) {
	lang := 1
	blocks := []*larkdocx.Block{
		{BlockId: strPtr("b1"), BlockType: intPtr(int(BlockTypeBullet)), Bullet: listText("首段"), Children: []string{"t1", "c1"}},
		{BlockId: strPtr("t1"), BlockType: intPtr(int(BlockTypeText)), Text: listText("第二段")},
		{BlockId: strPtr("c1"), BlockType: intPtr(int(BlockTypeCode)), Code: &larkdocx.Text{
			Elements: listText("x := 1").Elements, Style: &larkdocx.TextStyle{Language: &lang},
		}},
	}
	md, err := NewBlockToMarkdown(blocks, ConvertOptions{}).Convert()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "- 首段\n\n  第二段\n\n  ```") {
		t.Fatalf("子块应缩进并空行分隔:\n%s", md)
	}
	result := convertForTest(t, md, ConvertOptions{})
	if len(result.BlockNodes) != 1 {
		t.Fatalf("再导入应仍是 1 个列表项, got %d:\n%s", len(result.BlockNodes), md)
	}
	item := result.BlockNodes[0]
	if blockText(item.Block) != "首段" || len(item.Children) != 2 || blockText(item.Children[0].Block) != "第二段" || item.Children[1].Block.Code == nil {
		t.Fatalf("往返后结构异常: %q %#v", blockText(item.Block), item.Children)
	}
}

// ===== 缺陷 7：单元格公式 =====

// TestTableCellInlineMathConverted 单元格里的 $...$ 与正文一致地转为公式元素；<br> 分隔符保持独立。
func TestTableCellInlineMathConverted(t *testing.T) {
	md := "| 公式 | 说明 |\n| --- | --- |\n| $x^2$ | 前缀 $\\alpha$ 后缀<br>第二行 |\n"
	td := convertForTest(t, md, ConvertOptions{UploadImages: true, EmbedTableImages: true}).TableDatas[0]
	first := td.CellElements[2]
	if len(first) != 1 || first[0].Equation == nil || *first[0].Equation.Content != "x^2" {
		t.Fatalf("纯公式单元格应为公式元素: %#v", first)
	}
	second := td.CellElements[3]
	var eq, br int
	for _, e := range second {
		if e.Equation != nil && *e.Equation.Content == "\\alpha" {
			eq++
		}
		if e.TextRun != nil && *e.TextRun.Content == "\n" {
			br++
		}
	}
	if eq != 1 || br != 1 || textOf(second) != "前缀 $\\alpha$ 后缀\n第二行" {
		t.Fatalf("混排单元格公式/换行异常: %q eq=%d br=%d", textOf(second), eq, br)
	}
}
