// 部分实现改编自 larksuite/cli（MIT License, Copyright (c) 2026 Lark Technologies Pte. Ltd.）
// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/docxparse"
	"github.com/spf13/pflag"
)

func resetDocScriptFlags() {
	docScriptCmd.Flags().VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue); f.Changed = false })
}

// runDocScriptForTest 设置 flag 后直接调用 RunE，返回 stdout 与错误。
func runDocScriptForTest(t *testing.T, flags map[string]string) (string, error) {
	t.Helper()
	resetDocScriptFlags()
	t.Cleanup(resetDocScriptFlags)
	for name, value := range flags {
		if err := docScriptCmd.Flags().Set(name, value); err != nil {
			t.Fatalf("设置 --%s 失败: %v", name, err)
		}
	}
	var runErr error
	out := captureStdout(t, func() { runErr = docScriptCmd.RunE(docScriptCmd, nil) })
	return out, runErr
}

func chdirDocScriptTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	return dir
}

func decodeDocScriptParse(t *testing.T, out string) docScriptParseResult {
	t.Helper()
	var result docScriptParseResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("解析输出失败: %v\n%s", err, out)
	}
	return result
}

func docScriptTestBlockCount(blocks []docxparse.BlockShare, typ string) int {
	for _, block := range blocks {
		if block.Type == typ {
			return block.Count
		}
	}
	return 0
}

func requireDocScriptUsage(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("应报错（包含 %q）", want)
	}
	if !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("错误应为用法错误（退出码 2）: %v", err)
	}
	if want != "" && !strings.Contains(err.Error(), want) {
		t.Fatalf("错误 = %v，应包含 %q", err, want)
	}
}

func writeDocScriptPNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
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

func TestDocScriptParsesAndProfilesXML(t *testing.T) {
	out, err := runDocScriptForTest(t, map[string]string{
		"command": "parse",
		"content": `<title>标题</title><p>一个苹果是 an apple。</p>`,
	})
	if err != nil {
		t.Fatalf("parse 失败: %v", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &top); err != nil {
		t.Fatal(err)
	}
	if len(top) != 2 || top["assessment"] == nil || top["profile"] == nil {
		t.Fatalf("输出应只含 assessment 与 profile: %s", out)
	}
	var profileFields map[string]json.RawMessage
	_ = json.Unmarshal(top["profile"], &profileFields)
	if len(profileFields) != 4 || profileFields["breakdown"] != nil {
		t.Fatalf("profile 字段应为 4 个且不暴露 breakdown: %s", top["profile"])
	}
	result := decodeDocScriptParse(t, out)
	if result.Assessment.Status != docScriptAssessmentPassed {
		t.Fatalf("assessment = %+v", result.Assessment)
	}
	p := result.Profile
	if p.WordCount != 10 || p.CharCount != 15 || p.BlockCount != 2 ||
		docScriptTestBlockCount(p.Blocks, "title") != 1 || docScriptTestBlockCount(p.Blocks, "p") != 1 {
		t.Fatalf("profile = %+v", p)
	}
}

func TestDocScriptReturnsPresentationDecisionDiagnostics(t *testing.T) {
	decision := `{
		"audience": "reader", "reader_task": "understand", "genre_contract": "none", "adapter": null,
		"presentation_mode": "rich", "word_count": {"min": 18, "max": 22},
		"visual_plan": {"reason": "visual", "blocks": [
			{"type":"img","min_count":1,"purpose":"show the result"},
			{"type":"whiteboard","min_count":1,"purpose":"show the flow"},
			{"type":"html5-block","min_count":1,"purpose":"make the state explorable"}
		]}
	}`
	out, err := runDocScriptForTest(t, map[string]string{
		"command":               "parse",
		"content":               `<title>标题</title><p>一个苹果是 an apple。</p><img/>`,
		"presentation-decision": decision,
	})
	if err != nil {
		t.Fatalf("诊断结果不应让命令失败: %v", err)
	}
	result := decodeDocScriptParse(t, out)
	if result.Assessment.Status != docScriptAssessmentFailed || len(result.Diagnostics) != 3 {
		t.Fatalf("应 failed 且有 3 条诊断: %s", out)
	}
	wc := result.Diagnostics[0]
	if wc.Code != docScriptCodeWordCountRange || wc.Severity != docScriptDiagnosticError ||
		wc.Expected == nil || *wc.Expected.Min != 18 || *wc.Expected.Max != 22 || wc.Actual == nil || *wc.Actual != 10 ||
		wc.Suggested != "把字数调整到 18-22 之间。" {
		t.Fatalf("字数诊断 = %+v", wc)
	}
	for i, want := range []string{"whiteboard", "html5-block"} {
		d := result.Diagnostics[i+1]
		if d.Code != docScriptCodeRequiredBlock || d.Expected == nil || d.Expected.Type != want ||
			d.Expected.MinCount != 1 || d.Actual == nil || *d.Actual != 0 {
			t.Fatalf("%s 诊断 = %+v", want, d)
		}
	}
	if !strings.Contains(result.Diagnostics[1].Msg, "用途：show the flow") {
		t.Fatalf("诊断应带 purpose: %+v", result.Diagnostics[1])
	}
}

func TestDocScriptBlockedRemoteImagesAreGrouped(t *testing.T) {
	decision := `{"visual_plan":{"blocks":[{"type":"img","min_count":1}]}}`
	out, err := runDocScriptForTest(t, map[string]string{
		"command":               "parse",
		"content":               `<title>Draft</title><img href="http://127.0.0.1/one.png"/><img href="http://127.0.0.1/two.png"/>`,
		"presentation-decision": decision,
	})
	if err != nil {
		t.Fatal(err)
	}
	result := decodeDocScriptParse(t, out)
	if result.Assessment.Status != docScriptAssessmentFailed || len(result.Diagnostics) != 1 {
		t.Fatalf("应合并为一条图片诊断: %s", out)
	}
	d := result.Diagnostics[0]
	if d.Code != docScriptCodeImageSource || len(d.ImageIndices) != 2 || d.ImageIndices[0] != 1 || d.ImageIndices[1] != 2 ||
		d.Msg != "不允许访问本地/内网地址" || !strings.Contains(d.Suggested, "下载到草稿工作区") {
		t.Fatalf("图片诊断 = %+v", d)
	}
	if strings.Contains(d.Msg, "#") {
		t.Fatalf("msg 不应重复图片序号前缀: %q", d.Msg)
	}
}

func TestDocScriptRemoteImageDiagnosticMapping(t *testing.T) {
	orig := docScriptProbeRemoteImage
	t.Cleanup(func() { docScriptProbeRemoteImage = orig })
	results := map[string]error{
		"https://example.com/a.png": &client.RemoteImageProbeError{Kind: client.RemoteImageUnavailable, Reason: "HTTP 404"},
		"https://example.com/b.png": &client.RemoteImageProbeError{Kind: client.RemoteImageFormat, Reason: `响应的 Content-Type "text/html" 不是支持的图片类型`},
		"https://example.com/c.png": &client.RemoteImageProbeError{Kind: client.RemoteImageTooLarge, Reason: "超过 20MiB 上限"},
		"https://example.com/d.png": nil,
		"https://example.com/e.png": &client.RemoteImageProbeError{Kind: client.RemoteImageUnavailable, Reason: "HTTP 404"},
		"https://example.com/f.png": errors.New("未知错误"),
	}
	var probed []string
	docScriptProbeRemoteImage = func(rawURL string) error {
		probed = append(probed, rawURL)
		return results[rawURL]
	}
	content := `<title>T</title>`
	for _, name := range []string{"a", "b", "c", "d", "e", "f"} {
		content += fmt.Sprintf(`<img href="https://example.com/%s.png"/>`, name)
	}
	diagnostics := docScriptResourceDiagnostics(content, "")
	if len(probed) != 6 {
		t.Fatalf("应逐个探测 6 张图片: %v", probed)
	}
	want := []struct {
		code    string
		indices []int
	}{
		{docScriptCodeImageMissing, []int{1, 5}},
		{docScriptCodeImageFormat, []int{2}},
		{docScriptCodeImageTooLarge, []int{3}},
		{docScriptCodeImagePreflight, []int{6}},
	}
	if len(diagnostics) != len(want) {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
	for i, w := range want {
		if diagnostics[i].Code != w.code || fmt.Sprint(diagnostics[i].ImageIndices) != fmt.Sprint(w.indices) {
			t.Fatalf("第 %d 条诊断 = %+v，期望 %s %v", i, diagnostics[i], w.code, w.indices)
		}
	}
	if diagnostics[0].Msg != "HTTP 404" {
		t.Fatalf("msg 应为探测原因: %+v", diagnostics[0])
	}
}

func TestDocScriptParseWithoutDecisionSkipsResourcePreflight(t *testing.T) {
	orig := docScriptProbeRemoteImage
	t.Cleanup(func() { docScriptProbeRemoteImage = orig })
	docScriptProbeRemoteImage = func(string) error { t.Fatal("没有决策时不应探测远程图片"); return nil }
	out, err := runDocScriptForTest(t, map[string]string{
		"command": "parse",
		"content": `<title>Draft</title><img href="http://127.0.0.1/image.png"/><img path="@missing.png"/>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	result := decodeDocScriptParse(t, out)
	if result.Assessment.Status != docScriptAssessmentPassed || len(result.Diagnostics) != 0 {
		t.Fatalf("没有决策时不做资源预检: %s", out)
	}
}

func TestDocScriptLocalResourcePreflight(t *testing.T) {
	dir := chdirDocScriptTemp(t)
	if err := os.MkdirAll("assets", 0o755); err != nil {
		t.Fatal(err)
	}
	writeDocScriptPNG(t, filepath.Join("assets", "ok.png"))
	if err := os.WriteFile(filepath.Join("assets", "fake.png"), []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("assets", "empty.pdf"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("assets", "report.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("assets", "flow.mmd"), []byte("flowchart LR\nA-->B"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("assets", "widget.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 源 XML 目录回退：draft/ 下的 local.png 只在 XML 目录存在
	if err := os.MkdirAll("draft", 0o755); err != nil {
		t.Fatal(err)
	}
	writeDocScriptPNG(t, filepath.Join("draft", "local.png"))

	tests := []struct {
		name        string
		content     string
		contentPath string
		wantErr     string
	}{
		{name: "all ok", content: `<img path="@assets/ok.png" width="300"/><source path="@./assets/report.pdf" name="报告.pdf"/>` +
			`<whiteboard type="mermaid" path="@assets/flow.mmd"></whiteboard><whiteboard type="mermaid">@assets/flow.mmd</whiteboard>` +
			`<html5-block path="@assets/widget.html"/>`},
		{name: "absolute path", content: fmt.Sprintf(`<img path="@%s"/>`, filepath.Join(dir, "assets", "ok.png"))},
		{name: "fallback to xml dir", content: `<img path="@local.png"/>`, contentPath: filepath.Join("draft", "draft.xml")},
		{name: "no fallback for inline", content: `<img path="@local.png"/>`, wantErr: "本地图片 #1: 文件不存在"},
		{name: "missing", content: `<img path="@assets/ok.png"/><img path="@nope.png"/>`, wantErr: "本地图片 #2"},
		{name: "not image", content: `<img path="@assets/fake.png"/>`, wantErr: "不是支持的 BMP"},
		{name: "empty file", content: `<source path="@assets/empty.pdf"/>`, wantErr: "本地附件 #1: 文件不能为空"},
		{name: "missing at", content: `<img path="assets/ok.png"/>`, wantErr: "必须以 @ 开头"},
		{name: "reserved marker", content: `<img path="@lcli_img_0123"/>`, wantErr: "保留的占位标记"},
		{name: "path with href", content: `<img path="@assets/ok.png" href="https://example.com/a.png"/>`, wantErr: "不能与 href 同时使用"},
		{name: "bad source name", content: `<source path="@assets/report.pdf" name="a/b.pdf"/>`, wantErr: "name 必须是不含路径分隔符"},
		{name: "relative href", content: `<img href="/a.png"/>`, wantErr: "远程图片 #1 的 href 必须是"},
		{name: "href userinfo", content: `<img href="https://u:p@example.com/a.png"/>`, wantErr: "不带用户名密码"},
		{name: "sensitive dir", content: `<img path="@/etc/passwd"/>`, wantErr: "敏感目录"},
		{name: "parent escape", content: `<img path="@../x.png"/>`, wantErr: "越出当前目录"},
		{name: "whiteboard bad type", content: `<whiteboard type="dot" path="@assets/flow.mmd"/>`, wantErr: `type 必须是 "svg"`},
		{name: "whiteboard ext", content: `<whiteboard type="svg" path="@assets/flow.mmd"/>`, wantErr: "必须指向 .svg 文件"},
		{name: "whiteboard path and body", content: `<whiteboard type="mermaid" path="@assets/flow.mmd">graph TD</whiteboard>`, wantErr: "不能同时使用 path 和标签内内容"},
		{name: "whiteboard missing file", content: `<whiteboard type="mermaid" path="@nope.mmd"/>`, wantErr: "无法读取"},
		{name: "html5 inline body", content: `<html5-block><div>x</div></html5-block>`, wantErr: "不能写在 <html5-block> 标签体内"},
		{name: "html5 data-ref", content: `<html5-block data-ref="html5_1"/>`, wantErr: "reference_map"},
		{name: "html5 ext", content: `<html5-block path="@assets/flow.mmd"/>`, wantErr: "必须指向 .html 文件"},
		{name: "duplicate attr", content: `<img path="@assets/ok.png" PATH="@nope.png"/>`, wantErr: `属性 "path" 重复`},
		{name: "unterminated tag", content: `<img path="@assets/ok.png"`, wantErr: "资源标签未闭合"},
		{name: "comment ignored", content: `<!-- <img path="@nope.png"/> --><p>x</p>`},
		{name: "cdata ignored", content: `<pre><code><![CDATA[<img path="@nope.png"/>]]></code></pre>`},
		{name: "inline whiteboard skipped", content: `<whiteboard type="mermaid">graph TD; A-->B</whiteboard>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diagnostics := docScriptResourceDiagnostics(tt.content, tt.contentPath)
			if tt.wantErr == "" {
				if len(diagnostics) != 0 {
					t.Fatalf("不应有诊断: %+v", diagnostics)
				}
				return
			}
			if len(diagnostics) != 1 || diagnostics[0].Code != docScriptCodeResourceCheck ||
				!strings.Contains(diagnostics[0].Msg, tt.wantErr) {
				t.Fatalf("diagnostics = %+v，期望一条包含 %q 的 resource_preflight_failed", diagnostics, tt.wantErr)
			}
		})
	}
}

func TestDocScriptLocalResourceFailureSkipsRemoteProbe(t *testing.T) {
	chdirDocScriptTemp(t)
	orig := docScriptProbeRemoteImage
	t.Cleanup(func() { docScriptProbeRemoteImage = orig })
	docScriptProbeRemoteImage = func(string) error { t.Fatal("本地资源失败时不应继续探测远程图片"); return nil }
	diagnostics := docScriptResourceDiagnostics(`<img href="https://example.com/a.png"/><img path="@nope.png"/>`, "")
	if len(diagnostics) != 1 || diagnostics[0].Code != docScriptCodeResourceCheck || !strings.Contains(diagnostics[0].Msg, "#2") {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

func TestDocScriptInitDraftPersistsDecisionForAutomaticParse(t *testing.T) {
	dir := chdirDocScriptTemp(t)
	decision := `{"audience":"reader","reader_task":"understand","genre_contract":"none","adapter":null,"presentation_mode":"rich","word_count":{"min":18,"max":22},"visual_plan":{"reason":"visual","blocks":[{"type":"img","min_count":1,"purpose":"show"},{"type":"whiteboard","min_count":1,"purpose":"flow"},{"type":"html5-block","min_count":1,"purpose":"explore"}]}}`
	out, err := runDocScriptForTest(t, map[string]string{"command": "init-draft", "presentation-decision": decision})
	if err != nil {
		t.Fatalf("init-draft 失败: %v", err)
	}
	var raw map[string]json.RawMessage
	_ = json.Unmarshal([]byte(out), &raw)
	if len(raw) != 4 {
		t.Fatalf("init-draft 输出应只有 cwd/workspace/draft_path/tip: %s", out)
	}
	var result docScriptDraftResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	realDir, _ := filepath.EvalSymlinks(dir)
	realCWD, _ := filepath.EvalSymlinks(result.CWD)
	if realCWD != realDir || result.Tip != docScriptDraftTip {
		t.Fatalf("cwd/tip 异常: %+v", result)
	}
	if result.Workspace != filepath.Dir(result.DraftPath) || filepath.IsAbs(result.Workspace) || filepath.Dir(result.Workspace) != "." {
		t.Fatalf("workspace/draft_path 应为相对路径且 draft 在工作区内: %+v", result)
	}
	if !isDocScriptWorkspacePath(result.DraftPath) {
		t.Fatalf("draft_path 不符合 draft_<8hex>_folder/draft.xml: %q", result.DraftPath)
	}
	if _, err := os.Stat(result.DraftPath); !os.IsNotExist(err) {
		t.Fatalf("draft.xml 不应被创建: %v", err)
	}
	saved, err := os.ReadFile(filepath.Join(result.Workspace, docScriptDecisionFile))
	if err != nil || string(saved) != decision {
		t.Fatalf("保存的决策 = %q, %v", saved, err)
	}
	if err := os.WriteFile(result.DraftPath, []byte(`<title>标题</title><p>一个苹果是 an apple。</p><img/>`), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err = runDocScriptForTest(t, map[string]string{"command": "parse", "content": "@./" + result.DraftPath})
	if err != nil {
		t.Fatalf("parse 失败: %v", err)
	}
	parsed := decodeDocScriptParse(t, out)
	if parsed.Assessment.Status != docScriptAssessmentFailed || len(parsed.Diagnostics) != 3 {
		t.Fatalf("应自动加载保存的决策并给出 3 条诊断: %s", out)
	}
	// 显式决策优先于保存的决策
	out, err = runDocScriptForTest(t, map[string]string{"command": "parse", "content": "@./" + result.DraftPath, "presentation-decision": "{}"})
	if err != nil {
		t.Fatal(err)
	}
	if parsed := decodeDocScriptParse(t, out); parsed.Assessment.Status != docScriptAssessmentPassed {
		t.Fatalf("显式 --presentation-decision 应优先: %s", out)
	}
}

func TestDocScriptInitDraftCreatesUniqueWorkspaces(t *testing.T) {
	chdirDocScriptTemp(t)
	seen := map[string]bool{}
	for i := 0; i < 3; i++ {
		out, err := runDocScriptForTest(t, map[string]string{"command": "init-draft", "presentation-decision": "{}"})
		if err != nil {
			t.Fatal(err)
		}
		var result docScriptDraftResult
		_ = json.Unmarshal([]byte(out), &result)
		if seen[result.Workspace] {
			t.Fatalf("工作区重复: %s", result.Workspace)
		}
		seen[result.Workspace] = true
		entries, _ := os.ReadDir(result.Workspace)
		if len(entries) != 1 || entries[0].Name() != docScriptDecisionFile {
			t.Fatalf("工作区内只应有决策文件: %+v", entries)
		}
	}
}

func TestDocScriptInitDraftNormalizesShellQuotes(t *testing.T) {
	decision := `{"audience":"普通读者","reader_task":"复现实验","genre_contract":null,"adapter":null,"presentation_mode":"normal","visual_plan":{"reason":"复现实验","blocks":[]}}`
	tests := []struct {
		name, input, want string
	}{
		{name: "windows shim quotes", input: "'" + decision + "'", want: decision},
		{
			name:  "powershell keys and values",
			input: `{audience:a,reader_task:b,genre_contract:null,adapter:null,presentation_mode:normal,word_count:{min:10,max:null},visual_plan:{reason:c,blocks:[{type:img,min_count:1,purpose:d}]}}`,
			want:  `{"audience":"a","reader_task":"b","genre_contract":null,"adapter":null,"presentation_mode":"normal","word_count":{"min":10,"max":null},"visual_plan":{"reason":"c","blocks":[{"type":"img","min_count":1,"purpose":"d"}]}}`,
		},
		{
			name:  "powershell values only",
			input: `{"audience":a,"reader_task":b,"genre_contract":null,"adapter":null,"presentation_mode":normal,"visual_plan":{"reason":c,"blocks":[]}}`,
			want:  `{"audience":"a","reader_task":"b","genre_contract":null,"adapter":null,"presentation_mode":"normal","visual_plan":{"reason":"c","blocks":[]}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chdirDocScriptTemp(t)
			out, err := runDocScriptForTest(t, map[string]string{"command": "init-draft", "presentation-decision": tt.input})
			if err != nil {
				t.Fatalf("init-draft 失败: %v", err)
			}
			var result docScriptDraftResult
			_ = json.Unmarshal([]byte(out), &result)
			saved, err := os.ReadFile(filepath.Join(result.Workspace, docScriptDecisionFile))
			if err != nil || string(saved) != tt.want {
				t.Fatalf("保存的决策 = %q, 期望 %q (%v)", saved, tt.want, err)
			}
		})
	}
}

func TestDocScriptDecisionRecoveryKeepsOriginalValidation(t *testing.T) {
	dir := chdirDocScriptTemp(t)
	// 引号恢复后仍按原 schema 校验：未知字段、非正 min_count 都报错，且不创建任何文件
	for _, tt := range []struct{ input, want string }{
		{input: `{audience:a,reader_task:b,presentation_mode:decorative,visual_plan:{reason:c,blocks:[{type:img,min_count:0}]}}`, want: "visual_plan.blocks[0].min_count 必须是正整数"},
		{input: `'{"visual_plan":{"blocks":[]},"unexpected":true}'`, want: `unknown field "unexpected"`},
	} {
		_, err := runDocScriptForTest(t, map[string]string{"command": "init-draft", "presentation-decision": tt.input})
		requireDocScriptUsage(t, err, tt.want)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("失败的 init-draft 不应创建文件: %+v", entries)
	}
}

func TestDocScriptMangledInlineDecisionSuggestsFileInput(t *testing.T) {
	chdirDocScriptTemp(t)
	_, err := runDocScriptForTest(t, map[string]string{"command": "init-draft", "presentation-decision": `{audience:reader,reviewer,reader_task:understand}`})
	requireDocScriptUsage(t, err, docScriptDecisionShellHint)
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("应保留 *json.SyntaxError 原因: %T %v", err, err)
	}
}

func TestDocScriptDecisionFileIsStrictAndAcceptsBOM(t *testing.T) {
	chdirDocScriptTemp(t)
	if err := os.WriteFile("bad.json", []byte(`{audience:reader,reader_task:understand}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := runDocScriptForTest(t, map[string]string{"command": "init-draft", "presentation-decision": "@./bad.json"})
	requireDocScriptUsage(t, err, "Presentation Decision JSON")
	if strings.Contains(err.Error(), "提示") {
		t.Fatalf("@文件输入不做 shell 引号恢复，也不给出引号提示: %v", err)
	}
	decision := `{"audience":"普通读者","visual_plan":{"blocks":[]}}`
	if err := os.WriteFile("good.json", []byte("\uFEFF"+decision), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runDocScriptForTest(t, map[string]string{"command": "init-draft", "presentation-decision": "@./good.json"})
	if err != nil {
		t.Fatalf("带 BOM 的决策文件应可用: %v", err)
	}
	var result docScriptDraftResult
	_ = json.Unmarshal([]byte(out), &result)
	if saved, _ := os.ReadFile(filepath.Join(result.Workspace, docScriptDecisionFile)); string(saved) != decision {
		t.Fatalf("保存的决策应去掉 BOM: %q", saved)
	}
}

func TestDocScriptDecisionFromStdin(t *testing.T) {
	chdirDocScriptTemp(t)
	orig := docScriptStdin
	t.Cleanup(func() { docScriptStdin = orig })
	docScriptStdin = strings.NewReader(`{"word_count":{"min":1,"max":null}}`)
	out, err := runDocScriptForTest(t, map[string]string{"command": "parse", "content": "<p>两个字</p>", "presentation-decision": "-"})
	if err != nil {
		t.Fatal(err)
	}
	if result := decodeDocScriptParse(t, out); result.Assessment.Status != docScriptAssessmentPassed {
		t.Fatalf("stdin 决策应生效: %s", out)
	}
	_, err = runDocScriptForTest(t, map[string]string{"command": "parse", "content": "-", "presentation-decision": "-"})
	requireDocScriptUsage(t, err, "最多只能有一个读取标准输入")
}

func TestDocScriptInitDraftValidation(t *testing.T) {
	dir := chdirDocScriptTemp(t)
	for _, tt := range []struct {
		flags map[string]string
		want  string
	}{
		{flags: map[string]string{"command": "init-draft"}, want: "需要 --presentation-decision"},
		{flags: map[string]string{"command": "init-draft", "presentation-decision": "{}", "content": "<p>x</p>"}, want: "不支持 --content"},
		{flags: map[string]string{"command": "init-draft", "presentation-decision": "{}", "doc": "doxcnxxx"}, want: "不支持 --doc"},
		{flags: map[string]string{"command": "lint"}, want: "不支持的 --command"},
		{flags: map[string]string{"command": "parse"}, want: "其中之一"},
		{flags: map[string]string{"command": "parse", "content": "<p>x</p>", "doc": "doxcnxxx"}, want: "互斥"},
		{flags: map[string]string{"command": "parse", "doc": "https://xxx.feishu.cn/sheets/shtcnxxx"}, want: "--doc"},
		{flags: map[string]string{"command": "parse", "content": "@"}, want: "@ 后面的文件路径不能为空"},
		{flags: map[string]string{"command": "parse", "content": "@./nope.xml"}, want: "不存在"},
		{flags: map[string]string{"command": "parse", "content": "@/etc/passwd"}, want: "敏感目录"},
		{flags: map[string]string{"command": "parse", "content": "<p>x</p>", "as": "robot"}, want: "--as"},
		{flags: map[string]string{"command": "parse", "content": "<p>x</p>", "format": "yaml"}, want: "--format"},
	} {
		_, err := runDocScriptForTest(t, tt.flags)
		requireDocScriptUsage(t, err, tt.want)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("校验失败不应创建文件: %+v", entries)
	}
}

func TestDocScriptRejectsInvalidPresentationDecision(t *testing.T) {
	prefix := `{"audience":"reader","reader_task":"understand","genre_contract":"none","adapter":null,"presentation_mode":"normal",`
	for _, tt := range []struct{ name, decision, want string }{
		{"invalid json", `{`, "JSON"},
		{"unknown field", prefix + `"visual_plan":{"reason":"x","blocks":[],"image_enabled":true}}`, `unknown field "image_enabled"`},
		{"legacy fields", `{"target":"reader","visual_plan":{"blocks":[]}}`, `unknown field "target"`},
		{"removed hard_rules", prefix + `"hard_rules":[],"visual_plan":{"blocks":[]}}`, `unknown field "hard_rules"`},
		{"integer word count", `{"word_count":20}`, "word_count"},
		{"null word count", `{"word_count":null}`, "应省略 word_count"},
		{"missing word count bound", `{"word_count":{"min":10}}`, "缺少 word_count.max"},
		{"empty word count range", `{"word_count":{"min":null,"max":null}}`, "至少要设置 min 或 max"},
		{"non-positive word count", `{"word_count":{"min":0,"max":10}}`, "word_count.min 必须是正整数"},
		{"non-positive max", `{"word_count":{"min":null,"max":-1}}`, "word_count.max 必须是正整数"},
		{"reversed range", `{"word_count":{"min":20,"max":10}}`, "min 不能大于"},
		{"unknown block type", `{"visual_plan":{"blocks":[{"type":"future-widget","min_count":1}]}}`, "不是支持的展示块类型"},
		{"text block not presentation", `{"visual_plan":{"blocks":[{"type":"p","min_count":2}]}}`, "不是支持的展示块类型"},
		{"empty type", `{"visual_plan":{"blocks":[{"type":""}]}}`, "不是支持的展示块类型"},
		{"non-positive min_count", `{"visual_plan":{"blocks":[{"type":"img","min_count":0}]}}`, "min_count 必须是正整数"},
		{"negative min_count without type", `{"visual_plan":{"blocks":[{"min_count":-1}]}}`, "min_count 必须是正整数"},
		{"float min_count", `{"visual_plan":{"blocks":[{"min_count":1.5}]}}`, "JSON"},
		{"string min_count", `{"visual_plan":{"blocks":[{"min_count":"1"}]}}`, "JSON"},
		{"numeric type", `{"visual_plan":{"blocks":[{"type":42}]}}`, "不是支持的展示块类型"}, // 内联输入经引号恢复后仍按 schema 拒绝
		{"visual_plan string", `{"visual_plan":"bad"}`, "JSON"},
		{"blocks object", `{"visual_plan":{"blocks":{}}}`, "JSON"},
		{"duplicate type", `{"visual_plan":{"blocks":[{"type":"img","min_count":1},{"type":"img","min_count":2}]}}`, "重复声明了类型"},
		{"multiple values", `{} {}`, "多个 JSON 值"},
		{"null", `null`, "必须是 JSON 对象"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runDocScriptForTest(t, map[string]string{"command": "parse", "content": "<p>text</p>", "presentation-decision": tt.decision})
			requireDocScriptUsage(t, err, tt.want)
		})
	}
}

func TestDocScriptOptionalBlockConstraints(t *testing.T) {
	for _, tt := range []struct{ name, decision, wantCode string }{
		{"no constraints", `{}`, ""},
		{"no visual plan", `{"audience":"reader"}`, ""},
		{"null visual plan", `{"visual_plan":null}`, ""},
		{"no blocks", `{"visual_plan":{}}`, ""},
		{"null blocks", `{"visual_plan":{"blocks":null}}`, ""},
		{"empty entry", `{"visual_plan":{"blocks":[{}]}}`, ""},
		{"no type", `{"visual_plan":{"blocks":[{"min_count":2}]}}`, ""},
		{"no minimum", `{"visual_plan":{"blocks":[{"type":"whiteboard"}]}}`, ""},
		{"null fields", `{"visual_plan":{"blocks":[{"type":null,"min_count":null}]}}`, ""},
		{"null descriptions", `{"audience":null,"reader_task":null,"genre_contract":null,"adapter":null,"presentation_mode":null,"visual_plan":{"reason":null,"blocks":[]}}`, ""},
		{"word count satisfied", `{"word_count":{"min":1,"max":5}}`, ""},
		{"word count enforced", `{"word_count":{"min":10,"max":null}}`, docScriptCodeWordCountRange},
		{"complete constraint", `{"visual_plan":{"blocks":[{"type":"table","min_count":1}]}}`, docScriptCodeRequiredBlock},
		{"partial entries", `{"visual_plan":{"blocks":[{},{"type":"table"},{"min_count":1},{"type":"table","min_count":1}]}}`, docScriptCodeRequiredBlock},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out, err := runDocScriptForTest(t, map[string]string{"command": "parse", "content": "<p>plain text</p>", "presentation-decision": tt.decision})
			if err != nil {
				t.Fatal(err)
			}
			result := decodeDocScriptParse(t, out)
			if tt.wantCode == "" {
				if result.Assessment.Status != docScriptAssessmentPassed || len(result.Diagnostics) != 0 {
					t.Fatalf("应通过: %s", out)
				}
				return
			}
			if result.Assessment.Status != docScriptAssessmentFailed || len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != tt.wantCode {
				t.Fatalf("应只有一条 %s: %s", tt.wantCode, out)
			}
		})
	}
}

func TestDocScriptPresentationDiagnosticsRules(t *testing.T) {
	minimum, maximum := 9, 11
	decision := docScriptDecision{WordCount: &docScriptDecisionWords{Min: &minimum, Max: &maximum}}
	for count, wantWarn := range map[int]bool{8: true, 9: false, 10: false, 11: false, 12: true} {
		if got := len(docScriptPresentationDiagnostics(docScriptPublicProfile{WordCount: count}, decision)) > 0; got != wantWarn {
			t.Fatalf("word_count=%d 诊断=%v，期望 %v（闭区间）", count, got, wantWarn)
		}
	}
	for _, tt := range []struct {
		raw   string
		count int
		want  string
	}{
		{`{"word_count":{"min":10,"max":null}}`, 9, "把字数增加到至少 10。"},
		{`{"word_count":{"min":null,"max":20}}`, 21, "把字数压缩到不超过 20。"},
	} {
		d, err := parseDocScriptDecision(tt.raw)
		if err != nil {
			t.Fatal(err)
		}
		diags := docScriptPresentationDiagnostics(docScriptPublicProfile{WordCount: tt.count}, d)
		if len(diags) != 1 || diags[0].Suggested != tt.want {
			t.Fatalf("单边字数建议 = %+v，期望 %q", diags, tt.want)
		}
	}
	// list 按 ul + ol 合计
	d, err := parseDocScriptDecision(`{"visual_plan":{"blocks":[{"type":" list ","min_count":2}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	profile := docScriptPublicProfile{Blocks: []docxparse.BlockShare{{Type: "ul", Count: 1}, {Type: "ol", Count: 1}}}
	if diags := docScriptPresentationDiagnostics(profile, d); len(diags) != 0 {
		t.Fatalf("ul+ol 应满足 2 个 list: %+v", diags)
	}
	three := 3
	d.VisualPlan.Blocks[0].MinCount = &three
	diags := docScriptPresentationDiagnostics(profile, d)
	if len(diags) != 1 || diags[0].Expected.Type != "list" || diags[0].Expected.MinCount != 3 || *diags[0].Actual != 2 {
		t.Fatalf("list 诊断 = %+v", diags)
	}
	// 通用展示块（table）按画像目录计数
	d, _ = parseDocScriptDecision(`{"visual_plan":{"blocks":[{"type":"table","min_count":2,"purpose":"对比"}]}}`)
	diags = docScriptPresentationDiagnostics(docScriptPublicProfile{Blocks: []docxparse.BlockShare{{Type: "table", Count: 1}}}, d)
	if len(diags) != 1 || diags[0].Expected.Type != "table" || *diags[0].Actual != 1 {
		t.Fatalf("table 诊断 = %+v", diags)
	}
}

func TestDocScriptOmitsDiagnosticsWhenDecisionPasses(t *testing.T) {
	chdirDocScriptTemp(t)
	writeDocScriptPNG(t, "result.png")
	if err := os.WriteFile("flow.mmd", []byte("flowchart LR\nA --> B"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("widget.html", []byte("<html><body>status</body></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	decision := `{"word_count":{"min":9,"max":11},"visual_plan":{"blocks":[{"type":"img","min_count":1},{"type":"whiteboard","min_count":1},{"type":"html5-block","min_count":1}]}}`
	out, err := runDocScriptForTest(t, map[string]string{
		"command":               "parse",
		"content":               `<title>标题</title><p>一个苹果是 an apple。</p><img path="@result.png"/><whiteboard type="mermaid" path="@flow.mmd"></whiteboard><html5-block path="@widget.html"></html5-block>`,
		"presentation-decision": decision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "diagnostics") || decodeDocScriptParse(t, out).Assessment.Status != docScriptAssessmentPassed {
		t.Fatalf("通过时不应输出 diagnostics: %s", out)
	}
}

func TestDocScriptRejectsMarkdownAndUnsafeXML(t *testing.T) {
	for _, content := range []string{"# 标题\n\n- item", `<!DOCTYPE document><p>text</p>`} {
		_, err := runDocScriptForTest(t, map[string]string{"command": "parse", "content": content})
		requireDocScriptUsage(t, err, "无法把 --content 解析为 DocxXML")
	}
}

func TestDocScriptProfilesCompatibleMalformedXML(t *testing.T) {
	out, err := runDocScriptForTest(t, map[string]string{"command": "parse", "content": `<title>T</title><ul><li>one<li>two</ul><p>tail</p`})
	if err != nil {
		t.Fatal(err)
	}
	result := decodeDocScriptParse(t, out)
	p := result.Profile
	if result.Assessment.Status != docScriptAssessmentPassed || p.BlockCount != 5 ||
		docScriptTestBlockCount(p.Blocks, "li") != 2 || docScriptTestBlockCount(p.Blocks, "p") != 1 {
		t.Fatalf("容错解析画像 = %s", out)
	}
}

func TestDocScriptContentFromFileAndAtEscape(t *testing.T) {
	chdirDocScriptTemp(t)
	if err := os.WriteFile("doc.xml", []byte("\uFEFF<title>T</title><p>x</p>"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runDocScriptForTest(t, map[string]string{"command": "parse", "content": "@./doc.xml"})
	if err != nil {
		t.Fatal(err)
	}
	if p := decodeDocScriptParse(t, out).Profile; p.BlockCount != 2 {
		t.Fatalf("@文件应被读取: %s", out)
	}
	// @@ 转义：内容按字面 "@<p>..." 处理，不是 XML
	_, err = runDocScriptForTest(t, map[string]string{"command": "parse", "content": "@@<p>x</p>"})
	requireDocScriptUsage(t, err, "必须以 '<' 开头")
}

func TestDocScriptDryRun(t *testing.T) {
	dir := chdirDocScriptTemp(t)
	out, err := runDocScriptForTest(t, map[string]string{"command": "parse", "content": "<p>text</p>", "dry-run": "true"})
	if err != nil {
		t.Fatal(err)
	}
	var plan map[string]any
	_ = json.Unmarshal([]byte(out), &plan)
	if plan["dry_run"] != true || plan["command"] != "parse" || plan["network"] != false || plan["api"] != nil {
		t.Fatalf("本地 parse dry-run = %s", out)
	}

	out, err = runDocScriptForTest(t, map[string]string{
		"command": "parse", "content": `<img href="https://93.184.216.34/image.png"/>`,
		"presentation-decision": "{}", "dry-run": "true",
	})
	if err != nil {
		t.Fatal(err)
	}
	plan = nil
	_ = json.Unmarshal([]byte(out), &plan)
	if plan["network"] != true || plan["presentation_decision"] != true {
		t.Fatalf("有远程图片预检时 dry-run 应声明 network=true: %s", out)
	}

	out, err = runDocScriptForTest(t, map[string]string{"command": "parse", "doc": "https://xxx.feishu.cn/docx/doxcnDryRun", "dry-run": "true"})
	if err != nil {
		t.Fatal(err)
	}
	var online struct {
		API []struct {
			Method string         `json:"method"`
			URL    string         `json:"url"`
			Body   map[string]any `json:"body"`
		} `json:"api"`
		DocumentID string `json:"document_id"`
		Network    bool   `json:"network"`
	}
	_ = json.Unmarshal([]byte(out), &online)
	if len(online.API) != 1 || online.API[0].Method != "POST" || online.API[0].URL != "/open-apis/docs_ai/v1/documents/doxcnDryRun/fetch" ||
		online.API[0].Body["format"] != "xml" || online.DocumentID != "doxcnDryRun" || !online.Network {
		t.Fatalf("在线 dry-run = %s", out)
	}

	out, err = runDocScriptForTest(t, map[string]string{"command": "init-draft", "presentation-decision": "{}", "dry-run": "true"})
	if err != nil {
		t.Fatal(err)
	}
	plan = nil
	_ = json.Unmarshal([]byte(out), &plan)
	if plan["creates_workspace"] != true || plan["creates_draft_file"] != false || plan["directory_pattern"] != docScriptDraftDirPattern || plan["network"] != false {
		t.Fatalf("init-draft dry-run = %s", out)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("dry-run 不应写文件: %+v", entries)
	}

	// dry-run 也会校验保存的决策
	if err := os.MkdirAll("ws", 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join("ws", "draft.xml"), []byte("<p>draft</p>"), 0o600)
	_ = os.WriteFile(filepath.Join("ws", docScriptDecisionFile), []byte(`{"invalid":true}`), 0o600)
	// dry-run 与正式执行都会校验已保存的决策；损坏时与官方一致为校验错误（退出码 2），并保留原因链
	for _, dryRun := range []string{"true", "false"} {
		_, err = runDocScriptForTest(t, map[string]string{"command": "parse", "content": "@ws/draft.xml", "dry-run": dryRun})
		requireDocScriptUsage(t, err, "已保存的 Presentation Decision")
		if !strings.Contains(err.Error(), `unknown field "invalid"`) || !strings.Contains(err.Error(), "重新执行 init-draft") {
			t.Fatalf("应说明损坏原因与修复方式: %v", err)
		}
	}
	_ = os.WriteFile(filepath.Join("ws", docScriptDecisionFile), []byte(`{"word_count":`), 0o600)
	_, err = runDocScriptForTest(t, map[string]string{"command": "parse", "content": "@ws/draft.xml"})
	requireDocScriptUsage(t, err, "已保存的 Presentation Decision")
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) && !strings.Contains(err.Error(), "unexpected EOF") {
		t.Fatalf("截断的决策文件应保留 JSON 解析原因: %T %v", err, err)
	}
}

func TestDocScriptJQFilter(t *testing.T) {
	out, err := runDocScriptForTest(t, map[string]string{"command": "parse", "content": "<p>a b c</p>", "jq": ".profile.word_count"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "3" {
		t.Fatalf("--jq 输出 = %q", out)
	}
}

func TestDocScriptParsesOnlineDocument(t *testing.T) {
	var fetchBodies []map[string]any
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		paths = append(paths, r.Method+" "+r.URL.Path)
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.URL.Path == "/open-apis/wiki/v2/spaces/node_by_token":
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"node":{"node_token":"wikcnScript","obj_token":"doxcnFromWiki","obj_type":"docx","space_id":"1"}}}`)
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/fetch"):
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			fetchBodies = append(fetchBodies, body)
			content := `<title>在线文档</title><p>Hello world</p>`
			if strings.Contains(r.URL.Path, "doxcnFromWiki") {
				content = `<p>从 Wiki 读取</p>`
			}
			raw, _ := json.Marshal(content)
			fmt.Fprintf(w, `{"code":0,"msg":"ok","data":{"document":{"document_id":"x","content":%s}}}`, raw)
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)

	out, err := runDocScriptForTest(t, map[string]string{"command": "parse", "doc": "doxcnScriptToken", "as": "bot"})
	if err != nil {
		t.Fatalf("在线解析失败: %v", err)
	}
	if p := decodeDocScriptParse(t, out).Profile; p.BlockCount != 2 || docScriptTestBlockCount(p.Blocks, "title") != 1 {
		t.Fatalf("在线画像 = %s", out)
	}
	if len(fetchBodies) != 1 || fetchBodies[0]["format"] != "xml" || fetchBodies[0]["extra_param"] != docsFetchExtraParam {
		t.Fatalf("fetch 请求体 = %+v", fetchBodies)
	}
	opt, _ := fetchBodies[0]["export_option"].(map[string]any)
	if opt["export_block_id"] != false || opt["export_style_attrs"] != false || opt["export_cite_extra_data"] != false {
		t.Fatalf("export_option 应全部为 false: %+v", opt)
	}

	out, err = runDocScriptForTest(t, map[string]string{"command": "parse", "doc": "https://xxx.feishu.cn/wiki/wikcnScript", "as": "bot"})
	if err != nil {
		t.Fatalf("wiki URL 解析失败: %v (paths=%v)", err, paths)
	}
	if p := decodeDocScriptParse(t, out).Profile; p.BlockCount != 1 || docScriptTestBlockCount(p.Blocks, "p") != 1 {
		t.Fatalf("wiki 画像 = %s", out)
	}
	if !strings.Contains(strings.Join(paths, ","), "POST /open-apis/docs_ai/v1/documents/doxcnFromWiki/fetch") {
		t.Fatalf("wiki 应先解析为底层 docx: %v", paths)
	}
}

func TestDocScriptOnlineUnparsableContentIsUsageError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
			return
		}
		fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"document":{"document_id":"x","content":"<!DOCTYPE x><p>bad</p>"}}}`)
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)
	_, err := runDocScriptForTest(t, map[string]string{"command": "parse", "doc": "doxcnBadXML", "as": "bot"})
	requireDocScriptUsage(t, err, "无法把在线文档内容解析为 DocxXML")
	if !strings.Contains(err.Error(), "DOCTYPE") {
		t.Fatalf("应保留解析失败原因: %v", err)
	}
}

func TestDocScriptOnlineMissingContentIsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
			return
		}
		fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"document":{"document_id":"x"}}}`)
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)
	_, err := runDocScriptForTest(t, map[string]string{"command": "parse", "doc": "doxcnNoContent", "as": "bot"})
	if err == nil || !strings.Contains(err.Error(), "document.content") || clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("缺少 content 应报一般错误: %v", err)
	}
}

func TestDocScriptWorkspaceCleanupOnlyTouchesExpectedPaths(t *testing.T) {
	chdirDocScriptTemp(t)
	for _, path := range []string{"draft.xml", "other/draft.xml", "draft_zzzzzzzz_folder/draft.xml", "draft_1234_folder/draft.xml",
		"x/draft_12345678_folder/draft.xml", "draft_12345678_folder/other.xml"} {
		if isDocScriptWorkspacePath(path) {
			t.Fatalf("%q 不应被视为草稿工作区", path)
		}
		if err := removeDocScriptWorkspace(path); err == nil {
			t.Fatalf("清理非预期路径 %q 应被拒绝", path)
		}
	}
	// 工作区里有用户文件时只删决策文件、目录保留（非递归删除）
	ws := "draft_0a1b2c3d_folder"
	if err := os.Mkdir(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(ws, docScriptDecisionFile), []byte("{}"), 0o600)
	_ = os.WriteFile(filepath.Join(ws, "keep.png"), []byte("x"), 0o600)
	if err := removeDocScriptWorkspace(filepath.Join(ws, docScriptDraftXMLFileName)); err == nil {
		t.Fatal("目录非空时删除目录应失败")
	}
	if _, err := os.Stat(filepath.Join(ws, "keep.png")); err != nil {
		t.Fatalf("用户文件不应被删除: %v", err)
	}
	_ = os.Remove(filepath.Join(ws, "keep.png"))
	_ = os.WriteFile(filepath.Join(ws, docScriptDecisionFile), []byte("{}"), 0o600)
	if err := removeDocScriptWorkspace(filepath.Join(ws, docScriptDraftXMLFileName)); err != nil {
		t.Fatalf("清理本次工作区失败: %v", err)
	}
	if _, err := os.Stat(ws); !os.IsNotExist(err) {
		t.Fatalf("工作区应被删除: %v", err)
	}
}

func TestDocScriptHelpExamplesQuoteAtFiles(t *testing.T) {
	if strings.Contains(docScriptCmd.Long, "--content @") || strings.Contains(docScriptCmd.Long, "--presentation-decision @") {
		t.Fatal("帮助示例中的 @文件参数应加引号（PowerShell 会展开裸 @）")
	}
	for _, want := range []string{`--command init-draft --presentation-decision`, `--content "@./draft_`} {
		if !strings.Contains(docScriptCmd.Long, want) {
			t.Fatalf("帮助示例缺少 %q", want)
		}
	}
}

func TestDocScriptParseDecisionRejectsInvalidProvidedFieldsStrictly(t *testing.T) {
	// 直接解析（@文件 / stdin 路径）不做引号恢复：类型不符一律是 JSON 解码错误
	for _, raw := range []string{
		`{"visual_plan":"bad"}`, `{"visual_plan":{"blocks":{}}}`, `{"visual_plan":{"blocks":[{"type":42}]}}`,
		`{"visual_plan":{"blocks":[{"min_count":1.5}]}}`, `{"visual_plan":{"blocks":[{"min_count":"1"}]}}`,
	} {
		_, err := parseDocScriptDecision(raw)
		requireDocScriptUsage(t, err, "Presentation Decision JSON")
	}
}

func TestDocScriptInitDraftRejectsSensitiveWorkingDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sensitive := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sensitive, 0o700); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	if err := os.Chdir(sensitive); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	_, err := runDocScriptForTest(t, map[string]string{"command": "init-draft", "presentation-decision": "{}"})
	requireDocScriptUsage(t, err, "敏感目录")
	if entries, _ := os.ReadDir(sensitive); len(entries) != 0 {
		t.Fatalf("敏感目录中不应创建工作区: %+v", entries)
	}
}
