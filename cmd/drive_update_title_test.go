package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

type titleMock struct {
	mu        sync.Mutex
	current   string
	patches   []map[string]any
	patchType []string
	patchPath []string
	patchCode int
}

func (m *titleMock) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if mockAuthHandler(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/drive/v1/metas/batch_query":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"metas": []map[string]any{{"title": m.current}}}})
		case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/open-apis/drive/v1/files/"):
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			m.mu.Lock()
			m.patches = append(m.patches, body)
			m.patchType = append(m.patchType, r.URL.Query().Get("type"))
			m.patchPath = append(m.patchPath, r.URL.Path)
			m.mu.Unlock()
			if m.patchCode != 0 {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": m.patchCode, "msg": "err"})
				return
			}
			_, _ = io.WriteString(w, `{"code":0,"data":{}}`)
		default:
			http.NotFound(w, r)
		}
	}
}

func runUpdateTitle(t *testing.T, srvURL string, flags map[string]string) (string, error) {
	t.Helper()
	cleanup := setupCmdTestConfig(t, srvURL)
	t.Cleanup(cleanup)
	c := driveUpdateTitleCmd
	resetDriveCmdFlags(t, c)
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
	err := c.RunE(c, nil)
	w.Close()
	os.Stdout = oldStdout
	out, _ := io.ReadAll(r)
	return string(out), err
}

func TestDriveUpdateTitle_FileKeepsExtension(t *testing.T) {
	m := &titleMock{current: "report.pdf"}
	srv := httptest.NewServer(m.handler())
	defer srv.Close()
	out, err := runUpdateTitle(t, srv.URL, map[string]string{"token": "boxcnA", "type": "file", "title": "report-v2"})
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if len(m.patches) != 1 || m.patches[0]["new_title"] != "report-v2.pdf" || m.patchType[0] != "file" || m.patchPath[0] != "/open-apis/drive/v1/files/boxcnA" {
		t.Fatalf("patches=%v type=%v path=%v", m.patches, m.patchType, m.patchPath)
	}
	if !strings.Contains(out, `"extension_appended": ".pdf"`) || !strings.Contains(out, `"previous_title": "report.pdf"`) {
		t.Fatalf("输出缺扩展名信息:\n%s", out)
	}

	// 改变扩展名默认拒绝，且不发 PATCH
	m.patches = nil
	if _, err := runUpdateTitle(t, srv.URL, map[string]string{"token": "boxcnA", "type": "file", "title": "report.docx"}); err == nil {
		t.Fatal("修改扩展名应被拒绝")
	}
	if len(m.patches) != 0 {
		t.Fatal("拒绝时不应发 PATCH")
	}
	// allow 原样提交
	if _, err := runUpdateTitle(t, srv.URL, map[string]string{"token": "boxcnA", "type": "file", "title": "report.docx", "on-extension-mismatch": "allow"}); err != nil || m.patches[0]["new_title"] != "report.docx" {
		t.Fatalf("allow 应原样提交: %v %v", err, m.patches)
	}
}

func TestDriveUpdateTitle_URLAndValidation(t *testing.T) {
	m := &titleMock{}
	srv := httptest.NewServer(m.handler())
	defer srv.Close()
	if _, err := runUpdateTitle(t, srv.URL, map[string]string{"url": "https://example.feishu.cn/base/bascnX", "title": "新表"}); err != nil {
		t.Fatal(err)
	}
	if m.patchType[0] != "bitable" || m.patchPath[0] != "/open-apis/drive/v1/files/bascnX" {
		t.Fatalf("base URL 应归一为 bitable: %v %v", m.patchType, m.patchPath)
	}
	for _, bad := range []map[string]string{
		{"token": "doxcnA", "title": "x"},                                                   // 裸 token 缺 --type
		{"token": "doxcnA", "type": "mindnote", "title": "x"},                               // 服务端不支持
		{"url": "https://example.feishu.cn/docx/doxcnA", "type": "sheet", "title": "x"},     // 类型冲突
		{"token": "doxcnA", "type": "docx", "title": "   "},                                 // 空标题
		{"token": "doxcnA", "type": "docx", "title": "x", "on-extension-mismatch": "allow"}, // 非 file 不接受
	} {
		if _, err := runUpdateTitle(t, srv.URL, bad); err == nil {
			t.Errorf("%v 应报错", bad)
		}
	}
	if len(m.patches) != 1 {
		t.Fatalf("非法输入不应发请求: %d", len(m.patches))
	}

	m.patchCode = 981004
	if _, err := runUpdateTitle(t, srv.URL, map[string]string{"token": "doxcnA", "type": "docx", "title": "x"}); err == nil || !strings.Contains(err.Error(), "编辑权限") {
		t.Fatalf("981004 应给出权限提示: %v", err)
	}
}

// P1-14：file delete 以 async 模式提交，返回 task_id 时轮询，删除任务的 fail 状态报错。
func TestFileDelete_AsyncPolling(t *testing.T) {
	var gotAsync string
	status := "fail"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mockAuthHandler(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodDelete:
			gotAsync = r.URL.Query().Get("async")
			_, _ = io.WriteString(w, `{"code":0,"data":{"task_id":"tk1"}}`)
		case r.URL.Path == "/open-apis/drive/v1/files/task_check":
			_, _ = io.WriteString(w, `{"code":0,"data":{"status":"`+status+`"}}`)
		}
	}))
	defer srv.Close()
	cleanup := setupCmdTestConfig(t, srv.URL)
	defer cleanup()
	c := deleteFileCmd
	resetDriveCmdFlags(t, c)
	_ = c.Flags().Set("type", "folder")
	_ = c.Flags().Set("force", "true")
	_ = c.Flags().Set("output", "json")

	captureStdout(t, func() {
		err := c.RunE(c, []string{"fldcnX"})
		if err == nil || !strings.Contains(err.Error(), "任务失败") {
			t.Fatalf("删除任务 fail 应报错: %v", err)
		}
	})
	if gotAsync != "true" {
		t.Fatalf("应以 async=true 提交，got %q", gotAsync)
	}
	status = "success"
	out := captureStdout(t, func() {
		if err := c.RunE(c, []string{"fldcnX"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, `"deleted": true`) {
		t.Fatalf("成功应输出 deleted=true:\n%s", out)
	}
}
