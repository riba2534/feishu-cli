package converter

import (
	"testing"
)

func TestBlockTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		expected BlockType
		actual   BlockType
	}{
		{"BlockTypePage", 1, BlockTypePage},
		{"BlockTypeText", 2, BlockTypeText},
		{"BlockTypeHeading1", 3, BlockTypeHeading1},
		{"BlockTypeHeading2", 4, BlockTypeHeading2},
		{"BlockTypeHeading3", 5, BlockTypeHeading3},
		{"BlockTypeHeading4", 6, BlockTypeHeading4},
		{"BlockTypeHeading5", 7, BlockTypeHeading5},
		{"BlockTypeHeading6", 8, BlockTypeHeading6},
		{"BlockTypeHeading7", 9, BlockTypeHeading7},
		{"BlockTypeHeading8", 10, BlockTypeHeading8},
		{"BlockTypeHeading9", 11, BlockTypeHeading9},
		{"BlockTypeBullet", 12, BlockTypeBullet},
		{"BlockTypeOrdered", 13, BlockTypeOrdered},
		{"BlockTypeCode", 14, BlockTypeCode},
		{"BlockTypeQuote", 15, BlockTypeQuote},
		{"BlockTypeEquation", 16, BlockTypeEquation},
		{"BlockTypeTodo", 17, BlockTypeTodo},
		{"BlockTypeBitable", 18, BlockTypeBitable},
		{"BlockTypeCallout", 19, BlockTypeCallout},
		{"BlockTypeChatCard", 20, BlockTypeChatCard},
		{"BlockTypeDiagram", 21, BlockTypeDiagram},
		{"BlockTypeDivider", 22, BlockTypeDivider},
		{"BlockTypeFile", 23, BlockTypeFile},
		{"BlockTypeGrid", 24, BlockTypeGrid},
		{"BlockTypeGridColumn", 25, BlockTypeGridColumn},
		{"BlockTypeIframe", 26, BlockTypeIframe},
		{"BlockTypeImage", 27, BlockTypeImage},
		{"BlockTypeISV", 28, BlockTypeISV},
		{"BlockTypeMindNote", 29, BlockTypeMindNote},
		{"BlockTypeSheet", 30, BlockTypeSheet},
		{"BlockTypeTable", 31, BlockTypeTable},
		{"BlockTypeTableCell", 32, BlockTypeTableCell},
		{"BlockTypeView", 33, BlockTypeView},
		{"BlockTypeQuoteContainer", 34, BlockTypeQuoteContainer},
		{"BlockTypeTask", 35, BlockTypeTask},
		{"BlockTypeOKR", 36, BlockTypeOKR},
		{"BlockTypeOKRObjective", 37, BlockTypeOKRObjective},
		{"BlockTypeOKRKeyResult", 38, BlockTypeOKRKeyResult},
		{"BlockTypeOKRProgress", 39, BlockTypeOKRProgress},
		{"BlockTypeAddOns", 40, BlockTypeAddOns},
		{"BlockTypeJiraIssue", 41, BlockTypeJiraIssue},
		{"BlockTypeWikiCatalog", 42, BlockTypeWikiCatalog},
		{"BlockTypeBoard", 43, BlockTypeBoard},
		{"BlockTypeUndefined", 999, BlockTypeUndefined},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.actual != tt.expected {
				t.Errorf("%s = %d, 期望 %d", tt.name, tt.actual, tt.expected)
			}
		})
	}
}

func TestDiagramTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		expected DiagramType
		actual   DiagramType
	}{
		{"DiagramTypeFlowchart", 1, DiagramTypeFlowchart},
		{"DiagramTypeUML", 2, DiagramTypeUML},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.actual != tt.expected {
				t.Errorf("%s = %d, 期望 %d", tt.name, tt.actual, tt.expected)
			}
		})
	}
}

func TestTextStyleDefaults(t *testing.T) {
	style := TextStyle{}

	if style.Bold != false {
		t.Error("TextStyle.Bold 默认值应为 false")
	}
	if style.Italic != false {
		t.Error("TextStyle.Italic 默认值应为 false")
	}
	if style.Strikethrough != false {
		t.Error("TextStyle.Strikethrough 默认值应为 false")
	}
	if style.Underline != false {
		t.Error("TextStyle.Underline 默认值应为 false")
	}
	if style.InlineCode != false {
		t.Error("TextStyle.InlineCode 默认值应为 false")
	}
	if style.Link != nil {
		t.Error("TextStyle.Link 默认值应为 nil")
	}
}

func TestTextStyleWithLink(t *testing.T) {
	link := &LinkInfo{URL: "https://example.com"}
	style := TextStyle{
		Bold: true,
		Link: link,
	}

	if !style.Bold {
		t.Error("TextStyle.Bold 应为 true")
	}
	if style.Link == nil {
		t.Error("TextStyle.Link 不应为 nil")
	}
	if style.Link.URL != "https://example.com" {
		t.Errorf("TextStyle.Link.URL = %q, 期望 %q", style.Link.URL, "https://example.com")
	}
}

func TestLinkInfo(t *testing.T) {
	link := LinkInfo{URL: "https://feishu.cn/docx/abc123"}

	if link.URL != "https://feishu.cn/docx/abc123" {
		t.Errorf("LinkInfo.URL = %q, 期望 %q", link.URL, "https://feishu.cn/docx/abc123")
	}
}

func TestImageInfo(t *testing.T) {
	img := ImageInfo{
		Token:     "token123",
		URL:       "https://example.com/image.png",
		LocalPath: "./assets/image_1.png",
	}

	if img.Token != "token123" {
		t.Errorf("ImageInfo.Token = %q, 期望 %q", img.Token, "token123")
	}
	if img.URL != "https://example.com/image.png" {
		t.Errorf("ImageInfo.URL = %q, 期望 %q", img.URL, "https://example.com/image.png")
	}
	if img.LocalPath != "./assets/image_1.png" {
		t.Errorf("ImageInfo.LocalPath = %q, 期望 %q", img.LocalPath, "./assets/image_1.png")
	}
}

func TestConvertOptions(t *testing.T) {
	opts := ConvertOptions{
		DownloadImages: true,
		AssetsDir:      "./assets",
		UploadImages:   true,
		DocumentID:     "docx_abc123",
	}

	if !opts.DownloadImages {
		t.Error("ConvertOptions.DownloadImages 应为 true")
	}
	if opts.AssetsDir != "./assets" {
		t.Errorf("ConvertOptions.AssetsDir = %q, 期望 %q", opts.AssetsDir, "./assets")
	}
	if !opts.UploadImages {
		t.Error("ConvertOptions.UploadImages 应为 true")
	}
	if opts.DocumentID != "docx_abc123" {
		t.Errorf("ConvertOptions.DocumentID = %q, 期望 %q", opts.DocumentID, "docx_abc123")
	}
}

func TestConvertOptionsDefaults(t *testing.T) {
	opts := ConvertOptions{}

	if opts.DownloadImages != false {
		t.Error("ConvertOptions.DownloadImages 默认值应为 false")
	}
	if opts.AssetsDir != "" {
		t.Errorf("ConvertOptions.AssetsDir 默认值应为空, 得到 %q", opts.AssetsDir)
	}
	if opts.UploadImages != false {
		t.Error("ConvertOptions.UploadImages 默认值应为 false")
	}
	if opts.DocumentID != "" {
		t.Errorf("ConvertOptions.DocumentID 默认值应为空, 得到 %q", opts.DocumentID)
	}
}

func TestHeadingLevelRange(t *testing.T) {
	// 验证标题级别范围 (1-9)
	headingTypes := []BlockType{
		BlockTypeHeading1, BlockTypeHeading2, BlockTypeHeading3,
		BlockTypeHeading4, BlockTypeHeading5, BlockTypeHeading6,
		BlockTypeHeading7, BlockTypeHeading8, BlockTypeHeading9,
	}

	for i, ht := range headingTypes {
		expectedLevel := i + 1
		actualLevel := int(ht) - int(BlockTypeHeading1) + 1
		if actualLevel != expectedLevel {
			t.Errorf("Heading%d 级别计算错误: 得到 %d, 期望 %d", expectedLevel, actualLevel, expectedLevel)
		}
	}
}
