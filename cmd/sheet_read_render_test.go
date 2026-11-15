package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/client"
)

// TestFormatSheetCellText 文本模式下富文本段渲染为 text(link)，不再输出 Go 的 map[...] 语法。
func TestFormatSheetCellText(t *testing.T) {
	var cell any
	_ = json.Unmarshal([]byte(`[{"cellPosition":null,"link":"https://example.com/a","text":"官网","type":"url"},{"text":" 与 ","type":"text"},{"text":"@张三","type":"mention"}]`), &cell)
	cases := []struct {
		name string
		cell any
		want string
	}{
		{"富文本段", cell, "官网(https://example.com/a) 与 @张三"},
		{"链接文本即 URL", []any{map[string]any{"text": "https://x.cn", "link": "https://x.cn"}}, "https://x.cn"},
		{"只有 link", []any{map[string]any{"link": "https://x.cn"}}, "https://x.cn"},
		{"单个对象", map[string]any{"text": "a", "link": "https://b"}, "a(https://b)"},
		{"字符串", "hello", "hello"},
		{"数字", float64(42), "42"},
		{"nil", nil, "<nil>"},
		{"无法识别的对象回退 %v", []any{map[string]any{"foo": "bar"}}, "map[foo:bar]"},
	}
	for _, c := range cases {
		if got := formatSheetCellText(c.cell); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// TestSheetRead_TextRendersRichCells sheet read 文本模式输出 text(link)，-o json 保持接口原样。
func TestSheetRead_TextRendersRichCells(t *testing.T) {
	m := newSheetMockServer(t)
	m.respond = func(w http.ResponseWriter, r *http.Request, body []byte) bool {
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"valueRange":{"range":"s1!A1:B1","values":[["名称",[{"cellPosition":null,"link":"https://example.com/a","text":"官网","type":"url"}]]]}}}`)
		return true
	}
	out, err := runSheetCmdForTest(t, sheetReadCmd, []string{"shtTok", "s1!A1:B1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "名称 | 官网(https://example.com/a)") || strings.Contains(out, "map[") {
		t.Errorf("文本输出 = %q", out)
	}
	out, err = runSheetCmdForTest(t, sheetReadCmd, []string{"shtTok", "s1!A1:B1"}, map[string]string{"output": "json"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"link": "https://example.com/a"`) || !strings.Contains(out, `"type": "url"`) {
		t.Errorf("JSON 输出应保留富文本结构: %s", out)
	}
}

// TestWarnSheetUnsupportedCells read-rich 遇到 #UNSUPPORTED_TYPE 时在 stderr 列出单元格地址。
func TestWarnSheetUnsupportedCells(t *testing.T) {
	var buf bytes.Buffer
	res := []*client.CellRangeV3{{Range: "s1!C2:D3", Values: [][][]*client.CellElement{
		{{{Type: client.UnsupportedCellType}}, {{Type: "text", Text: &client.TextElement{Text: "x"}}}},
		{nil, {{Type: client.UnsupportedCellType}}},
	}}}
	warnSheetUnsupportedCells(&buf, res, []string{"s1!C2:D3"})
	if got := buf.String(); !strings.Contains(got, "C2, D3") || !strings.Contains(got, "read-plain") {
		t.Errorf("告警 = %q", got)
	}
	buf.Reset()
	warnSheetUnsupportedCells(&buf, []*client.CellRangeV3{{Range: "s1!A1"}}, nil)
	if buf.Len() != 0 {
		t.Errorf("无不支持类型时不应告警: %q", buf.String())
	}
}

// TestSheetReadRich_WarnsUnsupported read-rich 命令遇到 #UNSUPPORTED_TYPE 时 stderr 告警，stdout JSON 不变。
func TestSheetReadRich_WarnsUnsupported(t *testing.T) {
	m := newSheetMockServer(t)
	m.respond = func(w http.ResponseWriter, r *http.Request, body []byte) bool {
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"value_ranges":[{"range":"s1!C2:C3","values":[[[{"type":"#UNSUPPORTED_TYPE"}]],[[{"type":"#UNSUPPORTED_TYPE"}]]]}]}}`)
		return true
	}
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	out, runErr := runSheetCmdForTest(t, sheetReadRichCmd, []string{"shtTok", "s1", "C2:C3"}, map[string]string{"output": "json"})
	os.Stderr = oldStderr
	_ = w.Close()
	errOut, _ := io.ReadAll(r)
	if runErr != nil {
		t.Fatal(runErr)
	}
	if !strings.Contains(string(errOut), "C2, C3") {
		t.Errorf("stderr 应列出不支持类型的单元格: %q", errOut)
	}
	if strings.Contains(out, "警告") || !strings.Contains(out, "#UNSUPPORTED_TYPE") {
		t.Errorf("stdout 应只含原样 JSON: %s", out)
	}
}
