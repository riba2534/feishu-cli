package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// mirrorMock 模拟一个文件夹的列举与文件下载。
type mirrorMock struct {
	mu        sync.Mutex
	files     []map[string]any
	contents  map[string]string // token → 内容
	statusFor map[string]int    // token → 下载时返回的 HTTP 错误
	downloads map[string]int
}

func (m *mirrorMock) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if mockAuthHandler(w, r) {
			return
		}
		if strings.HasSuffix(r.URL.Path, "/download") {
			token := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/open-apis/drive/v1/files/"), "/download")
			m.mu.Lock()
			if m.downloads == nil {
				m.downloads = map[string]int{}
			}
			m.downloads[token]++
			st := m.statusFor[token]
			m.mu.Unlock()
			if st != 0 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(st)
				code := 1061004
				if st == http.StatusBadRequest {
					code = 99991679
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "msg": "denied"})
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, m.contents[token])
			return
		}
		if r.URL.Path == "/open-apis/drive/v1/files" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"has_more": false, "files": m.files}})
			return
		}
		http.NotFound(w, r)
	}
}

func runMirrorCmd(t *testing.T, srvURL string, which string, setup func(dir string), flags map[string]string) (string, string, error) {
	t.Helper()
	cleanup := setupCmdTestConfig(t, srvURL)
	t.Cleanup(cleanup)
	cwd, _ := os.Getwd()
	dir, err := os.MkdirTemp(cwd, "test_mirror_")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if setup != nil {
		setup(dir)
	}
	c := drivePullCmd
	if which == "status" {
		c = driveStatusCmd
	}
	resetDriveCmdFlags(t, c)
	_ = c.Flags().Set("folder-token", "fld_root")
	_ = c.Flags().Set("local-dir", dir)
	_ = c.Flags().Set("user-access-token", "u-test-token")
	_ = c.Flags().Set("output", "json")
	for k, v := range flags {
		if err := c.Flags().Set(k, v); err != nil {
			t.Fatalf("set %s: %v", k, err)
		}
	}
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	runErr := c.RunE(c, nil)
	w.Close()
	os.Stdout = oldStdout
	out, _ := io.ReadAll(r)
	return string(out), dir, runErr
}

// 文件名含 ".." 子串（report..v2.pdf）是合法文件名，必须能下载；只有整段为 ".." 才拒绝。
func TestDrivePull_DotDotInFileNameAllowedButTraversalRejected(t *testing.T) {
	m := &mirrorMock{
		files: []map[string]any{
			{"token": "boxcn_ok", "name": "report..v2.pdf", "type": "file", "modified_time": "1700000000"},
			{"token": "boxcn_evil", "name": "..", "type": "file"},
		},
		contents: map[string]string{"boxcn_ok": "pdf-bytes", "boxcn_evil": "evil"},
	}
	srv := httptest.NewServer(m.handler())
	defer srv.Close()

	out, dir, err := runMirrorCmd(t, srv.URL, "pull", nil, nil)
	if err == nil {
		t.Fatalf("含 '..' 段的远端名应失败\n%s", out)
	}
	got, readErr := os.ReadFile(filepath.Join(dir, "report..v2.pdf"))
	if readErr != nil || string(got) != "pdf-bytes" {
		t.Fatalf("report..v2.pdf 应下载成功: %v %q\n%s", readErr, got, out)
	}
	if m.downloads["boxcn_evil"] != 0 {
		t.Fatal("越界路径不应发起下载")
	}
	if !strings.Contains(out, `"error_class": "unsafe_path"`) {
		t.Fatalf("应标记 unsafe_path:\n%s", out)
	}
	// 下载后本地 mtime 对齐远端 modified_time
	info, _ := os.Stat(filepath.Join(dir, "report..v2.pdf"))
	if !info.ModTime().Equal(time.Unix(1700000000, 0)) {
		t.Fatalf("mtime = %v, want 1700000000", info.ModTime())
	}
}

func TestSafeMirrorTarget(t *testing.T) {
	root := t.TempDir()
	for _, ok := range []string{"a.txt", "report..v2.pdf", "sub/a..b/c.txt", "...hidden"} {
		if _, err := safeMirrorTarget(root, ok); err != nil {
			t.Errorf("%q 应合法: %v", ok, err)
		}
	}
	for _, bad := range []string{"..", "../x", "a/../../x", "a//b", "./a", "a\\..\\b", ""} {
		if _, err := safeMirrorTarget(root, bad); err == nil {
			t.Errorf("%q 应被拒绝", bad)
		}
	}
	// 本地符号链接目录指向 root 之外
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err == nil {
		if _, err := safeMirrorTarget(root, "link/x.txt"); err == nil {
			t.Error("经符号链接逃逸应被拒绝")
		}
	}
}

// smart：本地 mtime 不早于远端 modified_time 时跳过下载。
func TestDrivePull_SmartSkipsUpToDate(t *testing.T) {
	m := &mirrorMock{
		files:    []map[string]any{{"token": "boxcn_a", "name": "a.txt", "type": "file", "modified_time": "1600000000"}},
		contents: map[string]string{"boxcn_a": "remote"},
	}
	srv := httptest.NewServer(m.handler())
	defer srv.Close()

	out, dir, err := runMirrorCmd(t, srv.URL, "pull", func(dir string) {
		_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("local-newer"), 0o644)
	}, map[string]string{"if-exists": "smart"})
	if err != nil {
		t.Fatalf("smart 跳过不应报错: %v\n%s", err, out)
	}
	if m.downloads["boxcn_a"] != 0 {
		t.Fatalf("本地更新时不应下载")
	}
	got, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(got) != "local-newer" {
		t.Fatalf("本地文件被改动: %q", got)
	}
}

// 远端纯文件重名：默认 fail；rename 时副本以哈希后缀另存；newest 选最新。
func TestDrivePull_OnDuplicateRemote(t *testing.T) {
	files := []map[string]any{
		{"token": "boxcn_old", "name": "d.txt", "type": "file", "created_time": "100", "modified_time": "100"},
		{"token": "boxcn_new", "name": "d.txt", "type": "file", "created_time": "200", "modified_time": "300"},
	}
	contents := map[string]string{"boxcn_old": "old", "boxcn_new": "new"}

	srv := httptest.NewServer((&mirrorMock{files: files, contents: contents}).handler())
	defer srv.Close()
	if _, _, err := runMirrorCmd(t, srv.URL, "pull", nil, nil); err == nil || !strings.Contains(err.Error(), "重复相对路径") {
		t.Fatalf("默认应 fail-closed: %v", err)
	}

	out, dir, err := runMirrorCmd(t, srv.URL, "pull", nil, map[string]string{"on-duplicate-remote": "rename"})
	if err != nil {
		t.Fatalf("rename 应成功: %v\n%s", err, out)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "d.txt")); string(got) != "old" {
		t.Fatalf("rename 下最旧文件保留原名，got %q", got)
	}
	entries, _ := os.ReadDir(dir)
	foundSuffix := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "d__lark_") && strings.HasSuffix(e.Name(), ".txt") {
			foundSuffix = true
		}
	}
	if !foundSuffix {
		t.Fatalf("副本应以 __lark_<hash> 后缀另存: %v", entries)
	}

	_, dir, err = runMirrorCmd(t, srv.URL, "pull", nil, map[string]string{"on-duplicate-remote": "newest"})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "d.txt")); string(got) != "new" {
		t.Fatalf("newest 应取 modified_time 最大者，got %q", got)
	}
}

// 缺 scope（终止类错误）时整批终止，--delete-local 被跳过。
func TestDrivePull_TerminalFailureAborts(t *testing.T) {
	m := &mirrorMock{
		files: []map[string]any{
			{"token": "boxcn_a", "name": "a.txt", "type": "file"},
		},
		statusFor: map[string]int{"boxcn_a": http.StatusBadRequest},
	}
	srv := httptest.NewServer(m.handler())
	defer srv.Close()
	out, dir, err := runMirrorCmd(t, srv.URL, "pull", func(dir string) {
		_ = os.WriteFile(filepath.Join(dir, "orphan.txt"), []byte("keep"), 0o644)
	}, map[string]string{"delete-local": "true", "yes": "true"})
	if err == nil || !strings.Contains(err.Error(), "user_scope_missing") {
		t.Fatalf("缺 scope 应终止: %v\n%s", err, out)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "orphan.txt")); statErr != nil {
		t.Fatal("终止时不得执行 --delete-local")
	}
}

// status 只对两边都存在的文件计算哈希；--quick 不下载远端内容。
func TestDriveStatus_HashesOnlyBothSidesAndQuick(t *testing.T) {
	m := &mirrorMock{
		files: []map[string]any{
			{"token": "boxcn_both", "name": "both.txt", "type": "file", "modified_time": "1700000000"},
			{"token": "boxcn_remote", "name": "remote_only.txt", "type": "file"},
		},
		contents: map[string]string{"boxcn_both": "same", "boxcn_remote": "r"},
	}
	srv := httptest.NewServer(m.handler())
	defer srv.Close()

	setup := func(dir string) {
		p := filepath.Join(dir, "both.txt")
		_ = os.WriteFile(p, []byte("same"), 0o644)
		_ = os.Chtimes(p, time.Unix(1700000000, 0), time.Unix(1700000000, 0))
		// 仅本地存在的文件不可读：旧实现先给所有本地文件算哈希，会因此失败
		lp := filepath.Join(dir, "local_only.txt")
		_ = os.WriteFile(lp, []byte("x"), 0o000)
	}
	out, _, err := runMirrorCmd(t, srv.URL, "status", setup, nil)
	if err != nil {
		t.Fatalf("status 失败: %v\n%s", err, out)
	}
	var res struct {
		Detection string `json:"detection"`
		Unchanged []struct {
			RelPath string `json:"rel_path"`
		} `json:"unchanged"`
		NewLocal []struct {
			RelPath string `json:"rel_path"`
		} `json:"new_local"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if res.Detection != "exact" || len(res.Unchanged) != 1 || len(res.NewLocal) != 1 {
		t.Fatalf("res = %+v", res)
	}
	if m.downloads["boxcn_remote"] != 0 || m.downloads["boxcn_both"] != 1 {
		t.Fatalf("只应下载两边都有的文件: %v", m.downloads)
	}

	m.downloads = nil
	out, _, err = runMirrorCmd(t, srv.URL, "status", setup, map[string]string{"quick": "true"})
	if err != nil {
		t.Fatalf("quick 失败: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"detection": "quick"`) || len(m.downloads) != 0 {
		t.Fatalf("--quick 不应下载远端内容: downloads=%v\n%s", m.downloads, out)
	}
	if !strings.Contains(out, `"rel_path": "both.txt"`) {
		t.Fatalf("quick 结果缺 both.txt:\n%s", out)
	}
}
