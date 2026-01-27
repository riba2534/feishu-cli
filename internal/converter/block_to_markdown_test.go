package converter

import (
	"strings"
	"testing"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
)

// 辅助函数：创建字符串指针
func strPtr(s string) *string {
	return &s
}

// 辅助函数：创建布尔指针
func boolPtr(b bool) *bool {
	return &b
}

// 创建简单文本块
func createTextBlock(id string, content string) *larkdocx.Block {
	blockType := int(BlockTypeText)
	return &larkdocx.Block{
		BlockId:   strPtr(id),
		BlockType: &blockType,
		Text: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{
					TextRun: &larkdocx.TextRun{
						Content: strPtr(content),
					},
				},
			},
		},
	}
}

// 创建标题块
func createHeadingBlock(id string, level int, content string) *larkdocx.Block {
	blockType := int(BlockTypeHeading1) + level - 1
	headingText := &larkdocx.Text{
		Elements: []*larkdocx.TextElement{
			{
				TextRun: &larkdocx.TextRun{
					Content: strPtr(content),
				},
			},
		},
	}

	block := &larkdocx.Block{
		BlockId:   strPtr(id),
		BlockType: &blockType,
	}

	switch level {
	case 1:
		block.Heading1 = headingText
	case 2:
		block.Heading2 = headingText
	case 3:
		block.Heading3 = headingText
	case 4:
		block.Heading4 = headingText
	case 5:
		block.Heading5 = headingText
	case 6:
		block.Heading6 = headingText
	case 7:
		block.Heading7 = headingText
	case 8:
		block.Heading8 = headingText
	case 9:
		block.Heading9 = headingText
	}

	return block
}

// 创建无序列表块
func createBulletBlock(id string, content string) *larkdocx.Block {
	blockType := int(BlockTypeBullet)
	return &larkdocx.Block{
		BlockId:   strPtr(id),
		BlockType: &blockType,
		Bullet: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{
					TextRun: &larkdocx.TextRun{
						Content: strPtr(content),
					},
				},
			},
		},
	}
}

// 创建有序列表块
func createOrderedBlock(id string, content string) *larkdocx.Block {
	blockType := int(BlockTypeOrdered)
	return &larkdocx.Block{
		BlockId:   strPtr(id),
		BlockType: &blockType,
		Ordered: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{
					TextRun: &larkdocx.TextRun{
						Content: strPtr(content),
					},
				},
			},
		},
	}
}

// 创建代码块
func createCodeBlock(id string, language int, content string) *larkdocx.Block {
	blockType := int(BlockTypeCode)
	return &larkdocx.Block{
		BlockId:   strPtr(id),
		BlockType: &blockType,
		Code: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{
					TextRun: &larkdocx.TextRun{
						Content: strPtr(content),
					},
				},
			},
			Style: &larkdocx.TextStyle{
				Language: &language,
			},
		},
	}
}

// 创建引用块
func createQuoteBlock(id string, content string) *larkdocx.Block {
	blockType := int(BlockTypeQuote)
	return &larkdocx.Block{
		BlockId:   strPtr(id),
		BlockType: &blockType,
		Quote: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{
					TextRun: &larkdocx.TextRun{
						Content: strPtr(content),
					},
				},
			},
		},
	}
}

// 创建待办块
func createTodoBlock(id string, content string, done bool) *larkdocx.Block {
	blockType := int(BlockTypeTodo)
	return &larkdocx.Block{
		BlockId:   strPtr(id),
		BlockType: &blockType,
		Todo: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{
					TextRun: &larkdocx.TextRun{
						Content: strPtr(content),
					},
				},
			},
			Style: &larkdocx.TextStyle{
				Done: &done,
			},
		},
	}
}

// 创建分割线块
func createDividerBlock(id string) *larkdocx.Block {
	blockType := int(BlockTypeDivider)
	return &larkdocx.Block{
		BlockId:   strPtr(id),
		BlockType: &blockType,
	}
}

// 创建公式块
func createEquationBlock(id string, formula string) *larkdocx.Block {
	blockType := int(BlockTypeEquation)
	return &larkdocx.Block{
		BlockId:   strPtr(id),
		BlockType: &blockType,
		Equation: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{
					TextRun: &larkdocx.TextRun{
						Content: strPtr(formula),
					},
				},
			},
		},
	}
}

func TestNewBlockToMarkdown(t *testing.T) {
	blocks := []*larkdocx.Block{
		createTextBlock("block1", "Hello"),
		createTextBlock("block2", "World"),
	}

	converter := NewBlockToMarkdown(blocks, ConvertOptions{})

	if converter == nil {
		t.Fatal("NewBlockToMarkdown() 返回 nil")
	}

	if len(converter.blocks) != 2 {
		t.Errorf("blocks 数量 = %d, 期望 2", len(converter.blocks))
	}

	if len(converter.blockMap) != 2 {
		t.Errorf("blockMap 数量 = %d, 期望 2", len(converter.blockMap))
	}
}

func TestNewBlockToMarkdown_NilBlocks(t *testing.T) {
	converter := NewBlockToMarkdown(nil, ConvertOptions{})

	if converter == nil {
		t.Fatal("NewBlockToMarkdown(nil) 返回 nil")
	}

	if len(converter.blocks) != 0 {
		t.Errorf("blocks 数量 = %d, 期望 0", len(converter.blocks))
	}
}

func TestBlockToMd_TextBlock(t *testing.T) {
	blocks := []*larkdocx.Block{
		createTextBlock("block1", "Hello World"),
	}

	converter := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if !strings.Contains(result, "Hello World") {
		t.Errorf("结果不包含预期内容: %s", result)
	}
}

func TestBlockToMd_HeadingBlocks(t *testing.T) {
	tests := []struct {
		level    int
		expected string
	}{
		{1, "# "},
		{2, "## "},
		{3, "### "},
		{4, "#### "},
		{5, "##### "},
		{6, "###### "},
		{7, "###### "}, // 级别 7-9 应限制为 6
		{8, "###### "},
		{9, "###### "},
	}

	for _, tt := range tests {
		t.Run(string(rune('0'+tt.level)), func(t *testing.T) {
			blocks := []*larkdocx.Block{
				createHeadingBlock("block1", tt.level, "标题内容"),
			}

			converter := NewBlockToMarkdown(blocks, ConvertOptions{})
			result, err := converter.Convert()

			if err != nil {
				t.Fatalf("Convert() 返回错误: %v", err)
			}

			if !strings.HasPrefix(result, tt.expected) {
				t.Errorf("结果 = %q, 期望以 %q 开头", result, tt.expected)
			}
		})
	}
}

func TestBlockToMd_BulletList(t *testing.T) {
	blocks := []*larkdocx.Block{
		createBulletBlock("block1", "项目一"),
		createBulletBlock("block2", "项目二"),
	}

	converter := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if !strings.Contains(result, "- 项目一") {
		t.Errorf("结果不包含 '- 项目一': %s", result)
	}
	if !strings.Contains(result, "- 项目二") {
		t.Errorf("结果不包含 '- 项目二': %s", result)
	}
}

func TestBlockToMd_OrderedList(t *testing.T) {
	blocks := []*larkdocx.Block{
		createOrderedBlock("block1", "第一项"),
		createOrderedBlock("block2", "第二项"),
	}

	converter := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if !strings.Contains(result, "1. 第一项") {
		t.Errorf("结果不包含 '1. 第一项': %s", result)
	}
	if !strings.Contains(result, "1. 第二项") {
		t.Errorf("结果不包含 '1. 第二项': %s", result)
	}
}

func TestBlockToMd_CodeBlock(t *testing.T) {
	// language code 22 = Go
	blocks := []*larkdocx.Block{
		createCodeBlock("block1", 22, "fmt.Println(\"Hello\")"),
	}

	converter := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if !strings.Contains(result, "```go") {
		t.Errorf("结果不包含代码块语言标记: %s", result)
	}
	if !strings.Contains(result, "fmt.Println") {
		t.Errorf("结果不包含代码内容: %s", result)
	}
	if !strings.Contains(result, "```") {
		t.Errorf("结果不包含代码块结束标记: %s", result)
	}
}

func TestBlockToMd_QuoteBlock(t *testing.T) {
	blocks := []*larkdocx.Block{
		createQuoteBlock("block1", "这是一段引用"),
	}

	converter := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if !strings.Contains(result, "> 这是一段引用") {
		t.Errorf("结果不包含引用格式: %s", result)
	}
}

func TestBlockToMd_TodoBlock(t *testing.T) {
	tests := []struct {
		name     string
		done     bool
		expected string
	}{
		{"未完成", false, "- [ ] "},
		{"已完成", true, "- [x] "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks := []*larkdocx.Block{
				createTodoBlock("block1", "任务内容", tt.done),
			}

			converter := NewBlockToMarkdown(blocks, ConvertOptions{})
			result, err := converter.Convert()

			if err != nil {
				t.Fatalf("Convert() 返回错误: %v", err)
			}

			if !strings.Contains(result, tt.expected) {
				t.Errorf("结果 = %q, 期望包含 %q", result, tt.expected)
			}
		})
	}
}

func TestBlockToMd_DividerBlock(t *testing.T) {
	blocks := []*larkdocx.Block{
		createDividerBlock("block1"),
	}

	converter := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if !strings.Contains(result, "---") {
		t.Errorf("结果不包含分割线: %s", result)
	}
}

func TestBlockToMd_EquationBlock(t *testing.T) {
	blocks := []*larkdocx.Block{
		createEquationBlock("block1", "E = mc^2"),
	}

	converter := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if !strings.Contains(result, "$$") {
		t.Errorf("结果不包含公式标记: %s", result)
	}
	if !strings.Contains(result, "E = mc^2") {
		t.Errorf("结果不包含公式内容: %s", result)
	}
}

func TestBlockToMd_EmptyBlocks(t *testing.T) {
	blocks := []*larkdocx.Block{}

	converter := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if result != "\n" {
		t.Errorf("空块列表的结果 = %q, 期望 %q", result, "\n")
	}
}

func TestBlockToMd_SkipsPageBlock(t *testing.T) {
	pageType := int(BlockTypePage)
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("page1"),
			BlockType: &pageType,
		},
		createTextBlock("block1", "内容"),
	}

	converter := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	// Page 块应该被跳过
	if strings.Contains(result, "page") {
		t.Errorf("结果不应包含 page 块")
	}
	if !strings.Contains(result, "内容") {
		t.Errorf("结果应包含文本内容")
	}
}

func TestBlockToMd_UnknownBlockType(t *testing.T) {
	unknownType := 999
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("unknown1"),
			BlockType: &unknownType,
		},
	}

	converter := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	// 未知块类型应转为注释
	if !strings.Contains(result, "<!-- Unknown block type") {
		t.Errorf("未知块类型应转为注释: %s", result)
	}
}

func TestBlockToMd_NilBlockType(t *testing.T) {
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("block1"),
			BlockType: nil, // nil block type
		},
	}

	converter := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	// nil block type 应被跳过
	if result != "\n" {
		t.Errorf("nil block type 的结果 = %q", result)
	}
}

func TestBlockToMd_TextWithStyles(t *testing.T) {
	blockType := int(BlockTypeText)
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("block1"),
			BlockType: &blockType,
			Text: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{
						TextRun: &larkdocx.TextRun{
							Content: strPtr("粗体文本"),
							TextElementStyle: &larkdocx.TextElementStyle{
								Bold: boolPtr(true),
							},
						},
					},
				},
			},
		},
	}

	converter := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if !strings.Contains(result, "**粗体文本**") {
		t.Errorf("结果不包含粗体格式: %s", result)
	}
}

func TestBlockToMd_TextWithItalic(t *testing.T) {
	blockType := int(BlockTypeText)
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("block1"),
			BlockType: &blockType,
			Text: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{
						TextRun: &larkdocx.TextRun{
							Content: strPtr("斜体文本"),
							TextElementStyle: &larkdocx.TextElementStyle{
								Italic: boolPtr(true),
							},
						},
					},
				},
			},
		},
	}

	converter := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if !strings.Contains(result, "*斜体文本*") {
		t.Errorf("结果不包含斜体格式: %s", result)
	}
}

func TestBlockToMd_TextWithStrikethrough(t *testing.T) {
	blockType := int(BlockTypeText)
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("block1"),
			BlockType: &blockType,
			Text: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{
						TextRun: &larkdocx.TextRun{
							Content: strPtr("删除线文本"),
							TextElementStyle: &larkdocx.TextElementStyle{
								Strikethrough: boolPtr(true),
							},
						},
					},
				},
			},
		},
	}

	converter := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if !strings.Contains(result, "~~删除线文本~~") {
		t.Errorf("结果不包含删除线格式: %s", result)
	}
}

func TestBlockToMd_TextWithInlineCode(t *testing.T) {
	blockType := int(BlockTypeText)
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("block1"),
			BlockType: &blockType,
			Text: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{
						TextRun: &larkdocx.TextRun{
							Content: strPtr("code"),
							TextElementStyle: &larkdocx.TextElementStyle{
								InlineCode: boolPtr(true),
							},
						},
					},
				},
			},
		},
	}

	converter := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if !strings.Contains(result, "`code`") {
		t.Errorf("结果不包含行内代码格式: %s", result)
	}
}

func TestBlockToMd_TextWithLink(t *testing.T) {
	blockType := int(BlockTypeText)
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("block1"),
			BlockType: &blockType,
			Text: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{
						TextRun: &larkdocx.TextRun{
							Content: strPtr("链接文本"),
							TextElementStyle: &larkdocx.TextElementStyle{
								Link: &larkdocx.Link{
									Url: strPtr("https://example.com"),
								},
							},
						},
					},
				},
			},
		},
	}

	converter := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if !strings.Contains(result, "[链接文本](https://example.com)") {
		t.Errorf("结果不包含链接格式: %s", result)
	}
}

func TestBlockToMd_MultipleElements(t *testing.T) {
	blockType := int(BlockTypeText)
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("block1"),
			BlockType: &blockType,
			Text: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{
						TextRun: &larkdocx.TextRun{
							Content: strPtr("普通文本"),
						},
					},
					{
						TextRun: &larkdocx.TextRun{
							Content: strPtr("粗体"),
							TextElementStyle: &larkdocx.TextElementStyle{
								Bold: boolPtr(true),
							},
						},
					},
					{
						TextRun: &larkdocx.TextRun{
							Content: strPtr("更多文本"),
						},
					},
				},
			},
		},
	}

	converter := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if !strings.Contains(result, "普通文本") {
		t.Errorf("结果不包含普通文本")
	}
	if !strings.Contains(result, "**粗体**") {
		t.Errorf("结果不包含粗体文本")
	}
	if !strings.Contains(result, "更多文本") {
		t.Errorf("结果不包含更多文本")
	}
}

func TestLanguageCodeToName(t *testing.T) {
	tests := []struct {
		code     int
		expected string
	}{
		{1, "plaintext"},
		{2, "abap"},
		{22, "go"},
		{47, "python"},
		{29, "java"},
		{30, "javascript"},
		{60, "typescript"},
		{0, "plaintext"},   // 未知代码返回 plaintext
		{999, "plaintext"}, // 未知代码返回 plaintext
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := languageCodeToName(tt.code)
			if result != tt.expected {
				t.Errorf("languageCodeToName(%d) = %q, 期望 %q", tt.code, result, tt.expected)
			}
		})
	}
}

func TestBlockToMd_MixedContent(t *testing.T) {
	blocks := []*larkdocx.Block{
		createHeadingBlock("h1", 1, "标题"),
		createTextBlock("p1", "段落内容"),
		createBulletBlock("li1", "列表项"),
		createDividerBlock("div1"),
		createCodeBlock("code1", 22, "fmt.Println()"),
	}

	converter := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	// 验证所有内容都存在
	if !strings.Contains(result, "# 标题") {
		t.Error("缺少标题")
	}
	if !strings.Contains(result, "段落内容") {
		t.Error("缺少段落")
	}
	if !strings.Contains(result, "- 列表项") {
		t.Error("缺少列表项")
	}
	if !strings.Contains(result, "---") {
		t.Error("缺少分割线")
	}
	if !strings.Contains(result, "```go") {
		t.Error("缺少代码块")
	}
}
