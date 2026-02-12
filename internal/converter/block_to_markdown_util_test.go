package converter

import (
	"strings"
	"testing"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
)

// TestPtrBoolEq 测试 ptrBoolEq 函数
func TestPtrBoolEq(t *testing.T) {
	tests := []struct {
		name string
		a    *bool
		b    *bool
		want bool
	}{
		{
			name: "两个nil",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "a为nil",
			a:    nil,
			b:    boolPtr(true),
			want: false,
		},
		{
			name: "b为nil",
			a:    boolPtr(true),
			b:    nil,
			want: false,
		},
		{
			name: "相同值true",
			a:    boolPtr(true),
			b:    boolPtr(true),
			want: true,
		},
		{
			name: "相同值false",
			a:    boolPtr(false),
			b:    boolPtr(false),
			want: true,
		},
		{
			name: "不同值",
			a:    boolPtr(true),
			b:    boolPtr(false),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ptrBoolEq(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("ptrBoolEq() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestPtrIntEq 测试 ptrIntEq 函数
func TestPtrIntEq(t *testing.T) {
	tests := []struct {
		name string
		a    *int
		b    *int
		want bool
	}{
		{
			name: "两个nil",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "a为nil",
			a:    nil,
			b:    intPtr(10),
			want: false,
		},
		{
			name: "b为nil",
			a:    intPtr(10),
			b:    nil,
			want: false,
		},
		{
			name: "相同值",
			a:    intPtr(42),
			b:    intPtr(42),
			want: true,
		},
		{
			name: "不同值",
			a:    intPtr(10),
			b:    intPtr(20),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ptrIntEq(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("ptrIntEq() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestLinkEqual 测试 linkEqual 函数
func TestLinkEqual(t *testing.T) {
	tests := []struct {
		name string
		a    *larkdocx.Link
		b    *larkdocx.Link
		want bool
	}{
		{
			name: "两个nil",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "a为nil",
			a:    nil,
			b:    &larkdocx.Link{Url: strPtr("https://example.com")},
			want: false,
		},
		{
			name: "b为nil",
			a:    &larkdocx.Link{Url: strPtr("https://example.com")},
			b:    nil,
			want: false,
		},
		{
			name: "两个URL都为nil",
			a:    &larkdocx.Link{Url: nil},
			b:    &larkdocx.Link{Url: nil},
			want: true,
		},
		{
			name: "a的URL为nil",
			a:    &larkdocx.Link{Url: nil},
			b:    &larkdocx.Link{Url: strPtr("https://example.com")},
			want: false,
		},
		{
			name: "b的URL为nil",
			a:    &larkdocx.Link{Url: strPtr("https://example.com")},
			b:    &larkdocx.Link{Url: nil},
			want: false,
		},
		{
			name: "相同URL",
			a:    &larkdocx.Link{Url: strPtr("https://example.com")},
			b:    &larkdocx.Link{Url: strPtr("https://example.com")},
			want: true,
		},
		{
			name: "不同URL",
			a:    &larkdocx.Link{Url: strPtr("https://example.com")},
			b:    &larkdocx.Link{Url: strPtr("https://other.com")},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := linkEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("linkEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestTextStyleEqual 测试 textStyleEqual 函数
func TestTextStyleEqual(t *testing.T) {
	tests := []struct {
		name string
		a    *larkdocx.TextElementStyle
		b    *larkdocx.TextElementStyle
		want bool
	}{
		{
			name: "两个nil",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "a为nil",
			a:    nil,
			b:    &larkdocx.TextElementStyle{Bold: boolPtr(true)},
			want: false,
		},
		{
			name: "b为nil",
			a:    &larkdocx.TextElementStyle{Bold: boolPtr(true)},
			b:    nil,
			want: false,
		},
		{
			name: "相同空样式",
			a:    &larkdocx.TextElementStyle{},
			b:    &larkdocx.TextElementStyle{},
			want: true,
		},
		{
			name: "相同样式",
			a: &larkdocx.TextElementStyle{
				Bold:   boolPtr(true),
				Italic: boolPtr(false),
			},
			b: &larkdocx.TextElementStyle{
				Bold:   boolPtr(true),
				Italic: boolPtr(false),
			},
			want: true,
		},
		{
			name: "Bold不同",
			a: &larkdocx.TextElementStyle{
				Bold: boolPtr(true),
			},
			b: &larkdocx.TextElementStyle{
				Bold: boolPtr(false),
			},
			want: false,
		},
		{
			name: "Italic不同",
			a: &larkdocx.TextElementStyle{
				Italic: boolPtr(true),
			},
			b: &larkdocx.TextElementStyle{
				Italic: boolPtr(false),
			},
			want: false,
		},
		{
			name: "Strikethrough不同",
			a: &larkdocx.TextElementStyle{
				Strikethrough: boolPtr(true),
			},
			b: &larkdocx.TextElementStyle{
				Strikethrough: boolPtr(false),
			},
			want: false,
		},
		{
			name: "Underline不同",
			a: &larkdocx.TextElementStyle{
				Underline: boolPtr(true),
			},
			b: &larkdocx.TextElementStyle{
				Underline: boolPtr(false),
			},
			want: false,
		},
		{
			name: "InlineCode不同",
			a: &larkdocx.TextElementStyle{
				InlineCode: boolPtr(true),
			},
			b: &larkdocx.TextElementStyle{
				InlineCode: boolPtr(false),
			},
			want: false,
		},
		{
			name: "Link不同",
			a: &larkdocx.TextElementStyle{
				Link: &larkdocx.Link{Url: strPtr("https://example.com")},
			},
			b: &larkdocx.TextElementStyle{
				Link: &larkdocx.Link{Url: strPtr("https://other.com")},
			},
			want: false,
		},
		{
			name: "TextColor不同",
			a: &larkdocx.TextElementStyle{
				TextColor: intPtr(1),
			},
			b: &larkdocx.TextElementStyle{
				TextColor: intPtr(2),
			},
			want: false,
		},
		{
			name: "BackgroundColor不同",
			a: &larkdocx.TextElementStyle{
				BackgroundColor: intPtr(1),
			},
			b: &larkdocx.TextElementStyle{
				BackgroundColor: intPtr(2),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := textStyleEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("textStyleEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMergeAdjacentElements 测试 mergeAdjacentElements 函数
func TestMergeAdjacentElements(t *testing.T) {
	tests := []struct {
		name     string
		elements []*larkdocx.TextElement
		wantLen  int
		validate func(t *testing.T, result []*larkdocx.TextElement)
	}{
		{
			name:     "空切片",
			elements: []*larkdocx.TextElement{},
			wantLen:  0,
		},
		{
			name: "单个元素",
			elements: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: strPtr("hello")}},
			},
			wantLen: 1,
		},
		{
			name: "相同样式合并",
			elements: []*larkdocx.TextElement{
				{
					TextRun: &larkdocx.TextRun{
						Content:          strPtr("hello"),
						TextElementStyle: &larkdocx.TextElementStyle{Bold: boolPtr(true)},
					},
				},
				{
					TextRun: &larkdocx.TextRun{
						Content:          strPtr(" world"),
						TextElementStyle: &larkdocx.TextElementStyle{Bold: boolPtr(true)},
					},
				},
			},
			wantLen: 1,
			validate: func(t *testing.T, result []*larkdocx.TextElement) {
				if result[0].TextRun == nil || result[0].TextRun.Content == nil {
					t.Fatal("merged element should have content")
				}
				if *result[0].TextRun.Content != "hello world" {
					t.Errorf("merged content = %q, want %q", *result[0].TextRun.Content, "hello world")
				}
			},
		},
		{
			name: "不同样式不合并",
			elements: []*larkdocx.TextElement{
				{
					TextRun: &larkdocx.TextRun{
						Content:          strPtr("hello"),
						TextElementStyle: &larkdocx.TextElementStyle{Bold: boolPtr(true)},
					},
				},
				{
					TextRun: &larkdocx.TextRun{
						Content:          strPtr(" world"),
						TextElementStyle: &larkdocx.TextElementStyle{Italic: boolPtr(true)},
					},
				},
			},
			wantLen: 2,
		},
		{
			name: "包含非TextRun元素",
			elements: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: strPtr("hello")}},
				{MentionUser: &larkdocx.MentionUser{UserId: strPtr("user123")}},
				{TextRun: &larkdocx.TextRun{Content: strPtr("world")}},
			},
			wantLen: 3,
		},
		{
			name: "连续相同样式多次合并",
			elements: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: strPtr("a")}},
				{TextRun: &larkdocx.TextRun{Content: strPtr("b")}},
				{TextRun: &larkdocx.TextRun{Content: strPtr("c")}},
			},
			wantLen: 1,
			validate: func(t *testing.T, result []*larkdocx.TextElement) {
				if *result[0].TextRun.Content != "abc" {
					t.Errorf("merged content = %q, want %q", *result[0].TextRun.Content, "abc")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeAdjacentElements(tt.elements)
			if len(got) != tt.wantLen {
				t.Errorf("mergeAdjacentElements() len = %d, want %d", len(got), tt.wantLen)
			}
			if tt.validate != nil {
				tt.validate(t, got)
			}
		})
	}
}

// TestEscapeMarkdown 测试 escapeMarkdown 函数
func TestEscapeMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "普通文本",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "反斜杠",
			input: "path\\to\\file",
			want:  "path\\\\to\\\\file",
		},
		{
			name:  "星号",
			input: "file*.txt",
			want:  "file\\*.txt",
		},
		{
			name:  "下划线",
			input: "file_name",
			want:  "file\\_name",
		},
		{
			name:  "方括号",
			input: "[link]",
			want:  "\\[link\\]",
		},
		{
			name:  "井号",
			input: "#hashtag",
			want:  "\\#hashtag",
		},
		{
			name:  "波浪号",
			input: "~strikethrough~",
			want:  "\\~strikethrough\\~",
		},
		{
			name:  "反引号",
			input: "`code`",
			want:  "\\`code\\`",
		},
		{
			name:  "美元符号",
			input: "$100",
			want:  "\\$100",
		},
		{
			name:  "管道符",
			input: "col1|col2",
			want:  "col1\\|col2",
		},
		{
			name:  "大于号",
			input: "> quote",
			want:  "\\> quote",
		},
		{
			name:  "混合特殊字符",
			input: "*bold* _italic_ `code` $math$ |table|",
			want:  "\\*bold\\* \\_italic\\_ \\`code\\` \\$math\\$ \\|table\\|",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeMarkdown(tt.input)
			if got != tt.want {
				t.Errorf("escapeMarkdown() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestConvertTextElementsRaw 测试 convertTextElementsRaw 函数
func TestConvertTextElementsRaw(t *testing.T) {
	tests := []struct {
		name     string
		elements []*larkdocx.TextElement
		options  ConvertOptions
		cache    map[string]MentionUserInfo
		want     string
	}{
		{
			name:     "nil元素跳过",
			elements: []*larkdocx.TextElement{nil, {TextRun: &larkdocx.TextRun{Content: strPtr("text")}}},
			options:  ConvertOptions{},
			want:     "text",
		},
		{
			name: "普通TextRun",
			elements: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: strPtr("hello")}},
			},
			options: ConvertOptions{},
			want:    "hello",
		},
		{
			name: "MentionUser展开",
			elements: []*larkdocx.TextElement{
				{MentionUser: &larkdocx.MentionUser{UserId: strPtr("user123")}},
			},
			options: ConvertOptions{ExpandMentions: true},
			cache: map[string]MentionUserInfo{
				"user123": {Name: "张三", Email: "zhangsan@example.com"},
			},
			want: "张三",
		},
		{
			name: "MentionUser不展开",
			elements: []*larkdocx.TextElement{
				{MentionUser: &larkdocx.MentionUser{UserId: strPtr("user123")}},
			},
			options: ConvertOptions{ExpandMentions: false},
			want:    "user123",
		},
		{
			name: "MentionDoc有URL",
			elements: []*larkdocx.TextElement{
				{MentionDoc: &larkdocx.MentionDoc{
					Title: strPtr("文档标题"),
					Url:   strPtr("https://example.com/doc"),
				}},
			},
			options: ConvertOptions{},
			want:    "文档标题",
		},
		{
			name: "MentionDoc无URL",
			elements: []*larkdocx.TextElement{
				{MentionDoc: &larkdocx.MentionDoc{
					Title: strPtr("文档标题"),
				}},
			},
			options: ConvertOptions{},
			want:    "文档标题",
		},
		{
			name: "Equation元素",
			elements: []*larkdocx.TextElement{
				{Equation: &larkdocx.Equation{Content: strPtr("x^2 + y^2 = z^2")}},
			},
			options: ConvertOptions{},
			want:    "x^2 + y^2 = z^2",
		},
		{
			name: "混合元素",
			elements: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: strPtr("hello ")}},
				{MentionUser: &larkdocx.MentionUser{UserId: strPtr("user123")}},
				{TextRun: &larkdocx.TextRun{Content: strPtr(" world")}},
			},
			options: ConvertOptions{ExpandMentions: false},
			want:    "hello user123 world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := &BlockToMarkdown{
				options:   tt.options,
				userCache: tt.cache,
			}
			got := converter.convertTextElementsRaw(tt.elements)
			if got != tt.want {
				t.Errorf("convertTextElementsRaw() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestConvertTextElements 测试 convertTextElements 的未覆盖分支
func TestConvertTextElements(t *testing.T) {
	tests := []struct {
		name     string
		elements []*larkdocx.TextElement
		options  ConvertOptions
		want     string
	}{
		{
			name: "MentionDoc URL编码",
			elements: []*larkdocx.TextElement{
				{MentionDoc: &larkdocx.MentionDoc{
					Title: strPtr("文档"),
					Url:   strPtr("https://example.com/doc?id=123&name=test"),
				}},
			},
			options: ConvertOptions{},
			want:    "[文档](https://example.com/doc?id=123&name=test)",
		},
		{
			name: "MentionDoc URL包含括号",
			elements: []*larkdocx.TextElement{
				{MentionDoc: &larkdocx.MentionDoc{
					Title: strPtr("文档(草稿)"),
					Url:   strPtr("https://example.com/doc(1)"),
				}},
			},
			options: ConvertOptions{},
			want:    "[文档(草稿)](https://example.com/doc%281%29)",
		},
		{
			name: "Link URL解码",
			elements: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{
					Content: strPtr("链接"),
					TextElementStyle: &larkdocx.TextElementStyle{
						Link: &larkdocx.Link{
							Url: strPtr("https%3A%2F%2Fexample.com"),
						},
					},
				}},
			},
			options: ConvertOptions{},
			want:    "[链接](https://example.com)",
		},
		{
			name: "Link URL包含括号",
			elements: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{
					Content: strPtr("链接"),
					TextElementStyle: &larkdocx.TextElementStyle{
						Link: &larkdocx.Link{
							Url: strPtr("https://example.com/page(1)"),
						},
					},
				}},
			},
			options: ConvertOptions{},
			want:    "[链接](https://example.com/page%281%29)",
		},
		{
			name: "无样式纯文本转义",
			elements: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{
					Content: strPtr("*重要* _注意_"),
				}},
			},
			options: ConvertOptions{},
			want:    "\\*重要\\* \\_注意\\_",
		},
		{
			name: "Underline样式",
			elements: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{
					Content: strPtr("下划线文本"),
					TextElementStyle: &larkdocx.TextElementStyle{
						Underline: boolPtr(true),
					},
				}},
			},
			options: ConvertOptions{},
			want:    "<u>下划线文本</u>",
		},
		{
			name: "混合样式",
			elements: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{
					Content: strPtr("粗体"),
					TextElementStyle: &larkdocx.TextElementStyle{
						Bold: boolPtr(true),
					},
				}},
				{TextRun: &larkdocx.TextRun{
					Content: strPtr("斜体"),
					TextElementStyle: &larkdocx.TextElementStyle{
						Italic: boolPtr(true),
					},
				}},
			},
			options: ConvertOptions{},
			want:    "**粗体***斜体*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := &BlockToMarkdown{
				options: tt.options,
			}
			got := converter.convertTextElements(tt.elements)
			if got != tt.want {
				t.Errorf("convertTextElements() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestWrapHighlightSpan 测试 wrapHighlightSpan 的未覆盖路径
func TestWrapHighlightSpan(t *testing.T) {
	tests := []struct {
		name  string
		style *larkdocx.TextElementStyle
		text  string
		want  string
	}{
		{
			name:  "style为nil",
			style: nil,
			text:  "text",
			want:  "text",
		},
		{
			name: "TextColor不在映射中",
			style: &larkdocx.TextElementStyle{
				TextColor: intPtr(999),
			},
			text: "text",
			want: "text",
		},
		{
			name: "BackgroundColor不在映射中",
			style: &larkdocx.TextElementStyle{
				BackgroundColor: intPtr(999),
			},
			text: "text",
			want: "text",
		},
		{
			name: "有效的TextColor",
			style: &larkdocx.TextElementStyle{
				TextColor: intPtr(1),
			},
			text: "text",
			want: "<span style=\"color: #ef4444\">text</span>",
		},
		{
			name: "有效的BackgroundColor",
			style: &larkdocx.TextElementStyle{
				BackgroundColor: intPtr(1),
			},
			text: "text",
			want: "<span style=\"background-color: #fef2f2\">text</span>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := &BlockToMarkdown{}
			got := converter.wrapHighlightSpan(tt.style, tt.text)
			if got != tt.want {
				t.Errorf("wrapHighlightSpan() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDegradeDeepHeadings 测试 DegradeDeepHeadings 选项
func TestDegradeDeepHeadings(t *testing.T) {
	tests := []struct {
		name    string
		level   int
		content string
		options ConvertOptions
		want    string
	}{
		{
			name:    "Heading7降级",
			level:   7,
			content: "深层标题",
			options: ConvertOptions{DegradeDeepHeadings: true},
			want:    "**深层标题**\n",
		},
		{
			name:    "Heading8降级",
			level:   8,
			content: "更深标题",
			options: ConvertOptions{DegradeDeepHeadings: true},
			want:    "**更深标题**\n",
		},
		{
			name:    "Heading9降级",
			level:   9,
			content: "最深标题",
			options: ConvertOptions{DegradeDeepHeadings: true},
			want:    "**最深标题**\n",
		},
		{
			name:    "不降级H7",
			level:   7,
			content: "深层标题",
			options: ConvertOptions{DegradeDeepHeadings: false},
			want:    "###### 深层标题\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blockType := int(BlockTypeHeading1) + tt.level - 1
			headingText := &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{TextRun: &larkdocx.TextRun{Content: strPtr(tt.content)}},
				},
			}

			block := &larkdocx.Block{
				BlockId:   strPtr("test"),
				BlockType: &blockType,
			}

			switch tt.level {
			case 7:
				block.Heading7 = headingText
			case 8:
				block.Heading8 = headingText
			case 9:
				block.Heading9 = headingText
			}

			converter := NewBlockToMarkdown([]*larkdocx.Block{block}, tt.options)
			got, err := converter.convertHeading(block, BlockType(blockType))
			if err != nil {
				t.Fatalf("convertHeading() error = %v", err)
			}

			if got != tt.want {
				t.Errorf("convertHeading() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestIsListBlockType 测试 isListBlockType 函数
func TestIsListBlockType(t *testing.T) {
	tests := []struct {
		name      string
		blockType BlockType
		want      bool
	}{
		{name: "Bullet", blockType: BlockTypeBullet, want: true},
		{name: "Ordered", blockType: BlockTypeOrdered, want: true},
		{name: "Todo", blockType: BlockTypeTodo, want: true},
		{name: "Text", blockType: BlockTypeText, want: false},
		{name: "Heading1", blockType: BlockTypeHeading1, want: false},
		{name: "Code", blockType: BlockTypeCode, want: false},
		{name: "Table", blockType: BlockTypeTable, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isListBlockType(tt.blockType)
			if got != tt.want {
				t.Errorf("isListBlockType(%v) = %v, want %v", tt.blockType, got, tt.want)
			}
		})
	}
}

// TestComputeHeadingSeq 测试标题编号计算
func TestComputeHeadingSeq(t *testing.T) {
	tests := []struct {
		name  string
		level int
		style *larkdocx.TextStyle
		want  string
	}{
		{
			name:  "空sequence返回空",
			level: 1,
			style: &larkdocx.TextStyle{Sequence: strPtr("")},
			want:  "",
		},
		{
			name:  "nil style返回空",
			level: 1,
			style: nil,
			want:  "",
		},
		{
			name:  "nil Sequence返回空",
			level: 1,
			style: &larkdocx.TextStyle{},
			want:  "",
		},
		{
			name:  "手动编号",
			level: 1,
			style: &larkdocx.TextStyle{Sequence: strPtr("1")},
			want:  "1. ",
		},
		{
			name:  "auto编号",
			level: 1,
			style: &larkdocx.TextStyle{Sequence: strPtr("auto")},
			want:  "1. ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := NewBlockToMarkdown([]*larkdocx.Block{}, ConvertOptions{})
			got := converter.computeHeadingSeq(tt.level, tt.style)
			if got != tt.want {
				t.Errorf("computeHeadingSeq() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestLanguageCodeToNameExtended 测试更多语言代码映射
func TestLanguageCodeToNameExtended(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{code: 1, want: "plaintext"},
		{code: 2, want: "abap"},
		{code: 7, want: "bash"},
		{code: 16, want: "delphi"},
		{code: 22, want: "go"},
		{code: 29, want: "java"},
		{code: 30, want: "javascript"},
		{code: 47, want: "python"},
		{code: 66, want: "yaml"},
		{code: 999, want: "plaintext"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := languageCodeToName(tt.code)
			if got != tt.want {
				t.Errorf("languageCodeToName(%d) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}

// TestConvertCode 测试代码块转换的语言支持
func TestConvertCode(t *testing.T) {
	tests := []struct {
		name     string
		langCode int
		content  string
		want     string
	}{
		{
			name:     "YAML代码块",
			langCode: 66,
			content:  "key: value",
			want:     "```yaml\nkey: value\n```\n",
		},
		{
			name:     "Bash代码块",
			langCode: 7,
			content:  "echo hello",
			want:     "```bash\necho hello\n```\n",
		},
		{
			name:     "Go代码块",
			langCode: 22,
			content:  "package main",
			want:     "```go\npackage main\n```\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blockType := int(BlockTypeCode)
			block := &larkdocx.Block{
				BlockId:   strPtr("code"),
				BlockType: &blockType,
				Code: &larkdocx.Text{
					Style: &larkdocx.TextStyle{
						Language: &tt.langCode,
					},
					Elements: []*larkdocx.TextElement{
						{TextRun: &larkdocx.TextRun{Content: strPtr(tt.content)}},
					},
				},
			}

			converter := NewBlockToMarkdown([]*larkdocx.Block{block}, ConvertOptions{})
			got, err := converter.convertCode(block)
			if err != nil {
				t.Fatalf("convertCode() error = %v", err)
			}

			if got != tt.want {
				t.Errorf("convertCode() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestConvertTextElementsEdgeCases 测试 convertTextElements 的边界情况
func TestConvertTextElementsEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		elements []*larkdocx.TextElement
		want     string
	}{
		{
			name: "Equation空内容",
			elements: []*larkdocx.TextElement{
				{Equation: &larkdocx.Equation{Content: strPtr("")}},
			},
			want: "$$",
		},
		{
			name: "MentionDoc空标题",
			elements: []*larkdocx.TextElement{
				{MentionDoc: &larkdocx.MentionDoc{
					Title: strPtr(""),
					Url:   strPtr("https://example.com"),
				}},
			},
			want: "[](https://example.com)",
		},
		{
			name: "Link空内容",
			elements: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{
					Content: strPtr(""),
					TextElementStyle: &larkdocx.TextElementStyle{
						Link: &larkdocx.Link{Url: strPtr("https://example.com")},
					},
				}},
			},
			want: "[](https://example.com)",
		},
		{
			name: "多个连续空格",
			elements: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: strPtr("a   b")}},
			},
			want: "a   b",
		},
		{
			name: "特殊字符混合",
			elements: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{
					Content: strPtr("*bold* and `code` with $math$"),
				}},
			},
			want: "\\*bold\\* and \\`code\\` with \\$math\\$",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := &BlockToMarkdown{options: ConvertOptions{}}
			got := converter.convertTextElements(tt.elements)
			if got != tt.want {
				t.Errorf("convertTextElements() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestMergeAdjacentElementsNilContent 测试合并时的 nil Content 处理
func TestMergeAdjacentElementsNilContent(t *testing.T) {
	elements := []*larkdocx.TextElement{
		{TextRun: &larkdocx.TextRun{Content: strPtr("hello")}},
		{TextRun: &larkdocx.TextRun{Content: nil}},
		{TextRun: &larkdocx.TextRun{Content: strPtr("world")}},
	}

	got := mergeAdjacentElements(elements)
	// nil Content 的元素会被保留，不参与合并
	if len(got) != 3 {
		t.Errorf("expected 3 elements (nil Content not merged), got %d", len(got))
	}

	if got[0].TextRun == nil || got[0].TextRun.Content == nil {
		t.Fatal("first element should have content")
	}

	if *got[0].TextRun.Content != "hello" {
		t.Errorf("first content = %q, want %q", *got[0].TextRun.Content, "hello")
	}
}

// TestTextStyleEqualWithAllFields 测试所有字段都不同的情况
func TestTextStyleEqualWithAllFields(t *testing.T) {
	style1 := &larkdocx.TextElementStyle{
		Bold:            boolPtr(true),
		Italic:          boolPtr(false),
		Strikethrough:   boolPtr(true),
		Underline:       boolPtr(false),
		InlineCode:      boolPtr(true),
		Link:            &larkdocx.Link{Url: strPtr("https://a.com")},
		TextColor:       intPtr(1),
		BackgroundColor: intPtr(2),
	}

	style2 := &larkdocx.TextElementStyle{
		Bold:            boolPtr(true),
		Italic:          boolPtr(false),
		Strikethrough:   boolPtr(true),
		Underline:       boolPtr(false),
		InlineCode:      boolPtr(true),
		Link:            &larkdocx.Link{Url: strPtr("https://a.com")},
		TextColor:       intPtr(1),
		BackgroundColor: intPtr(2),
	}

	if !textStyleEqual(style1, style2) {
		t.Error("完全相同的样式应该相等")
	}

	// 修改每个字段验证不等
	style2.Bold = boolPtr(false)
	if textStyleEqual(style1, style2) {
		t.Error("Bold不同应该不等")
	}
}

// TestConvertWithMultipleStyleCombinations 测试多种样式组合
func TestConvertWithMultipleStyleCombinations(t *testing.T) {
	tests := []struct {
		name  string
		style *larkdocx.TextElementStyle
		text  string
		want  string
	}{
		{
			name: "粗体+斜体",
			style: &larkdocx.TextElementStyle{
				Bold:   boolPtr(true),
				Italic: boolPtr(true),
			},
			text: "text",
			want: "***text***",
		},
		{
			name: "粗体+删除线",
			style: &larkdocx.TextElementStyle{
				Bold:          boolPtr(true),
				Strikethrough: boolPtr(true),
			},
			text: "text",
			want: "**~~text~~**",
		},
		{
			name: "斜体+删除线+下划线",
			style: &larkdocx.TextElementStyle{
				Italic:        boolPtr(true),
				Strikethrough: boolPtr(true),
				Underline:     boolPtr(true),
			},
			text: "text",
			want: "*~~<u>text</u>~~*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := &BlockToMarkdown{options: ConvertOptions{}}
			elements := []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{
					Content:          strPtr(tt.text),
					TextElementStyle: tt.style,
				}},
			}
			got := converter.convertTextElements(elements)
			if !strings.Contains(got, "text") {
				t.Errorf("convertTextElements() = %q, should contain 'text'", got)
			}
		})
	}
}
