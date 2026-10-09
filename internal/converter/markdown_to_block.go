package converter

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// isMarkdownEscapable 判断字节是否为 CommonMark §2.4 可转义的 ASCII 标点。
// 范围：!"#$%&'()*+,-./:;<=>?@[\]^_`{|}~
func isMarkdownEscapable(b byte) bool {
	return (b >= '!' && b <= '/') || (b >= ':' && b <= '@') ||
		(b >= '[' && b <= '`') || (b >= '{' && b <= '~')
}

// unescapeMarkdownText 去除 CommonMark 反斜杠转义。
// goldmark 的 Segment.Value 返回源文件原始字节，不处理转义序列。
// 例如 "1\." → "1."、"\[1\]" → "[1]"、"prompt\_len" → "prompt_len"。
func unescapeMarkdownText(s string) string {
	if strings.IndexByte(s, '\\') < 0 {
		return s
	}
	var buf strings.Builder
	buf.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && isMarkdownEscapable(s[i+1]) {
			buf.WriteByte(s[i+1])
			i++
			continue
		}
		buf.WriteByte(s[i])
	}
	return buf.String()
}

// unescapeMarkdownBytes 是 unescapeMarkdownText 的 []byte 版本，用于 buf.Write 场景。
func unescapeMarkdownBytes(b []byte) []byte {
	if bytes.IndexByte(b, '\\') < 0 {
		return b
	}
	buf := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		if b[i] == '\\' && i+1 < len(b) && isMarkdownEscapable(b[i+1]) {
			buf = append(buf, b[i+1])
			i++
			continue
		}
		buf = append(buf, b[i])
	}
	return buf
}

// 飞书 API 限制单个表格最多 9 行（包括表头）、9 列
const maxTableRows = 9
const maxTableCols = 9

// 表格列宽配置（单位：像素）
const (
	minColumnWidth   = 80  // 最小列宽
	maxColumnWidth   = 400 // 最大列宽
	defaultDocWidth  = 700 // 飞书文档默认可用宽度
	charWidthChinese = 14  // 中文字符宽度
	charWidthEnglish = 8   // 英文/数字字符宽度
	columnPadding    = 16  // 列内边距
)

// calculateColumnWidths 根据单元格内容计算每列的宽度
func calculateColumnWidths(headerContents []string, dataRows [][]string, cols int) []int {
	if cols == 0 {
		return nil
	}

	// 计算每列的最大内容宽度
	maxWidths := make([]int, cols)

	// 计算单个字符串的显示宽度
	calcTextWidth := func(s string) int {
		width := 0
		for _, r := range s {
			if r > 127 { // 非 ASCII 字符（中文等）
				width += charWidthChinese
			} else {
				width += charWidthEnglish
			}
		}
		return width + columnPadding
	}

	// 处理表头
	for i, content := range headerContents {
		if i < cols {
			w := calcTextWidth(content)
			if w > maxWidths[i] {
				maxWidths[i] = w
			}
		}
	}

	// 处理数据行
	for _, row := range dataRows {
		for i, content := range row {
			if i < cols {
				w := calcTextWidth(content)
				if w > maxWidths[i] {
					maxWidths[i] = w
				}
			}
		}
	}

	// 应用最小/最大限制
	totalWidth := 0
	for i := range maxWidths {
		if maxWidths[i] < minColumnWidth {
			maxWidths[i] = minColumnWidth
		}
		if maxWidths[i] > maxColumnWidth {
			maxWidths[i] = maxColumnWidth
		}
		totalWidth += maxWidths[i]
	}

	// 如果总宽度小于文档宽度，按比例扩展
	if totalWidth < defaultDocWidth && cols > 0 {
		extra := (defaultDocWidth - totalWidth) / cols
		for i := range maxWidths {
			maxWidths[i] += extra
			if maxWidths[i] > maxColumnWidth {
				maxWidths[i] = maxColumnWidth
			}
		}
	}

	return maxWidths
}

// colWidthCommentRe 匹配独占一行的 `<!-- feishu-colwidth: 80, 200, *, 30% -->` 注释。
// 单元支持：纯整数（像素，含负数 → parseColWidthList 容错为 0/auto）、`N%`（按
// defaultDocWidth 换算）、`*` 或空（该列走 auto）。
//
// 捕获组用 `[^>]*` 而非 `[^-]+`：后者会让含 `-` 的 payload（包括用户笔误的负数）
// 整体不匹配从而静默丢失，前者把所有非 `>` 字符交给 parseColWidthList 容错处理，
// 配合 alignAndClampColumnWidths 的 clamp 仍能给出有意义结果，并由 warnColumnWidthMismatch
// 在长度不匹配时提示。
var colWidthCommentRe = regexp.MustCompile(`(?s)^\s*<!--\s*feishu-colwidth\s*:\s*([^>]*?)-->\s*$`)

// parseColWidthList 解析逗号分隔的列宽片段。
// 单元含义：
//   - 整数 → 像素值
//   - "N%" → defaultDocWidth * N / 100
//   - "*" 或空 → 0，调用方应将该列回退到 auto 计算
//
// 单位识别失败的片段也返回 0（容错降级到 auto）。
func parseColWidthList(s string) []int {
	parts := strings.Split(s, ",")
	out := make([]int, 0, len(parts))
	for _, raw := range parts {
		p := strings.TrimSpace(raw)
		if p == "" || p == "*" {
			out = append(out, 0)
			continue
		}
		if rest, ok := strings.CutSuffix(p, "%"); ok {
			if pct, err := strconv.Atoi(strings.TrimSpace(rest)); err == nil {
				out = append(out, pct*defaultDocWidth/100)
				continue
			}
			out = append(out, 0)
			continue
		}
		if v, err := strconv.Atoi(p); err == nil {
			out = append(out, v)
			continue
		}
		out = append(out, 0)
	}
	return out
}

// resolveColumnWidths 综合 pendingColWidth、ConvertOptions、auto 启发式得到最终列宽数组。
// 优先级：单表注释 > options.ColumnWidthMode == explicit > options.ColumnWidthMode == fixed > auto
//
// 所有路径最终都过 [minColumnWidth, maxColumnWidth] clamp，长度严格等于 cols。
// 注释一旦消费立即清空 pendingColWidth，避免泄漏到下一张表。
func (c *MarkdownToBlock) resolveColumnWidths(headerContents []string, dataRows [][]string, cols int) []int {
	if cols <= 0 {
		return nil
	}

	// 优先级 1：单表注释
	if len(c.pendingColWidth) > 0 {
		raw := c.pendingColWidth
		c.pendingColWidth = nil // 消费即清空
		c.warnColumnWidthMismatch("注释", raw, cols)
		return alignAndClampColumnWidths(raw, headerContents, dataRows, cols)
	}

	// 优先级 2：CLI flag explicit
	if c.options.ColumnWidthMode == "explicit" && len(c.options.ColumnWidthValues) > 0 {
		c.warnColumnWidthMismatch("--table-column-width", c.options.ColumnWidthValues, cols)
		return alignAndClampColumnWidths(c.options.ColumnWidthValues, headerContents, dataRows, cols)
	}

	// 优先级 3：CLI flag fixed
	if c.options.ColumnWidthMode == "fixed" {
		per := defaultDocWidth / cols
		widths := make([]int, cols)
		for i := range widths {
			widths[i] = per
		}
		return clampColumnWidths(widths)
	}

	// 默认 auto：保留启发式
	return calculateColumnWidths(headerContents, dataRows, cols)
}

// warnColumnWidthMismatch 当用户给出的列宽数量与表实际列数不一致时，
// 向 stderr 打印一行提示。默认开启（这是用户显式输入的列宽，应当被告知是否生效）。
// 不阻塞流程：长度不足走 auto、超出截断（见 alignAndClampColumnWidths）。
func (c *MarkdownToBlock) warnColumnWidthMismatch(source string, values []int, cols int) {
	if len(values) == cols {
		return
	}
	if len(values) > cols {
		fmt.Fprintf(os.Stderr,
			"[警告] %s 提供了 %d 个列宽，但表只有 %d 列：尾部 %d 项被截断\n",
			source, len(values), cols, len(values)-cols)
	} else {
		fmt.Fprintf(os.Stderr,
			"[警告] %s 提供了 %d 个列宽，但表有 %d 列：缺失 %d 列将走 auto 计算\n",
			source, len(values), cols, cols-len(values))
	}
}

// alignAndClampColumnWidths 把用户指定值对齐到目标列数：
//   - 长度不足：缺位走 auto（按当前列内容的 calculateColumnWidths 取值）
//   - 长度超出：截断尾部
//   - 0 占位：该列走 auto
//   - 非零值：clamp 到 [minColumnWidth, maxColumnWidth]
func alignAndClampColumnWidths(values []int, headers []string, rows [][]string, cols int) []int {
	if cols <= 0 {
		return nil
	}
	autoWidths := calculateColumnWidths(headers, rows, cols)
	out := make([]int, cols)
	for i := 0; i < cols; i++ {
		if i < len(values) && values[i] > 0 {
			out[i] = values[i]
		} else if i < len(autoWidths) {
			out[i] = autoWidths[i]
		} else {
			out[i] = minColumnWidth
		}
		if out[i] < minColumnWidth {
			out[i] = minColumnWidth
		}
		if out[i] > maxColumnWidth {
			out[i] = maxColumnWidth
		}
	}
	return out
}

// clampColumnWidths 对所有列宽过 [minColumnWidth, maxColumnWidth] 范围。
func clampColumnWidths(widths []int) []int {
	for i := range widths {
		if widths[i] < minColumnWidth {
			widths[i] = minColumnWidth
		}
		if widths[i] > maxColumnWidth {
			widths[i] = maxColumnWidth
		}
	}
	return widths
}

// isTableOrColWidthHTMLBlock 判断节点是否是『可消费 pendingColWidth』的两类节点：
//   - *east.Table：紧邻其下的表格，会消费 pending
//   - *ast.HTMLBlock 且原文匹配 colWidthCommentRe：列宽注释本身，会写入 pending
//
// 其他任何块（Heading/Paragraph/CodeBlock/FencedCodeBlock/TextBlock/List/Blockquote/ThematicBreak/
// 普通 HTMLBlock 等）都不应保留 pending，必须在主 walk 入口清空，
// 否则悬浮注释会污染下游无关表格（review fix-1b 防回归）。
func (c *MarkdownToBlock) isTableOrColWidthHTMLBlock(n ast.Node) bool {
	if _, ok := n.(*east.Table); ok {
		return true
	}
	if h, ok := n.(*ast.HTMLBlock); ok {
		raw := c.getHTMLBlockText(h)
		if colWidthCommentRe.MatchString(raw) {
			return true
		}
	}
	return false
}

// MarkdownToBlock converts Markdown to Feishu blocks
type MarkdownToBlock struct {
	source       []byte
	options      ConvertOptions
	basePath     string // base path for resolving relative image paths
	imageStats   ImageStats
	imageSources []string // 记录每个 Image Block 对应的图片来源路径
	// cellImageSink 非 nil 时（仅 EmbedTableImages 下的表格单元格提取期间），
	// extractChildElements 把可嵌入图片源收集到此处并跳过占位文本，留待导入层在单元格内建 Image 子块。
	cellImageSink *[]string
	videoStats    VideoStats
	videoSources  []string // 记录每个视频 File Block 对应的视频来源路径
	fileStats     VideoStats
	// mediaRefs 记录建块后才能补齐内容的块（图片/附件/视频/带 token 画板），见 MediaRef。
	mediaRefs map[*larkdocx.Block]*MediaRef
	// degradations 记录转换期已降级为占位文本的内容（见 Degradation）。
	degradations []Degradation
	// tableDataByBlock 记录每个表格块（含分栏列等嵌套位置）的填充数据。
	tableDataByBlock map[*larkdocx.Block]*TableData

	// pendingColWidth 暂存最近一条 <!-- feishu-colwidth: ... --> 注释解析出的宽度数组，
	// 由紧邻其下的 ast.Table 消费一次后清空。0 表示该列走 auto。
	// 当列数与表实际列数不一致时，不足补 minColumnWidth、超出截断。
	pendingColWidth []int
}

// NewMarkdownToBlock creates a new converter
func NewMarkdownToBlock(source []byte, options ConvertOptions, basePath string) *MarkdownToBlock {
	return &MarkdownToBlock{
		source:   source,
		options:  options,
		basePath: basePath,
	}
}

// BlockNode represents a block that may contain nested child blocks.
// Used to support hierarchical structures like nested lists in Feishu.
type BlockNode struct {
	Block    *larkdocx.Block
	Children []*BlockNode
}

// FlattenBlockNodes flattens a tree of BlockNodes into a flat list of blocks (depth-first)
func FlattenBlockNodes(nodes []*BlockNode) []*larkdocx.Block {
	var result []*larkdocx.Block
	for _, n := range nodes {
		if n == nil || n.Block == nil {
			continue
		}
		result = append(result, n.Block)
		if len(n.Children) > 0 {
			result = append(result, FlattenBlockNodes(n.Children)...)
		}
	}
	return result
}

// normalizeBlockquoteEnding 确保引用块后有空行分隔
// goldmark 的 lazy continuation 会将引用块后紧跟的非引用行视为引用块的一部分，
// 通过在引用块最后一行和非引用行之间插入空行来终止引用块
func normalizeBlockquoteEnding(source []byte) []byte {
	lines := strings.Split(string(source), "\n")
	var result []string
	inFence := false
	fenceLen := 0

	for i, line := range lines {
		result = append(result, line)
		trimmed := strings.TrimSpace(line)

		// 代码围栏检测（跳过围栏内的内容）
		backticks := 0
		for _, ch := range trimmed {
			if ch == '`' {
				backticks++
			} else {
				break
			}
		}
		if !inFence && backticks >= 3 {
			inFence = true
			fenceLen = backticks
			continue
		}
		if inFence && backticks >= fenceLen {
			rest := strings.TrimLeft(trimmed, "`")
			if strings.TrimSpace(rest) == "" {
				inFence = false
				fenceLen = 0
			}
			continue
		}
		if inFence {
			continue
		}

		// 当前行是引用行，下一行非空且不是引用行 → 插入空行终止引用块
		if i+1 < len(lines) {
			nextTrimmed := strings.TrimSpace(lines[i+1])
			if strings.HasPrefix(trimmed, ">") &&
				nextTrimmed != "" &&
				!strings.HasPrefix(nextTrimmed, ">") {
				result = append(result, "")
			}
		}
	}
	return []byte(strings.Join(result, "\n"))
}

func createTextEquationBlock(formula string) *larkdocx.Block {
	blockType := int(BlockTypeText)
	return &larkdocx.Block{
		BlockType: &blockType,
		Text: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{Equation: &larkdocx.Equation{Content: &formula}},
			},
		},
	}
}

const blockEquationHTMLTag = "block-equation"

type rawHTMLBlockMode int

const (
	rawHTMLBlockNone rawHTMLBlockMode = iota
	rawHTMLBlockUntilBlank
	rawHTMLBlockUntilClosingTag
	rawHTMLBlockUntilCommentEnd
	rawHTMLBlockUntilProcessingEnd
	rawHTMLBlockUntilCDATAEnd
	rawHTMLBlockUntilDeclarationEnd
)

type rawHTMLBlockState struct {
	mode rawHTMLBlockMode
	tag  string
}

var commonMarkHTMLBlockTags = map[string]struct{}{
	"address": {}, "article": {}, "aside": {}, "base": {}, "basefont": {},
	"blockquote": {}, "body": {}, "caption": {}, "center": {}, "col": {},
	"colgroup": {}, "dd": {}, "details": {}, "dialog": {}, "dir": {},
	"div": {}, "dl": {}, "dt": {}, "fieldset": {}, "figcaption": {},
	"figure": {}, "footer": {}, "form": {}, "frame": {}, "frameset": {},
	"h1": {}, "h2": {}, "h3": {}, "h4": {}, "h5": {}, "h6": {},
	"head": {}, "header": {}, "hr": {}, "html": {}, "iframe": {},
	"legend": {}, "li": {}, "link": {}, "main": {}, "menu": {},
	"menuitem": {}, "nav": {}, "noframes": {}, "ol": {}, "optgroup": {},
	"option": {}, "p": {}, "param": {}, "section": {}, "source": {},
	"summary": {}, "table": {}, "tbody": {}, "td": {}, "tfoot": {},
	"th": {}, "thead": {}, "title": {}, "tr": {}, "track": {}, "ul": {},
}

func parseFenceMarker(line string) (byte, int, string) {
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	if indent > 3 {
		return 0, 0, ""
	}

	restLine := line[indent:]
	if len(restLine) == 0 {
		return 0, 0, ""
	}
	marker := restLine[0]
	if marker != '`' && marker != '~' {
		return 0, 0, ""
	}
	count := 0
	for count < len(restLine) && restLine[count] == marker {
		count++
	}
	if count < 3 {
		return 0, 0, ""
	}
	return marker, count, restLine[count:]
}

func commonMarkBlockStartText(line string) (string, bool) {
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	if indent > 3 {
		return "", false
	}
	return strings.TrimSpace(line[indent:]), true
}

func rawHTMLBlockStart(line string) (rawHTMLBlockState, bool) {
	trimmed, ok := commonMarkBlockStartText(line)
	if !ok || trimmed == "" || trimmed[0] != '<' {
		return rawHTMLBlockState{}, false
	}

	lower := strings.ToLower(trimmed)
	switch {
	case strings.HasPrefix(lower, "<!--"):
		return rawHTMLBlockState{mode: rawHTMLBlockUntilCommentEnd}, true
	case strings.HasPrefix(lower, "<?"):
		return rawHTMLBlockState{mode: rawHTMLBlockUntilProcessingEnd}, true
	case strings.HasPrefix(lower, "<![cdata["):
		return rawHTMLBlockState{mode: rawHTMLBlockUntilCDATAEnd}, true
	case len(lower) >= 3 && strings.HasPrefix(lower, "<!") && lower[2] >= 'a' && lower[2] <= 'z':
		return rawHTMLBlockState{mode: rawHTMLBlockUntilDeclarationEnd}, true
	}

	nameStart := 1
	if len(lower) > 1 && lower[1] == '/' {
		nameStart = 2
	}
	nameEnd := nameStart
	for nameEnd < len(lower) {
		ch := lower[nameEnd]
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			nameEnd++
			continue
		}
		break
	}
	if nameEnd == nameStart {
		return rawHTMLBlockState{}, false
	}

	tag := lower[nameStart:nameEnd]
	if _, ok := commonMarkHTMLBlockTags[tag]; ok {
		return rawHTMLBlockState{mode: rawHTMLBlockUntilBlank}, true
	}
	switch tag {
	case "pre", "script", "style", "textarea":
		return rawHTMLBlockState{mode: rawHTMLBlockUntilClosingTag, tag: tag}, true
	}
	return rawHTMLBlockState{}, false
}

func rawHTMLBlockClosed(line string, state rawHTMLBlockState) bool {
	lower := strings.ToLower(line)
	switch state.mode {
	case rawHTMLBlockUntilBlank:
		return strings.TrimSpace(line) == ""
	case rawHTMLBlockUntilClosingTag:
		return strings.Contains(lower, "</"+state.tag+">")
	case rawHTMLBlockUntilCommentEnd:
		return strings.Contains(lower, "-->")
	case rawHTMLBlockUntilProcessingEnd:
		return strings.Contains(lower, "?>")
	case rawHTMLBlockUntilCDATAEnd:
		return strings.Contains(lower, "]]>")
	case rawHTMLBlockUntilDeclarationEnd:
		return strings.Contains(lower, ">")
	default:
		return true
	}
}

func encodeBlockEquationHTML(formula string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(formula))
	return fmt.Sprintf("<%s data-base64=\"%s\"/>", blockEquationHTMLTag, encoded)
}

func decodeBlockEquationHTML(tag *HTMLTag) (string, bool) {
	if tag == nil || tag.Name != blockEquationHTMLTag {
		return "", false
	}
	if encoded := tag.Attrs["data-base64"]; encoded != "" {
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err == nil {
			return string(decoded), true
		}
	}
	if tag.Content != "" {
		return strings.TrimSpace(tag.Content), true
	}
	return "", false
}

func rewriteBlockEquationsToHTML(source []byte) ([]byte, bool) {
	lines := strings.Split(string(source), "\n")
	out := make([]string, 0, len(lines))
	foundEquation := false
	inFence := false
	var fenceMarker byte
	fenceLen := 0
	rawHTMLState := rawHTMLBlockState{}

	appendBlankIfNeeded := func() {
		if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
			out = append(out, "")
		}
	}

	for i := 0; i < len(lines); {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if inFence {
			out = append(out, line)
			if marker, count, rest := parseFenceMarker(line); marker == fenceMarker && count >= fenceLen && strings.TrimSpace(rest) == "" {
				inFence = false
				fenceMarker = 0
				fenceLen = 0
			}
			i++
			continue
		}

		if rawHTMLState.mode != rawHTMLBlockNone {
			out = append(out, line)
			if rawHTMLBlockClosed(line, rawHTMLState) {
				rawHTMLState = rawHTMLBlockState{}
			}
			i++
			continue
		}

		if trimmed == "$$" {
			i++
			var equationLines []string
			closed := false
			for i < len(lines) {
				if strings.TrimSpace(lines[i]) == "$$" {
					closed = true
					i++
					break
				}
				equationLines = append(equationLines, lines[i])
				i++
			}
			formula := strings.Join(equationLines, "\n")
			if closed && strings.TrimSpace(formula) != "" {
				foundEquation = true
				appendBlankIfNeeded()
				out = append(out, encodeBlockEquationHTML(formula))
				if i < len(lines) && strings.TrimSpace(lines[i]) != "" {
					out = append(out, "")
				}
			} else {
				out = append(out, "$$")
				out = append(out, equationLines...)
				if closed {
					out = append(out, "$$")
				}
			}
			continue
		}

		if marker, count, _ := parseFenceMarker(line); count >= 3 {
			inFence = true
			fenceMarker = marker
			fenceLen = count
		} else if state, ok := rawHTMLBlockStart(line); ok {
			rawHTMLState = state
			if rawHTMLBlockClosed(line, rawHTMLState) {
				rawHTMLState = rawHTMLBlockState{}
			}
		}
		out = append(out, line)
		i++
	}

	if !foundEquation {
		return source, false
	}
	return []byte(strings.Join(out, "\n")), true
}

// gridOpenRe 匹配独占一行（缩进 ≤3）的 <grid ...> 开标签（非自闭合）。
var gridOpenRe = regexp.MustCompile(`(?i)^ {0,3}<grid(\s[^>]*)?>\s*$`)

// gridCloseRe 匹配独占一行的 </grid>。
var gridCloseRe = regexp.MustCompile(`(?i)^\s*</grid\s*>\s*$`)

// rewriteMultilineGridBlocks 把跨多行（列内含空行、多个块）的 <grid>...</grid> 折叠成单行
// <grid ... data-base64="..."/>。
//
// CommonMark 的自定义 HTML 块（type 7）遇到空行即结束：doc export 输出的分栏（列内多段落/列表、
// 空列的 <column>\n\n</column>）会被切断，列内容泄漏到分栏之外。折叠后由 handleHTMLGridBlock 解码原始内容。
// 代码围栏内的 <grid> 不处理。
func rewriteMultilineGridBlocks(source []byte) []byte {
	if !bytes.Contains(bytes.ToLower(source), []byte("<grid")) {
		return source
	}
	lines := strings.Split(string(source), "\n")
	out := make([]string, 0, len(lines))
	inFence := false
	var fenceMarker byte
	fenceLen := 0
	changed := false
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if inFence {
			out = append(out, line)
			if marker, count, rest := parseFenceMarker(line); marker == fenceMarker && count >= fenceLen && strings.TrimSpace(rest) == "" {
				inFence = false
			}
			continue
		}
		if marker, count, _ := parseFenceMarker(line); count >= 3 {
			inFence, fenceMarker, fenceLen = true, marker, count
			out = append(out, line)
			continue
		}
		if !gridOpenRe.MatchString(line) {
			out = append(out, line)
			continue
		}
		// 查找配对的 </grid>（允许嵌套计数；找不到则原样保留，交给常规解析）
		depth, end := 1, -1
		for j := i + 1; j < len(lines); j++ {
			if gridOpenRe.MatchString(lines[j]) {
				depth++
			} else if gridCloseRe.MatchString(lines[j]) {
				depth--
				if depth == 0 {
					end = j
					break
				}
			}
		}
		if end < 0 {
			out = append(out, line)
			continue
		}
		inner := strings.Join(lines[i+1:end], "\n")
		indent := line[:len(line)-len(strings.TrimLeft(line, " "))] // 保留缩进，列表项内的分栏不跳出列表
		openTag := strings.TrimSuffix(strings.TrimSpace(line), ">")
		folded := fmt.Sprintf("%s%s data-base64=\"%s\"/>", indent, openTag, base64.StdEncoding.EncodeToString([]byte(inner)))
		if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
			out = append(out, "")
		}
		out = append(out, folded, "")
		i = end
		changed = true
	}
	if !changed {
		return source
	}
	return []byte(strings.Join(out, "\n"))
}

// ConvertWithTableData converts Markdown to Feishu blocks and returns table data for content filling
func (c *MarkdownToBlock) ConvertWithTableData() (*ConvertResult, error) {
	// 预处理：确保引用块后有空行分隔，避免 goldmark 的 lazy continuation
	c.source = normalizeBlockquoteEnding(c.source)

	// 预处理：跨多行（含空行）的 <grid> 折叠为单行，避免 HTML 块在空行处被截断
	c.source = rewriteMultilineGridBlocks(c.source)

	c.source, _ = rewriteBlockEquationsToHTML(c.source)

	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
	)

	reader := text.NewReader(c.source)
	doc := md.Parser().Parse(reader)

	result := &ConvertResult{}
	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		// pendingColWidth 守护：注释只对紧邻其下的 Table 生效。
		// 任何 Document 直接子节点（不是 Document 本身），如果不是 Table 也不是命中
		// colWidthCommentRe 的 HTMLBlock，都要清空 pending —— 防止悬浮注释跨越
		// Heading/Paragraph/CodeBlock(缩进)/TextBlock(link-ref-def)/List 等节点
		// 污染下游表格列宽。
		if n.Parent() == doc {
			if !c.isTableOrColWidthHTMLBlock(n) {
				c.pendingColWidth = nil
			}
		}

		switch node := n.(type) {
		case *ast.Heading:
			block, err := c.convertHeading(node)
			if err != nil {
				return ast.WalkStop, err
			}
			if block != nil {
				result.BlockNodes = append(result.BlockNodes, &BlockNode{Block: block})
			}
			return ast.WalkSkipChildren, nil

		case *ast.Paragraph:
			nodes, err := c.convertParagraph(node)
			if err != nil {
				return ast.WalkStop, err
			}
			result.BlockNodes = append(result.BlockNodes, nodes...)
			return ast.WalkSkipChildren, nil

		case *ast.FencedCodeBlock:
			block, err := c.convertCodeBlock(node)
			if err != nil {
				return ast.WalkStop, err
			}
			if block != nil {
				result.BlockNodes = append(result.BlockNodes, &BlockNode{Block: block})
			}
			return ast.WalkSkipChildren, nil

		case *ast.List:
			listNodes, err := c.convertList(node)
			if err != nil {
				return ast.WalkStop, err
			}
			result.BlockNodes = append(result.BlockNodes, listNodes...)
			return ast.WalkSkipChildren, nil

		case *ast.Blockquote:
			quoteNodes, err := c.convertBlockquote(node)
			if err != nil {
				return ast.WalkStop, err
			}
			result.BlockNodes = append(result.BlockNodes, quoteNodes...)
			return ast.WalkSkipChildren, nil

		case *east.Table:
			// 使用支持大表格拆分的方法
			tableResults := c.convertTableWithDataMultiple(node)
			for _, tableResult := range tableResults {
				if tableResult != nil {
					result.BlockNodes = append(result.BlockNodes, &BlockNode{Block: tableResult.Block})
					result.TableDatas = append(result.TableDatas, tableResult.TableData)
					c.addTableData(tableResult.Block, tableResult.TableData)
				}
			}
			return ast.WalkSkipChildren, nil

		case *ast.ThematicBreak:
			result.BlockNodes = append(result.BlockNodes, &BlockNode{Block: c.createDividerBlock()})
			return ast.WalkContinue, nil

		case *ast.HTMLBlock:
			// 处理块级 HTML 标签（如 <image/>, <callout>...</callout>）
			raw := c.getHTMLBlockText(node)

			// 优先匹配 <!-- feishu-colwidth: ... --> 注释，命中后暂存到 pendingColWidth，
			// 由紧邻其下的 ast.Table 消费一次。其他类型块（含其他 HTMLBlock）已在
			// walk 入口处统一清空 pending（见上方 isTableOrColWidthHTMLBlock 判断）。
			if m := colWidthCommentRe.FindStringSubmatch(raw); m != nil {
				c.pendingColWidth = parseColWidthList(m[1])
				return ast.WalkSkipChildren, nil
			}

			tag := ParseHTMLTag(raw)
			if tag != nil {
				blocks := c.handleBlockHTMLTag(tag)
				result.BlockNodes = append(result.BlockNodes, blocks...)
			}
			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	})

	if err != nil {
		return nil, err
	}

	result.ImageStats = c.imageStats
	result.ImageSources = c.imageSources
	result.VideoStats = c.videoStats
	result.VideoSources = c.videoSources
	result.FileStats = c.fileStats
	result.MediaRefs = c.mediaRefs
	result.Degradations = c.degradations
	result.TableDataByBlock = c.tableDataByBlock
	return result, nil
}

// addTableData 登记表格块的填充数据。
func (c *MarkdownToBlock) addTableData(block *larkdocx.Block, td *TableData) {
	if block == nil || td == nil {
		return
	}
	if c.tableDataByBlock == nil {
		c.tableDataByBlock = make(map[*larkdocx.Block]*TableData)
	}
	c.tableDataByBlock[block] = td
}

// addMediaRef 登记一个建块后需补齐内容的块。
func (c *MarkdownToBlock) addMediaRef(block *larkdocx.Block, ref *MediaRef) {
	if block == nil || ref == nil {
		return
	}
	if c.mediaRefs == nil {
		c.mediaRefs = make(map[*larkdocx.Block]*MediaRef)
	}
	c.mediaRefs[block] = ref
}

// addDegradation 登记一项转换期降级（导入层计入 failures）。
func (c *MarkdownToBlock) addDegradation(kind, source, reason string) {
	c.degradations = append(c.degradations, Degradation{Kind: kind, Source: source, Reason: reason})
}

// newTextBlock 构造只含给定元素的 Text 块。
func newTextBlock(elements ...*larkdocx.TextElement) *larkdocx.Block {
	bt := int(BlockTypeText)
	return &larkdocx.Block{BlockType: &bt, Text: &larkdocx.Text{Elements: elements}}
}

// Convert converts Markdown to Feishu blocks (flat list, nesting info is lost).
// For nested list support, use ConvertWithTableData() which preserves BlockNode hierarchy.
func (c *MarkdownToBlock) Convert() ([]*larkdocx.Block, error) {
	result, err := c.ConvertWithTableData()
	if err != nil {
		return nil, err
	}
	return FlattenBlockNodes(result.BlockNodes), nil
}

func (c *MarkdownToBlock) convertHeading(node *ast.Heading) (*larkdocx.Block, error) {
	elements := c.extractTextElements(node)

	level := node.Level
	if level > 9 {
		level = 9
	}

	blockType := int(BlockTypeHeading1) + level - 1

	block := &larkdocx.Block{
		BlockType: &blockType,
	}

	headingText := &larkdocx.Text{Elements: elements}
	switch level {
	case 1:
		block.Heading1 = headingText
	case 2:
		block.Heading2 = headingText
	case 3:
		block.Heading3 = headingText
	case 4:
		block.Heading4 = headingText
	case 5:
		block.Heading5 = headingText
	case 6:
		block.Heading6 = headingText
	case 7:
		block.Heading7 = headingText
	case 8:
		block.Heading8 = headingText
	case 9:
		block.Heading9 = headingText
	}

	return block, nil
}

func (c *MarkdownToBlock) convertParagraph(node *ast.Paragraph) ([]*BlockNode, error) {
	// Check if paragraph contains only an image
	if node.ChildCount() == 1 {
		if img, ok := node.FirstChild().(*ast.Image); ok {
			block, err := c.convertImage(img)
			if err != nil {
				return nil, err
			}
			if block == nil {
				return nil, nil
			}
			return []*BlockNode{{Block: block}}, nil
		}
	}

	// 检查段落是否只包含一个 <image> HTML 标签
	if block := c.tryConvertHTMLImageParagraph(node); block != nil {
		return []*BlockNode{{Block: block}}, nil
	}
	if block := c.tryConvertHTMLVideoParagraph(node); block != nil {
		return []*BlockNode{{Block: block}}, nil
	}

	// 按 SoftLineBreak 分行，每行创建独立的 Text 块
	// 解决连续行（无空行分隔）被合并为一段的问题
	lines := c.extractParagraphLines(node)
	if len(lines) == 0 {
		return nil, nil
	}

	var nodes []*BlockNode
	for _, lineElements := range lines {
		if len(lineElements) == 0 {
			continue
		}
		bt := int(BlockTypeText)
		nodes = append(nodes, &BlockNode{
			Block: &larkdocx.Block{
				BlockType: &bt,
				Text:      &larkdocx.Text{Elements: lineElements},
			},
		})
	}
	return nodes, nil
}

func (c *MarkdownToBlock) convertCodeBlock(node *ast.FencedCodeBlock) (*larkdocx.Block, error) {
	lang := string(node.Language(c.source))
	langCode := languageNameToCode(lang)

	var content bytes.Buffer
	lines := node.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		content.Write(line.Value(c.source))
	}

	text := strings.TrimRight(content.String(), "\n")
	textContent := text

	blockType := int(BlockTypeCode)
	return &larkdocx.Block{
		BlockType: &blockType,
		Code: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{
					TextRun: &larkdocx.TextRun{
						Content: &textContent,
					},
				},
			},
			Style: &larkdocx.TextStyle{
				Language: &langCode,
			},
		},
	}, nil
}

func (c *MarkdownToBlock) convertList(node *ast.List) ([]*BlockNode, error) {
	var nodes []*BlockNode
	isOrdered := node.IsOrdered()

	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if listItem, ok := child.(*ast.ListItem); ok {
			bn, err := c.convertListItem(listItem, isOrdered)
			if err != nil {
				return nil, err
			}
			if bn != nil {
				nodes = append(nodes, bn)
			}
		}
	}

	return nodes, nil
}

func (c *MarkdownToBlock) convertListItem(node *ast.ListItem, isOrdered bool) (*BlockNode, error) {
	// 列表项正文只取首个段落（Paragraph / 紧凑列表的 TextBlock）；其后的段落、代码块、引用、嵌套列表等
	// 按原顺序转换为该列表项的子块，不再把第二段起的文字并进首段（此前多段落会被无分隔地拼接）。
	lead := listItemLeadNode(node)

	if lead != nil && lead.ChildCount() > 0 {
		// GFM 任务列表：复选框位于首段第一个子节点
		if cb, ok := lead.FirstChild().(*east.TaskCheckBox); ok {
			block, err := c.convertGFMTaskListItem(lead, cb.IsChecked)
			if err != nil {
				return nil, err
			}
			children, err := c.convertListItemChildren(node, lead)
			if err != nil {
				return nil, err
			}
			return &BlockNode{Block: block, Children: children}, nil
		}
		// 兼容未被 GFM 识别的 "[ ] " / "[x] " 纯文本前缀
		if tb, ok := lead.(*ast.TextBlock); ok {
			if txt, ok := tb.FirstChild().(*ast.Text); ok {
				text := unescapeMarkdownText(string(txt.Segment.Value(c.source)))
				if strings.HasPrefix(text, "[ ] ") || strings.HasPrefix(text, "[x] ") || strings.HasPrefix(text, "[X] ") {
					block, err := c.convertTaskListItem(node, text)
					if err != nil {
						return nil, err
					}
					children, err := c.convertListItemChildren(node, lead)
					if err != nil {
						return nil, err
					}
					return &BlockNode{Block: block, Children: children}, nil
				}
			}
		}
	}

	var elements []*larkdocx.TextElement
	if lead != nil {
		elements = c.extractTextElements(lead)
	}

	children, err := c.convertListItemChildren(node, lead)
	if err != nil {
		return nil, err
	}

	// 过滤空列表项（飞书 API 不接受空内容的列表块）
	if (len(elements) == 0 || !hasNonEmptyContent(elements)) && len(children) == 0 {
		return nil, nil
	}

	// 如果没有直接文本但有子块，创建空文本的父块
	if len(elements) == 0 || !hasNonEmptyContent(elements) {
		empty := ""
		elements = []*larkdocx.TextElement{{TextRun: &larkdocx.TextRun{Content: &empty}}}
	}

	var block *larkdocx.Block
	if isOrdered {
		blockType := int(BlockTypeOrdered)
		block = &larkdocx.Block{
			BlockType: &blockType,
			Ordered:   &larkdocx.Text{Elements: elements},
		}
	} else {
		blockType := int(BlockTypeBullet)
		block = &larkdocx.Block{
			BlockType: &blockType,
			Bullet:    &larkdocx.Text{Elements: elements},
		}
	}

	return &BlockNode{Block: block, Children: children}, nil
}

// listItemLeadNode 返回列表项的正文节点：首个子节点为 Paragraph（松散列表）或 TextBlock（紧凑列表）时返回它，
// 否则（列表项以代码块、引用、嵌套列表等开头）返回 nil。
func listItemLeadNode(node *ast.ListItem) ast.Node {
	first := node.FirstChild()
	switch first.(type) {
	case *ast.Paragraph, *ast.TextBlock:
		return first
	}
	return nil
}

// convertListItemChildren 把列表项中正文（lead）之外的块级子节点按原顺序转换为子块。
// 飞书列表块（Bullet/Ordered/Todo）可挂文本、代码、引用、标题、分割线、图片等子块（2026-10 实测）。
// 无法结构化表达的节点（如列表项内的表格）降级为独立文本子块，不并入正文。
func (c *MarkdownToBlock) convertListItemChildren(node *ast.ListItem, lead ast.Node) ([]*BlockNode, error) {
	var children []*BlockNode
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if child == lead {
			continue
		}
		switch n := child.(type) {
		case *ast.List:
			childNodes, err := c.convertList(n)
			if err != nil {
				return nil, err
			}
			children = append(children, childNodes...)
		case *ast.FencedCodeBlock:
			block, err := c.convertCodeBlock(n)
			if err != nil {
				return nil, err
			}
			if block != nil {
				children = append(children, &BlockNode{Block: block})
			}
		case *ast.Paragraph:
			nodes, err := c.convertParagraph(n)
			if err != nil {
				return nil, err
			}
			children = append(children, nodes...)
		case *ast.TextBlock:
			for _, line := range c.extractParagraphLines(n) {
				if len(line) > 0 {
					children = append(children, &BlockNode{Block: newTextBlock(line...)})
				}
			}
		case *ast.Blockquote:
			quoteNodes, err := c.convertBlockquote(n)
			if err != nil {
				return nil, err
			}
			children = append(children, quoteNodes...)
		case *ast.Heading:
			block, err := c.convertHeading(n)
			if err != nil {
				return nil, err
			}
			if block != nil {
				children = append(children, &BlockNode{Block: block})
			}
		case *ast.ThematicBreak:
			children = append(children, &BlockNode{Block: c.createDividerBlock()})
		case *ast.HTMLBlock:
			if tag := ParseHTMLTag(c.getHTMLBlockText(n)); tag != nil {
				children = append(children, c.handleBlockHTMLTag(tag)...)
			}
		default:
			elements := c.extractTextElements(child)
			if len(elements) > 0 && hasNonEmptyContent(elements) {
				children = append(children, &BlockNode{Block: newTextBlock(elements...)})
			}
		}
	}
	return children, nil
}

func (c *MarkdownToBlock) convertTaskListItem(node *ast.ListItem, text string) (*larkdocx.Block, error) {
	done := strings.HasPrefix(text, "[x] ") || strings.HasPrefix(text, "[X] ")

	// Remove checkbox prefix from text
	text = strings.TrimPrefix(text, "[ ] ")
	text = strings.TrimPrefix(text, "[x] ")
	text = strings.TrimPrefix(text, "[X] ")

	blockType := int(BlockTypeTodo)
	return &larkdocx.Block{
		BlockType: &blockType,
		Todo: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{
					TextRun: &larkdocx.TextRun{
						Content: &text,
					},
				},
			},
			Style: &larkdocx.TextStyle{
				Done: &done,
			},
		},
	}, nil
}

// convertGFMTaskListItem 以列表项正文节点（含 TaskCheckBox 的首段）构造 Todo 块。
func (c *MarkdownToBlock) convertGFMTaskListItem(node ast.Node, isChecked bool) (*larkdocx.Block, error) {
	// Extract text elements, skipping the TaskCheckBox node
	elements := c.extractTextElementsSkipCheckbox(node)

	blockType := int(BlockTypeTodo)
	return &larkdocx.Block{
		BlockType: &blockType,
		Todo: &larkdocx.Text{
			Elements: elements,
			Style: &larkdocx.TextStyle{
				Done: &isChecked,
			},
		},
	}, nil
}

func (c *MarkdownToBlock) extractTextElementsSkipCheckbox(node ast.Node) []*larkdocx.TextElement {
	var elements []*larkdocx.TextElement

	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		// Skip TaskCheckBox nodes
		if _, ok := n.(*east.TaskCheckBox); ok {
			return ast.WalkSkipChildren, nil
		}

		// 跳过嵌套列表——它们作为 BlockNode.Children 单独处理
		if _, ok := n.(*ast.List); ok {
			return ast.WalkSkipChildren, nil
		}

		switch child := n.(type) {
		case *ast.Text:
			text := unescapeMarkdownText(string(child.Segment.Value(c.source)))
			if text != "" {
				elements = append(elements, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{
						Content: &text,
					},
				})
			}

		case *ast.String:
			text := string(child.Value)
			if text != "" {
				elements = append(elements, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{
						Content: &text,
					},
				})
			}

		case *ast.Emphasis:
			childElems := c.extractChildElements(child)
			bold := child.Level == 2
			italic := child.Level == 1
			for _, elem := range childElems {
				applyTextStyle(elem, bold, italic, false)
				elements = append(elements, elem)
			}
			return ast.WalkSkipChildren, nil

		case *ast.CodeSpan:
			text := c.getNodeText(child)
			if text != "" {
				inlineCode := true
				elements = append(elements, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{
						Content: &text,
						TextElementStyle: &larkdocx.TextElementStyle{
							InlineCode: &inlineCode,
						},
					},
				})
			}
			return ast.WalkSkipChildren, nil

		case *east.Strikethrough:
			childElems := c.extractChildElements(child)
			for _, elem := range childElems {
				applyTextStyle(elem, false, false, true)
				elements = append(elements, elem)
			}
			return ast.WalkSkipChildren, nil

		case *ast.Link:
			text := c.getNodeText(child)
			url := string(child.Destination)
			if text != "" {
				elements = append(elements, createLinkElement(text, url))
			}
			return ast.WalkSkipChildren, nil

		case *ast.AutoLink:
			linkURL := string(child.URL(c.source))
			label := string(child.Label(c.source))
			if label == "" {
				label = linkURL
			}
			if linkURL != "" {
				elements = append(elements, createLinkElement(label, linkURL))
			}
			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	})

	return elements
}

func (c *MarkdownToBlock) convertBlockquote(node *ast.Blockquote) ([]*BlockNode, error) {
	// Check for callout syntax [!TYPE]
	// goldmark 可能将 [!NOTE] 拆分为多个 Text 节点，需要合并后匹配
	var calloutType string
	calloutRe := regexp.MustCompile(`^\[!(\w+)\]`)
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if para, ok := child.(*ast.Paragraph); ok {
			// 合并段落首行所有文本节点
			var firstLineText string
			for inline := para.FirstChild(); inline != nil; inline = inline.NextSibling() {
				if txt, ok := inline.(*ast.Text); ok {
					firstLineText += unescapeMarkdownText(string(txt.Segment.Value(c.source)))
					if txt.SoftLineBreak() {
						break
					}
				} else {
					break
				}
			}
			if match := calloutRe.FindStringSubmatch(firstLineText); match != nil {
				calloutType = match[1]
				break
			}
		}
	}

	if calloutType != "" {
		calloutNode, err := c.convertCallout(node, calloutType)
		if err != nil {
			return nil, err
		}
		return []*BlockNode{calloutNode}, nil
	}

	// 使用 QuoteContainer 容器块，支持嵌套结构
	containerBlockType := int(BlockTypeQuoteContainer)
	containerBlock := &larkdocx.Block{
		BlockType:      &containerBlockType,
		QuoteContainer: &larkdocx.QuoteContainer{},
	}

	var children []*BlockNode
	paragraphCount := 0 // 跟踪已处理的段落数，用于插入段落间空行
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Paragraph:
			// 段落间插入空 Text 块，对应引用块内的 > 空行
			if paragraphCount > 0 {
				emptyBlockType := int(BlockTypeText)
				emptyContent := ""
				children = append(children, &BlockNode{
					Block: &larkdocx.Block{
						BlockType: &emptyBlockType,
						Text: &larkdocx.Text{Elements: []*larkdocx.TextElement{
							{TextRun: &larkdocx.TextRun{Content: &emptyContent}},
						}},
					},
				})
			}
			paragraphCount++

			// 按行拆分段落内容
			lines := c.extractQuoteLines(n)
			textBlockType := int(BlockTypeText)
			for _, line := range lines {
				if len(line) > 0 {
					children = append(children, &BlockNode{
						Block: &larkdocx.Block{
							BlockType: &textBlockType,
							Text:      &larkdocx.Text{Elements: line},
						},
					})
				}
			}
		case *ast.List:
			listNodes, err := c.convertList(n)
			if err == nil {
				children = append(children, listNodes...)
			}
		case *ast.FencedCodeBlock:
			block, err := c.convertCodeBlock(n)
			if err == nil && block != nil {
				children = append(children, &BlockNode{Block: block})
			}
		case *ast.Blockquote:
			// 嵌套引用 `> >`：飞书不允许 QuoteContainer/Callout 作为 QuoteContainer 的子块
			// （1770030 invalid parent children relation，整批子块连同外层内容一起失败）。
			// 扁平化：把内层引用/Callout 的子块直接并入外层引用，保证内容不丢。
			nestedNodes, err := c.convertBlockquote(n)
			if err == nil {
				children = append(children, flattenQuoteChildren(nestedNodes)...)
			}
		default:
			// 其他节点，提取文本
			lines := c.extractQuoteLines(child)
			textBlockType := int(BlockTypeText)
			for _, line := range lines {
				if len(line) > 0 {
					children = append(children, &BlockNode{
						Block: &larkdocx.Block{
							BlockType: &textBlockType,
							Text:      &larkdocx.Text{Elements: line},
						},
					})
				}
			}
		}
	}

	// 如果没有子块，创建一个含空内容的文本子块
	if len(children) == 0 {
		textBlockType := int(BlockTypeText)
		emptyContent := ""
		children = append(children, &BlockNode{
			Block: &larkdocx.Block{
				BlockType: &textBlockType,
				Text: &larkdocx.Text{Elements: []*larkdocx.TextElement{
					{TextRun: &larkdocx.TextRun{Content: &emptyContent}},
				}},
			},
		})
	}

	return []*BlockNode{{Block: containerBlock, Children: children}}, nil
}

// flattenQuoteChildren 把不能直接作为 QuoteContainer 子块的容器（QuoteContainer / Callout）
// 递归展开为其子块，其余节点原样保留。用于嵌套引用扁平化。
func flattenQuoteChildren(nodes []*BlockNode) []*BlockNode {
	var out []*BlockNode
	for _, n := range nodes {
		if n == nil || n.Block == nil || n.Block.BlockType == nil {
			continue
		}
		switch BlockType(*n.Block.BlockType) {
		case BlockTypeQuoteContainer, BlockTypeCallout:
			out = append(out, flattenQuoteChildren(n.Children)...)
		default:
			out = append(out, n)
		}
	}
	return out
}

// extractQuoteLines 从 AST 节点提取文本元素，按 SoftLineBreak 拆分为多行
// 用于引用块（blockquote），将连续 > 行正确拆分为独立的 Quote 块
func (c *MarkdownToBlock) extractQuoteLines(node ast.Node) [][]*larkdocx.TextElement {
	var lines [][]*larkdocx.TextElement
	var currentLine []*larkdocx.TextElement

	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch child := n.(type) {
		case *ast.Text:
			text := unescapeMarkdownText(string(child.Segment.Value(c.source)))
			if text != "" {
				currentLine = append(currentLine, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{Content: &text},
				})
			}
			if child.SoftLineBreak() {
				if len(currentLine) > 0 {
					lines = append(lines, currentLine)
					currentLine = nil
				}
			}

		case *ast.String:
			text := string(child.Value)
			if text != "" {
				currentLine = append(currentLine, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{Content: &text},
				})
			}

		case *ast.Emphasis:
			childElems := c.extractChildElements(child)
			bold := child.Level == 2
			italic := child.Level == 1
			for _, elem := range childElems {
				applyTextStyle(elem, bold, italic, false)
				currentLine = append(currentLine, elem)
			}
			return ast.WalkSkipChildren, nil

		case *ast.CodeSpan:
			text := c.getNodeText(child)
			if text != "" {
				inlineCode := true
				currentLine = append(currentLine, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{
						Content:          &text,
						TextElementStyle: &larkdocx.TextElementStyle{InlineCode: &inlineCode},
					},
				})
			}
			return ast.WalkSkipChildren, nil

		case *east.Strikethrough:
			childElems := c.extractChildElements(child)
			for _, elem := range childElems {
				applyTextStyle(elem, false, false, true)
				currentLine = append(currentLine, elem)
			}
			return ast.WalkSkipChildren, nil

		case *ast.Link:
			text := c.getNodeText(child)
			url := string(child.Destination)
			if text != "" {
				currentLine = append(currentLine, createLinkElement(text, url))
			}
			return ast.WalkSkipChildren, nil

		case *ast.AutoLink:
			linkURL := string(child.URL(c.source))
			label := string(child.Label(c.source))
			if label == "" {
				label = linkURL
			}
			if linkURL != "" {
				currentLine = append(currentLine, createLinkElement(label, linkURL))
			}
			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	})

	// 添加最后一行
	if len(currentLine) > 0 {
		lines = append(lines, currentLine)
	}

	return lines
}

// extractParagraphLines 从段落 AST 节点提取文本元素，按 SoftLineBreak 拆分为多行。
//
// Known limitation：位于行内容器（Emphasis/Strikethrough/Link）内部的换行符不会触发分行——
// 这些节点走 WalkSkipChildren，子节点的 SoftLineBreak 无法到达顶层。
func (c *MarkdownToBlock) extractParagraphLines(node ast.Node) [][]*larkdocx.TextElement {
	var lines [][]*larkdocx.TextElement
	var currentLine []*larkdocx.TextElement
	inUnderline := false

	// 辅助：将 <u>/<mark> 等样式状态应用到 elem
	applyUnderlineIfNeeded := func(elem *larkdocx.TextElement) {
		if !inUnderline || elem == nil || elem.TextRun == nil {
			return
		}
		underline := true
		if elem.TextRun.TextElementStyle == nil {
			elem.TextRun.TextElementStyle = &larkdocx.TextElementStyle{}
		}
		elem.TextRun.TextElementStyle.Underline = &underline
	}

	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch child := n.(type) {
		case *ast.Text:
			text := unescapeMarkdownText(string(child.Segment.Value(c.source)))
			if text != "" {
				elem := &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{Content: &text},
				}
				applyUnderlineIfNeeded(elem)
				currentLine = append(currentLine, elem)
			}
			if child.SoftLineBreak() {
				if len(currentLine) > 0 {
					lines = append(lines, splitInlineMath(currentLine))
					currentLine = nil
				}
			}

		case *ast.String:
			text := string(child.Value)
			if text != "" {
				elem := &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{Content: &text},
				}
				applyUnderlineIfNeeded(elem)
				currentLine = append(currentLine, elem)
			}

		case *ast.Emphasis:
			childElems := c.extractChildElements(child)
			bold := child.Level == 2
			italic := child.Level == 1
			for _, elem := range childElems {
				applyTextStyle(elem, bold, italic, false)
				applyUnderlineIfNeeded(elem)
				currentLine = append(currentLine, elem)
			}
			return ast.WalkSkipChildren, nil

		case *ast.CodeSpan:
			text := c.getNodeText(child)
			if text != "" {
				inlineCode := true
				elem := &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{
						Content:          &text,
						TextElementStyle: &larkdocx.TextElementStyle{InlineCode: &inlineCode},
					},
				}
				applyUnderlineIfNeeded(elem)
				currentLine = append(currentLine, elem)
			}
			return ast.WalkSkipChildren, nil

		case *east.Strikethrough:
			childElems := c.extractChildElements(child)
			for _, elem := range childElems {
				applyTextStyle(elem, false, false, true)
				applyUnderlineIfNeeded(elem)
				currentLine = append(currentLine, elem)
			}
			return ast.WalkSkipChildren, nil

		case *ast.Link:
			text := c.getNodeText(child)
			url := string(child.Destination)
			if text != "" {
				elem := createLinkElement(text, url)
				applyUnderlineIfNeeded(elem)
				currentLine = append(currentLine, elem)
			}
			return ast.WalkSkipChildren, nil

		case *ast.AutoLink:
			linkURL := string(child.URL(c.source))
			label := string(child.Label(c.source))
			if label == "" {
				label = linkURL
			}
			if linkURL != "" {
				elem := createLinkElement(label, linkURL)
				applyUnderlineIfNeeded(elem)
				currentLine = append(currentLine, elem)
			}
			return ast.WalkSkipChildren, nil

		case *ast.Image:
			// 内联图片：统一降级为「[图片: alt]」占位（http(s) 保留链接，本地路径用 alt 不泄漏原始路径）。
			elem := c.imageInlinePlaceholder(child)
			applyUnderlineIfNeeded(elem)
			currentLine = append(currentLine, elem)
			return ast.WalkSkipChildren, nil

		case *ast.RawHTML:
			// 处理内联 HTML 标签：<br> 作为软换行拆分，<u>/</u> 切换下划线状态，
			// <mark>/</mark>（暂以下划线近似）、其他标签按占位/纯文本保留，避免静默丢失。
			var htmlBuf bytes.Buffer
			for i := 0; i < child.Segments.Len(); i++ {
				seg := child.Segments.At(i)
				htmlBuf.Write(c.source[seg.Start:seg.Stop])
			}
			rawOriginal := strings.TrimSpace(htmlBuf.String())
			raw := strings.ToLower(rawOriginal)
			switch {
			case raw == "<br>" || raw == "<br/>" || raw == "<br />":
				// <br> 视为软换行，当前行收尾并开启新行
				if len(currentLine) > 0 {
					lines = append(lines, splitInlineMath(currentLine))
					currentLine = nil
				}
			case raw == "<u>", raw == "<mark>":
				inUnderline = true
			case raw == "</u>", raw == "</mark>":
				inUnderline = false
			default:
				// 尝试解析自定义 HTML 标签（如 <mention-user/>）
				if tag := ParseHTMLTag(rawOriginal); tag != nil {
					if elems := c.handleInlineHTMLTag(tag, &inUnderline); len(elems) > 0 {
						for _, elem := range elems {
							applyUnderlineIfNeeded(elem)
							currentLine = append(currentLine, elem)
						}
					}
				}
				// 其他未识别的 HTML 标签丢弃（与 extractTextElements/extractChildElements 保持一致）
			}
			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	})

	// 添加最后一行
	if len(currentLine) > 0 {
		lines = append(lines, splitInlineMath(currentLine))
	}

	return lines
}

func (c *MarkdownToBlock) convertCallout(node *ast.Blockquote, calloutType string) (*BlockNode, error) {
	// Map callout type to background color（枚举见 CalloutColorForType）
	bgColor := CalloutColorForType(calloutType)

	blockType := int(BlockTypeCallout)
	calloutBlock := &larkdocx.Block{
		BlockType: &blockType,
		Callout: &larkdocx.Callout{
			BackgroundColor: &bgColor,
		},
	}

	// 提取 Callout 子块内容（跳过 [!TYPE] 首行）
	var children []*BlockNode
	firstPara := true
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if firstPara {
			if para, ok := child.(*ast.Paragraph); ok {
				firstPara = false
				// 跳过 [!TYPE] 标记，提取剩余内容
				elements := c.extractCalloutParaElements(para, calloutType)
				if len(elements) > 0 && hasNonEmptyContent(elements) {
					textBlockType := int(BlockTypeText)
					children = append(children, &BlockNode{
						Block: &larkdocx.Block{
							BlockType: &textBlockType,
							Text:      &larkdocx.Text{Elements: elements},
						},
					})
				}
				continue
			}
			firstPara = false
		}

		// 处理后续子节点
		switch n := child.(type) {
		case *ast.Paragraph:
			elements := c.extractTextElements(n)
			if len(elements) > 0 && hasNonEmptyContent(elements) {
				textBlockType := int(BlockTypeText)
				children = append(children, &BlockNode{
					Block: &larkdocx.Block{
						BlockType: &textBlockType,
						Text:      &larkdocx.Text{Elements: elements},
					},
				})
			}
		case *ast.List:
			listNodes, err := c.convertList(n)
			if err == nil {
				children = append(children, listNodes...)
			}
		case *ast.FencedCodeBlock:
			block, err := c.convertCodeBlock(n)
			if err == nil && block != nil {
				children = append(children, &BlockNode{Block: block})
			}
		default:
			// 其他块级节点，提取文本
			elements := c.extractTextElements(child)
			if len(elements) > 0 && hasNonEmptyContent(elements) {
				textBlockType := int(BlockTypeText)
				children = append(children, &BlockNode{
					Block: &larkdocx.Block{
						BlockType: &textBlockType,
						Text:      &larkdocx.Text{Elements: elements},
					},
				})
			}
		}
	}

	return &BlockNode{Block: calloutBlock, Children: children}, nil
}

// extractCalloutParaElements 提取 Callout 首段落的文本元素，跳过 [!TYPE] 标记
func (c *MarkdownToBlock) extractCalloutParaElements(para *ast.Paragraph, calloutType string) []*larkdocx.TextElement {
	elements := c.extractTextElements(para)
	if len(elements) == 0 {
		return nil
	}

	// goldmark 可能将 [!NOTE] 拆分为多个 TextElement（如 "[" + "!NOTE" + "]"），
	// 需要先合并文本再匹配，然后按匹配长度移除对应的元素。
	prefix := "[!" + strings.ToUpper(calloutType) + "]"

	// 先尝试单元素匹配
	for i, elem := range elements {
		if elem.TextRun != nil && elem.TextRun.Content != nil {
			content := *elem.TextRun.Content
			if idx := strings.Index(content, prefix); idx >= 0 {
				remaining := strings.TrimSpace(content[idx+len(prefix):])
				if remaining == "" {
					elements = append(elements[:i], elements[i+1:]...)
				} else {
					elements[i].TextRun.Content = &remaining
				}
				return elements
			}
		}
	}

	// 单元素未找到，尝试跨元素合并匹配
	var concat string
	for _, elem := range elements {
		if elem.TextRun != nil && elem.TextRun.Content != nil {
			concat += *elem.TextRun.Content
		}
	}

	if idx := strings.Index(concat, prefix); idx >= 0 {
		// 确定需要跳过的字节数
		skipBytes := idx + len(prefix)
		consumed := 0
		cutIdx := 0
		for cutIdx < len(elements) {
			elem := elements[cutIdx]
			if elem.TextRun != nil && elem.TextRun.Content != nil {
				consumed += len(*elem.TextRun.Content)
			}
			cutIdx++
			if consumed >= skipBytes {
				break
			}
		}

		// cutIdx 之前的元素全部移除
		remaining := elements[cutIdx:]

		// 如果最后一个被消费的元素有多余内容，保留尾部
		if consumed > skipBytes {
			lastElem := elements[cutIdx-1]
			if lastElem.TextRun != nil && lastElem.TextRun.Content != nil {
				tail := (*lastElem.TextRun.Content)[len(*lastElem.TextRun.Content)-(consumed-skipBytes):]
				tail = strings.TrimSpace(tail)
				if tail != "" {
					tailElem := &larkdocx.TextElement{
						TextRun: &larkdocx.TextRun{Content: &tail, TextElementStyle: lastElem.TextRun.TextElementStyle},
					}
					remaining = append([]*larkdocx.TextElement{tailElem}, remaining...)
				}
			}
		}

		// 移除剩余元素中的前导空白和换行
		for len(remaining) > 0 {
			first := remaining[0]
			if first.TextRun != nil && first.TextRun.Content != nil {
				trimmed := strings.TrimLeft(*first.TextRun.Content, " \t\n\r")
				if trimmed == "" {
					remaining = remaining[1:]
					continue
				}
				first.TextRun.Content = &trimmed
			}
			break
		}

		return remaining
	}

	return elements
}

func (c *MarkdownToBlock) convertImage(node *ast.Image) (*larkdocx.Block, error) {
	dest := string(node.Destination)

	// feishu://media/<token> 是飞书素材引用：token 绑定源文档，不能直接挂到新块上（1770013），
	// 开启上传时由导入层下载该素材再重新上传到新建的空 Image 块（素材复用）。
	if token, ok := strings.CutPrefix(dest, FeishuMediaScheme); ok {
		return c.newTokenImageBlock(token, 0, 0, 0), nil
	}

	if !c.options.UploadImages {
		c.imageStats.Skipped++
		return c.createImagePlaceholder(dest), nil
	}

	// 图片三步法上传：
	// 1. 创建空 Image Block → 获得 imageBlockID
	// 2. UploadMediaWithExtra(filePath, "docx_image", imageBlockID, ..., extra) → 获得 fileToken
	// 3. ReplaceImage(documentID, imageBlockID, fileToken, opts) → 绑定 token 与显示宽高
	// 此处仅创建空 Image Block，记录图片来源路径，实际上传在 cmd 层完成。
	c.imageStats.Total++
	c.imageSources = append(c.imageSources, dest)
	block := newEmptyImageBlock()
	c.addMediaRef(block, &MediaRef{Kind: MediaKindImage, Source: dest})
	return block, nil
}

func newEmptyImageBlock() *larkdocx.Block {
	blockType := int(BlockTypeImage)
	return &larkdocx.Block{
		BlockType: &blockType,
		Image:     &larkdocx.Image{},
	}
}

// newTokenImageBlock 处理引用已有素材 token 的图片（<image token> / feishu://media/）。
// 建块接口不接受带 token 的 Image（1770001），开启上传时建空 Image 块并登记 MediaRef，
// 由导入层下载原素材后重新上传绑定；关闭上传时降级为占位文本（计入 Skipped）。
func (c *MarkdownToBlock) newTokenImageBlock(token string, width, height, align int) *larkdocx.Block {
	ref := FeishuMediaScheme + token
	if token == "" || !c.options.UploadImages {
		c.imageStats.Skipped++
		return c.createImagePlaceholder(ref)
	}
	c.imageStats.Total++
	block := newEmptyImageBlock()
	c.addMediaRef(block, &MediaRef{Kind: MediaKindImage, Token: token, Width: width, Height: height, Align: align})
	return block
}

func (c *MarkdownToBlock) createImagePlaceholder(url string) *larkdocx.Block {
	text := fmt.Sprintf("[Image: %s]", url)
	blockType := int(BlockTypeText)
	return &larkdocx.Block{
		BlockType: &blockType,
		Text: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{
					TextRun: &larkdocx.TextRun{
						Content: &text,
					},
				},
			},
		},
	}
}

func (c *MarkdownToBlock) createDividerBlock() *larkdocx.Block {
	blockType := int(BlockTypeDivider)
	return &larkdocx.Block{
		BlockType: &blockType,
		Divider:   &larkdocx.Divider{},
	}
}

// TableData stores table information for later content filling
type TableData struct {
	Rows         int
	Cols         int
	CellContents []string                  // 纯文本内容（兼容）
	CellElements [][]*larkdocx.TextElement // 富文本元素（保留链接等样式）
	HasHeader    bool
	// ExtraRowContents: 超过 maxTableRows 后需通过 insert_table_row API 追加的数据行（纯文本）
	// 每个元素为一行的单元格内容（长度应等于 Cols）
	ExtraRowContents [][]string
	// ExtraRowElements: 超过 maxTableRows 后需追加的数据行（富文本元素，与 ExtraRowContents 对应）
	ExtraRowElements [][][]*larkdocx.TextElement
	// CellImages: 各单元格内待嵌入的图片源（本地路径 / URL），按最终表格单元格行优先顺序对齐
	// （表头单元格 + 初始数据行单元格 + 追加行单元格，长度 = 最终行数 × 列数）。
	// 仅在 EmbedTableImages 开启且表内确有单元格图片时非 nil；空表示该格无图。导入层据此在单元格内建 Image 子块。
	CellImages [][]string
}

// ConvertTableResult contains both the block and the table data for content filling
type ConvertTableResult struct {
	Block     *larkdocx.Block
	TableData *TableData
}

func (c *MarkdownToBlock) convertTable(node *east.Table) (*larkdocx.Block, error) {
	result := c.convertTableWithData(node)
	if result == nil {
		return nil, nil
	}
	return result.Block, nil
}

// splitColumnGroups 将列索引分组，每组最多 maxTableCols 列。
// 第一列（通常是标识/名称列）在所有分组中保留，避免拆分后数据行无法识别。
// 列数 ≤ maxTableCols 时返回 nil 表示无需拆分。
func splitColumnGroups(totalCols int) [][]int {
	if totalCols <= maxTableCols {
		return nil
	}
	var groups [][]int
	// 第一组：col0 ~ col(maxTableCols-1)
	first := make([]int, maxTableCols)
	for i := 0; i < maxTableCols; i++ {
		first[i] = i
	}
	groups = append(groups, first)
	// 后续组：col0（标识列）+ 连续数据列，每组最多 maxTableCols 列
	maxDataPerGroup := maxTableCols - 1 // 留一列给标识列
	for i := maxTableCols; i < totalCols; i += maxDataPerGroup {
		end := i + maxDataPerGroup
		if end > totalCols {
			end = totalCols
		}
		group := []int{0} // 保留第一列
		for j := i; j < end; j++ {
			group = append(group, j)
		}
		groups = append(groups, group)
	}
	return groups
}

// extractColumns 从一行纯文本中提取指定列
func extractColumns(row []string, colIndices []int) []string {
	result := make([]string, len(colIndices))
	for i, idx := range colIndices {
		if idx < len(row) {
			result[i] = row[idx]
		}
	}
	return result
}

// extractColumnElements 从一行富文本元素中提取指定列
func extractColumnElements(rowElements [][]*larkdocx.TextElement, colIndices []int) [][]*larkdocx.TextElement {
	result := make([][]*larkdocx.TextElement, len(colIndices))
	for i, idx := range colIndices {
		if idx < len(rowElements) {
			result[i] = rowElements[idx]
		}
	}
	return result
}

// extractIntColumns 与 extractColumns 同形，用于把列宽数组按列拆分索引切片。
// 索引越界默认补 0（resolveColumnWidths 会把 0 视作 auto）。
func extractIntColumns(values []int, colIndices []int) []int {
	result := make([]int, len(colIndices))
	for i, idx := range colIndices {
		if idx < len(values) {
			result[i] = values[idx]
		}
	}
	return result
}

func (c *MarkdownToBlock) convertTableWithData(node *east.Table) *ConvertTableResult {
	results := c.convertTableWithDataMultiple(node)
	if len(results) == 0 {
		return nil
	}
	return results[0]
}

// convertTableWithDataMultiple 将大表格拆分成多个小表格（每个最多 9 行）
func (c *MarkdownToBlock) convertTableWithDataMultiple(node *east.Table) []*ConvertTableResult {
	// Count rows and columns, collect all cell contents (plain text + rich elements)
	var cols int
	var headerContents []string
	var headerElements [][]*larkdocx.TextElement
	var headerImages [][]string                     // 各表头单元格的图片源（与 headerElements 对齐）
	var dataRows [][]string                         // 纯文本，用于列宽计算
	var dataRowElements [][][]*larkdocx.TextElement // 富文本元素，保留链接等样式
	var dataRowImages [][][]string                  // 各数据行各单元格的图片源（与 dataRowElements 对齐）
	hasHeader := false

	for row := node.FirstChild(); row != nil; row = row.NextSibling() {
		if header, ok := row.(*east.TableHeader); ok {
			cols = row.ChildCount()
			hasHeader = true
			for cell := header.FirstChild(); cell != nil; cell = cell.NextSibling() {
				if tc, ok := cell.(*east.TableCell); ok {
					elems, imgs := c.extractCellElementsCollectingImages(tc)
					headerContents = append(headerContents, c.cellFallbackContent(tc, elems, imgs))
					headerElements = append(headerElements, elems)
					headerImages = append(headerImages, imgs)
				}
			}
		} else if tr, ok := row.(*east.TableRow); ok {
			if cols == 0 {
				cols = row.ChildCount()
			}
			var rowContents []string
			var rowElements [][]*larkdocx.TextElement
			var rowImages [][]string
			for cell := tr.FirstChild(); cell != nil; cell = cell.NextSibling() {
				if tc, ok := cell.(*east.TableCell); ok {
					elems, imgs := c.extractCellElementsCollectingImages(tc)
					rowContents = append(rowContents, c.cellFallbackContent(tc, elems, imgs))
					rowElements = append(rowElements, elems)
					rowImages = append(rowImages, imgs)
				}
			}
			dataRows = append(dataRows, rowContents)
			dataRowElements = append(dataRowElements, rowElements)
			dataRowImages = append(dataRowImages, rowImages)
		}
	}

	totalRows := len(dataRows)
	if hasHeader {
		totalRows++
	}
	if totalRows == 0 || cols == 0 {
		return nil
	}

	// 按列分组（超过 maxTableCols 列时拆分，保留首列作为标识列）
	colGroups := splitColumnGroups(cols)

	// buildRowSplitResults 对一组列的数据执行行拆分，返回拆分后的子表格列表
	buildRowSplitResults := func(groupCols int, groupHeader []string, groupHeaderElems [][]*larkdocx.TextElement,
		groupDataRows [][]string, groupDataRowElements [][][]*larkdocx.TextElement, groupColWidths []int, groupHasHeader bool,
		groupHeaderImages [][]string, groupDataImages [][][]string) []*ConvertTableResult {

		groupTotalRows := len(groupDataRows)
		if groupHasHeader {
			groupTotalRows++
		}

		// 构建 TableData
		buildTableData := func(rows int, chunkDataRows [][]string, chunkDataElements [][][]*larkdocx.TextElement) *TableData {
			var cellContents []string
			var cellElements [][]*larkdocx.TextElement
			if groupHasHeader {
				cellContents = append(cellContents, groupHeader...)
				cellElements = append(cellElements, groupHeaderElems...)
			}
			for _, row := range chunkDataRows {
				cellContents = append(cellContents, row...)
			}
			for _, row := range chunkDataElements {
				cellElements = append(cellElements, row...)
			}
			return &TableData{
				Rows:         rows,
				Cols:         groupCols,
				CellContents: cellContents,
				CellElements: cellElements,
				HasHeader:    groupHasHeader,
			}
		}

		// 行数≤限制：直接创建单表
		// 行数>限制：创建 maxTableRows 行的初始表，剩余行通过 insert_table_row API 追加
		// （飞书 API 限制 create_block 单表最多 9 行，但 insert_table_row 可突破此限制）
		initialDataRowCount := maxTableRows
		if groupHasHeader {
			initialDataRowCount = maxTableRows - 1
		}
		if initialDataRowCount > len(groupDataRows) {
			initialDataRowCount = len(groupDataRows)
		}

		initialDataRows := groupDataRows[:initialDataRowCount]
		initialDataElements := groupDataRowElements[:initialDataRowCount]
		extraDataRows := groupDataRows[initialDataRowCount:]
		extraDataElements := groupDataRowElements[initialDataRowCount:]

		initialRows := initialDataRowCount
		if groupHasHeader {
			initialRows++
		}
		blockType := int(BlockTypeTable)
		headerRow := groupHasHeader
		block := &larkdocx.Block{
			BlockType: &blockType,
			Table: &larkdocx.Table{
				Property: &larkdocx.TableProperty{
					RowSize:     &initialRows,
					ColumnSize:  &groupCols,
					ColumnWidth: groupColWidths,
					HeaderRow:   &headerRow,
				},
			},
		}
		td := buildTableData(initialRows, initialDataRows, initialDataElements)
		if len(extraDataRows) > 0 {
			td.ExtraRowContents = extraDataRows
			td.ExtraRowElements = extraDataElements
		}

		// 组装单元格图片源，顺序与最终表格单元格行优先一致：表头 + 初始数据行 + 追加行。
		// 仅当确有图片时才挂载（保持无图表格 CellImages 为 nil，导入层据此跳过嵌入）。
		var cellImages [][]string
		if groupHasHeader {
			cellImages = append(cellImages, groupHeaderImages...)
		}
		// 初始行 + 追加行在 groupDataImages 中本就按最终单元格顺序连续排列，一趟遍历即可
		// （等价于按 initialDataRowCount 切两段拼接，且避免 initialDataRowCount 越界 panic）。
		for _, row := range groupDataImages {
			cellImages = append(cellImages, row...)
		}
		for _, imgs := range cellImages {
			if len(imgs) > 0 {
				td.CellImages = cellImages
				break
			}
		}
		return []*ConvertTableResult{{Block: block, TableData: td}}
	}

	// 无需列拆分：直接走行拆分逻辑
	if colGroups == nil {
		columnWidths := c.resolveColumnWidths(headerContents, dataRows, cols)
		return buildRowSplitResults(cols, headerContents, headerElements, dataRows, dataRowElements, columnWidths, hasHeader, headerImages, dataRowImages)
	}

	// 需要列拆分：对每个列组提取数据，再分别行拆分
	// 显式指定的 pendingColWidth 也按列组切片：第 i 组取出 pendingColWidth 中对应索引的子集，
	// 让用户能一次性写完整张表的宽度，而不必关心列拆分。
	var splitPendingColWidth []int
	if len(c.pendingColWidth) > 0 {
		splitPendingColWidth = c.pendingColWidth
		c.pendingColWidth = nil // 列拆分场景下消费一次，避免泄漏到下一表
	}
	// 显式 flag(--table-column-width=explicit) 的列宽也要按列组切片，否则第二组及以后会取错列
	// （与上面注释 splitPendingColWidth 对称）：拆分后第 i 组是 col0(标识列)+本组其余列，必须用
	// extractIntColumns 取对应索引，而不是把完整数组传给每组让 alignAndClamp 截前 N 个（会错位，
	// 并打印"提供 N 个列宽但表只有 M 列"的误导警告）。options 跨整篇文档共享，故循环内临时替换、函数返回时恢复。
	var splitExplicitColWidth []int
	if c.options.ColumnWidthMode == "explicit" && len(c.options.ColumnWidthValues) > 0 {
		splitExplicitColWidth = c.options.ColumnWidthValues
		origExplicit := c.options.ColumnWidthValues
		defer func() { c.options.ColumnWidthValues = origExplicit }()
	}
	var results []*ConvertTableResult
	for _, colIndices := range colGroups {
		groupCols := len(colIndices)

		// 提取该列组的表头
		var groupHeader []string
		var groupHeaderElems [][]*larkdocx.TextElement
		var groupHeaderImages [][]string
		if hasHeader {
			groupHeader = extractColumns(headerContents, colIndices)
			groupHeaderElems = extractColumnElements(headerElements, colIndices)
			groupHeaderImages = extractColumnImages(headerImages, colIndices)
		}

		// 提取该列组的数据行
		groupDataRows := make([][]string, len(dataRows))
		groupDataRowElements := make([][][]*larkdocx.TextElement, len(dataRowElements))
		groupDataImages := make([][][]string, len(dataRowImages))
		for i, row := range dataRows {
			groupDataRows[i] = extractColumns(row, colIndices)
		}
		for i, rowElems := range dataRowElements {
			groupDataRowElements[i] = extractColumnElements(rowElems, colIndices)
		}
		for i, rowImgs := range dataRowImages {
			groupDataImages[i] = extractColumnImages(rowImgs, colIndices)
		}

		// 计算该列组的列宽：先把 pending(注释)/explicit(flag) 切到本组的索引，再走 resolveColumnWidths
		if splitPendingColWidth != nil {
			c.pendingColWidth = extractIntColumns(splitPendingColWidth, colIndices)
		}
		if splitExplicitColWidth != nil {
			c.options.ColumnWidthValues = extractIntColumns(splitExplicitColWidth, colIndices)
		}
		groupColWidths := c.resolveColumnWidths(groupHeader, groupDataRows, groupCols)

		// 对该列组执行行拆分
		results = append(results, buildRowSplitResults(groupCols, groupHeader, groupHeaderElems, groupDataRows, groupDataRowElements, groupColWidths, hasHeader, groupHeaderImages, groupDataImages)...)
	}

	return results
}

// inlineMathRegex 匹配行内公式 $...$，不匹配 $$ 或 \$
var inlineMathRegex = regexp.MustCompile(`(?:^|[^\\$])\$([^$\n]+?)\$`)

// isPlainTextRun 判断是否为完全无样式的纯文本元素（可安全合并）
func isPlainTextRun(elem *larkdocx.TextElement) bool {
	if elem == nil || elem.TextRun == nil || elem.TextRun.Content == nil {
		return false
	}
	if elem.TextRun.TextElementStyle == nil {
		return true
	}
	style := elem.TextRun.TextElementStyle
	if (style.Bold != nil && *style.Bold) ||
		(style.Italic != nil && *style.Italic) ||
		(style.Strikethrough != nil && *style.Strikethrough) ||
		(style.Underline != nil && *style.Underline) ||
		(style.InlineCode != nil && *style.InlineCode) ||
		style.Link != nil {
		return false
	}
	return true
}

// mergeAdjacentPlainTextRuns 合并相邻的无样式纯文本元素
// goldmark 的 GFM 扩展可能将连续文本拆分为多个 Text 节点，
// 合并后才能正确匹配跨节点的 $...$ 行内公式。
func mergeAdjacentPlainTextRuns(elements []*larkdocx.TextElement) []*larkdocx.TextElement {
	if len(elements) <= 1 {
		return elements
	}
	var merged []*larkdocx.TextElement
	for _, elem := range elements {
		if isPlainTextRun(elem) && len(merged) > 0 && isPlainTextRun(merged[len(merged)-1]) {
			combined := *merged[len(merged)-1].TextRun.Content + *elem.TextRun.Content
			merged[len(merged)-1].TextRun.Content = &combined
			continue
		}
		merged = append(merged, elem)
	}
	return merged
}

// splitInlineMath 将包含行内 $...$ 公式的文本元素拆分为文本+公式+文本
func splitInlineMath(elements []*larkdocx.TextElement) []*larkdocx.TextElement {
	// 先合并相邻纯文本元素，避免 goldmark 拆分导致 $...$ 被截断
	elements = mergeAdjacentPlainTextRuns(elements)

	var result []*larkdocx.TextElement
	for _, elem := range elements {
		if !isPlainTextRun(elem) {
			result = append(result, elem)
			continue
		}

		text := *elem.TextRun.Content
		// 查找所有 $...$ 匹配
		matches := inlineMathRegex.FindAllStringSubmatchIndex(text, -1)
		if len(matches) == 0 {
			result = append(result, elem)
			continue
		}

		pos := 0
		for _, match := range matches {
			// match[0]:match[1] 是完整匹配, match[2]:match[3] 是公式内容
			// 完整匹配可能包含前导字符（[^\\$] 消耗了一个字符）
			dollarStart := match[0]
			for dollarStart < match[1] && text[dollarStart] != '$' {
				dollarStart++
			}

			// 前导文本
			if dollarStart > pos {
				prefix := text[pos:dollarStart]
				result = append(result, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{Content: &prefix, TextElementStyle: elem.TextRun.TextElementStyle},
				})
			}

			// 公式内容
			formula := text[match[2]:match[3]]
			result = append(result, &larkdocx.TextElement{
				Equation: &larkdocx.Equation{Content: &formula},
			})

			pos = match[1]
		}

		// 剩余文本
		if pos < len(text) {
			remaining := text[pos:]
			result = append(result, &larkdocx.TextElement{
				TextRun: &larkdocx.TextRun{Content: &remaining, TextElementStyle: elem.TextRun.TextElementStyle},
			})
		}
	}
	return result
}

func (c *MarkdownToBlock) extractTextElements(node ast.Node) []*larkdocx.TextElement {
	var elements []*larkdocx.TextElement

	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch child := n.(type) {
		case *ast.Text:
			text := unescapeMarkdownText(string(child.Segment.Value(c.source)))
			if text != "" {
				elements = append(elements, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{
						Content: &text,
					},
				})
			}

		case *ast.String:
			text := string(child.Value)
			if text != "" {
				elements = append(elements, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{
						Content: &text,
					},
				})
			}

		case *ast.Emphasis:
			// 递归提取子元素，保留内部链接等信息，然后叠加粗体/斜体样式
			childElems := c.extractChildElements(child)
			bold := child.Level == 2
			italic := child.Level == 1
			for _, elem := range childElems {
				applyTextStyle(elem, bold, italic, false)
				elements = append(elements, elem)
			}
			return ast.WalkSkipChildren, nil

		case *ast.CodeSpan:
			text := c.getNodeText(child)
			if text != "" {
				inlineCode := true
				elements = append(elements, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{
						Content: &text,
						TextElementStyle: &larkdocx.TextElementStyle{
							InlineCode: &inlineCode,
						},
					},
				})
			}
			return ast.WalkSkipChildren, nil

		case *east.Strikethrough:
			// 递归提取子元素，保留内部链接等信息，然后叠加删除线样式
			childElems := c.extractChildElements(child)
			for _, elem := range childElems {
				applyTextStyle(elem, false, false, true)
				elements = append(elements, elem)
			}
			return ast.WalkSkipChildren, nil

		case *ast.Link:
			text := c.getNodeText(child)
			url := string(child.Destination)
			if text != "" {
				elements = append(elements, createLinkElement(text, url))
			}
			return ast.WalkSkipChildren, nil

		case *ast.AutoLink:
			linkURL := string(child.URL(c.source))
			label := string(child.Label(c.source))
			if label == "" {
				label = linkURL
			}
			if linkURL != "" {
				elements = append(elements, createLinkElement(label, linkURL))
			}
			return ast.WalkSkipChildren, nil

		case *ast.Image:
			// 内联图片：统一降级为「[图片: alt]」占位（与 extractChildElements / extractParagraphLines 一致）。
			elements = append(elements, c.imageInlinePlaceholder(child))
			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	})

	// 行内公式 $...$ 后处理
	elements = splitInlineMath(elements)

	return elements
}

func (c *MarkdownToBlock) getNodeText(node ast.Node) string {
	return c.getNodeTextWithDepth(node, 0)
}

func (c *MarkdownToBlock) getNodeTextWithDepth(node ast.Node, depth int) string {
	// 递归深度检查，防止栈溢出
	if depth > maxRecursionDepth {
		return "[递归深度超限]"
	}

	var buf bytes.Buffer
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Text:
			buf.Write(unescapeMarkdownBytes(n.Segment.Value(c.source)))
		case *ast.String:
			buf.Write(n.Value)
		case *ast.RawHTML:
			// 处理 <br> 标签为换行符
			var htmlBuf bytes.Buffer
			for i := 0; i < n.Segments.Len(); i++ {
				seg := n.Segments.At(i)
				htmlBuf.Write(c.source[seg.Start:seg.Stop])
			}
			raw := strings.TrimSpace(strings.ToLower(htmlBuf.String()))
			if raw == "<br>" || raw == "<br/>" || raw == "<br />" {
				buf.WriteString("\n")
			}
		default:
			buf.WriteString(c.getNodeTextWithDepth(child, depth+1))
		}
	}
	return buf.String()
}

// extractChildElements 递归提取子节点的 TextElement，保留链接等内联信息。
// 用于 Emphasis/Strikethrough 内部可能包含 Link 等节点的场景。
func (c *MarkdownToBlock) extractChildElements(node ast.Node) []*larkdocx.TextElement {
	var elements []*larkdocx.TextElement
	inUnderline := false // 跟踪 <u>...</u> 状态

	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Text:
			text := unescapeMarkdownText(string(n.Segment.Value(c.source)))
			if text != "" {
				elem := &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{Content: &text},
				}
				if inUnderline {
					underline := true
					if elem.TextRun.TextElementStyle == nil {
						elem.TextRun.TextElementStyle = &larkdocx.TextElementStyle{}
					}
					elem.TextRun.TextElementStyle.Underline = &underline
				}
				elements = append(elements, elem)
			}
		case *ast.String:
			text := string(n.Value)
			if text != "" {
				elem := &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{Content: &text},
				}
				if inUnderline {
					underline := true
					if elem.TextRun.TextElementStyle == nil {
						elem.TextRun.TextElementStyle = &larkdocx.TextElementStyle{}
					}
					elem.TextRun.TextElementStyle.Underline = &underline
				}
				elements = append(elements, elem)
			}
		case *ast.Link:
			text := c.getNodeText(n)
			url := string(n.Destination)
			if text != "" {
				elem := createLinkElement(text, url)
				if inUnderline && elem.TextRun != nil {
					underline := true
					if elem.TextRun.TextElementStyle == nil {
						elem.TextRun.TextElementStyle = &larkdocx.TextElementStyle{}
					}
					elem.TextRun.TextElementStyle.Underline = &underline
				}
				elements = append(elements, elem)
			}
		case *ast.CodeSpan:
			text := c.getNodeText(n)
			if text != "" {
				inlineCode := true
				elements = append(elements, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{
						Content:          &text,
						TextElementStyle: &larkdocx.TextElementStyle{InlineCode: &inlineCode},
					},
				})
			}
		case *ast.Emphasis:
			childElems := c.extractChildElements(n)
			bold := n.Level == 2
			italic := n.Level == 1
			for _, elem := range childElems {
				applyTextStyle(elem, bold, italic, false)
				if inUnderline && elem.TextRun != nil {
					underline := true
					if elem.TextRun.TextElementStyle == nil {
						elem.TextRun.TextElementStyle = &larkdocx.TextElementStyle{}
					}
					elem.TextRun.TextElementStyle.Underline = &underline
				}
				elements = append(elements, elem)
			}
		case *east.Strikethrough:
			childElems := c.extractChildElements(n)
			for _, elem := range childElems {
				applyTextStyle(elem, false, false, true)
				if inUnderline && elem.TextRun != nil {
					underline := true
					if elem.TextRun.TextElementStyle == nil {
						elem.TextRun.TextElementStyle = &larkdocx.TextElementStyle{}
					}
					elem.TextRun.TextElementStyle.Underline = &underline
				}
				elements = append(elements, elem)
			}
		case *ast.AutoLink:
			linkURL := string(n.URL(c.source))
			label := string(n.Label(c.source))
			if label == "" {
				label = linkURL
			}
			if linkURL != "" {
				elem := createLinkElement(label, linkURL)
				if inUnderline && elem.TextRun != nil {
					underline := true
					if elem.TextRun.TextElementStyle == nil {
						elem.TextRun.TextElementStyle = &larkdocx.TextElementStyle{}
					}
					elem.TextRun.TextElementStyle.Underline = &underline
				}
				elements = append(elements, elem)
			}
		case *ast.RawHTML:
			// 处理 HTML 标签
			var htmlBuf bytes.Buffer
			for i := 0; i < n.Segments.Len(); i++ {
				seg := n.Segments.At(i)
				htmlBuf.Write(c.source[seg.Start:seg.Stop])
			}
			rawOriginal := strings.TrimSpace(htmlBuf.String())
			raw := strings.ToLower(rawOriginal)
			switch {
			case raw == "<br>" || raw == "<br/>" || raw == "<br />":
				newline := "\n"
				elements = append(elements, &larkdocx.TextElement{
					TextRun: &larkdocx.TextRun{Content: &newline},
				})
			case raw == "<u>":
				inUnderline = true
			case raw == "</u>":
				inUnderline = false
			default:
				// 尝试解析自定义 HTML 标签（如 <mention-user/>, <mention-doc>...</mention-doc>）
				tag := ParseHTMLTag(rawOriginal)
				if tag != nil && tag.Name == "image" {
					// 行内 <image token=.../>（doc export 在表格单元格内输出的图片）：单元格嵌入场景收集为
					// feishu://media/<token> 交给导入层复用素材；其它场景降级为占位文本，不再静默丢弃。
					src := ""
					if token := strings.TrimSpace(tag.Attrs["token"]); token != "" {
						src = FeishuMediaScheme + token
					} else {
						src = strings.TrimSpace(tag.Attrs["url"]) // 本地路径 / http(s) / feishu://media/
					}
					if isEmbeddableImageDest(src) {
						if c.cellImageSink != nil && c.options.UploadImages {
							*c.cellImageSink = append(*c.cellImageSink, src)
						} else {
							placeholder := fmt.Sprintf("[图片: %s]", src)
							elements = append(elements, &larkdocx.TextElement{TextRun: &larkdocx.TextRun{Content: &placeholder}})
						}
					}
				} else if tag != nil {
					if elems := c.handleInlineHTMLTag(tag, &inUnderline); len(elems) > 0 {
						elements = append(elements, elems...)
					}
				}
			}
		case *ast.Image:
			dest := string(n.Destination)
			// 表格单元格真嵌入场景：收集可嵌入图片源，跳过占位文本（导入层会在单元格内建 Image 子块）。
			if c.cellImageSink != nil && c.options.UploadImages && isEmbeddableImageDest(dest) {
				*c.cellImageSink = append(*c.cellImageSink, dest)
				continue
			}
			// 其它场景（非单元格 / 关闭上传 / feishu:// 内部引用）：降级为占位文本或链接，避免静默丢失。
			// 非嵌入场景（非单元格 / 关闭上传 / feishu:// 内部引用）：降级为统一的「[图片: alt]」占位。
			elem := c.imageInlinePlaceholder(n)
			if inUnderline && elem.TextRun != nil {
				if elem.TextRun.TextElementStyle == nil {
					elem.TextRun.TextElementStyle = &larkdocx.TextElementStyle{}
				}
				underline := true
				elem.TextRun.TextElementStyle.Underline = &underline
			}
			elements = append(elements, elem)
		default:
			// 未知内联节点，递归提取子元素
			childElems := c.extractChildElements(child)
			if inUnderline {
				for _, elem := range childElems {
					if elem.TextRun != nil {
						underline := true
						if elem.TextRun.TextElementStyle == nil {
							elem.TextRun.TextElementStyle = &larkdocx.TextElementStyle{}
						}
						elem.TextRun.TextElementStyle.Underline = &underline
					}
				}
			}
			elements = append(elements, childElems...)
		}
	}
	return elements
}

// isEmbeddableImageDest 判断图片地址能否经图片管线上传嵌入：
// 空地址、飞书内部媒体引用（feishu://media/，token 绑定源文档不可跨文档复用）不可嵌入；
// 其余本地路径或 http(s) URL 均可（resolveImageSource 会下载 URL / 读取本地文件）。
func isEmbeddableImageDest(dest string) bool {
	if dest == "" {
		return false
	}
	if token, ok := strings.CutPrefix(dest, FeishuMediaScheme); ok {
		// feishu://media/<token>：导入层下载原素材后重新上传（素材复用），token 为空则不可嵌入
		return token != ""
	}
	return true
}

// extractCellElementsCollectingImages 提取表格单元格的富文本元素。
// EmbedTableImages 开启时，临时挂上 cellImageSink，把单元格内可嵌入图片源收集出来（同时 extractChildElements
// 会跳过这些图片的占位文本），返回 (元素, 图片源列表)；关闭时退化为普通 extractChildElements（图片走占位降级）。
func (c *MarkdownToBlock) extractCellElementsCollectingImages(node ast.Node) ([]*larkdocx.TextElement, []string) {
	if !c.options.EmbedTableImages {
		return splitCellInlineMath(c.extractChildElements(node)), nil
	}
	var sink []string
	prev := c.cellImageSink
	c.cellImageSink = &sink
	elems := c.extractChildElements(node)
	c.cellImageSink = prev
	return splitCellInlineMath(elems), sink
}

// splitCellInlineMath 对表格单元格元素做与正文一致的行内公式（$...$）转换。
// 单元格内 <br> 产生的独立 "\n" 元素是填充层拆分多段的分隔符，不能被 splitInlineMath 的相邻纯文本合并吞掉，
// 因此按 "\n" 分段分别转换后再用原分隔符拼回。
func splitCellInlineMath(elements []*larkdocx.TextElement) []*larkdocx.TextElement {
	isBreak := func(e *larkdocx.TextElement) bool {
		return e != nil && e.TextRun != nil && e.TextRun.Content != nil && *e.TextRun.Content == "\n"
	}
	var out, seg []*larkdocx.TextElement
	flush := func() {
		if len(seg) > 0 {
			out = append(out, splitInlineMath(seg)...)
			seg = nil
		}
	}
	for _, e := range elements {
		if isBreak(e) {
			flush()
			out = append(out, e)
			continue
		}
		seg = append(seg, e)
	}
	flush()
	return out
}

// cellFallbackContent 返回单元格的纯文本，用于列宽计算 + 富文本为空时的兜底填充。
// 纯图片单元格（富文本被收集进 imgs 后 elems 为空）返回 ""，避免 getNodeText 的图片 alt
// 经填充兜底路径泄漏成单元格里多余的标题文字（issue #164 跟进）。
func (c *MarkdownToBlock) cellFallbackContent(node ast.Node, elems []*larkdocx.TextElement, imgs []string) string {
	if len(elems) == 0 && len(imgs) > 0 {
		return ""
	}
	return c.getNodeText(node)
}

// imageInlinePlaceholder 把内联图片降级为统一的占位 TextElement，是 extractChildElements /
// extractParagraphLines / extractTextElements 三处内联提取器共用的单一实现：
//   - http(s) 地址：用链接元素「[图片: alt]」保留可点击地址；
//   - 本地路径 / 其它：纯文本「[图片: alt]」（alt 缺省回退到 dest）。
//
// 统一中文前缀「[图片:」并优先用 alt（而非原始路径），避免三处各写一份、格式不一致或泄漏本地路径。
// 下划线等上下文样式由各调用点按自身状态叠加。
func (c *MarkdownToBlock) imageInlinePlaceholder(node *ast.Image) *larkdocx.TextElement {
	dest := string(node.Destination)
	alt := c.getNodeText(node)
	if alt == "" {
		alt = dest
	}
	if hasValidURLPrefix(dest) {
		return createLinkElement(fmt.Sprintf("[图片: %s]", alt), dest)
	}
	placeholder := fmt.Sprintf("[图片: %s]", alt)
	return &larkdocx.TextElement{TextRun: &larkdocx.TextRun{Content: &placeholder}}
}

// extractColumnImages 从一行单元格图片源中按列索引取子集（与 extractColumnElements 同形，用于列拆分）。
func extractColumnImages(rowImages [][]string, colIndices []int) [][]string {
	result := make([][]string, len(colIndices))
	for i, idx := range colIndices {
		if idx < len(rowImages) {
			result[i] = rowImages[idx]
		}
	}
	return result
}

// applyTextStyle 向 TextElement 叠加样式（不覆盖已有样式）
func applyTextStyle(elem *larkdocx.TextElement, bold, italic, strikethrough bool) {
	if elem == nil || elem.TextRun == nil {
		return
	}
	if elem.TextRun.TextElementStyle == nil {
		elem.TextRun.TextElementStyle = &larkdocx.TextElementStyle{}
	}
	s := elem.TextRun.TextElementStyle
	if bold {
		s.Bold = &bold
	}
	if italic {
		s.Italic = &italic
	}
	if strikethrough {
		s.Strikethrough = &strikethrough
	}
}

// normalizeURL 尝试标准化 URL
// 1. 将 feishu:// 内部协议转换为 https:// 链接（API 不接受 feishu:// 协议）
// 2. 解码 URL 编码的链接，例如 "https%3A%2F%2Fexample.com" → "https://example.com"
func normalizeURL(rawURL string) string {
	// feishu:// 内部协议转换为 HTTPS 链接（按当前品牌选择 www.feishu.cn / www.larksuite.com，
	// 由服务端重定向到租户域名；Lark 租户不能落到飞书国内站）
	urlBase := client.ResourceURLBase()
	if strings.HasPrefix(rawURL, "feishu://doc/") {
		return urlBase + "/docx/" + strings.TrimPrefix(rawURL, "feishu://doc/")
	}
	if strings.HasPrefix(rawURL, "feishu://wiki/") {
		return urlBase + "/wiki/" + strings.TrimPrefix(rawURL, "feishu://wiki/")
	}
	if strings.HasPrefix(rawURL, "feishu://") {
		// 其他 feishu:// 链接，尝试通用转换
		return urlBase + "/" + strings.TrimPrefix(rawURL, "feishu://")
	}

	// URL 解码：用 PathUnescape 而非 QueryUnescape，避免 query 中字面 `+` 被错误解码为空格
	// （RFC 3986：`+` 在 URL query 中是字面字符；form-urlencoded 才把 `+` 当空格）。
	// 与 block_to_markdown.go 的导出方向对称，保证 round-trip 不退化。
	if decoded, err := url.PathUnescape(rawURL); err == nil && decoded != rawURL {
		return decoded
	}
	return rawURL
}

// hasValidURLPrefix 检查 URL 是否以支持的协议开头
func hasValidURLPrefix(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

// createLinkElement 创建链接 TextElement，自动过滤无效 URL 并解码 URL 编码的链接
func createLinkElement(text, rawURL string) *larkdocx.TextElement {
	u := normalizeURL(rawURL)
	if !hasValidURLPrefix(u) {
		return &larkdocx.TextElement{
			TextRun: &larkdocx.TextRun{Content: &text},
		}
	}
	return &larkdocx.TextElement{
		TextRun: &larkdocx.TextRun{
			Content: &text,
			TextElementStyle: &larkdocx.TextElementStyle{
				Link: &larkdocx.Link{Url: &u},
			},
		},
	}
}

// hasNonEmptyContent checks if text elements have non-empty content
func hasNonEmptyContent(elements []*larkdocx.TextElement) bool {
	for _, e := range elements {
		if e.TextRun != nil && e.TextRun.Content != nil {
			content := strings.TrimSpace(*e.TextRun.Content)
			if content != "" {
				return true
			}
		}
	}
	return false
}

// getHTMLBlockText 从 ast.HTMLBlock 节点提取原始 HTML 文本。
//
// 注意 goldmark 的 ast.HTMLBlock.Lines() 在多行 HTML 注释场景下经常只返回首行
// （例如 `<!-- feishu-colwidth: 80,200\n-->` 只能拿到 `<!-- feishu-colwidth: 80,200`），
// 直接用 node.Lines() 拼接会丢掉闭合标签。这里先用 node.Lines() 拿到首行 Start，
// 再扫源码到 `-->` 闭合标记，确保多行注释被完整还原。
func (c *MarkdownToBlock) getHTMLBlockText(node *ast.HTMLBlock) string {
	lines := node.Lines()
	if lines.Len() == 0 {
		return ""
	}
	first := lines.At(0)
	last := lines.At(lines.Len() - 1)

	// 累积 Lines() 给出的所有行（在普通情况下就够用）
	var buf bytes.Buffer
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		buf.Write(seg.Value(c.source))
	}
	rawJoined := buf.String()

	// 仅对 HTML 注释做"扩展闭合"：首行有 `<!--` 但累积文本里没 `-->` → 沿源码向后查找 `-->`。
	// 其他类型的 HTMLBlock（标签型）不做扩展，避免误把后续内容卷进来。
	if strings.Contains(rawJoined, "<!--") && !strings.Contains(rawJoined, "-->") {
		closingIdx := strings.Index(string(c.source[last.Stop:]), "-->")
		if closingIdx >= 0 {
			extEnd := last.Stop + closingIdx + len("-->")
			extended := string(c.source[first.Start:extEnd])
			return strings.TrimSpace(extended)
		}
	}
	return strings.TrimSpace(rawJoined)
}

// handleInlineHTMLTag 处理行内 HTML 标签，返回对应的 TextElement 列表
// 支持 <mention-user id="ou_xxx"/>, <mention-doc token="xxx" type="docx">标题</mention-doc>
func (c *MarkdownToBlock) handleInlineHTMLTag(tag *HTMLTag, inUnderline *bool) []*larkdocx.TextElement {
	var elements []*larkdocx.TextElement

	switch tag.Name {
	case "mention-user":
		userID := tag.Attrs["id"]
		if userID != "" {
			elements = append(elements, &larkdocx.TextElement{
				MentionUser: &larkdocx.MentionUser{
					UserId: &userID,
				},
			})
		}

	case "mention-doc":
		token := tag.Attrs["token"]
		docType := tag.Attrs["type"]
		title := tag.Content
		if token != "" {
			objType := mapDocTypeToObjType(docType)
			elements = append(elements, &larkdocx.TextElement{
				MentionDoc: &larkdocx.MentionDoc{
					Token:   &token,
					ObjType: &objType,
					Title:   &title,
				},
			})
		}
	}

	return elements
}

// handleBlockHTMLTag 处理块级 HTML 标签，返回对应的 BlockNode 列表
// 支持 <image/>, <callout>, <grid>, <whiteboard/>, <sheet/>, <bitable/>, <file/>
func (c *MarkdownToBlock) handleBlockHTMLTag(tag *HTMLTag) []*BlockNode {
	switch tag.Name {
	case blockEquationHTMLTag:
		if formula, ok := decodeBlockEquationHTML(tag); ok {
			return []*BlockNode{{Block: createTextEquationBlock(formula)}}
		}
		return nil
	case "image":
		return c.handleHTMLImageBlock(tag)
	case "callout":
		return c.handleHTMLCalloutBlock(tag)
	case "grid":
		return c.handleHTMLGridBlock(tag)
	case "whiteboard":
		return c.handleHTMLWhiteboardBlock(tag)
	case "sheet":
		return c.handleHTMLSheetBlock(tag)
	case "bitable":
		return c.handleHTMLBitableBlock(tag)
	case "file":
		return c.handleHTMLFileBlock(tag)
	case "video":
		return c.handleHTMLVideoBlock(tag)
	}
	return nil
}

// handleHTMLImageBlock 处理块级 <image token="..." width="800" height="600" align="center"/>
func (c *MarkdownToBlock) handleHTMLImageBlock(tag *HTMLTag) []*BlockNode {
	token := tag.Attrs["token"]
	imgURL := tag.Attrs["url"]
	width := parseHTMLIntAttr(tag.Attrs["width"])
	height := parseHTMLIntAttr(tag.Attrs["height"])
	align := parseHTMLAlignAttr(tag.Attrs["align"])

	// 有 token 时（doc export 的 roundtrip 输出）：建块接口拒绝带 token 的 Image（1770001），
	// 改为建空块 + 素材复用（下载原素材 → 上传到新块 → replace_image，保留原显示宽高/对齐）。
	if token != "" {
		return []*BlockNode{{Block: c.newTokenImageBlock(token, width, height, align)}}
	}

	// 有 url 时按照图片上传流程处理
	if imgURL != "" {
		if mediaToken, ok := strings.CutPrefix(imgURL, FeishuMediaScheme); ok {
			return []*BlockNode{{Block: c.newTokenImageBlock(mediaToken, width, height, align)}}
		}
		if !c.options.UploadImages {
			c.imageStats.Skipped++
			return []*BlockNode{{Block: c.createImagePlaceholder(imgURL)}}
		}
		c.imageStats.Total++
		c.imageSources = append(c.imageSources, imgURL)
		block := newEmptyImageBlock()
		c.addMediaRef(block, &MediaRef{Kind: MediaKindImage, Source: imgURL, Width: width, Height: height, Align: align})
		return []*BlockNode{{Block: block}}
	}

	return nil
}

// handleHTMLCalloutBlock 处理块级 <callout type="NOTE" color="6">内容</callout>
// Phase 2: 基本框架，内容作为纯文本处理；Phase 3 将支持递归 Markdown 转换
func (c *MarkdownToBlock) handleHTMLCalloutBlock(tag *HTMLTag) []*BlockNode {
	// 确定背景色：color 为飞书背景色枚举原值（1-14），type 按 CalloutColorForType 映射
	bgColor := CalloutColorForType(tag.Attrs["type"])
	if colorStr := tag.Attrs["color"]; colorStr != "" {
		if v := parseHTMLIntAttr(colorStr); v >= 1 && v <= 14 {
			bgColor = v
		}
	}

	blockType := int(BlockTypeCallout)
	calloutBlock := &larkdocx.Block{
		BlockType: &blockType,
		Callout: &larkdocx.Callout{
			BackgroundColor: &bgColor,
		},
	}

	// 将内容作为文本子块
	var children []*BlockNode
	content := strings.TrimSpace(tag.Content)
	if content != "" {
		textBlockType := int(BlockTypeText)
		children = append(children, &BlockNode{
			Block: &larkdocx.Block{
				BlockType: &textBlockType,
				Text: &larkdocx.Text{
					Elements: []*larkdocx.TextElement{
						{TextRun: &larkdocx.TextRun{Content: &content}},
					},
				},
			},
		})
	}

	return []*BlockNode{{Block: calloutBlock, Children: children}}
}

// 分栏列数的服务端取值范围（grid.column_size，实测 min 2 / max 5）。
const (
	gridMinColumns = 2
	gridMaxColumns = 5
)

// handleHTMLGridBlock 处理块级 <grid cols="2"><column>左栏</column><column>右栏</column></grid>
// 创建 Grid Block (type=24) + GridColumn 子块 (type=25)，每个 column 内容递归转换为 BlockNode
func (c *MarkdownToBlock) handleHTMLGridBlock(tag *HTMLTag) []*BlockNode {
	// 跨多行的 <grid> 已被预处理折叠为 data-base64（见 rewriteMultilineGridBlocks）
	content := tag.Content
	if encoded := tag.Attrs["data-base64"]; encoded != "" {
		if decoded, err := base64.StdEncoding.DecodeString(encoded); err == nil {
			content = string(decoded)
		}
	}

	// 解析 <column>...</column> 内容
	columnContents := ParseGridColumns(content)

	// 如果没有 <column> 标签但有内容，将全部内容作为单列
	if len(columnContents) == 0 && strings.TrimSpace(content) != "" {
		columnContents = []string{strings.TrimSpace(content)}
	}

	// 列数：服务端 column_size 只接受 2-5（实测 1 / 6 均报 99992402）。
	// <column> 多于 cols 属性时按实际列数建（不再丢弃多出的列内容）；超过 5 列时多出的列内容并入最后一列。
	cols := parseHTMLIntAttrDefault(tag.Attrs["cols"], 2)
	if len(columnContents) > cols {
		cols = len(columnContents)
	}
	if cols < gridMinColumns {
		cols = gridMinColumns
	}
	if cols > gridMaxColumns {
		cols = gridMaxColumns
	}
	if len(columnContents) > cols {
		merged := strings.Join(columnContents[cols-1:], "\n\n")
		columnContents = append(columnContents[:cols-1:cols-1], merged)
	}

	// 创建 Grid Block。注意：GridColumn 子块由服务端按 column_size 自动生成（显式创建 grid_column
	// 子块会被拒绝：1770028 block not support create children），导入层把下面每列的子块写入对应的服务端列。
	gridBlockType := int(BlockTypeGrid)
	gridBlock := &larkdocx.Block{
		BlockType: &gridBlockType,
		Grid: &larkdocx.Grid{
			ColumnSize: &cols,
		},
	}

	// 创建 GridColumn 子块
	var gridChildren []*BlockNode
	for i := 0; i < cols; i++ {
		colBlockType := int(BlockTypeGridColumn)
		// 默认等宽
		widthRatio := 100 / cols
		colBlock := &larkdocx.Block{
			BlockType:  &colBlockType,
			GridColumn: &larkdocx.GridColumn{WidthRatio: &widthRatio},
		}

		var colChildren []*BlockNode
		if i < len(columnContents) && columnContents[i] != "" {
			// 递归转换 column 内容为 BlockNode
			colChildren = c.convertInnerMarkdown(columnContents[i])
		}

		// 如果没有子块，创建空文本子块
		if len(colChildren) == 0 {
			textBlockType := int(BlockTypeText)
			empty := ""
			colChildren = []*BlockNode{{
				Block: &larkdocx.Block{
					BlockType: &textBlockType,
					Text: &larkdocx.Text{
						Elements: []*larkdocx.TextElement{
							{TextRun: &larkdocx.TextRun{Content: &empty}},
						},
					},
				},
			}}
		}

		gridChildren = append(gridChildren, &BlockNode{Block: colBlock, Children: colChildren})
	}

	return []*BlockNode{{Block: gridBlock, Children: gridChildren}}
}

// convertInnerMarkdown 将内嵌的 Markdown 文本递归转换为 BlockNode 列表
// 用于 <grid><column> 中的 Markdown 内容
func (c *MarkdownToBlock) convertInnerMarkdown(markdown string) []*BlockNode {
	inner := NewMarkdownToBlock([]byte(markdown), c.options, c.basePath)
	result, err := inner.ConvertWithTableData()
	if err != nil || result == nil {
		return nil
	}
	// 合并 inner 的图片统计和来源
	c.imageStats.Total += inner.imageStats.Total
	c.imageStats.Skipped += inner.imageStats.Skipped
	c.imageSources = append(c.imageSources, inner.imageSources...)
	c.videoStats.Total += inner.videoStats.Total
	c.videoStats.Skipped += inner.videoStats.Skipped
	c.videoSources = append(c.videoSources, inner.videoSources...)
	c.fileStats.Total += inner.fileStats.Total
	c.fileStats.Skipped += inner.fileStats.Skipped
	for block, ref := range inner.mediaRefs {
		c.addMediaRef(block, ref)
	}
	c.degradations = append(c.degradations, inner.degradations...)
	for block, td := range inner.tableDataByBlock {
		c.addTableData(block, td)
	}
	return result.BlockNodes
}

// handleHTMLWhiteboardBlock 处理 <whiteboard type="blank"/> → Board Block (type=43)
func (c *MarkdownToBlock) handleHTMLWhiteboardBlock(tag *HTMLTag) []*BlockNode {
	blockType := int(BlockTypeBoard)
	block := &larkdocx.Block{
		BlockType: &blockType,
		Board:     &larkdocx.Board{},
	}

	// 带 token（doc export 的 roundtrip 输出）：建块接口拒绝带 token 的 Board（1770001），
	// 画板 token 由服务端生成。这里只建空画板，登记源画板 token，由导入层复制源画板节点。
	if token := strings.TrimSpace(tag.Attrs["token"]); token != "" {
		c.addMediaRef(block, &MediaRef{Kind: MediaKindWhiteboard, Token: token})
	}

	return []*BlockNode{{Block: block}}
}

// handleHTMLSheetBlock 处理 <sheet rows="5" cols="5"/> → Sheet Block (type=30)
func (c *MarkdownToBlock) handleHTMLSheetBlock(tag *HTMLTag) []*BlockNode {
	rows := parseHTMLIntAttrDefault(tag.Attrs["rows"], 3)
	cols := parseHTMLIntAttrDefault(tag.Attrs["cols"], 3)

	// 带 token（引用已有电子表格）：建块接口拒绝带 token 的 Sheet（1770001），也无法把已有表格挂到新文档。
	// 降级为指向原表格的链接文本并登记 Degradation（导入层计入 failures），不让整篇导入失败。
	if token := strings.TrimSpace(tag.Attrs["token"]); token != "" {
		sheetURL := client.BuildResourceURL(client.ResourceTypeSheet, token)
		ref := token
		if id := strings.TrimSpace(tag.Attrs["id"]); id != "" {
			sheetURL += "?sheet=" + url.QueryEscape(id)
			ref = token + "_" + id
		}
		c.addDegradation("sheet", ref, "建块接口不支持引用已有电子表格（1770001），已降级为原表格链接；需要内嵌数据请用 doc export 默认的 --expand-sheets 导出为 Markdown 表格")
		return []*BlockNode{{Block: newTextBlock(createLinkElement("[电子表格: "+ref+"]", sheetURL))}}
	}

	blockType := int(BlockTypeSheet)
	sheet := &larkdocx.Sheet{
		RowSize:    &rows,
		ColumnSize: &cols,
	}

	return []*BlockNode{{Block: &larkdocx.Block{
		BlockType: &blockType,
		Sheet:     sheet,
	}}}
}

// handleHTMLBitableBlock 处理 <bitable view="table"/> → Bitable Block (type=18)
func (c *MarkdownToBlock) handleHTMLBitableBlock(tag *HTMLTag) []*BlockNode {
	viewType := 1 // 默认数据表视图
	switch strings.ToLower(tag.Attrs["view"]) {
	case "kanban":
		viewType = 2
	case "calendar":
		viewType = 3
	case "gallery":
		viewType = 4
	case "gantt":
		viewType = 5
	case "form":
		viewType = 6
	}

	// 带 token（引用已有多维表格）：建块接口拒绝带 token 的 Bitable（1770001）。
	// 降级为指向原多维表格的链接文本并登记 Degradation，不让整篇导入失败。
	if token := strings.TrimSpace(tag.Attrs["token"]); token != "" {
		appToken, tableID, _ := strings.Cut(token, "_")
		baseURL := client.BuildResourceURL(client.ResourceTypeBitable, appToken)
		if tableID != "" {
			baseURL += "?table=" + url.QueryEscape(tableID)
		}
		c.addDegradation("bitable", token, "建块接口不支持引用已有多维表格（1770001），已降级为原多维表格链接")
		return []*BlockNode{{Block: newTextBlock(createLinkElement("[多维表格: "+token+"]", baseURL))}}
	}

	blockType := int(BlockTypeBitable)
	bitable := &larkdocx.Bitable{
		ViewType: &viewType,
	}

	return []*BlockNode{{Block: &larkdocx.Block{
		BlockType: &blockType,
		Bitable:   bitable,
	}}}
}

// handleHTMLFileBlock 处理 <file token="..." name="..." view-type="1"/> → File Block (type=23)
func (c *MarkdownToBlock) handleHTMLFileBlock(tag *HTMLTag) []*BlockNode {
	token := strings.TrimSpace(tag.Attrs["token"])
	name := strings.TrimSpace(tag.Attrs["name"])
	if token == "" && name == "" {
		return nil
	}
	label := name
	if label == "" {
		label = FeishuMediaScheme + token
	}

	// 只有文件名、没有素材 token：没有可上传/复用的内容，降级为占位文本并登记。
	if token == "" {
		c.fileStats.Skipped++
		c.addDegradation("file", name, "<file> 标签缺少 token，无可导入的附件内容，已降级为占位文本")
		return []*BlockNode{{Block: c.createMediaPlaceholder("File", label)}}
	}
	if !c.options.UploadImages {
		c.fileStats.Skipped++
		return []*BlockNode{{Block: c.createMediaPlaceholder("File", label)}}
	}

	// 建块接口只接受 {"token":""} 的空 File 块（带 token 或 name 均 1770001），
	// 由导入层下载原素材后上传到新 File 块并 replace_file（附件复用）。
	c.fileStats.Total++
	block := newEmptyFileBlock(parseHTMLIntAttr(tag.Attrs["view-type"]))
	c.addMediaRef(block, &MediaRef{Kind: MediaKindFile, Token: token, Name: name, Video: IsVideoFilename(name)})
	return []*BlockNode{{Block: block}}
}

// newEmptyFileBlock 构造可被建块接口接受的空 File 块（token 必须为空字符串，不能带 name）。
// 服务端返回的是外层 View 块（block_type=33），其 children[0] 才是 File 块；viewType>0 时写入 View 的展示方式。
func newEmptyFileBlock(viewType int) *larkdocx.Block {
	blockType := int(BlockTypeFile)
	empty := ""
	file := &larkdocx.File{Token: &empty}
	if viewType > 0 {
		file.ViewType = &viewType
	}
	return &larkdocx.Block{BlockType: &blockType, File: file}
}

// handleHTMLVideoBlock 处理 <video src="./demo.mp4" controls></video> → File Block (type=23)
func (c *MarkdownToBlock) handleHTMLVideoBlock(tag *HTMLTag) []*BlockNode {
	src := strings.TrimSpace(tag.Attrs["src"])
	if src == "" {
		return nil
	}

	name := strings.TrimSpace(tag.Attrs["data-name"])
	if name == "" {
		name = strings.TrimSpace(tag.Attrs["name"])
	}
	viewType := parseHTMLIntAttrDefault(tag.Attrs["data-view-type"], 0)
	if viewType <= 0 {
		viewType = parseHTMLIntAttrDefault(tag.Attrs["view-type"], 2)
	}
	if viewType <= 0 {
		viewType = 2
	}

	if token, ok := strings.CutPrefix(src, FeishuMediaScheme); ok {
		if token == "" {
			return nil
		}
		if !c.options.UploadImages {
			c.videoStats.Skipped++
			return []*BlockNode{{Block: c.createMediaPlaceholder("Video", src)}}
		}
		// 视频素材 token：同附件一样建空 File 块，由导入层下载原素材后重新上传（建块不能带 token/name，1770001）。
		c.videoStats.Total++
		block := newEmptyFileBlock(viewType)
		c.addMediaRef(block, &MediaRef{Kind: MediaKindFile, Token: token, Name: name, Video: true})
		return []*BlockNode{{Block: block}}
	}

	if !c.options.UploadImages {
		c.videoStats.Skipped++
		return []*BlockNode{{Block: c.createMediaPlaceholder("Video", src)}}
	}

	if name == "." || name == string(filepath.Separator) || name == "" {
		name = filepath.Base(src)
		if name == "." || name == string(filepath.Separator) || name == "" {
			name = "video.mp4"
		}
	}

	c.videoStats.Total++
	c.videoSources = append(c.videoSources, src)
	block := newEmptyFileBlock(viewType)
	c.addMediaRef(block, &MediaRef{Kind: MediaKindFile, Source: src, Name: name, Video: true})
	return []*BlockNode{{Block: block}}
}

// parseHTMLIntAttrDefault 解析 HTML 属性中的整数值，失败返回 defaultVal
func parseHTMLIntAttrDefault(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	var v int
	if _, err := fmt.Sscanf(s, "%d", &v); err != nil {
		return defaultVal
	}
	return v
}

// tryConvertHTMLImageParagraph 检查段落是否只包含一个 <image> HTML 标签
// 如果是，转换为 Image Block；否则返回 nil
func (c *MarkdownToBlock) tryConvertHTMLImageParagraph(node *ast.Paragraph) *larkdocx.Block {
	// 检查段落子节点：可能是一个或多个 RawHTML 节点组成 <image ... />
	var htmlBuf bytes.Buffer
	onlyHTML := true
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if raw, ok := child.(*ast.RawHTML); ok {
			for i := 0; i < raw.Segments.Len(); i++ {
				seg := raw.Segments.At(i)
				htmlBuf.Write(c.source[seg.Start:seg.Stop])
			}
		} else {
			onlyHTML = false
			break
		}
	}

	if !onlyHTML || htmlBuf.Len() == 0 {
		return nil
	}

	rawStr := strings.TrimSpace(htmlBuf.String())
	if !IsHTMLTag(rawStr, "image") {
		return nil
	}

	tag := ParseHTMLTag(rawStr)
	if tag == nil {
		return nil
	}

	nodes := c.handleHTMLImageBlock(tag)
	if len(nodes) > 0 {
		return nodes[0].Block
	}
	return nil
}

// tryConvertHTMLVideoParagraph 检查段落是否只包含一个 <video> HTML 标签
func (c *MarkdownToBlock) tryConvertHTMLVideoParagraph(node *ast.Paragraph) *larkdocx.Block {
	var htmlBuf bytes.Buffer
	onlyHTML := true
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if raw, ok := child.(*ast.RawHTML); ok {
			for i := 0; i < raw.Segments.Len(); i++ {
				seg := raw.Segments.At(i)
				htmlBuf.Write(c.source[seg.Start:seg.Stop])
			}
		} else {
			onlyHTML = false
			break
		}
	}

	if !onlyHTML || htmlBuf.Len() == 0 {
		return nil
	}

	rawStr := strings.TrimSpace(htmlBuf.String())
	if !IsHTMLTag(rawStr, "video") {
		return nil
	}

	tag := ParseHTMLTag(rawStr)
	if tag == nil {
		return nil
	}

	nodes := c.handleHTMLVideoBlock(tag)
	if len(nodes) > 0 {
		return nodes[0].Block
	}
	return nil
}

func (c *MarkdownToBlock) createMediaPlaceholder(kind, ref string) *larkdocx.Block {
	text := fmt.Sprintf("[%s: %s]", kind, ref)
	blockType := int(BlockTypeText)
	return &larkdocx.Block{
		BlockType: &blockType,
		Text: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{{
				TextRun: &larkdocx.TextRun{Content: &text},
			}},
		},
	}
}

// parseHTMLIntAttr 解析 HTML 属性中的整数值，失败返回 0
func parseHTMLIntAttr(s string) int {
	if s == "" {
		return 0
	}
	var v int
	fmt.Sscanf(s, "%d", &v)
	return v
}

// parseHTMLAlignAttr 将对齐字符串映射为飞书整数 (1=left, 2=center, 3=right)
func parseHTMLAlignAttr(s string) int {
	switch strings.ToLower(s) {
	case "left":
		return 1
	case "center":
		return 2
	case "right":
		return 3
	default:
		return 0
	}
}

// languageNameToCode converts language name to Feishu language code
func languageNameToCode(name string) int {
	languages := map[string]int{
		"plaintext":    1,
		"abap":         2,
		"ada":          3,
		"apache":       4,
		"apex":         5,
		"assembly":     6,
		"bash":         7,
		"sh":           7,
		"shell":        57,
		"csharp":       8,
		"cs":           8,
		"cpp":          9,
		"c++":          9,
		"c":            10,
		"cobol":        11,
		"css":          12,
		"coffeescript": 13,
		"coffee":       13,
		"d":            14,
		"dart":         15,
		"delphi":       16,
		"django":       17,
		"dockerfile":   18,
		"docker":       18,
		"erlang":       19,
		"fortran":      20,
		"foxpro":       21,
		"go":           22,
		"golang":       22,
		"groovy":       23,
		"html":         24,
		"htmlbars":     25,
		"http":         26,
		"haskell":      27,
		"json":         28,
		"java":         29,
		"javascript":   30,
		"js":           30,
		"julia":        31,
		"kotlin":       32,
		"kt":           32,
		"latex":        33,
		"tex":          33,
		"lisp":         34,
		"lua":          35,
		"matlab":       36,
		"makefile":     37,
		"make":         37,
		"markdown":     38,
		"md":           38,
		"nginx":        39,
		"objectivec":   40,
		"objc":         40,
		"openedgeabl":  41,
		"php":          42,
		"perl":         43,
		"powershell":   44,
		"ps1":          44,
		"prolog":       45,
		"protobuf":     46,
		"proto":        46,
		"python":       47,
		"py":           47,
		"r":            48,
		"rpm":          49,
		"ruby":         50,
		"rb":           50,
		"rust":         51,
		"rs":           51,
		"sas":          52,
		"scss":         53,
		"sql":          54,
		"scala":        55,
		"scheme":       56,
		"swift":        58,
		"thrift":       59,
		"typescript":   60,
		"ts":           60,
		"vbscript":     61,
		"verilog":      62,
		"vhdl":         63,
		"visualbasic":  64,
		"vb":           64,
		"xml":          65,
		"yaml":         66,
		"yml":          66,
	}

	if code, ok := languages[strings.ToLower(name)]; ok {
		return code
	}
	return 1 // plaintext
}
