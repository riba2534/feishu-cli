package cmd

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/converter"
	"github.com/spf13/cobra"
)

// doc read —— 大文档的选择性读取：大纲 / 按标题取节 / 关键词定位。
// 与 doc export（整篇导出）互补：AI 处理大文档时先 --outline 看结构，
// 再 --heading 只取目标章节，或 --keyword 直接定位内容，避免整篇拉取撑爆上下文。
var docReadCmd = &cobra.Command{
	Use:   "read <document_id|url>",
	Short: "选择性读取文档（大纲 / 按标题取节 / 关键词定位）",
	Long: `对大文档做选择性读取，避免整篇导出。三种模式（三选一）：

  --outline            输出标题大纲（层级缩进 + block_id），先看结构再决定读哪节
  --heading <文本>     按标题定位章节：输出该标题到下一个同级/更高级标题之间的 Markdown
  --keyword <正则>     按内容定位：输出命中行及上下文（--context 控制上下文行数）

示例:
  # 第一步：看结构
  feishu-cli doc read ABC123 --outline

  # 第二步：只读"性能优化"这一节
  feishu-cli doc read ABC123 --heading "性能优化"

  # 或直接按关键词定位（支持正则，多个词用 | 连接）
  feishu-cli doc read ABC123 --keyword "QPS|限流" --context 5

提示:
  - --heading 按子串匹配标题文本，命中多个时取第一个并提示其余候选
  - block 级精确分页读取用 doc blocks；整篇导出用 doc export`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		documentID, err := extractDocToken(args[0])
		if err != nil {
			return err
		}
		outline, _ := cmd.Flags().GetBool("outline")
		heading, _ := cmd.Flags().GetString("heading")
		keyword, _ := cmd.Flags().GetString("keyword")
		contextLines, _ := cmd.Flags().GetInt("context")

		modes := 0
		for _, on := range []bool{outline, heading != "", keyword != ""} {
			if on {
				modes++
			}
		}
		if modes != 1 {
			return fmt.Errorf("请从 --outline / --heading / --keyword 中选择且仅选择一种模式")
		}

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		if outline {
			blocks, err := client.GetAllBlocksWithToken(documentID, userAccessToken)
			if err != nil {
				return fmt.Errorf("获取块失败: %w", err)
			}
			count := 0
			for _, block := range blocks {
				level, text, ok := converter.HeadingInfo(block)
				if !ok {
					continue
				}
				id := ""
				if block.BlockId != nil {
					id = *block.BlockId
				}
				fmt.Printf("%s- %s  [%s]\n", strings.Repeat("  ", level-1), text, id)
				count++
			}
			if count == 0 {
				fmt.Println("（文档无标题块）")
			}
			return nil
		}

		markdown, err := exportDocMarkdown(documentID, userAccessToken)
		if err != nil {
			return err
		}

		if heading != "" {
			section, candidates := sliceMarkdownSection(markdown, heading)
			if section == "" {
				return fmt.Errorf("未找到包含 %q 的标题（用 --outline 查看全部标题）", heading)
			}
			if len(candidates) > 1 {
				fmt.Fprintf(cmd.ErrOrStderr(), "提示: 有 %d 个标题匹配 %q，已输出第一个；其余: %s\n",
					len(candidates), heading, strings.Join(candidates[1:], " / "))
			}
			fmt.Print(section)
			return nil
		}

		// keyword 模式
		re, err := regexp.Compile(keyword)
		if err != nil {
			return fmt.Errorf("--keyword 正则无效: %w", err)
		}
		hits := grepMarkdownLines(markdown, re, contextLines)
		if len(hits) == 0 {
			return fmt.Errorf("未找到匹配 %q 的内容", keyword)
		}
		fmt.Print(strings.Join(hits, "\n---\n"))
		fmt.Println()
		return nil
	},
}

// exportDocMarkdown 复用导出管线把整篇文档转为 Markdown（不下载图片）。
func exportDocMarkdown(documentID, userAccessToken string) (string, error) {
	blocks, err := client.GetAllBlocksWithToken(documentID, userAccessToken)
	if err != nil {
		return "", fmt.Errorf("获取块失败: %w", err)
	}
	conv := converter.NewBlockToMarkdown(blocks, converter.ConvertOptions{
		DocumentID:      documentID,
		UserAccessToken: userAccessToken,
	})
	markdown, err := conv.Convert()
	if err != nil {
		return "", fmt.Errorf("转换为 Markdown 失败: %w", err)
	}
	return markdown, nil
}

var markdownHeadingRe = regexp.MustCompile(`^(#{1,9})\s+(.*)$`)

// sliceMarkdownSection 在 Markdown 里按标题子串取节：从第一个匹配标题行到
// 下一个同级或更高级标题行之前。返回 (节内容, 全部匹配的标题文本)。
// 跳过代码围栏内的 # 行，避免把代码注释误判为标题。
//
// 匹配做两级归一：先把渲染后的标题行还原为纯文本（去反斜杠转义/链接/mention 标记，
// 与 --outline 的块树纯文本输出对齐——outline 输出复制给 --heading 必须能命中）；
// 仍无命中时退化为"仅保留字母数字与 CJK"的宽松匹配兜底。
func sliceMarkdownSection(markdown, headingSubstr string) (string, []string) {
	lines := strings.Split(markdown, "\n")
	type headingLine struct {
		idx   int
		level int
		text  string
	}
	inFence := false
	var headings []headingLine
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if m := markdownHeadingRe.FindStringSubmatch(line); m != nil {
			headings = append(headings, headingLine{idx: i, level: len(m[1]), text: normalizeMarkdownHeadingText(m[2])})
		}
	}
	match := func(text, substr string) bool { return strings.Contains(text, substr) }
	var candidates []string
	start, level := -1, 0
	for pass := 0; pass < 2 && start < 0; pass++ {
		if pass == 1 {
			// 宽松兜底：忽略空格/标点差异
			match = func(text, substr string) bool {
				return strings.Contains(canonicalHeadingKey(text), canonicalHeadingKey(substr))
			}
			candidates = nil
		}
		for _, h := range headings {
			if match(h.text, headingSubstr) {
				candidates = append(candidates, h.text)
				if start < 0 {
					start, level = h.idx, h.level
				}
			}
		}
	}
	if start < 0 {
		return "", nil
	}
	end := len(lines)
	inFence = false
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if m := markdownHeadingRe.FindStringSubmatch(lines[i]); m != nil && len(m[1]) <= level {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n") + "\n", candidates
}

// grepMarkdownLines 按正则匹配行，返回每个命中点带上下文的片段（相邻/重叠命中合并）。
func grepMarkdownLines(markdown string, re *regexp.Regexp, contextLines int) []string {
	if contextLines < 0 {
		contextLines = 0
	}
	lines := strings.Split(markdown, "\n")
	var matched []int
	for i, line := range lines {
		if re.MatchString(line) {
			matched = append(matched, i)
		}
	}
	if len(matched) == 0 {
		return nil
	}
	// 合并重叠区间
	type span struct{ lo, hi int }
	var spans []span
	for _, idx := range matched {
		lo, hi := idx-contextLines, idx+contextLines
		if lo < 0 {
			lo = 0
		}
		if hi > len(lines)-1 {
			hi = len(lines) - 1
		}
		if n := len(spans); n > 0 && lo <= spans[n-1].hi+1 {
			if hi > spans[n-1].hi {
				spans[n-1].hi = hi
			}
			continue
		}
		spans = append(spans, span{lo, hi})
	}
	out := make([]string, 0, len(spans))
	for _, s := range spans {
		out = append(out, strings.Join(lines[s.lo:s.hi+1], "\n"))
	}
	return out
}

// stripMarkdownEmphasis 去掉标题文本里的行内标记字符，便于子串匹配。
func stripMarkdownEmphasis(s string) string {
	return strings.NewReplacer("**", "", "*", "", "`", "", "~~", "", "__", "").Replace(strings.TrimSpace(s))
}

var (
	markdownLinkRe    = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	markdownMentionRe = regexp.MustCompile(`<mention-[a-z]+[^>]*/?>`)
	markdownEscapeRe  = regexp.MustCompile(`\\([\\` + "`" + `*_{}\[\]()#+\-.!~|>$])`)
)

// normalizeMarkdownHeadingText 把渲染后的 Markdown 标题文本还原为纯文本：
// 链接取显示文本、去 mention 标记、去反斜杠转义、去强调标记。
// 目标：与 converter.HeadingInfo（--outline 输出）的纯文本形态一致，
// 保证 outline → --heading 的复制粘贴工作流可靠命中。
func normalizeMarkdownHeadingText(s string) string {
	s = markdownLinkRe.ReplaceAllString(s, "$1")
	s = markdownMentionRe.ReplaceAllString(s, "")
	s = markdownEscapeRe.ReplaceAllString(s, "$1")
	return stripMarkdownEmphasis(s)
}

// canonicalHeadingKey 宽松匹配键：仅保留字母、数字与非标点非空白字符（含 CJK），转小写。
func canonicalHeadingKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r > 127 || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func init() {
	docCmd.AddCommand(docReadCmd)
	docReadCmd.Flags().Bool("outline", false, "输出标题大纲（层级 + block_id）")
	docReadCmd.Flags().String("heading", "", "按标题子串定位并输出该章节")
	docReadCmd.Flags().String("keyword", "", "按内容正则定位，输出命中行及上下文")
	docReadCmd.Flags().Int("context", 3, "--keyword 模式的上下文行数（默认 3）")
	docReadCmd.Flags().String("user-access-token", "", "User Access Token")
}
