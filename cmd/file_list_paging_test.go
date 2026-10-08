package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func runFileListForTest(t *testing.T, srvURL string, flags map[string]string) (stdout, stderr string, err error) {
	t.Helper()
	cleanup := setupCmdTestConfig(t, srvURL)
	t.Cleanup(cleanup)
	c := listFilesCmd
	resetDriveCmdFlags(t, c)
	_ = c.Flags().Set("user-access-token", "u-test-token")
	_ = c.Flags().Set("output", "json")
	for k, v := range flags {
		if e := c.Flags().Set(k, v); e != nil {
			t.Fatalf("set %s: %v", k, e)
		}
	}
	var errBuf strings.Builder
	c.SetErr(&errBuf)
	defer c.SetErr(nil)
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err = c.RunE(c, []string{"fld_root"})
	w.Close()
	os.Stdout = oldStdout
	out, _ := io.ReadAll(r)
	return string(out), errBuf.String(), err
}

// P0-2：file list 只取首页时必须提示 has_more 与 page_token；--page-all 翻完全部页。
func TestFileList_PaginationHintAndPageAll(t *testing.T) {
	var tokens []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mockAuthHandler(w, r) {
			return
		}
		pt := r.URL.Query().Get("page_token")
		tokens = append(tokens, pt)
		w.Header().Set("Content-Type", "application/json")
		switch pt {
		case "":
			_, _ = io.WriteString(w, `{"code":0,"data":{"files":[{"token":"t1","name":"a","type":"file"}],"has_more":true,"next_page_token":"p2"}}`)
		case "p2":
			_, _ = io.WriteString(w, `{"code":0,"data":{"files":[{"token":"t2","name":"b","type":"file"}],"has_more":false}}`)
		}
	}))
	defer srv.Close()

	out, stderr, err := runFileListForTest(t, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	var files []map[string]any
	if err := json.Unmarshal([]byte(out), &files); err != nil || len(files) != 1 {
		t.Fatalf("首页应输出 1 项数组: %v\n%s", err, out)
	}
	if !strings.Contains(stderr, "has_more=true") || !strings.Contains(stderr, "--page-token 'p2'") {
		t.Fatalf("应在 stderr 提示续翻: %q", stderr)
	}

	tokens = nil
	out, stderr, err = runFileListForTest(t, srv.URL, map[string]string{"page-all": "true"})
	if err != nil {
		t.Fatal(err)
	}
	files = nil
	_ = json.Unmarshal([]byte(out), &files)
	if len(files) != 2 || strings.Contains(stderr, "has_more=true") {
		t.Fatalf("--page-all 应拉全 2 项且无续翻提示: files=%d stderr=%q tokens=%v", len(files), stderr, tokens)
	}

	_, _, err = runFileListForTest(t, srv.URL, map[string]string{"page-token": "p2"})
	if err != nil || tokens[len(tokens)-1] != "p2" {
		t.Fatalf("--page-token 应从 p2 续翻: err=%v tokens=%v", err, tokens)
	}
}
