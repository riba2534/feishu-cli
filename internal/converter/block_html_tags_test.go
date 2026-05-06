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

func TestImportWhiteboardWithToken(t *testing.T) {
	md := "<whiteboard token=\"board_abc\" type=\"blank\"/>\n"
	conv := NewMarkdownToBlock([]byte(md), ConvertOptions{}, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	block := result.BlockNodes[0].Block
	if block.Board == nil || block.Board.Token == nil {
		t.Fatal("expected Board with token")
	}
	if *block.Board.Token != "board_abc" {
		t.Errorf("expected token 'board_abc', got %q", *block.Board.Token)
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
	tests := []struct {
		name      string
		md        string
		wantToken string
	}{
		{
			name:      "legacy combined token",
			md:        "<sheet rows=\"5\" cols=\"8\" token=\"sheet_xyz\"/>\n",
			wantToken: "sheet_xyz",
		},
		{
			name:      "split token and id",
			md:        "<sheet rows=\"5\" cols=\"8\" token=\"sheet\" id=\"xyz\"/>\n",
			wantToken: "sheet_xyz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewMarkdownToBlock([]byte(tt.md), ConvertOptions{}, "")
			result, err := conv.ConvertWithTableData()
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			block := result.BlockNodes[0].Block
			if *block.Sheet.RowSize != 5 {
				t.Errorf("expected RowSize=5, got %d", *block.Sheet.RowSize)
			}
			if *block.Sheet.ColumnSize != 8 {
				t.Errorf("expected ColumnSize=8, got %d", *block.Sheet.ColumnSize)
			}
			if *block.Sheet.Token != tt.wantToken {
				t.Errorf("expected token %q, got %q", tt.wantToken, *block.Sheet.Token)
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
	md := "<bitable view=\"kanban\" token=\"bt_abc\"/>\n"
	conv := NewMarkdownToBlock([]byte(md), ConvertOptions{}, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	block := result.BlockNodes[0].Block
	if *block.Bitable.ViewType != 2 {
		t.Errorf("expected ViewType=2 (kanban), got %d", *block.Bitable.ViewType)
	}
	if *block.Bitable.Token != "bt_abc" {
		t.Errorf("expected token 'bt_abc', got %q", *block.Bitable.Token)
	}
}

// --- File Import ---

func TestImportFileWithTokenAndName(t *testing.T) {
	md := "<file token=\"file_abc\" name=\"report.pdf\"/>\n"
	conv := NewMarkdownToBlock([]byte(md), ConvertOptions{}, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(result.BlockNodes) != 1 {
		t.Fatalf("expected 1 block, got %d", len(result.BlockNodes))
	}
	block := result.BlockNodes[0].Block
	if int(BlockType(*block.BlockType)) != int(BlockTypeFile) {
		t.Errorf("expected File block type (23), got %d", *block.BlockType)
	}
	if block.File == nil {
		t.Fatal("expected File field")
	}
	if *block.File.Token != "file_abc" {
		t.Errorf("expected token 'file_abc', got %q", *block.File.Token)
	}
	if *block.File.Name != "report.pdf" {
		t.Errorf("expected name 'report.pdf', got %q", *block.File.Name)
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
	conv := NewMarkdownToBlock([]byte(md), ConvertOptions{}, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	block := result.BlockNodes[0].Block
	if block.File.ViewType == nil || *block.File.ViewType != 2 {
		t.Errorf("expected ViewType=2, got %v", block.File.ViewType)
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
	if block.Board.Token == nil || *block.Board.Token != "board_roundtrip" {
		t.Errorf("expected token 'board_roundtrip', got %v", block.Board.Token)
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
	block := result.BlockNodes[0].Block
	if block.Sheet == nil {
		t.Fatal("expected Sheet block after roundtrip")
	}
	if *block.Sheet.Token != "sheet_rt" {
		t.Errorf("token: got %q, want 'sheet_rt'", *block.Sheet.Token)
	}
	if *block.Sheet.RowSize != 5 {
		t.Errorf("rows: got %d, want 5", *block.Sheet.RowSize)
	}
	if *block.Sheet.ColumnSize != 8 {
		t.Errorf("cols: got %d, want 8", *block.Sheet.ColumnSize)
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
	if block.Bitable == nil {
		t.Fatal("expected Bitable block after roundtrip")
	}
	if *block.Bitable.Token != "bt_rt" {
		t.Errorf("token: got %q, want 'bt_rt'", *block.Bitable.Token)
	}
	if *block.Bitable.ViewType != 2 {
		t.Errorf("viewType: got %d, want 2", *block.Bitable.ViewType)
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

	conv2 := NewMarkdownToBlock([]byte(md), ConvertOptions{}, "")
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
	if *block.File.Token != "file_rt" {
		t.Errorf("token: got %q, want 'file_rt'", *block.File.Token)
	}
	if *block.File.Name != "test.pdf" {
		t.Errorf("name: got %q, want 'test.pdf'", *block.File.Name)
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
