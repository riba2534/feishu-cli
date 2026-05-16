package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

func TestExtractGFMTables_Standard(t *testing.T) {
	md := `# Header

Some prose.

| Name | Age | City |
| ---- | --- | ---- |
| Alice | 30 | NYC |
| Bob | 25 | LA |

More prose.
`
	tables := extractGFMTables(md)
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	want := [][]string{
		{"Name", "Age", "City"},
		{"Alice", "30", "NYC"},
		{"Bob", "25", "LA"},
	}
	if !reflect.DeepEqual(tables[0], want) {
		t.Errorf("table mismatch:\n got: %v\nwant: %v", tables[0], want)
	}
}

func TestExtractGFMTables_NoLeadingTrailingPipe(t *testing.T) {
	md := `Name | Age
--- | ---
Alice | 30
Bob | 25
`
	tables := extractGFMTables(md)
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	want := [][]string{
		{"Name", "Age"},
		{"Alice", "30"},
		{"Bob", "25"},
	}
	if !reflect.DeepEqual(tables[0], want) {
		t.Errorf("table mismatch: got %v want %v", tables[0], want)
	}
}

func TestExtractGFMTables_AlignmentColons(t *testing.T) {
	md := `| L | C | R |
| :--- | :---: | ---: |
| a | b | c |
`
	tables := extractGFMTables(md)
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if !reflect.DeepEqual(tables[0], [][]string{{"L", "C", "R"}, {"a", "b", "c"}}) {
		t.Errorf("alignment colons should be ignored, got %v", tables[0])
	}
}

func TestExtractGFMTables_EscapedPipe(t *testing.T) {
	md := `| key | value |
| --- | --- |
| price | a \| b |
| range | 1\|2\|3 |
`
	tables := extractGFMTables(md)
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	want := [][]string{
		{"key", "value"},
		{"price", "a | b"},
		{"range", "1|2|3"},
	}
	if !reflect.DeepEqual(tables[0], want) {
		t.Errorf("escaped pipe mismatch: got %v want %v", tables[0], want)
	}
}

func TestExtractGFMTables_Multiple(t *testing.T) {
	md := `# A

| a | b |
| - | - |
| 1 | 2 |

text in between

| x | y |
| - | - |
| 9 | 8 |
| 7 | 6 |
`
	tables := extractGFMTables(md)
	if len(tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(tables))
	}
	if !reflect.DeepEqual(tables[0], [][]string{{"a", "b"}, {"1", "2"}}) {
		t.Errorf("table 0 mismatch: %v", tables[0])
	}
	if !reflect.DeepEqual(tables[1], [][]string{{"x", "y"}, {"9", "8"}, {"7", "6"}}) {
		t.Errorf("table 1 mismatch: %v", tables[1])
	}
}

func TestExtractGFMTables_None(t *testing.T) {
	md := `# Just text

no tables here

just | a single | pipe but no separator line
`
	tables := extractGFMTables(md)
	if len(tables) != 0 {
		t.Fatalf("expected 0 tables, got %d", len(tables))
	}
}

func TestExtractGFMTables_SkipsFencedCodeBlock(t *testing.T) {
	md := "```markdown\n| fake | table |\n| --- | --- |\n| no | import |\n```\n\n| real | table |\n| --- | --- |\n| yes | import |\n"
	tables := extractGFMTables(md)
	if len(tables) != 1 {
		t.Fatalf("expected 1 real table, got %d: %#v", len(tables), tables)
	}
	want := [][]string{{"real", "table"}, {"yes", "import"}}
	if !reflect.DeepEqual(tables[0], want) {
		t.Errorf("table mismatch: got %v want %v", tables[0], want)
	}
}

func TestExtractGFMTables_HeaderSeparatorColumnMismatch(t *testing.T) {
	md := `| a | b | c |
| --- | --- |
| 1 | 2 | 3 |
`
	tables := extractGFMTables(md)
	if len(tables) != 0 {
		t.Fatalf("expected mismatched header/separator not to be recognized, got %#v", tables)
	}
}

func TestExtractGFMTables_RaggedRowsPad(t *testing.T) {
	md := `| a | b | c |
| - | - | - |
| 1 | 2 |
| 3 | 4 | 5 | 6 |
`
	tables := extractGFMTables(md)
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	want := [][]string{
		{"a", "b", "c"},
		{"1", "2", ""},  // 短行补空
		{"3", "4", "5"}, // 长行截断到 colCount=3
	}
	if !reflect.DeepEqual(tables[0], want) {
		t.Errorf("ragged rows mismatch:\n got: %v\nwant: %v", tables[0], want)
	}
}

func TestExtractGFMTables_EmptyCells(t *testing.T) {
	md := `| a | b | c |
| - | - | - |
|  |  |  |
| x |  | y |
`
	tables := extractGFMTables(md)
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	want := [][]string{
		{"a", "b", "c"},
		{"", "", ""},
		{"x", "", "y"},
	}
	if !reflect.DeepEqual(tables[0], want) {
		t.Errorf("empty cells mismatch: got %v", tables[0])
	}
}

func TestLooksLikeTableLine(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "", false},
		{"only whitespace", "   ", false},
		{"plain text", "hello world", false},
		{"single pipe", "|", true},
		{"with pipe", "a | b", true},
		{"escaped pipe only", `a \| b`, false},
		{"escaped + real pipe", `a \| b | c`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := looksLikeTableLine(tt.in)
			if got != tt.want {
				t.Errorf("looksLikeTableLine(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsSeparatorLine(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"standard", "| --- | --- |", true},
		{"with alignment", "| :--- | :---: | ---: |", true},
		{"single col", "|---|", true},
		{"no pipe", "---", true}, // splitTableRow 一行无管道时返回 1 个 cell
		{"data row not separator", "| a | b |", false},
		{"mixed cells", "| --- | a |", false},
		{"all empty cells", "|  |  |", false},
		{"empty string", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSeparatorLine(tt.in)
			if got != tt.want {
				t.Errorf("isSeparatorLine(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsSeparatorCell(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"---", true},
		{"-", true},
		{":---", true},
		{"---:", true},
		{":---:", true},
		{":-:", true},
		{"::", false},   // 没有 -
		{"---a", false}, // 含字母
		{"", false},     // 空
		{":", false},    // 只有冒号
		{"::---", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := isSeparatorCell(tt.in)
			if got != tt.want {
				t.Errorf("isSeparatorCell(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestSplitTableRow(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"both pipes", "| a | b | c |", []string{"a", "b", "c"}},
		{"no pipes at boundary", "a | b | c", []string{"a", "b", "c"}},
		{"trim whitespace", "|  a   |   b  |", []string{"a", "b"}},
		{"empty cells", "|  | x |", []string{"", "x"}},
		{"escaped pipe", `| a \| b | c |`, []string{"a | b", "c"}},
		{"multiple escaped", `| 1\|2\|3 | x |`, []string{"1|2|3", "x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitTableRow(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitTableRow(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestPadRow(t *testing.T) {
	tests := []struct {
		name string
		row  []string
		col  int
		want []string
	}{
		{"exact fit", []string{"a", "b", "c"}, 3, []string{"a", "b", "c"}},
		{"pad short", []string{"a"}, 3, []string{"a", "", ""}},
		{"truncate long", []string{"a", "b", "c", "d"}, 2, []string{"a", "b"}},
		{"empty input", nil, 3, []string{"", "", ""}},
		{"zero cols", []string{"a"}, 0, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := padRow(tt.row, tt.col)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("padRow(%v, %d) = %v, want %v", tt.row, tt.col, got, tt.want)
			}
		})
	}
}

func TestValidateSheetImportMDSize_RejectsOver5000Cells(t *testing.T) {
	rows := make([][]string, 51)
	for i := range rows {
		rows[i] = make([]string, 100)
	}
	err := validateSheetImportMDSize(rows)
	if err == nil {
		t.Fatal("expected over 5000 cells to be rejected")
	}
	if !strings.Contains(err.Error(), "5100") || !strings.Contains(err.Error(), "5000") {
		t.Fatalf("error should mention actual and limit cells, got: %v", err)
	}
}

func TestRunSheetImportMD_JSONStdoutIsPureJSON(t *testing.T) {
	mdPath := writeTempMarkdown(t, `| name | age |
| --- | --- |
| Alice | 30 |
`)
	cmd := newSheetImportMDTestCommand(t)
	if err := cmd.Flags().Set("output", "json"); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	deps := sheetImportMDTestDeps(t, func(ctx context.Context, title, folderToken, userAccessToken string) (*client.SpreadsheetInfo, error) {
		return &client.SpreadsheetInfo{
			SpreadsheetToken: "sht_test",
			Title:            title,
			URL:              "https://feishu.cn/sheets/sht_test",
		}, nil
	})

	stdout := captureStdout(t, func() {
		if err := runSheetImportMD(cmd, []string{mdPath}, deps); err != nil {
			t.Fatalf("runSheetImportMD returned error: %v", err)
		}
	})

	var got sheetImportMDResult
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout should be pure JSON, got %q, error: %v", stdout, err)
	}
	if got.SpreadsheetToken != "sht_test" || got.Rows != 2 || got.Cols != 2 {
		t.Fatalf("unexpected JSON result: %+v", got)
	}
	if strings.Contains(stdout, "解析到表格") || strings.Contains(stdout, "正在") {
		t.Fatalf("stdout contains progress text: %q", stdout)
	}
	if !strings.Contains(stderr.String(), "解析到表格") || !strings.Contains(stderr.String(), "正在写入") {
		t.Fatalf("expected progress on stderr in json mode, got: %q", stderr.String())
	}
}

func TestRunSheetImportMD_PrecheckRejectsBeforeCreate(t *testing.T) {
	mdPath := writeTempMarkdown(t, buildMarkdownTable(51, 100))
	cmd := newSheetImportMDTestCommand(t)
	deps := sheetImportMDTestDeps(t, func(ctx context.Context, title, folderToken, userAccessToken string) (*client.SpreadsheetInfo, error) {
		t.Fatal("CreateSpreadsheet should not be called when precheck fails")
		return nil, nil
	})

	err := runSheetImportMD(cmd, []string{mdPath}, deps)
	if err == nil {
		t.Fatal("expected precheck error")
	}
	if !strings.Contains(err.Error(), "5100") || !strings.Contains(err.Error(), "5000") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunSheetImportMD_FolderFlagPreferred(t *testing.T) {
	mdPath := writeTempMarkdown(t, `| name | age |
| --- | --- |
| Alice | 30 |
`)
	cmd := newSheetImportMDTestCommand(t)
	if err := cmd.Flags().Set("folder", "fld_preferred"); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	deps := sheetImportMDTestDeps(t, func(ctx context.Context, title, folderToken, userAccessToken string) (*client.SpreadsheetInfo, error) {
		if folderToken != "fld_preferred" {
			t.Fatalf("folder token = %q, want fld_preferred", folderToken)
		}
		return &client.SpreadsheetInfo{
			SpreadsheetToken: "sht_test",
			Title:            title,
			URL:              "https://feishu.cn/sheets/sht_test",
		}, nil
	})

	if err := runSheetImportMD(cmd, []string{mdPath}, deps); err != nil {
		t.Fatalf("runSheetImportMD returned error: %v", err)
	}
}

func newSheetImportMDTestCommand(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "import-md"}
	cmd.Flags().StringP("title", "t", "", "")
	cmd.Flags().StringP("folder", "f", "", "")
	cmd.Flags().String("folder-token", "", "")
	cmd.Flags().Int("table-index", 0, "")
	cmd.Flags().StringP("output", "o", "text", "")
	cmd.Flags().String("user-access-token", "", "")
	return cmd
}

func sheetImportMDTestDeps(t *testing.T, create func(context.Context, string, string, string) (*client.SpreadsheetInfo, error)) sheetImportMDDeps {
	t.Helper()
	return sheetImportMDDeps{
		validate:          func() error { return nil },
		resolveUserToken:  func(*cobra.Command) string { return "user-token" },
		createSpreadsheet: create,
		querySheets: func(ctx context.Context, spreadsheetToken, userAccessToken string) ([]*client.SheetInfo, error) {
			return []*client.SheetInfo{{SheetID: "sheet1"}}, nil
		},
		writeCells: func(ctx context.Context, spreadsheetToken, rangeStr string, values [][]any, userAccessToken string) (*client.CellRange, error) {
			if rangeStr != "sheet1!A1:B2" {
				t.Fatalf("unexpected range: %s", rangeStr)
			}
			return &client.CellRange{Range: rangeStr, Values: values}, nil
		},
	}
}

func writeTempMarkdown(t *testing.T, content string) string {
	t.Helper()
	path := t.TempDir() + "/input.md"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func buildMarkdownTable(rows, cols int) string {
	var b strings.Builder
	for c := 0; c < cols; c++ {
		fmt.Fprintf(&b, "| h%d ", c)
	}
	b.WriteString("|\n")
	for c := 0; c < cols; c++ {
		b.WriteString("| --- ")
	}
	b.WriteString("|\n")
	for r := 1; r < rows; r++ {
		for c := 0; c < cols; c++ {
			fmt.Fprintf(&b, "| %d-%d ", r, c)
		}
		b.WriteString("|\n")
	}
	return b.String()
}
