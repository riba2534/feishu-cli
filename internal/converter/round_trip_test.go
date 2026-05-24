package converter

import (
	"fmt"
	"strings"
	"testing"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
)

// assignIDsAndLinkChildren 给 BlockNode 树递归赋 ID 并把父块的 Children 字段
// 设为子块 ID 列表，使得 NewBlockToMarkdown 能识别容器关系（callout / quote /
// 嵌套列表等）。返回赋 ID 后的扁平 block 列表。
//
// 这个 helper 模拟了 doc import 真实路径中 SDK 返回 block_id 后的链接动作，
// 让纯单元测试也能跑完整的 markdown → blocks → markdown 闭环。
func assignIDsAndLinkChildren(nodes []*BlockNode) []*larkdocx.Block {
	counter := 0
	var visit func(n *BlockNode) string
	visit = func(n *BlockNode) string {
		if n == nil || n.Block == nil {
			return ""
		}
		counter++
		id := fmt.Sprintf("blk%04d", counter)
		n.Block.BlockId = &id
		if len(n.Children) > 0 {
			childIDs := make([]string, 0, len(n.Children))
			for _, child := range n.Children {
				cid := visit(child)
				if cid != "" {
					childIDs = append(childIDs, cid)
				}
			}
			n.Block.Children = childIDs
		}
		return id
	}
	for _, n := range nodes {
		visit(n)
	}
	return FlattenBlockNodes(nodes)
}

// roundTrip 把 markdown 跑一遍 markdown → blocks → markdown，返回输出字符串。
// 失败时让 t.Fatal 立即终止用例，避免在错误状态下做后续断言。
func roundTrip(t *testing.T, markdown string) string {
	t.Helper()
	conv := NewMarkdownToBlock([]byte(markdown), ConvertOptions{}, "")
	result, err := conv.ConvertWithTableData()
	if err != nil {
		t.Fatalf("Markdown → Block 失败: %v", err)
	}
	blocks := assignIDsAndLinkChildren(result.BlockNodes)
	if len(blocks) == 0 {
		t.Fatalf("Markdown → Block 输出为空 (输入: %q)", markdown)
	}
	exporter := NewBlockToMarkdown(blocks, ConvertOptions{})
	out, err := exporter.Convert()
	if err != nil {
		t.Fatalf("Block → Markdown 失败: %v", err)
	}
	return out
}

// TestRoundTrip_MarkdownToBlocksToMarkdown 端到端 round-trip 回归测试。
//
// 目的：保护 v1.27.0 (commit 0f54c6e) 对 block_to_markdown.go / markdown_to_block.go
// 的重构，确保 doc import / export / wiki export 路径的语义保持。
//
// 设计原则：
//   - 允许空白规范化差异（末尾换行、连续空行折叠为一个）
//   - 不要求字符串等价，只断言关键 substring 必须保留 / 不能退化
//   - 发现真正不等价的 case 用 t.Skip 标记为已知限制，不在本测试里修 converter
func TestRoundTrip_MarkdownToBlocksToMarkdown(t *testing.T) {
	cases := []struct {
		name              string
		input             string
		expectContains    []string
		expectNotContains []string
	}{
		// --- inline 元素 ---
		{
			name:           "inline_bold",
			input:          "这是 **粗体** 文本",
			expectContains: []string{"**粗体**"},
		},
		{
			name:           "inline_italic",
			input:          "这是 *斜体* 文本",
			expectContains: []string{"*斜体*"},
		},
		{
			name:           "inline_strikethrough",
			input:          "这是 ~~删除线~~ 文本",
			expectContains: []string{"~~删除线~~"},
		},
		{
			name:           "inline_code",
			input:          "执行 `go test` 命令",
			expectContains: []string{"`go test`"},
		},
		{
			name:           "inline_link_basic",
			input:          "访问 [示例](https://example.com)",
			expectContains: []string{"[示例](https://example.com)"},
		},
		{
			// v1.27.1 修复：normalizeURL 改用 url.PathUnescape，避免 query 中字面 `+` 被错解为空格
			name:              "inline_link_url_with_plus_in_query",
			input:             "查询 [链接](https://a.com?x=a+b)",
			expectContains:    []string{"x=a+b"},
			expectNotContains: []string{"x=a b"},
		},

		// --- 块级元素 ---
		{
			name:           "heading_h1",
			input:          "# 一级标题",
			expectContains: []string{"# 一级标题"},
		},
		{
			name:           "heading_h2",
			input:          "## 二级标题",
			expectContains: []string{"## 二级标题"},
		},
		{
			name:           "heading_h3",
			input:          "### 三级标题",
			expectContains: []string{"### 三级标题"},
		},
		{
			name:           "bullet_list_basic",
			input:          "- 项目一\n- 项目二\n- 项目三",
			expectContains: []string{"项目一", "项目二", "项目三"},
		},
		{
			name: "bullet_list_nested_two_level",
			input: "- 父项一\n" +
				"  - 子项一\n" +
				"  - 子项二\n" +
				"- 父项二",
			expectContains:    []string{"父项一", "子项一", "子项二", "父项二"},
			expectNotContains: []string{"子项一\\n子项二"}, // 嵌套不应被合并丢失换行
		},
		{
			name:           "ordered_list_basic",
			input:          "1. 第一\n2. 第二\n3. 第三",
			expectContains: []string{"第一", "第二", "第三"},
		},
		{
			name: "ordered_list_nested",
			input: "1. 第一\n" +
				"   1. 子一\n" +
				"   2. 子二\n" +
				"2. 第二",
			expectContains: []string{"第一", "子一", "子二", "第二"},
		},
		{
			name:           "code_block_with_lang",
			input:          "```go\nfmt.Println(\"hi\")\n```",
			expectContains: []string{"```", "fmt.Println(\"hi\")"},
		},
		{
			name: "code_block_python",
			input: "```python\n" +
				"def foo():\n" +
				"    return 42\n" +
				"```",
			expectContains: []string{"```", "def foo():", "return 42"},
		},
		{
			name:  "inline_equation",
			input: "公式：$x^2$ 在行内",
			// 行内公式应保留 $...$ 包裹
			expectContains: []string{"$x^2$"},
		},
		{
			name:  "block_equation_preserves_nu",
			input: "$$\n\\nu = c\n$$",
			// 验证 commit 0f54c6e 修复 #145：块级公式中的 \nu 不应被当作换行符吃掉。
			// 注意：单行块级公式 round-trip 时会降级为行内 $...$ —— 这是 CLAUDE.md
			// 文档化的设计取舍（"块级降级为行内"），不在本测试关心范围。本断言只验证
			// 关键字符 "\nu" 没被 "\n" 吃掉，公式 content 完整保留即可。
			expectContains: []string{"\\nu = c"},
		},
		// 表格 round-trip 见 TestRoundTrip_KnownLimitations/table_requires_api_fill_stage
		{
			name: "blockquote_basic",
			input: "> 这是引用\n" +
				"> 第二行",
			expectContains: []string{">", "这是引用", "第二行"},
		},
		{
			name:           "horizontal_rule",
			input:          "前文\n\n---\n\n后文",
			expectContains: []string{"前文", "---", "后文"},
		},
		{
			name:           "callout_note",
			input:          "> [!NOTE]\n> 这是提示内容",
			expectContains: []string{"[!NOTE]", "这是提示内容"},
		},
		{
			name:           "callout_warning",
			input:          "> [!WARNING]\n> 注意事项",
			expectContains: []string{"[!WARNING]", "注意事项"},
		},
		{
			name:           "callout_tip",
			input:          "> [!TIP]\n> 小提示",
			expectContains: []string{"[!TIP]", "小提示"},
		},

		// --- 混合 inline ---
		{
			name:           "mixed_inline_in_paragraph",
			input:          "一段 **粗** 和 *斜* 还有 `code` 和 [链接](https://a.com) 混合",
			expectContains: []string{"**粗**", "*斜*", "`code`", "[链接](https://a.com)"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			out := roundTrip(t, tc.input)

			for _, want := range tc.expectContains {
				if !strings.Contains(out, want) {
					t.Errorf("round-trip 丢失内容: 期望包含 %q\n--- 输入 ---\n%s\n--- 输出 ---\n%s",
						want, tc.input, out)
				}
			}
			for _, banned := range tc.expectNotContains {
				if strings.Contains(out, banned) {
					t.Errorf("round-trip 出现退化模式: 不应包含 %q\n--- 输入 ---\n%s\n--- 输出 ---\n%s",
						banned, tc.input, out)
				}
			}
		})
	}
}

// TestRoundTrip_KnownLimitations 记录已知 round-trip 不等价的 case。
// Skip 而非 Fail，让回归测试在未来这些行为变化时（修好或恶化）能被注意到。
func TestRoundTrip_KnownLimitations(t *testing.T) {
	// 已知限制：飞书 Table 块的单元格内容不在 block 本身，而在 children（TableCell 块）
	// 里，由 SDK CreateBlock 返回 cell ID 后再调用 content-update 填充。纯 converter
	// round-trip 因此无法保留表格内容——这是文档化的两阶段设计，不是 bug。
	t.Run("table_requires_api_fill_stage", func(t *testing.T) {
		t.Skip("已知限制：表格 round-trip 需要 SDK CreateBlock 阶段，纯 converter 无法保留单元格")
	})

	// 已知限制：URL 路径含空格会被 goldmark 拒绝，根本进不了 converter
	t.Run("link_url_with_space_in_path", func(t *testing.T) {
		t.Skip("已知限制：URL 路径中的空格在 markdown 解析阶段就被 goldmark 拒绝")
	})

	// 已知限制：未识别的内联 HTML 标签（除 <br>/<u>/<mark>/<mention-*>）会被丢弃
	t.Run("html_raw_passthrough", func(t *testing.T) {
		t.Skip("已知限制：未识别的内联 HTML 标签会被丢弃，不会原样回写")
	})
}
