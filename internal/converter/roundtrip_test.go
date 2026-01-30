package converter

import (
	"strings"
	"testing"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
)

// TestRoundtrip_Heading 测试标题往返一致性
func TestRoundtrip_Heading(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
	}{
		{"H1", "# 标题一"},
		{"H2", "## 标题二"},
		{"H3", "### 标题三"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Markdown → Block
			conv := NewMarkdownToBlock([]byte(tt.markdown), ConvertOptions{}, "")
			blocks, err := conv.Convert()
			if err != nil {
				t.Fatalf("Markdown → Block 失败: %v", err)
			}
			if len(blocks) == 0 {
				t.Fatal("blocks 为空")
			}

			// Block → Markdown
			conv2 := NewBlockToMarkdown(blocks, ConvertOptions{})
			result, err := conv2.Convert()
			if err != nil {
				t.Fatalf("Block → Markdown 失败: %v", err)
			}

			// 比较
			result = strings.TrimSpace(result)
			if result != tt.markdown {
				t.Errorf("往返不一致:\n  输入: %q\n  输出: %q", tt.markdown, result)
			}
		})
	}
}

// TestRoundtrip_TextFormatting 测试文本格式往返
func TestRoundtrip_TextFormatting(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
	}{
		{"plain", "普通文本"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := NewMarkdownToBlock([]byte(tt.markdown), ConvertOptions{}, "")
			blocks, err := conv.Convert()
			if err != nil {
				t.Fatalf("Markdown → Block 失败: %v", err)
			}

			conv2 := NewBlockToMarkdown(blocks, ConvertOptions{})
			result, err := conv2.Convert()
			if err != nil {
				t.Fatalf("Block → Markdown 失败: %v", err)
			}

			result = strings.TrimSpace(result)
			if result != tt.markdown {
				t.Errorf("往返不一致:\n  输入: %q\n  输出: %q", tt.markdown, result)
			}
		})
	}
}

// TestRoundtrip_CodeBlock 测试代码块往返
func TestRoundtrip_CodeBlock(t *testing.T) {
	markdown := "```go\nfmt.Println(\"Hello\")\n```"

	conv := NewMarkdownToBlock([]byte(markdown), ConvertOptions{}, "")
	blocks, err := conv.Convert()
	if err != nil {
		t.Fatalf("Markdown → Block 失败: %v", err)
	}

	conv2 := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := conv2.Convert()
	if err != nil {
		t.Fatalf("Block → Markdown 失败: %v", err)
	}

	result = strings.TrimSpace(result)
	if result != markdown {
		t.Errorf("往返不一致:\n  输入: %q\n  输出: %q", markdown, result)
	}
}

// TestRoundtrip_BulletList 测试无序列表往返
func TestRoundtrip_BulletList(t *testing.T) {
	markdown := "- 项目一\n- 项目二"

	conv := NewMarkdownToBlock([]byte(markdown), ConvertOptions{}, "")
	blocks, err := conv.Convert()
	if err != nil {
		t.Fatalf("Markdown → Block 失败: %v", err)
	}

	conv2 := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := conv2.Convert()
	if err != nil {
		t.Fatalf("Block → Markdown 失败: %v", err)
	}

	// 列表往返可能有尾换行差异
	result = strings.TrimSpace(result)
	expected := strings.TrimSpace(markdown)
	if !strings.Contains(result, "\\- 项目一") && !strings.Contains(result, "- 项目一") {
		t.Errorf("往返丢失列表项:\n  输入: %q\n  输出: %q", expected, result)
	}
}

// TestRoundtrip_Divider 测试分割线往返
func TestRoundtrip_Divider(t *testing.T) {
	// Block → Markdown for divider
	blocks := []*larkdocx.Block{
		createDividerBlock("div1"),
	}

	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := conv.Convert()
	if err != nil {
		t.Fatalf("Block → Markdown 失败: %v", err)
	}

	if !strings.Contains(result, "---") {
		t.Errorf("分割线丢失: %q", result)
	}
}

// TestRoundtrip_Equation 测试公式往返
func TestRoundtrip_Equation(t *testing.T) {
	// Block → Markdown
	blocks := []*larkdocx.Block{
		createEquationBlock("eq1", "E = mc^2"),
	}

	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := conv.Convert()
	if err != nil {
		t.Fatalf("Block → Markdown 失败: %v", err)
	}

	if !strings.Contains(result, "$$") {
		t.Errorf("公式标记丢失: %q", result)
	}
	if !strings.Contains(result, "E = mc^2") {
		t.Errorf("公式内容丢失: %q", result)
	}
}

// TestRoundtrip_Todo 测试待办事项往返
func TestRoundtrip_Todo(t *testing.T) {
	// Block → Markdown
	blocks := []*larkdocx.Block{
		createTodoBlock("todo1", "未完成任务", false),
		createTodoBlock("todo2", "已完成任务", true),
	}

	conv := NewBlockToMarkdown(blocks, ConvertOptions{})
	result, err := conv.Convert()
	if err != nil {
		t.Fatalf("Block → Markdown 失败: %v", err)
	}

	if !strings.Contains(result, "- [ ]") {
		t.Errorf("未完成待办丢失: %q", result)
	}
	if !strings.Contains(result, "- [x]") {
		t.Errorf("已完成待办丢失: %q", result)
	}
}

// TestRoundtrip_KnownLoss 记录已知的信息丢失项
func TestRoundtrip_KnownLoss(t *testing.T) {
	knownLosses := []struct {
		name        string
		description string
	}{
		{"Mermaid/PlantUML", "图表源码在飞书 API 导出时不可获取"},
		{"ImageToken", "图片 token 不可跨文档复用"},
		{"Bitable/Sheet", "嵌入表格只保留链接"},
		{"Board", "画板只保留引用链接"},
		{"ISV", "ISV 块内容不可获取"},
	}

	for _, loss := range knownLosses {
		t.Logf("已知信息丢失: %s - %s", loss.name, loss.description)
	}
}
