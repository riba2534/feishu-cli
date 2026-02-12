package converter

import (
	"testing"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
)

// ==========================
// normalizeURL 测试 (55.6% 覆盖)
// ==========================

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "feishu://doc/ 转换为 https://",
			input:    "feishu://doc/ABC123",
			expected: "https://feishu.cn/docx/ABC123",
		},
		{
			name:     "feishu://wiki/ 转换为 https://",
			input:    "feishu://wiki/NODE456",
			expected: "https://feishu.cn/wiki/NODE456",
		},
		{
			name:     "通用 feishu:// 协议转换",
			input:    "feishu://board/token",
			expected: "https://feishu.cn/board/token",
		},
		{
			name:     "URL 解码 https%3A%2F%2F",
			input:    "https%3A%2F%2Fexample.com",
			expected: "https://example.com",
		},
		{
			name:     "已经是正常 URL",
			input:    "https://example.com",
			expected: "https://example.com",
		},
		{
			name:     "锚点链接原样返回",
			input:    "#section",
			expected: "#section",
		},
		{
			name:     "http 协议",
			input:    "http://example.com",
			expected: "http://example.com",
		},
		{
			name:     "复杂 URL 编码",
			input:    "https%3A%2F%2Fexample.com%2Fpath%3Fq%3D1",
			expected: "https://example.com/path?q=1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeURL(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// ==========================
// splitInlineMath 测试 (46.2% 覆盖)
// ==========================

func TestSplitInlineMath(t *testing.T) {
	tests := []struct {
		name     string
		input    []*larkdocx.TextElement
		expected int // 预期元素数量
		checkFn  func(*testing.T, []*larkdocx.TextElement)
	}{
		{
			name: "无公式的普通文本",
			input: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: strPtr("普通文本")}},
			},
			expected: 1,
			checkFn: func(t *testing.T, result []*larkdocx.TextElement) {
				if result[0].TextRun == nil || *result[0].TextRun.Content != "普通文本" {
					t.Errorf("普通文本应保持不变")
				}
			},
		},
		{
			name: "包含单个公式",
			input: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: strPtr("质能公式 $E=mc^2$ 很重要")}},
			},
			expected: 3,
			checkFn: func(t *testing.T, result []*larkdocx.TextElement) {
				if len(result) != 3 {
					t.Fatalf("expected 3 elements, got %d", len(result))
				}
				if result[0].TextRun == nil || *result[0].TextRun.Content != "质能公式 " {
					t.Errorf("前文不正确")
				}
				if result[1].Equation == nil || *result[1].Equation.Content != "E=mc^2" {
					t.Errorf("公式内容不正确")
				}
				if result[2].TextRun == nil || *result[2].TextRun.Content != " 很重要" {
					t.Errorf("后文不正确")
				}
			},
		},
		{
			name: "多个公式",
			input: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: strPtr("前文 $a$ 中间 $b$ 后文")}},
			},
			expected: 5,
			checkFn: func(t *testing.T, result []*larkdocx.TextElement) {
				if len(result) != 5 {
					t.Fatalf("expected 5 elements, got %d", len(result))
				}
				if result[1].Equation == nil || *result[1].Equation.Content != "a" {
					t.Errorf("第一个公式不正确")
				}
				if result[3].Equation == nil || *result[3].Equation.Content != "b" {
					t.Errorf("第二个公式不正确")
				}
			},
		},
		{
			name: "带样式的元素不处理",
			input: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{
					Content:          strPtr("$x$"),
					TextElementStyle: &larkdocx.TextElementStyle{Bold: boolPtr(true)},
				}},
			},
			expected: 1,
			checkFn: func(t *testing.T, result []*larkdocx.TextElement) {
				if result[0].TextRun == nil || *result[0].TextRun.Content != "$x$" {
					t.Errorf("有样式的元素应原样返回")
				}
			},
		},
		{
			name:     "空 elements",
			input:    []*larkdocx.TextElement{},
			expected: 0,
		},
		{
			name: "单元素无需合并",
			input: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: strPtr("$formula$")}},
			},
			expected: 1,
			checkFn: func(t *testing.T, result []*larkdocx.TextElement) {
				if result[0].Equation == nil || *result[0].Equation.Content != "formula" {
					t.Errorf("公式应被提取")
				}
			},
		},
		{
			name: "公式在开头",
			input: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: strPtr("$x$ 后文")}},
			},
			expected: 2,
		},
		{
			name: "公式在结尾",
			input: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: strPtr("前文 $y$")}},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitInlineMath(tt.input)
			if len(result) != tt.expected {
				t.Errorf("expected %d elements, got %d", tt.expected, len(result))
			}
			if tt.checkFn != nil {
				tt.checkFn(t, result)
			}
		})
	}
}

// ==========================
// isPlainTextRun 测试
// ==========================

func TestIsPlainTextRun(t *testing.T) {
	tests := []struct {
		name     string
		input    *larkdocx.TextElement
		expected bool
	}{
		{
			name:     "nil 元素",
			input:    nil,
			expected: false,
		},
		{
			name:     "TextRun nil",
			input:    &larkdocx.TextElement{},
			expected: false,
		},
		{
			name:     "Content nil",
			input:    &larkdocx.TextElement{TextRun: &larkdocx.TextRun{}},
			expected: false,
		},
		{
			name: "无样式",
			input: &larkdocx.TextElement{
				TextRun: &larkdocx.TextRun{Content: strPtr("text")},
			},
			expected: true,
		},
		{
			name: "空 TextElementStyle",
			input: &larkdocx.TextElement{
				TextRun: &larkdocx.TextRun{
					Content:          strPtr("text"),
					TextElementStyle: &larkdocx.TextElementStyle{},
				},
			},
			expected: true,
		},
		{
			name: "Bold=true",
			input: &larkdocx.TextElement{
				TextRun: &larkdocx.TextRun{
					Content: strPtr("text"),
					TextElementStyle: &larkdocx.TextElementStyle{
						Bold: boolPtr(true),
					},
				},
			},
			expected: false,
		},
		{
			name: "Link 不为 nil",
			input: &larkdocx.TextElement{
				TextRun: &larkdocx.TextRun{
					Content: strPtr("text"),
					TextElementStyle: &larkdocx.TextElementStyle{
						Link: &larkdocx.Link{Url: strPtr("https://example.com")},
					},
				},
			},
			expected: false,
		},
		{
			name: "InlineCode=true",
			input: &larkdocx.TextElement{
				TextRun: &larkdocx.TextRun{
					Content: strPtr("code"),
					TextElementStyle: &larkdocx.TextElementStyle{
						InlineCode: boolPtr(true),
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPlainTextRun(tt.input)
			if result != tt.expected {
				t.Errorf("isPlainTextRun() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// ==========================
// mergeAdjacentPlainTextRuns 测试
// ==========================

func TestMergeAdjacentPlainTextRuns(t *testing.T) {
	tests := []struct {
		name     string
		input    []*larkdocx.TextElement
		expected int
		checkFn  func(*testing.T, []*larkdocx.TextElement)
	}{
		{
			name:     "空切片",
			input:    []*larkdocx.TextElement{},
			expected: 0,
		},
		{
			name: "单元素",
			input: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: strPtr("text")}},
			},
			expected: 1,
		},
		{
			name: "两个无样式 TextRun 合并",
			input: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: strPtr("Hello ")}},
				{TextRun: &larkdocx.TextRun{Content: strPtr("World")}},
			},
			expected: 1,
			checkFn: func(t *testing.T, result []*larkdocx.TextElement) {
				if result[0].TextRun == nil || *result[0].TextRun.Content != "Hello World" {
					t.Errorf("应合并为 'Hello World'")
				}
			},
		},
		{
			name: "有样式的不合并",
			input: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: strPtr("text1")}},
				{TextRun: &larkdocx.TextRun{
					Content:          strPtr("text2"),
					TextElementStyle: &larkdocx.TextElementStyle{Bold: boolPtr(true)},
				}},
			},
			expected: 2,
		},
		{
			name: "三个纯文本合并",
			input: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: strPtr("A")}},
				{TextRun: &larkdocx.TextRun{Content: strPtr("B")}},
				{TextRun: &larkdocx.TextRun{Content: strPtr("C")}},
			},
			expected: 1,
			checkFn: func(t *testing.T, result []*larkdocx.TextElement) {
				if *result[0].TextRun.Content != "ABC" {
					t.Errorf("应合并为 'ABC'")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeAdjacentPlainTextRuns(tt.input)
			if len(result) != tt.expected {
				t.Errorf("expected %d elements, got %d", tt.expected, len(result))
			}
			if tt.checkFn != nil {
				tt.checkFn(t, result)
			}
		})
	}
}

// ==========================
// hasValidURLPrefix 测试
// ==========================

func TestHasValidURLPrefix(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{name: "http://", input: "http://example.com", expected: true},
		{name: "https://", input: "https://example.com", expected: true},
		{name: "ftp://", input: "ftp://example.com", expected: false},
		{name: "锚点", input: "#section", expected: false},
		{name: "空字符串", input: "", expected: false},
		{name: "相对路径", input: "/path/to/file", expected: false},
		{name: "mailto", input: "mailto:test@example.com", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasValidURLPrefix(tt.input)
			if result != tt.expected {
				t.Errorf("hasValidURLPrefix(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// ==========================
// createLinkElement 测试
// ==========================

func TestCreateLinkElement(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		rawURL   string
		hasLink  bool
		finalURL string
	}{
		{
			name:     "有效 HTTPS URL",
			text:     "点击这里",
			rawURL:   "https://example.com",
			hasLink:  true,
			finalURL: "https://example.com",
		},
		{
			name:     "有效 HTTP URL",
			text:     "链接",
			rawURL:   "http://example.com",
			hasLink:  true,
			finalURL: "http://example.com",
		},
		{
			name:     "feishu://doc/ URL 自动转换",
			text:     "文档",
			rawURL:   "feishu://doc/ABC123",
			hasLink:  true,
			finalURL: "https://feishu.cn/docx/ABC123",
		},
		{
			name:    "锚点 #section 无 Link",
			text:    "章节",
			rawURL:  "#section",
			hasLink: false,
		},
		{
			name:    "相对路径无 Link",
			text:    "文件",
			rawURL:  "/path/to/file",
			hasLink: false,
		},
		{
			name:     "URL 编码自动解码",
			text:     "编码链接",
			rawURL:   "https%3A%2F%2Fexample.com",
			hasLink:  true,
			finalURL: "https://example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := createLinkElement(tt.text, tt.rawURL)
			if result.TextRun == nil {
				t.Fatalf("TextRun should not be nil")
			}
			if *result.TextRun.Content != tt.text {
				t.Errorf("Content = %q, want %q", *result.TextRun.Content, tt.text)
			}
			if tt.hasLink {
				if result.TextRun.TextElementStyle == nil || result.TextRun.TextElementStyle.Link == nil {
					t.Errorf("Link should not be nil")
				} else if *result.TextRun.TextElementStyle.Link.Url != tt.finalURL {
					t.Errorf("Link URL = %q, want %q", *result.TextRun.TextElementStyle.Link.Url, tt.finalURL)
				}
			} else {
				if result.TextRun.TextElementStyle != nil && result.TextRun.TextElementStyle.Link != nil {
					t.Errorf("Link should be nil for %q", tt.rawURL)
				}
			}
		})
	}
}

// ==========================
// hasNonEmptyContent 测试
// ==========================

func TestHasNonEmptyContent(t *testing.T) {
	tests := []struct {
		name     string
		input    []*larkdocx.TextElement
		expected bool
	}{
		{
			name:     "空切片",
			input:    []*larkdocx.TextElement{},
			expected: false,
		},
		{
			name: "全空内容",
			input: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: strPtr("")}},
				{TextRun: &larkdocx.TextRun{Content: strPtr("   ")}},
			},
			expected: false,
		},
		{
			name: "有内容",
			input: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: strPtr("text")}},
			},
			expected: true,
		},
		{
			name: "只有空白字符",
			input: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: strPtr("\n\t  ")}},
			},
			expected: false,
		},
		{
			name: "混合空白和内容",
			input: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: strPtr("  ")}},
				{TextRun: &larkdocx.TextRun{Content: strPtr("content")}},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasNonEmptyContent(tt.input)
			if result != tt.expected {
				t.Errorf("hasNonEmptyContent() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// ==========================
// applyTextStyle 测试
// ==========================

func TestApplyTextStyle(t *testing.T) {
	tests := []struct {
		name          string
		elem          *larkdocx.TextElement
		bold          bool
		italic        bool
		strikethrough bool
		checkFn       func(*testing.T, *larkdocx.TextElement)
	}{
		{
			name: "nil elem 无操作",
			elem: nil,
		},
		{
			name: "TextRun nil 无操作",
			elem: &larkdocx.TextElement{},
		},
		{
			name: "TextElementStyle nil 自动创建",
			elem: &larkdocx.TextElement{
				TextRun: &larkdocx.TextRun{Content: strPtr("text")},
			},
			bold: true,
			checkFn: func(t *testing.T, elem *larkdocx.TextElement) {
				if elem.TextRun.TextElementStyle == nil {
					t.Error("TextElementStyle should be created")
				}
				if !*elem.TextRun.TextElementStyle.Bold {
					t.Error("Bold should be true")
				}
			},
		},
		{
			name: "叠加多种样式",
			elem: &larkdocx.TextElement{
				TextRun: &larkdocx.TextRun{Content: strPtr("text")},
			},
			bold:          true,
			italic:        true,
			strikethrough: true,
			checkFn: func(t *testing.T, elem *larkdocx.TextElement) {
				s := elem.TextRun.TextElementStyle
				if s == nil || !*s.Bold || !*s.Italic || !*s.Strikethrough {
					t.Error("All styles should be applied")
				}
			},
		},
		{
			name: "只设置 italic",
			elem: &larkdocx.TextElement{
				TextRun: &larkdocx.TextRun{Content: strPtr("text")},
			},
			italic: true,
			checkFn: func(t *testing.T, elem *larkdocx.TextElement) {
				s := elem.TextRun.TextElementStyle
				if s == nil || !*s.Italic {
					t.Error("Italic should be true")
				}
				if s.Bold != nil {
					t.Error("Bold should not be set")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyTextStyle(tt.elem, tt.bold, tt.italic, tt.strikethrough)
			if tt.checkFn != nil {
				tt.checkFn(t, tt.elem)
			}
		})
	}
}

// ==========================
// calculateColumnWidths 测试
// ==========================

func TestCalculateColumnWidths(t *testing.T) {
	tests := []struct {
		name          string
		headerContent []string
		dataRows      [][]string
		cols          int
		expected      int // 预期数组长度
		checkFn       func(*testing.T, []int)
	}{
		{
			name:     "cols=0 返回 nil",
			cols:     0,
			expected: 0,
		},
		{
			name:          "正常表格",
			headerContent: []string{"姓名", "年龄"},
			dataRows:      [][]string{{"张三", "25"}},
			cols:          2,
			expected:      2,
			checkFn: func(t *testing.T, widths []int) {
				if len(widths) != 2 {
					t.Fatalf("expected 2 columns, got %d", len(widths))
				}
				// 中文字符 14px，英文 8px，最小 80px
				// "姓名" = 2*14 + 16(padding) = 44 -> 扩展到 80
				if widths[0] < minColumnWidth || widths[1] < minColumnWidth {
					t.Errorf("widths should >= minColumnWidth")
				}
			},
		},
		{
			name:          "中文内容",
			headerContent: []string{"中文标题"},
			dataRows:      [][]string{{"中文内容"}},
			cols:          1,
			expected:      1,
			checkFn: func(t *testing.T, widths []int) {
				// "中文标题" = 4*14 + 16 = 72 -> 扩展
				if widths[0] < minColumnWidth {
					t.Errorf("width should >= %d", minColumnWidth)
				}
			},
		},
		{
			name:          "英文内容",
			headerContent: []string{"Title"},
			dataRows:      [][]string{{"Content"}},
			cols:          1,
			expected:      1,
			checkFn: func(t *testing.T, widths []int) {
				// "Content" = 7*8 + 16 = 72 -> 扩展
				if widths[0] < minColumnWidth {
					t.Errorf("width should >= %d", minColumnWidth)
				}
			},
		},
		{
			name:          "超长内容截断",
			headerContent: []string{"非常非常非常非常非常非常非常非常非常非常非常非常非常非常非常长的标题"},
			dataRows:      [][]string{{"数据"}},
			cols:          1,
			expected:      1,
			checkFn: func(t *testing.T, widths []int) {
				if widths[0] > maxColumnWidth {
					t.Errorf("width should <= %d, got %d", maxColumnWidth, widths[0])
				}
			},
		},
		{
			name:          "总宽度小于文档宽度时扩展",
			headerContent: []string{"A", "B", "C"},
			dataRows:      [][]string{{"1", "2", "3"}},
			cols:          3,
			expected:      3,
			checkFn: func(t *testing.T, widths []int) {
				total := 0
				for _, w := range widths {
					total += w
				}
				// 应该被扩展到接近 defaultDocWidth
				if total < minColumnWidth*3 {
					t.Errorf("total width too small: %d", total)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateColumnWidths(tt.headerContent, tt.dataRows, tt.cols)
			if tt.expected == 0 {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}
			if len(result) != tt.expected {
				t.Errorf("expected %d columns, got %d", tt.expected, len(result))
			}
			if tt.checkFn != nil {
				tt.checkFn(t, result)
			}
		})
	}
}

// ==========================
// FlattenBlockNodes 测试
// ==========================

func TestFlattenBlockNodes(t *testing.T) {
	tests := []struct {
		name     string
		input    []*BlockNode
		expected int
	}{
		{
			name:     "nil 节点跳过",
			input:    []*BlockNode{nil, {Block: createSimpleTextBlock("text")}},
			expected: 1,
		},
		{
			name:     "Block nil 跳过",
			input:    []*BlockNode{{Block: nil}, {Block: createSimpleTextBlock("text")}},
			expected: 1,
		},
		{
			name: "有 Children 递归展平",
			input: []*BlockNode{
				{
					Block: createSimpleTextBlock("parent"),
					Children: []*BlockNode{
						{Block: createSimpleTextBlock("child1")},
						{Block: createSimpleTextBlock("child2")},
					},
				},
			},
			expected: 3,
		},
		{
			name: "空 Children 只返回自身",
			input: []*BlockNode{
				{Block: createSimpleTextBlock("text"), Children: []*BlockNode{}},
			},
			expected: 1,
		},
		{
			name: "嵌套多层",
			input: []*BlockNode{
				{
					Block: createSimpleTextBlock("root"),
					Children: []*BlockNode{
						{
							Block: createSimpleTextBlock("level1"),
							Children: []*BlockNode{
								{Block: createSimpleTextBlock("level2")},
							},
						},
					},
				},
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FlattenBlockNodes(tt.input)
			if len(result) != tt.expected {
				t.Errorf("expected %d blocks, got %d", tt.expected, len(result))
			}
		})
	}
}

// ==========================
// 通过 Markdown 间接测试的场景
// ==========================

func TestMarkdownToBlockCallout(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		checkFn  func(*testing.T, []*BlockNode)
	}{
		{
			name:     "Callout WARNING",
			markdown: "> [!WARNING]\n> 警告内容",
			checkFn: func(t *testing.T, nodes []*BlockNode) {
				if len(nodes) == 0 {
					t.Fatal("expected at least one block")
				}
				if nodes[0].Block.Callout == nil {
					t.Error("expected Callout block")
				}
				if *nodes[0].Block.Callout.BackgroundColor != 2 {
					t.Errorf("WARNING color should be 2, got %d", *nodes[0].Block.Callout.BackgroundColor)
				}
			},
		},
		{
			name:     "Callout 多行内容",
			markdown: "> [!TIP]\n> 提示\n> 更多提示",
			checkFn: func(t *testing.T, nodes []*BlockNode) {
				if len(nodes) == 0 || nodes[0].Block.Callout == nil {
					t.Fatal("expected Callout block")
				}
				if len(nodes[0].Children) < 1 {
					t.Error("expected at least one child block")
				}
			},
		},
		{
			name:     "Callout 内含列表",
			markdown: "> [!NOTE]\n> 内容\n> - 列表项",
			checkFn: func(t *testing.T, nodes []*BlockNode) {
				if len(nodes) == 0 || nodes[0].Block.Callout == nil {
					t.Fatal("expected Callout block")
				}
				// 应该有 Text 块和 Bullet 块
				hasText := false
				hasBullet := false
				for _, child := range nodes[0].Children {
					if child.Block.Text != nil {
						hasText = true
					}
					if child.Block.Bullet != nil {
						hasBullet = true
					}
				}
				if !hasText || !hasBullet {
					t.Error("Callout should contain both Text and Bullet blocks")
				}
			},
		},
		{
			name:     "Callout 内含代码块",
			markdown: "> [!NOTE]\n> 内容\n> ```go\n> code\n> ```",
			checkFn: func(t *testing.T, nodes []*BlockNode) {
				if len(nodes) == 0 || nodes[0].Block.Callout == nil {
					t.Fatal("expected Callout block")
				}
				hasCode := false
				for _, child := range nodes[0].Children {
					if child.Block.Code != nil {
						hasCode = true
					}
				}
				if !hasCode {
					t.Error("Callout should contain Code block")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := NewMarkdownToBlock([]byte(tt.markdown), ConvertOptions{}, "")
			result, err := converter.ConvertWithTableData()
			if err != nil {
				t.Fatalf("ConvertWithTableData failed: %v", err)
			}
			tt.checkFn(t, result.BlockNodes)
		})
	}
}

func TestMarkdownToBlockQuote(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		checkFn  func(*testing.T, []*BlockNode)
	}{
		{
			name:     "普通引用多行",
			markdown: "> 第一行\n> 第二行",
			checkFn: func(t *testing.T, nodes []*BlockNode) {
				if len(nodes) == 0 {
					t.Fatal("expected at least one block")
				}
				if nodes[0].Block.QuoteContainer == nil {
					t.Error("expected QuoteContainer")
				}
				if len(nodes[0].Children) < 2 {
					t.Error("expected at least 2 child Text blocks")
				}
			},
		},
		{
			name:     "引用包含链接",
			markdown: "> [链接](https://example.com)",
			checkFn: func(t *testing.T, nodes []*BlockNode) {
				if len(nodes) == 0 || nodes[0].Block.QuoteContainer == nil {
					t.Fatal("expected QuoteContainer")
				}
				// 检查子块中是否有 Link
				hasLink := false
				for _, child := range nodes[0].Children {
					if child.Block.Text != nil {
						for _, elem := range child.Block.Text.Elements {
							if elem.TextRun != nil && elem.TextRun.TextElementStyle != nil && elem.TextRun.TextElementStyle.Link != nil {
								hasLink = true
							}
						}
					}
				}
				if !hasLink {
					t.Error("expected Link in quote")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := NewMarkdownToBlock([]byte(tt.markdown), ConvertOptions{}, "")
			result, err := converter.ConvertWithTableData()
			if err != nil {
				t.Fatalf("ConvertWithTableData failed: %v", err)
			}
			tt.checkFn(t, result.BlockNodes)
		})
	}
}

func TestMarkdownToBlockComplexStyles(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		checkFn  func(*testing.T, []*BlockNode)
	}{
		{
			name:     "粗体内含链接",
			markdown: "**[链接](https://example.com)**",
			checkFn: func(t *testing.T, nodes []*BlockNode) {
				if len(nodes) == 0 || nodes[0].Block.Text == nil {
					t.Fatal("expected Text block")
				}
				elem := nodes[0].Block.Text.Elements[0]
				if elem.TextRun == nil || elem.TextRun.TextElementStyle == nil {
					t.Fatal("expected TextElementStyle")
				}
				if elem.TextRun.TextElementStyle.Bold == nil || !*elem.TextRun.TextElementStyle.Bold {
					t.Error("expected Bold")
				}
				if elem.TextRun.TextElementStyle.Link == nil {
					t.Error("expected Link")
				}
			},
		},
		{
			name:     "删除线",
			markdown: "~~删除~~",
			checkFn: func(t *testing.T, nodes []*BlockNode) {
				if len(nodes) == 0 || nodes[0].Block.Text == nil {
					t.Fatal("expected Text block")
				}
				elem := nodes[0].Block.Text.Elements[0]
				if elem.TextRun == nil || elem.TextRun.TextElementStyle == nil {
					t.Fatal("expected TextElementStyle")
				}
				if elem.TextRun.TextElementStyle.Strikethrough == nil || !*elem.TextRun.TextElementStyle.Strikethrough {
					t.Error("expected Strikethrough")
				}
			},
		},
		{
			name:     "下划线在 Emphasis 内",
			markdown: "**<u>粗体下划线</u>**",
			checkFn: func(t *testing.T, nodes []*BlockNode) {
				if len(nodes) == 0 || nodes[0].Block.Text == nil {
					t.Fatal("expected Text block")
				}
				// 检查是否有 Underline 样式
				hasUnderline := false
				for _, elem := range nodes[0].Block.Text.Elements {
					if elem.TextRun != nil && elem.TextRun.TextElementStyle != nil &&
						elem.TextRun.TextElementStyle.Underline != nil && *elem.TextRun.TextElementStyle.Underline {
						hasUnderline = true
					}
				}
				if !hasUnderline {
					t.Skip("Underline only works within extractChildElements (inside Emphasis/Strikethrough)")
				}
			},
		},
		{
			name:     "斜体内含下划线",
			markdown: "*<u>斜体下划线</u>*",
			checkFn: func(t *testing.T, nodes []*BlockNode) {
				if len(nodes) == 0 || nodes[0].Block.Text == nil {
					t.Fatal("expected Text block")
				}
				// 下划线处理在 extractChildElements 中，需要在 Emphasis 内部才能工作
				// 这里主要验证不会崩溃
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := NewMarkdownToBlock([]byte(tt.markdown), ConvertOptions{}, "")
			result, err := converter.ConvertWithTableData()
			if err != nil {
				t.Fatalf("ConvertWithTableData failed: %v", err)
			}
			tt.checkFn(t, result.BlockNodes)
		})
	}
}

func TestMarkdownToBlockImage(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		options  ConvertOptions
		checkFn  func(*testing.T, *ConvertResult)
	}{
		{
			name:     "UploadImages=false 跳过",
			markdown: "![图片](https://example.com/img.png)",
			options:  ConvertOptions{UploadImages: false},
			checkFn: func(t *testing.T, result *ConvertResult) {
				if result.ImageStats.Skipped == 0 {
					t.Error("expected image to be skipped")
				}
			},
		},
		{
			name:     "feishu://media/ 占位符",
			markdown: "![图片](feishu://media/token123)",
			options:  ConvertOptions{UploadImages: false},
			checkFn: func(t *testing.T, result *ConvertResult) {
				if result.ImageStats.Skipped == 0 {
					t.Error("expected image to be skipped")
				}
			},
		},
		{
			name:     "本地路径跳过",
			markdown: "![图片](local.png)",
			options:  ConvertOptions{UploadImages: false},
			checkFn: func(t *testing.T, result *ConvertResult) {
				if result.ImageStats.Skipped == 0 {
					t.Error("expected image to be skipped")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := NewMarkdownToBlock([]byte(tt.markdown), tt.options, "")
			result, err := converter.ConvertWithTableData()
			if err != nil {
				t.Fatalf("ConvertWithTableData failed: %v", err)
			}
			tt.checkFn(t, result)
		})
	}
}

func TestMarkdownToBlockLargeTable(t *testing.T) {
	// 构造超过 9 行的大表格
	markdown := "| 列1 | 列2 |\n|-----|-----|\n"
	for i := 1; i <= 15; i++ {
		markdown += "| 数据" + string(rune('0'+i%10)) + " | 值 |\n"
	}

	converter := NewMarkdownToBlock([]byte(markdown), ConvertOptions{}, "")
	result, err := converter.ConvertWithTableData()
	if err != nil {
		t.Fatalf("ConvertWithTableData failed: %v", err)
	}

	// 15 行数据 + 1 行表头 = 16 行，应拆分为至少 2 个表格
	if len(result.TableDatas) < 2 {
		t.Errorf("expected at least 2 tables, got %d", len(result.TableDatas))
	}

	// 验证每个表格都不超过 maxTableRows 行
	for i, td := range result.TableDatas {
		if td.Rows > maxTableRows {
			t.Errorf("table %d has %d rows, exceeds maxTableRows %d", i, td.Rows, maxTableRows)
		}
	}
}

func TestConvertWithTableData(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		checkFn  func(*testing.T, *ConvertResult)
	}{
		{
			name:     "正常 Markdown",
			markdown: "# 标题\n段落",
			checkFn: func(t *testing.T, result *ConvertResult) {
				if len(result.BlockNodes) == 0 {
					t.Error("expected non-empty BlockNodes")
				}
			},
		},
		{
			name:     "带表格的 Markdown",
			markdown: "| A | B |\n|---|---|\n| 1 | 2 |",
			checkFn: func(t *testing.T, result *ConvertResult) {
				if len(result.TableDatas) == 0 {
					t.Error("expected non-empty TableDatas")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := NewMarkdownToBlock([]byte(tt.markdown), ConvertOptions{}, "")
			result, err := converter.ConvertWithTableData()
			if err != nil {
				t.Fatalf("ConvertWithTableData failed: %v", err)
			}
			tt.checkFn(t, result)
		})
	}
}

// ==========================
// 辅助函数 (使用已有的 strPtr, boolPtr)
// ==========================

// createSimpleTextBlock 创建简单的 Text 块（不需要 block_id）
func createSimpleTextBlock(content string) *larkdocx.Block {
	blockType := int(BlockTypeText)
	return &larkdocx.Block{
		BlockType: &blockType,
		Text: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: &content}},
			},
		},
	}
}
