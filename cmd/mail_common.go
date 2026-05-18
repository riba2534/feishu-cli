package cmd

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/mail"
	"strings"
	"time"
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
	InReplyTo    string // 回复场景：原邮件的 Message-ID header
	References   string // 回复场景：原邮件的 References header
	InlineImages []inlineImagePart
}

// inlineImagePart 内嵌图片 part（用于 multipart/related）
// CID 不包 "<>"；Filename 为附件展示名；Bytes 是 raw 内容；MIME 是 Content-Type
type inlineImagePart struct {
	CID      string
	Filename string
	Bytes    []byte
	MIME     string
}

// buildEMLBase64URL 构造一个符合 RFC 5322 的 EML，base64 URL-safe 编码
// 简化版：不支持附件和 CID 内联图片，纯文本或 HTML 二选一
func buildEMLBase64URL(input mailMessageInput) (string, error) {
	if len(input.To) == 0 {
		return "", fmt.Errorf("邮件至少需要一个 --to")
	}
	// 安全：拒绝 header 值含 CR/LF，避免 SMTP header injection（构造任意额外 header 或 split message）
	for name, val := range map[string]string{
		"--from":        input.From,
		"--from-name":   input.FromName,
		"--subject":     input.Subject,
		"--in-reply-to": input.InReplyTo,
		"--references":  input.References,
	} {
		if strings.ContainsAny(val, "\r\n") {
			return "", fmt.Errorf("%s 不能含 CR/LF 字符（防 header injection）", name)
		}
	}
	for _, addr := range append(append(append([]string{}, input.To...), input.CC...), input.BCC...) {
		if strings.ContainsAny(addr, "\r\n") {
			return "", fmt.Errorf("收件人地址 %q 不能含 CR/LF 字符（防 header injection）", addr)
		}
	}
	// inline 图片 filename 也走相同 sanitize（用于 Content-Type name= 和 Content-Disposition filename=）
	for _, img := range input.InlineImages {
		if strings.ContainsAny(img.Filename, "\r\n") || strings.ContainsAny(img.CID, "\r\n") {
			return "", fmt.Errorf("内嵌图片 filename/cid 不能含 CR/LF 字符（防 MIME header injection）")
		}
	}

	var b strings.Builder

	// From
	if input.From != "" {
		if input.FromName != "" {
			fmt.Fprintf(&b, "From: %s <%s>\r\n", mimeEncodeHeader(input.FromName), input.From)
		} else {
			fmt.Fprintf(&b, "From: %s\r\n", input.From)
		}
	}

	// To
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(cleanAddresses(input.To), ", "))
	if len(input.CC) > 0 {
		fmt.Fprintf(&b, "Cc: %s\r\n", strings.Join(cleanAddresses(input.CC), ", "))
	}
	if len(input.BCC) > 0 {
		fmt.Fprintf(&b, "Bcc: %s\r\n", strings.Join(cleanAddresses(input.BCC), ", "))
	}

	// Subject (MIME encoded-word if non-ASCII)
	fmt.Fprintf(&b, "Subject: %s\r\n", mimeEncodeHeader(input.Subject))

	// Date
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))

	// MIME-Version
	b.WriteString("MIME-Version: 1.0\r\n")

	// In-Reply-To / References（回复场景）
	if input.InReplyTo != "" {
		fmt.Fprintf(&b, "In-Reply-To: %s\r\n", input.InReplyTo)
	}
	if input.References != "" {
		fmt.Fprintf(&b, "References: %s\r\n", input.References)
	}

	// Body
	hasInline := strings.TrimSpace(input.BodyHTML) != "" && len(input.InlineImages) > 0
	// 防御性：有内嵌图片但 HTML body 为空时直接报错，避免静默丢弃 InlineImages
	if len(input.InlineImages) > 0 && strings.TrimSpace(input.BodyHTML) == "" {
		return "", fmt.Errorf("内嵌图片需要 HTML body（BodyHTML 为空时 InlineImages 会被静默丢弃）")
	}
	if hasInline {
		// multipart/related：HTML body + 内嵌图片
		// RFC 2046 §5.1.1: 每个 boundary delimiter line 前面必须有一个 CRLF，
		// 该 CRLF 在概念上属于 boundary 而非 preceding part body。
		// 实现上：每个 part body 后写 "\r\n\r\n"（结尾 CRLF + 分隔空行）再跟 boundary 行。
		boundary, berr := mailBoundary()
		if berr != nil {
			return "", berr
		}
		fmt.Fprintf(&b, "Content-Type: multipart/related; boundary=\"%s\"\r\n\r\n", boundary)
		// HTML part
		fmt.Fprintf(&b, "--%s\r\n", boundary)
		b.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
		b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		b.WriteString(base64Encode([]byte(input.BodyHTML)))
		b.WriteString("\r\n\r\n")
		// 每张内嵌图片
		for _, img := range input.InlineImages {
			fmt.Fprintf(&b, "--%s\r\n", boundary)
			mime := img.MIME
			if mime == "" {
				mime = "application/octet-stream"
			}
			fname := img.Filename
			if fname == "" {
				fname = img.CID
			}
			fmt.Fprintf(&b, "Content-Type: %s; name=\"%s\"\r\n", mime, fname)
			b.WriteString("Content-Transfer-Encoding: base64\r\n")
			fmt.Fprintf(&b, "Content-ID: <%s>\r\n", img.CID)
			fmt.Fprintf(&b, "Content-Disposition: inline; filename=\"%s\"\r\n\r\n", fname)
			b.WriteString(base64Encode(img.Bytes))
			b.WriteString("\r\n\r\n")
		}
		fmt.Fprintf(&b, "--%s--\r\n", boundary)
	} else if strings.TrimSpace(input.BodyHTML) != "" {
		b.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
		b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		b.WriteString(base64Encode([]byte(input.BodyHTML)))
	} else {
		b.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
		b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		b.WriteString(base64Encode([]byte(input.BodyText)))
	}

	// 整个 EML → base64 URL-safe（无 padding，飞书 API 要求 RawURLEncoding）
	raw := b.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw)), nil
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

// parseEmailList 解析逗号分隔的邮箱列表，并校验每项格式
func parseEmailList(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// 允许 "Name <email>" 或 "email"
		if _, err := mail.ParseAddress(p); err != nil {
			return nil, fmt.Errorf("邮箱地址格式不正确: %q (%w)", p, err)
		}
		out = append(out, p)
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

// ensureReplySubject 确保 subject 带 "Re: " 前缀（避免重复）
func ensureReplySubject(original string) string {
	lower := strings.ToLower(strings.TrimSpace(original))
	if strings.HasPrefix(lower, "re:") || strings.HasPrefix(lower, "re：") {
		return original
	}
	return "Re: " + original
}

// ensureForwardSubject 确保 subject 带 "Fwd: " 前缀
func ensureForwardSubject(original string) string {
	lower := strings.ToLower(strings.TrimSpace(original))
	if strings.HasPrefix(lower, "fwd:") || strings.HasPrefix(lower, "fw:") {
		return original
	}
	return "Fwd: " + original
}

// buildQuotedBody 构造引用块（用于 reply/forward）
// quotePrefix: "> " 普通引用，或 "" 折叠
func buildQuotedBody(body, quotePrefix string) string {
	if strings.TrimSpace(body) == "" {
		return ""
	}
	lines := strings.Split(body, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, quotePrefix+line)
	}
	return strings.Join(out, "\n")
}
