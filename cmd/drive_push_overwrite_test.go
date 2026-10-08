package cmd

import (
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// resetDriveCmdFlags 在测试结束时把命令 flag 恢复为默认值（cobra 命令是包级全局，flag 状态会串到其他用例）。
func resetDriveCmdFlags(t *testing.T, c *cobra.Command) {
	t.Helper()
	t.Cleanup(func() {
		c.Flags().VisitAll(func(f *pflag.Flag) {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
	})
}

// pushMockServer 模拟 drive push 所需端点：列举（一个已存在文件 a.txt）、upload_all、DELETE。
type pushMockServer struct {
	mu             sync.Mutex
	uploadForms    []map[string]string
	deleteCalls    int
	uploadStatus   int
	uploadBody     string
	listFiles      []map[string]any
	prepareBodies  []map[string]any
	requestedPaths []string
}

func (m *pushMockServer) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if mockAuthHandler(w, r) {
			return
		}
		m.mu.Lock()
		m.requestedPaths = append(m.requestedPaths, r.Method+" "+r.URL.Path)
		m.mu.Unlock()
		switch {
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/open-apis/drive/v1/files/"):
			m.mu.Lock()
			m.deleteCalls++
			m.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"code":0,"msg":"success","data":{}}`)
		case r.URL.Path == "/open-apis/drive/v1/files/upload_all":
			mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
				t.Errorf("upload_all 应为 multipart，got %q", r.Header.Get("Content-Type"))
				return
			}
			form := map[string]string{}
			mr := multipart.NewReader(r.Body, params["boundary"])
			for {
				part, perr := mr.NextPart()
				if perr != nil {
					break
				}
				b, _ := io.ReadAll(part)
				form[part.FormName()] = string(b)
			}
			m.mu.Lock()
			m.uploadForms = append(m.uploadForms, form)
			m.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if m.uploadStatus != 0 {
				w.WriteHeader(m.uploadStatus)
			}
			_, _ = io.WriteString(w, m.uploadBody)
		case r.URL.Path == "/open-apis/drive/v1/files" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "msg": "success",
				"data": map[string]any{"has_more": false, "files": m.listFiles},
			})
		default:
			http.NotFound(w, r)
		}
	}
}

func runPushForTest(t *testing.T, srvURL, ifExists string, files map[string]string) (string, error) {
	t.Helper()
	cleanup := setupCmdTestConfig(t, srvURL)
	t.Cleanup(cleanup)

	cwd, _ := os.Getwd()
	tmpDir, err := os.MkdirTemp(cwd, "test_push_ow_")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	c := drivePushCmd
	resetDriveCmdFlags(t, c)
	_ = c.Flags().Set("folder-token", "fld_root")
	_ = c.Flags().Set("local-dir", tmpDir)
	_ = c.Flags().Set("user-access-token", "u-test-token")
	_ = c.Flags().Set("if-exists", ifExists)
	_ = c.Flags().Set("output", "json")

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	runErr := c.RunE(c, nil)
	w.Close()
	os.Stdout = oldStdout
	out, _ := io.ReadAll(r)
	return string(out), runErr
}

// P0：--if-exists overwrite 必须原地覆盖（upload_all 带 file_token），绝不先删后传。
func TestDrivePush_OverwriteInPlaceKeepsFileToken(t *testing.T) {
	m := &pushMockServer{
		listFiles:  []map[string]any{{"token": "boxcn_old", "name": "a.txt", "type": "file", "modified_time": "1700000000"}},
		uploadBody: `{"code":0,"msg":"success","data":{"file_token":"boxcn_old","version":"7"}}`,
	}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()

	out, err := runPushForTest(t, srv.URL, "overwrite", map[string]string{"a.txt": "new content"})
	if err != nil {
		t.Fatalf("覆盖应成功: %v\n%s", err, out)
	}
	if m.deleteCalls != 0 {
		t.Fatalf("覆盖不得调用 DELETE（先删后传会断开链接/协作者/评论），deleteCalls=%d", m.deleteCalls)
	}
	if len(m.uploadForms) != 1 {
		t.Fatalf("upload_all 调用次数 = %d", len(m.uploadForms))
	}
	form := m.uploadForms[0]
	if form["file_token"] != "boxcn_old" || form["parent_node"] != "fld_root" || form["file_name"] != "a.txt" || form["file"] != "new content" {
		t.Fatalf("upload_all 表单 = %+v", form)
	}
	var payload struct {
		Summary map[string]any `json:"summary"`
		Items   []struct {
			Action    string `json:"action"`
			FileToken string `json:"file_token"`
			Version   string `json:"version"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("解析输出失败: %v\n%s", err, out)
	}
	if len(payload.Items) != 1 || payload.Items[0].Action != "overwritten" || payload.Items[0].FileToken != "boxcn_old" || payload.Items[0].Version != "7" {
		t.Fatalf("items = %+v", payload.Items)
	}
}

// 覆盖失败时直接报错，不回退为删除；远端文件保持不动。
func TestDrivePush_OverwriteFailureNeverDeletes(t *testing.T) {
	m := &pushMockServer{
		listFiles:    []map[string]any{{"token": "boxcn_old", "name": "a.txt", "type": "file"}},
		uploadStatus: http.StatusBadRequest,
		uploadBody:   `{"code":1061002,"msg":"params error"}`,
	}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()

	out, err := runPushForTest(t, srv.URL, "overwrite", map[string]string{"a.txt": "new content"})
	if err == nil {
		t.Fatalf("覆盖失败应返回错误\n%s", out)
	}
	if m.deleteCalls != 0 {
		t.Fatalf("覆盖失败绝不能删除远端文件，deleteCalls=%d", m.deleteCalls)
	}
	if !strings.Contains(out, `"error_class": "invalid_parameters"`) || !strings.Contains(out, `"aborted": true`) {
		t.Fatalf("参数错误应分级为 invalid_parameters 并终止整批:\n%s", out)
	}
}

// 服务端未返回 version（租户未灰度覆盖字段）时不得虚报成功。
func TestDrivePush_OverwriteWithoutVersionFails(t *testing.T) {
	m := &pushMockServer{
		listFiles:  []map[string]any{{"token": "boxcn_old", "name": "a.txt", "type": "file"}},
		uploadBody: `{"code":0,"msg":"success","data":{"file_token":"boxcn_old"}}`,
	}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()

	out, err := runPushForTest(t, srv.URL, "overwrite", map[string]string{"a.txt": "x"})
	if err == nil || !strings.Contains(out, "version") {
		t.Fatalf("缺 version 应失败, err=%v\n%s", err, out)
	}
	if m.deleteCalls != 0 {
		t.Fatalf("deleteCalls=%d", m.deleteCalls)
	}
}

// smart：远端 modified_time 不早于本地 mtime 时跳过，否则原地覆盖。
func TestDrivePush_SmartSkipsWhenRemoteNewer(t *testing.T) {
	m := &pushMockServer{
		// 远端时间远在未来 → 本地不比远端新 → 跳过
		listFiles:  []map[string]any{{"token": "boxcn_old", "name": "a.txt", "type": "file", "modified_time": "4102444800000"}},
		uploadBody: `{"code":0,"msg":"success","data":{"file_token":"boxcn_old","version":"8"}}`,
	}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()

	out, err := runPushForTest(t, srv.URL, "smart", map[string]string{"a.txt": "x"})
	if err != nil {
		t.Fatalf("smart 跳过不应报错: %v\n%s", err, out)
	}
	if len(m.uploadForms) != 0 || !strings.Contains(out, `"action": "skipped"`) {
		t.Fatalf("远端更新时应跳过, uploads=%d\n%s", len(m.uploadForms), out)
	}

	m2 := &pushMockServer{
		listFiles:  []map[string]any{{"token": "boxcn_old", "name": "a.txt", "type": "file", "modified_time": "1000000000"}},
		uploadBody: `{"code":0,"msg":"success","data":{"file_token":"boxcn_old","version":"8"}}`,
	}
	srv2 := httptest.NewServer(m2.handler(t))
	defer srv2.Close()
	out, err = runPushForTest(t, srv2.URL, "smart", map[string]string{"a.txt": "x"})
	if err != nil || len(m2.uploadForms) != 1 || m2.uploadForms[0]["file_token"] != "boxcn_old" {
		t.Fatalf("远端更旧时应原地覆盖: err=%v forms=%+v\n%s", err, m2.uploadForms, out)
	}
}

// 新文件上传仍走普通上传（不带 file_token），限流类错误终止整批、剩余文件不再尝试。
func TestDrivePush_RateLimitAbortsBatch(t *testing.T) {
	m := &pushMockServer{
		uploadStatus: http.StatusBadRequest,
		uploadBody:   `{"code":99991400,"msg":"request trigger frequency limit"}`,
	}
	srv := httptest.NewServer(m.handler(t))
	defer srv.Close()

	out, err := runPushForTest(t, srv.URL, "skip", map[string]string{"a.txt": "1", "b.txt": "2", "c.txt": "3"})
	if err == nil {
		t.Fatalf("限流应终止并返回错误\n%s", out)
	}
	if !strings.Contains(out, `"abort_reason": "rate_limited"`) || !strings.Contains(out, `"aborted": true`) {
		t.Fatalf("应标记 rate_limited 终止:\n%s", out)
	}
	if strings.Contains(out, `"rel_path": "c.txt"`) {
		t.Fatalf("终止后不应继续尝试剩余文件:\n%s", out)
	}
}

func TestClassifyDriveBatchFailure(t *testing.T) {
	cases := []struct {
		err      string
		push     bool
		class    string
		terminal bool
	}{
		{"上传文件失败: code=99991672, msg=scope", true, "app_scope_missing", true},
		{"上传文件失败: code=99991679, msg=user scope", false, "user_scope_missing", true},
		{"code=1061004, msg=forbidden", false, "permission_denied", true},
		{"code=99991400, msg=frequency limit", true, "rate_limited", true},
		{"code=1061002, msg=params error", true, "invalid_parameters", true},
		{"code=1062507, msg=children over limit", true, "parent_sibling_limit", false},
		{"code=1061045, msg=conflict", true, "conflict", true},
		{"code=1061045, msg=conflict", false, "conflict", false},
		{"code=1061007, msg=deleted", false, "remote_not_found", false},
		{"code=1061001, msg=unknown", true, "server_error", true},
	}
	for _, c := range cases {
		d := classifyDriveBatchFailure(errorString(c.err), c.push)
		if d.Class != c.class || d.Terminal != c.terminal {
			t.Errorf("%q push=%v → %+v, want class=%s terminal=%v", c.err, c.push, d, c.class, c.terminal)
		}
	}
}

type errorString string

func (e errorString) Error() string { return string(e) }
