package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/v2/internal/converter"
)

func TestNormalizeSheetExportFormat(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "xlsx", want: "xlsx"},
		{input: " CSV ", want: "csv"},
		{input: "md", want: "markdown"},
		{input: "MARKDOWN", want: "markdown"},
	}

	for _, tt := range tests {
		if got := normalizeSheetExportFormat(tt.input); got != tt.want {
			t.Fatalf("normalizeSheetExportFormat(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSheetExportFileExt(t *testing.T) {
	if got := sheetExportFileExt("markdown"); got != "md" {
		t.Fatalf("sheetExportFileExt(markdown) = %q, want md", got)
	}
	if got := sheetExportFileExt("csv"); got != "csv" {
		t.Fatalf("sheetExportFileExt(csv) = %q, want csv", got)
	}
}

func TestExportSheetAsMarkdown(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "sheet.md")
	var gotToken, gotSheetID, gotUserToken string

	err := exportSheetAsMarkdown("sht_test", "sheet1", outputPath, "u-test",
		func(spreadsheetToken, sheetID, userAccessToken string) ([]*converter.SheetData, error) {
			gotToken = spreadsheetToken
			gotSheetID = sheetID
			gotUserToken = userAccessToken
			return []*converter.SheetData{
				{
					Title: "Sheet1",
					Values: [][]any{
						{"姓名", "数量"},
						{"苹果", float64(2)},
					},
				},
			}, nil
		},
		os.WriteFile,
	)
	if err != nil {
		t.Fatalf("exportSheetAsMarkdown() error = %v", err)
	}
	if gotToken != "sht_test" || gotSheetID != "sheet1" || gotUserToken != "u-test" {
		t.Fatalf("fetch args = (%q, %q, %q)", gotToken, gotSheetID, gotUserToken)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, want := range []string{"## Sheet1", "| 姓名 | 数量 |", "| 苹果 | 2 |"} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("output missing %q:\n%s", want, content)
		}
	}
}

func TestExportSheetAsMarkdownFetchError(t *testing.T) {
	err := exportSheetAsMarkdown("sht_test", "", "unused.md", "u-test",
		func(spreadsheetToken, sheetID, userAccessToken string) ([]*converter.SheetData, error) {
			return nil, errors.New("boom")
		},
		os.WriteFile,
	)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("exportSheetAsMarkdown() error = %v, want boom", err)
	}
}
