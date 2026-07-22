package cmd

import (
	"strings"
	"testing"
)

func TestNormalizeMarkdownHeadingText(t *testing.T) {
	cases := map[string]string{
		`user\_config 说明`:                 "user_config 说明",
		`**加粗** 标题`:                       "加粗 标题",
		`[官网](https://example.com) 入口`:    "官网 入口",
		`前缀 <mention-user id="ou_x"/> 后缀`: "前缀  后缀",
		`带 \[方括号\] 与 \#号`:                 "带 [方括号] 与 #号",
		"`code` 标题":                       "code 标题",
	}
	for in, want := range cases {
		if got := normalizeMarkdownHeadingText(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSliceMarkdownSectionEscapedHeading(t *testing.T) {
	md := "# 总览\n\n开头\n\n## user\\_config 说明\n\n配置内容\n\n### 子节\n\n子内容\n\n## 其他\n\n其他内容\n"
	// outline 输出的纯文本形态必须能命中
	section, candidates := sliceMarkdownSection(md, "user_config 说明")
	if section == "" {
		t.Fatal("转义标题应能被纯文本 heading 命中（outline→heading 工作流）")
	}
	if !strings.Contains(section, "配置内容") || !strings.Contains(section, "子内容") {
		t.Errorf("章节应含正文与子节，实际: %q", section)
	}
	if strings.Contains(section, "其他内容") {
		t.Errorf("章节不应包含下一同级节，实际: %q", section)
	}
	if len(candidates) != 1 {
		t.Errorf("candidates = %v", candidates)
	}
}

func TestSliceMarkdownSectionFenceAndFallback(t *testing.T) {
	md := "## 性能优化\n\n```go\n# 这不是标题\n```\n\n正文\n\n## 部署\n\n部署内容\n"
	section, _ := sliceMarkdownSection(md, "性能优化")
	if !strings.Contains(section, "# 这不是标题") || strings.Contains(section, "部署内容") {
		t.Errorf("围栏内 # 不应截断章节，实际: %q", section)
	}
	// 宽松兜底：用户少打了空格
	if s, _ := sliceMarkdownSection(md, "性能 优化"); s == "" {
		t.Error("canonical 兜底应命中忽略空格差异的查询")
	}
	if s, _ := sliceMarkdownSection(md, "不存在的标题"); s != "" {
		t.Error("不存在的标题应返回空")
	}
}
