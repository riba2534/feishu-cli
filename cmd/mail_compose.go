package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
)

// reply / reply-all / forward 共用的"原邮件"读取与引用块构造。
//
// 对齐官方 shortcuts/mail（helpers.go / mail_quote.go / mail_reply*.go / mail_forward.go）：
//   - 原邮件正文先 base64url 解码再引用；纯文本引用清理终端控制字符；
//   - HTML 引用块中所有来自原邮件的文本（发件人、主题、正文）一律 HTML 转义，避免注入；
//   - In-Reply-To 用 <smtp_message_id>，并写 X-LMS-Reply-To-Message-Id（原邮件 message_id）；
//   - 回复地址优先 Reply-To；回复自己发出的邮件时改为回复原收件人。

// mailAddr 地址（显示名 + 邮箱）。
type mailAddr struct {
	Name string
	Addr string
}

// String 返回 RFC 5322 规范编码形式（用于写 EML 头）。
func (a mailAddr) String() string { return formatMailAddress(a.Name, a.Addr) }

// mailComposeSource 回复/转发所需的原邮件信息（正文已解码）。
type mailComposeSource struct {
	MessageID     string
	ThreadID      string
	SMTPMessageID string
	Subject       string
	From          mailAddr
	To            []mailAddr
	CC            []mailAddr
	ReplyTo       []mailAddr
	References    string // 原邮件 References 链（原样，写入时再规范化）
	InternalDate  string // 毫秒时间戳
	BodyPlain     string // 已解码 + 终端安全清理
	BodyHTML      string // 已解码（不可信，嵌入 HTML 前不得直接拼接）
	Attachments   []mailSourceAttachment
}

// mailSourceAttachment 原邮件附件元信息。
type mailSourceAttachment struct {
	ID             string
	Filename       string
	IsInline       bool
	CID            string
	AttachmentType int // 2 = 超大附件（云文档卡片，不随 EML 携带）
}

// fetchMailComposeSource 拉取原邮件（format=full）并解析为 mailComposeSource。
func fetchMailComposeSource(mailboxID, messageID, userAccessToken string) (*mailComposeSource, error) {
	data, err := client.GetMailMessage(mailboxID, messageID, "full", userAccessToken)
	if err != nil {
		return nil, fmt.Errorf("获取原邮件失败: %w", err)
	}
	return parseMailComposeSource(data)
}

// parseMailComposeSource 解析 GET message 的 data（兼容 {message:{...}} 与直接是邮件对象）。
func parseMailComposeSource(data json.RawMessage) (*mailComposeSource, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var top map[string]any
	if err := dec.Decode(&top); err != nil {
		return nil, fmt.Errorf("解析原邮件失败: %w", err)
	}
	msg, _ := top["message"].(map[string]any)
	if msg == nil {
		msg = top
	}
	src := &mailComposeSource{
		MessageID:     mailAnyString(msg["message_id"]),
		ThreadID:      mailAnyString(msg["thread_id"]),
		SMTPMessageID: strings.TrimSpace(mailAnyString(msg["smtp_message_id"])),
		Subject:       sanitizeMailSingleLine(mailAnyString(msg["subject"])),
		From:          mailAddrFromAny(msg["head_from"]),
		To:            mailAddrListFromAny(msg["to"]),
		CC:            mailAddrListFromAny(msg["cc"]),
		ReplyTo:       parseMailAddrString(mailReplyToString(msg["reply_to"])),
		References:    mailReferencesString(msg["references"]),
		InternalDate:  mailAnyString(msg["internal_date"]),
	}
	html := decodeMailBase64URL(mailAnyString(msg["body_html"]))
	// 服务端 body_plain_text 会把换行折叠成空格，引用原文时优先从 HTML 正文还原段落结构
	plain := ""
	if strings.TrimSpace(html) != "" {
		plain = mailHTMLToText(html)
	}
	if strings.TrimSpace(plain) == "" {
		plain = decodeMailBase64URL(mailAnyString(msg["body_plain_text"]))
	}
	if strings.TrimSpace(plain) == "" {
		plain = decodeMailBase64URL(mailAnyString(msg["body_preview"]))
	}
	src.BodyPlain = sanitizeMailText(plain)
	src.BodyHTML = html
	if atts, ok := msg["attachments"].([]any); ok {
		for _, a := range atts {
			am, _ := a.(map[string]any)
			if am == nil {
				continue
			}
			inline, _ := am["is_inline"].(bool)
			attType, _ := strconv.Atoi(mailAnyString(am["attachment_type"]))
			src.Attachments = append(src.Attachments, mailSourceAttachment{
				ID:             mailAnyString(am["id"]),
				Filename:       mailAnyString(am["filename"]),
				IsInline:       inline,
				CID:            mailAnyString(am["cid"]),
				AttachmentType: attType,
			})
		}
	}
	return src, nil
}

func mailAddrFromAny(v any) mailAddr {
	m, _ := v.(map[string]any)
	if m == nil {
		return mailAddr{}
	}
	return mailAddr{
		Name: sanitizeMailSingleLine(mailAnyString(m["name"])),
		Addr: strings.TrimSpace(mailAnyString(m["mail_address"])),
	}
}

func mailAddrListFromAny(v any) []mailAddr {
	list, _ := v.([]any)
	out := make([]mailAddr, 0, len(list))
	for _, it := range list {
		if a := mailAddrFromAny(it); a.Addr != "" {
			out = append(out, a)
		}
	}
	return out
}

// mailReplyToString reply_to 可能是字符串、地址对象或地址对象数组，统一成地址列表字符串。
func mailReplyToString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case map[string]any:
		return mailAddrFromAny(val).String()
	case []any:
		parts := make([]string, 0, len(val))
		for _, it := range val {
			switch item := it.(type) {
			case string:
				parts = append(parts, item)
			case map[string]any:
				if s := mailAddrFromAny(item).String(); s != "" {
					parts = append(parts, s)
				}
			}
		}
		return strings.Join(parts, ", ")
	}
	return ""
}

// mailReferencesString references 可能是字符串或字符串数组。
func mailReferencesString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case []any:
		parts := make([]string, 0, len(val))
		for _, it := range val {
			if s := mailAnyString(it); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}

// parseMailAddrString 解析地址列表字符串；整体解析失败时逐项宽松解析。
func parseMailAddrString(raw string) []mailAddr {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if list, err := mail.ParseAddressList(raw); err == nil {
		out := make([]mailAddr, 0, len(list))
		for _, a := range list {
			out = append(out, mailAddr{Name: a.Name, Addr: a.Address})
		}
		return out
	}
	var out []mailAddr
	for _, p := range strings.Split(raw, ",") {
		if a, err := mail.ParseAddress(strings.TrimSpace(p)); err == nil {
			out = append(out, mailAddr{Name: a.Name, Addr: a.Address})
		}
	}
	return out
}

// mailSelfAddressSet 当前用户的地址集合（小写）：mailbox profile 主地址 + 显式 mailbox 地址。
func mailSelfAddressSet(primary, mailboxID string) map[string]bool {
	set := make(map[string]bool, 2)
	if p := strings.ToLower(strings.TrimSpace(primary)); p != "" {
		set[p] = true
	}
	if m := strings.ToLower(strings.TrimSpace(mailboxID)); m != "" && m != "me" && strings.Contains(m, "@") {
		set[m] = true
	}
	return set
}

// buildMailReplyRecipients 计算回复的 To / Cc。
//
//   - 普通邮件：To = Reply-To（存在时）否则原发件人；reply-all 额外把原 To（排除自己）并入 To、原 Cc（排除自己）放入 Cc；
//   - 自己发出的邮件（发件人是自己）：To = 原 To（排除自己）+ Reply-To（若有）；reply-all 时原 Cc 仍放 Cc
//     （对齐官方 mail_reply_all.go 的 self-sent 处理）；若排除后为空（自己发给自己），回复给自己；
//   - 全程按小写邮箱去重。
func buildMailReplyRecipients(src *mailComposeSource, self map[string]bool, replyAll bool) (to, cc []mailAddr, err error) {
	seen := make(map[string]bool)
	add := func(list *[]mailAddr, a mailAddr, skipSelf bool) {
		key := strings.ToLower(strings.TrimSpace(a.Addr))
		if key == "" || seen[key] {
			return
		}
		if skipSelf && self[key] {
			return
		}
		seen[key] = true
		*list = append(*list, a)
	}

	selfSent := src.From.Addr != "" && self[strings.ToLower(src.From.Addr)]
	if selfSent {
		for _, a := range src.To {
			add(&to, a, true)
		}
		for _, a := range src.ReplyTo {
			add(&to, a, true)
		}
		if replyAll {
			for _, a := range src.CC {
				add(&cc, a, true)
			}
		}
		if len(to) == 0 && len(cc) == 0 {
			// 自己发给自己的邮件：回复仍发给自己
			add(&to, src.From, false)
		}
	} else {
		targets := src.ReplyTo
		if len(targets) == 0 && src.From.Addr != "" {
			targets = []mailAddr{src.From}
		}
		for _, a := range targets {
			add(&to, a, false)
		}
		if replyAll {
			for _, a := range src.To {
				add(&to, a, true)
			}
			for _, a := range src.CC {
				add(&cc, a, true)
			}
		}
	}
	if len(to) == 0 && len(cc) > 0 {
		to, cc = cc[:1], cc[1:]
	}
	if len(to) == 0 {
		return nil, nil, fmt.Errorf("无法确定回复收件人（原邮件缺少发件人 / Reply-To / 收件人信息）")
	}
	return to, cc, nil
}

func mailAddrStrings(list []mailAddr) []string {
	out := make([]string, 0, len(list))
	for _, a := range list {
		if s := a.String(); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// buildMailReplyReferences 生成回复的 References：原 References 链 + 原邮件 Message-ID（规范化为 <id>，去重）。
func buildMailReplyReferences(src *mailComposeSource) string {
	refs := strings.TrimSpace(src.References)
	if src.SMTPMessageID != "" {
		refs = strings.TrimSpace(refs + " " + src.SMTPMessageID)
	}
	return normalizeMailReferences(refs)
}

// ==================== 引用块 ====================

// mailQuoteLabels 引用块元信息标签（按原主题语言选择中/英文，与官方一致）。
type mailQuoteLabels struct {
	From, Date, Subject, To, Cc, Separator, Colon string
	lang                                          string
}

func mailQuoteLabelsFor(subject string) mailQuoteLabels {
	for _, r := range subject {
		if (r >= 0x4e00 && r <= 0x9fff) || (r >= 0x3400 && r <= 0x4dbf) || (r >= 0xf900 && r <= 0xfaff) || (r >= 0x3040 && r <= 0x30ff) {
			return mailQuoteLabels{From: "发件人", Date: "时间", Subject: "主题", To: "收件人", Cc: "抄送",
				Separator: "--------- 转发消息 ---------", Colon: "：", lang: "zh"}
		}
	}
	return mailQuoteLabels{From: "From", Date: "Date", Subject: "Subject", To: "To", Cc: "Cc",
		Separator: "---------- Forwarded message ---------", Colon: ": ", lang: "en"}
}

var mailZhWeekdays = [7]string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}

// formatMailQuoteDate 格式化原邮件时间（毫秒时间戳，本地时区）。
func formatMailQuoteDate(internalDate, lang string) string {
	ms, err := strconv.ParseInt(strings.TrimSpace(internalDate), 10, 64)
	if err != nil || ms <= 0 {
		return ""
	}
	t := time.UnixMilli(ms).Local()
	if lang == "zh" {
		return fmt.Sprintf("%s (%s) %s", t.Format("2006年1月2日"), mailZhWeekdays[t.Weekday()], t.Format("15:04"))
	}
	return t.Format("Mon, 02 Jan 2006 15:04 MST")
}

// plainMailAddr 纯文本引用中的地址：`"Name" <addr>` 或 `<addr>`。
func plainMailAddr(a mailAddr) string {
	if a.Name != "" {
		return fmt.Sprintf("%q <%s>", a.Name, a.Addr)
	}
	return "<" + a.Addr + ">"
}

func plainMailAddrList(list []mailAddr) string {
	parts := make([]string, 0, len(list))
	for _, a := range list {
		parts = append(parts, plainMailAddr(a))
	}
	return strings.Join(parts, ", ")
}

// buildMailPlainMetaRows 纯文本元信息行（发件人/时间/主题/收件人/抄送），每行加 linePrefix。
func buildMailPlainMetaRows(src *mailComposeSource, linePrefix string) string {
	l := mailQuoteLabelsFor(src.Subject)
	var sb strings.Builder
	row := func(label, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		sb.WriteString(linePrefix + label + l.Colon + value + "\n")
	}
	if src.From.Addr != "" {
		row(l.From, plainMailAddr(src.From))
	}
	row(l.Date, formatMailQuoteDate(src.InternalDate, l.lang))
	row(l.Subject, src.Subject)
	row(l.To, plainMailAddrList(src.To))
	row(l.Cc, plainMailAddrList(src.CC))
	return sb.String()
}

// buildMailPlainReplyQuote 纯文本回复引用块：元信息 + "> " 前缀正文。
func buildMailPlainReplyQuote(src *mailComposeSource) string {
	if src.BodyPlain == "" && src.From.Addr == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n\n")
	sb.WriteString(buildMailPlainMetaRows(src, "> "))
	sb.WriteString(">\n")
	for _, line := range strings.Split(strings.TrimRight(src.BodyPlain, "\n"), "\n") {
		sb.WriteString("> " + line + "\n")
	}
	return sb.String()
}

// buildMailPlainForward 纯文本转发正文：附言 + 分隔线 + 元信息 + 原文。
func buildMailPlainForward(src *mailComposeSource, comment string) string {
	l := mailQuoteLabelsFor(src.Subject)
	var sb strings.Builder
	if comment != "" {
		sb.WriteString(comment)
		sb.WriteString("\n\n")
	}
	sb.WriteString(l.Separator + "\n")
	sb.WriteString(buildMailPlainMetaRows(src, ""))
	sb.WriteString("\n")
	sb.WriteString(src.BodyPlain)
	return sb.String()
}

// HTML 引用块结构对齐飞书邮箱客户端（官方 mail_quote.go）：
// history-quote-wrapper → adit-html-block（回复为 --collapsed 折叠块，转发为 --header 展开块）。
// draft-edit 依赖 history-quote-wrapper 定位并保留引用块。
const (
	mailQuoteWrapperClass  = "history-quote-wrapper"
	mailQuoteBorderStyle   = "border-left: none; padding-left: 0px;"
	mailQuoteMetaStyle     = "padding: 12px; background: rgb(245, 246, 247); color: rgb(31, 35, 41); border-radius: 4px; margin-bottom: 12px;"
	mailQuoteSeparatorCSS  = "color: rgb(100, 106, 115); margin-top: 24px; margin-bottom: 8px;"
	mailQuotePlainBodyHTML = `<pre style="white-space:pre-wrap">%s</pre>`
)

func htmlMailAddr(a mailAddr) string {
	addr := mailHTMLEscape(a.Addr)
	if a.Name != "" {
		return fmt.Sprintf(`"%s"&lt;%s&gt;`, mailHTMLEscape(a.Name), addr)
	}
	return "&lt;" + addr + "&gt;"
}

func htmlMailAddrList(list []mailAddr) string {
	parts := make([]string, 0, len(list))
	for _, a := range list {
		parts = append(parts, "<span>"+htmlMailAddr(a)+"</span>")
	}
	return strings.Join(parts, ", ")
}

// buildMailHTMLMetaRows HTML 元信息行；所有值均已转义。
func buildMailHTMLMetaRows(src *mailComposeSource) string {
	l := mailQuoteLabelsFor(src.Subject)
	var sb strings.Builder
	row := func(label, contentHTML string) {
		if strings.TrimSpace(contentHTML) == "" {
			return
		}
		fmt.Fprintf(&sb, `<div class="lme-line-signal"><span style="">%s: %s</span></div>`, mailHTMLEscape(label), contentHTML)
	}
	if src.From.Addr != "" {
		row(l.From, htmlMailAddr(src.From))
	}
	row(l.Date, mailHTMLEscape(formatMailQuoteDate(src.InternalDate, l.lang)))
	row(l.Subject, mailHTMLEscape(src.Subject))
	row(l.To, htmlMailAddrList(src.To))
	row(l.Cc, htmlMailAddrList(src.CC))
	return sb.String()
}

// buildMailQuotedBodyHTML 原邮件正文的 HTML 形式：使用已解码、已清理的纯文本，整体转义后放入 <pre>。
// 不直接嵌入原邮件 HTML：原 HTML 不可信，且其中 cid: 内联图片不会随回复携带。
func buildMailQuotedBodyHTML(src *mailComposeSource) string {
	if strings.TrimSpace(src.BodyPlain) == "" {
		return ""
	}
	return "<div>" + fmt.Sprintf(mailQuotePlainBodyHTML, mailHTMLEscape(src.BodyPlain)) + "</div>"
}

// buildMailHTMLReplyQuote HTML 回复引用块（折叠块）。
func buildMailHTMLReplyQuote(src *mailComposeSource) string {
	if src.BodyPlain == "" && src.From.Addr == "" {
		return ""
	}
	meta := fmt.Sprintf(`<div class="adit-html-block__attr history-quote-meta-wrapper history-quote-gap-tag" style="margin-top: 24px; %s"><div style="word-break: break-word;">%s</div></div>`,
		mailQuoteMetaStyle, buildMailHTMLMetaRows(src))
	return `<div class="` + mailQuoteWrapperClass + `"><div data-html-block="quote" data-mail-html-ignore="">` +
		`<div class="adit-html-block adit-html-block--collapsed" style="` + mailQuoteBorderStyle + `">` +
		`<div><div>` + meta + buildMailQuotedBodyHTML(src) + `</div></div>` +
		`</div></div></div>`
}

// buildMailHTMLForwardQuote HTML 转发引用块（展开块，带分隔行）。
func buildMailHTMLForwardQuote(src *mailComposeSource) string {
	l := mailQuoteLabelsFor(src.Subject)
	sep := fmt.Sprintf(`<div class="history-quote-forward-title lme-line-signal history-quote-gap-tag" style="%s">%s</div>`,
		mailQuoteSeparatorCSS, mailHTMLEscape(l.Separator))
	meta := fmt.Sprintf(`<div class="adit-html-block__header history-quote-meta-after-forward-title history-quote-meta-wrapper" style="margin-top: 2px; %s"><div style="word-break: break-word;">%s</div></div>`,
		mailQuoteMetaStyle, buildMailHTMLMetaRows(src))
	return `<div class="` + mailQuoteWrapperClass + `"><div data-html-block="quote" data-mail-html-ignore="">` +
		`<div class="adit-html-block adit-html-block--header" style="` + mailQuoteBorderStyle + `">` +
		`<div>` + sep + meta + buildMailQuotedBodyHTML(src) + `</div>` +
		`</div></div></div>`
}

// mailPlainTextToHTML 把用户输入的纯文本转为 HTML 片段（转义 + 换行转 <br>）。
func mailPlainTextToHTML(s string) string {
	return strings.ReplaceAll(mailHTMLEscape(s), "\n", "<br>")
}
