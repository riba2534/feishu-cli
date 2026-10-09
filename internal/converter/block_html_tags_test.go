package converter

import (
	"strings"
	"testing"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
)

// ===========================================================================
// Phase 3: Block-level HTML Tag Import Tests
// ===========================================================================

// --- Grid Import ---

func TestImportGridBasic(t *testing.T) {
	md := "<grid cols=\"2\">\n<column>\n左栏内容\n</column>\n<column>\n右栏内容\n</column>\n</grid>\n"
	conv := NewMarkdownToBlock([]byte(md), ConvertOptions{}, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("ConvertWithTableData() error = %v", err)
	}
	if len(result.BlockNodes) != 1 {
		t.Fatalf("expected 1 block node, got %d", len(result.BlockNodes))
	}
	gridNode := result.BlockNodes[0]
	if gridNode.Block.Grid == nil {
		t.Fatal("expected Grid block")
	}
	if *gridNode.Block.Grid.ColumnSize != 2 {
		t.Errorf("expected ColumnSize=2, got %d", *gridNode.Block.Grid.ColumnSize)
	}
	if len(gridNode.Children) != 2 {
		t.Fatalf("expected 2 column children, got %d", len(gridNode.Children))
	}
	// 检查每个 column 有内容子块
	for i, col := range gridNode.Children {
		if col.Block.GridColumn == nil {
			t.Errorf("column %d: expected GridColumn block", i)
		}
		if len(col.Children) == 0 {
			t.Errorf("column %d: expected children, got 0", i)
		}
	}
}

func TestImportGridDefaultCols(t *testing.T) {
	// 不指定 cols 属性时默认为 2
	md := "<grid>\n<column>\nA\n</column>\n<column>\nB\n</column>\n</grid>\n"
	conv := NewMarkdownToBlock([]byte(md), ConvertOptions{}, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(result.BlockNodes) != 1 {
		t.Fatalf("expected 1 block, got %d", len(result.BlockNodes))
	}
	if *result.BlockNodes[0].Block.Grid.ColumnSize != 2 {
		t.Errorf("expected default ColumnSize=2, got %d", *result.BlockNodes[0].Block.Grid.ColumnSize)
	}
}

func TestImportGridMaxCols(t *testing.T) {
	// cols > 5 应被截断到 5
	md := "<grid cols=\"10\">\n<column>\nA\n</column>\n</grid>\n"
	conv := NewMarkdownToBlock([]byte(md), ConvertOptions{}, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if *result.BlockNodes[0].Block.Grid.ColumnSize != 5 {
		t.Errorf("expected max ColumnSize=5, got %d", *result.BlockNodes[0].Block.Grid.ColumnSize)
	}
}

// --- Whiteboard Import ---

func TestImportWhiteboardBlank(t *testing.T) {
	md := "<whiteboard type=\"blank\"/>\n"
	conv := NewMarkdownToBlock([]byte(md), ConvertOptions{}, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(result.BlockNodes) != 1 {
		t.Fatalf("expected 1 block, got %d", len(result.BlockNodes))
	}
	block := result.BlockNodes[0].Block
	if int(BlockType(*block.BlockType)) != int(BlockTypeBoard) {
		t.Errorf("expected Board block type (43), got %d", *block.BlockType)
	}
	if block.Board == nil {
		t.Fatal("expected Board field")
	}
}

// TestImportWhiteboardWithToken 带 token 的画板：建块接口拒绝带 token 的 Board（1770001），
// 转换器只建空画板并登记 MediaRef，由导入层复制源画板节点。
func TestImportWhiteboardWithToken(t *testing.T) {
	md := "<whiteboard token=\"board_abc\" type=\"blank\"/>\n"
	conv := NewMarkdownToBlock([]byte(md), ConvertOptions{}, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	block := result.BlockNodes[0].Block
	if block.Board == nil {
		t.Fatal("expected Board block")
	}
	if block.Board.Token != nil {
		t.Fatalf("建块请求不能带画板 token（服务端 1770001），got %q", *block.Board.Token)
	}
	ref := result.MediaRefs[block]
	if ref == nil || ref.Kind != MediaKindWhiteboard || ref.Token != "board_abc" {
		t.Fatalf("expected whiteboard MediaRef with source token, got %#v", ref)
	}
}

// --- Sheet Import ---

func TestImportSheetDefault(t *testing.T) {
	md := "<sheet/>\n"
	conv := NewMarkdownToBlock([]byte(md), ConvertOptions{}, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(result.BlockNodes) != 1 {
		t.Fatalf("expected 1 block, got %d", len(result.BlockNodes))
	}
	block := result.BlockNodes[0].Block
	if int(BlockType(*block.BlockType)) != int(BlockTypeSheet) {
		t.Errorf("expected Sheet block type (30), got %d", *block.BlockType)
	}
	if block.Sheet == nil {
		t.Fatal("expected Sheet field")
	}
	if *block.Sheet.RowSize != 3 {
		t.Errorf("expected default RowSize=3, got %d", *block.Sheet.RowSize)
	}
	if *block.Sheet.ColumnSize != 3 {
		t.Errorf("expected default ColumnSize=3, got %d", *block.Sheet.ColumnSize)
	}
}

func TestImportSheetWithAttrs(t *testing.T) {
	conv := NewMarkdownToBlock([]byte("<sheet rows=\"5\" cols=\"8\"/>\n"), ConvertOptions{}, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	block := result.BlockNodes[0].Block
	if *block.Sheet.RowSize != 5 || *block.Sheet.ColumnSize != 8 {
		t.Errorf("expected 5x8, got %dx%d", *block.Sheet.RowSize, *block.Sheet.ColumnSize)
	}
	if block.Sheet.Token != nil {
		t.Errorf("新建空表格不应带 token")
	}
}

// TestImportSheetWithTokenDegradesToLink 引用已有电子表格的 <sheet token>：建块接口拒绝（1770001），
// 降级为指向原表格的链接文本并登记 Degradation，不再让整篇导入失败。
func TestImportSheetWithTokenDegradesToLink(t *testing.T) {
	tests := []struct {
		name    string
		md      string
		wantRef string
		wantURL string
	}{
		{"legacy combined token", "<sheet rows=\"5\" cols=\"8\" token=\"sheet_xyz\"/>\n", "sheet_xyz", "/sheets/sheet_xyz"},
		{"split token and id", "<sheet rows=\"5\" cols=\"8\" token=\"sheet\" id=\"xyz\"/>\n", "sheet_xyz", "/sheets/sheet?sheet=xyz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NewMarkdownToBlock([]byte(tt.md), ConvertOptions{}, "").ConvertWithTableData()
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			block := result.BlockNodes[0].Block
			if block.Sheet != nil || block.Text == nil {
				t.Fatalf("带 token 的 sheet 应降级为文本块, got %#v", block)
			}
			el := block.Text.Elements[0]
			if !strings.Contains(*el.TextRun.Content, tt.wantRef) {
				t.Errorf("占位文本应含原引用 %q: %q", tt.wantRef, *el.TextRun.Content)
			}
			if el.TextRun.TextElementStyle == nil || el.TextRun.TextElementStyle.Link == nil ||
				!strings.Contains(*el.TextRun.TextElementStyle.Link.Url, tt.wantURL) {
				t.Errorf("占位文本应链接到原表格 %q: %#v", tt.wantURL, el.TextRun.TextElementStyle)
			}
			if len(result.Degradations) != 1 || result.Degradations[0].Kind != "sheet" || result.Degradations[0].Source != tt.wantRef {
				t.Errorf("应登记 sheet 降级: %#v", result.Degradations)
			}
		})
	}
}

// --- Bitable Import ---

func TestImportBitableDefault(t *testing.T) {
	md := "<bitable/>\n"
	conv := NewMarkdownToBlock([]byte(md), ConvertOptions{}, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(result.BlockNodes) != 1 {
		t.Fatalf("expected 1 block, got %d", len(result.BlockNodes))
	}
	block := result.BlockNodes[0].Block
	if int(BlockType(*block.BlockType)) != int(BlockTypeBitable) {
		t.Errorf("expected Bitable block type (18), got %d", *block.BlockType)
	}
	if block.Bitable == nil {
		t.Fatal("expected Bitable field")
	}
	if *block.Bitable.ViewType != 1 {
		t.Errorf("expected default ViewType=1, got %d", *block.Bitable.ViewType)
	}
}

func TestImportBitableKanban(t *testing.T) {
	md := "<bitable view=\"kanban\"/>\n"
	conv := NewMarkdownToBlock([]byte(md), ConvertOptions{}, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	block := result.BlockNodes[0].Block
	if *block.Bitable.ViewType != 2 {
		t.Errorf("expected ViewType=2 (kanban), got %d", *block.Bitable.ViewType)
	}
}

// TestImportBitableWithTokenDegradesToLink 引用已有多维表格的 <bitable token>：降级为链接文本 + Degradation。
func TestImportBitableWithTokenDegradesToLink(t *testing.T) {
	md := "<bitable view=\"kanban\" token=\"bt_abc\"/>\n"
	result, err := NewMarkdownToBlock([]byte(md), ConvertOptions{}, "").ConvertWithTableData()
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	block := result.BlockNodes[0].Block
	if block.Bitable != nil || block.Text == nil {
		t.Fatalf("带 token 的 bitable 应降级为文本块, got %#v", block)
	}
	el := block.Text.Elements[0]
	if el.TextRun.TextElementStyle == nil || el.TextRun.TextElementStyle.Link == nil ||
		!strings.Contains(*el.TextRun.TextElementStyle.Link.Url, "/base/bt?table=abc") {
		t.Errorf("应链接到原多维表格: %#v", el.TextRun)
	}
	if len(result.Degradations) != 1 || result.Degradations[0].Kind != "bitable" {
		t.Errorf("应登记 bitable 降级: %#v", result.Degradations)
	}
}

// --- File Import ---

// TestImportFileWithTokenAndName 带 token 的附件：建块只能用 {"token":""}（带 token/name 均 1770001），
// 转换器建空 File 块并登记 MediaRef（复用原素材 + 原文件名），关闭上传时降级为占位文本。
func TestImportFileWithTokenAndName(t *testing.T) {
	md := "<file token=\"file_abc\" name=\"report.pdf\"/>\n"
	result, err := NewMarkdownToBlock([]byte(md), ConvertOptions{UploadImages: true}, "").ConvertWithTableData()
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(result.BlockNodes) != 1 {
		t.Fatalf("expected 1 block, got %d", len(result.BlockNodes))
	}
	block := result.BlockNodes[0].Block
	if int(BlockType(*block.BlockType)) != int(BlockTypeFile) || block.File == nil {
		t.Fatalf("expected File block, got %#v", block)
	}
	if block.File.Token == nil || *block.File.Token != "" || block.File.Name != nil {
		t.Fatalf("建块请求只能带空 token、不能带 name: %#v", block.File)
	}
	ref := result.MediaRefs[block]
	if ref == nil || ref.Kind != MediaKindFile || ref.Token != "file_abc" || ref.Name != "report.pdf" || ref.Video {
		t.Fatalf("unexpected MediaRef: %#v", ref)
	}
	if result.FileStats.Total != 1 {
		t.Fatalf("FileStats = %#v", result.FileStats)
	}

	off, _ := NewMarkdownToBlock([]byte(md), ConvertOptions{}, "").ConvertWithTableData()
	if b := off.BlockNodes[0].Block; b.Text == nil || off.FileStats.Skipped != 1 || len(off.MediaRefs) != 0 {
		t.Fatalf("关闭上传时应降级为占位文本: %#v stats=%#v", b, off.FileStats)
	}
}

// TestImportFileNameOnlyDegrades 只有 name 没有 token 的 <file>：无可导入内容，降级为占位文本并登记。
func TestImportFileNameOnlyDegrades(t *testing.T) {
	result, err := NewMarkdownToBlock([]byte("<file name=\"a.pdf\"/>\n"), ConvertOptions{UploadImages: true}, "").ConvertWithTableData()
	if err != nil {
		t.Fatal(err)
	}
	if b := result.BlockNodes[0].Block; b.Text == nil {
		t.Fatalf("expected placeholder text, got %#v", b)
	}
	if len(result.Degradations) != 1 || result.Degradations[0].Kind != "file" {
		t.Fatalf("expected file degradation: %#v", result.Degradations)
	}
}

func TestImportFileEmpty(t *testing.T) {
	md := "<file/>\n"
	conv := NewMarkdownToBlock([]byte(md), ConvertOptions{}, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	// 空 file 标签（无 token 无 name）应被忽略
	if len(result.BlockNodes) != 0 {
		t.Errorf("expected 0 blocks for empty <file/>, got %d", len(result.BlockNodes))
	}
}

func TestImportFileWithViewType(t *testing.T) {
	md := "<file token=\"file_abc\" name=\"doc.docx\" view-type=\"2\"/>\n"
	conv := NewMarkdownToBlock([]byte(md), ConvertOptions{UploadImages: true}, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	block := result.BlockNodes[0].Block
	if block.File.ViewType == nil || *block.File.ViewType != 2 {
		t.Errorf("expected ViewType=2, got %v", block.File.ViewType)
	}
}

func TestImportVideoWithLocalSrc(t *testing.T) {
	md := "<video src=\"./demo.mp4\" controls></video>\n"
	conv := NewMarkdownToBlock([]byte(md), ConvertOptions{UploadImages: true}, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(result.BlockNodes) != 1 {
		t.Fatalf("expected 1 block, got %d", len(result.BlockNodes))
	}
	block := result.BlockNodes[0].Block
	if int(BlockType(*block.BlockType)) != int(BlockTypeFile) {
		t.Fatalf("expected File block type (23), got %d", *block.BlockType)
	}
	if block.File == nil || block.File.Token == nil || *block.File.Token != "" || block.File.Name != nil {
		t.Fatalf("视频建块只能带空 token（带 name 服务端 1770001）, got %#v", block.File)
	}
	if len(result.VideoSources) != 1 || result.VideoSources[0] != "./demo.mp4" {
		t.Fatalf("expected video source ./demo.mp4, got %#v", result.VideoSources)
	}
	ref := result.MediaRefs[block]
	if ref == nil || ref.Kind != MediaKindFile || !ref.Video || ref.Source != "./demo.mp4" || ref.Name != "demo.mp4" {
		t.Fatalf("expected video MediaRef, got %#v", ref)
	}
}

// ===========================================================================
// Phase 3: Block-level HTML Tag Export Tests
// ===========================================================================

// --- Grid Export ---

func TestExportGridWithColumns(t *testing.T) {
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("grid1"),
			BlockType: intPtr(int(BlockTypeGrid)),
			Grid: &larkdocx.Grid{
				ColumnSize: intPtr(3),
			},
			Children: []string{"col1", "col2", "col3"},
		},
		{
			BlockId:    strPtr("col1"),
			BlockType:  intPtr(int(BlockTypeGridColumn)),
			GridColumn: &larkdocx.GridColumn{},
			Children:   []string{"text1"},
		},
		createTextBlock("text1", "First"),
		{
			BlockId:    strPtr("col2"),
			BlockType:  intPtr(int(BlockTypeGridColumn)),
			GridColumn: &larkdocx.GridColumn{},
			Children:   []string{"text2"},
		},
		createTextBlock("text2", "Second"),
		{
			BlockId:    strPtr("col3"),
			BlockType:  intPtr(int(BlockTypeGridColumn)),
			GridColumn: &larkdocx.GridColumn{},
			Children:   []string{"text3"},
		},
		createTextBlock("text3", "Third"),
	}

	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	got, err := conv.Convert()
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}

	if !strings.Contains(got, "<grid cols=\"3\">") {
		t.Errorf("expected <grid cols=\"3\">, got:\n%s", got)
	}
	if !strings.Contains(got, "<column>") {
		t.Errorf("expected <column>, got:\n%s", got)
	}
	if !strings.Contains(got, "</column>") {
		t.Errorf("expected </column>, got:\n%s", got)
	}
	if !strings.Contains(got, "</grid>") {
		t.Errorf("expected </grid>, got:\n%s", got)
	}
	if !strings.Contains(got, "First") {
		t.Errorf("expected 'First' in output, got:\n%s", got)
	}
}

// --- Sheet Export ---

func TestExportSheetWithRowsCols(t *testing.T) {
	rows := 5
	cols := 8
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("s1"),
			BlockType: intPtr(int(BlockTypeSheet)),
			Sheet: &larkdocx.Sheet{
				Token:      strPtr("sheet_xyz"),
				RowSize:    &rows,
				ColumnSize: &cols,
			},
		},
	}
	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	got, err := conv.Convert()
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	got = strings.TrimSpace(got)
	want := `<sheet token="sheet" id="xyz" rows="5" cols="8"/>`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// --- Bitable Export ---

func TestExportBitableKanban(t *testing.T) {
	viewType := 2
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("b1"),
			BlockType: intPtr(int(BlockTypeBitable)),
			Bitable: &larkdocx.Bitable{
				Token:    strPtr("bt_123"),
				ViewType: &viewType,
			},
		},
	}
	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	got, err := conv.Convert()
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	got = strings.TrimSpace(got)
	want := `<bitable token="bt_123" view="kanban"/>`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// --- Board/Whiteboard Export ---

func TestExportBoardAsWhiteboard(t *testing.T) {
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("board1"),
			BlockType: intPtr(int(BlockTypeBoard)),
			Board: &larkdocx.Board{
				Token: strPtr("board_xyz"),
			},
		},
	}
	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	got, err := conv.Convert()
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	got = strings.TrimSpace(got)
	want := `<whiteboard token="board_xyz" type="blank"/>`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestExportBoardEmptyToken(t *testing.T) {
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("board1"),
			BlockType: intPtr(int(BlockTypeBoard)),
			Board: &larkdocx.Board{
				Token: strPtr(""),
			},
		},
	}
	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	got, err := conv.Convert()
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	got = strings.TrimSpace(got)
	want := `<whiteboard type="blank"/>`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// --- File Export ---

func TestExportFileTag(t *testing.T) {
	vt := 2
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("f1"),
			BlockType: intPtr(int(BlockTypeFile)),
			File: &larkdocx.File{
				Token:    strPtr("file_abc"),
				Name:     strPtr("report.pdf"),
				ViewType: &vt,
			},
		},
	}
	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	got, err := conv.Convert()
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	got = strings.TrimSpace(got)
	want := `<file token="file_abc" name="report.pdf" view-type="2"/>`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// ===========================================================================
// Phase 3: Roundtrip Tests (export -> import -> check)
// ===========================================================================

func TestRoundtripWhiteboard(t *testing.T) {
	// 导出 Board → <whiteboard .../> → 导入回 Board
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("board1"),
			BlockType: intPtr(int(BlockTypeBoard)),
			Board: &larkdocx.Board{
				Token: strPtr("board_roundtrip"),
			},
		},
	}
	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	md, err := conv.Convert()
	if err != nil {
		t.Fatalf("export error: %v", err)
	}

	// 导入
	conv2 := NewMarkdownToBlock([]byte(md), ConvertOptions{}, "")
	result, err := conv2.ConvertWithTableData()
	if err != nil {
		t.Fatalf("import error: %v", err)
	}
	if len(result.BlockNodes) != 1 {
		t.Fatalf("expected 1 block, got %d", len(result.BlockNodes))
	}
	block := result.BlockNodes[0].Block
	if block.Board == nil {
		t.Fatal("expected Board block after roundtrip")
	}
	// 建块不能带 token（1770001）：导回为空画板 + 源画板 MediaRef（导入层复制源画板节点）
	if block.Board.Token != nil {
		t.Errorf("建块请求不应带画板 token, got %v", *block.Board.Token)
	}
	if ref := result.MediaRefs[block]; ref == nil || ref.Kind != MediaKindWhiteboard || ref.Token != "board_roundtrip" {
		t.Errorf("expected whiteboard MediaRef 'board_roundtrip', got %#v", ref)
	}
}

func TestRoundtripSheet(t *testing.T) {
	rows := 5
	cols := 8
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("s1"),
			BlockType: intPtr(int(BlockTypeSheet)),
			Sheet: &larkdocx.Sheet{
				Token:      strPtr("sheet_rt"),
				RowSize:    &rows,
				ColumnSize: &cols,
			},
		},
	}
	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	md, err := conv.Convert()
	if err != nil {
		t.Fatalf("export error: %v", err)
	}

	conv2 := NewMarkdownToBlock([]byte(md), ConvertOptions{}, "")
	result, err := conv2.ConvertWithTableData()
	if err != nil {
		t.Fatalf("import error: %v", err)
	}
	if len(result.BlockNodes) != 1 {
		t.Fatalf("expected 1 block, got %d", len(result.BlockNodes))
	}
	// 引用已有表格无法建块（1770001）：导回为指向原表格的链接文本 + Degradation
	block := result.BlockNodes[0].Block
	if block.Text == nil || !strings.Contains(*block.Text.Elements[0].TextRun.Content, "sheet_rt") {
		t.Fatalf("expected link placeholder after roundtrip, got %#v", block)
	}
	if len(result.Degradations) != 1 || result.Degradations[0].Kind != "sheet" {
		t.Errorf("expected sheet degradation, got %#v", result.Degradations)
	}
}

func TestRoundtripBitable(t *testing.T) {
	viewType := 2
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("b1"),
			BlockType: intPtr(int(BlockTypeBitable)),
			Bitable: &larkdocx.Bitable{
				Token:    strPtr("bt_rt"),
				ViewType: &viewType,
			},
		},
	}
	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	md, err := conv.Convert()
	if err != nil {
		t.Fatalf("export error: %v", err)
	}

	conv2 := NewMarkdownToBlock([]byte(md), ConvertOptions{}, "")
	result, err := conv2.ConvertWithTableData()
	if err != nil {
		t.Fatalf("import error: %v", err)
	}
	if len(result.BlockNodes) != 1 {
		t.Fatalf("expected 1 block, got %d", len(result.BlockNodes))
	}
	block := result.BlockNodes[0].Block
	if block.Text == nil || !strings.Contains(*block.Text.Elements[0].TextRun.Content, "bt_rt") {
		t.Fatalf("expected link placeholder after roundtrip, got %#v", block)
	}
	if len(result.Degradations) != 1 || result.Degradations[0].Kind != "bitable" {
		t.Errorf("expected bitable degradation, got %#v", result.Degradations)
	}
}

func TestRoundtripFile(t *testing.T) {
	vt := 2
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("f1"),
			BlockType: intPtr(int(BlockTypeFile)),
			File: &larkdocx.File{
				Token:    strPtr("file_rt"),
				Name:     strPtr("test.pdf"),
				ViewType: &vt,
			},
		},
	}
	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	md, err := conv.Convert()
	if err != nil {
		t.Fatalf("export error: %v", err)
	}

	conv2 := NewMarkdownToBlock([]byte(md), ConvertOptions{UploadImages: true}, "")
	result, err := conv2.ConvertWithTableData()
	if err != nil {
		t.Fatalf("import error: %v", err)
	}
	if len(result.BlockNodes) != 1 {
		t.Fatalf("expected 1 block, got %d", len(result.BlockNodes))
	}
	block := result.BlockNodes[0].Block
	if block.File == nil {
		t.Fatal("expected File block after roundtrip")
	}
	// 建块只带空 token；原 token/name 进 MediaRef，由导入层下载原附件再上传（附件复用）
	if block.File.Token == nil || *block.File.Token != "" || block.File.Name != nil {
		t.Errorf("建块请求只能带空 token: %#v", block.File)
	}
	if ref := result.MediaRefs[block]; ref == nil || ref.Token != "file_rt" || ref.Name != "test.pdf" {
		t.Errorf("expected MediaRef file_rt/test.pdf, got %#v", ref)
	}
	if *block.File.ViewType != 2 {
		t.Errorf("viewType: got %d, want 2", *block.File.ViewType)
	}
}

// ===========================================================================
// ParseGridColumns Tests
// ===========================================================================

func TestParseGridColumns(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int // expected number of columns
	}{
		{"two columns", "<column>\nA\n</column>\n<column>\nB\n</column>", 2},
		{"three columns", "<column>X</column><column>Y</column><column>Z</column>", 3},
		{"no columns", "just text", 0},
		{"empty", "", 0},
		{"one column", "<column>only one</column>", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cols := ParseGridColumns(tt.content)
			if len(cols) != tt.want {
				t.Errorf("ParseGridColumns() returned %d columns, want %d", len(cols), tt.want)
			}
		})
	}
}

func TestParseGridColumnsContent(t *testing.T) {
	content := "<column>\nHello World\n</column>\n<column>\nFoo Bar\n</column>"
	cols := ParseGridColumns(content)
	if len(cols) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(cols))
	}
	if cols[0] != "Hello World" {
		t.Errorf("col[0] = %q, want 'Hello World'", cols[0])
	}
	if cols[1] != "Foo Bar" {
		t.Errorf("col[1] = %q, want 'Foo Bar'", cols[1])
	}
}

// ===========================================================================
// parseHTMLIntAttrDefault Tests
// ===========================================================================

func TestParseHTMLIntAttrDefault(t *testing.T) {
	tests := []struct {
		input      string
		defaultVal int
		want       int
	}{
		{"5", 3, 5},
		{"", 3, 3},
		{"abc", 3, 3},
		{"0", 3, 0},
		{"-1", 3, -1},
	}
	for _, tt := range tests {
		got := parseHTMLIntAttrDefault(tt.input, tt.defaultVal)
		if got != tt.want {
			t.Errorf("parseHTMLIntAttrDefault(%q, %d) = %d, want %d", tt.input, tt.defaultVal, got, tt.want)
		}
	}
}
