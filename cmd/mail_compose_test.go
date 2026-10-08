package cmd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"os"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func TestNormalizeMailMessageID(t *testing.T) {
	cases := map[string]string{
		"abc@example.com":      "<abc@example.com>",
		"<abc@example.com>":    "<abc@example.com>",
		"  <abc@example.com> ": "<abc@example.com>",
		"":                     "",
		"<>":                   "",
	}
	for in, want := range cases {
		if got := normalizeMailMessageID(in); got != want {
			t.Errorf("normalizeMailMessageID(%q) = %q, want %q", in, got, want)
		}
	}
	if got := normalizeMailReferences("<a@x> b@x, a@x  <c@x>"); got != "<a@x> <b@x> <c@x>" {
		t.Errorf("normalizeMailReferences = %q", got)
	}
}

// TestBuildEMLReplyHeaders In-Reply-To/References 必须带尖括号；X-LMS-Reply-To-Message-Id 仅在 In-Reply-To 存在时写入。
func TestBuildEMLReplyHeaders(t *testing.T) {
	raw, err := buildEMLBytes(mailMessageInput{
		From:                "me@example.com",
		To:                  []string{"a@example.com"},
		Subject:             "Re: hi",
		BodyText:            "x",
		InReplyTo:           "orig-id@example.com",
		References:          "<r1@example.com> orig-id@example.com",
		LMSReplyToMessageID: "lms_msg_1",
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{
		"In-Reply-To: <orig-id@example.com>\r\n",
		"References: <r1@example.com> <orig-id@example.com>\r\n",
		"X-LMS-Reply-To-Message-Id: lms_msg_1\r\n",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("EML 缺少 %q:\n%s", want, s)
		}
	}

	raw, err = buildEMLBytes(mailMessageInput{To: []string{"a@example.com"}, BodyText: "x", LMSReplyToMessageID: "lms"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "X-LMS-Reply-To-Message-Id") {
		t.Errorf("无 In-Reply-To 时不应写 X-LMS 头:\n%s", raw)
	}
}

// TestBuildEMLAddressEncoding 显示名含逗号/非 ASCII 时必须规范编码，收件方解析后仍是同一个地址。
func TestBuildEMLAddressEncoding(t *testing.T) {
	to, err := parseEmailList(`"Doe, John" <john@example.com>, 张三 <zs@example.com>, plain@example.com`)
	if err != nil {
		t.Fatal(err)
	}
	if len(to) != 3 {
		t.Fatalf("应解析出 3 个地址，得到 %d: %v", len(to), to)
	}
	raw, err := buildEMLBytes(mailMessageInput{From: "me@example.com", FromName: "Me, Myself", To: to, BodyText: "x"})
	if err != nil {
		t.Fatal(err)
	}
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	list, err := msg.Header.AddressList("To")
	if err != nil {
		t.Fatalf("To 头无法按 RFC 5322 解析: %v (%s)", err, msg.Header.Get("To"))
	}
	if len(list) != 3 || list[0].Name != "Doe, John" || list[1].Name != "张三" || list[2].Address != "plain@example.com" {
		t.Errorf("To 解析结果不符: %+v", list)
	}
	from, err := msg.Header.AddressList("From")
	if err != nil || len(from) != 1 || from[0].Name != "Me, Myself" {
		t.Errorf("From 解析结果不符: %+v err=%v", from, err)
	}
}

func TestBuildEMLWithAttachments(t *testing.T) {
	raw, err := buildEMLBytes(mailMessageInput{
		To:          []string{"a@example.com"},
		BodyHTML:    "<p>x</p>",
		Attachments: []mailAttachmentPart{{Filename: "报告.pdf", MIME: "application/pdf", Bytes: []byte("PDFDATA")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if ct := msg.Header.Get("Content-Type"); !strings.HasPrefix(ct, "multipart/mixed;") {
		t.Fatalf("有附件时应为 multipart/mixed，得到 %q", ct)
	}
	body, _ := io.ReadAll(msg.Body)
	s := string(body)
	if !strings.Contains(s, "Content-Type: text/html") {
		t.Errorf("缺少正文 part: %s", s)
	}
	if !strings.Contains(s, "Content-Disposition: attachment; filename=\"=?utf-8?b?") {
		t.Errorf("非 ASCII 附件名应 RFC 2047 编码: %s", s)
	}
	if !strings.Contains(s, base64.StdEncoding.EncodeToString([]byte("PDFDATA"))) {
		t.Errorf("附件内容缺失: %s", s)
	}
}

func TestBuildMailReplyRecipients(t *testing.T) {
	self := mailSelfAddressSet("me@example.com", "me")
	me := mailAddr{Name: "Me", Addr: "me@example.com"}
	alice := mailAddr{Name: "Alice", Addr: "alice@example.com"}
	bob := mailAddr{Name: "Bob", Addr: "bob@example.com"}
	carol := mailAddr{Name: "Carol", Addr: "carol@example.com"}
	list := mailAddr{Name: "List", Addr: "list@example.com"}
	addrs := func(l []mailAddr) string {
		out := make([]string, 0, len(l))
		for _, a := range l {
			out = append(out, a.Addr)
		}
		return strings.Join(out, ",")
	}
	cases := []struct {
		name             string
		src              mailComposeSource
		replyAll         bool
		wantTo, wantCc   string
		wantErrSubstring string
	}{
		{name: "普通回复给发件人", src: mailComposeSource{From: alice, To: []mailAddr{me, bob}, CC: []mailAddr{carol}}, wantTo: "alice@example.com"},
		{name: "Reply-To 优先", src: mailComposeSource{From: alice, ReplyTo: []mailAddr{list}, To: []mailAddr{me}}, wantTo: "list@example.com"},
		{name: "reply-all 排除自己并去重", src: mailComposeSource{From: alice, To: []mailAddr{me, bob, {Addr: "ALICE@example.com"}}, CC: []mailAddr{carol, {Addr: "Me@Example.com"}}}, replyAll: true, wantTo: "alice@example.com,bob@example.com", wantCc: "carol@example.com"},
		{name: "回复自己发出的邮件给原收件人", src: mailComposeSource{From: me, To: []mailAddr{bob}, CC: []mailAddr{carol}}, wantTo: "bob@example.com"},
		{name: "reply-all 自己发出的邮件保持 To/Cc", src: mailComposeSource{From: me, To: []mailAddr{bob}, CC: []mailAddr{carol, me}}, replyAll: true, wantTo: "bob@example.com", wantCc: "carol@example.com"},
		{name: "自己发给自己回复给自己", src: mailComposeSource{From: me, To: []mailAddr{me}}, wantTo: "me@example.com"},
		{name: "无任何地址报错", src: mailComposeSource{}, wantErrSubstring: "无法确定回复收件人"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.src
			to, cc, err := buildMailReplyRecipients(&src, self, tc.replyAll)
			if tc.wantErrSubstring != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrSubstring) {
					t.Fatalf("err = %v, want %q", err, tc.wantErrSubstring)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if addrs(to) != tc.wantTo || addrs(cc) != tc.wantCc {
				t.Errorf("to=%q cc=%q, want to=%q cc=%q", addrs(to), addrs(cc), tc.wantTo, tc.wantCc)
			}
		})
	}
}

// mailComposeMockServer 模拟原邮件 / profile / 附件下载链接 / 创建草稿；返回捕获到的草稿 EML。
type mailComposeMock struct {
	srv       *httptest.Server
	draftEML  []byte
	draftHits int
}

func newMailComposeMock(t *testing.T, msg map[string]any) *mailComposeMock {
	t.Helper()
	m := &mailComposeMock{}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		reply := func(data any) {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "msg": "ok", "data": data})
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/profile"):
			reply(map[string]any{"primary_email_address": "me@example.com", "name": "Me"})
		case strings.HasSuffix(r.URL.Path, "/attachments/download_url"):
			ids := r.URL.Query()["attachment_ids"]
			var urls []any
			for _, id := range ids {
				urls = append(urls, map[string]any{"attachment_id": id, "download_url": "https://download.example.com/" + id})
			}
			reply(map[string]any{"download_urls": urls})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/drafts"):
			m.draftHits++
			var body struct {
				Raw string `json:"raw"`
			}
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &body)
			eml, err := base64.RawURLEncoding.DecodeString(body.Raw)
			if err != nil {
				t.Errorf("草稿 raw 不是 base64url: %v", err)
			}
			m.draftEML = eml
			reply(map[string]any{"draft": map[string]any{"draft_id": "d_new"}})
		case strings.Contains(r.URL.Path, "/messages/"):
			reply(map[string]any{"message": msg})
		default:
			t.Errorf("意外请求: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	setupMailAttendanceCmdTestConfig(t, m.srv.URL)
	return m
}

func decodedEMLParts(t *testing.T, eml []byte) (map[string]string, string) {
	t.Helper()
	msg, err := mail.ReadMessage(bytes.NewReader(eml))
	if err != nil {
		t.Fatalf("EML 无法解析: %v\n%s", err, eml)
	}
	headers := map[string]string{}
	for k := range msg.Header {
		headers[k] = msg.Header.Get(k)
	}
	body, _ := io.ReadAll(msg.Body)
	// 正文 part 为 base64，解码所有 base64 块以便断言
	var decoded strings.Builder
	for _, chunk := range strings.Split(string(body), "\r\n\r\n") {
		clean := strings.ReplaceAll(strings.TrimSpace(chunk), "\r\n", "")
		if b, err := base64.StdEncoding.DecodeString(clean); err == nil && len(clean) > 0 {
			decoded.Write(b)
			decoded.WriteString("\n")
		}
	}
	return headers, decoded.String()
}

func mailSourceMessage() map[string]any {
	return map[string]any{
		"message_id":      "lms_orig_1",
		"thread_id":       "th_1",
		"smtp_message_id": "orig-smtp@example.com",
		"references":      "<root@example.com>",
		"subject":         "季度计划 <script>",
		"head_from":       map[string]any{"mail_address": "alice@example.com", "name": "Alice, A"},
		"reply_to":        "List <list@example.com>",
		"to":              []any{map[string]any{"mail_address": "me@example.com", "name": "Me"}},
		"internal_date":   "1790757108345",
		"body_plain_text": b64url("第一行 <script>alert(1)</script> 第二行"),
		"body_html":       b64url("<div>第一行 &lt;script&gt;alert(1)&lt;/script&gt;</div><div>第二行</div>"),
		"attachments": []any{
			map[string]any{"id": "att1", "filename": "a.txt", "attachment_type": 1},
			map[string]any{"id": "img1", "filename": "i.png", "is_inline": true, "cid": "c1"},
			map[string]any{"id": "big1", "filename": "big.zip", "attachment_type": 2},
		},
	}
}

// TestMailReplyCmd_PlainQuoteAndHeaders 回复：引用块为解码后的原文；In-Reply-To 带尖括号；写 X-LMS；收件人优先 Reply-To。
func TestMailReplyCmd_PlainQuoteAndHeaders(t *testing.T) {
	m := newMailComposeMock(t, mailSourceMessage())
	defer m.srv.Close()

	_ = mailReplyCmd.Flags().Set("user-access-token", "u-test")
	_ = mailReplyCmd.Flags().Set("message-id", "lms_orig_1")
	_ = mailReplyCmd.Flags().Set("body", "收到")
	_ = mailReplyCmd.Flags().Set("plain-text", "true")
	_ = mailReplyCmd.Flags().Set("html", "false")
	_ = mailReplyCmd.Flags().Set("output", "json")
	defer func() { _ = mailReplyCmd.Flags().Set("plain-text", "false") }()

	var err error
	captureMailStdout(t, func() { err = mailReplyCmd.RunE(mailReplyCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	headers, text := decodedEMLParts(t, m.draftEML)
	if got := headers["In-Reply-To"]; got != "<orig-smtp@example.com>" {
		t.Errorf("In-Reply-To = %q", got)
	}
	if got := headers["References"]; got != "<root@example.com> <orig-smtp@example.com>" {
		t.Errorf("References = %q", got)
	}
	if got := headers["X-Lms-Reply-To-Message-Id"]; got != "lms_orig_1" {
		t.Errorf("X-LMS-Reply-To-Message-Id = %q", got)
	}
	if got := headers["To"]; got != `"List" <list@example.com>` {
		t.Errorf("To 应优先使用 Reply-To，得到 %q", got)
	}
	if !strings.Contains(text, "> 第一行 <script>alert(1)</script>") || !strings.Contains(text, "> 第二行") {
		t.Errorf("纯文本引用块应为解码后的原文:\n%s", text)
	}
	if strings.Contains(string(m.draftEML), mailSourceMessage()["body_plain_text"].(string)) {
		t.Errorf("引用块中出现了未解码的 base64 原文")
	}
}

// TestMailReplyCmd_HTMLQuoteEscaped HTML 模式回复：引用块中的原文、主题、显示名全部转义。
func TestMailReplyCmd_HTMLQuoteEscaped(t *testing.T) {
	m := newMailComposeMock(t, mailSourceMessage())
	defer m.srv.Close()

	_ = mailReplyCmd.Flags().Set("user-access-token", "u-test")
	_ = mailReplyCmd.Flags().Set("message-id", "lms_orig_1")
	_ = mailReplyCmd.Flags().Set("body", "<p>收到</p>")
	_ = mailReplyCmd.Flags().Set("plain-text", "false")
	_ = mailReplyCmd.Flags().Set("html", "false")
	_ = mailReplyCmd.Flags().Set("output", "json")

	var err error
	captureMailStdout(t, func() { err = mailReplyCmd.RunE(mailReplyCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	_, text := decodedEMLParts(t, m.draftEML)
	if !strings.Contains(text, "<p>收到</p>") {
		t.Errorf("用户 HTML 正文缺失:\n%s", text)
	}
	if strings.Contains(text, "<script>") {
		t.Errorf("HTML 引用块含未转义的 <script>:\n%s", text)
	}
	for _, want := range []string{"&lt;script&gt;alert(1)&lt;/script&gt;", "季度计划 &lt;script&gt;", `class="history-quote-wrapper"`, "Alice, A"} {
		if !strings.Contains(text, want) {
			t.Errorf("HTML 引用块缺少 %q:\n%s", want, text)
		}
	}
}

// TestMailForwardCmd_CarriesOriginalAttachments 转发：携带原邮件普通附件（跳过内联与超大附件），写 In-Reply-To 与 X-LMS。
func TestMailForwardCmd_CarriesOriginalAttachments(t *testing.T) {
	m := newMailComposeMock(t, mailSourceMessage())
	defer m.srv.Close()
	var downloaded []string
	old := downloadMailAttachment
	downloadMailAttachment = func(u string) ([]byte, error) {
		downloaded = append(downloaded, u)
		return []byte("ATTACHMENT-BYTES"), nil
	}
	defer func() { downloadMailAttachment = old }()

	_ = mailForwardCmd.Flags().Set("user-access-token", "u-test")
	_ = mailForwardCmd.Flags().Set("message-id", "lms_orig_1")
	_ = mailForwardCmd.Flags().Set("to", "bob@example.com")
	_ = mailForwardCmd.Flags().Set("body", "请看")
	_ = mailForwardCmd.Flags().Set("output", "json")

	var err error
	out := captureMailStdout(t, func() { err = mailForwardCmd.RunE(mailForwardCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	if len(downloaded) != 1 || !strings.HasSuffix(downloaded[0], "/att1") {
		t.Errorf("应只下载普通附件 att1，实际 %v", downloaded)
	}
	headers, text := decodedEMLParts(t, m.draftEML)
	if headers["In-Reply-To"] != "<orig-smtp@example.com>" || headers["X-Lms-Reply-To-Message-Id"] != "lms_orig_1" {
		t.Errorf("转发头不符: %v", headers)
	}
	if !strings.HasPrefix(headers["Content-Type"], "multipart/mixed") {
		t.Errorf("带附件转发应为 multipart/mixed: %q", headers["Content-Type"])
	}
	if !strings.Contains(string(m.draftEML), `filename="a.txt"`) || !strings.Contains(text, "ATTACHMENT-BYTES") {
		t.Errorf("原附件未随转发携带:\n%s", m.draftEML)
	}
	if !strings.Contains(text, "--------- 转发消息 ---------") || !strings.Contains(text, "第一行 <script>alert(1)</script>") {
		t.Errorf("转发正文应含分隔行与解码后的原文:\n%s", text)
	}
	if !strings.Contains(out, "a.txt") {
		t.Errorf("JSON 输出应列出附件: %s", out)
	}

	// --no-original-attachments：不下载、不携带
	downloaded = nil
	_ = mailForwardCmd.Flags().Set("no-original-attachments", "true")
	defer func() { _ = mailForwardCmd.Flags().Set("no-original-attachments", "false") }()
	captureMailStdout(t, func() { err = mailForwardCmd.RunE(mailForwardCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	if len(downloaded) != 0 || strings.Contains(string(m.draftEML), `filename="a.txt"`) {
		t.Errorf("--no-original-attachments 不应携带原附件")
	}
}

func TestLoadMailAttachmentsFromFlags_Validation(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, size int) string {
		p := dir + "/" + name
		if err := os.WriteFile(p, bytes.Repeat([]byte("a"), size), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	setAttach := func(vals ...string) {
		if err := mailSendCmd.Flags().Lookup("attach").Value.(pflag.SliceValue).Replace(vals); err != nil {
			t.Fatal(err)
		}
	}
	defer setAttach()

	setAttach(write("ok.txt", 10) + "," + write("报告.pdf", 20))
	parts, err := loadMailAttachmentsFromFlags(mailSendCmd)
	if err != nil || len(parts) != 2 || parts[0].Filename != "ok.txt" || parts[1].MIME != "application/pdf" {
		t.Fatalf("合法附件加载失败: %v %+v", err, parts)
	}
	setAttach(write("run.exe", 10))
	if _, err := loadMailAttachmentsFromFlags(mailSendCmd); err == nil || !strings.Contains(err.Error(), "禁止") {
		t.Errorf("可执行扩展名应被拒绝: %v", err)
	}
	setAttach(write("big.dat", 19*1024*1024))
	if _, err := loadMailAttachmentsFromFlags(mailSendCmd); err == nil || !strings.Contains(err.Error(), "25MB") {
		t.Errorf("超过 25MB（编码后）应报错: %v", err)
	}
	setAttach(dir)
	if _, err := loadMailAttachmentsFromFlags(mailSendCmd); err == nil || !strings.Contains(err.Error(), "普通文件") {
		t.Errorf("目录应被拒绝: %v", err)
	}
}

// TestMailReplyCmd_DisplayNameWithComma 回归：原发件人显示名含逗号时，旧实现写成 `Alice, A <a@x>`，
// 收件方会拆成两个收件人。现在按 RFC 5322 加引号。
func TestMailReplyCmd_DisplayNameWithComma(t *testing.T) {
	src := mailSourceMessage()
	delete(src, "reply_to")
	m := newMailComposeMock(t, src)
	defer m.srv.Close()

	_ = mailReplyCmd.Flags().Set("user-access-token", "u-test")
	_ = mailReplyCmd.Flags().Set("message-id", "lms_orig_1")
	_ = mailReplyCmd.Flags().Set("body", "ok")
	_ = mailReplyCmd.Flags().Set("output", "json")
	var err error
	captureMailStdout(t, func() { err = mailReplyCmd.RunE(mailReplyCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	msg, err := mail.ReadMessage(bytes.NewReader(m.draftEML))
	if err != nil {
		t.Fatal(err)
	}
	list, err := msg.Header.AddressList("To")
	if err != nil || len(list) != 1 || list[0].Name != "Alice, A" || list[0].Address != "alice@example.com" {
		t.Errorf("To 应解析为单个收件人 \"Alice, A\"，得到 %+v err=%v（原始头 %q）", list, err, msg.Header.Get("To"))
	}
}
