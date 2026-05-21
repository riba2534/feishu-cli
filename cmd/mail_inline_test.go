package cmd

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/client"
)

// TestMailScanInlineImagePaths_BasicAndSchemeSkip 验证扫描器:
//  1. 抓 <img src="本地路径">
//  2. 跳过 cid:/http:/data:/// scheme
//  3. 重复路径只返回一次
func TestMailScanInlineImagePaths_BasicAndSchemeSkip(t *testing.T) {
	html := `
		<p>hi</p>
		<img src="./a.png">
		<IMG SRC='b.jpg'>
		<img src="http://cdn/x.png">
		<img src="cid:already">
		<img src="data:image/png;base64,xxxx">
		<img src="//cdn/y.png">
		<img src="./a.png">
	`
	got := client.ScanInlineImagePaths(html)
	want := []string{"./a.png", "b.jpg"}
	if len(got) != len(want) {
		t.Fatalf("ScanInlineImagePaths: got %d items %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] got %q, want %q", i, got[i], want[i])
		}
	}
}

// TestMailReplaceInlineImageSrc 验证 src 被替换为 cid:xxx 且只替换匹配项
func TestMailReplaceInlineImageSrc(t *testing.T) {
	html := `<p>x</p><img src="./a.png"><img src="b.jpg"><img src="http://keep.me/y.png">`
	refs := []client.MailInlineImageRef{
		{RawSrc: "./a.png", CID: "cid_a"},
		{RawSrc: "b.jpg", CID: "cid_b"},
	}
	out := client.ReplaceInlineImageSrc(html, refs)
	if !strings.Contains(out, `src="cid:cid_a"`) {
		t.Errorf("missing cid:cid_a in output: %s", out)
	}
	if !strings.Contains(out, `src="cid:cid_b"`) {
		t.Errorf("missing cid:cid_b in output: %s", out)
	}
	if !strings.Contains(out, `src="http://keep.me/y.png"`) {
		t.Errorf("外部 http URL 被错误改写: %s", out)
	}
}

// TestBuildEMLBase64URL_WithInlineImages 验证 multipart/related 结构生成
func TestBuildEMLBase64URL_WithInlineImages(t *testing.T) {
	input := mailMessageInput{
		From:     "me@example.com",
		To:       []string{"a@example.com"},
		Subject:  "test",
		BodyHTML: `<p>x</p><img src="cid:cid_a">`,
		InlineImages: []inlineImagePart{
			{CID: "cid_a", Filename: "a.png", MIME: "image/png", Bytes: []byte{0x89, 0x50, 0x4e, 0x47}},
		},
	}
	rawB64, err := buildEMLBase64URL(input)
	if err != nil {
		t.Fatalf("buildEMLBase64URL: %v", err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(rawB64)
	if err != nil {
		t.Fatalf("decode rawB64: %v", err)
	}
	raw := string(decoded)
	if !strings.Contains(raw, "Content-Type: multipart/related;") {
		t.Errorf("missing multipart/related: %s", raw)
	}
	if !strings.Contains(raw, "Content-ID: <cid_a>") {
		t.Errorf("missing Content-ID <cid_a>: %s", raw)
	}
	if !strings.Contains(raw, "Content-Disposition: inline") {
		t.Errorf("missing inline disposition: %s", raw)
	}
	if !strings.Contains(raw, "Content-Type: image/png") {
		t.Errorf("missing image/png part header: %s", raw)
	}
	if !strings.Contains(raw, "Content-Type: text/html") {
		t.Errorf("missing text/html part header: %s", raw)
	}
}

// TestBuildEMLBase64URL_MultipartCRLFSeparator 验证 RFC 2046 §5.1.1 要求：
// 每个 boundary delimiter line 前必须有一个 CRLF（独立于 part body 内容）
// 即 body 末尾必须出现 "\r\n\r\n--boundary" 而非紧贴 "<base64>\r\n--boundary"
func TestBuildEMLBase64URL_MultipartCRLFSeparator(t *testing.T) {
	input := mailMessageInput{
		From:     "me@example.com",
		To:       []string{"a@example.com"},
		Subject:  "test",
		BodyHTML: `<p>x</p><img src="cid:cid_a">`,
		InlineImages: []inlineImagePart{
			{CID: "cid_a", Filename: "a.png", MIME: "image/png", Bytes: []byte{0x89, 0x50, 0x4e, 0x47}},
			{CID: "cid_b", Filename: "b.png", MIME: "image/png", Bytes: []byte{0x89, 0x50, 0x4e, 0x48}},
		},
	}
	rawB64, err := buildEMLBase64URL(input)
	if err != nil {
		t.Fatalf("buildEMLBase64URL: %v", err)
	}
	decoded, _ := base64.RawURLEncoding.DecodeString(rawB64)
	raw := string(decoded)
	// boundary delimiter 之前必须是 "\r\n\r\n"（part body 结尾 CRLF + 空行）
	// 数 "\r\n\r\n--" 的出现次数：应该 ≥ part 数（HTML + 2 inline = 3 个分隔）
	cnt := strings.Count(raw, "\r\n\r\n--")
	if cnt < 3 {
		t.Errorf("RFC 2046 CRLF 分隔不足：got %d 次 `\\r\\n\\r\\n--`，want ≥3\nraw=%q", cnt, raw)
	}
}

// TestBuildEMLBase64URL_InlineWithoutHTMLRejected 验证 InlineImages 非空但 HTML body 为空时报错
// 防御性断言：避免静默丢弃 InlineImages
func TestBuildEMLBase64URL_InlineWithoutHTMLRejected(t *testing.T) {
	input := mailMessageInput{
		From:     "me@example.com",
		To:       []string{"a@example.com"},
		Subject:  "test",
		BodyText: "plain only",
		InlineImages: []inlineImagePart{
			{CID: "cid_a", Filename: "a.png", MIME: "image/png", Bytes: []byte{0x89}},
		},
	}
	_, err := buildEMLBase64URL(input)
	if err == nil {
		t.Fatal("应当报错：InlineImages 非空但 BodyHTML 为空")
	}
	if !strings.Contains(err.Error(), "HTML") {
		t.Errorf("错误信息未提到 HTML body 要求: %v", err)
	}
}

// TestBuildEMLBase64URL_WithoutInline 没有内嵌图片时不应该走 multipart/related（保持原路径）
func TestBuildEMLBase64URL_WithoutInline(t *testing.T) {
	input := mailMessageInput{
		From:     "me@example.com",
		To:       []string{"a@example.com"},
		Subject:  "test",
		BodyHTML: "<p>x</p>",
	}
	rawB64, err := buildEMLBase64URL(input)
	if err != nil {
		t.Fatalf("buildEMLBase64URL: %v", err)
	}
	decoded, _ := base64.RawURLEncoding.DecodeString(rawB64)
	raw := string(decoded)
	if strings.Contains(raw, "multipart/related") {
		t.Errorf("纯 HTML body 不应使用 multipart/related: %s", raw)
	}
	if !strings.Contains(raw, "Content-Type: text/html") {
		t.Errorf("missing text/html: %s", raw)
	}
}

// TestToMailTemplateAddrs 验证 "Name <email>" 拆分
func TestToMailTemplateAddrs(t *testing.T) {
	in := []string{"Alice <a@example.com>", "b@example.com", "  ", "Bob<b2@example.com>"}
	got := toMailTemplateAddrs(in)
	if len(got) != 3 {
		t.Fatalf("len=%d, want 3, got=%+v", len(got), got)
	}
	if got[0].Name != "Alice" || got[0].MailAddress != "a@example.com" {
		t.Errorf("[0] %+v", got[0])
	}
	if got[1].Name != "" || got[1].MailAddress != "b@example.com" {
		t.Errorf("[1] %+v", got[1])
	}
	if got[2].Name != "Bob" || got[2].MailAddress != "b2@example.com" {
		t.Errorf("[2] %+v", got[2])
	}
}

// TestGenerateMailCID 验证 CID 长度且唯一
func TestGenerateMailCID(t *testing.T) {
	a, err := client.GenerateMailCID()
	if err != nil {
		t.Fatalf("GenerateMailCID: %v", err)
	}
	b, err := client.GenerateMailCID()
	if err != nil {
		t.Fatalf("GenerateMailCID: %v", err)
	}
	if a == b {
		t.Errorf("two CIDs should differ: %s == %s", a, b)
	}
	if len(a) != 20 {
		t.Errorf("CID length should be 20 hex chars, got %d (%q)", len(a), a)
	}
}
