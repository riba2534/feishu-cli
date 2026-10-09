package cmd

import (
	"fmt"
	"regexp"
	"strings"
)

// Markdown → post（md 标签）样式归一，移植自官方 shortcuts/im/helpers.go optimizeMarkdownStyle：
//  1. 先用占位符保护代码块；
//  2. 原文含 H1~H3 时降级标题：H1→H4、H2~H6→H5（飞书 post 的大标题过于醒目）；
//  3. 连续标题之间、表格前后补空行（否则 md 渲染会粘连）；
//  4. 还原代码块，压缩 3 个以上连续换行。
//
// 与官方不同：剔除非 img_ 图片引用拆成独立步骤（stripNonIMMarkdownImages），
// 必须在 --upload-images 上传本地图片之后执行，否则本地路径会在上传前被删掉。
var (
	mdReH2toH6     = regexp.MustCompile(`(?m)^#{2,6} (.+)$`)
	mdReH1         = regexp.MustCompile(`(?m)^# (.+)$`)
	mdReHasH1toH3  = regexp.MustCompile(`(?m)^#{1,3} `)
	mdReConsecH    = regexp.MustCompile(`(?m)^(#{4,5} .+)\n{1,2}(#{4,5} )`)
	mdReTableNoGap = regexp.MustCompile(`(?m)^([^|\n].*)\n(\|.+\|)`)
	mdReTableAfter = regexp.MustCompile(`(?m)((?:^\|.+\|[^\S\n]*\n?)+)`)
	mdReExcessNL   = regexp.MustCompile(`\n{3,}`)
	mdReImage      = regexp.MustCompile(`!\[[^\]]*\]\(([^)\s]+)\)`)
	mdReCodeBlock  = regexp.MustCompile("```[\\s\\S]*?```")
)

func optimizeMarkdownStyle(text string) string {
	const mark = "___FEISHU_CLI_CB_"
	var codeBlocks []string
	r := mdReCodeBlock.ReplaceAllStringFunc(text, func(m string) string {
		idx := len(codeBlocks)
		codeBlocks = append(codeBlocks, m)
		return fmt.Sprintf("%s%d___", mark, idx)
	})

	// 只有原文含 H1~H3 时才降级；先处理 H2~H6 再处理 H1，顺序不能反。
	if mdReHasH1toH3.MatchString(r) {
		r = mdReH2toH6.ReplaceAllString(r, "##### $1")
		r = mdReH1.ReplaceAllString(r, "#### $1")
	}
	r = mdReConsecH.ReplaceAllString(r, "$1\n\n$2")
	r = mdReTableNoGap.ReplaceAllString(r, "$1\n\n$2")
	r = mdReTableAfter.ReplaceAllString(r, "$1\n")

	for i, block := range codeBlocks {
		r = strings.Replace(r, fmt.Sprintf("%s%d___", mark, i), block, 1)
	}
	return mdReExcessNL.ReplaceAllString(r, "\n\n")
}

// stripNonIMMarkdownImages 删除 md 中无法渲染的图片引用（post 的 md 标签只认 img_xxx），
// 返回处理后的文本与被删除的图片地址，调用方据此在 stderr 提示。代码块内的内容不处理。
func stripNonIMMarkdownImages(text string) (string, []string) {
	if !strings.Contains(text, "![") {
		return text, nil
	}
	const mark = "___FEISHU_CLI_IMGCB_"
	var codeBlocks []string
	r := mdReCodeBlock.ReplaceAllStringFunc(text, func(m string) string {
		idx := len(codeBlocks)
		codeBlocks = append(codeBlocks, m)
		return fmt.Sprintf("%s%d___", mark, idx)
	})
	var removed []string
	r = mdReImage.ReplaceAllStringFunc(r, func(m string) string {
		sub := mdReImage.FindStringSubmatch(m)
		if len(sub) == 2 && strings.HasPrefix(sub[1], "img_") {
			return m
		}
		if len(sub) == 2 {
			removed = append(removed, sub[1])
		}
		return ""
	})
	for i, block := range codeBlocks {
		r = strings.Replace(r, fmt.Sprintf("%s%d___", mark, i), block, 1)
	}
	return r, removed
}
