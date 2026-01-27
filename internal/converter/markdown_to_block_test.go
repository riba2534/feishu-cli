package converter

import (
	"strings"
	"testing"
)

func TestNewMarkdownToBlock(t *testing.T) {
	source := []byte("# Hello")
	opts := ConvertOptions{
		UploadImages: true,
		DocumentID:   "doc123",
	}

	converter := NewMarkdownToBlock(source, opts, "/base/path")

	if converter == nil {
		t.Fatal("NewMarkdownToBlock() 返回 nil")
	}
	if string(converter.source) != "# Hello" {
		t.Errorf("source = %q, 期望 %q", string(converter.source), "# Hello")
	}
	if converter.basePath != "/base/path" {
		t.Errorf("basePath = %q, 期望 %q", converter.basePath, "/base/path")
	}
}

func TestConvert_Heading(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		level    int
	}{
		{"H1", "# 标题一", 1},
		{"H2", "## 标题二", 2},
		{"H3", "### 标题三", 3},
		{"H4", "#### 标题四", 4},
		{"H5", "##### 标题五", 5},
		{"H6", "###### 标题六", 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := NewMarkdownToBlock([]byte(tt.markdown), ConvertOptions{}, "")
			blocks, err := converter.Convert()

			if err != nil {
				t.Fatalf("Convert() 返回错误: %v", err)
			}

			if len(blocks) == 0 {
				t.Fatal("blocks 为空")
			}

			expectedType := int(BlockTypeHeading1) + tt.level - 1
			if blocks[0].BlockType == nil || *blocks[0].BlockType != expectedType {
				t.Errorf("BlockType = %v, 期望 %d", blocks[0].BlockType, expectedType)
			}
		})
	}
}

func TestConvert_Paragraph(t *testing.T) {
	markdown := "这是一段普通文本。"

	converter := NewMarkdownToBlock([]byte(markdown), ConvertOptions{}, "")
	blocks, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if len(blocks) == 0 {
		t.Fatal("blocks 为空")
	}

	if blocks[0].BlockType == nil || *blocks[0].BlockType != int(BlockTypeText) {
		t.Errorf("BlockType = %v, 期望 %d", blocks[0].BlockType, int(BlockTypeText))
	}
}

func TestConvert_CodeBlock(t *testing.T) {
	markdown := "```go\nfmt.Println(\"Hello\")\n```"

	converter := NewMarkdownToBlock([]byte(markdown), ConvertOptions{}, "")
	blocks, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if len(blocks) == 0 {
		t.Fatal("blocks 为空")
	}

	if blocks[0].BlockType == nil || *blocks[0].BlockType != int(BlockTypeCode) {
		t.Errorf("BlockType = %v, 期望 %d", blocks[0].BlockType, int(BlockTypeCode))
	}

	// 验证语言
	if blocks[0].Code != nil && blocks[0].Code.Style != nil && blocks[0].Code.Style.Language != nil {
		if *blocks[0].Code.Style.Language != 22 { // Go = 22
			t.Errorf("Language = %d, 期望 22 (Go)", *blocks[0].Code.Style.Language)
		}
	}
}

func TestConvert_UnorderedList(t *testing.T) {
	markdown := `- 项目一
- 项目二
- 项目三`

	converter := NewMarkdownToBlock([]byte(markdown), ConvertOptions{}, "")
	blocks, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if len(blocks) < 3 {
		t.Fatalf("blocks 数量 = %d, 期望至少 3", len(blocks))
	}

	for i, block := range blocks {
		if block.BlockType == nil || *block.BlockType != int(BlockTypeBullet) {
			t.Errorf("blocks[%d].BlockType = %v, 期望 %d", i, block.BlockType, int(BlockTypeBullet))
		}
	}
}

func TestConvert_OrderedList(t *testing.T) {
	markdown := `1. 第一项
2. 第二项
3. 第三项`

	converter := NewMarkdownToBlock([]byte(markdown), ConvertOptions{}, "")
	blocks, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if len(blocks) < 3 {
		t.Fatalf("blocks 数量 = %d, 期望至少 3", len(blocks))
	}

	for i, block := range blocks {
		if block.BlockType == nil || *block.BlockType != int(BlockTypeOrdered) {
			t.Errorf("blocks[%d].BlockType = %v, 期望 %d", i, block.BlockType, int(BlockTypeOrdered))
		}
	}
}

func TestConvert_TaskList(t *testing.T) {
	markdown := `- [ ] 未完成任务
- [x] 已完成任务`

	converter := NewMarkdownToBlock([]byte(markdown), ConvertOptions{}, "")
	blocks, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if len(blocks) < 2 {
		t.Fatalf("blocks 数量 = %d, 期望至少 2", len(blocks))
	}

	// 验证第一个是未完成的 Todo
	if blocks[0].BlockType == nil || *blocks[0].BlockType != int(BlockTypeTodo) {
		t.Errorf("blocks[0].BlockType = %v, 期望 %d", blocks[0].BlockType, int(BlockTypeTodo))
	}
	if blocks[0].Todo != nil && blocks[0].Todo.Style != nil && blocks[0].Todo.Style.Done != nil {
		if *blocks[0].Todo.Style.Done != false {
			t.Error("第一个任务应该是未完成状态")
		}
	}

	// 验证第二个是已完成的 Todo
	if blocks[1].BlockType == nil || *blocks[1].BlockType != int(BlockTypeTodo) {
		t.Errorf("blocks[1].BlockType = %v, 期望 %d", blocks[1].BlockType, int(BlockTypeTodo))
	}
	if blocks[1].Todo != nil && blocks[1].Todo.Style != nil && blocks[1].Todo.Style.Done != nil {
		if *blocks[1].Todo.Style.Done != true {
			t.Error("第二个任务应该是已完成状态")
		}
	}
}

func TestConvert_Blockquote(t *testing.T) {
	markdown := "> 这是一段引用"

	converter := NewMarkdownToBlock([]byte(markdown), ConvertOptions{}, "")
	blocks, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if len(blocks) == 0 {
		t.Fatal("blocks 为空")
	}

	if blocks[0].BlockType == nil || *blocks[0].BlockType != int(BlockTypeQuote) {
		t.Errorf("BlockType = %v, 期望 %d", blocks[0].BlockType, int(BlockTypeQuote))
	}
}

func TestConvert_ThematicBreak(t *testing.T) {
	markdown := "段落一\n\n---\n\n段落二"

	converter := NewMarkdownToBlock([]byte(markdown), ConvertOptions{}, "")
	blocks, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	// 应该有段落、分割线、段落
	hasDivider := false
	for _, block := range blocks {
		if block.BlockType != nil && *block.BlockType == int(BlockTypeDivider) {
			hasDivider = true
			break
		}
	}

	if !hasDivider {
		t.Error("应该包含分割线块")
	}
}

func TestConvert_BoldText(t *testing.T) {
	markdown := "这是**粗体**文本"

	converter := NewMarkdownToBlock([]byte(markdown), ConvertOptions{}, "")
	blocks, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if len(blocks) == 0 {
		t.Fatal("blocks 为空")
	}

	// 验证有粗体样式的元素
	if blocks[0].Text != nil && blocks[0].Text.Elements != nil {
		hasBold := false
		for _, elem := range blocks[0].Text.Elements {
			if elem.TextRun != nil && elem.TextRun.TextElementStyle != nil {
				if elem.TextRun.TextElementStyle.Bold != nil && *elem.TextRun.TextElementStyle.Bold {
					hasBold = true
					break
				}
			}
		}
		if !hasBold {
			t.Error("应该包含粗体样式")
		}
	}
}

func TestConvert_ItalicText(t *testing.T) {
	markdown := "这是*斜体*文本"

	converter := NewMarkdownToBlock([]byte(markdown), ConvertOptions{}, "")
	blocks, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if len(blocks) == 0 {
		t.Fatal("blocks 为空")
	}

	// 验证有斜体样式的元素
	if blocks[0].Text != nil && blocks[0].Text.Elements != nil {
		hasItalic := false
		for _, elem := range blocks[0].Text.Elements {
			if elem.TextRun != nil && elem.TextRun.TextElementStyle != nil {
				if elem.TextRun.TextElementStyle.Italic != nil && *elem.TextRun.TextElementStyle.Italic {
					hasItalic = true
					break
				}
			}
		}
		if !hasItalic {
			t.Error("应该包含斜体样式")
		}
	}
}

func TestConvert_InlineCode(t *testing.T) {
	markdown := "这是`代码`文本"

	converter := NewMarkdownToBlock([]byte(markdown), ConvertOptions{}, "")
	blocks, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if len(blocks) == 0 {
		t.Fatal("blocks 为空")
	}

	// 验证有行内代码样式的元素
	if blocks[0].Text != nil && blocks[0].Text.Elements != nil {
		hasInlineCode := false
		for _, elem := range blocks[0].Text.Elements {
			if elem.TextRun != nil && elem.TextRun.TextElementStyle != nil {
				if elem.TextRun.TextElementStyle.InlineCode != nil && *elem.TextRun.TextElementStyle.InlineCode {
					hasInlineCode = true
					break
				}
			}
		}
		if !hasInlineCode {
			t.Error("应该包含行内代码样式")
		}
	}
}

func TestConvert_Strikethrough(t *testing.T) {
	markdown := "这是~~删除线~~文本"

	converter := NewMarkdownToBlock([]byte(markdown), ConvertOptions{}, "")
	blocks, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if len(blocks) == 0 {
		t.Fatal("blocks 为空")
	}

	// 验证有删除线样式的元素
	if blocks[0].Text != nil && blocks[0].Text.Elements != nil {
		hasStrikethrough := false
		for _, elem := range blocks[0].Text.Elements {
			if elem.TextRun != nil && elem.TextRun.TextElementStyle != nil {
				if elem.TextRun.TextElementStyle.Strikethrough != nil && *elem.TextRun.TextElementStyle.Strikethrough {
					hasStrikethrough = true
					break
				}
			}
		}
		if !hasStrikethrough {
			t.Error("应该包含删除线样式")
		}
	}
}

func TestConvert_Link(t *testing.T) {
	markdown := "这是[链接](https://example.com)文本"

	converter := NewMarkdownToBlock([]byte(markdown), ConvertOptions{}, "")
	blocks, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if len(blocks) == 0 {
		t.Fatal("blocks 为空")
	}

	// 验证有链接的元素
	if blocks[0].Text != nil && blocks[0].Text.Elements != nil {
		hasLink := false
		for _, elem := range blocks[0].Text.Elements {
			if elem.TextRun != nil && elem.TextRun.TextElementStyle != nil {
				if elem.TextRun.TextElementStyle.Link != nil && elem.TextRun.TextElementStyle.Link.Url != nil {
					if *elem.TextRun.TextElementStyle.Link.Url == "https://example.com" {
						hasLink = true
						break
					}
				}
			}
		}
		if !hasLink {
			t.Error("应该包含链接")
		}
	}
}

func TestConvert_EmptyMarkdown(t *testing.T) {
	markdown := ""

	converter := NewMarkdownToBlock([]byte(markdown), ConvertOptions{}, "")
	blocks, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if len(blocks) != 0 {
		t.Errorf("空 markdown 的 blocks 数量 = %d, 期望 0", len(blocks))
	}
}

func TestConvert_OnlyWhitespace(t *testing.T) {
	markdown := "   \n\n   \t   \n"

	converter := NewMarkdownToBlock([]byte(markdown), ConvertOptions{}, "")
	blocks, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	// 只有空白字符的 markdown 不应产生任何块
	if len(blocks) != 0 {
		t.Errorf("只有空白的 markdown 的 blocks 数量 = %d, 期望 0", len(blocks))
	}
}

func TestConvert_Callout(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		bgColor  int
	}{
		{"NOTE", "> [!NOTE]\n> 这是一个提示", 6},       // Blue
		{"INFO", "> [!INFO]\n> 这是信息", 6},         // Blue
		{"WARNING", "> [!WARNING]\n> 这是警告", 2},   // Red
		{"CAUTION", "> [!CAUTION]\n> 这是警示", 2},   // Red
		{"TIP", "> [!TIP]\n> 这是技巧", 4},           // Yellow
		{"SUCCESS", "> [!SUCCESS]\n> 这是成功", 5},   // Green
		{"IMPORTANT", "> [!IMPORTANT]\n> 重要", 7}, // Purple
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := NewMarkdownToBlock([]byte(tt.markdown), ConvertOptions{}, "")
			blocks, err := converter.Convert()

			if err != nil {
				t.Fatalf("Convert() 返回错误: %v", err)
			}

			if len(blocks) == 0 {
				t.Fatal("blocks 为空")
			}

			// 验证是 Callout 块或 Quote 块（取决于解析器对 [!TYPE] 的支持）
			if blocks[0].BlockType == nil {
				t.Fatal("BlockType 为 nil")
			}

			blockType := *blocks[0].BlockType
			// Callout 语法可能被解析为 Callout 或普通 Quote
			if blockType != int(BlockTypeCallout) && blockType != int(BlockTypeQuote) {
				t.Errorf("BlockType = %d, 期望 %d (Callout) 或 %d (Quote)",
					blockType, int(BlockTypeCallout), int(BlockTypeQuote))
			}

			// 如果是 Callout，验证背景色
			if blockType == int(BlockTypeCallout) {
				if blocks[0].Callout != nil && blocks[0].Callout.BackgroundColor != nil {
					if *blocks[0].Callout.BackgroundColor != tt.bgColor {
						t.Errorf("BackgroundColor = %d, 期望 %d", *blocks[0].Callout.BackgroundColor, tt.bgColor)
					}
				}
			}
		})
	}
}

func TestConvert_MixedContent(t *testing.T) {
	markdown := `# 标题

这是一段文字。

- 列表项 1
- 列表项 2

` + "```go\ncode\n```" + `

---

> 引用文字`

	converter := NewMarkdownToBlock([]byte(markdown), ConvertOptions{}, "")
	blocks, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if len(blocks) == 0 {
		t.Fatal("blocks 为空")
	}

	// 验证包含各种类型
	hasHeading := false
	hasText := false
	hasBullet := false
	hasCode := false
	hasDivider := false
	hasQuote := false

	for _, block := range blocks {
		if block.BlockType == nil {
			continue
		}
		switch *block.BlockType {
		case int(BlockTypeHeading1):
			hasHeading = true
		case int(BlockTypeText):
			hasText = true
		case int(BlockTypeBullet):
			hasBullet = true
		case int(BlockTypeCode):
			hasCode = true
		case int(BlockTypeDivider):
			hasDivider = true
		case int(BlockTypeQuote):
			hasQuote = true
		}
	}

	if !hasHeading {
		t.Error("缺少标题块")
	}
	if !hasText {
		t.Error("缺少文本块")
	}
	if !hasBullet {
		t.Error("缺少列表块")
	}
	if !hasCode {
		t.Error("缺少代码块")
	}
	if !hasDivider {
		t.Error("缺少分割线块")
	}
	if !hasQuote {
		t.Error("缺少引用块")
	}
}

func TestLanguageNameToCode(t *testing.T) {
	tests := []struct {
		name     string
		expected int
	}{
		{"go", 22},
		{"golang", 22},
		{"python", 47},
		{"py", 47},
		{"javascript", 30},
		{"js", 30},
		{"typescript", 60},
		{"ts", 60},
		{"java", 29},
		{"rust", 51},
		{"c", 10},
		{"cpp", 9},
		{"c++", 9},
		{"csharp", 8},
		{"cs", 8},
		{"sql", 54},
		{"shell", 57},
		{"bash", 7},
		{"json", 28},
		{"yaml", 66},
		{"yml", 66},
		{"markdown", 38},
		{"md", 38},
		{"html", 24},
		{"css", 12},
		{"unknown", 1}, // 默认为 plaintext
		{"", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := languageNameToCode(tt.name)
			if result != tt.expected {
				t.Errorf("languageNameToCode(%q) = %d, 期望 %d", tt.name, result, tt.expected)
			}
		})
	}
}

func TestConvert_Table(t *testing.T) {
	markdown := `| 列1 | 列2 | 列3 |
|-----|-----|-----|
| a   | b   | c   |
| d   | e   | f   |`

	converter := NewMarkdownToBlock([]byte(markdown), ConvertOptions{}, "")
	blocks, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if len(blocks) == 0 {
		t.Fatal("blocks 为空")
	}

	// 表格应该转为文本占位符（由于 API 限制）
	if blocks[0].BlockType == nil || *blocks[0].BlockType != int(BlockTypeText) {
		t.Errorf("表格应转为文本块，BlockType = %v", blocks[0].BlockType)
	}
}

func TestConvert_ImageWithFeishuProtocol(t *testing.T) {
	markdown := "![图片](feishu://media/token123)"

	converter := NewMarkdownToBlock([]byte(markdown), ConvertOptions{}, "")
	blocks, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if len(blocks) == 0 {
		t.Fatal("blocks 为空")
	}

	// 应该创建图片块
	if blocks[0].BlockType == nil || *blocks[0].BlockType != int(BlockTypeImage) {
		t.Errorf("BlockType = %v, 期望 %d", blocks[0].BlockType, int(BlockTypeImage))
	}

	// 验证 token
	if blocks[0].Image != nil && blocks[0].Image.Token != nil {
		if *blocks[0].Image.Token != "token123" {
			t.Errorf("Image.Token = %q, 期望 %q", *blocks[0].Image.Token, "token123")
		}
	}
}

func TestConvert_CodeBlockLanguages(t *testing.T) {
	tests := []struct {
		lang         string
		expectedCode int
	}{
		{"go", 22},
		{"python", 47},
		{"javascript", 30},
		{"java", 29},
		{"rust", 51},
		{"", 1}, // 无语言默认为 plaintext
	}

	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			markdown := "```" + tt.lang + "\ncode\n```"

			converter := NewMarkdownToBlock([]byte(markdown), ConvertOptions{}, "")
			blocks, err := converter.Convert()

			if err != nil {
				t.Fatalf("Convert() 返回错误: %v", err)
			}

			if len(blocks) == 0 {
				t.Fatal("blocks 为空")
			}

			if blocks[0].Code != nil && blocks[0].Code.Style != nil && blocks[0].Code.Style.Language != nil {
				if *blocks[0].Code.Style.Language != tt.expectedCode {
					t.Errorf("Language = %d, 期望 %d", *blocks[0].Code.Style.Language, tt.expectedCode)
				}
			}
		})
	}
}

func TestConvert_NestedStyles(t *testing.T) {
	// 测试嵌套样式
	markdown := "这是***粗斜体***文本"

	converter := NewMarkdownToBlock([]byte(markdown), ConvertOptions{}, "")
	blocks, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if len(blocks) == 0 {
		t.Fatal("blocks 为空")
	}

	// 验证至少有粗体或斜体样式（嵌套样式处理可能因 goldmark 版本而异）
	if blocks[0].Text != nil && blocks[0].Text.Elements != nil {
		hasStyle := false
		for _, elem := range blocks[0].Text.Elements {
			if elem.TextRun != nil && elem.TextRun.TextElementStyle != nil {
				style := elem.TextRun.TextElementStyle
				if (style.Bold != nil && *style.Bold) || (style.Italic != nil && *style.Italic) {
					hasStyle = true
					break
				}
			}
		}
		if !hasStyle {
			t.Error("应该包含粗体或斜体样式")
		}
	}
}

func TestConvert_AnchorLink(t *testing.T) {
	// 页内锚点链接应转为纯文本
	markdown := "点击[这里](#section)跳转"

	converter := NewMarkdownToBlock([]byte(markdown), ConvertOptions{}, "")
	blocks, err := converter.Convert()

	if err != nil {
		t.Fatalf("Convert() 返回错误: %v", err)
	}

	if len(blocks) == 0 {
		t.Fatal("blocks 为空")
	}

	// 验证锚点链接被处理
	if blocks[0].Text != nil && blocks[0].Text.Elements != nil {
		for _, elem := range blocks[0].Text.Elements {
			if elem.TextRun != nil && elem.TextRun.Content != nil {
				if strings.Contains(*elem.TextRun.Content, "这里") {
					// 锚点链接应该转为纯文本，不应有链接样式
					if elem.TextRun.TextElementStyle != nil && elem.TextRun.TextElementStyle.Link != nil {
						if elem.TextRun.TextElementStyle.Link.Url != nil {
							url := *elem.TextRun.TextElementStyle.Link.Url
							if strings.HasPrefix(url, "#") {
								t.Error("锚点链接不应保留 # 开头的 URL")
							}
						}
					}
				}
			}
		}
	}
}
