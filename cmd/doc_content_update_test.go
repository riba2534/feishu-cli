package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveMarkdownContentInlineUnescapesNewlines(t *testing.T) {
	got, err := resolveMarkdownContent("# 标题\\n\\n内容", "")
	if err != nil {
		t.Fatalf("resolveMarkdownContent() 返回错误: %v", err)
	}
	want := "# 标题\n\n内容"
	if got != want {
		t.Fatalf("resolveMarkdownContent() = %q，期望 %q", got, want)
	}
}

func TestResolveMarkdownContentFilePreservesLatexBackslash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "content.md")
	want := "$$\n\\nu + 1\n$$"
	if err := os.WriteFile(path, []byte(want), 0644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}

	got, err := resolveMarkdownContent("", path)
	if err != nil {
		t.Fatalf("resolveMarkdownContent() 返回错误: %v", err)
	}
	if got != want {
		t.Fatalf("resolveMarkdownContent() = %q，期望 %q", got, want)
	}
}
