package cmd

import (
	"regexp"
	"strings"
	"testing"
)

// TestGrepMarkdownLinesMatchesEscapedText 本地引擎导出时把 _ * 等转义成 \_，
// 按原文搜索（ANCHOR_TOKEN_ALPHA）也要命中，输出保持导出原样。
func TestGrepMarkdownLinesMatchesEscapedText(t *testing.T) {
	md := "第一段\n含 ANCHOR\\_TOKEN\\_ALPHA 的段落\n第三段"
	hits := grepMarkdownLines(md, regexp.MustCompile("ANCHOR_TOKEN_ALPHA"), 0)
	if len(hits) != 1 || !strings.Contains(hits[0], `ANCHOR\_TOKEN\_ALPHA`) {
		t.Fatalf("应命中转义后的行并原样输出: %q", hits)
	}
	// 按转义写法搜索仍然可用
	if hits := grepMarkdownLines(md, regexp.MustCompile(`ANCHOR\\_TOKEN`), 0); len(hits) != 1 {
		t.Fatalf("按转义写法搜索应命中: %q", hits)
	}
	if got := unescapeMarkdownPunct(`a\_b\*c\\d\中`); got != `a_b*c\d\中` {
		t.Fatalf("unescapeMarkdownPunct = %q", got)
	}
}
