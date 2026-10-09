package cmd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// 邮件正文解码与展示。
//
// 飞书 Mail OpenAPI 的 body_plain_text / body_html / body_preview 字段是 base64url 编码
// （实测有的带 padding、有的不带）。读命令与 reply/forward 引用原文前必须先解码，
// 否则输出与引用块都是一串 base64。解码后的纯文本来自外部邮件，属于不可信输入：
// 终端展示前清除 ANSI 转义、裸 CR、C0/C1 控制字符与 BiDi/零宽字符，防止终端注入。

// mailBodyFields 需要 base64url 解码的正文字段。
var mailBodyFields = []string{"body_plain_text", "body_html", "body_preview"}

// decodeMailBase64URL 解码 base64url（兼容带/不带 padding，也兼容标准 base64）。
// 解码失败时原样返回，避免把本来就是明文的值（服务端行为变化或 mock）弄丢。
func decodeMailBase64URL(s string) string {
	if s == "" {
		return ""
	}
	for _, enc := range []*base64.Encoding{base64.URLEncoding, base64.RawURLEncoding, base64.StdEncoding, base64.RawStdEncoding} {
		// 解码结果不是合法 UTF-8 时视为"本来就不是 base64"，继续尝试/原样返回
		if b, err := enc.DecodeString(s); err == nil && utf8.Valid(b) {
			return string(b)
		}
	}
	return s
}

// mailANSIEscapeRe 匹配 ANSI CSI（ESC [ ... final）与 OSC（ESC ] ... BEL/ST）序列。
var mailANSIEscapeRe = regexp.MustCompile("\x1b\\[[0-?]*[ -/]*[@-~]|\x1b\\][^\x07\x1b]*(?:\x07|\x1b\\\\)")

// isMailDangerousRune 判断是否为终端展示时必须剔除的字符：
// C0 控制字符（保留 \n、\t）、DEL、C1 控制字符、BiDi 覆盖/隔离、零宽字符、行/段分隔符、BOM。
func isMailDangerousRune(r rune) bool {
	switch {
	case r == '\n' || r == '\t':
		return false
	case r < 0x20 || r == 0x7f:
		return true
	case r >= 0x80 && r <= 0x9f:
		return true
	case r >= 0x200B && r <= 0x200D: // ZWSP / ZWNJ / ZWJ
		return true
	case r == 0xFEFF: // BOM / ZWNBSP
		return true
	case r >= 0x202A && r <= 0x202E: // BiDi: LRE/RLE/PDF/LRO/RLO
		return true
	case r >= 0x2028 && r <= 0x2029: // LS / PS
		return true
	case r >= 0x2066 && r <= 0x2069: // LRI/RLI/FSI/PDI
		return true
	}
	return false
}

// sanitizeMailText 清理来自邮件的不可信文本，用于终端展示与纯文本引用。
// 保留换行与制表符，CRLF 统一为 LF。
func sanitizeMailText(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = mailANSIEscapeRe.ReplaceAllString(s, "")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if isMailDangerousRune(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// sanitizeMailSingleLine 在 sanitizeMailText 基础上去掉换行，用于主题、显示名等单行字段。
func sanitizeMailSingleLine(s string) string {
	s = sanitizeMailText(s)
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.ReplaceAll(s, "\t", " ")
}

// decodeMailMessageBodies 就地解码单封邮件对象中的正文字段。
// keepRaw=true 时保持 API 原始 base64url 值（--raw-body 兼容旧行为）。
// body_plain_text 解码后额外做终端安全清理；为空时回落到 body_preview（与官方一致）。
func decodeMailMessageBodies(msg map[string]any, keepRaw bool) {
	if msg == nil || keepRaw {
		return
	}
	for _, field := range mailBodyFields {
		if s, ok := msg[field].(string); ok && s != "" {
			msg[field] = decodeMailBase64URL(s)
		}
	}
	plain, _ := msg["body_plain_text"].(string)
	if plain == "" {
		if preview, _ := msg["body_preview"].(string); preview != "" {
			plain = preview
		}
	}
	if _, has := msg["body_plain_text"]; has || plain != "" {
		msg["body_plain_text"] = sanitizeMailText(plain)
	}
}

// decodeMailPayloadBodies 解析 mail message/messages/thread 的 data，并就地解码所有邮件正文。
// 兼容三种形态：{"message":{...}}、{"messages":[...]}、{"thread":{"messages":[...]}}，
// 以及直接是邮件对象的情况。使用 UseNumber 保留大整数精度。
func decodeMailPayloadBodies(data json.RawMessage, keepRaw bool) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var top any
	if err := dec.Decode(&top); err != nil {
		return nil, fmt.Errorf("解析邮件响应失败: %w", err)
	}
	for _, msg := range collectMailMessages(top) {
		decodeMailMessageBodies(msg, keepRaw)
	}
	return top, nil
}

// collectMailMessages 从 data 中收集所有邮件对象（不复制，返回的 map 可就地修改）。
func collectMailMessages(top any) []map[string]any {
	root, ok := top.(map[string]any)
	if !ok {
		return nil
	}
	var out []map[string]any
	if m, ok := root["message"].(map[string]any); ok {
		out = append(out, m)
	}
	if list, ok := root["messages"].([]any); ok {
		for _, it := range list {
			if m, ok := it.(map[string]any); ok {
				out = append(out, m)
			}
		}
	}
	if th, ok := root["thread"].(map[string]any); ok {
		if list, ok := th["messages"].([]any); ok {
			for _, it := range list {
				if m, ok := it.(map[string]any); ok {
					out = append(out, m)
				}
			}
		}
	}
	if len(out) == 0 {
		if _, ok := root["message_id"]; ok {
			out = append(out, root)
		}
	}
	return out
}

// mailAnyString 把 JSON 值转成字符串（兼容 string / json.Number / float64）。
func mailAnyString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case json.Number:
		return val.String()
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", val)
	}
}

// mailFormatMillis 把毫秒时间戳格式化为本地时间；无法解析时原样返回。
func mailFormatMillis(v any) string {
	s := strings.TrimSpace(mailAnyString(v))
	if s == "" {
		return ""
	}
	ms, err := strconv.ParseInt(s, 10, 64)
	if err != nil || ms <= 0 {
		return s
	}
	return time.UnixMilli(ms).Local().Format("2006-01-02 15:04:05")
}

// mailAddressDisplay 把 {mail_address,name} 对象格式化为单行可读文本。
func mailAddressDisplay(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	addr := sanitizeMailSingleLine(mailAnyString(m["mail_address"]))
	name := sanitizeMailSingleLine(mailAnyString(m["name"]))
	if name != "" && addr != "" {
		return fmt.Sprintf("%s <%s>", name, addr)
	}
	if addr != "" {
		return addr
	}
	return name
}

func mailAddressListDisplay(v any) string {
	list, ok := v.([]any)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(list))
	for _, it := range list {
		if s := mailAddressDisplay(it); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ", ")
}

// renderMailMessageText 以可读文本输出单封邮件（已解码的 msg）。
func renderMailMessageText(w io.Writer, msg map[string]any) {
	line := func(label, value string) {
		if strings.TrimSpace(value) != "" {
			fmt.Fprintf(w, "%s: %s\n", label, value)
		}
	}
	line("主题", sanitizeMailSingleLine(mailAnyString(msg["subject"])))
	line("发件人", mailAddressDisplay(msg["head_from"]))
	line("收件人", mailAddressListDisplay(msg["to"]))
	line("抄送", mailAddressListDisplay(msg["cc"]))
	line("密送", mailAddressListDisplay(msg["bcc"]))
	line("时间", mailFormatMillis(msg["internal_date"]))
	line("邮件 ID", mailAnyString(msg["message_id"]))
	line("线程 ID", mailAnyString(msg["thread_id"]))
	line("文件夹", mailAnyString(msg["folder_id"]))
	if labels, ok := msg["label_ids"].([]any); ok && len(labels) > 0 {
		names := make([]string, 0, len(labels))
		for _, l := range labels {
			names = append(names, mailAnyString(l))
		}
		line("标签", strings.Join(names, ", "))
	}
	if atts, ok := msg["attachments"].([]any); ok && len(atts) > 0 {
		names := make([]string, 0, len(atts))
		for _, a := range atts {
			am, _ := a.(map[string]any)
			name := sanitizeMailSingleLine(mailAnyString(am["filename"]))
			if inline, _ := am["is_inline"].(bool); inline {
				name += "（内联）"
			}
			names = append(names, name)
		}
		line(fmt.Sprintf("附件(%d)", len(atts)), strings.Join(names, ", "))
	}
	// 服务端的 body_plain_text 会把换行折叠成空格，展示时优先从（已解码的）HTML 正文还原段落结构；
	// --raw-body 时 HTML 仍是 base64，不能拿来转换
	body := ""
	if htmlBody, _ := msg["body_html"].(string); strings.TrimSpace(htmlBody) != "" && strings.Contains(htmlBody, "<") {
		body = sanitizeMailText(mailHTMLToText(htmlBody))
	}
	if strings.TrimSpace(body) == "" {
		body, _ = msg["body_plain_text"].(string)
		body = sanitizeMailText(body)
	}
	if raw, ok := msg["raw"].(string); ok && raw != "" && body == "" {
		body = sanitizeMailText(decodeMailBase64URL(raw))
	}
	fmt.Fprintln(w, "----------------------------------------")
	if strings.TrimSpace(body) == "" {
		fmt.Fprintln(w, "（无正文）")
		return
	}
	fmt.Fprintln(w, strings.TrimRight(body, "\n"))
}

// renderMailPayloadText 按 message/messages/thread 形态输出可读文本。
func renderMailPayloadText(w io.Writer, payload any) {
	root, _ := payload.(map[string]any)
	if root == nil {
		fmt.Fprintln(w, mailAnyString(payload))
		return
	}
	if th, ok := root["thread"].(map[string]any); ok {
		msgs, _ := th["messages"].([]any)
		threadID := mailAnyString(th["thread_id"])
		if threadID == "" && len(msgs) > 0 {
			if first, ok := msgs[0].(map[string]any); ok {
				threadID = mailAnyString(first["thread_id"])
			}
		}
		fmt.Fprintf(w, "线程 %s（共 %d 封，按时间升序）\n\n", threadID, len(msgs))
		renderMailMessageList(w, msgs)
		return
	}
	if list, ok := root["messages"].([]any); ok {
		fmt.Fprintf(w, "共 %d 封邮件\n\n", len(list))
		renderMailMessageList(w, list)
		if missing, ok := root["unavailable_message_ids"].([]any); ok && len(missing) > 0 {
			ids := make([]string, 0, len(missing))
			for _, id := range missing {
				ids = append(ids, mailAnyString(id))
			}
			sort.Strings(ids)
			fmt.Fprintf(w, "\n未获取到的邮件 ID（%d）: %s\n", len(ids), strings.Join(ids, ", "))
		}
		return
	}
	for _, msg := range collectMailMessages(root) {
		renderMailMessageText(w, msg)
		return
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}

func renderMailMessageList(w io.Writer, list []any) {
	for i, it := range list {
		msg, ok := it.(map[string]any)
		if !ok {
			continue
		}
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "[%d]\n", i+1)
		renderMailMessageText(w, msg)
	}
}

// ==================== HTML → 纯文本（用于引用与展示） ====================

// mailHTMLToText 把 HTML 正文转为纯文本：丢弃 script/style/head，按 HTML 语义折叠空白（<pre> 内保留），
// 块级标签与 <br> 转换行，解码 HTML 实体，并压缩多余空行。仅用于展示与引用，不追求完美排版。
func mailHTMLToText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inPre := 0
	pendingSpace := false
	lastNewline := true
	writeText := func(text string) {
		if inPre > 0 {
			b.WriteString(text)
			lastNewline = strings.HasSuffix(text, "\n")
			pendingSpace = false
			return
		}
		for _, r := range text {
			if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' {
				pendingSpace = true
				continue
			}
			if pendingSpace && !lastNewline {
				b.WriteByte(' ')
			}
			pendingSpace = false
			b.WriteRune(r)
			lastNewline = false
		}
	}
	newline := func() {
		b.WriteByte('\n')
		lastNewline = true
		pendingSpace = false
	}
	i := 0
	for i < len(s) {
		if s[i] != '<' {
			next := strings.IndexByte(s[i:], '<')
			if next < 0 {
				next = len(s) - i
			}
			writeText(strings.ReplaceAll(html.UnescapeString(s[i:i+next]), "\u00a0", " "))
			i += next
			continue
		}
		if strings.HasPrefix(s[i:], "<!--") {
			if end := strings.Index(s[i+4:], "-->"); end >= 0 {
				i += 4 + end + 3
			} else {
				i = len(s)
			}
			continue
		}
		end := strings.IndexByte(s[i:], '>')
		if end < 0 {
			writeText(s[i:])
			break
		}
		tag := strings.ToLower(strings.TrimSpace(s[i+1 : i+end]))
		i += end + 1
		fields := strings.Fields(tag)
		name := ""
		if len(fields) > 0 {
			name = strings.TrimSuffix(fields[0], "/")
		}
		switch name {
		case "script", "style", "head", "title":
			closeTag := "</" + name
			if idx := strings.Index(strings.ToLower(s[i:]), closeTag); idx >= 0 {
				i += idx
				if gt := strings.IndexByte(s[i:], '>'); gt >= 0 {
					i += gt + 1
				} else {
					i = len(s)
				}
			} else {
				i = len(s)
			}
		case "pre":
			inPre++
			if !lastNewline {
				newline()
			}
		case "/pre":
			if inPre > 0 {
				inPre--
			}
			newline()
		case "br", "hr":
			newline()
		case "p", "div", "tr", "li", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "table", "ul", "ol",
			"/p", "/div", "/tr", "/li", "/h1", "/h2", "/h3", "/h4", "/h5", "/h6", "/blockquote", "/table", "/ul", "/ol":
			if !lastNewline {
				newline()
			}
		case "td", "th":
			pendingSpace = true
		}
	}
	lines := strings.Split(b.String(), "\n")
	for idx, line := range lines {
		lines[idx] = strings.TrimRight(line, " \t")
	}
	text := strings.Join(lines, "\n")
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(text)
}

// mailHTMLEscape 转义 HTML 五个特殊字符，用于把不可信文本嵌入 HTML。
func mailHTMLEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}
