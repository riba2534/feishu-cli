package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type driveDownloadMock struct {
	mu        sync.Mutex
	queryBody string // query_by_token 响应
	downloads []string
	auths     []string
}

func (m *driveDownloadMock) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if mockAuthHandler(w, r) {
			return
		}
		switch {
		case r.URL.Path == "/open-apis/drive/v2/files/query_by_token":
			w.Header().Set("Content-Type", "application/json")
			if m.queryBody == "" {
				w.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(w, `{"code":981004,"msg":"forbidden"}`)
				return
			}
			_, _ = io.WriteString(w, m.queryBody)
		case strings.HasSuffix(r.URL.Path, "/download"):
			tok := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/open-apis/drive/v1/files/"), "/download")
			m.mu.Lock()
			m.downloads = append(m.downloads, tok)
			m.auths = append(m.auths, r.Header.Get("Authorization"))
			m.mu.Unlock()
			w.Header().Set("Content-Disposition", `attachment; filename="report..v2.pdf"`)
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "content-of-"+tok)
		default:
			http.NotFound(w, r)
		}
	}
}

func runDriveDownloadForTest(t *testing.T, srvURL string, flags map[string]string) (string, string, string, error) {
	t.Helper()
	cleanup := setupCmdTestConfig(t, srvURL)
	t.Cleanup(cleanup)
	dir := t.TempDir()
	c := driveDownloadCmd
	resetDriveCmdFlags(t, c)
	_ = c.Flags().Set("user-access-token", "u-test-token")
	_ = c.Flags().Set("output-format", "json")
	_ = c.Flags().Set("output", dir)
	for k, v := range flags {
		if err := c.Flags().Set(k, v); err != nil {
			t.Fatalf("set %s: %v", k, err)
		}
	}
	var errBuf strings.Builder
	c.SetErr(&errBuf)
	defer c.SetErr(nil)
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := c.RunE(c, nil)
	w.Close()
	os.Stdout = oldStdout
	out, _ := io.ReadAll(r)
	return string(out), errBuf.String(), dir, err
}

// P1-13：在线文档 URL 离线即提示改用 export，不发请求。
func TestDriveDownload_OnlineDocURLSuggestsExport(t *testing.T) {
	m := &driveDownloadMock{}
	srv := httptest.NewServer(m.handler())
	defer srv.Close()
	_, _, _, err := runDriveDownloadForTest(t, srv.URL, map[string]string{"file-token": "https://example.feishu.cn/docx/doxcnABC"})
	if err == nil || !strings.Contains(err.Error(), "drive export") {
		t.Fatalf("docx URL 应提示改用 export: %v", err)
	}
	if len(m.downloads) != 0 {
		t.Fatal("不应发起下载")
	}
}

// P1-13：query_by_token 识别为在线文档时报错；识别为 wiki 包装的文件时自动解包下载底层 file。
func TestDriveDownload_QueryByTokenDetection(t *testing.T) {
	m := &driveDownloadMock{queryBody: `{"code":0,"data":{"obj_token":"shtcnX","obj_type":"sheet","is_wiki_token":false}}`}
	srv := httptest.NewServer(m.handler())
	defer srv.Close()
	_, _, _, err := runDriveDownloadForTest(t, srv.URL, map[string]string{"file-token": "shtcnX"})
	if err == nil || !strings.Contains(err.Error(), "drive export") || len(m.downloads) != 0 {
		t.Fatalf("sheet 应提示 export: %v downloads=%v", err, m.downloads)
	}

	m2 := &driveDownloadMock{queryBody: `{"code":0,"data":{"obj_token":"boxcnReal","obj_type":"file","is_wiki_token":true}}`}
	srv2 := httptest.NewServer(m2.handler())
	defer srv2.Close()
	out, _, dir, err := runDriveDownloadForTest(t, srv2.URL, map[string]string{"file-token": "wikcnNode"})
	if err != nil {
		t.Fatalf("wiki 包装的文件应解包下载: %v", err)
	}
	if len(m2.downloads) != 1 || m2.downloads[0] != "boxcnReal" {
		t.Fatalf("应下载底层 file token: %v", m2.downloads)
	}
	var res map[string]any
	_ = json.Unmarshal([]byte(out), &res)
	if res["wiki_token"] != "wikcnNode" || res["file_token"] != "boxcnReal" {
		t.Fatalf("输出 = %+v", res)
	}
	// 输出为目录时按 Content-Disposition 命名（文件名含 ".." 子串也合法）
	got, readErr := os.ReadFile(filepath.Join(dir, "report..v2.pdf"))
	if readErr != nil || string(got) != "content-of-boxcnReal" {
		t.Fatalf("默认文件名应取 Content-Disposition: %v %q", readErr, got)
	}
}

// 识别失败只告警、按原 token 继续；--as bot 使用 tenant token 流式下载。
func TestDriveDownload_QueryFailureFallsBackAndBotIdentity(t *testing.T) {
	m := &driveDownloadMock{} // query_by_token 返回 403
	srv := httptest.NewServer(m.handler())
	defer srv.Close()
	_, stderr, _, err := runDriveDownloadForTest(t, srv.URL, map[string]string{"file-token": "boxcnOrig", "as": "bot"})
	if err != nil {
		t.Fatalf("识别失败不应阻断下载: %v", err)
	}
	if !strings.Contains(stderr, "query_by_token") || !strings.Contains(stderr, "981004") {
		t.Fatalf("应在 stderr 告警: %q", stderr)
	}
	if len(m.downloads) != 1 || m.downloads[0] != "boxcnOrig" {
		t.Fatalf("应按原 token 下载: %v", m.downloads)
	}
	if m.auths[0] != "Bearer t-mock-token" {
		t.Fatalf("--as bot 应使用 tenant token，got %q", m.auths[0])
	}
}

func TestDriveDownload_ExistingFileRequiresOverwrite(t *testing.T) {
	m := &driveDownloadMock{queryBody: `{"code":0,"data":{"obj_token":"boxcnA","obj_type":"file"}}`}
	srv := httptest.NewServer(m.handler())
	defer srv.Close()
	p := filepath.Join(t.TempDir(), "exists.bin")
	_ = os.WriteFile(p, []byte("old"), 0o644)
	_, _, _, err := runDriveDownloadForTest(t, srv.URL, map[string]string{"file-token": "boxcnA", "output": p})
	if err == nil || !strings.Contains(err.Error(), "--overwrite") {
		t.Fatalf("已存在应要求 --overwrite: %v", err)
	}
	if len(m.downloads) != 0 {
		t.Fatal("显式文件路径已存在时不应联网下载")
	}
	_, _, _, err = runDriveDownloadForTest(t, srv.URL, map[string]string{"file-token": "boxcnA", "output": p, "overwrite": "true"})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(p); string(got) != fmt.Sprintf("content-of-%s", "boxcnA") {
		t.Fatalf("覆盖后内容 = %q", got)
	}
}
