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

	"github.com/riba2534/feishu-cli/v2/internal/clierr"
)

func writeTestPNG(t *testing.T, path string) {
	t.Helper()
	// 1x1 PNG
	data := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\rIDATx\x9cc\xf8\x0f\x00\x00\x01\x01\x00\x05\x18\xd8N\x00\x00\x00\x00IEND\xaeB`\x82")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareLocalDocResources(t *testing.T) {
	dir := t.TempDir()
	writeTestPNG(t, filepath.Join(dir, "a.png"))
	if err := os.WriteFile(filepath.Join(dir, "r.pdf"), []byte("%PDF-1.4"), 0o600); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	in := strings.Join([]string{
		"![相对](a.png) 与 ![官方](@./a.png) 与 ![尖括号](<@./a.png>)",
		"![远程](https://example.com/x.png)",
		`<source path="@./r.pdf" name="报告.pdf"/>`,
		"```",
		"![代码块内](a.png)",
		"```",
		"`![行内代码](a.png)`",
	}, "\n")
	out, res, err := prepareLocalDocResources(in, "markdown", dir)
	if err != nil {
		t.Fatalf("prepare 失败: %v", err)
	}
	if len(res) != 4 {
		t.Fatalf("资源数 = %d，期望 4（3 图 + 1 附件）", len(res))
	}
	if res[0].Kind != "image" || res[0].Width != 1 || res[0].Height != 1 {
		t.Fatalf("图片尺寸未解析: %+v", res[0])
	}
	if res[3].Kind != "file" || res[3].FileName != "报告.pdf" {
		t.Fatalf("附件解析异常: %+v", res[3])
	}
	marker := regexp.MustCompile(`<img path="@lcli_img_[0-9a-f]{32}" caption="相对"/>`)
	if !marker.MatchString(out) || !strings.Contains(out, "![远程](https://example.com/x.png)") ||
		!strings.Contains(out, "![代码块内](a.png)") || !strings.Contains(out, "`![行内代码](a.png)`") ||
		!regexp.MustCompile(`<source path="@lcli_file_[0-9a-f]{32}" name="报告.pdf"/>`).MatchString(out) {
		t.Fatalf("改写结果异常:\n%s", out)
	}

	if _, _, err := prepareLocalDocResources("![缺失](missing.png)", "markdown", dir); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("缺失文件应为用法错误: %v", err)
	}
	if _, _, err := prepareLocalDocResources(`<img path="@lcli_img_00"/>`, "xml", dir); err == nil {
		t.Fatal("手写保留占位标记应报错")
	}
}

type fakeResourceServer struct {
	mu          sync.Mutex
	puts        []map[string]any
	uploads     []string // parent_node
	batch       []map[string]any
	failFile    bool
	markerRe    *regexp.Regexp
	batchTokens []string
}

func (f *fakeResourceServer) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/open-apis/docs_ai/v1/documents/"):
			var body map[string]any
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &body)
			f.puts = append(f.puts, body)
			var newBlocks []map[string]any
			content, _ := body["content"].(string)
			for i, m := range f.markerRe.FindAllStringSubmatch(content, -1) {
				typ := "image"
				if m[1] == "file" {
					typ = "file"
				}
				newBlocks = append(newBlocks, map[string]any{"block_id": fmt.Sprintf("blk_%s_%d", typ, i), "block_token": m[0], "block_type": typ})
			}
			resp := map[string]any{"code": 0, "data": map[string]any{"document": map[string]any{"revision_id": 5, "new_blocks": newBlocks}, "result": "success"}}
			_ = json.NewEncoder(w).Encode(resp)
		case r.URL.Path == "/open-apis/drive/v1/medias/upload_all":
			_ = r.ParseMultipartForm(1 << 20)
			parent := r.FormValue("parent_node")
			f.uploads = append(f.uploads, parent+"|"+r.FormValue("parent_type")+"|"+r.FormValue("extra"))
			if f.failFile && r.FormValue("parent_type") == "docx_file" {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"code":1061002,"msg":"params error"}`)
				return
			}
			fmt.Fprintf(w, `{"code":0,"msg":"ok","data":{"file_token":"tok_%s"}}`, parent)
		case strings.HasSuffix(r.URL.Path, "/blocks/batch_update"):
			var body map[string]any
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &body)
			f.batch = append(f.batch, body)
			f.batchTokens = append(f.batchTokens, r.URL.Query().Get("client_token"))
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"blocks":[],"document_revision_id":6}}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/blocks/blk_file_1"):
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"block":{"block_id":"blk_file_1","block_type":23,"parent_id":"view_1"}}}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/blocks/view_1"):
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"block":{"block_id":"view_1","block_type":33,"parent_id":"doc","children":["blk_file_1"]}}}`)
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}
}

func runLocalResourceUpdate(t *testing.T, f *fakeResourceServer, output string) (string, string, error) {
	t.Helper()
	server := httptest.NewServer(f.handler(t))
	t.Cleanup(server.Close)
	initDocUpdateTestConfig(t, server.URL)
	orig := waitBetweenLocalUploads
	waitBetweenLocalUploads = 0
	t.Cleanup(func() { waitBetweenLocalUploads = orig })

	dir := t.TempDir()
	writeTestPNG(t, filepath.Join(dir, "a.png"))
	if err := os.WriteFile(filepath.Join(dir, "r.txt"), []byte("附件内容"), 0o600); err != nil {
		t.Fatal(err)
	}
	content, res, err := prepareLocalDocResources("段落\n\n![图](a.png)\n\n<source path=\"@"+filepath.Join(dir, "r.txt")+"\" name=\"r.txt\"/>", "markdown", dir)
	if err != nil {
		t.Fatalf("prepare 失败: %v", err)
	}
	p := &contentUpdateParams{mode: "append", content: content, resources: res, output: output}
	return runParams(p)
}

// TestContentUpdateLocalResourcesUploadAndBind 占位块 → 上传（parent_node=占位块）→ batch_update 绑定。
func TestContentUpdateLocalResourcesUploadAndBind(t *testing.T) {
	f := &fakeResourceServer{markerRe: regexp.MustCompile(`@lcli_(img|file)_[0-9a-f]{32}`)}
	stdout, _, err := runLocalResourceUpdate(t, f, "json")
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if len(f.uploads) != 2 || !strings.HasPrefix(f.uploads[0], "blk_image_0|docx_image|") ||
		!strings.HasPrefix(f.uploads[1], "blk_file_1|docx_file|") || !strings.Contains(f.uploads[0], `"drive_route_token":"doc"`) {
		t.Fatalf("上传请求异常: %v", f.uploads)
	}
	if len(f.batch) != 1 || f.batchTokens[0] == "" {
		t.Fatalf("应一次 batch_update 绑定并带 client_token: %v %v", f.batch, f.batchTokens)
	}
	reqs, _ := f.batch[0]["requests"].([]any)
	b, _ := json.Marshal(reqs)
	if !strings.Contains(string(b), `"replace_image":{"height":1,"token":"tok_blk_image_0","width":1}`) ||
		!strings.Contains(string(b), `"replace_file":{"token":"tok_blk_file_1"}`) {
		t.Fatalf("绑定请求异常: %s", b)
	}
	if !strings.Contains(stdout, `"local_resources"`) || strings.Count(stdout, `"status": "bound"`) != 2 {
		t.Fatalf("JSON 输出应包含逐项 bound 明细: %s", stdout)
	}
	if len(f.puts) != 1 {
		t.Fatalf("成功时不应有清理请求: %d", len(f.puts))
	}
}

// TestContentUpdateLocalResourcesFailureCleansPlaceholder 附件上传失败：删除外层视图块并非零退出。
func TestContentUpdateLocalResourcesFailureCleansPlaceholder(t *testing.T) {
	f := &fakeResourceServer{markerRe: regexp.MustCompile(`@lcli_(img|file)_[0-9a-f]{32}`), failFile: true}
	_, stderr, err := runLocalResourceUpdate(t, f, "")
	if err == nil || !strings.Contains(err.Error(), "未能写入") {
		t.Fatalf("部分失败应非零退出: %v", err)
	}
	if len(f.puts) != 2 || f.puts[1]["command"] != "block_delete" || f.puts[1]["block_id"] != "view_1" {
		t.Fatalf("应删除失败附件的外层视图块: %#v", f.puts)
	}
	if !strings.Contains(stderr, "上传失败") || !strings.Contains(stderr, "deleted") {
		t.Fatalf("stderr 应给出失败明细与清理结果: %q", stderr)
	}
}

func TestContentUpdateLocalResourcesRejectedForTextModes(t *testing.T) {
	dir := t.TempDir()
	writeTestPNG(t, filepath.Join(dir, "a.png"))
	md := filepath.Join(dir, "x.md")
	if err := os.WriteFile(md, []byte("![图](a.png)"), 0o600); err != nil {
		t.Fatal(err)
	}
	initDocUpdateTestConfig(t, "http://127.0.0.1:59997")
	_ = docContentUpdateCmd.Flags().Set("mode", "replace_all")
	_ = docContentUpdateCmd.Flags().Set("markdown-file", md)
	_ = docContentUpdateCmd.Flags().Set("selection-with-ellipsis", "旧")
	defer func() {
		for _, k := range []string{"mode", "markdown-file", "selection-with-ellipsis"} {
			fl := docContentUpdateCmd.Flags().Lookup(k)
			_ = fl.Value.Set(fl.DefValue)
			fl.Changed = false
		}
	}()
	err := docContentUpdateCmd.RunE(docContentUpdateCmd, []string{"doc"})
	if err == nil || !strings.Contains(err.Error(), "文本级替换无法插入图片") {
		t.Fatalf("文本级模式携带本地图片应报用法错误: %v", err)
	}
}
