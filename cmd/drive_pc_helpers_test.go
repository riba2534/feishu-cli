package cmd

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
)

// commentTestServer 模拟评论/权限相关 OpenAPI；handler 只处理业务路径，tenant token 由这里统一应答。
type recordedRequest struct {
	Method string
	Path   string
	Query  string
	Auth   string
	Body   string
}

func newCommentPermTestServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, body []byte)) (*httptest.Server, func() []recordedRequest) {
	t.Helper()
	var mu sync.Mutex
	var reqs []recordedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "tenant_access_token") {
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test-token","expire":7200}`)
			return
		}
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		reqs = append(reqs, recordedRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Auth: r.Header.Get("Authorization"), Body: string(body)})
		mu.Unlock()
		handler(w, r, body)
	}))
	t.Cleanup(server.Close)
	initWikiNodeDeleteTestConfig(t, server.URL)
	return server, func() []recordedRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]recordedRequest(nil), reqs...)
	}
}

// runCmdWithFlags 解析 flag（含父命令 persistent flag）后直接执行 RunE，结束后复位 flag。
func runCmdWithFlags(t *testing.T, c *cobra.Command, args []string, flags ...string) (stdout, stderr string, err error) {
	t.Helper()
	resetCommandTreeFlags(rootCmd)
	t.Cleanup(func() { resetCommandTreeFlags(rootCmd) })
	if perr := c.ParseFlags(flags); perr != nil {
		t.Fatalf("解析 flag 失败: %v", perr)
	}
	var errBuf bytes.Buffer
	c.SetErr(&errBuf)
	t.Cleanup(func() { c.SetErr(nil) })
	stdout = captureStdout(t, func() { err = c.RunE(c, args) })
	return stdout, errBuf.String(), err
}
