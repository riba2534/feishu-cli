package cmd

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

func writeDocTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// writeSizedPNG 写一张 w×h 的 PNG。
func writeSizedPNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareHTML5BlockRefs(t *testing.T) {
	chdirTemp(t)
	writeDocTestFile(t, "widget.html", "<html><body>hello</body></html>")
	writeDocTestFile(t, "two.html", "<p>two</p>")

	for _, format := range []string{"xml", "markdown"} {
		in, err := prepareDocsAIWriteInput(
			"before\n<html5-block path=\"@widget.html\"></html5-block>\n<html5-block path='@./two.html' alt=\"图\"/>\nafter",
			nil, docsAIWriteOptions{Format: format})
		if err != nil {
			t.Fatalf("[%s] prepare 失败: %v", format, err)
		}
		if !strings.Contains(in.Content, `<html5-block data-ref="html5_1"></html5-block>`) ||
			!strings.Contains(in.Content, `<html5-block alt="图" data-ref="html5_2"></html5-block>`) {
			t.Fatalf("[%s] 内容未改写为 data-ref:\n%s", format, in.Content)
		}
		group, _ := in.ReferenceMap[html5BlockTag].(map[string]html5RefEntry)
		if group["html5_1"].Data != "<html><body>hello</body></html>" || group["html5_2"].Data != "<p>two</p>" {
			t.Fatalf("[%s] reference_map 异常: %#v", format, in.ReferenceMap)
		}
	}

	// 已有 data-ref 配合 --reference-map；新 path 分配的 ref 跳过已占用的 html5_1；通用分组原样保留
	refMap := map[string]any{
		html5BlockTag: map[string]any{"html5_1": map[string]any{"data": "<i>old</i>"}},
		"widget":      map[string]any{"r1": map[string]any{"label": "x"}},
	}
	in, err := prepareDocsAIWriteInput(`<html5-block data-ref="html5_1"/><html5-block path="@widget.html"/>`, refMap, docsAIWriteOptions{Format: "xml"})
	if err != nil {
		t.Fatalf("prepare 失败: %v", err)
	}
	if !strings.Contains(in.Content, `<html5-block data-ref="html5_2"></html5-block>`) {
		t.Fatalf("新 ref 应跳过已占用编号: %s", in.Content)
	}
	b, _ := json.Marshal(in.ReferenceMap)
	if !strings.Contains(string(b), `"widget":{"r1":{"label":"x"}}`) || !strings.Contains(string(b), `"html5_1":{"data":"\u003ci\u003eold\u003c/i\u003e"}`) {
		t.Fatalf("reference_map 应保留通用分组与已有条目: %s", b)
	}
	if _, ok := refMap[html5BlockTag].(map[string]any); !ok {
		t.Fatal("不应修改调用方传入的 reference_map")
	}

	// reference_map 中 html5-block 条目的 path 读取为 data
	in, err = prepareDocsAIWriteInput(`<html5-block data-ref="h"/>`,
		map[string]any{html5BlockTag: map[string]any{"h": map[string]any{"path": "@widget.html"}}}, docsAIWriteOptions{Format: "xml"})
	if err != nil {
		t.Fatalf("reference_map path 解析失败: %v", err)
	}
	if g := in.ReferenceMap[html5BlockTag].(map[string]html5RefEntry); g["h"].Data == "" || g["h"].Path != "" {
		t.Fatalf("reference_map path 应换成 data: %#v", g)
	}

	// 围栏代码块内不处理（markdown）
	fenced := "```xml\n<html5-block path=\"@missing.html\"/>\n```\n"
	if in, err := prepareDocsAIWriteInput(fenced, nil, docsAIWriteOptions{Format: "markdown"}); err != nil || in.Content != fenced || in.ReferenceMap != nil {
		t.Fatalf("围栏内的 html5-block 应原样保留: %v %q", err, in.Content)
	}

	bad := []struct {
		content string
		refMap  map[string]any
		want    string
	}{
		{`<html5-block data-ref="nope"/>`, nil, "reference_map.html5-block.nope"},
		{`<html5-block data="x"/>`, nil, "保留给内部使用"},
		{`<html5-block path="@widget.html" data-ref="a"/>`, nil, "不能同时包含"},
		{`<html5-block/>`, nil, "需要 path"},
		{`<html5-block path="@widget.html">内联</html5-block>`, nil, "不能写内容"},
		{`<html5-block path="widget.html"/>`, nil, "必须以 @ 开头"},
		{`<html5-block path="@widget.txt"/>`, nil, ".html"},
		{`<html5-block path="@missing.html"/>`, nil, "不存在"},
		{`<html5-block path="@/etc/x.html"/>`, nil, "敏感"},
		{`<html5-block data-ref="h"/>`, map[string]any{html5BlockTag: map[string]any{"h": map[string]any{"data": "a", "path": "@widget.html"}}}, "只能使用 data 或 path"},
		{`<html5-block data-ref="h"/>`, map[string]any{html5BlockTag: "x"}, "必须是"},
	}
	for _, tc := range bad {
		_, err := prepareDocsAIWriteInput(tc.content, tc.refMap, docsAIWriteOptions{Format: "xml"})
		if err == nil || !strings.Contains(err.Error(), tc.want) || !clierr.HasKind(err, clierr.KindUsage) {
			t.Errorf("%s 期望用法错误含 %q，得到 %v", tc.content, tc.want, err)
		}
	}
}

func TestPrepareWhiteboardFileRefs(t *testing.T) {
	chdirTemp(t)
	writeDocTestFile(t, "diagram.svg", `<svg viewBox="0 0 10 10"><text>A</text></svg>`)
	writeDocTestFile(t, "flow.mmd", "flowchart TD\nA --> B & C")
	writeDocTestFile(t, "seq.puml", "@startuml\nAlice -> Bob: hi\n@enduml")

	content := strings.Join([]string{
		`<whiteboard type="svg" path="@diagram.svg"/>`,
		`<whiteboard type="mermaid">@flow.mmd</whiteboard>`,
		`<whiteboard type="plantUML" path="@seq.puml"></whiteboard>`,
		`<whiteboard type="blank"></whiteboard>`,
		`<whiteboard type="mermaid">graph LR; X-->Y</whiteboard>`,
	}, "\n")
	in, err := prepareDocsAIWriteInput(content, nil, docsAIWriteOptions{Format: "xml"})
	if err != nil {
		t.Fatalf("prepare 失败: %v", err)
	}
	for _, want := range []string{
		`<whiteboard type="svg"><svg viewBox="0 0 10 10"><text>A</text></svg></whiteboard>`,
		"<whiteboard type=\"mermaid\">flowchart TD\nA --> B &amp; C</whiteboard>",
		"<whiteboard type=\"plantuml\">@startuml\nAlice -> Bob: hi\n@enduml</whiteboard>",
		`<whiteboard type="blank"></whiteboard>`,
		`<whiteboard type="mermaid">graph LR; X-->Y</whiteboard>`,
	} {
		if !strings.Contains(in.Content, want) {
			t.Fatalf("内容缺少 %q:\n%s", want, in.Content)
		}
	}
	if strings.Contains(in.Content, `path="@`) || in.ReferenceMap != nil {
		t.Fatalf("画板文件引用应展开且不产生 reference_map: %s", in.Content)
	}

	// 自闭合画板之后紧跟另一个画板：不能跨元素匹配到后一个闭合标签
	in, err = prepareDocsAIWriteInput(`<whiteboard type="svg" path="@diagram.svg"/><p>中间</p><whiteboard type="mermaid">graph TD; A-->B</whiteboard>`, nil, docsAIWriteOptions{Format: "xml"})
	if err != nil || !strings.Contains(in.Content, `</svg></whiteboard><p>中间</p><whiteboard type="mermaid">graph TD; A-->B</whiteboard>`) {
		t.Fatalf("相邻画板处理异常: %v\n%s", err, in.Content)
	}

	// 全部错误一次汇总
	_, err = prepareDocsAIWriteInput(`<whiteboard type="svg" path="@a.svg"/><whiteboard type="mermaid" path="@b.mmd"/>`, nil, docsAIWriteOptions{Format: "xml"})
	if err == nil || !strings.Contains(err.Error(), "a.svg") || !strings.Contains(err.Error(), "b.mmd") || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("应汇总全部缺失路径: %v", err)
	}
	bad := map[string]string{
		`<whiteboard path="@diagram.svg"/>`:                         "只支持 type",
		`<whiteboard type="svg" path="@flow.mmd"/>`:                 "必须指向 .svg",
		`<whiteboard type="svg" path="diagram.svg"/>`:               "必须以 @ 开头",
		`<whiteboard type="svg" path="@diagram.svg">x</whiteboard>`: "不能同时写 path 与内联内容",
		`<whiteboard type="svg" path="@/etc/x.svg"/>`:               "敏感",
	}
	for c, want := range bad {
		if _, err := prepareDocsAIWriteInput(c, nil, docsAIWriteOptions{Format: "xml"}); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s 期望错误含 %q，得到 %v", c, want, err)
		}
	}
}

func TestReadReferenceMapFlag(t *testing.T) {
	dir := chdirTemp(t)
	writeDocTestFile(t, filepath.Join(dir, "ref.json"), `{"html5-block":{"html5_1":{"data":"<b>x</b>"}}}`)
	m, err := readReferenceMapFlag("@ref.json", false)
	if err != nil || m[html5BlockTag] == nil {
		t.Fatalf("@file 读取失败: %v %#v", err, m)
	}
	orig := docWriteStdin
	docWriteStdin = strings.NewReader(`{"a":{}}`)
	t.Cleanup(func() { docWriteStdin = orig })
	if m, err := readReferenceMapFlag("-", false); err != nil || m["a"] == nil {
		t.Fatalf("stdin 读取失败: %v %#v", err, m)
	}
	if m, err := readReferenceMapFlag("null", true); err != nil || m != nil {
		t.Fatalf("create 场景 null 应视为未提供: %v %#v", err, m)
	}
	for _, raw := range []string{"", "null", "[1]", "{bad", "@missing.json", "@"} {
		if _, err := readReferenceMapFlag(raw, false); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
			t.Errorf("%q 应为用法错误，得到 %v", raw, err)
		}
	}
}

// TestDocResourceFallbackToSourceDir XML 内容来自文件时，@ 相对路径在当前目录不存在则回退到文件所在目录；
// Markdown 不回退（对齐官方 statDocResource）。
func TestDocResourceFallbackToSourceDir(t *testing.T) {
	chdirTemp(t)
	src := filepath.Join("drafts", "doc.xml")
	writeDocTestFile(t, filepath.Join("drafts", "w.html"), "<p>w</p>")
	writeSizedPNG(t, filepath.Join("drafts", "a.png"), 2, 2)

	in, err := prepareDocsAIWriteInput(`<html5-block path="@w.html"/><img path="@a.png"/>`, nil, docsAIWriteOptions{Format: "xml", SourceFile: src})
	if err != nil {
		t.Fatalf("XML 应回退到内容文件目录: %v", err)
	}
	if len(in.Resources) != 1 || in.Resources[0].Path != filepath.Join("drafts", "a.png") {
		t.Fatalf("图片路径未回退: %+v", in.Resources)
	}
	if _, err := prepareDocsAIWriteInput(`<html5-block path="@w.html"/>`, nil, docsAIWriteOptions{Format: "markdown", SourceFile: src}); err == nil {
		t.Fatal("Markdown 不应回退到内容文件目录")
	}
	// 当前目录存在同名文件时优先当前目录
	writeDocTestFile(t, "w.html", "<p>cwd</p>")
	in, err = prepareDocsAIWriteInput(`<html5-block path="@w.html"/>`, nil, docsAIWriteOptions{Format: "xml", SourceFile: src})
	if err != nil || in.ReferenceMap[html5BlockTag].(map[string]html5RefEntry)["html5_1"].Data != "<p>cwd</p>" {
		t.Fatalf("应优先当前目录: %v %#v", err, in.ReferenceMap)
	}
}

func TestNormalizeDocImagePresentation(t *testing.T) {
	cases := []struct {
		attrs      string
		w, h       int
		wantScale  string // 空表示不带 scale
		wantWidthH string
	}{
		{``, 800, 600, "", `width="800" height="600"`},
		{``, 2040, 1000, "0.499999", `width="2040" height="1000"`},
		{` width="600"`, 1200, 800, "0.500000", `width="1200" height="800"`},
		{` width="50%"`, 1200, 800, "0.500000", `width="1200" height="800"`},
		{` height="200"`, 1200, 800, "0.250000", `width="1200" height="800"`},
		{` scale="0.3" width="900"`, 1200, 800, "0.300000", `width="1200" height="800"`},
		{` width="5000"`, 1200, 800, "0.849999", `width="1200" height="800"`},
		{` width="abc" scale="-1"`, 300, 200, "", `width="300" height="200"`},
	}
	for _, tc := range cases {
		tag := parseStartTag(`<img path="@a.png"` + tc.attrs + `/>`)
		normalizeDocImagePresentation(&tag, tc.w, tc.h)
		out := tag.render(true)
		scale, has := tag.get("scale")
		w, _ := tag.get("width")
		h, _ := tag.get("height")
		if (tc.wantScale == "") == has || (has && scale != tc.wantScale) || `width="`+w+`" height="`+h+`"` != tc.wantWidthH {
			t.Errorf("attrs=%q %dx%d → %s（期望 scale=%q）", tc.attrs, tc.w, tc.h, out, tc.wantScale)
		}
	}
}

// TestLocalResourceStrictVsLenient doc create（Strict）按官方严格校验；content-update 保持既有宽松行为。
func TestLocalResourceStrictVsLenient(t *testing.T) {
	chdirTemp(t)
	writeSizedPNG(t, "a.png", 1200, 800)
	writeDocTestFile(t, "fake.png", "not an image")
	writeDocTestFile(t, "r.txt", "附件")

	strict := docsAIWriteOptions{Format: "xml", Strict: true}
	lenient := docsAIWriteOptions{Format: "xml"}

	in, err := prepareDocsAIWriteInput(`<img path="@a.png" alt="说明" width="600" align="center"/>`, nil, strict)
	if err != nil {
		t.Fatalf("strict 正常图片失败: %v", err)
	}
	r := in.Resources[0]
	if !regexp.MustCompile(`^<img path="@lcli_img_[0-9a-f]{32}" caption="说明" width="1200" align="center" height="800" scale="0.500000"/>$`).MatchString(in.Content) {
		t.Fatalf("归一化/alt→caption 异常: %s", in.Content)
	}
	if r.Width != 1200 || r.Height != 800 || !r.HasScale || r.Scale != 0.5 || r.Align != "center" {
		t.Fatalf("资源展示参数异常: %+v", r)
	}
	req := r.replaceImageRequest("tok")
	if b, _ := json.Marshal(req); string(b) != `{"align":2,"height":800,"scale":0.5,"token":"tok","width":1200}` {
		t.Fatalf("replace_image 异常: %s", b)
	}

	strictBad := map[string]string{
		`<img path="@fake.png"/>`:                 "不是可识别",
		`![x](@fake.png)`:                         "不是可识别",
		`<img path="./a.png"/>`:                   "必须以 @ 开头",
		`<img path="@a.png" src="https://x/y"/>`:  "不能与 src",
		`<source path="@r.txt" name="../x.txt"/>`: "不含路径分隔符",
	}
	for c, want := range strictBad {
		opts := strict
		if strings.HasPrefix(c, "!") {
			opts.Format = "markdown"
		}
		if _, err := prepareDocsAIWriteInput(c, nil, opts); err == nil || !strings.Contains(err.Error(), want) || !clierr.HasKind(err, clierr.KindUsage) {
			t.Errorf("strict %s 期望用法错误含 %q，得到 %v", c, want, err)
		}
	}
	// 宽松模式（content-update 既有行为）：非 @ path 原样保留；无法识别的图片照常上传；非法 name 忽略
	in, err = prepareDocsAIWriteInput(`<img path="./a.png"/><img path="@fake.png" width="300"/><source path="@r.txt" name="../x.txt"/>`, nil, lenient)
	if err != nil {
		t.Fatalf("lenient 不应报错: %v", err)
	}
	if !strings.HasPrefix(in.Content, `<img path="./a.png"/>`) || len(in.Resources) != 2 || in.Resources[0].Width != 300 || in.Resources[1].FileName != "r.txt" {
		t.Fatalf("lenient 结果异常: %s %+v", in.Content, in.Resources)
	}
	// HTML 注释中的本地资源不处理
	in, err = prepareDocsAIWriteInput("<!-- <img path=\"@missing.png\"/>\n多行 -->\n<img path=\"@a.png\"/>", nil, strict)
	if err != nil || len(in.Resources) != 1 || !strings.HasPrefix(in.Content, "<!-- <img path=\"@missing.png\"/>\n多行 -->\n") {
		t.Fatalf("注释应原样保留: %v %q", err, in.Content)
	}
}

// TestPrepareRemoteImageHref <img href="https://..."/> 改写为占位标签，显示参数留到下载后归一化；
// Markdown 的 ![](https://...) 仍交给服务端，不产生资源。
func TestPrepareRemoteImageHref(t *testing.T) {
	for _, strict := range []bool{true, false} {
		opts := docsAIWriteOptions{Format: "xml", Strict: strict}
		in, err := prepareDocsAIWriteInput(`<img href="https://img.example.com/a.png?sig=secret#f" alt="远程" width="600" align="center"/>`, nil, opts)
		if err != nil {
			t.Fatalf("strict=%v href 改写失败: %v", strict, err)
		}
		if !regexp.MustCompile(`^<img path="@lcli_img_[0-9a-f]{32}" caption="远程"/>$`).MatchString(in.Content) {
			t.Fatalf("strict=%v 改写结果异常: %s", strict, in.Content)
		}
		r := in.Resources[0]
		if r.RemoteURL != "https://img.example.com/a.png?sig=secret#f" || r.URL != "https://img.example.com/a.png" || r.Path != "" ||
			len(r.requested) != 2 || r.requested[0] != (tagAttr{Name: "width", Value: "600"}) {
			t.Fatalf("strict=%v 资源异常: %+v", strict, r)
		}
	}
	md := "![网络图](https://img.example.com/a.png)"
	if in, err := prepareDocsAIWriteInput(md, nil, docsAIWriteOptions{Format: "markdown", Strict: true}); err != nil || in.Content != md || len(in.Resources) != 0 {
		t.Fatalf("Markdown 网络图片应原样交给服务端: %v %q", err, in.Content)
	}
	bad := map[string]string{
		`<img href="https://a.example.com/x.png" src="tok"/>`:   "不能与 src",
		`<img href="https://a.example.com/x.png" token="t"/>`:   "不能与 token",
		`<img href="ftp://a.example.com/x.png"/>`:               "绝对 http(s) URL",
		`<img href="https://u:p@a.example.com/x.png"/>`:         "不带用户名密码",
		`<img href="./x.png"/>`:                                 "绝对 http(s) URL",
		`<img href="https://a.example.com/x.png" img_key="k"/>`: "不能与 img_key",
	}
	for c, want := range bad {
		if _, err := prepareDocsAIWriteInput(c, nil, docsAIWriteOptions{Format: "xml"}); err == nil || !strings.Contains(err.Error(), want) || !clierr.HasKind(err, clierr.KindUsage) {
			t.Errorf("%s 期望用法错误含 %q，得到 %v", c, want, err)
		}
	}
}
