package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/riba2534/feishu-cli/v2/internal/clierr"
)

// fakeDocsAI 是一个最小的 docs_ai 服务端：内存中保存文档的 Markdown 序列化，
// 按真实服务端契约实现 fetch 与 str_replace（唯一命中才替换；未命中 1013、多处 1014），
// 其余命令只记录请求体。
type fakeDocsAI struct {
	mu       sync.Mutex
	doc      string
	revision int
	bodies   []map[string]any
	fetches  int
	children string // GET /children 的 items JSON（块级定位用）
}

func (f *fakeDocsAI) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Tt-Logid", "log-fake-123")
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/fetch"):
			f.fetches++
			var fb map[string]any
			_ = json.NewDecoder(r.Body).Decode(&fb)
			content := f.doc
			if fb["format"] == "xml" {
				content = fakeXMLView(f.doc)
			}
			resp := map[string]any{"code": 0, "msg": "", "data": map[string]any{
				"document": map[string]any{"content": content, "revision_id": f.revision, "document_id": "doc"},
			}}
			_ = json.NewEncoder(w).Encode(resp)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/children"):
			items := f.children
			if items == "" {
				items = "[]"
			}
			fmt.Fprintf(w, `{"code":0,"msg":"ok","data":{"items":%s,"has_more":false}}`, items)
		case r.Method == http.MethodPut:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.bodies = append(f.bodies, body)
			if body["command"] == "str_replace" {
				pattern, _ := body["pattern"].(string)
				content, _ := body["content"].(string)
				view := f.doc
				if body["format"] == "xml" {
					view = fakeXMLView(f.doc)
				}
				n := countOverlapping(view, pattern)
				switch {
				case n == 0:
					fmt.Fprintf(w, `{"code":0,"msg":"","data":{"document":{"revision_id":%d},"result":"failed","warnings":["degrade_code=1013,msg=str_replace pattern was not found in the document."]}}`, f.revision)
					return
				case n > 1:
					fmt.Fprintf(w, `{"code":0,"msg":"","data":{"document":{"revision_id":%d},"result":"failed","warnings":["degrade_code=1014,msg=str_replace pattern matched multiple locations."]}}`, f.revision)
					return
				}
				view = strings.Replace(view, pattern, content, 1)
				if body["format"] == "xml" {
					view = fakeMarkdownView(view)
				}
				f.doc = view
			}
			f.revision++
			fmt.Fprintf(w, `{"code":0,"msg":"","data":{"document":{"revision_id":%d},"result":"success","warnings":[]}}`, f.revision)
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}
}

// fakeXMLView 粗略模拟 XML 序列化：首行 "# 标题" 变为 <title>标题</title>，正文原样。
func fakeXMLView(doc string) string {
	if strings.HasPrefix(doc, "# ") {
		nl := strings.IndexByte(doc, '\n')
		if nl < 0 {
			return "<title>" + doc[2:] + "</title>"
		}
		return "<title>" + doc[2:nl] + "</title>" + doc[nl+1:]
	}
	return doc
}

func fakeMarkdownView(xml string) string {
	if strings.HasPrefix(xml, "<title>") {
		end := strings.Index(xml, "</title>")
		return "# " + xml[len("<title>"):end] + "\n" + xml[end+len("</title>"):]
	}
	return xml
}

func newFakeDocsAI(t *testing.T, doc string) *fakeDocsAI {
	t.Helper()
	f := &fakeDocsAI{doc: doc, revision: 7}
	server := httptest.NewServer(f.handler(t))
	t.Cleanup(server.Close)
	initDocUpdateTestConfig(t, server.URL)
	return f
}

func runParams(p *contentUpdateParams) (string, string, error) {
	var out, errOut bytes.Buffer
	p.stdout, p.stderr = &out, &errOut
	if p.docFormat == "" {
		p.docFormat = "markdown"
	}
	if p.documentID == "" {
		p.documentID = "doc"
	}
	if p.revisionID == 0 {
		p.revisionID = -1
	}
	err := executeContentUpdate(p)
	return out.String(), errOut.String(), err
}

// TestReplaceAllPlainTextKeepsRestOfParagraph 是 P0 回归：段落里还有其他文字时，
// replace_all 只替换文字本身，段落其余文字必须原样保留（此前整段被替换成"新文本"）。
func TestReplaceAllPlainTextKeepsRestOfParagraph(t *testing.T) {
	f := newFakeDocsAI(t, "# 标题\n\n段落开头 旧文本 位于句子中间，后面还有尾巴。\n\n另一段：前缀旧文本后缀。\n")
	// 同时提供块树：若实现退回"按块定位 + 整块 block_replace"，下面的命令断言会失败
	f.children = `[{"block_id":"p1","block_type":2,"text":{"elements":[{"text_run":{"content":"段落开头 旧文本 位于句子中间，后面还有尾巴。"}}]}},
		{"block_id":"p2","block_type":2,"text":{"elements":[{"text_run":{"content":"另一段：前缀旧文本后缀。"}}]}}]`
	stdout, _, err := runParams(&contentUpdateParams{mode: "replace_all", selEllipsis: "旧文本", content: "新文本"})
	if err != nil {
		t.Fatalf("replace_all 失败: %v", err)
	}
	want := "# 标题\n\n段落开头 新文本 位于句子中间，后面还有尾巴。\n\n另一段：前缀新文本后缀。\n"
	if f.doc != want {
		t.Fatalf("替换后文档 =\n%q\n期望\n%q", f.doc, want)
	}
	for _, b := range f.bodies {
		if b["command"] != "str_replace" {
			t.Fatalf("文本级替换不应发出 %v（整块替换会丢失段落其余文字）", b["command"])
		}
		if b["format"] != "xml" {
			t.Fatalf("纯文字替换应自动走 XML 序列化以保留下划线/颜色等样式，实际 format=%v", b["format"])
		}
	}
	if len(f.bodies) != 2 {
		t.Fatalf("PUT 次数 = %d，期望 2", len(f.bodies))
	}
	// 首次写入以读取时的版本为基准，第二次承接服务端返回的新版本
	if f.bodies[0]["revision_id"] != float64(7) || f.bodies[1]["revision_id"] != float64(8) {
		t.Fatalf("revision 串联异常: %v, %v", f.bodies[0]["revision_id"], f.bodies[1]["revision_id"])
	}
	if !strings.Contains(stdout, "共替换 2 处") {
		t.Fatalf("输出缺少替换计数: %q", stdout)
	}
}

// TestReplaceRangePlainTextSingleHitKeepsParagraph replace_range + 纯文本选择器：文本级、唯一命中。
func TestReplaceRangePlainTextSingleHitKeepsParagraph(t *testing.T) {
	f := newFakeDocsAI(t, "# 标题\n\n这段有 **粗体** 和 旧文本，还有其它文字。\n")
	if _, _, err := runParams(&contentUpdateParams{mode: "replace_range", selEllipsis: "旧文本", content: "**新文本**"}); err != nil {
		t.Fatalf("replace_range 失败: %v", err)
	}
	if want := "# 标题\n\n这段有 **粗体** 和 **新文本**，还有其它文字。\n"; f.doc != want {
		t.Fatalf("文档 = %q，期望 %q", f.doc, want)
	}
	if len(f.bodies) != 1 || f.bodies[0]["pattern"] != "旧文本" || f.bodies[0]["command"] != "str_replace" {
		t.Fatalf("请求体异常: %#v", f.bodies)
	}
	if f.bodies[0]["format"] != "markdown" {
		t.Fatalf("替换内容含 Markdown 语法时应按 markdown 序列化发送，实际 %v", f.bodies[0]["format"])
	}
}

func TestDeleteRangePlainTextDeletesOnlyText(t *testing.T) {
	f := newFakeDocsAI(t, "# 标题\n\n保留前缀（草稿）保留后缀\n")
	stdout, _, err := runParams(&contentUpdateParams{mode: "delete_range", selEllipsis: "（草稿）"})
	if err != nil {
		t.Fatalf("delete_range 失败: %v", err)
	}
	if want := "# 标题\n\n保留前缀保留后缀\n"; f.doc != want {
		t.Fatalf("文档 = %q，期望 %q", f.doc, want)
	}
	if f.bodies[0]["content"] != "" {
		t.Fatalf("删除文本应发送空 content，实际 %#v", f.bodies[0]["content"])
	}
	if !strings.Contains(stdout, "已删除文本") {
		t.Fatalf("输出异常: %q", stdout)
	}
}

func TestTextReplaceSingleModeRejectsMultipleHits(t *testing.T) {
	f := newFakeDocsAI(t, "# 标题\n\n甲 旧文本 乙\n\n丙 旧文本 丁\n")
	_, _, err := runParams(&contentUpdateParams{mode: "replace_range", selEllipsis: "旧文本", content: "新"})
	if err == nil || !strings.Contains(err.Error(), "出现 2 处") {
		t.Fatalf("多处命中应报错，得到: %v", err)
	}
	if !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("多处命中应为用法错误（exit 2），得到: %v", err)
	}
	if len(f.bodies) != 0 {
		t.Fatalf("报错前不应发出任何写请求，实际 %d 次", len(f.bodies))
	}
}

func TestTextReplaceNotFound(t *testing.T) {
	f := newFakeDocsAI(t, "# 标题\n\n只有 str\\_replace 转义形式\n")
	_, _, err := runParams(&contentUpdateParams{mode: "replace_all", selEllipsis: "不存在的词", content: "x"})
	if err == nil || !strings.Contains(err.Error(), "未找到文本") {
		t.Fatalf("未命中应报错，得到: %v", err)
	}
	if len(f.bodies) != 0 {
		t.Fatal("未命中不应发出写请求")
	}
}

func TestTextReplaceTitleOnlyHitIsRejected(t *testing.T) {
	f := newFakeDocsAI(t, "# 年度计划\n\n正文内容\n")
	_, _, err := runParams(&contentUpdateParams{mode: "replace_all", selEllipsis: "年度计划", content: "新计划"})
	if err == nil || !strings.Contains(err.Error(), "标题") {
		t.Fatalf("只命中标题应报错，得到: %v", err)
	}
	if len(f.bodies) != 0 {
		t.Fatal("只命中标题不应发出写请求")
	}
}

// 标题与正文都含 pattern 时，正文命中需带上下文避开标题（否则服务端报多处命中或误改标题）。
func TestTextReplaceAvoidsTitleOccurrence(t *testing.T) {
	f := newFakeDocsAI(t, "# 年度计划\n\n本年度计划如下\n")
	if _, _, err := runParams(&contentUpdateParams{mode: "replace_range", selEllipsis: "年度计划", content: "季度计划"}); err != nil {
		t.Fatalf("替换失败: %v", err)
	}
	if want := "# 年度计划\n\n本季度计划如下\n"; f.doc != want {
		t.Fatalf("文档 = %q，期望 %q", f.doc, want)
	}
}

// 同一行里完全重复的命中无法构造唯一窗口时，必须在写入前整体拒绝，而不是部分替换。
func TestTextReplaceUnresolvableFailsBeforeAnyWrite(t *testing.T) {
	f := newFakeDocsAI(t, "# 标题\n\n旧文本\n\n旧文本\n\n其它 旧文本 唯一\n")
	_, _, err := runParams(&contentUpdateParams{mode: "replace_all", selEllipsis: "旧文本", content: "新"})
	if err == nil || !strings.Contains(err.Error(), "无法构造全文唯一的替换上下文") {
		t.Fatalf("应整体拒绝，得到: %v", err)
	}
	if len(f.bodies) != 0 {
		t.Fatalf("拒绝前不应有任何写入，实际 %d 次", len(f.bodies))
	}
}

// 窗口上下文不得跨越 Markdown 标记（** 等），也不得把行首块标记带进 content。
func TestBuildTextReplaceWindowsStaysInsideSafeChars(t *testing.T) {
	doc := "# T\n\n- 列表项 foo\n- 其它项 foo bar\n\n**粗** foo 尾\n"
	occ := findTextOccurrences(doc, "foo", docsAITitleLineEnd(doc))
	if len(occ) != 3 {
		t.Fatalf("命中数 = %d，期望 3", len(occ))
	}
	windows, unresolved := buildTextReplaceWindows(doc, "foo", "X", occ, docsAITitleLineEnd(doc))
	if len(unresolved) != 0 {
		t.Fatalf("不应有无法解析的命中: %v", unresolved)
	}
	for _, w := range windows {
		if strings.HasPrefix(w.pattern, "-") || strings.Contains(w.pattern, "*") || strings.Contains(w.pattern, "\n") {
			t.Fatalf("窗口越界: %q", w.pattern)
		}
		if strings.HasPrefix(w.pattern, " ") || strings.HasSuffix(w.pattern, " ") {
			t.Fatalf("窗口首尾不应是空格: %q", w.pattern)
		}
		if countOverlapping(doc, w.pattern) != 1 {
			t.Fatalf("窗口不唯一: %q", w.pattern)
		}
		if strings.Replace(w.pattern, "foo", "X", 1) != w.content {
			t.Fatalf("content 应只替换命中部分: pattern=%q content=%q", w.pattern, w.content)
		}
	}
}

// TestStrReplaceModeServerFailureSurfacesWarnings 服务端 result=failed 时非零退出，并透出 warnings 与 log_id。
func TestStrReplaceModeServerFailureSurfacesWarnings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Tt-Logid", "log-abc")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case strings.HasSuffix(r.URL.Path, "/fetch"):
			fmt.Fprint(w, `{"code":0,"msg":"","data":{"document":{"content":"# T\n\nv1.0 发布","revision_id":3}}}`)
		case r.Method == http.MethodPut:
			// 读取后文档被并发修改：服务端已找不到
			fmt.Fprint(w, `{"code":0,"msg":"","data":{"document":{"revision_id":3},"result":"failed","warnings":["degrade_code=1013,msg=not found"]}}`)
		}
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)

	stdout, _, err := runParams(&contentUpdateParams{mode: "str_replace", pattern: "v1.0", content: "v2.0", output: "json", contentSet: true})
	if err == nil {
		t.Fatal("result=failed 必须非零退出")
	}
	for _, want := range []string{"degrade_code=1013", "log_id=log-abc"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("错误缺少 %q: %v", want, err)
		}
	}
	if !strings.Contains(stdout, `"result": "failed"`) || !strings.Contains(stdout, `"log_id": "log-abc"`) {
		t.Fatalf("JSON 模式应输出完整 data（含 log_id）: %s", stdout)
	}
}

// 纯文字自动走 XML 时，& < > 必须按 XML 文本节点转义（服务端 XML 序列化中 & 写作 &amp;）。
func TestTextReplaceXMLEscapesAmpersand(t *testing.T) {
	f := newFakeDocsAI(t, "# T\n\n研发 &amp; 测试 流程\n")
	if _, _, err := runParams(&contentUpdateParams{mode: "replace_all", selEllipsis: "研发 & 测试", content: "研发 & 运维"}); err != nil {
		t.Fatalf("替换失败: %v", err)
	}
	if len(f.bodies) != 1 || f.bodies[0]["pattern"] != "研发 &amp; 测试" || f.bodies[0]["content"] != "研发 &amp; 运维" {
		t.Fatalf("XML 转义异常: %#v", f.bodies)
	}
}

// 显式 --doc-format markdown 时尊重用户选择，不自动切 XML。
func TestTextReplaceExplicitMarkdownFormatRespected(t *testing.T) {
	f := newFakeDocsAI(t, "# T\n\n普通 词A 文本\n")
	if _, _, err := runParams(&contentUpdateParams{mode: "str_replace", pattern: "词A", content: "词B", docFormat: "markdown", formatSet: true}); err != nil {
		t.Fatalf("替换失败: %v", err)
	}
	if f.bodies[0]["format"] != "markdown" {
		t.Fatalf("显式 markdown 不应被改写，实际 %v", f.bodies[0]["format"])
	}
}
