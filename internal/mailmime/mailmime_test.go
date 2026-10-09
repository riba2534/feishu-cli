package mailmime

import (
	"strings"
	"testing"
)

const sampleEML = "From: <me@example.com>\r\n" +
	"To: <a@example.com>\r\n" +
	"Subject: =?UTF-8?B?5rWL6K+V?=\r\n" +
	"In-Reply-To: <orig@example.com>\r\n" +
	"X-Lms-Reply-To-Message-Id: lms1\r\n" +
	"Content-Type: multipart/mixed;\r\n" +
	" boundary=outer\r\n" +
	"\r\n" +
	"--outer\r\n" +
	"Content-Type: multipart/alternative; boundary=inner\r\n" +
	"\r\n" +
	"--inner\r\n" +
	"Content-Type: text/plain; charset=UTF-8\r\n" +
	"Content-Transfer-Encoding: quoted-printable\r\n" +
	"\r\n" +
	"hello =E4=BD=A0=E5=A5=BD\r\n" +
	"--inner\r\n" +
	"Content-Type: text/html; charset=UTF-8\r\n" +
	"Content-Transfer-Encoding: base64\r\n" +
	"\r\n" +
	"PHA+aGVsbG88L3A+\r\n" +
	"--inner--\r\n" +
	"--outer\r\n" +
	"Content-Type: application/pdf; name=\"a.pdf\"\r\n" +
	"Content-Disposition: attachment; filename=\"a.pdf\"\r\n" +
	"Content-Transfer-Encoding: base64\r\n" +
	"\r\n" +
	"UERGREFUQQ==\r\n" +
	"--outer--\r\n"

func TestParseSerializeRoundTrip(t *testing.T) {
	p, err := Parse([]byte(sampleEML))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(p.Bytes()); got != sampleEML {
		t.Errorf("未修改时序列化结果应与原文逐字节一致\n--- got ---\n%q\n--- want ---\n%q", got, sampleEML)
	}
	if p.Boundary != "outer" || len(p.Children) != 2 || len(p.Children[0].Children) != 2 {
		t.Fatalf("结构解析错误: boundary=%q children=%d", p.Boundary, len(p.Children))
	}
	if got := p.Get("content-type"); got != "multipart/mixed; boundary=outer" {
		t.Errorf("折行头去折行失败: %q", got)
	}
}

func TestBodyPartsAndDecoded(t *testing.T) {
	p, err := Parse([]byte(sampleEML))
	if err != nil {
		t.Fatal(err)
	}
	plain, html := p.BodyParts()
	if plain == nil || html == nil {
		t.Fatal("未找到正文 part")
	}
	if b, _ := plain.Decoded(); string(b) != "hello 你好\r\n" && string(b) != "hello 你好" {
		t.Errorf("quoted-printable 解码错误: %q", b)
	}
	if b, _ := html.Decoded(); string(b) != "<p>hello</p>" {
		t.Errorf("base64 解码错误: %q", b)
	}
	if !p.Children[1].IsAttachment() {
		t.Error("附件 part 应识别为 attachment")
	}
}

func TestEditPreservesOtherParts(t *testing.T) {
	p, err := Parse([]byte(sampleEML))
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Set("Subject", "new subject"); err != nil {
		t.Fatal(err)
	}
	p.Del("To")
	_, html := p.BodyParts()
	if err := html.SetContent("text/html", []byte("<p>changed</p>")); err != nil {
		t.Fatal(err)
	}
	out := string(p.Bytes())
	for _, want := range []string{
		"Subject: new subject\r\n",
		"In-Reply-To: <orig@example.com>\r\n",
		"X-Lms-Reply-To-Message-Id: lms1\r\n",
		"Content-Disposition: attachment; filename=\"a.pdf\"\r\nContent-Transfer-Encoding: base64\r\n\r\nUERGREFUQQ==\r\n--outer--\r\n",
		"hello =E4=BD=A0=E5=A5=BD\r\n--inner",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("输出缺少 %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "To: <a@example.com>") {
		t.Error("Del(To) 未生效")
	}
	// 重新解析修改后的内容
	p2, err := Parse([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	_, html2 := p2.BodyParts()
	if b, _ := html2.Decoded(); string(b) != "<p>changed</p>" {
		t.Errorf("修改后的 HTML = %q", b)
	}
}

func TestSetRejectsCRLF(t *testing.T) {
	p, _ := Parse([]byte("Subject: a\r\n\r\nbody"))
	if err := p.Set("Subject", "x\r\nBcc: evil@example.com"); err == nil {
		t.Fatal("头字段值含 CRLF 应报错")
	}
}

func TestParseSinglePartLF(t *testing.T) {
	raw := "Subject: hi\nContent-Type: text/plain\n\nline1\nline2\n"
	p, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if string(p.Bytes()) != raw {
		t.Errorf("LF 换行的单 part 往返不一致: %q", p.Bytes())
	}
	plain, html := p.BodyParts()
	if plain != p || html != nil {
		t.Error("单 part 正文识别错误")
	}
}

func TestRemoveChild(t *testing.T) {
	p, _ := Parse([]byte(sampleEML))
	_, html := p.BodyParts()
	if !p.RemoveChild(html) {
		t.Fatal("RemoveChild 应找到 html part")
	}
	if _, h := p.BodyParts(); h != nil {
		t.Error("html part 未移除")
	}
}

func TestParseRejectsGarbageHeader(t *testing.T) {
	if _, err := Parse([]byte("not a header line\r\n\r\nbody")); err == nil {
		t.Error("无法解析的头字段应报错")
	}
}
