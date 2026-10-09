package cmd

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
)

// 文本级替换（docs_ai str_replace）。
//
// 此前 `replace_all --selection-with-ellipsis "旧文本"` 把"含旧文本的整个顶层块"整块 block_replace，
// 段落其余文字全部丢失（实测：整段被替换成"新文本"）。现在纯文本选择器改走服务端 str_replace，
// 只替换文字本身，所在段落其余文字与行内样式原样保留。
//
// 服务端 str_replace 的契约（测试文档实测）：
//   - pattern 按文档的 Markdown（或 XML）序列化逐字匹配：`**B2**` 能匹配，跨样式的 "锚点 B2" 匹配不到；
//   - 只接受全文唯一命中：未命中 result=failed + degrade_code=1013，命中多处 result=failed + degrade_code=1014；
//   - 不接受 block_id / start_block_id 限定范围（3380002）；
//   - 文档标题在 Markdown 序列化中是第一行 "# 标题"，对它做 str_replace 会在正文新建一个 H1 而非改标题。
//
// 因此 replace_all 的多处命中采用"唯一上下文窗口"：先 fetch 当前文档序列化，为每处命中向两侧
// 扩展同一行内的安全字符（字母/数字/空格/中文标点，不跨 Markdown 标记、不跨行首），直到窗口全文唯一，
// 再逐个以 str_replace(窗口原文 → 窗口原文中的命中替换为新内容) 提交。任何一处无法构造唯一窗口时，
// 在写入前整体 fail-closed，不做部分替换。

const (
	degradeStrReplaceNotFound = 1013
	degradeStrReplaceMultiple = 1014
)

// textWindow 是一处命中对应的唯一替换窗口。
type textWindow struct {
	start, end int    // 窗口在序列化文本中的字节区间
	pattern    string // 发送给 str_replace 的 pattern（窗口原文）
	content    string // 发送给 str_replace 的 content（命中部分替换后的窗口）
}

// doTextReplace 文本级替换：all=false 要求唯一命中；all=true 替换全部命中。
func (p *contentUpdateParams) doTextReplace(pattern string, all bool) error {
	if pattern == "" {
		return clierr.Usagef("文本级替换的匹配文本不能为空")
	}
	// 格式选择：markdown 序列化下的 str_replace 会按 Markdown 重写所在段落，下划线、文字颜色等
	// Markdown 无法表达的样式随之丢失（实测）；XML 序列化下只改文字、块 ID 与样式都保留。
	// 因此未显式指定 --doc-format 且匹配文本与替换内容都是纯文字时，自动改走 XML。
	format := p.format()
	replacement := p.content
	displayPattern := pattern
	if format == "markdown" && !p.formatSet && isPlainInlineText(pattern) && isPlainInlineText(replacement) {
		format = "xml"
		pattern = xmlEscapeText(pattern)
		replacement = xmlEscapeText(replacement)
	} else if format == "markdown" {
		fmt.Fprintln(p.errOut(), "提示: 匹配文本或替换内容含 Markdown 语法，按 Markdown 序列化替换；所在段落中 Markdown 无法表达的样式（下划线、文字颜色等）会丢失。只替换纯文字时 CLI 会自动走 XML 以保留样式")
	}

	fetchBody := map[string]any{"format": format}
	data, err := client.FetchDocsAI(p.documentID, fetchBody, p.userToken)
	if err != nil {
		return fmt.Errorf("读取文档当前内容失败（文本级替换需要先定位命中）: %w", err)
	}
	doc, fetchedRev := client.DocsAIDocumentContent(data)
	bodyStart := docsAIBodyStart(doc, format)
	occ := findTextOccurrences(doc, pattern, bodyStart)

	if len(occ) == 0 {
		if strings.Contains(doc[:bodyStart], pattern) {
			return clierr.Usagef("文本 %q 只出现在文档标题中；content-update 的文本级替换不修改标题（服务端会在正文新建一个标题块）", displayPattern)
		}
		hint := "跨样式的文字需带上样式标签（如 <b>粗体</b>），& < > 按 &amp; &lt; &gt; 书写"
		if format == "markdown" {
			hint = "含 _ * [ ] 等字符时需写成转义形式（如 str\\_replace），跨样式的文字需带上样式标记（如 **粗体**）"
		}
		return fmt.Errorf("未找到文本 %q。文本级替换按文档的 %s 序列化逐字匹配：%s；可先用 `feishu-cli doc read %s --engine docs_ai --doc-format %s` 查看序列化原文",
			displayPattern, format, hint, p.documentID, format)
	}
	if !all && len(occ) > 1 {
		return clierr.Usagef("文本 %q 在文档中出现 %d 处，%s 只替换唯一命中，已拒绝执行以免改错位置；请带上更多上下文使其唯一，或改用 --mode replace_all 替换全部",
			displayPattern, len(occ), p.mode)
	}

	windows, unresolved := buildTextReplaceWindows(doc, pattern, replacement, occ, bodyStart)
	if len(unresolved) > 0 {
		var lines []string
		for i, at := range unresolved {
			if i >= 5 {
				lines = append(lines, "…")
				break
			}
			lines = append(lines, fmt.Sprintf("「%s」", excerptAround(doc, at, len(pattern))))
		}
		return fmt.Errorf("文本 %q 共 %d 处命中，其中 %d 处无法构造全文唯一的替换上下文（如同一行重复出现或紧邻 Markdown 标记），为避免改错位置已拒绝执行，文档未修改：%s；"+
			"请改用 --mode replace_range --block-id <块ID> 整块改写这些段落（块 ID 用 `feishu-cli doc read %s --with-ids` 获取）",
			displayPattern, len(occ), len(unresolved), strings.Join(lines, "、"), p.documentID)
	}

	revision := p.revisionID
	if revision == -1 && fetchedRev > 0 {
		// 以读取时的版本为基准写入：读取到写入之间若有他人修改，服务端拒绝而不是把替换写错位置
		revision = fetchedRev
	}

	replaced := 0
	var lastData map[string]any
	for i, w := range windows {
		body := p.newBody("str_replace")
		body["format"] = format
		body["pattern"] = w.pattern
		body["content"] = w.content
		d, err := p.sendUpdate(body, revision)
		if err != nil {
			err = explainStrReplaceError(err)
			if len(windows) == 1 {
				if rerr, ok := client.AsDocsAIResultError(err); ok && p.output == "json" && rerr.Data != nil {
					if perr := printJSONTo(p.out(), rerr.Data); perr != nil {
						return perr
					}
				}
				return fmt.Errorf("文本替换失败: %w", err)
			}
			return fmt.Errorf("全文替换未完全完成：共 %d 处匹配，已成功完成 %d 处，在第 %d 处替换失败: %w",
				len(windows), replaced, i+1, err)
		}
		replaced++
		lastData = d
		if p.output != "json" {
			p.printWarnings(d)
		}
		if i < len(windows)-1 {
			next := extractRevisionID(d)
			if next <= 0 {
				return fmt.Errorf("全文替换中断：共 %d 处匹配，已成功完成 %d 处，但服务端未返回新的 revision_id；为防止并发数据破坏已停止后续未保护替换",
					len(windows), replaced)
			}
			revision = next
		}
		if len(windows) > 10 && p.output != "json" && (i+1)%10 == 0 {
			fmt.Fprintf(p.errOut(), "  进度: %d/%d\n", i+1, len(windows))
		}
	}

	if p.output == "json" {
		if !all && lastData != nil {
			lastData["replaced_count"] = replaced
			lastData["granularity"] = "text"
			return printJSONTo(p.out(), lastData)
		}
		out := map[string]any{
			"document_id":    p.documentID,
			"replaced_count": replaced,
			"granularity":    "text",
		}
		if rev := extractRevisionID(lastData); rev > 0 {
			out["revision_id"] = rev
		}
		if logID, _ := lastData["log_id"].(string); logID != "" {
			out["log_id"] = logID
		}
		return printJSONTo(p.out(), out)
	}
	switch {
	case all:
		fmt.Fprintf(p.out(), "全文替换完成，共替换 %d 处（文本级，段落其余内容保留）\n", replaced)
	case p.content == "":
		fmt.Fprintf(p.out(), "已删除文本 %q（1 处）\n", strings.TrimSpace(p.selEllipsis+p.pattern))
	default:
		fmt.Fprintf(p.out(), "已替换文本 %q（1 处，段落其余内容保留）\n", strings.TrimSpace(p.selEllipsis+p.pattern))
	}
	return nil
}

// explainStrReplaceError 为 str_replace 的 degrade_code 补充可执行的中文说明。
func explainStrReplaceError(err error) error {
	rerr, ok := client.AsDocsAIResultError(err)
	if !ok {
		return err
	}
	switch {
	case rerr.HasDegradeCode(degradeStrReplaceNotFound):
		return fmt.Errorf("服务端未找到匹配文本（文档可能在读取后被修改，或序列化与读取结果不一致），请重新执行: %w", err)
	case rerr.HasDegradeCode(degradeStrReplaceMultiple):
		return fmt.Errorf("服务端认为匹配不唯一（文档可能在读取后被修改），请重新执行: %w", err)
	}
	return err
}

// docsAIBodyStart 返回正文起点，跳过文档标题：
//   - markdown 序列化第一行是标题（"# 标题" 或 <title>…</title>）；
//   - xml 序列化以 <title>…</title> 开头（整篇通常只有一行）。
func docsAIBodyStart(doc, format string) int {
	trimmed := strings.TrimLeft(doc, " \t\r\n")
	lead := len(doc) - len(trimmed)
	if strings.HasPrefix(trimmed, "<title>") || strings.HasPrefix(trimmed, "<title ") {
		if end := strings.Index(trimmed, "</title>"); end >= 0 {
			return lead + end + len("</title>")
		}
	}
	if format == "xml" {
		return 0
	}
	return docsAITitleLineEnd(doc)
}

// docsAITitleLineEnd 返回 markdown 序列化的正文起点：第一行若是标题（"# 标题" 或 <title>…</title>），跳过该行。
func docsAITitleLineEnd(doc string) int {
	first := doc
	nl := strings.IndexByte(doc, '\n')
	if nl >= 0 {
		first = doc[:nl]
	}
	t := strings.TrimSpace(first)
	if strings.HasPrefix(t, "# ") || strings.HasPrefix(t, "<title>") {
		if nl < 0 {
			return len(doc)
		}
		return nl + 1
	}
	return 0
}

// isPlainInlineText 判断文本不含 Markdown/XML 语法字符（可在两种序列化下逐字等价）。
func isPlainInlineText(s string) bool {
	return !strings.ContainsAny(s, "\\`*_~[]<>$|\n")
}

// xmlEscapeText 按 docs_ai XML 文本节点规则转义（& < >）。
func xmlEscapeText(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// findTextOccurrences 返回 pattern 在 doc[from:] 中所有不重叠命中的起始字节下标。
func findTextOccurrences(doc, pattern string, from int) []int {
	var out []int
	for i := from; i <= len(doc)-len(pattern); {
		j := strings.Index(doc[i:], pattern)
		if j < 0 {
			break
		}
		out = append(out, i+j)
		i += j + len(pattern)
	}
	return out
}

// countOverlapping 统计 w 在 doc 中的命中次数（允许重叠，保守判断唯一性）。
func countOverlapping(doc, w string) int {
	if w == "" {
		return 0
	}
	n := 0
	for i := 0; ; {
		j := strings.Index(doc[i:], w)
		if j < 0 {
			return n
		}
		n++
		i += j + 1
		if i >= len(doc) {
			return n
		}
	}
}

// allowedContextRune 判断字符能否作为替换窗口的上下文：只允许不会改变 Markdown/XML 结构的字符。
func allowedContextRune(r rune) bool {
	if r == ' ' || unicode.IsLetter(r) || unicode.IsDigit(r) {
		return true
	}
	if r > unicode.MaxASCII && unicode.IsPunct(r) {
		return true // 中文标点等
	}
	return strings.ContainsRune(",.;?", r)
}

// buildTextReplaceWindows 为每处命中构造全文唯一的替换窗口；无法构造的命中下标进入 unresolved。
func buildTextReplaceWindows(doc, pattern, replacement string, occ []int, bodyStart int) ([]textWindow, []int) {
	var windows []textWindow
	var unresolved []int
	for k, s := range occ {
		e := s + len(pattern)
		lineStart := strings.LastIndexByte(doc[:s], '\n') + 1
		lineEnd := len(doc)
		if nl := strings.IndexByte(doc[e:], '\n'); nl >= 0 {
			lineEnd = e + nl
		}
		// 左边界：不越过上一处命中、不触及行首第一个非空白字符（避免把 "- "/"1. "/"# " 等块标记带进内容）
		firstNonSpace := lineStart
		for firstNonSpace < s && (doc[firstNonSpace] == ' ' || doc[firstNonSpace] == '\t') {
			firstNonSpace++
		}
		leftMin := firstNonSpace + 1
		if k > 0 && occ[k-1]+len(pattern) > leftMin {
			leftMin = occ[k-1] + len(pattern)
		}
		if leftMin < bodyStart {
			leftMin = bodyStart
		}
		if leftMin > s {
			leftMin = s
		}
		rightMax := lineEnd
		if k+1 < len(occ) && occ[k+1] < rightMax {
			rightMax = occ[k+1]
		}

		l, r := s, e
		unique := func() bool { return countOverlapping(doc, doc[l:r]) == 1 }
		ok := unique()
		for !ok {
			grew := false
			if r < rightMax {
				rr, size := utf8.DecodeRuneInString(doc[r:])
				if allowedContextRune(rr) {
					r += size
					grew = true
				} else {
					rightMax = r
				}
			}
			if ok = unique(); ok {
				break
			}
			if l > leftMin {
				rl, size := utf8.DecodeLastRuneInString(doc[:l])
				if allowedContextRune(rl) {
					l -= size
					grew = true
				} else {
					leftMin = l
				}
			}
			if ok = unique(); ok {
				break
			}
			if !grew {
				break
			}
		}
		if !ok {
			unresolved = append(unresolved, s)
			continue
		}
		// 上下文两端不能是空白：Markdown 解析会吞掉首尾空格，导致相邻文字粘连
		for r > e {
			rr, size := utf8.DecodeLastRuneInString(doc[:r])
			if rr != ' ' {
				break
			}
			if r < rightMax {
				nr, nsize := utf8.DecodeRuneInString(doc[r:])
				if nr != ' ' && allowedContextRune(nr) {
					r += nsize
					break
				}
			}
			r -= size
		}
		for l < s {
			rl, size := utf8.DecodeRuneInString(doc[l:])
			if rl != ' ' {
				break
			}
			if l > leftMin {
				pr, psize := utf8.DecodeLastRuneInString(doc[:l])
				if pr != ' ' && allowedContextRune(pr) {
					l -= psize
					break
				}
			}
			l += size
		}
		if !unique() {
			unresolved = append(unresolved, s)
			continue
		}
		windows = append(windows, textWindow{
			start:   l,
			end:     r,
			pattern: doc[l:r],
			content: doc[l:s] + replacement + doc[e:r],
		})
	}
	return windows, unresolved
}

// excerptAround 截取命中附近的一小段文本用于报错展示。
func excerptAround(doc string, at, n int) string {
	start := at - 30
	if start < 0 {
		start = 0
	}
	end := at + n + 30
	if end > len(doc) {
		end = len(doc)
	}
	for start > 0 && !utf8.RuneStart(doc[start]) {
		start--
	}
	for end < len(doc) && !utf8.RuneStart(doc[end]) {
		end++
	}
	return strings.ReplaceAll(doc[start:end], "\n", "⏎")
}
