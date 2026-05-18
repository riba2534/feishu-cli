package converter

import (
	"fmt"
	"reflect"
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

func TestEmbeddedCellStringDefersMarkdownTableEscaping(t *testing.T) {
	got := embeddedCellString("A|B\nC")
	if got != "A|B\nC" {
		t.Fatalf("embeddedCellString() = %q, want raw trimmed text", got)
	}

	table := tableDataFromRows([][]string{{"标题|列"}, {got}}, 10, 10)
	md := formatMarkdownTable(table.Headers, table.Rows, table.TruncatedRows)
	if !strings.Contains(md, "| 标题\\|列 |") {
		t.Fatalf("expected header escaped once, got:\n%s", md)
	}
	if !strings.Contains(md, "| A\\|B<br>C |") {
		t.Fatalf("expected cell escaped once, got:\n%s", md)
	}
	if strings.Contains(md, `A\\|B`) {
		t.Fatalf("cell was double escaped:\n%s", md)
	}
}

func TestFetchBitableHeadersPaginates(t *testing.T) {
	oldBaseV3Call := baseV3Call
	defer func() { baseV3Call = oldBaseV3Call }()

	calls := 0
	baseV3Call = func(method, path string, params map[string]any, body any, userAccessToken string) (map[string]any, error) {
		calls++
		if method != "GET" {
			t.Fatalf("method = %s, want GET", method)
		}
		if userAccessToken != "user-token" {
			t.Fatalf("userAccessToken = %q, want user-token", userAccessToken)
		}
		switch calls {
		case 1:
			if _, ok := params["page_token"]; ok {
				t.Fatalf("first request should not include page_token: %#v", params)
			}
			return map[string]any{
				"items": []any{
					map[string]any{"field_name": "一列"},
					map[string]any{"field_name": "二列"},
				},
				"has_more":   true,
				"page_token": "next-page",
			}, nil
		case 2:
			if params["page_token"] != "next-page" {
				t.Fatalf("page_token = %#v, want next-page", params["page_token"])
			}
			return map[string]any{
				"items": []any{
					map[string]any{"field_name": "三列"},
				},
				"has_more": false,
			}, nil
		default:
			t.Fatalf("unexpected extra call %d", calls)
		}
		return nil, nil
	}

	got, err := fetchBitableHeaders("base", "table", 10, "user-token")
	if err != nil {
		t.Fatalf("fetchBitableHeaders() error = %v", err)
	}
	want := []string{"一列", "二列", "三列"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fetchBitableHeaders() = %#v, want %#v", got, want)
	}
}
