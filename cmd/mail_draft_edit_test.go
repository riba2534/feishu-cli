package cmd

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/v2/internal/mailmime"
)

func b64Lines(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// 服务端 format=raw 导出的 HTML 回复草稿形态：multipart/mixed(alternative(plain, html), 附件)，带回复头。
var draftHTMLReplyEML = "From: <me@example.com>\r\n" +
	"To: <alice@example.com>\r\n" +
	"Subject: Re: hi\r\n" +
	"In-Reply-To: <orig@example.com>\r\n" +
	"References: <root@example.com> <orig@example.com>\r\n" +
	"X-Lms-Reply-To-Message-Id: lms_orig\r\n" +
	"Message-Id: <draft@example.com>\r\n" +
	"Mime-Version: 1.0\r\n" +
	"Content-Type: multipart/mixed;\r\n boundary=OUT\r\n" +
	"\r\n" +
	"--OUT\r\n" +
	"Content-Type: multipart/alternative; boundary=ALT\r\n" +
	"\r\n" +
	"--ALT\r\n" +
	"Content-Transfer-Encoding: 7bit\r\n" +
	"Content-Type: text/plain; charset=UTF-8\r\n" +
	"\r\n" +
	"old reply\r\n\r\nFrom: alice\r\norig text\r\n" +
	"--ALT\r\n" +
	"Content-Transfer-Encoding: base64\r\n" +
	"Content-Type: text/html; charset=UTF-8\r\n" +
	"\r\n" +
	b64Lines(`<p>old reply</p><div class="history-quote-wrapper"><div>QUOTED &lt;orig&gt;</div></div>`) + "\r\n" +
	"--ALT--\r\n" +
	"--OUT\r\n" +
	"Content-Type: application/pdf; name=\"a.pdf\"\r\n" +
	"Content-Disposition: attachment; filename=\"a.pdf\"\r\n" +
	"Content-Transfer-Encoding: base64\r\n" +
	"\r\n" +
	"UERGREFUQQ==\r\n" +
	"--OUT--\r\n"

// 服务端为纯文本回复草稿导出的形态：HTML 备选为 "<p>" + 纯文本 + "</p>"（未转义）。
var draftPlainReplyEML = "To: <alice@example.com>\r\n" +
	"Subject: Re: hi\r\n" +
	"In-Reply-To: <orig@example.com>\r\n" +
	"Content-Type: multipart/alternative;\r\n boundary=ALT\r\n" +
	"\r\n" +
	"--ALT\r\n" +
	"Content-Transfer-Encoding: 7bit\r\n" +
	"Content-Type: text/plain; charset=UTF-8\r\n" +
	"\r\n" +
	"old reply\r\n\r\n> 发件人：<alice@example.com>\r\n>\r\n> orig <b>text</b>\r\n" +
	"--ALT\r\n" +
	"Content-Transfer-Encoding: quoted-printable\r\n" +
	"Content-Type: text/html; charset=UTF-8\r\n" +
	"\r\n" +
	"<p>old reply\r\n\r\n> 发件人：<alice@example.com>\r\n>\r\n> orig <b>text</b></p>\r\n" +
	"--ALT--\r\n"

func mailStrPtr(s string) *string { return &s }

func parseEdited(t *testing.T, eml []byte) (*mailmime.Part, string, string) {
	t.Helper()
	root, err := mailmime.Parse(eml)
	if err != nil {
		t.Fatalf("修改后的 EML 无法解析: %v\n%s", err, eml)
	}
	plain, html := root.BodyParts()
	var p, h string
	if plain != nil {
		b, _ := plain.Decoded()
		p = string(b)
	}
	if html != nil {
		b, _ := html.Decoded()
		h = string(b)
	}
	return root, p, h
}

// TestApplyMailDraftEdit_PreservesHeadersAttachmentsAndQuote 回归：此前 draft-edit 全量重建 EML，
// 编辑回复草稿会丢 In-Reply-To / References / X-LMS 头、附件与引用块。
func TestApplyMailDraftEdit_PreservesHeadersAttachmentsAndQuote(t *testing.T) {
	out, changed, err := applyMailDraftEdit([]byte(draftHTMLReplyEML), mailDraftEdit{
		body:    mailStrPtr("<p>new reply</p>"),
		subject: mailStrPtr("Re: 新主题"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(changed, ",") != "subject,body" {
		t.Errorf("changed = %v", changed)
	}
	root, plain, html := parseEdited(t, out)
	for h, want := range map[string]string{
		"In-Reply-To":               "<orig@example.com>",
		"References":                "<root@example.com> <orig@example.com>",
		"X-Lms-Reply-To-Message-Id": "lms_orig",
		"Message-Id":                "<draft@example.com>",
		"To":                        "<alice@example.com>",
	} {
		if got := root.Get(h); got != want {
			t.Errorf("头 %s = %q, want %q", h, got, want)
		}
	}
	if !strings.Contains(root.Get("Subject"), "=?") {
		t.Errorf("中文主题应 RFC 2047 编码: %q", root.Get("Subject"))
	}
	if !strings.HasPrefix(html, "<p>new reply</p><div class=\"history-quote-wrapper\">") || !strings.Contains(html, "QUOTED &lt;orig&gt;") {
		t.Errorf("HTML 正文应为新正文 + 原引用块: %q", html)
	}
	if !strings.HasPrefix(plain, "new reply") || !strings.Contains(plain, "QUOTED <orig>") {
		t.Errorf("text/plain 应同步为新正文的纯文本 + 引用: %q", plain)
	}
	if !strings.Contains(string(out), "Content-Disposition: attachment; filename=\"a.pdf\"\r\nContent-Transfer-Encoding: base64\r\n\r\nUERGREFUQQ==\r\n--OUT--") {
		t.Errorf("附件 part 未原样保留:\n%s", out)
	}
}

func TestApplyMailDraftEdit_DropQuoteAndPlainBodyIntoHTMLDraft(t *testing.T) {
	out, _, err := applyMailDraftEdit([]byte(draftHTMLReplyEML), mailDraftEdit{body: mailStrPtr("a < b\nline2"), dropQuote: true})
	if err != nil {
		t.Fatal(err)
	}
	_, _, html := parseEdited(t, out)
	if html != "a &lt; b<br>line2" {
		t.Errorf("纯文本正文写入 HTML 草稿应转义，且 --drop-quote 删除引用块: %q", html)
	}
}

// TestApplyMailDraftEdit_PlainDraft 纯文本草稿：保留 "> " 引用块，移除服务端合成的未转义 HTML 备选。
func TestApplyMailDraftEdit_PlainDraft(t *testing.T) {
	out, _, err := applyMailDraftEdit([]byte(draftPlainReplyEML), mailDraftEdit{body: mailStrPtr("new plain")})
	if err != nil {
		t.Fatal(err)
	}
	root, plain, html := parseEdited(t, out)
	if html != "" {
		t.Errorf("纯文本草稿的合成 HTML 备选应被移除: %q", html)
	}
	if !strings.HasPrefix(plain, "new plain\n\n> 发件人：<alice@example.com>") || !strings.Contains(plain, "> orig <b>text</b>") {
		t.Errorf("纯文本引用块未保留: %q", plain)
	}
	if root.Get("In-Reply-To") != "<orig@example.com>" {
		t.Errorf("In-Reply-To 丢失")
	}

	// 新正文为 HTML：引用块转义后放入 <pre>
	out, _, err = applyMailDraftEdit([]byte(draftPlainReplyEML), mailDraftEdit{body: mailStrPtr("<p>new</p>")})
	if err != nil {
		t.Fatal(err)
	}
	_, _, html = parseEdited(t, out)
	if !strings.HasPrefix(html, "<p>new</p><pre") || strings.Contains(html, "<b>text</b>") || !strings.Contains(html, "&lt;b&gt;text&lt;/b&gt;") {
		t.Errorf("HTML 新正文下纯文本引用块应转义: %q", html)
	}
}

func TestApplyMailDraftEdit_Recipients(t *testing.T) {
	to := []string{`"Doe, John" <john@example.com>`}
	empty := []string{}
	out, _, err := applyMailDraftEdit([]byte(draftHTMLReplyEML), mailDraftEdit{to: &to, cc: &empty, fromName: mailStrPtr("新名字")})
	if err != nil {
		t.Fatal(err)
	}
	root, _, _ := parseEdited(t, out)
	if got := root.Get("To"); got != `"Doe, John" <john@example.com>` {
		t.Errorf("To = %q", got)
	}
	if root.Has("Cc") {
		t.Error("--cc \"\" 应删除 Cc 头")
	}
	if got := root.Get("From"); !strings.Contains(got, "<me@example.com>") || !strings.Contains(got, "=?utf-8?") {
		t.Errorf("仅改 --from-name 时应保留原地址: %q", got)
	}
}

func TestSplitMailPlainQuote(t *testing.T) {
	body, quote := splitMailPlainQuote("hi\n> not quote\nbye", true)
	if quote != "" || body != "hi\n> not quote\nbye" {
		t.Errorf("单行 > 不应视为引用块: body=%q quote=%q", body, quote)
	}
	body, quote = splitMailPlainQuote("hi\n\nalice 写道:\n> a\n> b\n", true)
	if body != "hi" || quote != "\n\nalice 写道:\n> a\n> b\n" {
		t.Errorf("旧格式引用块拆分错误: body=%q quote=%q", body, quote)
	}
	body, quote = splitMailPlainQuote("fyi\n\n--------- 转发消息 ---------\n发件人：x\n\norig", false)
	if body != "fyi" || !strings.HasPrefix(quote, "\n\n--------- 转发消息") {
		t.Errorf("转发分隔行拆分错误: body=%q quote=%q", body, quote)
	}
	if _, q := splitMailPlainQuote("x\n> a\n> b", false); q != "" {
		t.Error("非回复草稿不应按 > 行拆分")
	}
}

// TestMailDraftEditCmd_ReadPatchWrite 端到端：GET format=raw（data.draft.message.raw 形态）→ 局部修改 → PUT。
func TestMailDraftEditCmd_ReadPatchWrite(t *testing.T) {
	var putRaw string
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			gotQuery = r.URL.RawQuery
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "msg": "ok", "data": map[string]any{
				"draft": map[string]any{"id": "d1", "message": map[string]any{
					"message_id": "d1", "raw": base64.RawURLEncoding.EncodeToString([]byte(draftHTMLReplyEML)),
				}},
			}})
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			var req struct {
				Raw string `json:"raw"`
			}
			_ = json.Unmarshal(body, &req)
			putRaw = req.Raw
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "msg": "ok", "data": map[string]any{}})
		default:
			t.Errorf("意外请求 %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	setupMailAttendanceCmdTestConfig(t, srv.URL)

	cmd := mailDraftEditCmd
	_ = cmd.Flags().Set("user-access-token", "u-test")
	_ = cmd.Flags().Set("draft-id", "d1")
	_ = cmd.Flags().Set("body", "<p>edited</p>")
	_ = cmd.Flags().Set("output", "json")
	defer func() {
		for _, f := range []string{"body", "draft-id"} {
			cmd.Flags().Lookup(f).Changed = false
		}
	}()
	var err error
	out := captureMailStdout(t, func() { err = cmd.RunE(cmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	if gotQuery != "format=raw" {
		t.Errorf("应以 format=raw 读取草稿，query=%q", gotQuery)
	}
	eml, err := base64.RawURLEncoding.DecodeString(putRaw)
	if err != nil {
		t.Fatalf("PUT raw 不是 base64url: %v", err)
	}
	root, _, html := parseEdited(t, eml)
	if root.Get("In-Reply-To") != "<orig@example.com>" || !strings.Contains(html, "history-quote-wrapper") || !strings.Contains(string(eml), "UERGREFUQQ==") {
		t.Errorf("draft-edit 未保留回复头/引用块/附件:\n%s", eml)
	}
	if !strings.Contains(out, `"body"`) {
		t.Errorf("输出应列出修改项: %s", out)
	}
}

func TestMailDraftEditCmd_RequiresSomeChange(t *testing.T) {
	setupMailAttendanceCmdTestConfig(t, "http://127.0.0.1:1")
	cmd := mailDraftEditCmd
	_ = cmd.Flags().Set("draft-id", "d1")
	defer func() { cmd.Flags().Lookup("draft-id").Changed = false }()
	err := cmd.RunE(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "至少指定一项修改") {
		t.Fatalf("未指定修改项应报用法错误，得到 %v", err)
	}
}
