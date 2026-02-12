package converter

import (
	"strings"
	"testing"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
)

// ============================================================================
// Table Tests
// ============================================================================

func TestConvertTable(t *testing.T) {
	tests := []struct {
		name    string
		blocks  []*larkdocx.Block
		want    string
		wantErr bool
	}{
		{
			name: "table is nil",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("table1"),
					BlockType: intPtr(int(BlockTypeTable)),
					Table:     nil,
				},
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "table cells is nil",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("table1"),
					BlockType: intPtr(int(BlockTypeTable)),
					Table: &larkdocx.Table{
						Cells: nil,
					},
				},
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "property is nil - rows and cols default to 0",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("table1"),
					BlockType: intPtr(int(BlockTypeTable)),
					Table: &larkdocx.Table{
						Cells:    []string{},
						Property: nil,
					},
				},
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "row size is nil",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("table1"),
					BlockType: intPtr(int(BlockTypeTable)),
					Table: &larkdocx.Table{
						Cells: []string{},
						Property: &larkdocx.TableProperty{
							RowSize:    nil,
							ColumnSize: intPtr(2),
						},
					},
				},
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "column size is nil",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("table1"),
					BlockType: intPtr(int(BlockTypeTable)),
					Table: &larkdocx.Table{
						Cells: []string{},
						Property: &larkdocx.TableProperty{
							RowSize:    intPtr(2),
							ColumnSize: nil,
						},
					},
				},
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "rows is zero",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("table1"),
					BlockType: intPtr(int(BlockTypeTable)),
					Table: &larkdocx.Table{
						Cells: []string{},
						Property: &larkdocx.TableProperty{
							RowSize:    intPtr(0),
							ColumnSize: intPtr(2),
						},
					},
				},
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "cols is zero",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("table1"),
					BlockType: intPtr(int(BlockTypeTable)),
					Table: &larkdocx.Table{
						Cells: []string{},
						Property: &larkdocx.TableProperty{
							RowSize:    intPtr(2),
							ColumnSize: intPtr(0),
						},
					},
				},
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "cells count insufficient",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("table1"),
					BlockType: intPtr(int(BlockTypeTable)),
					Table: &larkdocx.Table{
						Cells: []string{"cell1", "cell2"}, // only 2, need 4
						Property: &larkdocx.TableProperty{
							RowSize:    intPtr(2),
							ColumnSize: intPtr(2),
						},
					},
				},
			},
			want:    "|  |  |\n| --- | --- |", // rows becomes 1 (len(cells)/cols = 2/2 = 1)
			wantErr: false,
		},
		{
			name: "normal 2x2 table",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("table1"),
					BlockType: intPtr(int(BlockTypeTable)),
					Table: &larkdocx.Table{
						Cells: []string{"cell1", "cell2", "cell3", "cell4"},
						Property: &larkdocx.TableProperty{
							RowSize:    intPtr(2),
							ColumnSize: intPtr(2),
						},
					},
				},
				{
					BlockId:   strPtr("cell1"),
					BlockType: intPtr(int(BlockTypeTableCell)),
					TableCell: &larkdocx.TableCell{},
					Children:  []string{"text1"},
				},
				{
					BlockId:   strPtr("text1"),
					BlockType: intPtr(int(BlockTypeText)),
					Text: &larkdocx.Text{
						Elements: []*larkdocx.TextElement{
							{TextRun: &larkdocx.TextRun{Content: strPtr("A1")}},
						},
					},
				},
				{
					BlockId:   strPtr("cell2"),
					BlockType: intPtr(int(BlockTypeTableCell)),
					TableCell: &larkdocx.TableCell{},
					Children:  []string{"text2"},
				},
				{
					BlockId:   strPtr("text2"),
					BlockType: intPtr(int(BlockTypeText)),
					Text: &larkdocx.Text{
						Elements: []*larkdocx.TextElement{
							{TextRun: &larkdocx.TextRun{Content: strPtr("B1")}},
						},
					},
				},
				{
					BlockId:   strPtr("cell3"),
					BlockType: intPtr(int(BlockTypeTableCell)),
					TableCell: &larkdocx.TableCell{},
					Children:  []string{"text3"},
				},
				{
					BlockId:   strPtr("text3"),
					BlockType: intPtr(int(BlockTypeText)),
					Text: &larkdocx.Text{
						Elements: []*larkdocx.TextElement{
							{TextRun: &larkdocx.TextRun{Content: strPtr("A2")}},
						},
					},
				},
				{
					BlockId:   strPtr("cell4"),
					BlockType: intPtr(int(BlockTypeTableCell)),
					TableCell: &larkdocx.TableCell{},
					Children:  []string{"text4"},
				},
				{
					BlockId:   strPtr("text4"),
					BlockType: intPtr(int(BlockTypeText)),
					Text: &larkdocx.Text{
						Elements: []*larkdocx.TextElement{
							{TextRun: &larkdocx.TextRun{Content: strPtr("B2")}},
						},
					},
				},
			},
			want: "| A1 | B1 |\n| --- | --- |\n| A2 | B2 |",
		},
		{
			name: "normal 3x3 table",
			blocks: func() []*larkdocx.Block {
				blocks := []*larkdocx.Block{
					{
						BlockId:   strPtr("table1"),
						BlockType: intPtr(int(BlockTypeTable)),
						Table: &larkdocx.Table{
							Cells: []string{"c1", "c2", "c3", "c4", "c5", "c6", "c7", "c8", "c9"},
							Property: &larkdocx.TableProperty{
								RowSize:    intPtr(3),
								ColumnSize: intPtr(3),
							},
						},
					},
				}
				blocks = append(blocks, createTableCell("c1", "H1")...)
				blocks = append(blocks, createTableCell("c2", "H2")...)
				blocks = append(blocks, createTableCell("c3", "H3")...)
				blocks = append(blocks, createTableCell("c4", "R1C1")...)
				blocks = append(blocks, createTableCell("c5", "R1C2")...)
				blocks = append(blocks, createTableCell("c6", "R1C3")...)
				blocks = append(blocks, createTableCell("c7", "R2C1")...)
				blocks = append(blocks, createTableCell("c8", "R2C2")...)
				blocks = append(blocks, createTableCell("c9", "R2C3")...)
				return blocks
			}(),
			want: "| H1 | H2 | H3 |\n| --- | --- | --- |\n| R1C1 | R1C2 | R1C3 |\n| R2C1 | R2C2 | R2C3 |",
		},
		{
			name: "cell id not in blockMap",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("table1"),
					BlockType: intPtr(int(BlockTypeTable)),
					Table: &larkdocx.Table{
						Cells: []string{"nonexistent1", "nonexistent2", "nonexistent3", "nonexistent4"},
						Property: &larkdocx.TableProperty{
							RowSize:    intPtr(2),
							ColumnSize: intPtr(2),
						},
					},
				},
			},
			want: "|  |  |\n| --- | --- |\n|  |  |",
		},
		{
			name: "cell content contains pipe symbol",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("table1"),
					BlockType: intPtr(int(BlockTypeTable)),
					Table: &larkdocx.Table{
						Cells: []string{"cell1", "cell2"},
						Property: &larkdocx.TableProperty{
							RowSize:    intPtr(1),
							ColumnSize: intPtr(2),
						},
					},
				},
				{
					BlockId:   strPtr("cell1"),
					BlockType: intPtr(int(BlockTypeTableCell)),
					TableCell: &larkdocx.TableCell{},
					Children:  []string{"text1"},
				},
				{
					BlockId:   strPtr("text1"),
					BlockType: intPtr(int(BlockTypeText)),
					Text: &larkdocx.Text{
						Elements: []*larkdocx.TextElement{
							{TextRun: &larkdocx.TextRun{Content: strPtr("A|B")}},
						},
					},
				},
				{
					BlockId:   strPtr("cell2"),
					BlockType: intPtr(int(BlockTypeTableCell)),
					TableCell: &larkdocx.TableCell{},
					Children:  []string{"text2"},
				},
				{
					BlockId:   strPtr("text2"),
					BlockType: intPtr(int(BlockTypeText)),
					Text: &larkdocx.Text{
						Elements: []*larkdocx.TextElement{
							{TextRun: &larkdocx.TextRun{Content: strPtr("C|D")}},
						},
					},
				},
			},
			want: "| A\\\\|B | C\\\\|D |\n| --- | --- |", // Double backslash because it's escaped twice
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewBlockToMarkdown(tt.blocks, ConvertOptions{})
			got, err := conv.Convert()
			if (err != nil) != tt.wantErr {
				t.Errorf("Convert() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			got = strings.TrimSpace(got)
			if got != tt.want {
				t.Errorf("Convert() got:\n%s\n\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// Helper function to create table cell blocks
func createTableCell(cellID, content string) []*larkdocx.Block {
	textID := cellID + "_text"
	return []*larkdocx.Block{
		{
			BlockId:   strPtr(cellID),
			BlockType: intPtr(int(BlockTypeTableCell)),
			TableCell: &larkdocx.TableCell{},
			Children:  []string{textID},
		},
		{
			BlockId:   strPtr(textID),
			BlockType: intPtr(int(BlockTypeText)),
			Text: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{TextRun: &larkdocx.TextRun{Content: strPtr(content)}},
				},
			},
		},
	}
}

func flattenTableCells(cells [][]*larkdocx.Block) []*larkdocx.Block {
	var result []*larkdocx.Block
	for _, cell := range cells {
		result = append(result, cell...)
	}
	return result
}

// ============================================================================
// getCellTextWithDepth Tests (indirect through table tests + direct tests)
// ============================================================================

func TestGetCellTextWithDepth(t *testing.T) {
	tests := []struct {
		name   string
		blocks []*larkdocx.Block
		want   string
	}{
		{
			name: "cell has no children",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("table1"),
					BlockType: intPtr(int(BlockTypeTable)),
					Table: &larkdocx.Table{
						Cells: []string{"cell1"},
						Property: &larkdocx.TableProperty{
							RowSize:    intPtr(1),
							ColumnSize: intPtr(1),
						},
					},
				},
				{
					BlockId:   strPtr("cell1"),
					BlockType: intPtr(int(BlockTypeTableCell)),
					TableCell: &larkdocx.TableCell{},
					Children:  []string{},
				},
			},
			want: "|  |\n| --- |",
		},
		{
			name: "cell has multiple child blocks",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("table1"),
					BlockType: intPtr(int(BlockTypeTable)),
					Table: &larkdocx.Table{
						Cells: []string{"cell1"},
						Property: &larkdocx.TableProperty{
							RowSize:    intPtr(1),
							ColumnSize: intPtr(1),
						},
					},
				},
				{
					BlockId:   strPtr("cell1"),
					BlockType: intPtr(int(BlockTypeTableCell)),
					TableCell: &larkdocx.TableCell{},
					Children:  []string{"text1", "text2"},
				},
				{
					BlockId:   strPtr("text1"),
					BlockType: intPtr(int(BlockTypeText)),
					Text: &larkdocx.Text{
						Elements: []*larkdocx.TextElement{
							{TextRun: &larkdocx.TextRun{Content: strPtr("Line1")}},
						},
					},
				},
				{
					BlockId:   strPtr("text2"),
					BlockType: intPtr(int(BlockTypeText)),
					Text: &larkdocx.Text{
						Elements: []*larkdocx.TextElement{
							{TextRun: &larkdocx.TextRun{Content: strPtr("Line2")}},
						},
					},
				},
			},
			want: "| Line1<br>Line2 |\n| --- |",
		},
		{
			name: "child block content contains newlines",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("table1"),
					BlockType: intPtr(int(BlockTypeTable)),
					Table: &larkdocx.Table{
						Cells: []string{"cell1"},
						Property: &larkdocx.TableProperty{
							RowSize:    intPtr(1),
							ColumnSize: intPtr(1),
						},
					},
				},
				{
					BlockId:   strPtr("cell1"),
					BlockType: intPtr(int(BlockTypeTableCell)),
					TableCell: &larkdocx.TableCell{},
					Children:  []string{"text1"},
				},
				{
					BlockId:   strPtr("text1"),
					BlockType: intPtr(int(BlockTypeText)),
					Text: &larkdocx.Text{
						Elements: []*larkdocx.TextElement{
							{TextRun: &larkdocx.TextRun{Content: strPtr("Line1\nLine2\nLine3")}},
						},
					},
				},
			},
			want: "| Line1<br>Line2<br>Line3 |\n| --- |",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewBlockToMarkdown(tt.blocks, ConvertOptions{})
			got, err := conv.Convert()
			if err != nil {
				t.Errorf("Convert() error = %v", err)
				return
			}
			got = strings.TrimSpace(got)
			if got != tt.want {
				t.Errorf("Convert() got:\n%s\n\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// ============================================================================
// Callout Tests
// ============================================================================

func TestConvertCallout(t *testing.T) {
	tests := []struct {
		name   string
		blocks []*larkdocx.Block
		want   string
	}{
		{
			name: "callout is nil",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("callout1"),
					BlockType: intPtr(int(BlockTypeCallout)),
					Callout:   nil,
				},
			},
			want: "",
		},
		{
			name: "callout type WARNING (bgColor 2)",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("callout1"),
					BlockType: intPtr(int(BlockTypeCallout)),
					Callout: &larkdocx.Callout{
						BackgroundColor: intPtr(2),
					},
					Children: []string{"text1"},
				},
				createTextBlock("text1", "This is a warning"),
			},
			want: "> [!WARNING]\n> This is a warning",
		},
		{
			name: "callout type CAUTION (bgColor 3)",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("callout1"),
					BlockType: intPtr(int(BlockTypeCallout)),
					Callout: &larkdocx.Callout{
						BackgroundColor: intPtr(3),
					},
					Children: []string{"text1"},
				},
				createTextBlock("text1", "Be cautious"),
			},
			want: "> [!CAUTION]\n> Be cautious",
		},
		{
			name: "callout type TIP (bgColor 4)",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("callout1"),
					BlockType: intPtr(int(BlockTypeCallout)),
					Callout: &larkdocx.Callout{
						BackgroundColor: intPtr(4),
					},
					Children: []string{"text1"},
				},
				createTextBlock("text1", "Helpful tip"),
			},
			want: "> [!TIP]\n> Helpful tip",
		},
		{
			name: "callout type SUCCESS (bgColor 5)",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("callout1"),
					BlockType: intPtr(int(BlockTypeCallout)),
					Callout: &larkdocx.Callout{
						BackgroundColor: intPtr(5),
					},
					Children: []string{"text1"},
				},
				createTextBlock("text1", "Success message"),
			},
			want: "> [!SUCCESS]\n> Success message",
		},
		{
			name: "callout type NOTE (bgColor 6)",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("callout1"),
					BlockType: intPtr(int(BlockTypeCallout)),
					Callout: &larkdocx.Callout{
						BackgroundColor: intPtr(6),
					},
					Children: []string{"text1"},
				},
				createTextBlock("text1", "Note this"),
			},
			want: "> [!NOTE]\n> Note this",
		},
		{
			name: "callout type IMPORTANT (bgColor 7)",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("callout1"),
					BlockType: intPtr(int(BlockTypeCallout)),
					Callout: &larkdocx.Callout{
						BackgroundColor: intPtr(7),
					},
					Children: []string{"text1"},
				},
				createTextBlock("text1", "Very important"),
			},
			want: "> [!IMPORTANT]\n> Very important",
		},
		{
			name: "callout background color is nil - defaults to NOTE",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("callout1"),
					BlockType: intPtr(int(BlockTypeCallout)),
					Callout: &larkdocx.Callout{
						BackgroundColor: nil,
					},
					Children: []string{"text1"},
				},
				createTextBlock("text1", "Default note"),
			},
			want: "> [!NOTE]\n> Default note",
		},
		{
			name: "callout unknown background color - defaults to NOTE",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("callout1"),
					BlockType: intPtr(int(BlockTypeCallout)),
					Callout: &larkdocx.Callout{
						BackgroundColor: intPtr(99),
					},
					Children: []string{"text1"},
				},
				createTextBlock("text1", "Unknown color"),
			},
			want: "> [!NOTE]\n> Unknown color",
		},
		{
			name: "callout has no children",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("callout1"),
					BlockType: intPtr(int(BlockTypeCallout)),
					Callout: &larkdocx.Callout{
						BackgroundColor: intPtr(6),
					},
					Children: []string{},
				},
			},
			want: "> [!NOTE]",
		},
		{
			name: "callout has single text child",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("callout1"),
					BlockType: intPtr(int(BlockTypeCallout)),
					Callout: &larkdocx.Callout{
						BackgroundColor: intPtr(6),
					},
					Children: []string{"text1"},
				},
				createTextBlock("text1", "Single child"),
			},
			want: "> [!NOTE]\n> Single child",
		},
		{
			name: "callout has multiple children",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("callout1"),
					BlockType: intPtr(int(BlockTypeCallout)),
					Callout: &larkdocx.Callout{
						BackgroundColor: intPtr(6),
					},
					Children: []string{"text1", "text2", "text3"},
				},
				createTextBlock("text1", "First line"),
				createTextBlock("text2", "Second line"),
				createTextBlock("text3", "Third line"),
			},
			want: "> [!NOTE]\n> First line\n> Second line\n> Third line",
		},
		{
			name: "callout child content is empty - should skip",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("callout1"),
					BlockType: intPtr(int(BlockTypeCallout)),
					Callout: &larkdocx.Callout{
						BackgroundColor: intPtr(6),
					},
					Children: []string{"text1", "text2"},
				},
				createTextBlock("text1", ""),
				createTextBlock("text2", "Non-empty"),
			},
			want: "> [!NOTE]\n> Non-empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewBlockToMarkdown(tt.blocks, ConvertOptions{})
			got, err := conv.Convert()
			if err != nil {
				t.Errorf("Convert() error = %v", err)
				return
			}
			got = strings.TrimSpace(got)
			if got != tt.want {
				t.Errorf("Convert() got:\n%s\n\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// ============================================================================
// Grid Tests
// ============================================================================

func TestConvertGrid(t *testing.T) {
	tests := []struct {
		name   string
		blocks []*larkdocx.Block
		want   string
	}{
		{
			name: "grid is nil",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("grid1"),
					BlockType: intPtr(int(BlockTypeGrid)),
					Grid:      nil,
				},
			},
			want: "",
		},
		{
			name: "grid has no children",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("grid1"),
					BlockType: intPtr(int(BlockTypeGrid)),
					Grid: &larkdocx.Grid{
						ColumnSize: intPtr(2),
					},
					Children: []string{},
				},
			},
			want: "",
		},
		{
			name: "normal grid with 2 columns",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("grid1"),
					BlockType: intPtr(int(BlockTypeGrid)),
					Grid: &larkdocx.Grid{
						ColumnSize: intPtr(2),
					},
					Children: []string{"col1", "col2"},
				},
				{
					BlockId:    strPtr("col1"),
					BlockType:  intPtr(int(BlockTypeGridColumn)),
					GridColumn: &larkdocx.GridColumn{},
					Children:   []string{"text1"},
				},
				createTextBlock("text1", "Column 1 content"),
				{
					BlockId:    strPtr("col2"),
					BlockType:  intPtr(int(BlockTypeGridColumn)),
					GridColumn: &larkdocx.GridColumn{},
					Children:   []string{"text2"},
				},
				createTextBlock("text2", "Column 2 content"),
			},
			want: "Column 1 content\nColumn 2 content",
		},
		{
			name: "grid child is not GridColumn - should skip",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("grid1"),
					BlockType: intPtr(int(BlockTypeGrid)),
					Grid: &larkdocx.Grid{
						ColumnSize: intPtr(2),
					},
					Children: []string{"text1", "col2"},
				},
				createTextBlock("text1", "Not a column"),
				{
					BlockId:    strPtr("col2"),
					BlockType:  intPtr(int(BlockTypeGridColumn)),
					GridColumn: &larkdocx.GridColumn{},
					Children:   []string{"text2"},
				},
				createTextBlock("text2", "Column 2 content"),
			},
			want: "Column 2 content",
		},
		{
			name: "grid column is nil",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("grid1"),
					BlockType: intPtr(int(BlockTypeGrid)),
					Grid: &larkdocx.Grid{
						ColumnSize: intPtr(1),
					},
					Children: []string{"col1"},
				},
				{
					BlockId:    strPtr("col1"),
					BlockType:  intPtr(int(BlockTypeGridColumn)),
					GridColumn: nil,
					Children:   []string{"text1"},
				},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewBlockToMarkdown(tt.blocks, ConvertOptions{})
			got, err := conv.Convert()
			if err != nil {
				t.Errorf("Convert() error = %v", err)
				return
			}
			got = strings.TrimSpace(got)
			if got != tt.want {
				t.Errorf("Convert() got:\n%s\n\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// ============================================================================
// QuoteContainer Tests
// ============================================================================

func TestConvertQuoteContainer(t *testing.T) {
	tests := []struct {
		name   string
		blocks []*larkdocx.Block
		want   string
	}{
		{
			name: "quote container is nil",
			blocks: []*larkdocx.Block{
				{
					BlockId:        strPtr("qc1"),
					BlockType:      intPtr(int(BlockTypeQuoteContainer)),
					QuoteContainer: nil,
				},
			},
			want: "",
		},
		{
			name: "quote container has no children",
			blocks: []*larkdocx.Block{
				{
					BlockId:        strPtr("qc1"),
					BlockType:      intPtr(int(BlockTypeQuoteContainer)),
					QuoteContainer: &larkdocx.QuoteContainer{},
					Children:       []string{},
				},
			},
			want: "",
		},
		{
			name: "quote container has single text child",
			blocks: []*larkdocx.Block{
				{
					BlockId:        strPtr("qc1"),
					BlockType:      intPtr(int(BlockTypeQuoteContainer)),
					QuoteContainer: &larkdocx.QuoteContainer{},
					Children:       []string{"text1"},
				},
				createTextBlock("text1", "Quoted text"),
			},
			want: "> Quoted text",
		},
		{
			name: "quote container has multiple children",
			blocks: []*larkdocx.Block{
				{
					BlockId:        strPtr("qc1"),
					BlockType:      intPtr(int(BlockTypeQuoteContainer)),
					QuoteContainer: &larkdocx.QuoteContainer{},
					Children:       []string{"text1", "text2"},
				},
				createTextBlock("text1", "First paragraph"),
				createTextBlock("text2", "Second paragraph"),
			},
			want: "> First paragraph\n> Second paragraph",
		},
		{
			name: "quote container child has multiple lines",
			blocks: []*larkdocx.Block{
				{
					BlockId:        strPtr("qc1"),
					BlockType:      intPtr(int(BlockTypeQuoteContainer)),
					QuoteContainer: &larkdocx.QuoteContainer{},
					Children:       []string{"text1"},
				},
				{
					BlockId:   strPtr("text1"),
					BlockType: intPtr(int(BlockTypeText)),
					Text: &larkdocx.Text{
						Elements: []*larkdocx.TextElement{
							{TextRun: &larkdocx.TextRun{Content: strPtr("Line1\nLine2\nLine3")}},
						},
					},
				},
			},
			want: "> Line1\n> Line2\n> Line3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewBlockToMarkdown(tt.blocks, ConvertOptions{})
			got, err := conv.Convert()
			if err != nil {
				t.Errorf("Convert() error = %v", err)
				return
			}
			got = strings.TrimSpace(got)
			if got != tt.want {
				t.Errorf("Convert() got:\n%s\n\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// ============================================================================
// Image Tests (without external API calls)
// ============================================================================

func TestConvertImage(t *testing.T) {
	tests := []struct {
		name   string
		blocks []*larkdocx.Block
		opts   ConvertOptions
		want   string
	}{
		{
			name: "image is nil",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("img1"),
					BlockType: intPtr(int(BlockTypeImage)),
					Image:     nil,
				},
			},
			want: "",
		},
		{
			name: "image token is nil",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("img1"),
					BlockType: intPtr(int(BlockTypeImage)),
					Image: &larkdocx.Image{
						Token: nil,
					},
				},
			},
			want: "![image]()",
		},
		{
			name: "image token present but DownloadImages is false",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("img1"),
					BlockType: intPtr(int(BlockTypeImage)),
					Image: &larkdocx.Image{
						Token: strPtr("test_token_123"),
					},
				},
			},
			opts: ConvertOptions{
				DownloadImages: false,
			},
			want: "![image](feishu://media/test_token_123)",
		},
		{
			name: "image has child block for alt text",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("img1"),
					BlockType: intPtr(int(BlockTypeImage)),
					Image: &larkdocx.Image{
						Token: strPtr("test_token_456"),
					},
					Children: []string{"text1"},
				},
				createTextBlock("text1", "Custom alt text"),
			},
			opts: ConvertOptions{
				DownloadImages: false,
			},
			// Note: Image child blocks are not marked as childBlockIDs, so they appear separately
			want: "![Custom alt text](feishu://media/test_token_456)\n\nCustom alt text",
		},
		{
			name: "image no children uses default alt",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("img1"),
					BlockType: intPtr(int(BlockTypeImage)),
					Image: &larkdocx.Image{
						Token: strPtr("test_token_789"),
					},
					Children: []string{},
				},
			},
			opts: ConvertOptions{
				DownloadImages: false,
			},
			want: "![image](feishu://media/test_token_789)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewBlockToMarkdown(tt.blocks, tt.opts)
			got, err := conv.Convert()
			if err != nil {
				t.Errorf("Convert() error = %v", err)
				return
			}
			got = strings.TrimSpace(got)
			if got != tt.want {
				t.Errorf("Convert() got:\n%s\n\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// ============================================================================
// Bullet/Ordered Additional Branch Tests
// ============================================================================

func TestConvertBulletNil(t *testing.T) {
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("bullet1"),
			BlockType: intPtr(int(BlockTypeBullet)),
			Bullet:    nil,
		},
	}
	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	got, err := conv.Convert()
	if err != nil {
		t.Errorf("Convert() error = %v", err)
		return
	}
	got = strings.TrimSpace(got)
	if got != "" {
		t.Errorf("Convert() got %q, want empty", got)
	}
}

func TestConvertOrderedNil(t *testing.T) {
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("ordered1"),
			BlockType: intPtr(int(BlockTypeOrdered)),
			Ordered:   nil,
		},
	}
	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	got, err := conv.Convert()
	if err != nil {
		t.Errorf("Convert() error = %v", err)
		return
	}
	got = strings.TrimSpace(got)
	if got != "" {
		t.Errorf("Convert() got %q, want empty", got)
	}
}

func TestConvertOrderedWithNestedSublist(t *testing.T) {
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("ordered1"),
			BlockType: intPtr(int(BlockTypeOrdered)),
			Ordered: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{TextRun: &larkdocx.TextRun{Content: strPtr("Item 1")}},
				},
			},
			Children: []string{"ordered2"},
		},
		{
			BlockId:   strPtr("ordered2"),
			BlockType: intPtr(int(BlockTypeOrdered)),
			Ordered: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{TextRun: &larkdocx.TextRun{Content: strPtr("Subitem 1.1")}},
				},
			},
		},
	}
	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	got, err := conv.Convert()
	if err != nil {
		t.Errorf("Convert() error = %v", err)
	}
	got = strings.TrimSpace(got)
	want := "1. Item 1\n  1. Subitem 1.1"
	if got != want {
		t.Errorf("Convert() got:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestConvertOrderedWithCustomSequence(t *testing.T) {
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("ordered1"),
			BlockType: intPtr(int(BlockTypeOrdered)),
			Ordered: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{TextRun: &larkdocx.TextRun{Content: strPtr("Item 5")}},
				},
				Style: &larkdocx.TextStyle{
					Sequence: strPtr("5"),
				},
			},
		},
	}
	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	got, err := conv.Convert()
	if err != nil {
		t.Errorf("Convert() error = %v", err)
	}
	got = strings.TrimSpace(got)
	want := "5. Item 5"
	if got != want {
		t.Errorf("Convert() got %q, want %q", got, want)
	}
}

func TestConvertOrderedWithAutoSequence(t *testing.T) {
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("ordered1"),
			BlockType: intPtr(int(BlockTypeOrdered)),
			Ordered: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{TextRun: &larkdocx.TextRun{Content: strPtr("Auto item")}},
				},
				Style: &larkdocx.TextStyle{
					Sequence: strPtr("auto"),
				},
			},
		},
	}
	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	got, err := conv.Convert()
	if err != nil {
		t.Errorf("Convert() error = %v", err)
	}
	got = strings.TrimSpace(got)
	want := "1. Auto item"
	if got != want {
		t.Errorf("Convert() got %q, want %q", got, want)
	}
}

func TestConvertOrderedWithEmptySequence(t *testing.T) {
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("ordered1"),
			BlockType: intPtr(int(BlockTypeOrdered)),
			Ordered: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{TextRun: &larkdocx.TextRun{Content: strPtr("Empty seq")}},
				},
				Style: &larkdocx.TextStyle{
					Sequence: strPtr(""),
				},
			},
		},
	}
	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	got, err := conv.Convert()
	if err != nil {
		t.Errorf("Convert() error = %v", err)
	}
	got = strings.TrimSpace(got)
	want := "1. Empty seq"
	if got != want {
		t.Errorf("Convert() got %q, want %q", got, want)
	}
}

func TestConvertBulletWithNestedSublist(t *testing.T) {
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("bullet1"),
			BlockType: intPtr(int(BlockTypeBullet)),
			Bullet: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{TextRun: &larkdocx.TextRun{Content: strPtr("Item 1")}},
				},
			},
			Children: []string{"bullet2"},
		},
		{
			BlockId:   strPtr("bullet2"),
			BlockType: intPtr(int(BlockTypeBullet)),
			Bullet: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{TextRun: &larkdocx.TextRun{Content: strPtr("Subitem 1.1")}},
				},
			},
		},
	}
	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	got, err := conv.Convert()
	if err != nil {
		t.Errorf("Convert() error = %v", err)
	}
	got = strings.TrimSpace(got)
	// Verify indentation
	if !strings.Contains(got, "- Item 1") {
		t.Errorf("Missing top-level bullet")
	}
	if !strings.Contains(got, "  - Subitem 1.1") {
		t.Errorf("Missing indented subitem")
	}
}

// ============================================================================
// Text and Equation Nil Tests
// ============================================================================

func TestConvertTextNil(t *testing.T) {
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("text1"),
			BlockType: intPtr(int(BlockTypeText)),
			Text:      nil,
		},
	}
	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	got, err := conv.Convert()
	if err != nil {
		t.Errorf("Convert() error = %v", err)
	}
	got = strings.TrimSpace(got)
	if got != "" {
		t.Errorf("Convert() got %q, want empty", got)
	}
}

func TestConvertTextElementsNil(t *testing.T) {
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("text1"),
			BlockType: intPtr(int(BlockTypeText)),
			Text: &larkdocx.Text{
				Elements: nil,
			},
		},
	}
	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	got, err := conv.Convert()
	if err != nil {
		t.Errorf("Convert() error = %v", err)
	}
	got = strings.TrimSpace(got)
	if got != "" {
		t.Errorf("Convert() got %q, want empty", got)
	}
}

func TestConvertEquationNil(t *testing.T) {
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("eq1"),
			BlockType: intPtr(int(BlockTypeEquation)),
			Equation:  nil,
		},
	}
	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	got, err := conv.Convert()
	if err != nil {
		t.Errorf("Convert() error = %v", err)
	}
	got = strings.TrimSpace(got)
	if got != "" {
		t.Errorf("Convert() got %q, want empty", got)
	}
}

// ============================================================================
// AddOns Container Block Tests
// ============================================================================

func TestConvertAddOns(t *testing.T) {
	tests := []struct {
		name   string
		blocks []*larkdocx.Block
		want   string
	}{
		{
			name: "addons has no children",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("addons1"),
					BlockType: intPtr(int(BlockTypeAddOns)),
					AddOns:    &larkdocx.AddOns{},
					Children:  []string{},
				},
			},
			want: "",
		},
		{
			name: "addons has children - should recursively expand",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("addons1"),
					BlockType: intPtr(int(BlockTypeAddOns)),
					AddOns:    &larkdocx.AddOns{},
					Children:  []string{"text1", "text2"},
				},
				createTextBlock("text1", "First child"),
				createTextBlock("text2", "Second child"),
			},
			want: "First child\nSecond child",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewBlockToMarkdown(tt.blocks, ConvertOptions{})
			got, err := conv.Convert()
			if err != nil {
				t.Errorf("Convert() error = %v", err)
				return
			}
			got = strings.TrimSpace(got)
			if got != tt.want {
				t.Errorf("Convert() got:\n%s\n\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// ============================================================================
// File/Bitable/Sheet/ChatCard/Diagram/Board/Iframe/MindNote/WikiCatalog Tests
// ============================================================================

func TestConvertFile(t *testing.T) {
	tests := []struct {
		name   string
		blocks []*larkdocx.Block
		want   string
	}{
		{
			name: "file is nil",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("file1"),
					BlockType: intPtr(int(BlockTypeFile)),
					File:      nil,
				},
			},
			want: "",
		},
		{
			name: "file with token and name",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("file1"),
					BlockType: intPtr(int(BlockTypeFile)),
					File: &larkdocx.File{
						Token: strPtr("file_abc123"),
						Name:  strPtr("document.pdf"),
					},
				},
			},
			want: "[document.pdf](feishu://file/file_abc123)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewBlockToMarkdown(tt.blocks, ConvertOptions{})
			got, err := conv.Convert()
			if err != nil {
				t.Errorf("Convert() error = %v", err)
				return
			}
			got = strings.TrimSpace(got)
			if got != tt.want {
				t.Errorf("Convert() got:\n%s\n\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestConvertBitable(t *testing.T) {
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("bitable1"),
			BlockType: intPtr(int(BlockTypeBitable)),
			Bitable: &larkdocx.Bitable{
				Token: strPtr("bitable_token123"),
			},
		},
	}

	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	got, err := conv.Convert()
	if err != nil {
		t.Errorf("Convert() error = %v", err)
		return
	}
	got = strings.TrimSpace(got)
	want := "[Bitable: bitable_token123](https://feishu.cn/base/bitable_token123)"
	if got != want {
		t.Errorf("Convert() got:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestConvertSheet(t *testing.T) {
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("sheet1"),
			BlockType: intPtr(int(BlockTypeSheet)),
			Sheet: &larkdocx.Sheet{
				Token: strPtr("sheet_token456"),
			},
		},
	}

	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	got, err := conv.Convert()
	if err != nil {
		t.Errorf("Convert() error = %v", err)
		return
	}
	got = strings.TrimSpace(got)
	want := "[Sheet: sheet_token456](https://feishu.cn/sheets/sheet_token456)"
	if got != want {
		t.Errorf("Convert() got:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestConvertChatCard(t *testing.T) {
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("chat1"),
			BlockType: intPtr(int(BlockTypeChatCard)),
			ChatCard: &larkdocx.ChatCard{
				ChatId: strPtr("oc_abc123xyz"),
			},
		},
	}

	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	got, err := conv.Convert()
	if err != nil {
		t.Errorf("Convert() error = %v", err)
		return
	}
	got = strings.TrimSpace(got)
	want := "[ChatCard: oc_abc123xyz]"
	if got != want {
		t.Errorf("Convert() got:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestConvertDiagram(t *testing.T) {
	tests := []struct {
		name   string
		blocks []*larkdocx.Block
		want   string
	}{
		{
			name: "diagram is nil",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("diagram1"),
					BlockType: intPtr(int(BlockTypeDiagram)),
					Diagram:   nil,
				},
			},
			want: "",
		},
		{
			name: "diagram type flowchart (1)",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("diagram1"),
					BlockType: intPtr(int(BlockTypeDiagram)),
					Diagram: &larkdocx.Diagram{
						DiagramType: intPtr(1),
					},
				},
			},
			want: "```mermaid\n% Feishu Flowchart Diagram (type: 1)\n% Note: Mermaid source code is not accessible via API\n```",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewBlockToMarkdown(tt.blocks, ConvertOptions{})
			got, err := conv.Convert()
			if err != nil {
				t.Errorf("Convert() error = %v", err)
				return
			}
			got = strings.TrimSpace(got)
			if got != tt.want {
				t.Errorf("Convert() got:\n%s\n\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestConvertBoard(t *testing.T) {
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("board1"),
			BlockType: intPtr(int(BlockTypeBoard)),
			Board: &larkdocx.Board{
				Token: strPtr("board_token789"),
			},
		},
	}

	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	got, err := conv.Convert()
	if err != nil {
		t.Errorf("Convert() error = %v", err)
		return
	}
	got = strings.TrimSpace(got)
	want := "[画板/Whiteboard](feishu://board/board_token789)"
	if got != want {
		t.Errorf("Convert() got:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestConvertIframe(t *testing.T) {
	tests := []struct {
		name   string
		blocks []*larkdocx.Block
		want   string
	}{
		{
			name: "iframe is nil",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("iframe1"),
					BlockType: intPtr(int(BlockTypeIframe)),
					Iframe:    nil,
				},
			},
			want: "",
		},
		{
			name: "iframe with URL",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("iframe1"),
					BlockType: intPtr(int(BlockTypeIframe)),
					Iframe: &larkdocx.Iframe{
						Component: &larkdocx.IframeComponent{
							Url: strPtr("https://example.com/embed"),
						},
					},
				},
			},
			want: `<iframe src="https://example.com/embed" sandbox="allow-scripts allow-same-origin allow-presentation allow-forms allow-popups" allowfullscreen frameborder="0" style="width:100%; min-height:400px;"></iframe>`,
		},
		{
			name: "iframe URL is empty",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("iframe1"),
					BlockType: intPtr(int(BlockTypeIframe)),
					Iframe: &larkdocx.Iframe{
						Component: &larkdocx.IframeComponent{
							Url: strPtr(""),
						},
					},
				},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewBlockToMarkdown(tt.blocks, ConvertOptions{})
			got, err := conv.Convert()
			if err != nil {
				t.Errorf("Convert() error = %v", err)
				return
			}
			got = strings.TrimSpace(got)
			if got != tt.want {
				t.Errorf("Convert() got:\n%s\n\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestConvertMindNote(t *testing.T) {
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("mind1"),
			BlockType: intPtr(int(BlockTypeMindNote)),
			Mindnote: &larkdocx.Mindnote{
				Token: strPtr("mindnote_token999"),
			},
		},
	}

	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	got, err := conv.Convert()
	if err != nil {
		t.Errorf("Convert() error = %v", err)
		return
	}
	got = strings.TrimSpace(got)
	want := "[思维导图/MindNote](feishu://mindnote/mindnote_token999)"
	if got != want {
		t.Errorf("Convert() got:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestConvertWikiCatalog(t *testing.T) {
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("wiki1"),
			BlockType: intPtr(int(BlockTypeWikiCatalog)),
		},
	}

	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	got, err := conv.Convert()
	if err != nil {
		t.Errorf("Convert() error = %v", err)
		return
	}
	got = strings.TrimSpace(got)
	want := "[Wiki 目录 - 使用 'wiki nodes <space_id> --parent <node_token>' 获取子节点列表]"
	if got != want {
		t.Errorf("Convert() got:\n%s\n\nwant:\n%s", got, want)
	}
}

// ============================================================================
// AgendaItemContent/SyncSource/LinkPreview Child Expansion Tests
// ============================================================================

func TestConvertAgendaItemContent(t *testing.T) {
	tests := []struct {
		name   string
		blocks []*larkdocx.Block
		want   string
	}{
		{
			name: "no children",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("agenda1"),
					BlockType: intPtr(int(BlockTypeAgendaItemContent)),
					Children:  nil,
				},
			},
			want: "",
		},
		{
			name: "has children",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("agenda1"),
					BlockType: intPtr(int(BlockTypeAgendaItemContent)),
					Children:  []string{"text1"},
				},
				createTextBlock("text1", "Agenda content"),
			},
			want: "Agenda content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewBlockToMarkdown(tt.blocks, ConvertOptions{})
			got, err := conv.Convert()
			if err != nil {
				t.Errorf("Convert() error = %v", err)
				return
			}
			got = strings.TrimSpace(got)
			if got != tt.want {
				t.Errorf("Convert() got:\n%s\n\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestConvertSyncSource(t *testing.T) {
	tests := []struct {
		name   string
		blocks []*larkdocx.Block
		want   string
	}{
		{
			name: "no children",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("sync1"),
					BlockType: intPtr(int(BlockTypeSyncSource)),
					Children:  nil,
				},
			},
			want: "",
		},
		{
			name: "has children",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("sync1"),
					BlockType: intPtr(int(BlockTypeSyncSource)),
					Children:  []string{"text1"},
				},
				createTextBlock("text1", "Sync content"),
			},
			want: "Sync content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewBlockToMarkdown(tt.blocks, ConvertOptions{})
			got, err := conv.Convert()
			if err != nil {
				t.Errorf("Convert() error = %v", err)
				return
			}
			got = strings.TrimSpace(got)
			if got != tt.want {
				t.Errorf("Convert() got:\n%s\n\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestConvertLinkPreview(t *testing.T) {
	tests := []struct {
		name   string
		blocks []*larkdocx.Block
		want   string
	}{
		{
			name: "no children",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("lp1"),
					BlockType: intPtr(int(BlockTypeLinkPreview)),
					Children:  nil,
				},
			},
			want: "[链接预览]",
		},
		{
			name: "has children",
			blocks: []*larkdocx.Block{
				{
					BlockId:   strPtr("lp1"),
					BlockType: intPtr(int(BlockTypeLinkPreview)),
					Children:  []string{"text1"},
				},
				createTextBlock("text1", "Link description"),
			},
			want: "Link description",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewBlockToMarkdown(tt.blocks, ConvertOptions{})
			got, err := conv.Convert()
			if err != nil {
				t.Errorf("Convert() error = %v", err)
				return
			}
			got = strings.TrimSpace(got)
			if got != tt.want {
				t.Errorf("Convert() got:\n%s\n\nwant:\n%s", got, tt.want)
			}
		})
	}
}
