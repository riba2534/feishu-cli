package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

func writeUploadFixture(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// --file 的本地校验全部在联网前完成，且均为用法错误（退出码 2）。
func TestRunAPI_FileUsageErrors(t *testing.T) {
	isolateAPITestEnv(t)
	defer resetAPIFlags()
	file := writeUploadFixture(t, "a.txt", "hello")
	var hits int32
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(500)
	})
	defer cleanup()

	cases := []struct {
		name   string
		method string
		setup  func()
		want   string
	}{
		{"GET 不支持", "GET", func() { apiFile = file }, "只能用于 POST"},
		{"与 data-file 互斥", "POST", func() { apiFile = file; apiDataFile = file }, "--data-file 不能同时使用"},
		{"与 page-all 互斥", "POST", func() { apiFile = file; apiPageAll = true }, "--page-all 不能同时使用"},
		{"空路径", "POST", func() { apiFile = "file=" }, "缺少文件路径"},
		{"不存在", "POST", func() { apiFile = "file=" + file + ".nope" }, "不存在"},
		{"目录", "POST", func() { apiFile = filepath.Dir(file) }, "是目录"},
		{"敏感目录", "POST", func() { apiFile = filepath.Join(os.Getenv("HOME"), ".ssh", "id_rsa") }, ".ssh"},
		{"data 非对象", "POST", func() { apiFile = file; apiData = `[1]` }, "必须是 JSON 对象"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetAPIFlags()
			cmd := newTestAPICmd()
			tc.setup()
			err := cmd.RunE(cmd, []string{tc.method, "/open-apis/im/v1/files"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v, want contains %q", err, tc.want)
			}
			if code := exitCodeFor(err); code != clierr.ExitUsage {
				t.Fatalf("exit=%d, want 2 (%v)", code, err)
			}
		})
	}
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Fatalf("用法错误不应联网，实际请求 %d 次", n)
	}
}

// dry-run 只展示文件名/大小与表单字段，不输出文件内容，也不需要凭据。
func TestRunAPI_FileDryRun(t *testing.T) {
	isolateAPITestEnv(t)
	defer resetAPIFlags()
	file := writeUploadFixture(t, "report.txt", "SECRET-CONTENT")
	cmd := newTestAPICmd()
	apiFile = "doc=@" + file
	apiData = `{"file_type":"stream","file_name":"report.txt","size":14}`
	apiDryRun = true
	apiAs = "bot"
	var runErr error
	out := captureStdout(t, func() { runErr = cmd.RunE(cmd, []string{"POST", "/open-apis/im/v1/files"}) })
	if runErr != nil {
		t.Fatalf("dry-run: %v", runErr)
	}
	if strings.Contains(out, "SECRET-CONTENT") {
		t.Fatal("dry-run 不得输出文件内容")
	}
	var plan map[string]any
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("非 JSON: %v\n%s", err, out)
	}
	f := plan["file"].(map[string]any)
	if f["field"] != "doc" || f["file_name"] != "report.txt" || f["size"] != float64(14) || f["path"] != file {
		t.Fatalf("file 预览不符（@ 前缀应被剥掉）: %v", f)
	}
	// dry-run 不读文件内容，但路径（不存在 / 敏感目录）仍在预览前校验，退出码 2
	for _, bad := range []string{file + ".nope", filepath.Join(os.Getenv("HOME"), ".ssh", "id_rsa")} {
		resetAPIFlags()
		c2 := newTestAPICmd()
		apiFile, apiDryRun, apiAs = bad, true, "bot"
		var e2 error
		captureStdout(t, func() { e2 = c2.RunE(c2, []string{"POST", "/open-apis/im/v1/files"}) })
		if e2 == nil || exitCodeFor(e2) != clierr.ExitUsage {
			t.Fatalf("dry-run 非法路径 %q 应 exit 2: %v", bad, e2)
		}
	}
	fields := plan["form_fields"].(map[string]any)
	if fields["file_type"] != "stream" || fields["size"] != "14" || plan["content_type"] != "multipart/form-data" {
		t.Fatalf("表单字段不符: %v", plan)
	}
}

// 真实发送：multipart 带表单字段与真实文件名，Bearer 为 tenant token；业务错误照常非零。
func TestRunAPI_FileUploadMultipart(t *testing.T) {
	isolateAPITestEnv(t)
	defer resetAPIFlags()
	file := writeUploadFixture(t, "note.txt", "hello multipart")
	var got struct {
		auth, ct, field, fileName, content, fileType, query string
	}
	respBody := `{"code":0,"msg":"ok","data":{"file_key":"file_v2_xxx"}}`
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-upload","expire":7200}`)
			return
		}
		got.auth = r.Header.Get("Authorization")
		got.ct = r.Header.Get("Content-Type")
		got.query = r.URL.RawQuery
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("multipart 解析失败: %v", err)
			return
		}
		got.fileType = r.FormValue("file_type")
		for field, fhs := range r.MultipartForm.File {
			got.field, got.fileName = field, fhs[0].Filename
			fh, _ := fhs[0].Open()
			b, _ := io.ReadAll(fh)
			got.content = string(b)
		}
		_, _ = fmt.Fprint(w, respBody)
	})
	defer cleanup()

	cmd := newTestAPICmd()
	apiFile = file
	apiData = `{"file_type":"stream"}`
	apiParams = `{"x":"1"}`
	apiAs = "bot"
	var runErr error
	out := captureStdout(t, func() { runErr = cmd.RunE(cmd, []string{"POST", "/open-apis/im/v1/files"}) })
	if runErr != nil {
		t.Fatalf("upload: %v", runErr)
	}
	if got.auth != "Bearer t-upload" || !strings.HasPrefix(got.ct, "multipart/form-data; boundary=") {
		t.Fatalf("请求头不符: %+v", got)
	}
	if got.field != "file" || got.fileName != "note.txt" || got.content != "hello multipart" || got.fileType != "stream" || got.query != "x=1" {
		t.Fatalf("multipart 内容不符: %+v", got)
	}
	if !strings.Contains(out, "file_v2_xxx") {
		t.Fatalf("应输出响应: %s", out)
	}

	// 业务错误（随 HTTP 400 下发）返回非零，并保留 code
	respBody = `{"code":234001,"msg":"invalid param"}`
	resetAPIFlags()
	cmd = newTestAPICmd()
	apiFile, apiAs = file, "bot"
	captureStdout(t, func() { runErr = cmd.RunE(cmd, []string{"POST", "/open-apis/im/v1/files"}) })
	if runErr == nil || !strings.Contains(runErr.Error(), "code=234001") {
		t.Fatalf("业务错误应返回非零: %v", runErr)
	}
}

// 不带 --file 时 dry-run 输出结构与旧版一致（不出现 multipart 字段）。
func TestRunAPI_DryRunWithoutFileUnchanged(t *testing.T) {
	isolateAPITestEnv(t)
	defer resetAPIFlags()
	cmd := newTestAPICmd()
	apiDryRun, apiAs, apiData = true, "bot", `{"a":1}`
	var runErr error
	out := captureStdout(t, func() { runErr = cmd.RunE(cmd, []string{"POST", "/open-apis/x"}) })
	if runErr != nil {
		t.Fatal(runErr)
	}
	var plan map[string]any
	_ = json.Unmarshal([]byte(out), &plan)
	keys := make([]string, 0, len(plan))
	for k := range plan {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := "body,dry_run,method,page_all,page_limit,path,query,supported_tokens,will_use_user_tok"
	if got := strings.Join(keys, ","); got != want {
		t.Fatalf("dry-run 字段变化: %s", got)
	}
}
