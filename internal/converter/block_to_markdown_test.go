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
	// 使用一个不在映射中的类型值
	unknownType := 888
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

	// 未知块类型应转为含可读名称的注释
	if !strings.Contains(result, "<!-- 不支持的块类型: Unknown(888) (type=888) -->") {
		t.Errorf("未知块类型应转为含名称的注释: %s", result)
	}
}

func TestBlockToMd_UndefinedBlockType(t *testing.T) {
	// BlockTypeUndefined (999) 也应该输出有意义的注释
	undefinedType := int(BlockTypeUndefined)
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("undef1"),
			BlockType: &undefinedType,
		},
	}

	converter := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if !strings.Contains(result, "Undefined") {
		t.Errorf("Undefined 类型应包含名称: %s", result)
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

// intPtr 辅助函数
func intPtr(i int) *int {
	return &i
}

// TestBlockToMd_ISVBlock 测试 ISV 块导出
func TestBlockToMd_ISVBlock(t *testing.T) {
	tests := []struct {
		name           string
		typeID         string
		compID         string
		expectContains string
	}{
		{
			"TextDrawing",
			ISVTypeTextDrawing,
			"comp123",
			"```mermaid",
		},
		{
			"Timeline",
			ISVTypeTimeline,
			"comp456",
			"timeline",
		},
		{
			"Unknown ISV",
			"blk_unknown",
			"comp789",
			"ISV 应用块",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blockType := int(BlockTypeISV)
			block := &larkdocx.Block{
				BlockId:   strPtr("isv1"),
				BlockType: &blockType,
				Isv: &larkdocx.Isv{
					ComponentTypeId: strPtr(tt.typeID),
					ComponentId:     strPtr(tt.compID),
				},
			}

			pageType := int(BlockTypePage)
			blocks := []*larkdocx.Block{
				{BlockId: strPtr("page"), BlockType: &pageType, Children: []string{"isv1"}},
				block,
			}
			conv := NewBlockToMarkdown(blocks, ConvertOptions{})
			result, err := conv.Convert()
			if err != nil {
				t.Fatalf("转换失败: %v", err)
			}
			if !strings.Contains(result, tt.expectContains) {
				t.Errorf("ISV %s 输出不包含 %q:\n%s", tt.name, tt.expectContains, result)
			}
		})
	}
}

// TestBlockToMd_HeadingAutoSeq 测试标题自动编号
func TestBlockToMd_HeadingAutoSeq(t *testing.T) {
	// 构造带 Sequence="auto" 的标题块
	createSeqHeading := func(id string, level int, content string, seq string) *larkdocx.Block {
		block := createHeadingBlock(id, level, content)
		// 需要设置 Style.Sequence
		headingText := &larkdocx.Text{
			Style: &larkdocx.TextStyle{
				Sequence: strPtr(seq),
			},
			Elements: []*larkdocx.TextElement{
				{
					TextRun: &larkdocx.TextRun{
						Content: strPtr(content),
					},
				},
			},
		}
		switch level {
		case 1:
			block.Heading1 = headingText
		case 2:
			block.Heading2 = headingText
		case 3:
			block.Heading3 = headingText
		}
		return block
	}

	pageType := int(BlockTypePage)
	blocks := []*larkdocx.Block{
		{BlockId: strPtr("page"), BlockType: &pageType, Children: []string{"h1", "h2", "h3"}},
		createSeqHeading("h1", 1, "第一章", "auto"),
		createSeqHeading("h2", 1, "第二章", "auto"),
		createSeqHeading("h3", 1, "第三章", "5"), // 手动编号
	}

	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := conv.Convert()
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}

	if !strings.Contains(result, "# 1. 第一章") {
		t.Errorf("第一章编号不正确:\n%s", result)
	}
	if !strings.Contains(result, "# 2. 第二章") {
		t.Errorf("第二章编号不正确:\n%s", result)
	}
	if !strings.Contains(result, "# 5. 第三章") {
		t.Errorf("第三章编号不正确:\n%s", result)
	}
}

// TestBlockToMd_HighlightColor 测试文本高亮颜色导出
func TestBlockToMd_HighlightColor(t *testing.T) {
	textColor := 1 // Red
	bgColor := 5   // LightBlue

	blockType := int(BlockTypeText)
	block := &larkdocx.Block{
		BlockId:   strPtr("t1"),
		BlockType: &blockType,
		Text: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{
					TextRun: &larkdocx.TextRun{
						Content: strPtr("红色文本"),
						TextElementStyle: &larkdocx.TextElementStyle{
							TextColor: &textColor,
						},
					},
				},
				{
					TextRun: &larkdocx.TextRun{
						Content: strPtr("蓝底文本"),
						TextElementStyle: &larkdocx.TextElementStyle{
							BackgroundColor: &bgColor,
						},
					},
				},
				{
					TextRun: &larkdocx.TextRun{
						Content: strPtr("双色文本"),
						TextElementStyle: &larkdocx.TextElementStyle{
							TextColor:       &textColor,
							BackgroundColor: &bgColor,
						},
					},
				},
			},
		},
	}

	pageType := int(BlockTypePage)
	blocks := []*larkdocx.Block{
		{BlockId: strPtr("page"), BlockType: &pageType, Children: []string{"t1"}},
		block,
	}

	// 不开启 highlight 时，不应有 span
	t.Run("without_highlight", func(t *testing.T) {
		conv := NewBlockToMarkdown(blocks, ConvertOptions{Highlight: false})
		result, err := conv.Convert()
		if err != nil {
			t.Fatalf("转换失败: %v", err)
		}
		if strings.Contains(result, "<span") {
			t.Errorf("不开启 highlight 时不应包含 span:\n%s", result)
		}
	})

	// 开启 highlight
	t.Run("with_highlight", func(t *testing.T) {
		conv := NewBlockToMarkdown(blocks, ConvertOptions{Highlight: true})
		result, err := conv.Convert()
		if err != nil {
			t.Fatalf("转换失败: %v", err)
		}

		// 红色文本
		if !strings.Contains(result, `color: #ef4444`) {
			t.Errorf("红色文本颜色丢失:\n%s", result)
		}
		// 蓝底文本
		if !strings.Contains(result, `background-color: #eff6ff`) {
			t.Errorf("蓝底背景色丢失:\n%s", result)
		}
		// 双色文本应同时包含 color 和 background-color
		if !strings.Contains(result, `color: #ef4444; background-color: #eff6ff`) {
			t.Errorf("双色文本样式不正确:\n%s", result)
		}
	})
}

// TestBlockToMd_HighlightNoColor 测试无颜色时不输出 span
func TestBlockToMd_HighlightNoColor(t *testing.T) {
	zeroColor := 0
	blockType := int(BlockTypeText)
	block := &larkdocx.Block{
		BlockId:   strPtr("t1"),
		BlockType: &blockType,
		Text: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{
					TextRun: &larkdocx.TextRun{
						Content: strPtr("普通文本"),
						TextElementStyle: &larkdocx.TextElementStyle{
							TextColor:       &zeroColor,
							BackgroundColor: &zeroColor,
						},
					},
				},
			},
		},
	}

	pageType := int(BlockTypePage)
	blocks := []*larkdocx.Block{
		{BlockId: strPtr("page"), BlockType: &pageType, Children: []string{"t1"}},
		block,
	}

	conv := NewBlockToMarkdown(blocks, ConvertOptions{Highlight: true})
	result, err := conv.Convert()
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if strings.Contains(result, "<span") {
		t.Errorf("颜色值为 0 时不应输出 span:\n%s", result)
	}
}

// --- MentionUser 展开测试 ---

// mockUserResolver 实现 UserResolver 接口用于测试
type mockUserResolver struct {
	users map[string]MentionUserInfo
}

func (m *mockUserResolver) BatchResolve(userIDs []string) map[string]MentionUserInfo {
	result := make(map[string]MentionUserInfo)
	for _, id := range userIDs {
		if info, ok := m.users[id]; ok {
			result[id] = info
		}
	}
	return result
}

// 创建包含 MentionUser 的文本块
func createMentionUserBlock(id, userID string) *larkdocx.Block {
	blockType := int(BlockTypeText)
	return &larkdocx.Block{
		BlockId:   strPtr(id),
		BlockType: &blockType,
		Text: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{
					TextRun: &larkdocx.TextRun{
						Content: strPtr("你好 "),
					},
				},
				{
					MentionUser: &larkdocx.MentionUser{
						UserId: strPtr(userID),
					},
				},
				{
					TextRun: &larkdocx.TextRun{
						Content: strPtr(" 请查看"),
					},
				},
			},
		},
	}
}

func TestBlockToMd_MentionUserExpanded(t *testing.T) {
	blocks := []*larkdocx.Block{
		createMentionUserBlock("block1", "ou_123"),
	}

	resolver := &mockUserResolver{
		users: map[string]MentionUserInfo{
			"ou_123": {Name: "张三", Email: "zhangsan@example.com"},
		},
	}

	conv := NewBlockToMarkdownWithResolver(blocks, ConvertOptions{ExpandMentions: true}, resolver)
	result, err := conv.Convert()
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}

	if !strings.Contains(result, "[@张三](mailto:zhangsan@example.com)") {
		t.Errorf("展开的 mention 应包含 mailto 链接: %s", result)
	}
}

func TestBlockToMd_MentionUserNoEmail(t *testing.T) {
	blocks := []*larkdocx.Block{
		createMentionUserBlock("block1", "ou_456"),
	}

	resolver := &mockUserResolver{
		users: map[string]MentionUserInfo{
			"ou_456": {Name: "李四", Email: ""},
		},
	}

	conv := NewBlockToMarkdownWithResolver(blocks, ConvertOptions{ExpandMentions: true}, resolver)
	result, err := conv.Convert()
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}

	if !strings.Contains(result, "@李四") {
		t.Errorf("无邮箱时应输出 @名字: %s", result)
	}
	if strings.Contains(result, "mailto:") {
		t.Errorf("无邮箱时不应包含 mailto: %s", result)
	}
}

func TestBlockToMd_MentionUserFallback(t *testing.T) {
	blocks := []*larkdocx.Block{
		createMentionUserBlock("block1", "ou_unknown"),
	}

	resolver := &mockUserResolver{
		users: map[string]MentionUserInfo{}, // 空映射，不包含该用户
	}

	conv := NewBlockToMarkdownWithResolver(blocks, ConvertOptions{ExpandMentions: true}, resolver)
	result, err := conv.Convert()
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}

	if !strings.Contains(result, "@[user:ou_unknown]") {
		t.Errorf("找不到用户时应降级为原始格式: %s", result)
	}
}

func TestBlockToMd_MentionDisabled(t *testing.T) {
	blocks := []*larkdocx.Block{
		createMentionUserBlock("block1", "ou_123"),
	}

	// ExpandMentions=false，不传 resolver
	conv := NewBlockToMarkdown(blocks, ConvertOptions{ExpandMentions: false})
	result, err := conv.Convert()
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}

	if !strings.Contains(result, "@[user:ou_123]") {
		t.Errorf("ExpandMentions=false 应保持原始格式: %s", result)
	}
}

func TestCollectMentionUserIDs(t *testing.T) {
	blocks := []*larkdocx.Block{
		createMentionUserBlock("block1", "ou_aaa"),
		createMentionUserBlock("block2", "ou_bbb"),
		createMentionUserBlock("block3", "ou_aaa"), // 重复 ID
	}

	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	ids := conv.collectMentionUserIDs()

	// 验证去重
	if len(ids) != 2 {
		t.Errorf("collectMentionUserIDs 应去重，期望 2 个 ID，得到 %d: %v", len(ids), ids)
	}

	idSet := make(map[string]bool)
	for _, id := range ids {
		idSet[id] = true
	}
	if !idSet["ou_aaa"] || !idSet["ou_bbb"] {
		t.Errorf("缺少预期的 user ID: %v", ids)
	}
}

// --- 新块类型测试 ---

func TestBlockToMd_Agenda(t *testing.T) {
	pageType := int(BlockTypePage)
	agendaType := int(BlockTypeAgenda)
	agendaItemType := int(BlockTypeAgendaItem)
	agendaItemTitleType := int(BlockTypeAgendaItemTitle)

	blocks := []*larkdocx.Block{
		{BlockId: strPtr("page"), BlockType: &pageType, Children: []string{"agenda1"}},
		{
			BlockId:   strPtr("agenda1"),
			BlockType: &agendaType,
			Children:  []string{"item1"},
		},
		{
			BlockId:   strPtr("item1"),
			BlockType: &agendaItemType,
			Children:  []string{"title1"},
		},
		{
			BlockId:   strPtr("title1"),
			BlockType: &agendaItemTitleType,
			Text: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{TextRun: &larkdocx.TextRun{Content: strPtr("议程标题")}},
				},
			},
		},
	}

	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := conv.Convert()
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}

	if !strings.Contains(result, "---") {
		t.Errorf("Agenda 块应输出分隔线: %s", result)
	}
	if !strings.Contains(result, "**议程标题**") {
		t.Errorf("AgendaItemTitle 应加粗输出: %s", result)
	}
}

func TestBlockToMd_LinkPreview(t *testing.T) {
	linkPreviewType := int(BlockTypeLinkPreview)
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("lp1"),
			BlockType: &linkPreviewType,
		},
	}

	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := conv.Convert()
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}

	if !strings.Contains(result, "[链接预览]") {
		t.Errorf("无子块的 LinkPreview 应输出占位符: %s", result)
	}
}

func TestBlockToMd_SyncReference(t *testing.T) {
	pageType := int(BlockTypePage)
	syncRefType := int(BlockTypeSyncReference)
	textType := int(BlockTypeText)

	blocks := []*larkdocx.Block{
		{BlockId: strPtr("page"), BlockType: &pageType, Children: []string{"sync1"}},
		{
			BlockId:   strPtr("sync1"),
			BlockType: &syncRefType,
			Children:  []string{"child1"},
		},
		{
			BlockId:   strPtr("child1"),
			BlockType: &textType,
			Text: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{TextRun: &larkdocx.TextRun{Content: strPtr("同步内容")}},
				},
			},
		},
	}

	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := conv.Convert()
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}

	if !strings.Contains(result, "同步内容") {
		t.Errorf("SyncReference 应展开子块内容: %s", result)
	}
}

func TestBlockToMd_WikiCatalogV2(t *testing.T) {
	wikiCatType := int(BlockTypeWikiCatalogV2)
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("wc1"),
			BlockType: &wikiCatType,
		},
	}

	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := conv.Convert()
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}

	if !strings.Contains(result, "[知识库目录 V2]") {
		t.Errorf("WikiCatalogV2 输出不正确: %s", result)
	}
}

func TestBlockToMd_AITemplate(t *testing.T) {
	aiType := int(BlockTypeAITemplate)
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("ai1"),
			BlockType: &aiType,
		},
	}

	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := conv.Convert()
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}

	if !strings.Contains(result, "<!-- AI 模板块 -->") {
		t.Errorf("AITemplate 应输出 HTML 注释: %s", result)
	}
}

func TestBlockTypeName(t *testing.T) {
	tests := []struct {
		bt       BlockType
		expected string
	}{
		{BlockTypePage, "Page"},
		{BlockTypeText, "Text"},
		{BlockTypeHeading1, "Heading1"},
		{BlockTypeBullet, "Bullet"},
		{BlockTypeAddOns, "AddOns"},
		{BlockTypeAgenda, "Agenda"},
		{BlockTypeLinkPreview, "LinkPreview"},
		{BlockTypeSyncSource, "SyncSource"},
		{BlockTypeAITemplate, "AITemplate"},
		{BlockTypeUndefined, "Undefined"},
		{BlockType(888), "Unknown(888)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := BlockTypeName(tt.bt)
			if result != tt.expected {
				t.Errorf("BlockTypeName(%d) = %q, 期望 %q", int(tt.bt), result, tt.expected)
			}
		})
	}
}
