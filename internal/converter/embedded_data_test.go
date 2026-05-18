package converter

import (
	"fmt"
	"strings"
	"testing"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
)

type stubEmbeddedFetcher struct {
	sheetData   *EmbeddedTableData
	bitableData *EmbeddedTableData
	err         error
}

func (s stubEmbeddedFetcher) FetchSheet(token, sheetID string, maxRows, maxCols int, userAccessToken string) (*EmbeddedTableData, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.sheetData, nil
}

func (s stubEmbeddedFetcher) FetchBitable(baseToken, tableID string, maxRows, maxCols int, userAccessToken string) (*EmbeddedTableData, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.bitableData, nil
}

func TestConvertEmbeddedSheetToMarkdownTable(t *testing.T) {
	blockType := int(BlockTypeSheet)
	block := &larkdocx.Block{
		BlockType: &blockType,
		Sheet: &larkdocx.Sheet{
			Token: strPtr("sht123_sheet456"),
		},
	}
	conv := NewBlockToMarkdown([]*larkdocx.Block{block}, ConvertOptions{
		EmbeddedTableFetcher: stubEmbeddedFetcher{sheetData: &EmbeddedTableData{
			Headers:       []string{"姓名", "备注"},
			Rows:          [][]string{{"张三", "A|B\nC"}},
			TruncatedRows: 2,
		}},
	})

	got, err := conv.Convert()
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	wantParts := []string{
		"| 姓名 | 备注 |",
		"| --- | --- |",
		"| 张三 | A\\|B<br>C |",
		"> 还有 2 行未导出（设置 --max-embedded-rows 调整）",
	}
	for _, want := range wantParts {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in:\n%s", want, got)
		}
	}
}

func TestConvertEmbeddedBitableToMarkdownTable(t *testing.T) {
	blockType := int(BlockTypeBitable)
	viewType := 1
	block := &larkdocx.Block{
		BlockType: &blockType,
		Bitable: &larkdocx.Bitable{
			Token:    strPtr("bas123_tbl456"),
			ViewType: &viewType,
		},
	}
	conv := NewBlockToMarkdown([]*larkdocx.Block{block}, ConvertOptions{
		EmbeddedTableFetcher: stubEmbeddedFetcher{bitableData: &EmbeddedTableData{
			Headers: []string{"任务", "状态"},
			Rows:    [][]string{{"实现导出", "完成"}},
		}},
	})

	got, err := conv.Convert()
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	if !strings.Contains(got, "| 任务 | 状态 |") || !strings.Contains(got, "| 实现导出 | 完成 |") {
		t.Fatalf("expected markdown table, got:\n%s", got)
	}
}

func TestConvertEmbeddedTableFallsBackOnFetchError(t *testing.T) {
	blockType := int(BlockTypeSheet)
	rows := 10
	cols := 3
	block := &larkdocx.Block{
		BlockType: &blockType,
		Sheet: &larkdocx.Sheet{
			Token:      strPtr("sht123_sheet456"),
			RowSize:    &rows,
			ColumnSize: &cols,
		},
	}
	conv := NewBlockToMarkdown([]*larkdocx.Block{block}, ConvertOptions{
		EmbeddedTableFetcher: stubEmbeddedFetcher{err: fmt.Errorf("no permission")},
	})

	got, err := conv.Convert()
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	want := `<sheet token="sht123" id="sheet456" rows="10" cols="3"/>`
	if !strings.Contains(got, want) {
		t.Fatalf("expected fallback %q, got:\n%s", want, got)
	}
}

func TestConvertNonGridBitableKeepsPlaceholder(t *testing.T) {
	blockType := int(BlockTypeBitable)
	viewType := 2
	block := &larkdocx.Block{
		BlockType: &blockType,
		Bitable: &larkdocx.Bitable{
			Token:    strPtr("bas123_tbl456"),
			ViewType: &viewType,
		},
	}
	conv := NewBlockToMarkdown([]*larkdocx.Block{block}, ConvertOptions{
		EmbeddedTableFetcher: stubEmbeddedFetcher{bitableData: &EmbeddedTableData{
			Headers: []string{"任务"},
			Rows:    [][]string{{"不应展开"}},
		}},
	})

	got, err := conv.Convert()
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	want := `<bitable token="bas123_tbl456" view="kanban"/>`
	if !strings.Contains(got, want) {
		t.Fatalf("expected placeholder %q, got:\n%s", want, got)
	}
}
