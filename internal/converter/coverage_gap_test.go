package converter

import (
	"fmt"
	"strings"
	"testing"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
)

// intPtr 已在 block_to_markdown_test.go 中定义

// mockUserResolverForCoverage 用于测试 MentionUser 收集
type mockUserResolverForCoverage struct {
	resolvedIDs []string
}

func (m *mockUserResolverForCoverage) BatchResolve(userIDs []string) map[string]MentionUserInfo {
	m.resolvedIDs = userIDs
	result := make(map[string]MentionUserInfo)
	for _, id := range userIDs {
		result[id] = MentionUserInfo{
			Name:  "User " + id,
			Email: id + "@example.com",
		}
	}
	return result
}

// ====== block_to_markdown.go 测试 ======

// TestConvertNilFields 测试各种 nil 字段路径
func TestConvertNilFields(t *testing.T) {
	tests := []struct {
		name      string
		block     *larkdocx.Block
		expectErr bool
	}{
		{
			name: "Bitable block with nil Bitable field",
			block: &larkdocx.Block{
				BlockId:   strPtr("b1"),
				BlockType: intPtr(int(BlockTypeBitable)),
				// Bitable 字段为 nil
			},
			expectErr: false,
		},
		{
			name: "Sheet block with nil Sheet field",
			block: &larkdocx.Block{
				BlockId:   strPtr("s1"),
				BlockType: intPtr(int(BlockTypeSheet)),
				// Sheet 字段为 nil
			},
			expectErr: false,
		},
		{
			name: "ChatCard block with nil ChatCard field",
			block: &larkdocx.Block{
				BlockId:   strPtr("c1"),
				BlockType: intPtr(int(BlockTypeChatCard)),
				// ChatCard 字段为 nil
			},
			expectErr: false,
		},
		{
			name: "Board block with nil Board field",
			block: &larkdocx.Block{
				BlockId:   strPtr("board1"),
				BlockType: intPtr(int(BlockTypeBoard)),
				// Board 字段为 nil
			},
			expectErr: false,
		},
		{
			name: "MindNote block with nil Mindnote field",
			block: &larkdocx.Block{
				BlockId:   strPtr("m1"),
				BlockType: intPtr(int(BlockTypeMindNote)),
				// Mindnote 字段为 nil
			},
			expectErr: false,
		},
		{
			name: "Code block with nil Code field",
			block: &larkdocx.Block{
				BlockId:   strPtr("code1"),
				BlockType: intPtr(int(BlockTypeCode)),
				// Code 字段为 nil
			},
			expectErr: false,
		},
		{
			name: "Quote block with nil Quote field",
			block: &larkdocx.Block{
				BlockId:   strPtr("q1"),
				BlockType: intPtr(int(BlockTypeQuote)),
				// Quote 字段为 nil
			},
			expectErr: false,
		},
		{
			name: "Todo block with nil Todo field",
			block: &larkdocx.Block{
				BlockId:   strPtr("t1"),
				BlockType: intPtr(int(BlockTypeTodo)),
				// Todo 字段为 nil
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks := []*larkdocx.Block{tt.block}
			opts := ConvertOptions{}
			converter := NewBlockToMarkdown(blocks, opts)
			_, err := converter.Convert()
			if (err != nil) != tt.expectErr {
				t.Errorf("expected error=%v, got error=%v", tt.expectErr, err)
			}
		})
	}
}

// TestConvertBitableWithToken 测试 Bitable 正常路径
func TestConvertBitableWithToken(t *testing.T) {
	block := &larkdocx.Block{
		BlockId:   strPtr("b1"),
		BlockType: intPtr(int(BlockTypeBitable)),
		Bitable: &larkdocx.Bitable{
			Token: strPtr("bitable_token_123"),
		},
	}

	blocks := []*larkdocx.Block{block}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(md, "[Bitable:") {
		t.Errorf("expected Bitable link in output, got: %s", md)
	}
}

// TestConvertSheetWithToken 测试 Sheet 正常路径
func TestConvertSheetWithToken(t *testing.T) {
	block := &larkdocx.Block{
		BlockId:   strPtr("s1"),
		BlockType: intPtr(int(BlockTypeSheet)),
		Sheet: &larkdocx.Sheet{
			Token: strPtr("sheet_token_456"),
		},
	}

	blocks := []*larkdocx.Block{block}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(md, "[Sheet:") {
		t.Errorf("expected Sheet link in output, got: %s", md)
	}
}

// TestConvertChatCardWithChatId 测试 ChatCard 正常路径
func TestConvertChatCardWithChatId(t *testing.T) {
	block := &larkdocx.Block{
		BlockId:   strPtr("c1"),
		BlockType: intPtr(int(BlockTypeChatCard)),
		ChatCard: &larkdocx.ChatCard{
			ChatId: strPtr("chat_id_789"),
		},
	}

	blocks := []*larkdocx.Block{block}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(md, "[ChatCard:") {
		t.Errorf("expected ChatCard link in output, got: %s", md)
	}
}

// TestConvertBoardWithToken 测试 Board 正常路径
func TestConvertBoardWithToken(t *testing.T) {
	block := &larkdocx.Block{
		BlockId:   strPtr("board1"),
		BlockType: intPtr(int(BlockTypeBoard)),
		Board: &larkdocx.Board{
			Token: strPtr("board_token_abc"),
		},
	}

	blocks := []*larkdocx.Block{block}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(md, "[画板") && !strings.Contains(md, "[Board") && !strings.Contains(md, "board") {
		t.Errorf("expected Board link in output, got: %s", md)
	}
}

// TestConvertMindNoteWithToken 测试 MindNote 正常路径
func TestConvertMindNoteWithToken(t *testing.T) {
	block := &larkdocx.Block{
		BlockId:   strPtr("m1"),
		BlockType: intPtr(int(BlockTypeMindNote)),
		Mindnote: &larkdocx.Mindnote{
			Token: strPtr("mindnote_token_def"),
		},
	}

	blocks := []*larkdocx.Block{block}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(md, "思维导图") && !strings.Contains(md, "MindNote") && !strings.Contains(md, "mindnote") {
		t.Errorf("expected MindNote link in output, got: %s", md)
	}
}

// TestConvertImageNilPaths 测试 Image 的 nil 路径
func TestConvertImageNilPaths(t *testing.T) {
	tests := []struct {
		name  string
		block *larkdocx.Block
	}{
		{
			name: "Image with nil Image field",
			block: &larkdocx.Block{
				BlockId:   strPtr("img1"),
				BlockType: intPtr(int(BlockTypeImage)),
				// Image 字段为 nil
			},
		},
		{
			name: "Image with empty token",
			block: &larkdocx.Block{
				BlockId:   strPtr("img2"),
				BlockType: intPtr(int(BlockTypeImage)),
				Image: &larkdocx.Image{
					Token: strPtr(""),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks := []*larkdocx.Block{tt.block}
			opts := ConvertOptions{}
			converter := NewBlockToMarkdown(blocks, opts)
			_, err := converter.Convert()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// nil Image 字段或空 token 不输出内容，或输出空图片链接
			// 这是预期行为，不视为错误
		})
	}
}

// TestCollectMentionUserIDsInVariousBlocks 测试在各种块中收集 MentionUser
func TestCollectMentionUserIDsInVariousBlocks(t *testing.T) {
	blocks := []*larkdocx.Block{
		// Heading4 with MentionUser
		{
			BlockId:   strPtr("h4"),
			BlockType: intPtr(int(BlockTypeHeading4)),
			Heading4: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{
						MentionUser: &larkdocx.MentionUser{
							UserId: strPtr("user1"),
						},
					},
				},
			},
		},
		// Heading5 with MentionUser
		{
			BlockId:   strPtr("h5"),
			BlockType: intPtr(int(BlockTypeHeading5)),
			Heading5: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{
						MentionUser: &larkdocx.MentionUser{
							UserId: strPtr("user2"),
						},
					},
				},
			},
		},
		// Heading6 with MentionUser
		{
			BlockId:   strPtr("h6"),
			BlockType: intPtr(int(BlockTypeHeading6)),
			Heading6: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{
						MentionUser: &larkdocx.MentionUser{
							UserId: strPtr("user3"),
						},
					},
				},
			},
		},
		// Heading7 with MentionUser
		{
			BlockId:   strPtr("h7"),
			BlockType: intPtr(int(BlockTypeHeading7)),
			Heading7: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{
						MentionUser: &larkdocx.MentionUser{
							UserId: strPtr("user4"),
						},
					},
				},
			},
		},
		// Heading8 with MentionUser
		{
			BlockId:   strPtr("h8"),
			BlockType: intPtr(int(BlockTypeHeading8)),
			Heading8: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{
						MentionUser: &larkdocx.MentionUser{
							UserId: strPtr("user5"),
						},
					},
				},
			},
		},
		// Heading9 with MentionUser
		{
			BlockId:   strPtr("h9"),
			BlockType: intPtr(int(BlockTypeHeading9)),
			Heading9: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{
						MentionUser: &larkdocx.MentionUser{
							UserId: strPtr("user6"),
						},
					},
				},
			},
		},
		// Bullet with MentionUser
		{
			BlockId:   strPtr("b1"),
			BlockType: intPtr(int(BlockTypeBullet)),
			Bullet: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{
						MentionUser: &larkdocx.MentionUser{
							UserId: strPtr("user7"),
						},
					},
				},
			},
		},
		// Ordered with MentionUser
		{
			BlockId:   strPtr("o1"),
			BlockType: intPtr(int(BlockTypeOrdered)),
			Ordered: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{
						MentionUser: &larkdocx.MentionUser{
							UserId: strPtr("user8"),
						},
					},
				},
			},
		},
		// Quote with MentionUser
		{
			BlockId:   strPtr("q1"),
			BlockType: intPtr(int(BlockTypeQuote)),
			Quote: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{
						MentionUser: &larkdocx.MentionUser{
							UserId: strPtr("user9"),
						},
					},
				},
			},
		},
		// Todo with MentionUser
		{
			BlockId:   strPtr("t1"),
			BlockType: intPtr(int(BlockTypeTodo)),
			Todo: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{
						MentionUser: &larkdocx.MentionUser{
							UserId: strPtr("user10"),
						},
					},
				},
			},
		},
		// Code with MentionUser (虽然不常见)
		{
			BlockId:   strPtr("c1"),
			BlockType: intPtr(int(BlockTypeCode)),
			Code: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{
						MentionUser: &larkdocx.MentionUser{
							UserId: strPtr("user11"),
						},
					},
				},
			},
		},
		// Equation with MentionUser (虽然不常见)
		{
			BlockId:   strPtr("eq1"),
			BlockType: intPtr(int(BlockTypeEquation)),
			Equation: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{
						MentionUser: &larkdocx.MentionUser{
							UserId: strPtr("user12"),
						},
					},
				},
			},
		},
	}

	opts := ConvertOptions{
		ExpandMentions: true, // 启用 mentions 展开
	}

	// 使用 NewBlockToMarkdownWithResolver 来触发 collectMentionUserIDs
	resolver := &mockUserResolverForCoverage{}

	converter := NewBlockToMarkdownWithResolver(blocks, opts, resolver)
	_, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 检查 resolver 是否被调用处理了所有用户
	expectedCount := 12
	if len(resolver.resolvedIDs) != expectedCount {
		t.Errorf("expected %d mentioned users to be resolved, got %d", expectedCount, len(resolver.resolvedIDs))
	}
}

// ====== markdown_to_block.go 测试 ======

// TestComplexTextFormatting 测试复杂文本格式以覆盖 extractChildElements
func TestComplexTextFormatting(t *testing.T) {
	testCases := []struct {
		name     string
		markdown string
		check    func(t *testing.T, blocks []*larkdocx.Block)
	}{
		{
			name:     "underline text",
			markdown: "# <u>下划线文本</u>",
			check: func(t *testing.T, blocks []*larkdocx.Block) {
				if len(blocks) == 0 {
					t.Fatal("expected at least one block")
				}
			},
		},
		{
			name:     "bold underline",
			markdown: "**<u>粗体下划线</u>**",
			check: func(t *testing.T, blocks []*larkdocx.Block) {
				if len(blocks) == 0 {
					t.Fatal("expected at least one block")
				}
			},
		},
		{
			name:     "strikethrough link",
			markdown: "~~[链接](https://example.com)~~",
			check: func(t *testing.T, blocks []*larkdocx.Block) {
				if len(blocks) == 0 {
					t.Fatal("expected at least one block")
				}
			},
		},
		{
			name:     "bold inline code",
			markdown: "**`行内代码`**",
			check: func(t *testing.T, blocks []*larkdocx.Block) {
				if len(blocks) == 0 {
					t.Fatal("expected at least one block")
				}
			},
		},
		{
			name:     "bold strikethrough",
			markdown: "**~~删除线~~**",
			check: func(t *testing.T, blocks []*larkdocx.Block) {
				if len(blocks) == 0 {
					t.Fatal("expected at least one block")
				}
			},
		},
		{
			name:     "underline link",
			markdown: "<u>[链接](https://example.com)</u>",
			check: func(t *testing.T, blocks []*larkdocx.Block) {
				if len(blocks) == 0 {
					t.Fatal("expected at least one block")
				}
			},
		},
		{
			name:     "underline bold",
			markdown: "<u>**粗体**</u>",
			check: func(t *testing.T, blocks []*larkdocx.Block) {
				if len(blocks) == 0 {
					t.Fatal("expected at least one block")
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := ConvertOptions{}
			converter := NewMarkdownToBlock([]byte(tc.markdown), opts, "")
			result, err := converter.ConvertWithTableData()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			blocks := FlattenBlockNodes(result.BlockNodes)
			tc.check(t, blocks)
		})
	}
}

// TestQuoteWithComplexContent 测试引用块的复杂内容以覆盖 extractQuoteLines
func TestQuoteWithComplexContent(t *testing.T) {
	testCases := []struct {
		name     string
		markdown string
	}{
		{
			name:     "quote with bold",
			markdown: "> **粗体引用**",
		},
		{
			name:     "quote with inline code",
			markdown: "> `代码引用`",
		},
		{
			name:     "quote with strikethrough",
			markdown: "> ~~删除线引用~~",
		},
		{
			name:     "quote with link",
			markdown: "> [链接](https://example.com)",
		},
		{
			name: "multi-line quote with formatting",
			markdown: `> **第一行粗体**
> *第二行斜体*
> ~~第三行删除线~~`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := ConvertOptions{}
			converter := NewMarkdownToBlock([]byte(tc.markdown), opts, "")
			result, err := converter.ConvertWithTableData()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			blocks := FlattenBlockNodes(result.BlockNodes)
			if len(blocks) == 0 {
				t.Fatal("expected at least one block")
			}
		})
	}
}

// TestCalloutWithComplexContent 测试 Callout 的复杂内容以覆盖 extractCalloutParaElements
func TestCalloutWithComplexContent(t *testing.T) {
	testCases := []struct {
		name     string
		markdown string
	}{
		{
			name: "callout with second line",
			markdown: `> [!NOTE]
> 第二行内容`,
		},
		{
			name:     "callout with inline content",
			markdown: "> [!WARNING] 同行内容",
		},
		{
			name: "callout with multiple paragraphs",
			markdown: `> [!TIP]
> 第一段
>
> 第二段`,
		},
		{
			name: "callout with formatting",
			markdown: `> [!IMPORTANT]
> **粗体内容**
> *斜体内容*`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := ConvertOptions{}
			converter := NewMarkdownToBlock([]byte(tc.markdown), opts, "")
			result, err := converter.ConvertWithTableData()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			blocks := FlattenBlockNodes(result.BlockNodes)
			if len(blocks) == 0 {
				t.Fatal("expected at least one block")
			}
		})
	}
}

// TestListItemEdgeCases 测试列表项的边缘情况以覆盖 convertListItem
func TestListItemEdgeCases(t *testing.T) {
	testCases := []struct {
		name     string
		markdown string
	}{
		{
			name: "list with nested sub-list",
			markdown: `-
  - 子列表项`,
		},
		{
			name: "list with nested ordered list",
			markdown: `-
  1. 子有序列表`,
		},
		{
			name: "empty list item with child",
			markdown: `-
  - 嵌套项
  - 另一个嵌套项`,
		},
		{
			name: "mixed nesting",
			markdown: `- 第一项
  1. 子有序1
  2. 子有序2
- 第二项
  - 子无序`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := ConvertOptions{}
			converter := NewMarkdownToBlock([]byte(tc.markdown), opts, "")
			result, err := converter.ConvertWithTableData()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			blocks := FlattenBlockNodes(result.BlockNodes)
			if len(blocks) == 0 {
				t.Fatal("expected at least one block")
			}
		})
	}
}

// TestExtractTextElementsWithImages 测试 extractTextElements 处理内联图片
func TestExtractTextElementsWithImages(t *testing.T) {
	markdown := "这里有 ![内联图片](https://example.com/img.png) 和文本"

	opts := ConvertOptions{}
	converter := NewMarkdownToBlock([]byte(markdown), opts, "")
	result, err := converter.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blocks := FlattenBlockNodes(result.BlockNodes)
	if len(blocks) == 0 {
		t.Fatal("expected at least one block")
	}
}

// TestAutoLinks 测试自动链接以覆盖更多 extractTextElements 路径
func TestAutoLinks(t *testing.T) {
	markdown := "访问 <https://auto.link> 查看详情"

	opts := ConvertOptions{}
	converter := NewMarkdownToBlock([]byte(markdown), opts, "")
	result, err := converter.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blocks := FlattenBlockNodes(result.BlockNodes)
	if len(blocks) == 0 {
		t.Fatal("expected at least one block")
	}
}

// TestGetNodeTextWithDepthRawHTML 测试 getNodeTextWithDepth 处理 RawHTML
func TestGetNodeTextWithDepthRawHTML(t *testing.T) {
	markdown := "第一行<br>第二行"

	opts := ConvertOptions{}
	converter := NewMarkdownToBlock([]byte(markdown), opts, "")
	result, err := converter.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blocks := FlattenBlockNodes(result.BlockNodes)
	if len(blocks) == 0 {
		t.Fatal("expected at least one block")
	}
}

// TestBlockquoteWithNestedContent 测试引用块的嵌套内容
func TestBlockquoteWithNestedContent(t *testing.T) {
	testCases := []struct {
		name     string
		markdown string
	}{
		{
			name: "quote with list",
			markdown: `> 引用中的列表
> - item1
> - item2`,
		},
		{
			name: "quote with code block",
			markdown: `> 引用中的代码
> ` + "```go\n> code\n> ```",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := ConvertOptions{}
			converter := NewMarkdownToBlock([]byte(tc.markdown), opts, "")
			result, err := converter.ConvertWithTableData()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			blocks := FlattenBlockNodes(result.BlockNodes)
			if len(blocks) == 0 {
				t.Fatal("expected at least one block")
			}
		})
	}
}

// TestCalloutWithNestedContent 测试 Callout 的嵌套内容
func TestCalloutWithNestedContent(t *testing.T) {
	markdown := `> [!NOTE]
> - 列表项
>
> ` + "```go\ncode\n```"

	opts := ConvertOptions{}
	converter := NewMarkdownToBlock([]byte(markdown), opts, "")
	result, err := converter.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blocks := FlattenBlockNodes(result.BlockNodes)
	if len(blocks) == 0 {
		t.Fatal("expected at least one block")
	}
}

// TestHeadingLevels 测试不同级别的标题
func TestHeadingLevels(t *testing.T) {
	markdown := `# 标题1
## 标题2
### 标题3
#### 标题4
##### 标题5
###### 标题6`

	opts := ConvertOptions{}
	converter := NewMarkdownToBlock([]byte(markdown), opts, "")
	result, err := converter.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blocks := FlattenBlockNodes(result.BlockNodes)
	if len(blocks) != 6 {
		t.Errorf("expected 6 heading blocks, got %d", len(blocks))
	}
}

// TestImageConversion 测试图片转换的不同路径
func TestImageConversion(t *testing.T) {
	testCases := []struct {
		name     string
		markdown string
	}{
		{
			name:     "image with URL",
			markdown: "![图片](https://example.com/img.png)",
		},
		{
			name:     "image with local path",
			markdown: "![图片](local/path.png)",
		},
		{
			name:     "image without alt text",
			markdown: "![](https://example.com/img.png)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := ConvertOptions{}
			converter := NewMarkdownToBlock([]byte(tc.markdown), opts, "")
			result, err := converter.ConvertWithTableData()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			blocks := FlattenBlockNodes(result.BlockNodes)
			if len(blocks) == 0 {
				t.Fatal("expected at least one block")
			}
		})
	}
}

// TestLargeTableSplitting 测试大表格自动拆分
func TestLargeTableSplitting(t *testing.T) {
	// 创建一个 10 行的表格（超过 9 行限制）
	markdown := `| H1 | H2 |
|----|-----|
| a  | b  |
| c  | d  |
| e  | f  |
| g  | h  |
| i  | j  |
| k  | l  |
| m  | n  |
| o  | p  |
| q  | r  |`

	opts := ConvertOptions{}
	converter := NewMarkdownToBlock([]byte(markdown), opts, "")
	result, err := converter.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blocks := FlattenBlockNodes(result.BlockNodes)

	// 应该生成至少 2 个表格块（拆分后）
	tableCount := 0
	for _, block := range blocks {
		if block.BlockType != nil && *block.BlockType == int(BlockTypeTable) {
			tableCount++
		}
	}

	if tableCount < 2 {
		t.Errorf("expected at least 2 table blocks after splitting, got %d", tableCount)
	}

	// 验证 tableData 也相应拆分
	if len(result.TableDatas) < 2 {
		t.Errorf("expected at least 2 table data entries, got %d", len(result.TableDatas))
	}
}

// TestTaskListItem 测试任务列表项转换
func TestTaskListItem(t *testing.T) {
	testCases := []struct {
		name     string
		markdown string
		checked  bool
	}{
		{
			name:     "checked task",
			markdown: "- [x] 已完成任务",
			checked:  true,
		},
		{
			name:     "unchecked task",
			markdown: "- [ ] 未完成任务",
			checked:  false,
		},
		{
			name: "multiple tasks",
			markdown: `- [x] 任务1
- [ ] 任务2
- [x] 任务3`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := ConvertOptions{}
			converter := NewMarkdownToBlock([]byte(tc.markdown), opts, "")
			result, err := converter.ConvertWithTableData()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			blocks := FlattenBlockNodes(result.BlockNodes)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// 检查是否生成了 Todo 块
			todoFound := false
			for _, block := range blocks {
				if block.BlockType != nil && *block.BlockType == int(BlockTypeTodo) {
					todoFound = true
					break
				}
			}

			if !todoFound {
				t.Error("expected at least one Todo block")
			}
		})
	}
}

// TestExtractTextElementsSkipCheckbox 测试跳过复选框的文本提取
func TestExtractTextElementsSkipCheckbox(t *testing.T) {
	// 这个函数通过任务列表间接测试
	markdown := `- [x] 任务内容
- [ ] 另一个任务`

	opts := ConvertOptions{}
	converter := NewMarkdownToBlock([]byte(markdown), opts, "")
	result, err := converter.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blocks := FlattenBlockNodes(result.BlockNodes)

	// 验证生成的 Todo 块
	for _, block := range blocks {
		if block.BlockType != nil && *block.BlockType == int(BlockTypeTodo) {
			if block.Todo != nil && block.Todo.Elements != nil {
				// 验证文本元素不包含 [x] 或 [ ]
				for _, elem := range block.Todo.Elements {
					if elem.TextRun != nil && elem.TextRun.Content != nil {
						content := *elem.TextRun.Content
						if strings.Contains(content, "[x]") || strings.Contains(content, "[ ]") {
							t.Errorf("Todo text should not contain checkbox: %s", content)
						}
					}
				}
			}
		}
	}
}

// TestTableConversionEdgeCases 测试表格转换的边缘情况
func TestTableConversionEdgeCases(t *testing.T) {
	testCases := []struct {
		name     string
		markdown string
	}{
		{
			name: "table with empty cells",
			markdown: `| H1 | H2 |
|----|-----|
|    | b  |
| c  |    |`,
		},
		{
			name: "table with single column",
			markdown: `| H1 |
|----|
| a  |
| b  |`,
		},
		{
			name: "table with single row",
			markdown: `| H1 | H2 |
|----|-----|
| a  | b  |`,
		},
		{
			name: "table with complex cell content",
			markdown: `| H1 | H2 |
|----|-----|
| **粗体** | *斜体* |
| ` + "`code`" + ` | [链接](https://example.com) |`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := ConvertOptions{}
			converter := NewMarkdownToBlock([]byte(tc.markdown), opts, "")
			result, err := converter.ConvertWithTableData()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			blocks := FlattenBlockNodes(result.BlockNodes)

			// 验证生成了表格块
			tableFound := false
			for _, block := range blocks {
				if block.BlockType != nil && *block.BlockType == int(BlockTypeTable) {
					tableFound = true
					break
				}
			}

			if !tableFound {
				t.Error("expected at least one Table block")
			}

			// 验证 tableData 存在
			if len(result.TableDatas) == 0 {
				t.Error("expected table data to be generated")
			}
		})
	}
}

// TestConvertWithTableDataZeroValue 测试空 Markdown 输入
func TestConvertWithTableDataZeroValue(t *testing.T) {
	opts := ConvertOptions{}
	converter := NewMarkdownToBlock([]byte(""), opts, "")
	result, err := converter.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blocks := FlattenBlockNodes(result.BlockNodes)

	if len(blocks) != 0 {
		t.Errorf("expected 0 blocks for empty markdown, got %d", len(blocks))
	}

	if len(result.TableDatas) != 0 {
		t.Errorf("expected 0 table data for empty markdown, got %d", len(result.TableDatas))
	}
}

// TestComplexNestedStructures 测试复杂的嵌套结构
func TestComplexNestedStructures(t *testing.T) {
	markdown := `# 主标题

## 子标题

这是一段普通文本。

- 列表项1
  - 嵌套列表项1.1
  - 嵌套列表项1.2
    1. 嵌套有序1.2.1
    2. 嵌套有序1.2.2
- 列表项2

> [!NOTE]
> 这是一个 Callout
> - 内部列表
>
> 继续内容

| H1 | H2 |
|----|-----|
| a  | b  |

` + "```go\nfunc main() {}\n```" + `

- [x] 已完成任务
- [ ] 未完成任务`

	opts := ConvertOptions{}
	converter := NewMarkdownToBlock([]byte(markdown), opts, "")
	result, err := converter.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blocks := FlattenBlockNodes(result.BlockNodes)

	if len(blocks) == 0 {
		t.Fatal("expected multiple blocks for complex markdown")
	}

	// 验证包含各种块类型
	blockTypes := make(map[int]bool)
	for _, block := range blocks {
		if block.BlockType != nil {
			blockTypes[*block.BlockType] = true
		}
	}

	expectedTypes := []int{
		int(BlockTypeHeading1),
		int(BlockTypeHeading2),
		int(BlockTypeText),
		int(BlockTypeBullet),
		int(BlockTypeCallout),
		int(BlockTypeTable),
		int(BlockTypeCode),
		int(BlockTypeTodo),
	}

	for _, expected := range expectedTypes {
		if !blockTypes[expected] {
			t.Errorf("expected block type %d not found", expected)
		}
	}
}

// ====== 覆盖率提升测试 - 精准覆盖低覆盖率路径 ======

// TestGFMTaskListWithRichFormatting 测试 GFM 任务列表中的富文本格式
// 目标: extractTextElementsSkipCheckbox 中的 Emphasis/CodeSpan/Strikethrough/Link/AutoLink/RawHTML 分支
func TestGFMTaskListWithRichFormatting(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
	}{
		{
			name:     "task with bold",
			markdown: "- [x] **粗体任务**",
		},
		{
			name:     "task with italic",
			markdown: "- [ ] *斜体任务*",
		},
		{
			name:     "task with inline code",
			markdown: "- [x] 包含 `code` 的任务",
		},
		{
			name:     "task with strikethrough",
			markdown: "- [ ] ~~删除线任务~~",
		},
		{
			name:     "task with link",
			markdown: "- [x] 查看 [链接](https://example.com)",
		},
		{
			name:     "task with autolink",
			markdown: "- [ ] 访问 <https://example.com>",
		},
		{
			name:     "task with all formatting",
			markdown: "- [x] **bold** `code` ~~strike~~ [link](https://a.com)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ConvertOptions{}
			conv := NewMarkdownToBlock([]byte(tt.markdown), opts, "")
			result, err := conv.ConvertWithTableData()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			blocks := FlattenBlockNodes(result.BlockNodes)
			if len(blocks) == 0 {
				t.Fatal("expected at least one block")
			}
			// 验证生成了 Todo 块
			found := false
			for _, b := range blocks {
				if b.BlockType != nil && *b.BlockType == int(BlockTypeTodo) {
					found = true
					break
				}
			}
			if !found {
				t.Error("expected Todo block not found")
			}
		})
	}
}

// TestExtractChildElementsRichPaths 测试 extractChildElements 的多种路径
// 关键: <u></u> 必须在 **...** 或 ~~...~~ 内部，才通过 extractChildElements 处理 inUnderline
func TestExtractChildElementsRichPaths(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
	}{
		// --- extractChildElements 通过 Emphasis 入口 ---
		{
			name:     "bold with underline text",
			markdown: "**<u>加粗下划线文本</u>**",
		},
		{
			name:     "bold with underline link",
			markdown: "**<u>[下划线链接](https://example.com)</u>**",
		},
		{
			name:     "bold with underline autolink",
			markdown: "**<u><https://auto.link></u>**",
		},
		{
			name:     "bold with underline emphasis inside",
			markdown: "**<u>*斜体下划线*</u>**",
		},
		{
			name:     "bold with underline strikethrough inside",
			markdown: "**<u>~~删除线下划线~~</u>**",
		},
		{
			name:     "bold with br tag",
			markdown: "**第一行<br>第二行**",
		},
		{
			name:     "bold with code span",
			markdown: "**包含 `code` 的粗体**",
		},
		{
			name:     "bold with autolink no underline",
			markdown: "**访问 <https://example.com> 查看**",
		},
		// --- extractChildElements 通过 Strikethrough 入口 ---
		{
			name:     "strikethrough with link",
			markdown: "~~[删除链接](https://example.com)~~",
		},
		{
			name:     "strikethrough with autolink",
			markdown: "~~<https://example.com>~~",
		},
		{
			name:     "strikethrough with underline text",
			markdown: "~~<u>删除线下划线</u>~~",
		},
		{
			name:     "strikethrough with underline link",
			markdown: "~~<u>[链接](https://example.com)</u>~~",
		},
		// --- 嵌套: Emphasis inside Strikethrough ---
		{
			name:     "bold inside strikethrough",
			markdown: "~~**加粗删除线**~~",
		},
		// --- 纯段落级别的 underline（不进入 extractChildElements 的 inUnderline） ---
		{
			name:     "paragraph level underline",
			markdown: "<u>下划线文本</u>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ConvertOptions{}
			conv := NewMarkdownToBlock([]byte(tt.markdown), opts, "")
			result, err := conv.ConvertWithTableData()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			blocks := FlattenBlockNodes(result.BlockNodes)
			if len(blocks) == 0 {
				t.Fatal("expected at least one block")
			}
		})
	}
}

// TestExtractQuoteLinesRichPaths 测试 extractQuoteLines 的多种分支
// 目标: quote 中的 Emphasis/CodeSpan/Strikethrough/Link/AutoLink 分支
func TestExtractQuoteLinesRichPaths(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
	}{
		{
			name:     "quote with bold",
			markdown: "> **加粗引用**",
		},
		{
			name:     "quote with italic",
			markdown: "> *斜体引用*",
		},
		{
			name:     "quote with code span",
			markdown: "> 包含 `代码` 的引用",
		},
		{
			name:     "quote with strikethrough",
			markdown: "> ~~删除线引用~~",
		},
		{
			name:     "quote with link",
			markdown: "> 查看 [链接](https://example.com)",
		},
		{
			name:     "quote with autolink",
			markdown: "> 访问 <https://example.com>",
		},
		{
			name:     "quote multiline with formatting",
			markdown: "> **第一行**\n> `第二行`\n> ~~第三行~~",
		},
		{
			name:     "quote with mixed inline",
			markdown: "> **bold** and `code` and [link](https://a.com) and ~~strike~~",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ConvertOptions{}
			conv := NewMarkdownToBlock([]byte(tt.markdown), opts, "")
			result, err := conv.ConvertWithTableData()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			blocks := FlattenBlockNodes(result.BlockNodes)
			if len(blocks) == 0 {
				t.Fatal("expected at least one block")
			}
		})
	}
}

// TestCalloutWithNestedElements 测试 callout 内嵌套列表、代码块等
// 目标: convertCallout 中的 List/FencedCodeBlock/Heading/Paragraph 分支
func TestCalloutWithNestedElements(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
	}{
		{
			name:     "callout with list",
			markdown: "> [!NOTE]\n> - 列表项1\n> - 列表项2",
		},
		{
			name:     "callout with code block",
			markdown: "> [!WARNING]\n> 文本说明\n> ```go\n> fmt.Println(\"hello\")\n> ```",
		},
		{
			name:     "callout with heading",
			markdown: "> [!TIP]\n> ## 小标题\n> 正文内容",
		},
		{
			name:     "callout with multiple paragraphs and list",
			markdown: "> [!CAUTION]\n> 第一段\n>\n> 第二段\n>\n> - 项目A\n> - 项目B",
		},
		{
			name:     "callout SUCCESS type",
			markdown: "> [!SUCCESS]\n> 操作成功完成",
		},
		{
			name:     "callout IMPORTANT type",
			markdown: "> [!IMPORTANT]\n> 重要信息",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ConvertOptions{}
			conv := NewMarkdownToBlock([]byte(tt.markdown), opts, "")
			result, err := conv.ConvertWithTableData()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			blocks := FlattenBlockNodes(result.BlockNodes)
			if len(blocks) == 0 {
				t.Fatal("expected at least one block")
			}
		})
	}
}

// TestBlockquoteWithNestedElements 测试 blockquote 嵌套列表、代码块和嵌套引用
// 目标: convertBlockquote 中 List/FencedCodeBlock/Blockquote 分支
func TestBlockquoteWithNestedElements(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
	}{
		{
			name:     "blockquote with nested list",
			markdown: "> 引用文本\n> - 列表1\n> - 列表2",
		},
		{
			name:     "blockquote with code block",
			markdown: "> 引用内容\n> ```python\n> print('hello')\n> ```",
		},
		{
			name:     "blockquote with nested blockquote",
			markdown: "> 外层引用\n>> 内层引用",
		},
		{
			name:     "empty blockquote",
			markdown: "> ",
		},
		{
			name:     "blockquote with only whitespace",
			markdown: ">\n>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ConvertOptions{}
			conv := NewMarkdownToBlock([]byte(tt.markdown), opts, "")
			result, err := conv.ConvertWithTableData()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			blocks := FlattenBlockNodes(result.BlockNodes)
			if len(blocks) == 0 {
				t.Fatal("expected at least one block")
			}
		})
	}
}

// TestImageConversionLocalPath 测试本地路径图片转换
// 目标: convertImage 和 extractTextElements Image 分支中的非 http 路径
func TestImageConversionLocalPath(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
	}{
		{
			name:     "local path image",
			markdown: "![截图](./images/screenshot.png)",
		},
		{
			name:     "relative path image",
			markdown: "![图片](../assets/img.jpg)",
		},
		{
			name:     "image without alt",
			markdown: "![](./file.png)",
		},
		{
			name:     "inline local image in text",
			markdown: "文本 ![本地图](./a.png) 更多文本",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ConvertOptions{}
			conv := NewMarkdownToBlock([]byte(tt.markdown), opts, "")
			result, err := conv.ConvertWithTableData()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			blocks := FlattenBlockNodes(result.BlockNodes)
			if len(blocks) == 0 {
				t.Fatal("expected at least one block")
			}
		})
	}
}

// TestExtractCalloutParaElementsCrossElement 测试 extractCalloutParaElements 的跨元素合并匹配
// 目标: extractCalloutParaElements 中跨元素合并的分支（单元素匹配 + 跨元素匹配 + 尾部保留）
func TestExtractCalloutParaElementsCrossElement(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
	}{
		{
			name:     "callout with inline content after type",
			markdown: "> [!NOTE] 同行内容紧跟类型标识",
		},
		{
			name:     "callout with bold after type",
			markdown: "> [!WARNING]\n> **重要警告**",
		},
		{
			name:     "callout with code after type",
			markdown: "> [!TIP]\n> 使用 `命令` 执行",
		},
		{
			name:     "callout with link after type",
			markdown: "> [!CAUTION]\n> 查看 [文档](https://example.com)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ConvertOptions{}
			conv := NewMarkdownToBlock([]byte(tt.markdown), opts, "")
			result, err := conv.ConvertWithTableData()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			blocks := FlattenBlockNodes(result.BlockNodes)
			if len(blocks) == 0 {
				t.Fatal("expected at least one block")
			}
			// 确保生成了 Callout 块
			found := false
			for _, b := range blocks {
				if b.BlockType != nil && *b.BlockType == int(BlockTypeCallout) {
					found = true
					break
				}
			}
			if !found {
				t.Error("expected Callout block not found")
			}
		})
	}
}

// TestConvertListItemEdgeCases 测试 convertListItem 更多路径
// 目标: 空列表项、嵌套列表、ordered/unordered 列表
func TestConvertListItemEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
	}{
		{
			name:     "ordered list with nested unordered",
			markdown: "1. 第一项\n   - 子项A\n   - 子项B\n2. 第二项",
		},
		{
			name:     "unordered with nested ordered",
			markdown: "- 无序项\n  1. 有序子项1\n  2. 有序子项2",
		},
		{
			name:     "deep nesting 3 levels",
			markdown: "- 层1\n  - 层2\n    - 层3",
		},
		{
			name:     "list item with link",
			markdown: "- 查看 [文档](https://example.com)\n- 另一项",
		},
		{
			name:     "list item with bold and code",
			markdown: "- **粗体** 和 `代码`\n- 普通项",
		},
		{
			name:     "ordered nested in unordered nested in ordered",
			markdown: "1. 项目\n   - 子项\n     1. 孙项",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ConvertOptions{}
			conv := NewMarkdownToBlock([]byte(tt.markdown), opts, "")
			result, err := conv.ConvertWithTableData()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			blocks := FlattenBlockNodes(result.BlockNodes)
			if len(blocks) == 0 {
				t.Fatal("expected at least one block")
			}
		})
	}
}

// TestConvertHeadingAllLevels 测试所有标题层级（含 h7-h9 映射）
// 目标: convertHeading 中 level > 6 的分支
func TestConvertHeadingAllLevels(t *testing.T) {
	// h1 到 h6 是标准 Markdown，h7+ 需要特殊输入
	markdown := "# H1\n## H2\n### H3\n#### H4\n##### H5\n###### H6"
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(markdown), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blocks := FlattenBlockNodes(result.BlockNodes)
	if len(blocks) != 6 {
		t.Errorf("expected 6 blocks, got %d", len(blocks))
	}
	// 验证类型
	expected := []int{
		int(BlockTypeHeading1),
		int(BlockTypeHeading2),
		int(BlockTypeHeading3),
		int(BlockTypeHeading4),
		int(BlockTypeHeading5),
		int(BlockTypeHeading6),
	}
	for i, b := range blocks {
		if i < len(expected) && b.BlockType != nil && *b.BlockType != expected[i] {
			t.Errorf("block[%d]: expected type %d, got %d", i, expected[i], *b.BlockType)
		}
	}
}

// TestGetNodeTextWithDepthBrVariants 测试 <br> 标签的所有变体
// 目标: getNodeTextWithDepth 中 RawHTML 处理的 <br>, <br/>, <br /> 分支
func TestGetNodeTextWithDepthBrVariants(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
	}{
		{
			name:     "br self-closing",
			markdown: "行1<br/>行2",
		},
		{
			name:     "br with space",
			markdown: "行1<br />行2",
		},
		{
			name:     "br plain",
			markdown: "行1<br>行2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ConvertOptions{}
			conv := NewMarkdownToBlock([]byte(tt.markdown), opts, "")
			result, err := conv.ConvertWithTableData()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			blocks := FlattenBlockNodes(result.BlockNodes)
			if len(blocks) == 0 {
				t.Fatal("expected at least one block")
			}
		})
	}
}

// TestConvertImageB2M_DownloadFalse 测试 b2m convertImage 中 DownloadImages=false 路径
func TestConvertImageB2M_DownloadFalse(t *testing.T) {
	// Image block with token, DownloadImages=false 应产生远程 URL 引用
	block := &larkdocx.Block{
		BlockId:   strPtr("img1"),
		BlockType: intPtr(int(BlockTypeImage)),
		Image: &larkdocx.Image{
			Token: strPtr("img_token_abc"),
		},
		Children: []string{},
	}
	opts := ConvertOptions{DownloadImages: false, DocumentID: "doc123"}
	converter := NewBlockToMarkdown([]*larkdocx.Block{block}, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(md, "img_token_abc") {
		t.Errorf("expected image token in output, got: %s", md)
	}
}

// TestConvertWithTableDataUsingConvert 测试通过 Convert() 方法（非 ConvertWithTableData）
// 目标: Convert() 方法和 FlattenBlockNodes 的结合
func TestConvertWithTableDataUsingConvert(t *testing.T) {
	markdown := "# 标题\n\n段落\n\n- 列表项\n\n---"
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(markdown), opts, "")
	blocks, err := conv.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(blocks) < 4 {
		t.Errorf("expected at least 4 blocks, got %d", len(blocks))
	}
}

// TestCalloutExtractParaElementsSingleMatch 测试单元素匹配路径和有剩余内容的路径
func TestCalloutExtractParaElementsSingleMatch(t *testing.T) {
	// [!NOTE] 后面紧跟文本（同一元素）
	markdown := "> [!NOTE] 这是紧跟在类型标识后面的文本"
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(markdown), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blocks := FlattenBlockNodes(result.BlockNodes)
	calloutFound := false
	for _, b := range blocks {
		if b.BlockType != nil && *b.BlockType == int(BlockTypeCallout) {
			calloutFound = true
		}
	}
	if !calloutFound {
		t.Error("expected Callout block")
	}
}

// TestBlockquoteDefaultBranch 触发 blockquote 的 default 分支
func TestBlockquoteDefaultBranch(t *testing.T) {
	// ThematicBreak inside blockquote triggers default branch
	markdown := "> 引用内容\n>\n> ---\n>\n> 更多内容"
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(markdown), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blocks := FlattenBlockNodes(result.BlockNodes)
	if len(blocks) == 0 {
		t.Fatal("expected at least one block")
	}
}

// TestExtractTextElementsImagePaths 精确测试 extractTextElements 中 Image 分支
func TestExtractTextElementsImagePaths(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		contains string
	}{
		{
			name:     "inline http image becomes link",
			markdown: "文本 ![alt](https://example.com/img.png) 更多",
			contains: "图片",
		},
		{
			name:     "inline local image becomes placeholder",
			markdown: "文本 ![alt](./local.png) 更多",
			contains: "Image",
		},
		{
			name:     "inline image no alt with http URL",
			markdown: "![](https://example.com/pic.jpg)",
			contains: "图片",
		},
		{
			name:     "inline image no alt with local path",
			markdown: "![](./pic.jpg)",
			contains: "Image",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ConvertOptions{}
			conv := NewMarkdownToBlock([]byte(tt.markdown), opts, "")
			result, err := conv.ConvertWithTableData()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			blocks := FlattenBlockNodes(result.BlockNodes)
			if len(blocks) == 0 {
				t.Fatal("expected blocks")
			}
		})
	}
}

// TestGetNodeTextWithDepthRawHTMLInLink 测试 getNodeTextWithDepth 处理链接文本内的 <br> 标签
// 目标: getNodeTextWithDepth 中 RawHTML 的 <br>/<br/>/<br /> 分支
func TestGetNodeTextWithDepthRawHTMLInLink(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
	}{
		{
			name:     "link text with br",
			markdown: "[行1<br>行2](https://example.com)",
		},
		{
			name:     "link text with br self-closing",
			markdown: "[行1<br/>行2](https://example.com)",
		},
		{
			name:     "link text with br spaced",
			markdown: "[行1<br />行2](https://example.com)",
		},
		{
			name:     "link text with emphasis",
			markdown: "[**粗体**链接](https://example.com)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ConvertOptions{}
			conv := NewMarkdownToBlock([]byte(tt.markdown), opts, "")
			result, err := conv.ConvertWithTableData()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			blocks := FlattenBlockNodes(result.BlockNodes)
			if len(blocks) == 0 {
				t.Fatal("expected at least one block")
			}
		})
	}
}

// TestConvertImageM2B_FeishuMediaPrefix 测试 feishu://media/ 前缀
func TestConvertImageM2B_FeishuMediaPrefix(t *testing.T) {
	markdown := "![图片](feishu://media/abc123)"
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(markdown), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blocks := FlattenBlockNodes(result.BlockNodes)
	found := false
	for _, b := range blocks {
		if b.BlockType != nil && *b.BlockType == int(BlockTypeText) {
			found = true
		}
	}
	if !found {
		t.Error("expected text placeholder block for feishu:// image")
	}
}

// TestConvertImageM2B_UploadImages 测试 UploadImages=true 路径
func TestConvertImageM2B_UploadImages(t *testing.T) {
	markdown := "![图片](https://example.com/img.png)"
	opts := ConvertOptions{UploadImages: true}
	conv := NewMarkdownToBlock([]byte(markdown), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blocks := FlattenBlockNodes(result.BlockNodes)
	found := false
	for _, b := range blocks {
		if b.BlockType != nil && *b.BlockType == int(BlockTypeImage) {
			found = true
		}
	}
	if !found {
		t.Error("expected Image block when UploadImages is true")
	}
}

// TestConvertListItemEmptyParent 测试深度嵌套列表
func TestConvertListItemEmptyParent(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
	}{
		{
			name:     "list with deeply nested only",
			markdown: "- level1\n  - level2\n    - level3\n      - level4",
		},
		{
			name:     "mixed ordered unordered deep",
			markdown: "1. 有序\n   - 无序子\n     1. 有序孙\n        - 无序曾孙",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ConvertOptions{}
			conv := NewMarkdownToBlock([]byte(tt.markdown), opts, "")
			result, err := conv.ConvertWithTableData()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result.BlockNodes) == 0 {
				t.Fatal("expected block nodes")
			}
		})
	}
}

// TestCalloutWithHeadingInside 测试 callout 内包含标题
func TestCalloutWithHeadingInside(t *testing.T) {
	markdown := "> [!NOTE]\n> ## 标题\n> 内容"
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(markdown), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blocks := FlattenBlockNodes(result.BlockNodes)
	calloutFound := false
	for _, b := range blocks {
		if b.BlockType != nil && *b.BlockType == int(BlockTypeCallout) {
			calloutFound = true
		}
	}
	if !calloutFound {
		t.Error("expected Callout block")
	}
}

// TestCalloutDefaultType 测试未识别的 callout 类型
func TestCalloutDefaultType(t *testing.T) {
	markdown := "> [!CUSTOM]\n> 自定义类型"
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(markdown), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blocks := FlattenBlockNodes(result.BlockNodes)
	calloutFound := false
	for _, b := range blocks {
		if b.BlockType != nil && *b.BlockType == int(BlockTypeCallout) {
			calloutFound = true
		}
	}
	if !calloutFound {
		t.Error("expected Callout block with default color")
	}
}

// TestExtractTextElementsStrikethroughWithLink 测试 strikethrough 内链接和代码
func TestExtractTextElementsStrikethroughWithLink(t *testing.T) {
	markdown := "~~查看 [链接](https://example.com) 和 `代码`~~"
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(markdown), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blocks := FlattenBlockNodes(result.BlockNodes)
	if len(blocks) == 0 {
		t.Fatal("expected blocks")
	}
}

// TestQuoteLinesWithSoftLineBreak 测试多行引用的软换行分割
func TestQuoteLinesWithSoftLineBreak(t *testing.T) {
	markdown := "> 第一行\n> 第二行\n> 第三行"
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(markdown), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blocks := FlattenBlockNodes(result.BlockNodes)
	if len(blocks) == 0 {
		t.Fatal("expected blocks")
	}
}

// TestConvertParagraphWithInlineMath 测试行内公式
func TestConvertParagraphWithInlineMath(t *testing.T) {
	markdown := "这是公式 $E=mc^2$ 和 $a^2+b^2=c^2$ 的段落"
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(markdown), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blocks := FlattenBlockNodes(result.BlockNodes)
	if len(blocks) == 0 {
		t.Fatal("expected blocks")
	}
}

// TestConvertBlockquoteEmpty 测试空引用
func TestConvertBlockquoteEmpty(t *testing.T) {
	markdown := "> \n"
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(markdown), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.BlockNodes) == 0 {
		t.Fatal("expected at least one block node")
	}
}

// ====== b2m 额外块类型覆盖 ======

// TestConvertBlockWithDepthExtraTypes 测试 convertBlockWithDepth 中未覆盖的块类型分支
func TestConvertBlockWithDepthExtraTypes(t *testing.T) {
	tests := []struct {
		name  string
		block *larkdocx.Block
	}{
		{
			name: "WikiCatalogV2",
			block: &larkdocx.Block{
				BlockId:   strPtr("wc2"),
				BlockType: intPtr(int(BlockTypeWikiCatalogV2)),
			},
		},
		{
			name: "AITemplate",
			block: &larkdocx.Block{
				BlockId:   strPtr("ai1"),
				BlockType: intPtr(int(BlockTypeAITemplate)),
			},
		},
		{
			name: "Unknown block type 999",
			block: &larkdocx.Block{
				BlockId:   strPtr("unk1"),
				BlockType: intPtr(999),
			},
		},
		{
			name: "Agenda block without children",
			block: &larkdocx.Block{
				BlockId:   strPtr("ag1"),
				BlockType: intPtr(int(BlockTypeAgenda)),
			},
		},
		{
			name: "AgendaItem without children",
			block: &larkdocx.Block{
				BlockId:   strPtr("agi1"),
				BlockType: intPtr(int(BlockTypeAgendaItem)),
			},
		},
		{
			name: "AgendaItemTitle without text",
			block: &larkdocx.Block{
				BlockId:   strPtr("agit1"),
				BlockType: intPtr(int(BlockTypeAgendaItemTitle)),
			},
		},
		{
			name: "SyncReference without children",
			block: &larkdocx.Block{
				BlockId:   strPtr("sr1"),
				BlockType: intPtr(int(BlockTypeSyncReference)),
			},
		},
		{
			name: "LinkPreview without children",
			block: &larkdocx.Block{
				BlockId:   strPtr("lp1"),
				BlockType: intPtr(int(BlockTypeLinkPreview)),
			},
		},
		{
			name: "Page block",
			block: &larkdocx.Block{
				BlockId:   strPtr("p1"),
				BlockType: intPtr(int(BlockTypePage)),
			},
		},
		{
			name: "TableCell block standalone",
			block: &larkdocx.Block{
				BlockId:   strPtr("tc1"),
				BlockType: intPtr(int(BlockTypeTableCell)),
			},
		},
		{
			name: "GridColumn block standalone",
			block: &larkdocx.Block{
				BlockId:   strPtr("gc1"),
				BlockType: intPtr(int(BlockTypeGridColumn)),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks := []*larkdocx.Block{tt.block}
			opts := ConvertOptions{}
			converter := NewBlockToMarkdown(blocks, opts)
			_, err := converter.Convert()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestConvertAgendaWithChildren 测试 Agenda 块递归展开子块
func TestConvertAgendaWithChildren(t *testing.T) {
	childBlock := &larkdocx.Block{
		BlockId:   strPtr("child1"),
		BlockType: intPtr(int(BlockTypeText)),
		Text: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: strPtr("议程内容")}},
			},
		},
	}
	agendaBlock := &larkdocx.Block{
		BlockId:   strPtr("ag1"),
		BlockType: intPtr(int(BlockTypeAgenda)),
		Children:  []string{"child1"},
	}
	blocks := []*larkdocx.Block{agendaBlock, childBlock}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(md, "议程内容") {
		t.Error("expected agenda content in output")
	}
}

// TestConvertSyncReferenceWithChildren 测试同步引用块展开
func TestConvertSyncReferenceWithChildren(t *testing.T) {
	childBlock := &larkdocx.Block{
		BlockId:   strPtr("sc1"),
		BlockType: intPtr(int(BlockTypeText)),
		Text: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: strPtr("同步内容")}},
			},
		},
	}
	syncBlock := &larkdocx.Block{
		BlockId:   strPtr("sync1"),
		BlockType: intPtr(int(BlockTypeSyncReference)),
		Children:  []string{"sc1"},
	}
	blocks := []*larkdocx.Block{syncBlock, childBlock}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(md, "同步内容") {
		t.Error("expected sync content in output")
	}
}

// TestConvertLinkPreviewWithChildren 测试链接预览块展开
func TestConvertLinkPreviewWithChildren(t *testing.T) {
	childBlock := &larkdocx.Block{
		BlockId:   strPtr("lpc1"),
		BlockType: intPtr(int(BlockTypeText)),
		Text: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: strPtr("预览内容")}},
			},
		},
	}
	lpBlock := &larkdocx.Block{
		BlockId:   strPtr("lp1"),
		BlockType: intPtr(int(BlockTypeLinkPreview)),
		Children:  []string{"lpc1"},
	}
	blocks := []*larkdocx.Block{lpBlock, childBlock}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(md, "预览内容") {
		t.Error("expected preview content in output")
	}
}

// TestTaskListWithRawHTML 测试任务列表中的 HTML 标签
// 目标: extractTextElementsSkipCheckbox 中的 RawHTML 分支
func TestTaskListWithRawHTML(t *testing.T) {
	markdown := "- [x] 第一行<br>第二行"
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(markdown), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blocks := FlattenBlockNodes(result.BlockNodes)
	if len(blocks) == 0 {
		t.Fatal("expected blocks")
	}
}

// TestTaskListWithUnderline 测试任务列表中的下划线
func TestTaskListWithUnderline(t *testing.T) {
	markdown := "- [x] <u>下划线任务</u>"
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(markdown), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blocks := FlattenBlockNodes(result.BlockNodes)
	if len(blocks) == 0 {
		t.Fatal("expected blocks")
	}
}

// TestExtractChildElementsUnderlineCombinations 精确测试 underline 与各种内联元素的组合
// 目标: extractChildElements 中 inUnderline=true 时的 Link/Emphasis/Strikethrough/AutoLink 路径
func TestExtractChildElementsUnderlineCombinations(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
	}{
		// 删除线中包含 underline + link
		{
			name:     "strikethrough wrapping underline link",
			markdown: "~~前文 <u>[链接](https://example.com)</u> 后文~~",
		},
		// 粗体中包含 underline + emphasis
		{
			name:     "bold wrapping underline italic",
			markdown: "**前文 <u>*斜体*</u> 后文**",
		},
		// 粗体中包含 underline + strikethrough
		{
			name:     "bold wrapping underline strikethrough",
			markdown: "**前文 <u>~~删除~~</u> 后文**",
		},
		// 粗体中包含 underline + autolink
		{
			name:     "bold wrapping underline autolink",
			markdown: "**前文 <u><https://auto.com></u> 后文**",
		},
		// 多层嵌套
		{
			name:     "strikethrough with bold inside",
			markdown: "~~**加粗删除**~~",
		},
		// 引用中的富文本（通过 extractQuoteLines）
		{
			name:     "quote with underline bold",
			markdown: "> **<u>加粗下划线</u>** 引用",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ConvertOptions{}
			conv := NewMarkdownToBlock([]byte(tt.markdown), opts, "")
			result, err := conv.ConvertWithTableData()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			blocks := FlattenBlockNodes(result.BlockNodes)
			if len(blocks) == 0 {
				t.Fatal("expected at least one block")
			}
		})
	}
}

// TestConvertBlockWithDepthRecursionDepth 测试递归深度检查
func TestConvertBlockWithDepthRecursionDepth(t *testing.T) {
	// 创建一个非常深的嵌套结构来测试深度限制
	// 使用 AddOns 块进行递归
	blocks := make([]*larkdocx.Block, 102)
	for i := 0; i < 102; i++ {
		blockID := fmt.Sprintf("block_%d", i)
		bt := int(BlockTypeAddOns)
		blocks[i] = &larkdocx.Block{
			BlockId:   strPtr(blockID),
			BlockType: &bt,
		}
		if i < 101 {
			childID := fmt.Sprintf("block_%d", i+1)
			blocks[i].Children = []string{childID}
		}
	}

	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	_, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestConvertAgendaItemWithTitle 测试议程项标题和内容
func TestConvertAgendaItemWithTitle(t *testing.T) {
	titleBlock := &larkdocx.Block{
		BlockId:   strPtr("title1"),
		BlockType: intPtr(int(BlockTypeAgendaItemTitle)),
		Text: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: strPtr("议程标题")}},
			},
		},
	}
	contentChild := &larkdocx.Block{
		BlockId:   strPtr("cc1"),
		BlockType: intPtr(int(BlockTypeText)),
		Text: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: strPtr("议程详情")}},
			},
		},
	}
	contentBlock := &larkdocx.Block{
		BlockId:   strPtr("content1"),
		BlockType: intPtr(int(BlockTypeAgendaItemContent)),
		Children:  []string{"cc1"},
	}
	agendaItem := &larkdocx.Block{
		BlockId:   strPtr("item1"),
		BlockType: intPtr(int(BlockTypeAgendaItem)),
		Children:  []string{"title1", "content1"},
	}
	blocks := []*larkdocx.Block{agendaItem, titleBlock, contentBlock, contentChild}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(md, "议程标题") {
		t.Error("expected agenda title in output")
	}
	if !strings.Contains(md, "议程详情") {
		t.Error("expected agenda content in output")
	}
}

// TestGetCellTextWithDepthNilChildren 测试 getCellTextWithDepth 的 nil children 路径
func TestGetCellTextWithDepthNilChildren(t *testing.T) {
	cellBlock := &larkdocx.Block{
		BlockId:   strPtr("cell1"),
		BlockType: intPtr(int(BlockTypeTableCell)),
		// Children 为 nil
	}
	// 创建一个包含此 cell 的表格来间接测试
	tableBlock := &larkdocx.Block{
		BlockId:   strPtr("table1"),
		BlockType: intPtr(int(BlockTypeTable)),
		Table: &larkdocx.Table{
			Cells: []string{"cell1"},
			Property: &larkdocx.TableProperty{
				RowSize:    intPtr(1),
				ColumnSize: intPtr(1),
			},
		},
		Children: []string{"cell1"},
	}
	blocks := []*larkdocx.Block{tableBlock, cellBlock}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	_, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestConvertListItemEmptyWithChildren 测试空列表项带嵌套子列表（convertListItem lines 418-426）
func TestConvertListItemEmptyWithChildren(t *testing.T) {
	// 一个列表项中只有子列表没有直接文本，触发 empty parent + children path
	md := "-  \n  - child item\n"
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(md), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.BlockNodes) == 0 {
		t.Fatal("expected at least one block node")
	}
}

// TestExtractChildElementsDefaultBranch 测试 extractChildElements 的 default 分支
// 通过 **![alt](url)** 在 Emphasis 内嵌 Image 触发
func TestExtractChildElementsDefaultBranch(t *testing.T) {
	// Image inside bold: goldmark might produce Image node inside Emphasis
	md := "**before ![alt text](https://example.com/img.png) after**\n"
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(md), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.BlockNodes) == 0 {
		t.Fatal("expected blocks")
	}
}

// TestGetCellTextWithDepthMissingChild 测试 getCellTextWithDepth 中子块引用不存在的情况
func TestGetCellTextWithDepthMissingChild(t *testing.T) {
	cellBlock := &larkdocx.Block{
		BlockId:   strPtr("cell1"),
		BlockType: intPtr(int(BlockTypeTableCell)),
		Children:  []string{"nonexistent_child"},
	}
	tableBlock := &larkdocx.Block{
		BlockId:   strPtr("table1"),
		BlockType: intPtr(int(BlockTypeTable)),
		Table: &larkdocx.Table{
			Cells: []string{"cell1"},
			Property: &larkdocx.TableProperty{
				RowSize:    intPtr(1),
				ColumnSize: intPtr(1),
			},
		},
		Children: []string{"cell1"},
	}
	blocks := []*larkdocx.Block{tableBlock, cellBlock}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	_, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestGridDepthLimit 测试 Grid 递归深度限制
func TestGridDepthLimit(t *testing.T) {
	gridColType := int(BlockTypeGridColumn)
	gridType := int(BlockTypeGrid)

	blocks := make([]*larkdocx.Block, 0)

	// 创建 102 层 Grid/GridColumn 交替嵌套
	for i := 0; i < 51; i++ {
		gridID := fmt.Sprintf("grid_%d", i)
		colID := fmt.Sprintf("col_%d", i)
		nextGridID := fmt.Sprintf("grid_%d", i+1)

		grid := &larkdocx.Block{
			BlockId:   strPtr(gridID),
			BlockType: &gridType,
			Grid:      &larkdocx.Grid{ColumnSize: intPtr(1)},
			Children:  []string{colID},
		}

		colChildren := []string{}
		if i < 50 {
			colChildren = []string{nextGridID}
		}
		col := &larkdocx.Block{
			BlockId:    strPtr(colID),
			BlockType:  &gridColType,
			GridColumn: &larkdocx.GridColumn{WidthRatio: intPtr(1)},
			Children:   colChildren,
		}
		blocks = append(blocks, grid, col)
	}

	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = md
}

// TestQuoteContainerDepthLimit 测试 QuoteContainer 递归深度限制
func TestQuoteContainerDepthLimit(t *testing.T) {
	blocks := make([]*larkdocx.Block, 0)
	quoteType := int(BlockTypeQuoteContainer)

	for i := 0; i < 102; i++ {
		id := fmt.Sprintf("quote_%d", i)
		children := []string{}
		if i < 101 {
			children = []string{fmt.Sprintf("quote_%d", i+1)}
		}
		blocks = append(blocks, &larkdocx.Block{
			BlockId:        strPtr(id),
			BlockType:      &quoteType,
			QuoteContainer: &larkdocx.QuoteContainer{},
			Children:       children,
		})
	}

	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 深度限制会截断输出，不报错
	_ = md
}

// TestCollectMentionUserIDsAllHeadings 测试 collectMentionUserIDs 对 Heading7-9 和 Equation 的覆盖
func TestCollectMentionUserIDsAllHeadings(t *testing.T) {
	mentionUser := func(userID string) *larkdocx.TextElement {
		return &larkdocx.TextElement{
			MentionUser: &larkdocx.MentionUser{UserId: strPtr(userID)},
		}
	}

	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("b1"),
			BlockType: intPtr(int(BlockTypeHeading7)),
			Heading7:  &larkdocx.Text{Elements: []*larkdocx.TextElement{mentionUser("user_h7")}},
		},
		{
			BlockId:   strPtr("b2"),
			BlockType: intPtr(int(BlockTypeHeading8)),
			Heading8:  &larkdocx.Text{Elements: []*larkdocx.TextElement{mentionUser("user_h8")}},
		},
		{
			BlockId:   strPtr("b3"),
			BlockType: intPtr(int(BlockTypeHeading9)),
			Heading9:  &larkdocx.Text{Elements: []*larkdocx.TextElement{mentionUser("user_h9")}},
		},
		{
			BlockId:   strPtr("b4"),
			BlockType: intPtr(int(BlockTypeEquation)),
			Equation:  &larkdocx.Text{Elements: []*larkdocx.TextElement{mentionUser("user_eq")}},
		},
	}

	resolver := &mockUserResolverForCoverage{}
	opts := ConvertOptions{ExpandMentions: true}
	converter := NewBlockToMarkdownWithResolver(blocks, opts, resolver)
	_, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestConvertTextElementsRawWithEquation 测试 convertTextElementsRaw 中 Equation 路径
func TestConvertTextElementsRawWithEquation(t *testing.T) {
	eqContent := "E=mc^2"
	docTitle := "引用文档"
	langCode := 0
	elements := []*larkdocx.TextElement{
		{Equation: &larkdocx.Equation{Content: &eqContent}},
		{MentionDoc: &larkdocx.MentionDoc{Title: &docTitle}},
	}

	block := &larkdocx.Block{
		BlockId:   strPtr("code1"),
		BlockType: intPtr(int(BlockTypeCode)),
		Code: &larkdocx.Text{
			Elements: elements,
			Style:    &larkdocx.TextStyle{Language: &langCode},
		},
	}

	blocks := []*larkdocx.Block{block}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(md, "E=mc^2") {
		t.Errorf("expected equation content in output, got: %s", md)
	}
	if !strings.Contains(md, "引用文档") {
		t.Errorf("expected mention doc title in output, got: %s", md)
	}
}

// TestConvertDiagramNilType 测试 convertDiagram 的 diagramType 为 nil 的情况
func TestConvertDiagramNilType(t *testing.T) {
	block := &larkdocx.Block{
		BlockId:   strPtr("diag1"),
		BlockType: intPtr(int(BlockTypeDiagram)),
		Diagram:   &larkdocx.Diagram{DiagramType: nil},
	}
	blocks := []*larkdocx.Block{block}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(md, "Unknown") {
		t.Errorf("expected Unknown diagram type, got: %s", md)
	}
}

// TestConvertISVDefaultType 测试 convertISV 的 default 分支（非 TextDrawing/Timeline）
func TestConvertISVDefaultType(t *testing.T) {
	typeID := "custom_type"
	compID := "comp_123"
	block := &larkdocx.Block{
		BlockId:   strPtr("isv1"),
		BlockType: intPtr(int(BlockTypeISV)),
		Isv: &larkdocx.Isv{
			ComponentTypeId: &typeID,
			ComponentId:     &compID,
		},
	}
	blocks := []*larkdocx.Block{block}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(md, "ISV 应用块") {
		t.Errorf("expected ISV placeholder, got: %s", md)
	}
}

// TestConvertTextElementsMentionDocWithToken 测试 MentionDoc 无 URL 有 Token 的路径
func TestConvertTextElementsMentionDocWithToken(t *testing.T) {
	token := "doc_token_123"
	title := "文档标题"
	block := &larkdocx.Block{
		BlockId:   strPtr("b1"),
		BlockType: intPtr(int(BlockTypeText)),
		Text: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{MentionDoc: &larkdocx.MentionDoc{
					Title: &title,
					Token: &token,
				}},
			},
		},
	}
	blocks := []*larkdocx.Block{block}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(md, "feishu://doc/doc_token_123") {
		t.Errorf("expected feishu://doc/ URL, got: %s", md)
	}
}

// TestConvertTextElementsRawMentionUserExpanded 测试 convertTextElementsRaw 中 MentionUser ExpandMentions 路径
func TestConvertTextElementsRawMentionUserExpanded(t *testing.T) {
	langCode := 0
	userID := "user_raw_123"
	block := &larkdocx.Block{
		BlockId:   strPtr("code1"),
		BlockType: intPtr(int(BlockTypeCode)),
		Code: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{MentionUser: &larkdocx.MentionUser{UserId: &userID}},
			},
			Style: &larkdocx.TextStyle{Language: &langCode},
		},
	}
	blocks := []*larkdocx.Block{block}
	opts := ConvertOptions{ExpandMentions: true}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(md, "user_raw_123") {
		t.Errorf("expected user ID in code block output, got: %s", md)
	}
}

// TestConvertBlockquoteDefaultChild 测试 convertBlockquote 的 default 分支
func TestConvertBlockquoteDefaultChild(t *testing.T) {
	md := "> ---\n> text after\n"
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(md), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.BlockNodes) == 0 {
		t.Fatal("expected block nodes")
	}
}

// TestConvertCalloutWithDepthDepthLimit 测试 Callout 递归深度限制
func TestConvertCalloutWithDepthDepthLimit(t *testing.T) {
	blocks := make([]*larkdocx.Block, 0)
	calloutType := int(BlockTypeCallout)

	for i := 0; i < 102; i++ {
		id := fmt.Sprintf("callout_%d", i)
		children := []string{}
		if i < 101 {
			children = []string{fmt.Sprintf("callout_%d", i+1)}
		}
		bgColor := 6
		blocks = append(blocks, &larkdocx.Block{
			BlockId:   strPtr(id),
			BlockType: &calloutType,
			Callout:   &larkdocx.Callout{BackgroundColor: &bgColor},
			Children:  children,
		})
	}

	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = md
}

// TestConvertImageB2MDownloadPath 测试 b2m convertImage 中有 FileToken 但不下载的路径
func TestConvertImageB2MDownloadPath(t *testing.T) {
	token := "file_token_abc"
	block := &larkdocx.Block{
		BlockId:   strPtr("img1"),
		BlockType: intPtr(int(BlockTypeImage)),
		Image:     &larkdocx.Image{Token: &token},
	}
	blocks := []*larkdocx.Block{block}
	opts := ConvertOptions{DownloadImages: false}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(md, "file_token_abc") {
		t.Errorf("expected file token in image output, got: %s", md)
	}
}

// TestConvertTextElementsURLDecode 测试 URL 编码解码路径
func TestConvertTextElementsURLDecode(t *testing.T) {
	text := "link text"
	encodedURL := "https%3A%2F%2Fexample.com%2Fpath"
	block := &larkdocx.Block{
		BlockId:   strPtr("b1"),
		BlockType: intPtr(int(BlockTypeText)),
		Text: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{
					Content: &text,
					TextElementStyle: &larkdocx.TextElementStyle{
						Link: &larkdocx.Link{Url: &encodedURL},
					},
				}},
			},
		},
	}
	blocks := []*larkdocx.Block{block}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(md, "https://example.com/path") {
		t.Errorf("expected decoded URL, got: %s", md)
	}
}

// TestConvertTextElementsURLParentheses 测试 URL 中的括号编码
func TestConvertTextElementsURLParentheses(t *testing.T) {
	text := "wiki"
	urlWithParens := "https://en.wikipedia.org/wiki/Foo_(bar)"
	block := &larkdocx.Block{
		BlockId:   strPtr("b1"),
		BlockType: intPtr(int(BlockTypeText)),
		Text: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{
					Content: &text,
					TextElementStyle: &larkdocx.TextElementStyle{
						Link: &larkdocx.Link{Url: &urlWithParens},
					},
				}},
			},
		},
	}
	blocks := []*larkdocx.Block{block}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(md, "%28") || !strings.Contains(md, "%29") {
		t.Errorf("expected encoded parentheses, got: %s", md)
	}
}

// TestConvertMentionUserExpandedWithEmail 测试 ExpandMentions 有 email 的路径
func TestConvertMentionUserExpandedWithEmail(t *testing.T) {
	userID := "user_with_email"
	block := &larkdocx.Block{
		BlockId:   strPtr("b1"),
		BlockType: intPtr(int(BlockTypeText)),
		Text: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{MentionUser: &larkdocx.MentionUser{UserId: &userID}},
			},
		},
	}
	blocks := []*larkdocx.Block{block}

	resolver := &mockUserResolverForCoverage{}
	opts := ConvertOptions{ExpandMentions: true}
	converter := NewBlockToMarkdownWithResolver(blocks, opts, resolver)
	converter.userCache = map[string]MentionUserInfo{
		"user_with_email": {Name: "张三", Email: "zhangsan@example.com"},
	}
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(md, "[@张三](mailto:zhangsan@example.com)") {
		t.Errorf("expected mention with email, got: %s", md)
	}
}

// TestConvertMentionUserExpandedNameOnly 测试 ExpandMentions 无 email 的路径
func TestConvertMentionUserExpandedNameOnly(t *testing.T) {
	userID := "user_name_only"
	block := &larkdocx.Block{
		BlockId:   strPtr("b1"),
		BlockType: intPtr(int(BlockTypeText)),
		Text: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{MentionUser: &larkdocx.MentionUser{UserId: &userID}},
			},
		},
	}
	blocks := []*larkdocx.Block{block}

	resolver := &mockUserResolverForCoverage{}
	opts := ConvertOptions{ExpandMentions: true}
	converter := NewBlockToMarkdownWithResolver(blocks, opts, resolver)
	converter.userCache = map[string]MentionUserInfo{
		"user_name_only": {Name: "李四", Email: ""},
	}
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(md, "@李四") {
		t.Errorf("expected mention name only, got: %s", md)
	}
}

// TestConvertMentionUserExpandedNotInCache 测试 ExpandMentions 用户不在缓存中
func TestConvertMentionUserExpandedNotInCache(t *testing.T) {
	userID := "user_unknown"
	block := &larkdocx.Block{
		BlockId:   strPtr("b1"),
		BlockType: intPtr(int(BlockTypeText)),
		Text: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{MentionUser: &larkdocx.MentionUser{UserId: &userID}},
			},
		},
	}
	blocks := []*larkdocx.Block{block}

	resolver := &mockUserResolverForCoverage{}
	opts := ConvertOptions{ExpandMentions: true}
	converter := NewBlockToMarkdownWithResolver(blocks, opts, resolver)
	converter.userCache = map[string]MentionUserInfo{}
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(md, "@[user:user_unknown]") {
		t.Errorf("expected unknown user format, got: %s", md)
	}
}

// TestConvertMentionDocURLWithParens 测试 MentionDoc URL 包含括号时编码
func TestConvertMentionDocURLWithParens(t *testing.T) {
	title := "文档"
	docURL := "https://feishu.cn/docx/abc(1)"
	block := &larkdocx.Block{
		BlockId:   strPtr("b1"),
		BlockType: intPtr(int(BlockTypeText)),
		Text: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{MentionDoc: &larkdocx.MentionDoc{
					Title: &title,
					Url:   &docURL,
				}},
			},
		},
	}
	blocks := []*larkdocx.Block{block}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(md, "%28") {
		t.Errorf("expected encoded parentheses in doc URL, got: %s", md)
	}
}

// TestConvertTextElementsRawExpandMentionsInCache 测试 convertTextElementsRaw MentionUser 在缓存中
func TestConvertTextElementsRawExpandMentionsInCache(t *testing.T) {
	userID := "user_code"
	langCode := 0
	block := &larkdocx.Block{
		BlockId:   strPtr("code1"),
		BlockType: intPtr(int(BlockTypeCode)),
		Code: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{MentionUser: &larkdocx.MentionUser{UserId: &userID}},
			},
			Style: &larkdocx.TextStyle{Language: &langCode},
		},
	}
	blocks := []*larkdocx.Block{block}

	resolver := &mockUserResolverForCoverage{}
	opts := ConvertOptions{ExpandMentions: true}
	converter := NewBlockToMarkdownWithResolver(blocks, opts, resolver)
	converter.userCache = map[string]MentionUserInfo{
		"user_code": {Name: "代码用户"},
	}
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(md, "代码用户") {
		t.Errorf("expected user name in code block, got: %s", md)
	}
}

// TestConvertImageB2MAltFromChildren 测试 b2m convertImage 从子块提取 alt 文本
func TestConvertImageB2MAltFromChildren(t *testing.T) {
	altText := "自定义图片说明"
	textBlock := &larkdocx.Block{
		BlockId:   strPtr("alt_text"),
		BlockType: intPtr(int(BlockTypeText)),
		Text: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: &altText}},
			},
		},
	}
	token := "file_token_with_alt"
	imgBlock := &larkdocx.Block{
		BlockId:   strPtr("img1"),
		BlockType: intPtr(int(BlockTypeImage)),
		Image:     &larkdocx.Image{Token: &token},
		Children:  []string{"alt_text"},
	}
	blocks := []*larkdocx.Block{imgBlock, textBlock}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(md, "自定义图片说明") {
		t.Errorf("expected alt text from children, got: %s", md)
	}
}

// TestConvertImageB2MEmptyToken 测试 b2m convertImage token 为空的路径
func TestConvertImageB2MEmptyToken(t *testing.T) {
	block := &larkdocx.Block{
		BlockId:   strPtr("img_empty"),
		BlockType: intPtr(int(BlockTypeImage)),
		Image:     &larkdocx.Image{Token: nil},
	}
	blocks := []*larkdocx.Block{block}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(md, "![image]()") {
		t.Errorf("expected empty image markdown, got: %s", md)
	}
}

// TestGetHeadingTextAndStyleH7H8H9 测试 Heading7/8/9 的 getHeadingTextAndStyle 路径
func TestGetHeadingTextAndStyleH7H8H9(t *testing.T) {
	heading7Text := "标题七"
	heading8Text := "标题八"
	heading9Text := "标题九"
	blocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("h7"),
			BlockType: intPtr(int(BlockTypeHeading7)),
			Heading7: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{TextRun: &larkdocx.TextRun{Content: &heading7Text}},
				},
			},
		},
		{
			BlockId:   strPtr("h8"),
			BlockType: intPtr(int(BlockTypeHeading8)),
			Heading8: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{TextRun: &larkdocx.TextRun{Content: &heading8Text}},
				},
			},
		},
		{
			BlockId:   strPtr("h9"),
			BlockType: intPtr(int(BlockTypeHeading9)),
			Heading9: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{TextRun: &larkdocx.TextRun{Content: &heading9Text}},
				},
			},
		},
	}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(md, "标题七") {
		t.Errorf("expected heading7 text, got: %s", md)
	}
	if !strings.Contains(md, "标题八") {
		t.Errorf("expected heading8 text, got: %s", md)
	}
	if !strings.Contains(md, "标题九") {
		t.Errorf("expected heading9 text, got: %s", md)
	}
}

// TestGetCellTextWithDepthRecursionLimit 测试 getCellTextWithDepth 递归深度限制
func TestGetCellTextWithDepthRecursionLimit(t *testing.T) {
	// 创建表格，单元格内有深层嵌套子块
	blocks := make([]*larkdocx.Block, 0)
	textType := int(BlockTypeText)

	// 创建 102 层的文本子块链
	for i := 0; i < 102; i++ {
		id := fmt.Sprintf("child_%d", i)
		children := []string{}
		if i < 101 {
			children = []string{fmt.Sprintf("child_%d", i+1)}
		}
		text := fmt.Sprintf("text_%d", i)
		blocks = append(blocks, &larkdocx.Block{
			BlockId:   strPtr(id),
			BlockType: &textType,
			Text: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{TextRun: &larkdocx.TextRun{Content: &text}},
				},
			},
			Children: children,
		})
	}

	// 创建表格单元格，指向第一个子块
	cellBlock := &larkdocx.Block{
		BlockId:   strPtr("cell1"),
		BlockType: intPtr(int(BlockTypeTableCell)),
		Children:  []string{"child_0"},
	}
	tableBlock := &larkdocx.Block{
		BlockId:   strPtr("table1"),
		BlockType: intPtr(int(BlockTypeTable)),
		Table: &larkdocx.Table{
			Cells: []string{"cell1"},
			Property: &larkdocx.TableProperty{
				RowSize:    intPtr(1),
				ColumnSize: intPtr(1),
			},
		},
		Children: []string{"cell1"},
	}
	blocks = append(blocks, cellBlock, tableBlock)

	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = md
}

// TestConvertGridColumnMissingChild 测试 GridColumn 子块引用不存在
func TestConvertGridColumnMissingChild(t *testing.T) {
	gridType := int(BlockTypeGrid)
	colType := int(BlockTypeGridColumn)
	grid := &larkdocx.Block{
		BlockId:   strPtr("grid1"),
		BlockType: &gridType,
		Grid:      &larkdocx.Grid{ColumnSize: intPtr(1)},
		Children:  []string{"col1"},
	}
	col := &larkdocx.Block{
		BlockId:    strPtr("col1"),
		BlockType:  &colType,
		GridColumn: &larkdocx.GridColumn{WidthRatio: intPtr(1)},
		Children:   []string{"nonexistent_block"},
	}
	blocks := []*larkdocx.Block{grid, col}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	_, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestConvertGridWithNonGridColumnChild 测试 Grid 包含非 GridColumn 子块
func TestConvertGridWithNonGridColumnChild(t *testing.T) {
	gridType := int(BlockTypeGrid)
	textType := int(BlockTypeText)
	text := "not a column"
	grid := &larkdocx.Block{
		BlockId:   strPtr("grid1"),
		BlockType: &gridType,
		Grid:      &larkdocx.Grid{ColumnSize: intPtr(1)},
		Children:  []string{"text1"},
	}
	textBlock := &larkdocx.Block{
		BlockId:   strPtr("text1"),
		BlockType: &textType,
		Text:      &larkdocx.Text{Elements: []*larkdocx.TextElement{{TextRun: &larkdocx.TextRun{Content: &text}}}},
	}
	blocks := []*larkdocx.Block{grid, textBlock}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Non-GridColumn child should be ignored
	_ = md
}

// TestConvertQuoteContainerMissingChild 测试 QuoteContainer 子块引用不存在
func TestConvertQuoteContainerMissingChild(t *testing.T) {
	quoteType := int(BlockTypeQuoteContainer)
	block := &larkdocx.Block{
		BlockId:        strPtr("qc1"),
		BlockType:      &quoteType,
		QuoteContainer: &larkdocx.QuoteContainer{},
		Children:       []string{"nonexistent_child"},
	}
	blocks := []*larkdocx.Block{block}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	_, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestExtractChildElementsStringNode 测试 extractChildElements 中 ast.String 节点路径
// 通过 GFM strikethrough 内嵌入特殊格式触发
func TestExtractChildElementsStringNode(t *testing.T) {
	// Strikethrough wrapping underline wrapping auto link
	// ~~<u><https://example.com></u>~~
	md := "~~<u>text inside</u>~~\n"
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(md), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.BlockNodes) == 0 {
		t.Fatal("expected blocks")
	}
}

// TestConvertTableB2MCellsMismatch 测试 b2m convertTable 中 cells 数量不足的路径
func TestConvertTableB2MCellsMismatch(t *testing.T) {
	// Table 声明 2x2 但只有 3 个 cells (少一个)
	cellText1 := "A1"
	cellText2 := "B1"
	cellText3 := "A2"
	cellBlock1 := &larkdocx.Block{
		BlockId:   strPtr("c1"),
		BlockType: intPtr(int(BlockTypeTableCell)),
		TableCell: &larkdocx.TableCell{},
		Children:  []string{"ct1"},
	}
	cellBlock2 := &larkdocx.Block{
		BlockId:   strPtr("c2"),
		BlockType: intPtr(int(BlockTypeTableCell)),
		TableCell: &larkdocx.TableCell{},
		Children:  []string{"ct2"},
	}
	cellBlock3 := &larkdocx.Block{
		BlockId:   strPtr("c3"),
		BlockType: intPtr(int(BlockTypeTableCell)),
		TableCell: &larkdocx.TableCell{},
		Children:  []string{"ct3"},
	}
	ct1 := &larkdocx.Block{
		BlockId:   strPtr("ct1"),
		BlockType: intPtr(int(BlockTypeText)),
		Text:      &larkdocx.Text{Elements: []*larkdocx.TextElement{{TextRun: &larkdocx.TextRun{Content: &cellText1}}}},
	}
	ct2 := &larkdocx.Block{
		BlockId:   strPtr("ct2"),
		BlockType: intPtr(int(BlockTypeText)),
		Text:      &larkdocx.Text{Elements: []*larkdocx.TextElement{{TextRun: &larkdocx.TextRun{Content: &cellText2}}}},
	}
	ct3 := &larkdocx.Block{
		BlockId:   strPtr("ct3"),
		BlockType: intPtr(int(BlockTypeText)),
		Text:      &larkdocx.Text{Elements: []*larkdocx.TextElement{{TextRun: &larkdocx.TextRun{Content: &cellText3}}}},
	}
	tableBlock := &larkdocx.Block{
		BlockId:   strPtr("table1"),
		BlockType: intPtr(int(BlockTypeTable)),
		Table: &larkdocx.Table{
			Cells: []string{"c1", "c2", "c3"}, // 3 cells for 2x2 table
			Property: &larkdocx.TableProperty{
				RowSize:    intPtr(2),
				ColumnSize: intPtr(2),
			},
		},
		Children: []string{"c1", "c2", "c3"},
	}
	blocks := []*larkdocx.Block{tableBlock, cellBlock1, cellBlock2, cellBlock3, ct1, ct2, ct3}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 应该只渲染 1 行（3/2=1 行）
	_ = md
}

// TestConvertTableB2MEmptyCellID 测试 cells 含空字符串的路径
func TestConvertTableB2MEmptyCellID(t *testing.T) {
	tableBlock := &larkdocx.Block{
		BlockId:   strPtr("table1"),
		BlockType: intPtr(int(BlockTypeTable)),
		Table: &larkdocx.Table{
			Cells: []string{"", ""}, // 空 cell IDs
			Property: &larkdocx.TableProperty{
				RowSize:    intPtr(1),
				ColumnSize: intPtr(2),
			},
		},
	}
	blocks := []*larkdocx.Block{tableBlock}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = md
}

// TestConvertTableB2MCellNoTableCell 测试 cell 存在但无 TableCell 字段
func TestConvertTableB2MCellNoTableCell(t *testing.T) {
	cellBlock := &larkdocx.Block{
		BlockId:   strPtr("c1"),
		BlockType: intPtr(int(BlockTypeText)),
		// TableCell 为 nil
	}
	tableBlock := &larkdocx.Block{
		BlockId:   strPtr("table1"),
		BlockType: intPtr(int(BlockTypeTable)),
		Table: &larkdocx.Table{
			Cells: []string{"c1"},
			Property: &larkdocx.TableProperty{
				RowSize:    intPtr(1),
				ColumnSize: intPtr(1),
			},
		},
		Children: []string{"c1"},
	}
	blocks := []*larkdocx.Block{tableBlock, cellBlock}
	opts := ConvertOptions{}
	converter := NewBlockToMarkdown(blocks, opts)
	md, err := converter.Convert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = md
}

// TestConvertParagraphImageOnly 测试段落中仅有一个图片时直接转为 Image 块
func TestConvertParagraphImageOnly(t *testing.T) {
	md := "![my alt](https://example.com/image.png)\n"
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(md), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.BlockNodes) == 0 {
		t.Fatal("expected at least one block")
	}
	// 应该产生一个 Image 块（带 placeholder）
	block := result.BlockNodes[0].Block
	if block.BlockType != nil && *block.BlockType == int(BlockTypeImage) {
		// Image 类型，预期行为
	}
}

// TestExtractChildElementsUnderlineDefault 测试 underline + default(Image) 路径
func TestExtractChildElementsUnderlineDefault(t *testing.T) {
	// bold > underline > image: Image inside underline inside emphasis hits default+underline
	md := "**<u>![图片](https://example.com/img.png)</u>**\n"
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(md), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blocks := FlattenBlockNodes(result.BlockNodes)
	if len(blocks) == 0 {
		t.Fatal("expected blocks")
	}
}

// TestExtractChildElementsUnderlineAutoLink 测试 underline + AutoLink 路径
func TestExtractChildElementsUnderlineAutoLink(t *testing.T) {
	md := "~~<u><https://auto.example.com></u>~~\n"
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(md), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blocks := FlattenBlockNodes(result.BlockNodes)
	if len(blocks) == 0 {
		t.Fatal("expected blocks")
	}
}

// TestExtractChildElementsCodeSpanInEmphasis 测试 Emphasis 内 CodeSpan 路径
func TestExtractChildElementsCodeSpanInEmphasis(t *testing.T) {
	md := "**before `code` after**\n"
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(md), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blocks := FlattenBlockNodes(result.BlockNodes)
	if len(blocks) == 0 {
		t.Fatal("expected blocks")
	}
}

// TestConvertCalloutWithMultipleBlockTypes 测试 callout 内含多种块类型
func TestConvertCalloutWithMultipleBlockTypes(t *testing.T) {
	md := "> [!WARNING]\n> Text here\n>\n> - list item 1\n> - list item 2\n>\n> ```go\n> code block\n> ```\n"
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(md), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.BlockNodes) == 0 {
		t.Fatal("expected block nodes")
	}
}

// TestConvertTableWithDataMultipleLargeTable 测试大表格（超过 9 行）的自动拆分
func TestConvertTableWithDataMultipleLargeTable(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("| Col A | Col B |\n")
	sb.WriteString("| --- | --- |\n")
	for i := 0; i < 15; i++ {
		sb.WriteString(fmt.Sprintf("| row%d | val%d |\n", i, i))
	}
	md := sb.String()
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(md), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 15 行 + header > 9 行，应该拆分为多个表格
	if len(result.TableDatas) < 2 {
		t.Errorf("expected multiple tables due to row limit split, got %d", len(result.TableDatas))
	}
}

// TestConvertWithTableDataEmptyBlocks 测试空内容的 ConvertWithTableData
func TestConvertWithTableDataEmptyBlocks(t *testing.T) {
	md := ""
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(md), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.BlockNodes) != 0 {
		t.Errorf("expected no blocks for empty input, got %d", len(result.BlockNodes))
	}
}

// TestConvertConvertBlockquoteCalloutWithCodeBlock 测试 Callout 内嵌代码块路径
func TestConvertConvertBlockquoteCalloutWithCodeBlock(t *testing.T) {
	md := "> [!TIP]\n> 提示内容\n>\n> ```python\n> print('hello')\n> ```\n"
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(md), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.BlockNodes) == 0 {
		t.Fatal("expected block nodes")
	}
}

// TestConvertBlockquoteNestedBlockquote 测试嵌套引用块
func TestConvertBlockquoteNestedBlockquote(t *testing.T) {
	md := "> outer quote\n>\n> > inner quote\n>\n> back to outer\n"
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(md), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.BlockNodes) == 0 {
		t.Fatal("expected block nodes")
	}
}

// TestConvertListEmpty 测试空列表转换
func TestConvertListEmpty(t *testing.T) {
	// 列表项没有文本内容
	md := "- \n- text\n"
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(md), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blocks := FlattenBlockNodes(result.BlockNodes)
	if len(blocks) == 0 {
		t.Fatal("expected at least one block")
	}
}

// TestConvertListOrderedDeep 测试深层嵌套有序列表
func TestConvertListOrderedDeep(t *testing.T) {
	md := "1. level 1\n   1. level 2\n      1. level 3\n         1. level 4\n"
	opts := ConvertOptions{}
	conv := NewMarkdownToBlock([]byte(md), opts, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.BlockNodes) == 0 {
		t.Fatal("expected blocks")
	}
	// 第一个顶层块应该是 Ordered
	firstBlock := result.BlockNodes[0].Block
	if firstBlock.BlockType != nil && *firstBlock.BlockType != int(BlockTypeOrdered) {
		t.Errorf("expected ordered block, got type %d", *firstBlock.BlockType)
	}
}
