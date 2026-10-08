package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

func newBoardImportTestCmd(t *testing.T, flags map[string]string) *cobra.Command {
	t.Helper()
	c := &cobra.Command{Use: "import"}
	c.Flags().String("source-type", "file", "")
	c.Flags().String("syntax", "plantuml", "")
	c.Flags().String("diagram-type", "auto", "")
	c.Flags().String("style", "board", "")
	c.Flags().Int("parse-mode", 1, "")
	c.Flags().Bool("overwrite", false, "")
	c.Flags().String("engine", "server", "")
	c.Flags().String("client-token", "", "")
	c.Flags().Bool("dry-run", false, "")
	c.Flags().String("user-access-token", "", "")
	c.Flags().StringP("output", "o", "", "")
	for k, v := range flags {
		if err := c.Flags().Set(k, v); err != nil {
			t.Fatalf("set %s: %v", k, err)
		}
	}
	return c
}

// TestBoardImport_UnknownSyntaxIsUsageError 回归：未知 --syntax 不再静默按 PlantUML 发送，而是用法错误（exit 2），
// 且 dry-run 也会拦下。
func TestBoardImport_UnknownSyntaxIsUsageError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mockAuthHandler(w, r) {
			return
		}
		t.Errorf("未知语法不应发请求: %s %s", r.Method, r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))

	for _, dry := range []string{"false", "true"} {
		c := newBoardImportTestCmd(t, map[string]string{"syntax": "graphviz", "source-type": "content", "dry-run": dry})
		err := importDiagramCmd.RunE(c, []string{"wb1", "digraph{}"})
		if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
			t.Fatalf("dry-run=%s 未知 --syntax 应是用法错误，got %v", dry, err)
		}
	}
}

// TestBoardImport_SVGDryRunSyntaxType3 验证 --syntax svg 的 dry-run 请求体是 syntax_type=3 且不带 style/diagram_type。
func TestBoardImport_SVGDryRunSyntaxType3(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mockAuthHandler(w, r) {
			return
		}
		t.Errorf("dry-run 不应发请求: %s %s", r.Method, r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))

	c := newBoardImportTestCmd(t, map[string]string{"syntax": "svg", "source-type": "content", "dry-run": "true", "output": "json"})
	out, err := captureAppsStdout(t, func() error { return importDiagramCmd.RunE(c, []string{"wb1", "<svg/>"}) })
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("dry-run 输出不是 JSON: %v\n%s", err, out)
	}
	if got["syntax_type"] != float64(3) {
		t.Fatalf("syntax_type = %v, want 3\n%s", got["syntax_type"], out)
	}
	body := got["request"].(map[string]any)["body"].(map[string]any)
	if body["syntax_type"] != float64(3) {
		t.Fatalf("request.body.syntax_type = %v", body["syntax_type"])
	}
	if _, ok := body["style_type"]; ok {
		t.Fatalf("svg 请求体不应含 style_type: %v", body)
	}
}

// TestBoardImport_ClientTokenOnlyForLocalEngine：服务端引擎接口实测不认 client_token，带上会误导用户以为幂等。
func TestBoardImport_ClientTokenOnlyForLocalEngine(t *testing.T) {
	t.Cleanup(setupCmdTestConfig(t, "http://127.0.0.1:1"))
	c := newBoardImportTestCmd(t, map[string]string{"syntax": "mermaid", "source-type": "content", "client-token": "0123456789ab", "dry-run": "true"})
	err := importDiagramCmd.RunE(c, []string{"wb1", "graph TD\nA-->B"})
	if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("server 引擎 + --client-token 应是用法错误，got %v", err)
	}
}

// TestBoardImport_RetryDedupesLandedRequest 模拟首个 POST 实际已在服务端落地、但响应是 5xx：
// 重试前回读发现新顶层节点，应直接返回该节点，不再发第二个 POST（避免重复建图）。
func TestBoardImport_RetryDedupesLandedRequest(t *testing.T) {
	var mu sync.Mutex
	posts := 0
	landed := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mockAuthHandler(w, r) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/whiteboards/wb1/nodes"):
			if landed {
				_, _ = io.WriteString(w, `{"code":0,"data":{"nodes":[{"id":"old:1","type":"text_shape"},{"id":"t1:9","type":"section"},{"id":"o1:9","parent_id":"t1:9","type":"composite_shape"}]}}`)
			} else {
				_, _ = io.WriteString(w, `{"code":0,"data":{"nodes":[{"id":"old:1","type":"text_shape"}]}}`)
			}
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/nodes/plantuml"):
			posts++
			landed = true
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"code":500,"msg":"internal server error"}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))

	c := newBoardImportTestCmd(t, map[string]string{"syntax": "mermaid", "source-type": "content", "output": "json"})
	out, err := captureAppsStdout(t, func() error { return importDiagramCmd.RunE(c, []string{"wb1", "graph TD\nA-->B"}) })
	if err != nil {
		t.Fatalf("已落地的请求应按成功返回: %v", err)
	}
	if posts != 1 {
		t.Fatalf("重试前回读发现已落地，不应再 POST；实际 POST %d 次", posts)
	}
	if !strings.Contains(out, `"ticket_id": "t1:9"`) {
		t.Fatalf("应返回新落地的顶层节点 t1:9，实际:\n%s", out)
	}
}
