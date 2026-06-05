package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

// TestBuildHtmlboxRecordEscaping 验证含引号/尖括号/换行/中文的 HTML 被正确编码进 record JSON。
func TestBuildHtmlboxRecordEscaping(t *testing.T) {
	html := "<div class=\"x\" data-v='1'>中文 & <b>粗体</b>\n换行\t制表</div>"
	rec, err := buildHtmlboxRecord(html)
	if err != nil {
		t.Fatalf("buildHtmlboxRecord() error = %v", err)
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(rec), &m); err != nil {
		t.Fatalf("record 不是合法 JSON: %v\nrec=%s", err, rec)
	}
	if m["html"] != html {
		t.Fatalf("html 字段不一致:\n want %q\n got  %q", html, m["html"])
	}
}

// TestExtractHTMLFromRecord 验证 record → html 的解析与各种异常输入的兜底。
func TestExtractHTMLFromRecord(t *testing.T) {
	html := "<svg><animate attributeName=\"r\"/></svg>"
	rec, _ := buildHtmlboxRecord(html)
	if got := extractHTMLFromRecord(rec); got != html {
		t.Fatalf("extract 不一致: want %q got %q", html, got)
	}
	cases := map[string]string{
		"空字符串":      "",
		"非法 JSON":   "not-json{",
		"缺 html 字段": `{"foo":"bar"}`,
		"html 非字符串": `{"html":123}`,
	}
	for name, rec := range cases {
		if got := extractHTMLFromRecord(rec); got != "" {
			t.Fatalf("%s 应返回空，got %q", name, got)
		}
	}
}

// TestBuildHtmlboxBlock 验证构造出的块是 block_type=40 的 AddOns 块且字段正确。
func TestBuildHtmlboxBlock(t *testing.T) {
	rec, _ := buildHtmlboxRecord("<div/>")
	blk := buildHtmlboxBlock("blk_test123", rec)
	if blk.BlockType == nil || *blk.BlockType != blockTypeAddOns {
		t.Fatalf("block_type 应为 %d，got %v", blockTypeAddOns, blk.BlockType)
	}
	if blk.AddOns == nil {
		t.Fatalf("AddOns 不应为 nil")
	}
	if blk.AddOns.ComponentTypeId == nil || *blk.AddOns.ComponentTypeId != "blk_test123" {
		t.Fatalf("ComponentTypeId 不正确: %v", blk.AddOns.ComponentTypeId)
	}
	if blk.AddOns.Record == nil || *blk.AddOns.Record != rec {
		t.Fatalf("Record 不正确")
	}
}

// TestBuildHtmlboxRecordRoundtripUnicode 确认中文/emoji 在 record 往返中不被破坏。
func TestBuildHtmlboxRecordRoundtripUnicode(t *testing.T) {
	html := "<h1>飞书妙笔BOX 🎬 测试 — emoji ✅</h1>"
	rec, err := buildHtmlboxRecord(html)
	if err != nil {
		t.Fatalf("buildHtmlboxRecord() error = %v", err)
	}
	if got := extractHTMLFromRecord(rec); got != html {
		t.Fatalf("unicode roundtrip 失败:\n want %q\n got  %q", html, got)
	}
}

// newHTMLInputCmd 造一个带 --html / --html-file flag 的命令，用于测试 loadHTMLInput。
func newHTMLInputCmd(html, htmlFile string) *cobra.Command {
	c := &cobra.Command{}
	c.Flags().String("html", "", "")
	c.Flags().String("html-file", "", "")
	if html != "" {
		_ = c.Flags().Set("html", html)
	}
	if htmlFile != "" {
		_ = c.Flags().Set("html-file", htmlFile)
	}
	return c
}

// TestLoadHTMLInput 覆盖 --html/--html-file 互斥、空内容、无标签校验，
// 以及「不 TrimSpace 原文」这一保证——它是 get --raw 能逐字节还原写入内容的根。
func TestLoadHTMLInput(t *testing.T) {
	if _, err := loadHTMLInput(newHTMLInputCmd("<a>", "f.html")); err == nil {
		t.Fatal("同时传 --html 与 --html-file 应报错")
	}
	for _, in := range []string{"", "   \n\t  "} {
		if _, err := loadHTMLInput(newHTMLInputCmd(in, "")); err == nil {
			t.Fatalf("空/纯空白内容 %q 应报错", in)
		}
	}
	if _, err := loadHTMLInput(newHTMLInputCmd("hello world", "")); err == nil {
		t.Fatal("无 < 标签的内容应报错")
	}
	// 关键保证：首尾空白/换行必须原样保留，不能被 TrimSpace 掉。
	raw := "  \n<div>ok</div>\n\n  "
	got, err := loadHTMLInput(newHTMLInputCmd(raw, ""))
	if err != nil {
		t.Fatalf("合法 HTML 不应报错: %v", err)
	}
	if got != raw {
		t.Fatalf("loadHTMLInput 改动了原文（应逐字节保留）:\n want %q\n got  %q", raw, got)
	}
}

// TestLoadHTMLInputFromFile 验证 --html-file 读取且逐字节保留原文（含首尾换行）。
func TestLoadHTMLInputFromFile(t *testing.T) {
	raw := "\n<section>\n  <b>x</b>\n</section>\n"
	p := filepath.Join(t.TempDir(), "w.html")
	if err := os.WriteFile(p, []byte(raw), 0o644); err != nil {
		t.Fatalf("写临时文件失败: %v", err)
	}
	got, err := loadHTMLInput(newHTMLInputCmd("", p))
	if err != nil {
		t.Fatalf("读取 --html-file 失败: %v", err)
	}
	if got != raw {
		t.Fatalf("--html-file 内容应逐字节保留:\n want %q\n got  %q", raw, got)
	}
}

// TestBuildHtmlboxRecordRoundtripScript 确认含 </script>、HTML 注释、U+2028/2029 的 JS payload
// 往返不被破坏——htmlbox 的核心用途就是塞可执行 JS，这类 payload 必须能逐字节还原。
func TestBuildHtmlboxRecordRoundtripScript(t *testing.T) {
	html := "<script>var s='</script>';/*<!--*/var u='\u2028\u2029';if(a<b){doit();}</script>"
	rec, err := buildHtmlboxRecord(html)
	if err != nil {
		t.Fatalf("buildHtmlboxRecord() error = %v", err)
	}
	if got := extractHTMLFromRecord(rec); got != html {
		t.Fatalf("script payload roundtrip 失败:\n want %q\n got  %q", html, got)
	}
}
