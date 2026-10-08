package cmd

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// slidesTestRequest 记录 mock 服务端收到的一次 slides 请求。
type slidesTestRequest struct {
	Method string
	Path   string
	Query  url.Values
	Body   map[string]any
}

// slidesMockServer 按 "METHOD PATH-后缀" 路由返回预置响应，并记录所有请求。
type slidesMockServer struct {
	t        *testing.T
	mu       sync.Mutex
	requests []slidesTestRequest
	routes   map[string]func(r *http.Request, body map[string]any) (int, string)
}

func newSlidesMockServer(t *testing.T) (*slidesMockServer, *httptest.Server) {
	m := &slidesMockServer{t: t, routes: map[string]func(*http.Request, map[string]any) (int, string){}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mockAuthHandler(w, r) {
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			_ = json.Unmarshal(raw, &body)
		}
		m.mu.Lock()
		m.requests = append(m.requests, slidesTestRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Body: body})
		m.mu.Unlock()
		for key, fn := range m.routes {
			parts := strings.SplitN(key, " ", 2)
			if r.Method == parts[0] && strings.HasSuffix(r.URL.Path, parts[1]) {
				status, resp := fn(r, body)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, resp)
				return
			}
		}
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return m, srv
}

func (m *slidesMockServer) on(method, suffix string, fn func(r *http.Request, body map[string]any) (int, string)) {
	m.routes[method+" "+suffix] = fn
}

func (m *slidesMockServer) find(method, suffix string) []slidesTestRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []slidesTestRequest
	for _, r := range m.requests {
		if r.Method == method && strings.HasSuffix(r.Path, suffix) {
			out = append(out, r)
		}
	}
	return out
}

// resetSlidesCmdFlags 把命令 flag 复位到默认值（含 slice 类型），便于多个测试复用同一个全局命令对象。
func resetSlidesCmdFlags(c *cobra.Command) {
	c.Flags().VisitAll(func(f *pflag.Flag) {
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			_ = sv.Replace([]string{})
		} else {
			_ = f.Value.Set(f.DefValue)
		}
		f.Changed = false
	})
}

func runSlidesCmd(t *testing.T, c *cobra.Command, args []string, flags map[string]string) (string, error) {
	t.Helper()
	resetSlidesCmdFlags(c)
	t.Cleanup(func() { resetSlidesCmdFlags(c) })
	for k, v := range flags {
		if err := c.Flags().Set(k, v); err != nil {
			t.Fatalf("set --%s=%q: %v", k, v, err)
		}
	}
	return captureAppsStdout(t, func() error { return c.RunE(c, args) })
}

const testSlideXML = `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data><shape type="text" topLeftX="80" topLeftY="80" width="600" height="80"><content><p>hi</p></content></shape></data></slide>`

// TestSlidesGet_RejectsRevisionZero 回归：revision_id=0 过去被静默改成 -1；服务端对 0 报 3350001，
// 现在作为用法错误（exit 2）在本地拒绝，且不发请求。
func TestSlidesGet_RejectsRevisionZero(t *testing.T) {
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	for _, rev := range []string{"0", "-2"} {
		_, err := runSlidesCmd(t, slidesGetCmd, []string{"pres_x"}, map[string]string{"revision-id": rev})
		if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
			t.Fatalf("--revision-id %s 应是用法错误，got %v", rev, err)
		}
	}
	if len(m.requests) != 0 {
		t.Fatalf("非法 revision 不应发请求，实际 %d 次", len(m.requests))
	}
	if strings.Contains(slidesGetCmd.Long, "读取指定版本") {
		t.Fatal("帮助文案不应再承诺可读取指定版本（服务端忽略正整数版本号）")
	}
}

// TestGetSlides_PassesRevisionThrough 回归：client 不再把 0 改写成 -1，按调用方传入值下发。
func TestGetSlides_PassesRevisionThrough(t *testing.T) {
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	m.on("GET", "/xml_presentations/pres_x", func(r *http.Request, _ map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"xml_presentation":{"content":"<presentation/>","revision_id":3}}}`
	})
	if _, err := client.GetSlides("pres_x", 7); err != nil {
		t.Fatal(err)
	}
	if got := m.find("GET", "/xml_presentations/pres_x")[0].Query.Get("revision_id"); got != "7" {
		t.Fatalf("revision_id = %q, want 7", got)
	}
}

func TestSlidesGet_SingleSlideAndOutputFile(t *testing.T) {
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	m.on("GET", "/xml_presentations/pres_x/slide", func(r *http.Request, _ map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"revision_id":5,"slide":{"slide_id":"s2","content":"<slide id=\"s2\"/>"}}}`
	})
	out, err := runSlidesCmd(t, slidesGetCmd, []string{"pres_x"}, map[string]string{"slide-number": "2", "output": "json"})
	if err != nil {
		t.Fatal(err)
	}
	req := m.find("GET", "/slide")[0]
	if req.Query.Get("slide_number") != "2" || req.Query.Get("revision_id") != "-1" {
		t.Fatalf("query = %v", req.Query)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("输出不是 JSON: %v\n%s", err, out)
	}
	if got["scope"] != "slide" || got["slide_id"] != "s2" || got["content"] != `<slide id="s2"/>` {
		t.Fatalf("输出 = %v", got)
	}

	dir := t.TempDir()
	file := filepath.Join(dir, "page.xml")
	if _, err := runSlidesCmd(t, slidesGetCmd, []string{"pres_x"}, map[string]string{"slide-id": "s2", "output-file": file}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(file)
	if string(data) != `<slide id="s2"/>` {
		t.Fatalf("文件内容 = %q", data)
	}
}

// TestSlidesAddSlide_LintInBodyAndPlaceholderUpload 锁住两个契约：lint 开关以 lint_xml 放在请求体（不是 query），
// 以及 <img src="@path"> 先上传再替换为 file_token。
func TestSlidesAddSlide_LintInBodyAndPlaceholderUpload(t *testing.T) {
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	m.on("POST", "/medias/upload_all", func(*http.Request, map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"file_token":"boxcnIMG"}}`
	})
	m.on("POST", "/xml_presentations/pres_x/slide", func(*http.Request, map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"slide_id":"sNew","revision_id":9,"issues":[{"level":"warning"}]}}`
	})

	dir := t.TempDir()
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })
	_ = os.Chdir(dir)
	if err := os.WriteFile("pic.png", []byte("\x89PNG fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	slide := `<slide><data><img src="@./pic.png" topLeftX="0" topLeftY="0" width="10" height="10"/></data></slide>`

	out, err := runSlidesCmd(t, slidesAddSlideCmd, []string{"pres_x"}, map[string]string{"slide": slide, "before-slide-id": "s1"})
	if err != nil {
		t.Fatalf("add-slide: %v", err)
	}
	if len(m.find("POST", "/medias/upload_all")) != 1 {
		t.Fatal("应先上传占位图片")
	}
	req := m.find("POST", "/xml_presentations/pres_x/slide")[0]
	if req.Body[slidesLintBodyKey] != true {
		t.Fatalf("lint_xml 应在请求体且为 true: %v", req.Body)
	}
	if _, inQuery := req.Query[slidesLintBodyKey]; inQuery {
		t.Fatal("lint_xml 不能放在 query（会被网关丢弃）")
	}
	content := req.Body["slide"].(map[string]any)["content"].(string)
	if !strings.Contains(content, `src="boxcnIMG"`) || strings.Contains(content, "@./pic.png") {
		t.Fatalf("占位符应替换为 file_token: %s", content)
	}
	if req.Body["before_slide_id"] != "s1" {
		t.Fatalf("before_slide_id = %v", req.Body["before_slide_id"])
	}
	for _, want := range []string{`"slide_id": "sNew"`, `"images_uploaded": 1`, `"issues"`, `"revision_id": 9`} {
		if !strings.Contains(out, want) {
			t.Errorf("输出缺 %s:\n%s", want, out)
		}
	}

	// --no-lint → lint_xml=false
	if _, err := runSlidesCmd(t, slidesAddSlideCmd, []string{"pres_x"}, map[string]string{"slide": testSlideXML, "no-lint": "true"}); err != nil {
		t.Fatal(err)
	}
	reqs := m.find("POST", "/xml_presentations/pres_x/slide")
	if reqs[len(reqs)-1].Body[slidesLintBodyKey] != false {
		t.Fatalf("--no-lint 应下发 lint_xml=false: %v", reqs[len(reqs)-1].Body)
	}
	if _, ok := reqs[len(reqs)-1].Body["before_slide_id"]; ok {
		t.Fatal("未传 --before-slide-id 时不能下发空 before_slide_id")
	}
}

// TestSlidesAddSlide_LintRejectionHint 验证 4000153（随 HTTP 400 下发）被解析为业务码并补充 --no-lint 提示。
func TestSlidesAddSlide_LintRejectionHint(t *testing.T) {
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	report := `{"summary":{"error_count":2,"warning_count":1},"schema_issues":"x"}`
	m.on("POST", "/xml_presentations/pres_x/slide", func(*http.Request, map[string]any) (int, string) {
		b, _ := json.Marshal(map[string]any{"code": slidesLintBlockedCode, "msg": report})
		return 400, string(b)
	})
	_, err := runSlidesCmd(t, slidesAddSlideCmd, []string{"pres_x"}, map[string]string{"slide": testSlideXML})
	if err == nil {
		t.Fatal("期望 lint 拒绝错误")
	}
	if !client.HasAPICode(err, slidesLintBlockedCode) {
		t.Fatalf("应保留业务码 4000153: %v", err)
	}
	for _, want := range []string{"2 个 error 级问题", "--no-lint", "schema_issues"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("错误提示缺 %q: %v", want, err)
		}
	}
}

func TestSlidesAddSlide_RejectsInvalidXMLLocally(t *testing.T) {
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	for _, bad := range []string{"", "<presentation/>", `<?xml version="1.0"?><slide/>`, "<slide><data></slide>", "<slide/><slide/>"} {
		_, err := runSlidesCmd(t, slidesAddSlideCmd, []string{"pres_x"}, map[string]string{"slide": bad})
		if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
			t.Errorf("--slide %q 应是用法错误，got %v", bad, err)
		}
	}
	if len(m.requests) != 0 {
		t.Fatalf("本地校验失败不应发请求: %d", len(m.requests))
	}
}

// TestSlidesReplaceSlide_InjectsIDAndContent 锁住 block_replace 注入 id（缺了服务端 3350001）、
// <shape/> 补 <content/>、别名规范化，以及 failed_part_index/failed_reason/issues 透出。
func TestSlidesReplaceSlide_InjectsIDAndContent(t *testing.T) {
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	m.on("POST", "/xml_presentations/pres_x/slide/replace", func(*http.Request, map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"revision_id":4,"failed_part_index":1,"failed_reason":"block not found","issues":[]}}`
	})
	parts := `[{"action":"replace","target_id":"b1","content":"<shape type=\"text\"><content><p>x</p></content></shape>"},{"action":"block_insert","insertion":"<shape type=\"rect\" width=\"10\" height=\"10\"/>"}]`
	out, err := runSlidesCmd(t, slidesReplaceSlideCmd, []string{"pres_x"}, map[string]string{"slide-id": "s1", "parts": parts})
	if err != nil {
		t.Fatal(err)
	}
	req := m.find("POST", "/slide/replace")[0]
	if req.Query.Get("slide_id") != "s1" || req.Query.Get("revision_id") != "-1" {
		t.Fatalf("query = %v", req.Query)
	}
	sent := req.Body["parts"].([]any)
	p0 := sent[0].(map[string]any)
	if p0["action"] != "block_replace" || p0["block_id"] != "b1" || !strings.Contains(p0["replacement"].(string), `id="b1"`) {
		t.Fatalf("part0 = %v", p0)
	}
	p1 := sent[1].(map[string]any)
	if p1["insertion"] != `<shape type="rect" width="10" height="10"><content/></shape>` {
		t.Fatalf("part1 insertion = %v", p1["insertion"])
	}
	for _, want := range []string{`"failed_part_index": 1`, `"failed_reason": "block not found"`, `"normalizations"`, `"target_id"`} {
		if !strings.Contains(out, want) {
			t.Errorf("输出缺 %s:\n%s", want, out)
		}
	}
}

func TestParseSlidesReplaceParts_Errors(t *testing.T) {
	cases := map[string]string{
		"空":           ``,
		"非数组":         `{"action":"block_replace"}`,
		"空数组":         `[]`,
		"str_replace": `[{"action":"str_replace","pattern":"a","replacement":"b"}]`,
		"整页":          `[{"action":"slide_replace","replacement":"<slide/>"}]`,
		"未知 action":   `[{"action":"move"}]`,
		"缺 action":    `[{"block_id":"b"}]`,
		"未知字段":        `[{"action":"block_replace","blockId":"b","replacement":"<shape/>"}]`,
		"缺 block_id":  `[{"action":"block_replace","replacement":"<shape/>"}]`,
		"别名冲突":        `[{"action":"block_replace","block_id":"a","target_id":"b","replacement":"<shape/>"}]`,
		"类型错误":        `[{"action":"block_insert","insertion":123}]`,
	}
	for name, raw := range cases {
		if _, _, err := parseSlidesReplaceParts(raw); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
			t.Errorf("%s: 应是用法错误，got %v", name, err)
		}
	}
	tooMany := "[" + strings.TrimSuffix(strings.Repeat(`{"action":"block_insert","insertion":"<shape/>"},`, maxSlidesReplaceParts+1), ",") + "]"
	if _, _, err := parseSlidesReplaceParts(tooMany); err == nil {
		t.Error("超过 200 条应报错")
	}
}

// TestSlidesUpdateSlide_StripsNoteIDAndStampsRoot 锁住整页替换的两项改写与 failed_reason 失败语义。
func TestSlidesUpdateSlide_StripsNoteIDAndStampsRoot(t *testing.T) {
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	failed := false
	m.on("POST", "/xml_presentations/pres_x/slide/replace", func(*http.Request, map[string]any) (int, string) {
		if failed {
			return 200, `{"code":0,"data":{"failed_reason":"slide not found"}}`
		}
		return 200, `{"code":0,"data":{"revision_id":8}}`
	})
	content := `<slide><data><shape type="text" id="b1"><content><p>x</p></content></shape></data><note id="stale"><content><p>n</p></content></note></slide>`
	if _, err := runSlidesCmd(t, slidesUpdateSlideCmd, []string{"pres_x"}, map[string]string{"slide-id": "s1", "content": content}); err != nil {
		t.Fatal(err)
	}
	part := m.find("POST", "/slide/replace")[0].Body["parts"].([]any)[0].(map[string]any)
	repl := part["replacement"].(string)
	if part["block_id"] != "s1" || !strings.HasPrefix(repl, `<slide id="s1">`) {
		t.Fatalf("根元素应注入 slide id: %v", part)
	}
	if strings.Contains(repl, "stale") || !strings.Contains(repl, `<shape type="text" id="b1">`) {
		t.Fatalf("应只去掉 note 的 id、保留元素 id: %s", repl)
	}

	// 根 id 与 --slide-id 不一致 → 用法错误，不发请求
	before := len(m.requests)
	_, err := runSlidesCmd(t, slidesUpdateSlideCmd, []string{"pres_x"}, map[string]string{"slide-id": "s1", "content": `<slide id="s9"/>`})
	if err == nil || !clierr.HasKind(err, clierr.KindUsage) || len(m.requests) != before {
		t.Fatalf("根 id 不一致应本地拒绝: %v", err)
	}
	// 根不是 <slide> → 用法错误
	if _, err := runSlidesCmd(t, slidesUpdateSlideCmd, []string{"pres_x"}, map[string]string{"slide-id": "s1", "content": `<shape/>`}); err == nil {
		t.Fatal("根不是 <slide> 应报错")
	}
	// failed_reason → 失败退出
	failed = true
	_, err = runSlidesCmd(t, slidesUpdateSlideCmd, []string{"pres_x"}, map[string]string{"slide-id": "s1", "content": content})
	if err == nil || !strings.Contains(err.Error(), "未更新") || !strings.Contains(err.Error(), "slides get") {
		t.Fatalf("failed_reason 应按失败返回并给出 not found 提示: %v", err)
	}
}

func TestSlidesDeleteSlide_RequiresConfirmation(t *testing.T) {
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	m.on("DELETE", "/xml_presentations/pres_x/slide", func(*http.Request, map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"revision_id":3}}`
	})
	origInteractive := confirmIsInteractive
	confirmIsInteractive = func() bool { return false }
	t.Cleanup(func() { confirmIsInteractive = origInteractive })

	_, err := runSlidesCmd(t, slidesDeleteSlideCmd, []string{"pres_x"}, map[string]string{"slide-id": "s1"})
	if err == nil || !clierr.HasKind(err, clierr.KindConfirmationRequired) {
		t.Fatalf("非交互且无 --yes 应要求确认，got %v", err)
	}
	if len(m.find("DELETE", "/slide")) != 0 {
		t.Fatal("未确认不应发 DELETE")
	}
	// dry-run 优先于确认
	if _, err := runSlidesCmd(t, slidesDeleteSlideCmd, []string{"pres_x"}, map[string]string{"slide-id": "s1", "dry-run": "true"}); err != nil {
		t.Fatalf("dry-run 不需要确认: %v", err)
	}
	origYes := assumeYes
	assumeYes = true
	t.Cleanup(func() { assumeYes = origYes })
	out, err := runSlidesCmd(t, slidesDeleteSlideCmd, []string{"pres_x"}, map[string]string{"slide-id": "s1"})
	if err != nil {
		t.Fatal(err)
	}
	req := m.find("DELETE", "/slide")[0]
	if req.Query.Get("slide_id") != "s1" {
		t.Fatalf("query = %v", req.Query)
	}
	if !strings.Contains(out, `"deleted": true`) {
		t.Fatalf("输出 = %s", out)
	}
}

// TestSlidesCreate_PagesStopOnFailureWithProgress 验证带页面创建时：先建空 deck，逐页添加；
// 第 2 页失败时停止，并在错误中给出已创建的 deck 与进度。
func TestSlidesCreate_PagesStopOnFailureWithProgress(t *testing.T) {
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-test") // User 身份创建，不触发 Bot 自动授权
	m.on("POST", "/slides_ai/v1/xml_presentations", func(*http.Request, map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"xml_presentation_id":"pres_new","revision_id":1}}`
	})
	calls := 0
	m.on("POST", "/xml_presentations/pres_new/slide", func(*http.Request, map[string]any) (int, string) {
		calls++
		if calls == 2 {
			return 400, `{"code":3350001,"msg":"invalid param"}`
		}
		return 200, `{"code":0,"data":{"slide_id":"s1","revision_id":2}}`
	})
	pages, _ := json.Marshal([]string{testSlideXML, testSlideXML, testSlideXML})
	_, err := runSlidesCmd(t, slidesCreateCmd, nil, map[string]string{"title": "t", "slides": string(pages)})
	if err == nil {
		t.Fatal("第 2 页失败应报错")
	}
	for _, want := range []string{"pres_new", "1/3", "add-slide", "3350001"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("错误缺 %q: %v", want, err)
		}
	}
	if calls != 2 {
		t.Fatalf("失败后应停止，实际调用 %d 次", calls)
	}
}

func TestSlidesCreate_ValidatesPagesBeforeCreating(t *testing.T) {
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	cases := []map[string]string{
		{"slides": "null"},
		{"slides": ""},
		{"slides": `["<presentation/>"]`},
		{"slide": `<slide><data><img src="@./missing.png"/></data></slide>`},
	}
	for _, flags := range cases {
		_, err := runSlidesCmd(t, slidesCreateCmd, nil, flags)
		if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
			t.Errorf("%v 应是用法错误，got %v", flags, err)
		}
	}
	if len(m.requests) != 0 {
		t.Fatalf("页面校验失败不应创建演示文稿: %d 次请求", len(m.requests))
	}
}

func TestSlidesScreenshot_SavesFilesWithoutBase64OnStdout(t *testing.T) {
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	png := base64.StdEncoding.EncodeToString([]byte("PNGDATA"))
	jpg := base64.StdEncoding.EncodeToString([]byte("JPGDATA"))
	m.on("POST", "/xml_presentations/pres_x/slide_images", func(*http.Request, map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"slide_images":[{"slide_id":"s1","slide_number":1,"format":1,"data":"` + png + `"},{"slide_id":"s2","slide_number":2,"format":2,"data":"` + jpg + `"}]}}`
	})
	m.on("POST", "/slide_image/render", func(*http.Request, map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"slide_image":{"format":2,"data":"` + jpg + `"}}}`
	})
	dir := t.TempDir()
	out, err := runSlidesCmd(t, slidesScreenshotCmd, []string{"pres_x"}, map[string]string{"slide-number": "1,2", "output-dir": dir})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, png) || strings.Contains(out, jpg) {
		t.Fatal("stdout 不应包含 Base64 图片数据")
	}
	req := m.find("POST", "/slide_images")[0]
	if nums, _ := req.Body["slide_numbers"].([]any); len(nums) != 2 {
		t.Fatalf("slide_numbers = %v", req.Body)
	}
	b, err := os.ReadFile(filepath.Join(dir, "pres_x_p001_s1.png"))
	if err != nil || string(b) != "PNGDATA" {
		t.Fatalf("第 1 张截图: %v %q", err, b)
	}
	if _, err := os.Stat(filepath.Join(dir, "pres_x_p002_s2.jpg")); err != nil {
		t.Fatal(err)
	}
	// --output 扩展名与实际格式不符时按实际格式改名
	out, err = runSlidesCmd(t, slidesScreenshotCmd, nil, map[string]string{"content": testSlideXML, "output": filepath.Join(dir, "preview.png")})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, filepath.Join(dir, "preview.jpg")) {
		t.Fatalf("应按实际 jpeg 格式落盘为 preview.jpg:\n%s", out)
	}
	// 选择器校验
	for _, flags := range []map[string]string{
		{"slide-number": "1", "slide-id": "s1"},
		{"slide-number": "0"},
		{"slide-number": "1,2", "output": "a.png"},
		{"slide-id": "1,2,3,4,5,6,7,8,9,10,11"},
	} {
		if _, err := runSlidesCmd(t, slidesScreenshotCmd, []string{"pres_x"}, flags); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
			t.Errorf("%v 应是用法错误，got %v", flags, err)
		}
	}
}

func TestSlidesXMLHelpers(t *testing.T) {
	t.Run("ensureXMLRootID", func(t *testing.T) {
		cases := []struct{ in, want string }{
			{`<shape type="rect"/>`, `<shape type="rect" id="b1"/>`},
			{`<shape id="b1" type="rect"/>`, `<shape id="b1" type="rect"/>`},
			{`<shape id='old'><content/></shape>`, `<shape id='b1'><content/></shape>`},
			{`<shape data-id="x" type="rect"></shape>`, `<shape data-id="x" type="rect" id="b1"></shape>`},
			{"  <!-- c --><img src=\"t\"/>", "  <!-- c --><img src=\"t\" id=\"b1\"/>"},
		}
		for _, tc := range cases {
			got, err := ensureXMLRootID(tc.in, "b1")
			if err != nil || got != tc.want {
				t.Errorf("ensureXMLRootID(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
			}
		}
		if _, err := ensureXMLRootID("plain text", "b1"); err == nil {
			t.Error("无根元素应报错")
		}
	})
	t.Run("ensureShapeHasContent", func(t *testing.T) {
		cases := map[string]string{
			`<shape type="rect"/>`:                       `<shape type="rect"><content/></shape>`,
			`<shape type="rect"></shape>`:                `<shape type="rect"><content/></shape>`,
			`<shape><content><p>x</p></content></shape>`: `<shape><content><p>x</p></content></shape>`,
			`<shape><p>x</p></shape>`:                    `<shape><p>x</p></shape>`,
			`<img src="x"/>`:                             `<img src="x"/>`,
		}
		for in, want := range cases {
			if got := ensureShapeHasContent(in); got != want {
				t.Errorf("ensureShapeHasContent(%q) = %q, want %q", in, got, want)
			}
		}
	})
	t.Run("stripSlideNoteID", func(t *testing.T) {
		in := `<slide id="s"><data><shape id="b"><content><p>note id="x"</p></content></shape></data><note id='n1' x="1"><content/></note></slide>`
		want := `<slide id="s"><data><shape id="b"><content><p>note id="x"</p></content></shape></data><note x="1"><content/></note></slide>`
		if got := stripSlideNoteID(in); got != want {
			t.Errorf("stripSlideNoteID = %q, want %q", got, want)
		}
	})
	t.Run("placeholders", func(t *testing.T) {
		xmls := []string{`<img src="@./a.png"/><img  src = '@b.jpg' /><img src="token"/>`, `<img src="@./a.png"/>`}
		got := extractImagePlaceholderPaths(xmls)
		if len(got) != 2 || got[0] != "./a.png" || got[1] != "b.jpg" {
			t.Fatalf("extract = %v", got)
		}
		out := replaceImagePlaceholders(xmls[0], map[string]string{"./a.png": "T1", "b.jpg": "T2"})
		if out != `<img src="T1"/><img  src = 'T2' /><img src="token"/>` {
			t.Fatalf("replace = %q", out)
		}
	})
}

func TestResolveSlidesInputValue(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "page.xml")
	_ = os.WriteFile(p, []byte("\ufeff<slide/>"), 0o644)
	if got, err := resolveSlidesInputValue("@"+p, "--slide"); err != nil || got != "<slide/>" {
		t.Fatalf("@file = %q, %v（应去掉 BOM）", got, err)
	}
	if got, _ := resolveSlidesInputValue("@@literal", "--slide"); got != "@literal" {
		t.Fatalf("@@ 转义 = %q", got)
	}
	orig := slidesStdin
	slidesStdin = strings.NewReader("<slide>stdin</slide>")
	t.Cleanup(func() { slidesStdin = orig })
	if got, _ := resolveSlidesInputValue("-", "--slide"); got != "<slide>stdin</slide>" {
		t.Fatalf("stdin = %q", got)
	}
	if _, err := resolveSlidesInputValue("@"+filepath.Join(dir, "missing.xml"), "--slide"); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("缺失文件应是用法错误: %v", err)
	}
}

func TestEnrichSlidesWriteError_PassThroughOthers(t *testing.T) {
	base := errors.New("boom")
	if got := enrichSlidesWriteError(base, ""); got != base {
		t.Fatalf("其他错误应原样返回: %v", got)
	}
	if enrichSlidesWriteError(nil, "x") != nil {
		t.Fatal("nil 应返回 nil")
	}
}

func TestGetSlides_ZeroNotRewritten(t *testing.T) {
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	m.on("GET", "/xml_presentations/pres_x", func(r *http.Request, _ map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"xml_presentation":{"content":"<presentation/>","revision_id":3}}}`
	})
	_, _ = client.GetSlides("pres_x", 0)
	if got := m.find("GET", "/xml_presentations/pres_x")[0].Query.Get("revision_id"); got != "0" {
		t.Fatalf("client 不应把 0 改写成 -1，got %q", got)
	}
}

// TestSlidesMediaUpload_AcceptsSlidesURL --presentation-token 接受 /slides/ URL，docx URL 本地拒绝。
func TestSlidesMediaUpload_AcceptsSlidesURL(t *testing.T) {
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	m.on("POST", "/medias/upload_all", func(*http.Request, map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"file_token":"boxcnX"}}`
	})
	f := filepath.Join(t.TempDir(), "a.png")
	_ = os.WriteFile(f, []byte("png"), 0o644)
	out, err := runSlidesCmd(t, slidesMediaUploadCmd, nil, map[string]string{"file": f, "presentation-token": "https://xxx.feishu.cn/slides/pres_url", "output": "json"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"presentation_id": "pres_url"`) {
		t.Fatalf("应解析 URL 中的演示文稿 ID:\n%s", out)
	}
	if _, err := runSlidesCmd(t, slidesMediaUploadCmd, nil, map[string]string{"file": f, "presentation-token": "https://xxx.feishu.cn/docx/doc1"}); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("docx URL 应用法错误: %v", err)
	}
}
