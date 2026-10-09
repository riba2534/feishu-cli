package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/pflag"
)

// fakeDocWriteServer 模拟 docs_ai 建文档/更新 + 素材上传 + batch_update 绑定 + 占位块清理。
type fakeDocWriteServer struct {
	mu       sync.Mutex
	creates  []map[string]any
	puts     []map[string]any
	uploads  []string // parent_node|parent_type
	names    []string // file_name
	batches  []map[string]any
	failFile bool
	calls    int
}

var docWriteMarkerRe = regexp.MustCompile(`@lcli_(img|file)_[0-9a-f]{32}`)

func newDocsAIBlocks(content string) []map[string]any {
	var blocks []map[string]any
	for i, m := range docWriteMarkerRe.FindAllStringSubmatch(content, -1) {
		typ := "image"
		if m[1] == "file" {
			typ = "file"
		}
		blocks = append(blocks, map[string]any{"block_id": fmt.Sprintf("blk_%s_%d", typ, i), "block_token": m[0], "block_type": typ})
	}
	blocks = append(blocks, map[string]any{"block_id": "blk_wb", "block_token": "board_tok", "block_type": "whiteboard"})
	return blocks
}

func (f *fakeDocWriteServer) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls++
		readBody := func() map[string]any {
			var body map[string]any
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &body)
			return body
		}
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.Method == http.MethodPost && r.URL.Path == "/open-apis/docs_ai/v1/documents":
			body := readBody()
			f.creates = append(f.creates, body)
			content, _ := body["content"].(string)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
				"document": map[string]any{"document_id": "doxnew", "revision_id": 1, "url": "https://example.feishu.cn/docx/doxnew", "new_blocks": newDocsAIBlocks(content)},
				"result":   "success",
			}})
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/open-apis/docs_ai/v1/documents/"):
			body := readBody()
			f.puts = append(f.puts, body)
			content, _ := body["content"].(string)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
				"document": map[string]any{"revision_id": 3 + len(f.puts), "new_blocks": newDocsAIBlocks(content)}, "result": "success"}})
		case r.URL.Path == "/open-apis/drive/v1/medias/upload_all":
			_ = r.ParseMultipartForm(1 << 20)
			parent := r.FormValue("parent_node")
			f.uploads = append(f.uploads, parent+"|"+r.FormValue("parent_type"))
			f.names = append(f.names, r.FormValue("file_name"))
			if f.failFile && r.FormValue("parent_type") == "docx_file" {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"code":1061002,"msg":"params error"}`)
				return
			}
			fmt.Fprintf(w, `{"code":0,"msg":"ok","data":{"file_token":"tok_%s"}}`, parent)
		case strings.HasSuffix(r.URL.Path, "/blocks/batch_update"):
			f.batches = append(f.batches, readBody())
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"blocks":[],"document_revision_id":7}}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/blocks/blk_file_1"):
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"block":{"block_id":"blk_file_1","block_type":23,"parent_id":"view_1"}}}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/blocks/view_1"):
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"block":{"block_id":"view_1","block_type":33,"parent_id":"doxnew","children":["blk_file_1"]}}}`)
		default:
			t.Errorf("意外请求 %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}
}

// setupDocWriteFixtures 在临时工作目录准备 PNG（1200×800）、附件、HTML 与 Mermaid 源文件。
func setupDocWriteFixtures(t *testing.T) string {
	t.Helper()
	dir := chdirTemp(t)
	writeSizedPNG(t, filepath.Join(dir, "a.png"), 1200, 800)
	writeDocTestFile(t, filepath.Join(dir, "r.txt"), "附件内容")
	writeDocTestFile(t, filepath.Join(dir, "w.html"), "<html><body>widget</body></html>")
	writeDocTestFile(t, filepath.Join(dir, "f.mmd"), "flowchart TD\nA --> B")
	orig := waitBetweenLocalUploads
	waitBetweenLocalUploads = 0
	t.Cleanup(func() { waitBetweenLocalUploads = orig })
	return dir
}

const docWriteXML = `<title>t</title><p>图文</p><img path="@a.png" width="600"/><source path="@r.txt" name="说明.txt"/>` +
	`<html5-block path="@w.html"/><whiteboard type="mermaid" path="@f.mmd"/>`

func TestDocCreateLocalResourcesEndToEnd(t *testing.T) {
	setupDocWriteFixtures(t)
	f := &fakeDocWriteServer{}
	server := httptest.NewServer(f.handler(t))
	t.Cleanup(server.Close)
	initDocUpdateTestConfig(t, server.URL)

	c := newDocCreateTestCmd(t, "--doc-format", "xml", "--content", docWriteXML,
		"--reference-map", `{"widget":{"r1":{"label":"x"}}}`, "--user-access-token", "u-test", "-o", "json")
	var runErr error
	stdout := captureStdout(t, func() { runErr = runDocCreateDocsAI(c, "u-test") })
	if runErr != nil {
		t.Fatalf("创建失败: %v\n%s", runErr, stdout)
	}

	if len(f.creates) != 1 {
		t.Fatalf("应只发一次建文档请求: %d", len(f.creates))
	}
	body := f.creates[0]
	content, _ := body["content"].(string)
	if !regexp.MustCompile(`<img path="@lcli_img_[0-9a-f]{32}" width="1200" height="800" scale="0.500000"/>`).MatchString(content) ||
		!regexp.MustCompile(`<source path="@lcli_file_[0-9a-f]{32}" name="说明.txt"/>`).MatchString(content) ||
		!strings.Contains(content, `<html5-block data-ref="html5_1"></html5-block>`) ||
		!strings.Contains(content, "<whiteboard type=\"mermaid\">flowchart TD\nA --> B</whiteboard>") {
		t.Fatalf("请求内容改写异常:\n%s", content)
	}
	ref, _ := jsonMarshalNoEscape(body["reference_map"])
	if !strings.Contains(ref, `"html5_1":{"data":"<html><body>widget</body></html>"}`) || !strings.Contains(ref, `"widget":{"r1":{"label":"x"}}`) {
		t.Fatalf("reference_map 异常: %s", ref)
	}
	if body["extra_param"] == nil {
		t.Fatal("建文档应带异步 extra_param")
	}
	if strings.Join(f.uploads, ",") != "blk_image_0|docx_image,blk_file_1|docx_file" {
		t.Fatalf("上传请求异常: %v", f.uploads)
	}
	reqs, _ := json.Marshal(f.batches[0]["requests"])
	if !strings.Contains(string(reqs), `"replace_image":{"height":800,"scale":0.5,"token":"tok_blk_image_0","width":1200}`) ||
		!strings.Contains(string(reqs), `"replace_file":{"token":"tok_blk_file_1"}`) {
		t.Fatalf("绑定请求异常: %s", reqs)
	}
	if len(f.puts) != 0 {
		t.Fatalf("全部成功时不应清理: %v", f.puts)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("输出不是 JSON: %v\n%s", err, stdout)
	}
	if out["document_id"] != "doxnew" || fmt.Sprint(out["revision_id"]) != "7" {
		t.Fatalf("revision_id 应更新为绑定后的版本: %v", out)
	}
	if _, ok := out["local_resource_failures"]; ok {
		t.Fatal("全部成功时不应输出 local_resource_failures")
	}
	if strings.Count(stdout, `"status": "bound"`) != 2 || strings.Contains(stdout, "@lcli_") ||
		!strings.Contains(stdout, `"block_token": "tok_blk_image_0"`) || !strings.Contains(stdout, `"block_token": "board_tok"`) {
		t.Fatalf("输出应包含逐项 bound、new_blocks 换成素材 token 且不残留占位标记:\n%s", stdout)
	}
}

func TestDocCreateLocalResourceFailureCleansUp(t *testing.T) {
	setupDocWriteFixtures(t)
	f := &fakeDocWriteServer{failFile: true}
	server := httptest.NewServer(f.handler(t))
	t.Cleanup(server.Close)
	initDocUpdateTestConfig(t, server.URL)

	c := newDocCreateTestCmd(t, "--doc-format", "xml", "--content", `<img path="@a.png"/><source path="@r.txt"/>`, "-o", "json")
	var runErr error
	stdout := captureStdout(t, func() { runErr = runDocCreateDocsAI(c, "u-test") })
	if runErr == nil || !strings.Contains(runErr.Error(), "未能写入") || clierr.HasKind(runErr, clierr.KindUsage) {
		t.Fatalf("附件上传失败应以业务错误非零退出: %v", runErr)
	}
	if len(f.puts) != 1 || f.puts[0]["command"] != "block_delete" || f.puts[0]["block_id"] != "view_1" {
		t.Fatalf("应删除失败附件的外层视图块: %#v", f.puts)
	}
	if !strings.Contains(stdout, `"local_resource_failures"`) || !strings.Contains(stdout, `"cleanup_status": "succeeded"`) ||
		!strings.Contains(stdout, `"document_id": "doxnew"`) || strings.Contains(stdout, "@lcli_") {
		t.Fatalf("JSON 应保留文档信息并给出失败明细:\n%s", stdout)
	}
	if !strings.Contains(stdout, `"revision_id": 4`) {
		t.Fatalf("revision_id 应更新为清理后的版本:\n%s", stdout)
	}
}

func TestDocCreateDryRunNoNetwork(t *testing.T) {
	setupDocWriteFixtures(t)
	f := &fakeDocWriteServer{}
	server := httptest.NewServer(f.handler(t))
	t.Cleanup(server.Close)
	initDocUpdateTestConfig(t, server.URL)

	c := newDocCreateTestCmd(t, "--doc-format", "xml", "--content", docWriteXML, "--dry-run")
	var runErr error
	stdout := captureStdout(t, func() { runErr = runDocCreateDocsAI(c, "") })
	if runErr != nil {
		t.Fatalf("dry-run 失败: %v", runErr)
	}
	if f.calls != 0 {
		t.Fatalf("dry-run 不应联网，实际请求 %d 次", f.calls)
	}
	for _, want := range []string{`"dry_run": true`, `/open-apis/docs_ai/v1/documents`, `/open-apis/drive/v1/medias/upload_all`,
		`"parent_node": "<local_image_1_block_id>"`, `/blocks/batch_update`, `"scale": 0.5`, `"local_resources"`,
		`data-ref=\"html5_1\"`, `/open-apis/drive/v1/permissions/<created_document_id>/members`} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("dry-run 输出缺少 %s:\n%s", want, stdout)
		}
	}
}

// runContentUpdateFlags 以给定 flag 运行 doc content-update，结束后恢复全部 flag。
func runContentUpdateFlags(t *testing.T, doc string, flags map[string]string) (string, error) {
	t.Helper()
	t.Cleanup(func() {
		docContentUpdateCmd.Flags().VisitAll(func(fl *pflag.Flag) { _ = fl.Value.Set(fl.DefValue); fl.Changed = false })
	})
	for k, v := range flags {
		if err := docContentUpdateCmd.Flags().Set(k, v); err != nil {
			t.Fatalf("设置 --%s 失败: %v", k, err)
		}
	}
	var runErr error
	stdout := captureStdout(t, func() { runErr = docContentUpdateCmd.RunE(docContentUpdateCmd, []string{doc}) })
	return stdout, runErr
}

func TestContentUpdateReferenceMapAndLocalFiles(t *testing.T) {
	setupDocWriteFixtures(t)
	f := &fakeDocWriteServer{}
	server := httptest.NewServer(f.handler(t))
	t.Cleanup(server.Close)
	initDocUpdateTestConfig(t, server.URL)

	_, err := runContentUpdateFlags(t, "doxold", map[string]string{
		"mode": "append", "doc-format": "xml", "user-access-token": "u-test", "output": "json",
		"markdown":      `<html5-block path="@w.html"/><whiteboard type="mermaid">@f.mmd</whiteboard><img path="@a.png"/>`,
		"reference-map": `{"widget":{"r1":{"label":"x"}}}`,
	})
	if err != nil {
		t.Fatalf("content-update 失败: %v", err)
	}
	if len(f.puts) != 1 {
		t.Fatalf("应只发一次写入: %d", len(f.puts))
	}
	body := f.puts[0]
	content, _ := body["content"].(string)
	ref, _ := json.Marshal(body["reference_map"])
	if !strings.Contains(content, `<html5-block data-ref="html5_1"></html5-block>`) || !strings.Contains(content, "flowchart TD") ||
		!strings.Contains(string(ref), `"widget":{"r1":{"label":"x"}}`) || !strings.Contains(string(ref), `"html5_1":{"data":`) {
		t.Fatalf("写入请求异常: %s\n%s", content, ref)
	}
	if body["command"] != "block_insert_after" || body["block_id"] != "-1" || len(f.uploads) != 1 {
		t.Fatalf("append 协议或上传异常: %#v %v", body, f.uploads)
	}
}

func TestContentUpdateReferenceMapValidation(t *testing.T) {
	initDocUpdateTestConfig(t, "http://127.0.0.1:59997")
	cases := []struct {
		flags map[string]string
		want  string
	}{
		{map[string]string{"mode": "delete_range", "block-id": "b1", "reference-map": `{"a":{}}`}, "只能用于写入内容的模式"},
		{map[string]string{"mode": "append", "markdown": "x", "reference-map": `[1]`}, "reference_map JSON"},
		{map[string]string{"mode": "append", "markdown": "x", "reference-map": ``}, "非空 JSON 对象"},
		{map[string]string{"mode": "append", "doc-format": "xml", "markdown": `<html5-block path="@/etc/passwd.html"/>`}, "敏感"},
		{map[string]string{"mode": "append", "doc-format": "xml", "markdown": `<whiteboard type="svg" path="@nope.svg"/>`}, "不存在"},
	}
	for _, tc := range cases {
		_, err := runContentUpdateFlags(t, "doxold", tc.flags)
		if err == nil || !strings.Contains(err.Error(), tc.want) || !clierr.HasKind(err, clierr.KindUsage) {
			t.Errorf("%v 期望用法错误含 %q，得到 %v", tc.flags, tc.want, err)
		}
		docContentUpdateCmd.Flags().VisitAll(func(fl *pflag.Flag) { _ = fl.Value.Set(fl.DefValue); fl.Changed = false })
	}
}

func TestContentUpdateDryRunNoNetwork(t *testing.T) {
	setupDocWriteFixtures(t)
	f := &fakeDocWriteServer{}
	server := httptest.NewServer(f.handler(t))
	t.Cleanup(server.Close)
	initDocUpdateTestConfig(t, server.URL)

	stdout, err := runContentUpdateFlags(t, "https://example.feishu.cn/wiki/wikcnXXX", map[string]string{
		"mode": "insert_after", "selection-by-title": "## 目标", "dry-run": "true",
		"markdown": "![图](@a.png)\n\n<html5-block path=\"@w.html\"/>\n\n<img href=\"https://img.example.com/r.png?sig=x\"/>",
	})
	if err != nil {
		t.Fatalf("dry-run 失败: %v", err)
	}
	if f.calls != 0 {
		t.Fatalf("dry-run 不应联网，实际请求 %d 次", f.calls)
	}
	for _, want := range []string{`"dry_run": true`, `/open-apis/wiki/v2/spaces/node_by_token`, `"block_id": "<定位到的锚点块>"`,
		`/blocks/batch_update`, `upload_all`, `"reference_map"`, `"mode": "insert_after"`,
		`"url": "https://img.example.com/r.png"`, `<remote_image_2_filename>`} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("dry-run 输出缺少 %s:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "sig=x") {
		t.Fatalf("dry-run 不应输出远程地址的 query:\n%s", stdout)
	}
}

// stubRemoteImages 替换远程图片的 DNS 校验与下载（不访问外网）；fail 非空时下载返回该错误。
func stubRemoteImages(t *testing.T, w, h int, fail error) *[]string {
	t.Helper()
	var got []string
	origCheck, origDownload := checkRemoteImageURL, downloadRemoteImage
	t.Cleanup(func() { checkRemoteImageURL, downloadRemoteImage = origCheck, origDownload })
	checkRemoteImageURL = func(raw string) error {
		if strings.Contains(raw, "127.0.0.1") {
			return fmt.Errorf("不允许指向本机或内网地址")
		}
		return nil
	}
	dir := t.TempDir()
	writeSizedPNG(t, filepath.Join(dir, "remote.png"), w, h)
	data, _ := os.ReadFile(filepath.Join(dir, "remote.png"))
	downloadRemoteImage = func(raw string) (*client.RemoteImage, error) {
		got = append(got, raw)
		if fail != nil {
			return nil, fail
		}
		return &client.RemoteImage{Content: data, FileName: "image.png", Width: w, Height: h}, nil
	}
	return &got
}

func TestDocCreateRemoteImageHref(t *testing.T) {
	setupDocWriteFixtures(t)
	downloads := stubRemoteImages(t, 2040, 600, nil)
	f := &fakeDocWriteServer{}
	server := httptest.NewServer(f.handler(t))
	t.Cleanup(server.Close)
	initDocUpdateTestConfig(t, server.URL)

	c := newDocCreateTestCmd(t, "--doc-format", "xml", "-o", "json",
		"--content", `<img href="https://img.example.com/w.png?sig=1" width="50%" alt="远程"/><img path="@a.png"/>`)
	var runErr error
	stdout := captureStdout(t, func() { runErr = runDocCreateDocsAI(c, "u-test") })
	if runErr != nil {
		t.Fatalf("创建失败: %v\n%s", runErr, stdout)
	}
	if len(*downloads) != 1 || (*downloads)[0] != "https://img.example.com/w.png?sig=1" {
		t.Fatalf("应在文档写入后下载完整 href: %v", *downloads)
	}
	content, _ := f.creates[0]["content"].(string)
	if strings.Contains(content, "href") || strings.Contains(content, "width=\"50%\"") || !strings.Contains(content, `caption="远程"`) {
		t.Fatalf("远程图片标签应改写为占位且去掉显示参数: %s", content)
	}
	if strings.Join(f.names, ",") != "image.png,a.png" {
		t.Fatalf("上传文件名异常: %v", f.names)
	}
	reqs, _ := json.Marshal(f.batches[0]["requests"])
	if !strings.Contains(string(reqs), `"replace_image":{"height":600,"scale":0.499999,"token":"tok_blk_image_0","width":2040}`) {
		t.Fatalf("远程图片应按真实像素归一化后绑定: %s", reqs)
	}
	if !strings.Contains(stdout, `"url": "https://img.example.com/w.png"`) || strings.Contains(stdout, "sig=1") || strings.Count(stdout, `"status": "bound"`) != 2 {
		t.Fatalf("输出应包含去掉 query 的 url 且两项 bound:\n%s", stdout)
	}
}

func TestDocCreateRemoteImageFailures(t *testing.T) {
	setupDocWriteFixtures(t)
	stubRemoteImages(t, 10, 10, &client.RemoteImageError{Msg: "下载远程图片失败: HTTP 404"})
	f := &fakeDocWriteServer{}
	server := httptest.NewServer(f.handler(t))
	t.Cleanup(server.Close)
	initDocUpdateTestConfig(t, server.URL)

	// 下载失败：清理占位块，退出码非零，失败明细带去掉 query 的地址
	c := newDocCreateTestCmd(t, "--doc-format", "xml", "--content", `<img href="https://img.example.com/x.png"/>`)
	var runErr error
	captureStdout(t, func() { runErr = runDocCreateDocsAI(c, "u-test") })
	if runErr == nil || len(f.uploads) != 0 || len(f.puts) != 1 || f.puts[0]["command"] != "block_delete" || f.puts[0]["block_id"] != "blk_image_0" {
		t.Fatalf("下载失败应清理占位块并非零退出: err=%v uploads=%v puts=%v", runErr, f.uploads, f.puts)
	}

	// 指向本机/内网：写入前以用法错误拒绝，不发任何请求
	calls := f.calls
	c = newDocCreateTestCmd(t, "--doc-format", "xml", "--content", `<img href="http://127.0.0.1/x.png"/>`)
	captureStdout(t, func() { runErr = runDocCreateDocsAI(c, "u-test") })
	if runErr == nil || !clierr.HasKind(runErr, clierr.KindUsage) || f.calls != calls {
		t.Fatalf("内网地址应在写入前以用法错误拒绝: %v（请求 %d→%d）", runErr, calls, f.calls)
	}
}
