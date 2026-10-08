package cmd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// captureMailStdout 捕获 fn 执行期间写到 os.Stdout 的内容（并发读取，避免大输出阻塞管道）。
func captureMailStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	defer func() { os.Stdout = old }()
	fn()
	_ = w.Close()
	return <-done
}

func b64url(s string) string { return base64.URLEncoding.EncodeToString([]byte(s)) }

func TestDecodeMailBase64URL(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"padded", base64.URLEncoding.EncodeToString([]byte("你好 <b>")), "你好 <b>"},
		{"raw no padding", base64.RawURLEncoding.EncodeToString([]byte("hello world!")), "hello world!"},
		{"url alphabet", base64.URLEncoding.EncodeToString([]byte{0xe4, 0xbd, 0xa0, 0x3f, 0x3f, 0x3e}), "你??>"},
		{"empty", "", ""},
		{"not base64 kept", "plain text!", "plain text!"},
		{"decodes to invalid utf8 kept", base64.StdEncoding.EncodeToString([]byte{0xff, 0xfe, 0xfd}), base64.StdEncoding.EncodeToString([]byte{0xff, 0xfe, 0xfd})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := decodeMailBase64URL(tc.in); got != tc.want {
				t.Errorf("decodeMailBase64URL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSanitizeMailText(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"ansi csi", "a\x1b[31mred\x1b[0m b", "ared b"},
		{"osc hyperlink", "x\x1b]8;;https://evil\x07link\x1b]8;;\x07y", "xlinky"},
		{"crlf to lf", "l1\r\nl2\rl3", "l1\nl2l3"},
		{"bidi and zero width", "a\u202eb\u200bc\ufeffd\u2066e", "abcde"},
		{"c0 and c1 controls", "a\x00b\x07c\u0085d", "abcd"},
		{"keep tab newline cjk", "中\t文\n行", "中\t文\n行"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeMailText(tc.in); got != tc.want {
				t.Errorf("sanitizeMailText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestMailHTMLToText(t *testing.T) {
	in := "<html><head><style>.x{}</style></head><body>\n  <div>第一行 &amp; <b>粗</b></div>\n<p>第二段&nbsp;&lt;x&gt;</p><script>alert(1)</script>" +
		"<br/>尾巴<pre>a  b\n c</pre><!-- 注释 -->完</body></html>"
	got := mailHTMLToText(in)
	want := "第一行 & 粗\n第二段 <x>\n\n尾巴\na  b\n c\n完"
	if got != want {
		t.Errorf("mailHTMLToText =\n%q\nwant\n%q", got, want)
	}
}

// TestMailReadCmds_DecodeBodies 验证 message / messages / thread 默认把 base64url 正文解码为明文，
// 并清除 ANSI 控制序列；--raw-body 保留原值。回归：此前三条命令原样输出 base64。
func TestMailReadCmds_DecodeBodies(t *testing.T) {
	plainB64 := b64url("第一行\x1b[31m红\x1b[0m")
	htmlB64 := b64url("<p>你好 <b>世界</b></p>")
	msg := map[string]any{
		"message_id":      "m1",
		"subject":         "主题",
		"body_plain_text": plainB64,
		"body_html":       htmlB64,
		"internal_date":   "1790757108345",
		"head_from":       map[string]any{"mail_address": "a@example.com", "name": "A"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var data any
		switch {
		case strings.HasSuffix(r.URL.Path, "/messages/batch_get"):
			data = map[string]any{"messages": []any{msg}}
		case strings.Contains(r.URL.Path, "/threads/"):
			data = map[string]any{"thread": map[string]any{"thread_id": "t1", "messages": []any{msg}}}
		default:
			data = map[string]any{"message": msg}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "msg": "ok", "data": data})
	}))
	defer srv.Close()
	setupMailAttendanceCmdTestConfig(t, srv.URL)

	checkDecoded := func(t *testing.T, out string) {
		t.Helper()
		if strings.Contains(out, plainB64) || strings.Contains(out, htmlB64) {
			t.Fatalf("输出仍含 base64 原文: %s", out)
		}
		if !strings.Contains(out, "第一行红") {
			t.Errorf("body_plain_text 未解码或未清理 ANSI: %s", out)
		}
		if strings.Contains(out, "\\u001b") || strings.Contains(out, "\x1b") {
			t.Errorf("输出含 ESC 控制字符: %q", out)
		}
		if !strings.Contains(out, "<p>你好 <b>世界</b></p>") {
			t.Errorf("body_html 未解码: %s", out)
		}
	}

	t.Run("message json", func(t *testing.T) {
		_ = mailMessageCmd.Flags().Set("as", "user")
		_ = mailMessageCmd.Flags().Set("user-access-token", "u-test")
		_ = mailMessageCmd.Flags().Set("message-id", "m1")
		_ = mailMessageCmd.Flags().Set("output", "json")
		_ = mailMessageCmd.Flags().Set("raw-body", "false")
		var err error
		out := captureMailStdout(t, func() { err = mailMessageCmd.RunE(mailMessageCmd, nil) })
		if err != nil {
			t.Fatal(err)
		}
		checkDecoded(t, out)
	})
	t.Run("message raw-body keeps base64", func(t *testing.T) {
		_ = mailMessageCmd.Flags().Set("raw-body", "true")
		defer func() { _ = mailMessageCmd.Flags().Set("raw-body", "false") }()
		var err error
		out := captureMailStdout(t, func() { err = mailMessageCmd.RunE(mailMessageCmd, nil) })
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, plainB64) {
			t.Errorf("--raw-body 应保留原始 base64: %s", out)
		}
	})
	t.Run("message text mode", func(t *testing.T) {
		_ = mailMessageCmd.Flags().Set("output", "")
		defer func() { _ = mailMessageCmd.Flags().Set("output", "json") }()
		var err error
		out := captureMailStdout(t, func() { err = mailMessageCmd.RunE(mailMessageCmd, nil) })
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"主题: 主题", "发件人: A <a@example.com>", "你好 世界"} {
			if !strings.Contains(out, want) {
				t.Errorf("文本输出缺少 %q:\n%s", want, out)
			}
		}
		if strings.Contains(out, htmlB64) {
			t.Errorf("文本输出含 base64: %s", out)
		}
	})
	t.Run("messages json", func(t *testing.T) {
		_ = mailMessagesCmd.Flags().Set("as", "user")
		_ = mailMessagesCmd.Flags().Set("user-access-token", "u-test")
		_ = mailMessagesCmd.Flags().Set("message-ids", "m1")
		_ = mailMessagesCmd.Flags().Set("output", "json")
		var err error
		out := captureMailStdout(t, func() { err = mailMessagesCmd.RunE(mailMessagesCmd, nil) })
		if err != nil {
			t.Fatal(err)
		}
		checkDecoded(t, out)
	})
	t.Run("thread json", func(t *testing.T) {
		_ = mailThreadCmd.Flags().Set("as", "user")
		_ = mailThreadCmd.Flags().Set("user-access-token", "u-test")
		_ = mailThreadCmd.Flags().Set("thread-id", "t1")
		_ = mailThreadCmd.Flags().Set("output", "json")
		var err error
		out := captureMailStdout(t, func() { err = mailThreadCmd.RunE(mailThreadCmd, nil) })
		if err != nil {
			t.Fatal(err)
		}
		checkDecoded(t, out)
	})
}

// TestDecodeMailPayloadBodies_PreservesBigInt 解码时不得丢失大整数精度。
func TestDecodeMailPayloadBodies_PreservesBigInt(t *testing.T) {
	data := json.RawMessage(`{"message":{"message_id":"m","internal_date":17907571083451234567,"body_plain_text":"` + b64url("x") + `"}}`)
	payload, err := decodeMailPayloadBodies(data, false)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(payload)
	if !strings.Contains(string(out), "17907571083451234567") {
		t.Errorf("大整数精度丢失: %s", out)
	}
	if !strings.Contains(string(out), `"body_plain_text":"x"`) {
		t.Errorf("正文未解码: %s", out)
	}
}
