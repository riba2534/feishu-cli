package cmd

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

// 本文件把本项目 `doc export`（本地 Block→Markdown 转换器）产出的"本地方言"
// 转换为 docs_ai（PUT /open-apis/docs_ai/v1/documents/{id}）能正确理解的写法，
// 供 `doc content-update` 在发送前调用。
//
// 背景（实测，测试文档 overwrite 写回本地导出 Markdown）：
//   - `> [!NOTE]` 被 docs_ai 当普通引用，高亮块退化为 blockquote 且正文多出字面 "[!NOTE]"；
//   - `<image token=.../>` 不是 docs_ai 标签，图片被整块丢弃（degrade_code=5004）；
//   - `<whiteboard token=X type="blank"/>` 触发画板克隆，跨文档/部分画板克隆失败（degrade_code=2105）画板直接消失，
//     去掉 token 则按 type=blank 新建空白画板——两种结果都会丢失原画板内容；
//   - `<span style="color: #hex">` 颜色被静默丢弃；`<mention-user>`/`<mention-doc>` 不被识别。
//
// 能等价转换的结构就地改写；无法无损还原的结构（画板占位、多维表格、未下载的视频占位、
// 展开过的电子表格、导出时就未能表达的块）fail-closed 拒绝执行，绝不静默破坏原文档。

// dialectConversion 记录一次转换的统计，便于在 stderr 告知用户发生了哪些改写。
type dialectConversion struct {
	counts map[string]int
}

func (d *dialectConversion) add(kind string) {
	if d.counts == nil {
		d.counts = map[string]int{}
	}
	d.counts[kind]++
}

// summary 返回形如 "callout×2, image×1" 的统计；无改写时返回空串。
func (d dialectConversion) summary() string {
	if len(d.counts) == 0 {
		return ""
	}
	keys := make([]string, 0, len(d.counts))
	for k := range d.counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s×%d", k, d.counts[k]))
	}
	return strings.Join(parts, ", ")
}

// dialectIssue 是一处无法无损转换的本地方言。
type dialectIssue struct {
	line   int
	text   string
	reason string
}

// calloutDocsAIColors 把本地 callout 类型映射为 docs_ai 的 background-color / border-color，
// 与本地导入（markdown_to_block）的配色保持一致：WARNING 红、CAUTION 橙、TIP 黄、SUCCESS 绿、NOTE/INFO 蓝、IMPORTANT 紫。
var calloutDocsAIColors = map[string]string{
	"WARNING":   "red",
	"CAUTION":   "orange",
	"TIP":       "yellow",
	"SUCCESS":   "green",
	"NOTE":      "blue",
	"INFO":      "blue",
	"IMPORTANT": "purple",
}

// calloutColorByIndex 对应本地 <callout color="N">（飞书 Callout.BackgroundColor 2-7）。
var calloutColorByIndex = map[int]string{2: "red", 3: "orange", 4: "yellow", 5: "green", 6: "blue", 7: "purple"}

// textColorByHex / bgColorByHex 是本地导出 wrapHighlightSpan 所用 CSS 颜色（converter.fontColorMap /
// fontBgColorMap）到 docs_ai 颜色名的反向映射。
var textColorByHex = map[string]string{
	"#ef4444": "red", "#f97316": "orange", "#eab308": "yellow", "#22c55e": "green",
	"#3b82f6": "blue", "#a855f7": "purple", "#6b7280": "gray",
}

var bgColorByHex = map[string]string{
	"#fef2f2": "light-red", "#fff7ed": "light-orange", "#fefce8": "light-yellow", "#f0fdf4": "light-green",
	"#eff6ff": "light-blue", "#faf5ff": "light-purple", "#f9fafb": "light-gray",
	"#fecaca": "red", "#fed7aa": "orange", "#fef08a": "yellow", "#bbf7d0": "green",
	"#bfdbfe": "blue", "#e9d5ff": "purple", "#e5e7eb": "medium-gray",
}

var (
	ghCalloutStartRe = regexp.MustCompile(`^( {0,3})>[ \t]?\[!([A-Za-z]+)\][ \t]*(.*)$`)
	quoteLineRe      = regexp.MustCompile(`^( {0,3})>[ \t]?(.*)$`)
	gridOpenLineRe   = regexp.MustCompile(`^<grid\s+cols="(\d+)"\s*>$`)
	gridOpenInlineRe = regexp.MustCompile(`<grid\s+cols="(\d+)"\s*>`)
	htmlTagRe        = regexp.MustCompile(`<(/?)([a-zA-Z][a-zA-Z0-9-]*)((?:\s+[a-zA-Z_:][-a-zA-Z0-9_:.]*(?:\s*=\s*(?:"[^"]*"|'[^']*'))?)*)\s*(/?)>`)
	htmlAttrRe       = regexp.MustCompile(`([a-zA-Z_:][-a-zA-Z0-9_:.]*)\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	mentionDocRe     = regexp.MustCompile(`<mention-doc\b([^>]*)>(.*?)</mention-doc>`)
	unsupportedCmtRe = regexp.MustCompile(`<!--\s*(不支持的块类型|AI 模板块|递归深度超限|Grid 递归深度超限|GridColumn 递归深度超限)`)
	expandedSheetRe  = regexp.MustCompile(`^<!--\s*sheet\s+token="`)
	viewCommentRe    = regexp.MustCompile(`^<!--\s*不支持的块类型:\s*View\s*\(type=33\)\s*-->$`)
)

// convertLocalDialectForDocsAI 转换本地方言；遇到无法无损转换的结构返回 fail-closed 错误（exit 2）。
func convertLocalDialectForDocsAI(markdown string) (string, dialectConversion, error) {
	var conv dialectConversion
	var issues []dialectIssue
	lines := strings.Split(markdown, "\n")
	out := convertDialectLines(lines, 1, &conv, &issues)
	if len(issues) > 0 {
		return "", conv, dialectIssuesError(issues)
	}
	return strings.Join(out, "\n"), conv, nil
}

func dialectIssuesError(issues []dialectIssue) error {
	var b strings.Builder
	b.WriteString("检测到 doc export 本地方言中无法由 docs_ai 无损写回的结构，写回会丢失原内容，已拒绝执行：")
	const maxShow = 8
	for i, is := range issues {
		if i >= maxShow {
			fmt.Fprintf(&b, "\n  … 另有 %d 处", len(issues)-maxShow)
			break
		}
		text := is.text
		if r := []rune(text); len(r) > 80 {
			text = string(r[:80]) + "…"
		}
		fmt.Fprintf(&b, "\n  - 第 %d 行 %s：%s", is.line, text, is.reason)
	}
	b.WriteString("\n处理建议：改用 --mode replace_range / insert_* 只改写不含这些结构的章节（可配合 --block-id 精确定位，" +
		"block id 用 `feishu-cli doc read <doc> --with-ids` 获取）；或删除对应行表示明确放弃该内容；" +
		"画板可改写为 ```mermaid 代码块或 <whiteboard type=\"mermaid\">…</whiteboard> 重新生成。")
	return clierr.Usagef("%s", b.String())
}

// convertDialectLines 逐行转换；lineBase 为 lines[0] 在原文中的行号（用于报错定位）。
func convertDialectLines(lines []string, lineBase int, conv *dialectConversion, issues *[]dialectIssue) []string {
	out := make([]string, 0, len(lines))
	var fenceChar byte
	fenceLen := 0
	var gridCols []int // 嵌套 grid 的列数栈

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		lineNo := lineBase + i

		// 围栏代码块内原样保留
		if ch, n, ok := dialectFence(line); ok {
			if fenceChar == 0 {
				fenceChar, fenceLen = ch, n
				out = append(out, line)
				continue
			}
			if ch == fenceChar && n >= fenceLen && strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), string(ch))) == "" {
				fenceChar, fenceLen = 0, 0
			}
			out = append(out, line)
			continue
		}
		if fenceChar != 0 {
			out = append(out, line)
			continue
		}

		// GitHub 风格 callout：> [!TYPE] + 后续 > 行
		if m := ghCalloutStartRe.FindStringSubmatch(line); m != nil {
			typ := strings.ToUpper(m[2])
			inner := []string{}
			if strings.TrimSpace(m[3]) != "" {
				inner = append(inner, m[3])
			}
			j := i + 1
			for ; j < len(lines); j++ {
				qm := quoteLineRe.FindStringSubmatch(lines[j])
				if qm == nil {
					break
				}
				inner = append(inner, qm[2])
			}
			if typ == "WARNING" && strings.Contains(strings.Join(inner, "\n"), "同步块内容未展开") {
				*issues = append(*issues, dialectIssue{line: lineNo, text: strings.TrimSpace(line),
					reason: "同步块在导出时未能展开（只有警告占位），写回会用一段警告文字替换原同步块"})
				i = j - 1
				continue
			}
			color := calloutDocsAIColors[typ]
			if color == "" {
				color = "blue" // 与本地导入一致：未知类型按 NOTE 蓝色处理
			}
			if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
				out = append(out, "")
			}
			out = append(out, fmt.Sprintf(`<callout background-color="light-%s" border-color="%s">`, color, color), "")
			out = append(out, convertDialectLines(inner, lineNo+1, conv, issues)...)
			out = append(out, "", "</callout>")
			conv.add("callout")
			i = j - 1
			continue
		}

		trimmed := strings.TrimSpace(line)

		// 本地导出的 grid 多行形态：<grid cols="N"> / <column> / </column> / </grid>
		if m := gridOpenLineRe.FindStringSubmatch(trimmed); m != nil {
			n, _ := strconv.Atoi(m[1])
			gridCols = append(gridCols, n)
			out = append(out, "<grid>")
			conv.add("grid")
			continue
		}
		if len(gridCols) > 0 {
			switch trimmed {
			case "<column>":
				out = append(out, fmt.Sprintf(`<column width-ratio="%s">`, gridRatio(gridCols[len(gridCols)-1])), "")
				continue
			case "</column>":
				out = append(out, "", "</column>")
				continue
			case "</grid>":
				gridCols = gridCols[:len(gridCols)-1]
				out = append(out, "</grid>")
				continue
			}
		}

		// 整行级 fail-closed 标记
		if expandedSheetRe.MatchString(trimmed) {
			*issues = append(*issues, dialectIssue{line: lineNo, text: trimmed,
				reason: "这是导出时展开的内嵌电子表格，写回会把电子表格替换成普通表格"})
			out = append(out, line)
			continue
		}
		if viewCommentRe.MatchString(trimmed) {
			// 旧版导出把附件外层的视图块写成"不支持"注释，附件本身已单独导出：丢弃注释即可
			conv.add("view-comment")
			continue
		}
		if unsupportedCmtRe.MatchString(trimmed) {
			*issues = append(*issues, dialectIssue{line: lineNo, text: trimmed,
				reason: "导出时未能表达的块（只有注释占位），写回会删除原块"})
			out = append(out, line)
			continue
		}

		out = append(out, mapOutsideInlineCode(line, func(seg string) string {
			return convertDialectInline(seg, lineNo, conv, issues)
		}))
	}
	return out
}

// gridRatio 返回 N 列均分时的 width-ratio（docs_ai 要求各列之和为 1）。
func gridRatio(n int) string {
	if n <= 0 {
		n = 2
	}
	return strconv.FormatFloat(1/float64(n), 'f', 4, 64)
}

// dialectFence 判断是否为围栏代码块行（``` 或 ~~~，长度 ≥ 3，至多 3 空格缩进）。
func dialectFence(line string) (byte, int, bool) {
	indent := len(line) - len(strings.TrimLeft(line, " "))
	if indent > 3 {
		return 0, 0, false
	}
	s := line[indent:]
	if len(s) < 3 || (s[0] != '`' && s[0] != '~') {
		return 0, 0, false
	}
	ch := s[0]
	n := 0
	for n < len(s) && s[n] == ch {
		n++
	}
	if n < 3 {
		return 0, 0, false
	}
	if ch == '`' && strings.Contains(s[n:], "`") {
		return 0, 0, false // 反引号围栏的信息串不能含反引号（那是行内代码）
	}
	return ch, n, true
}

// mapOutsideInlineCode 只对行内代码（反引号串）以外的片段应用 fn。
func mapOutsideInlineCode(line string, fn func(string) string) string {
	if !strings.Contains(line, "`") {
		return fn(line)
	}
	var b strings.Builder
	start := 0 // 当前非代码片段起点
	i := 0
	for i < len(line) {
		if line[i] != '`' {
			i++
			continue
		}
		run := 0
		for i+run < len(line) && line[i+run] == '`' {
			run++
		}
		// 找同长度的闭合反引号串
		closeAt := -1
		for k := i + run; k < len(line); {
			if line[k] != '`' {
				k++
				continue
			}
			r := 0
			for k+r < len(line) && line[k+r] == '`' {
				r++
			}
			if r == run {
				closeAt = k
				break
			}
			k += r
		}
		if closeAt < 0 {
			i += run // 无闭合：按字面文本处理
			continue
		}
		b.WriteString(fn(line[start:i]))
		b.WriteString(line[i : closeAt+run])
		i = closeAt + run
		start = i
	}
	b.WriteString(fn(line[start:]))
	return b.String()
}

// convertDialectInline 转换行内（非代码）片段中的本地 HTML 扩展标签。
func convertDialectInline(seg string, lineNo int, conv *dialectConversion, issues *[]dialectIssue) string {
	if !strings.Contains(seg, "<") {
		return seg
	}
	// <mention-doc token=".." type="..">标题</mention-doc> → <cite type="doc" doc-id=".."/>
	seg = mentionDocRe.ReplaceAllStringFunc(seg, func(raw string) string {
		m := mentionDocRe.FindStringSubmatch(raw)
		attrs := parseDialectAttrs(m[1])
		if attrs["token"] == "" {
			return raw
		}
		conv.add("mention-doc")
		return fmt.Sprintf(`<cite type="doc" doc-id="%s"/>`, escapeDialectAttr(attrs["token"]))
	})

	// 单行 grid：<grid cols="N"><column>..</column>..</grid>
	if gridOpenInlineRe.MatchString(seg) {
		seg = convertInlineGrid(seg, conv)
	}

	return htmlTagRe.ReplaceAllStringFunc(seg, func(raw string) string {
		m := htmlTagRe.FindStringSubmatch(raw)
		closing, name, attrText := m[1] == "/", strings.ToLower(m[2]), m[3]
		if closing {
			return raw
		}
		attrs := parseDialectAttrs(attrText)
		switch name {
		case "image":
			token := attrs["token"]
			if token == "" {
				return raw
			}
			conv.add("image")
			out := fmt.Sprintf(`<img src="%s"`, escapeDialectAttr(token))
			for _, k := range []string{"width", "height", "caption"} {
				if v := attrs[k]; v != "" {
					out += fmt.Sprintf(` %s="%s"`, k, escapeDialectAttr(v))
				}
			}
			return out + "/>"
		case "file":
			token := attrs["token"]
			if token == "" {
				return raw
			}
			conv.add("file")
			out := fmt.Sprintf(`<source token="%s"`, escapeDialectAttr(token))
			if v := attrs["name"]; v != "" {
				out += fmt.Sprintf(` name="%s"`, escapeDialectAttr(v))
			}
			return out + "/>"
		case "mention-user":
			id := attrs["id"]
			if id == "" {
				return raw
			}
			conv.add("mention-user")
			return fmt.Sprintf(`<cite type="user" user-id="%s"/>`, escapeDialectAttr(id))
		case "span":
			style, ok := attrs["style"]
			if !ok {
				return raw
			}
			textColor, bgColor, known := parseDialectSpanStyle(style)
			if !known {
				return raw // 非本地导出产生的 style（用户自写），原样交给服务端
			}
			conv.add("span")
			out := "<span"
			if textColor != "" {
				out += fmt.Sprintf(` text-color="%s"`, textColor)
			}
			if bgColor != "" {
				out += fmt.Sprintf(` background-color="%s"`, bgColor)
			}
			return out + ">"
		case "callout":
			// 本地导入语法 <callout type="NOTE" color="6">：docs_ai 不认 type/color（degrade 5002，颜色丢失）
			_, hasType := attrs["type"]
			_, hasColor := attrs["color"]
			if !hasType && !hasColor {
				return raw
			}
			color := ""
			if v, err := strconv.Atoi(attrs["color"]); err == nil {
				color = calloutColorByIndex[v]
			}
			if color == "" {
				color = calloutDocsAIColors[strings.ToUpper(attrs["type"])]
			}
			if color == "" {
				color = "blue"
			}
			conv.add("callout")
			out := fmt.Sprintf(`<callout background-color="light-%s" border-color="%s"`, color, color)
			if v := attrs["emoji"]; v != "" {
				out += fmt.Sprintf(` emoji="%s"`, escapeDialectAttr(v))
			}
			if m[4] == "/" {
				return out + "/>"
			}
			return out + ">"
		case "whiteboard":
			_, hasToken := attrs["token"]
			if hasToken && strings.EqualFold(attrs["type"], "blank") {
				*issues = append(*issues, dialectIssue{line: lineNo, text: raw,
					reason: "本地导出的画板占位（token + type=\"blank\"）：docs_ai 会新建空白画板，或按 token 克隆（跨文档等场景实测 degrade_code=2105 失败），原画板内容无法还原"})
			}
			return raw
		case "bitable":
			*issues = append(*issues, dialectIssue{line: lineNo, text: raw,
				reason: "内嵌多维表格无法通过 Markdown 重新引用或复制"})
			return raw
		case "sheet":
			if _, hasToken := attrs["token"]; hasToken {
				*issues = append(*issues, dialectIssue{line: lineNo, text: raw,
					reason: "本地导出的内嵌电子表格占位：写回只会新建空白表格或复制出一份新表格，原表格引用无法保留"})
			}
			return raw
		case "video":
			if strings.HasPrefix(attrs["src"], "feishu://") {
				*issues = append(*issues, dialectIssue{line: lineNo, text: raw,
					reason: "未下载的视频占位（feishu:// 引用），写回会丢失视频"})
			}
			return raw
		}
		return raw
	})
}

// convertInlineGrid 处理单行 grid：<grid cols="N"> → <grid>，其后的 <column> 补 width-ratio。
func convertInlineGrid(seg string, conv *dialectConversion) string {
	var b strings.Builder
	rest := seg
	for {
		loc := gridOpenInlineRe.FindStringSubmatchIndex(rest)
		if loc == nil {
			b.WriteString(rest)
			break
		}
		n, _ := strconv.Atoi(rest[loc[2]:loc[3]])
		b.WriteString(rest[:loc[0]])
		b.WriteString("<grid>")
		conv.add("grid")
		rest = rest[loc[1]:]
		end := strings.Index(rest, "</grid>")
		body := rest
		if end >= 0 {
			body = rest[:end]
		}
		body = strings.ReplaceAll(body, "<column>", fmt.Sprintf(`<column width-ratio="%s">`, gridRatio(n)))
		b.WriteString(body)
		if end < 0 {
			break
		}
		rest = rest[end:]
	}
	return b.String()
}

// parseDialectSpanStyle 解析本地导出的 style="color: #x; background-color: #y"。
// 只有所有声明都能映射到 docs_ai 颜色名时 known=true。
func parseDialectSpanStyle(style string) (textColor, bgColor string, known bool) {
	decls := strings.Split(style, ";")
	seen := false
	for _, d := range decls {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		kv := strings.SplitN(d, ":", 2)
		if len(kv) != 2 {
			return "", "", false
		}
		key := strings.ToLower(strings.TrimSpace(kv[0]))
		val := strings.ToLower(strings.TrimSpace(kv[1]))
		switch key {
		case "color":
			c, ok := textColorByHex[val]
			if !ok {
				return "", "", false
			}
			textColor = c
		case "background-color":
			c, ok := bgColorByHex[val]
			if !ok {
				return "", "", false
			}
			bgColor = c
		default:
			return "", "", false
		}
		seen = true
	}
	return textColor, bgColor, seen
}

func parseDialectAttrs(s string) map[string]string {
	attrs := map[string]string{}
	for _, m := range htmlAttrRe.FindAllStringSubmatch(s, -1) {
		v := m[2]
		if v == "" {
			v = m[3]
		}
		attrs[strings.ToLower(m[1])] = v
	}
	return attrs
}

func escapeDialectAttr(s string) string {
	return strings.NewReplacer(`&`, "&amp;", `"`, "&quot;", `<`, "&lt;", `>`, "&gt;").Replace(s)
}
