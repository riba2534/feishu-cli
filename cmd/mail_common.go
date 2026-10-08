package cmd

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"mime"
	"net/mail"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// mailBoundary 生成一个 multipart 边界字符串（16-hex 随机）
func mailBoundary() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("生成 boundary 失败: %w", err)
	}
	return "fcli_" + hex.EncodeToString(b[:]), nil
}

// mailMessageInput 构造邮件的输入
type mailMessageInput struct {
	From         string   // 发件人邮箱地址
	FromName     string   // 发件人显示名
	To           []string // 收件人（"Name <email>" 或 "email"）
	CC           []string
	BCC          []string
	Subject      string
	BodyText     string // 纯文本 body
	BodyHTML     string // HTML body（与 BodyText 互斥，同时提供时优先 HTML）
	InReplyTo    string // 回复/转发场景：原邮件的 smtp_message_id（带不带尖括号均可，写入时规范化为 <id>）
	References   string // 回复场景：References 链（空白分隔的 message-id，写入时逐个规范化为 <id>）
	InlineImages []inlineImagePart
	Attachments  []mailAttachmentPart

	// LMSReplyToMessageID 原邮件在飞书邮箱内的 message_id。
	// 仅在 InReplyTo 非空时写入 X-LMS-Reply-To-Message-Id，飞书据此把回复/转发关联到原邮件（与官方一致）。
	LMSReplyToMessageID string
}

// inlineImagePart 内嵌图片 part（用于 multipart/related）
// CID 不包 "<>"；Filename 为附件展示名；Bytes 是 raw 内容；MIME 是 Content-Type
type inlineImagePart struct {
	CID      string
	Filename string
	Bytes    []byte
	MIME     string
}

// mailAttachmentPart 普通附件 part（用于 multipart/mixed）。
type mailAttachmentPart struct {
	Filename string
	MIME     string
	Bytes    []byte
}

// normalizeMailMessageID 把 Message-ID 规范化为 "<id>" 形式。
// 飞书 API 返回的 smtp_message_id 不带尖括号，而 RFC 5322 要求 In-Reply-To/References 中的
// msg-id 用尖括号包裹；不规范化会导致收件方客户端无法串联会话。空值返回空串。
func normalizeMailMessageID(id string) string {
	trimmed := strings.TrimSpace(id)
	trimmed = strings.TrimPrefix(trimmed, "<")
	trimmed = strings.TrimSuffix(trimmed, ">")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return ""
	}
	return "<" + trimmed + ">"
}

// normalizeMailReferences 规范化 References 链：按空白/逗号切分，逐个包尖括号并去重保序。
func normalizeMailReferences(refs string) string {
	fields := strings.FieldsFunc(refs, func(r rune) bool {
		return r == ' ' || r == '\t' || r == ','
	})
	seen := make(map[string]bool, len(fields))
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		id := normalizeMailMessageID(f)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return strings.Join(out, " ")
}

// formatMailAddress 用 net/mail 规范编码一个地址：显示名按 RFC 5322 加引号，非 ASCII 走 RFC 2047。
// 例如显示名含逗号的 `Doe, John` 会被写成 `"Doe, John" <j@example.com>`，不会被收件方拆成两个收件人。
func formatMailAddress(name, addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	return (&mail.Address{Name: strings.TrimSpace(name), Address: addr}).String()
}

// canonicalMailAddress 把 "Name <email>" / "email" 形式的输入规范化为 RFC 5322 编码形式；
// 无法解析时原样返回（输入在进入此处前已做过格式校验）。
func canonicalMailAddress(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if a, err := mail.ParseAddress(raw); err == nil {
		return a.String()
	}
	return raw
}

func formatMailAddressHeader(addrs []string) string {
	out := make([]string, 0, len(addrs))
	for _, a := range cleanAddresses(addrs) {
		if c := canonicalMailAddress(a); c != "" {
			out = append(out, c)
		}
	}
	return strings.Join(out, ", ")
}

// sanitizeMIMEFilename 去掉文件名里会破坏 MIME 参数的字符，非 ASCII 走 RFC 2047。
func sanitizeMIMEFilename(name string) string {
	name = strings.Map(func(r rune) rune {
		switch r {
		case '\r', '\n', '"', '\\':
			return '_'
		}
		return r
	}, name)
	if !isASCII(name) {
		return mime.BEncoding.Encode("utf-8", name)
	}
	return name
}

// buildEMLBase64URL 构造一个符合 RFC 5322 的 EML，base64 URL-safe 编码
// MIME 结构（外层仅在需要时出现）：multipart/mixed → multipart/related → 正文 part。
//   - 纯文本或 HTML 二选一；
//   - CID 内联图片（mail_inline.go 预处理）放在 multipart/related 中；
//   - 普通附件放在 multipart/mixed 中。
func buildEMLBase64URL(input mailMessageInput) (string, error) {
	raw, err := buildEMLBytes(input)
	if err != nil {
		return "", err
	}
	// 整个 EML → base64 URL-safe（无 padding，飞书 API 要求 RawURLEncoding）
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// buildEMLBytes 构造原始 EML 字节（buildEMLBase64URL 的内部实现，便于测试断言）。
func buildEMLBytes(input mailMessageInput) ([]byte, error) {
	if len(input.To) == 0 {
		return nil, fmt.Errorf("邮件至少需要一个 --to")
	}
	// 安全：拒绝 header 值含 CR/LF，避免 SMTP header injection（构造任意额外 header 或 split message）
	for name, val := range map[string]string{
		"--from":        input.From,
		"--from-name":   input.FromName,
		"--subject":     input.Subject,
		"--in-reply-to": input.InReplyTo,
		"--references":  input.References,
		"--message-id":  input.LMSReplyToMessageID,
	} {
		if strings.ContainsAny(val, "\r\n") {
			return nil, fmt.Errorf("%s 不能含 CR/LF 字符（防 header injection）", name)
		}
	}
	for _, addr := range append(append(append([]string{}, input.To...), input.CC...), input.BCC...) {
		if strings.ContainsAny(addr, "\r\n") {
			return nil, fmt.Errorf("收件人地址 %q 不能含 CR/LF 字符（防 header injection）", addr)
		}
	}
	// inline 图片 filename 也走相同 sanitize（用于 Content-Type name= 和 Content-Disposition filename=）
	for _, img := range input.InlineImages {
		if strings.ContainsAny(img.Filename, "\r\n") || strings.ContainsAny(img.CID, "\r\n") {
			return nil, fmt.Errorf("内嵌图片 filename/cid 不能含 CR/LF 字符（防 MIME header injection）")
		}
	}
	// 防御性：有内嵌图片但 HTML body 为空时直接报错，避免静默丢弃 InlineImages
	if len(input.InlineImages) > 0 && strings.TrimSpace(input.BodyHTML) == "" {
		return nil, fmt.Errorf("内嵌图片需要 HTML body（BodyHTML 为空时 InlineImages 会被静默丢弃）")
	}

	var b strings.Builder

	// From
	if input.From != "" {
		fmt.Fprintf(&b, "From: %s\r\n", formatMailAddress(input.FromName, input.From))
	}

	// To / Cc / Bcc：逐个地址按 RFC 5322 规范编码
	fmt.Fprintf(&b, "To: %s\r\n", formatMailAddressHeader(input.To))
	if len(input.CC) > 0 {
		fmt.Fprintf(&b, "Cc: %s\r\n", formatMailAddressHeader(input.CC))
	}
	if len(input.BCC) > 0 {
		fmt.Fprintf(&b, "Bcc: %s\r\n", formatMailAddressHeader(input.BCC))
	}

	// Subject (MIME encoded-word if non-ASCII)
	fmt.Fprintf(&b, "Subject: %s\r\n", mimeEncodeHeader(input.Subject))

	// Date
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))

	// MIME-Version
	b.WriteString("MIME-Version: 1.0\r\n")

	// In-Reply-To / References（回复、转发场景）：规范化为 <id>
	if inReplyTo := normalizeMailMessageID(input.InReplyTo); inReplyTo != "" {
		fmt.Fprintf(&b, "In-Reply-To: %s\r\n", inReplyTo)
		if lms := strings.TrimSpace(input.LMSReplyToMessageID); lms != "" {
			fmt.Fprintf(&b, "X-LMS-Reply-To-Message-Id: %s\r\n", lms)
		}
	}
	if refs := normalizeMailReferences(input.References); refs != "" {
		fmt.Fprintf(&b, "References: %s\r\n", refs)
	}

	if len(input.Attachments) > 0 {
		// multipart/mixed：正文实体 + 普通附件
		boundary, err := mailBoundary()
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&b, "Content-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n", boundary)
		fmt.Fprintf(&b, "--%s\r\n", boundary)
		if err := writeMailBodyEntity(&b, input); err != nil {
			return nil, err
		}
		b.WriteString("\r\n")
		for _, att := range input.Attachments {
			fmt.Fprintf(&b, "--%s\r\n", boundary)
			mimeType := att.MIME
			if mimeType == "" {
				mimeType = "application/octet-stream"
			}
			fname := sanitizeMIMEFilename(att.Filename)
			fmt.Fprintf(&b, "Content-Type: %s; name=\"%s\"\r\n", mimeType, fname)
			fmt.Fprintf(&b, "Content-Disposition: attachment; filename=\"%s\"\r\n", fname)
			b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
			b.WriteString(base64Encode(att.Bytes))
			b.WriteString("\r\n")
		}
		fmt.Fprintf(&b, "--%s--\r\n", boundary)
	} else if err := writeMailBodyEntity(&b, input); err != nil {
		return nil, err
	}

	return []byte(b.String()), nil
}

// writeMailBodyEntity 写出正文实体（含自身 Content-Type 头与内容）：
// 有内嵌图片时为 multipart/related(HTML + 图片)，否则为单一 text/html 或 text/plain。
func writeMailBodyEntity(b *strings.Builder, input mailMessageInput) error {
	hasInline := strings.TrimSpace(input.BodyHTML) != "" && len(input.InlineImages) > 0
	if hasInline {
		// multipart/related：HTML body + 内嵌图片
		// RFC 2046 §5.1.1: 每个 boundary delimiter line 前面必须有一个 CRLF，
		// 该 CRLF 在概念上属于 boundary 而非 preceding part body。
		// 实现上：每个 part body 后写 "\r\n\r\n"（结尾 CRLF + 分隔空行）再跟 boundary 行。
		boundary, berr := mailBoundary()
		if berr != nil {
			return berr
		}
		fmt.Fprintf(b, "Content-Type: multipart/related; boundary=\"%s\"\r\n\r\n", boundary)
		// HTML part
		fmt.Fprintf(b, "--%s\r\n", boundary)
		b.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
		b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		b.WriteString(base64Encode([]byte(input.BodyHTML)))
		b.WriteString("\r\n\r\n")
		// 每张内嵌图片
		for _, img := range input.InlineImages {
			fmt.Fprintf(b, "--%s\r\n", boundary)
			mimeType := img.MIME
			if mimeType == "" {
				mimeType = "application/octet-stream"
			}
			fname := img.Filename
			if fname == "" {
				fname = img.CID
			}
			fname = sanitizeMIMEFilename(fname)
			fmt.Fprintf(b, "Content-Type: %s; name=\"%s\"\r\n", mimeType, fname)
			b.WriteString("Content-Transfer-Encoding: base64\r\n")
			fmt.Fprintf(b, "Content-ID: <%s>\r\n", img.CID)
			fmt.Fprintf(b, "Content-Disposition: inline; filename=\"%s\"\r\n\r\n", fname)
			b.WriteString(base64Encode(img.Bytes))
			b.WriteString("\r\n\r\n")
		}
		fmt.Fprintf(b, "--%s--\r\n", boundary)
		return nil
	}
	if strings.TrimSpace(input.BodyHTML) != "" {
		b.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
		b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		b.WriteString(base64Encode([]byte(input.BodyHTML)))
		return nil
	}
	b.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	b.WriteString(base64Encode([]byte(input.BodyText)))
	return nil
}

// base64Encode 对 body 做标准 base64 编码，每 76 字符换行
func base64Encode(data []byte) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	var out strings.Builder
	for i := 0; i < len(encoded); i += 76 {
		end := i + 76
		if end > len(encoded) {
			end = len(encoded)
		}
		out.WriteString(encoded[i:end])
		out.WriteString("\r\n")
	}
	return out.String()
}

// mimeEncodeHeader 如果 header 包含非 ASCII，用 RFC 2047 encoded-word 编码
func mimeEncodeHeader(s string) string {
	if isASCII(s) {
		return s
	}
	// =?UTF-8?B?base64?=
	return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(s)) + "?="
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			return false
		}
	}
	return true
}

// cleanAddresses 清理地址列表（去空），保留 "Name <email>" 或 "email" 原始格式
func cleanAddresses(addrs []string) []string {
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		a = strings.TrimSpace(a)
		if a != "" {
			out = append(out, a)
		}
	}
	return out
}

// parseEmailList 解析逗号分隔的邮箱列表，并校验每项格式。
// 优先按 RFC 5322 地址列表解析（支持 `"Doe, John" <j@example.com>` 这类显示名含逗号的写法），
// 返回规范编码后的地址（显示名加引号 / 非 ASCII 走 RFC 2047）；整体解析失败时回退为逐项切分校验，
// 兼容 "a@example.com," 等宽松写法。
func parseEmailList(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	if list, err := mail.ParseAddressList(raw); err == nil {
		out := make([]string, 0, len(list))
		for _, a := range list {
			out = append(out, a.String())
		}
		return out, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// 允许 "Name <email>" 或 "email"
		a, err := mail.ParseAddress(p)
		if err != nil {
			return nil, fmt.Errorf("邮箱地址格式不正确: %q (%w)；显示名含逗号时请用英文双引号包裹，如 \"Doe, John\" <user@example.com>", p, err)
		}
		out = append(out, a.String())
	}
	return out, nil
}

// detectHTMLBody 粗略判断 body 是否为 HTML（含常见 HTML 标签）
func detectHTMLBody(body string) bool {
	lower := strings.ToLower(body)
	htmlMarkers := []string{"<html", "<body", "<div", "<p>", "<br", "<b>", "<i>", "<a ", "<table", "<h1", "<h2", "<h3"}
	for _, m := range htmlMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

// ensureReplySubject 确保 subject 带回复前缀（已有 Re:/回复： 时不重复）
func ensureReplySubject(original string) string {
	lower := strings.ToLower(strings.TrimSpace(original))
	for _, p := range []string{"re:", "re：", "回复：", "回复:", "答复：", "答复:"} {
		if strings.HasPrefix(lower, p) {
			return original
		}
	}
	return "Re: " + original
}

// ensureForwardSubject 确保 subject 带转发前缀（已有 Fwd:/Fw:/转发： 时不重复）
func ensureForwardSubject(original string) string {
	lower := strings.ToLower(strings.TrimSpace(original))
	for _, p := range []string{"fwd:", "fw:", "fwd：", "fw：", "转发：", "转发:"} {
		if strings.HasPrefix(lower, p) {
			return original
		}
	}
	return "Fwd: " + original
}

// resolveMailReadIdentity 解析 Mail 读命令的身份与 mailbox。
// 返回: token（空字符串表示 Bot，非空表示 User Token）, 规范化的 mailbox, error。
// 规则：
// 1. --as 仅支持 bot|user|auto；
// 2. Bot 身份（token == ""）下不支持 mailbox="me" 或留空，必须在网络请求前拒绝并要求指定具体邮箱地址；
// 3. User 身份（token != ""）下 mailbox 为空时默认 "me"。
func resolveMailReadIdentity(cmd *cobra.Command) (string, string, error) {
	token, err := resolveIdentityToken(cmd)
	if err != nil {
		return "", "", err
	}
	mailbox, _ := cmd.Flags().GetString("mailbox")
	mailbox = strings.TrimSpace(mailbox)
	if token == "" {
		// Bot 身份
		if mailbox == "" || mailbox == "me" {
			return "", "", fmt.Errorf("Bot 身份（--as bot）不支持 mailbox=\"me\"，请通过 --mailbox 指定具体邮箱地址（如 user@example.com）")
		}
	} else {
		// User 身份
		if mailbox == "" {
			mailbox = "me"
		}
	}
	return token, mailbox, nil
}
