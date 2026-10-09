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

// TestBoardExportCodeSource 覆盖 --source：从节点 syntax.code 取回 Mermaid/PlantUML 源码，多块时要求 --node-id。
func TestBoardExportCodeSource(t *testing.T) {
	nodes, err := parseBoardExportNodes(json.RawMessage(`[
		{"id":"t1:4","type":"section","syntax":{"code":"@startuml\nA -> B\n@enduml\n","syntax_type":1}},
		{"id":"t1:2","type":"section","syntax":{"code":"graph TD\nA-->B\n","syntax_type":2}},
		{"id":"o1:1","type":"composite_shape"},
		{"id":"x:1","type":"section","syntax":{"code":"<svg/>","syntax_type":3}}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	blocks := collectBoardSourceBlocks(nodes)
	if len(blocks) != 2 || blocks[0].NodeID != "t1:2" || blocks[0].Syntax != "mermaid" || blocks[1].Syntax != "plantuml" {
		t.Fatalf("blocks = %+v", blocks)
	}
	if err := exportBoardDiagramSource(nodes, "", "", false); err == nil || !clierr.HasKind(err, clierr.KindUsage) || !strings.Contains(err.Error(), "--node-id") {
		t.Fatalf("多块未指定 --node-id 应用法错误: %v", err)
	}
	out, err := captureAppsStdout(t, func() error { return exportBoardDiagramSource(nodes, "t1:2", "", false) })
	if err != nil || out != "graph TD\nA-->B\n" {
		t.Fatalf("stdout = %q, err=%v", out, err)
	}
	dir := t.TempDir()
	if _, err := captureAppsStdout(t, func() error { return exportBoardDiagramSource(nodes, "t1:4", dir+"/diagram", false) }); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(dir + "/diagram.puml"); err != nil || !strings.Contains(string(b), "@startuml") {
		t.Fatalf("应按语法补 .puml 扩展名: %v %q", err, b)
	}
	if err := exportBoardDiagramSource(nodes[2:3], "", "", false); err == nil {
		t.Fatal("没有源码块应报错")
	}
}

// TestBoardExportCodeSourceNoSilentOverwrite --source 写文件：已存在且未加 --overwrite 时报错、保留原文件；
// 加 --overwrite 才覆盖（与 board svg-export 一致）。
func TestBoardExportCodeSourceNoSilentOverwrite(t *testing.T) {
	nodes, err := parseBoardExportNodes(json.RawMessage(`[{"id":"t1:2","type":"section","syntax":{"code":"graph TD\nA-->B\n","syntax_type":2}}]`))
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/diagram.mmd"
	if err := os.WriteFile(path, []byte("ORIGINAL"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = captureAppsStdout(t, func() error { return exportBoardDiagramSource(nodes, "", path, false) })
	if err == nil || !strings.Contains(err.Error(), "--overwrite") {
		t.Fatalf("已存在文件未加 --overwrite 应报错，got %v", err)
	}
	if b, _ := os.ReadFile(path); string(b) != "ORIGINAL" {
		t.Fatalf("未加 --overwrite 不应改动原文件: %q", b)
	}
	// 不带扩展名时按语法补 .mmd 后再判断是否存在
	if _, err := captureAppsStdout(t, func() error { return exportBoardDiagramSource(nodes, "", strings.TrimSuffix(path, ".mmd"), false) }); err == nil {
		t.Fatal("补扩展名后的路径已存在，也应报错")
	}
	if _, err := captureAppsStdout(t, func() error { return exportBoardDiagramSource(nodes, "", path, true) }); err != nil {
		t.Fatalf("--overwrite 应覆盖: %v", err)
	}
	if b, _ := os.ReadFile(path); string(b) != "graph TD\nA-->B\n" {
		t.Fatalf("--overwrite 后内容 = %q", b)
	}
	if boardExportCodeCmd.Flags().Lookup("overwrite") == nil {
		t.Fatal("board export-code 应注册 --overwrite")
	}
}

// TestBoardUpdate_ClientTokenQuery 验证 board update --client-token 透传为 /nodes 的 query client_token（该端点实测幂等）。
func TestBoardUpdate_ClientTokenQuery(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mockAuthHandler(w, r) {
			return
		}
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"data":{"ids":["o1:1"],"client_token":"fp-token-0001"}}`)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	dir := t.TempDir()
	f := dir + "/nodes.json"
	_ = os.WriteFile(f, []byte(`[{"type":"composite_shape"}]`), 0o644)

	if _, err := runSlidesCmd(t, boardUpdateCmd, []string{"wb1", f}, map[string]string{"client-token": "short"}); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("过短 client-token 应用法错误: %v", err)
	}
	out, err := runSlidesCmd(t, boardUpdateCmd, []string{"wb1", f}, map[string]string{"client-token": "fp-token-0001", "output": "json"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotQuery, "client_token=fp-token-0001") || !strings.Contains(out, `"client_token": "fp-token-0001"`) {
		t.Fatalf("query=%q out=%s", gotQuery, out)
	}
}

// TestBoardExportCodeSVGModeNoSilentOverwrite 默认 svg 模式同样不静默覆盖已有文件。
func TestBoardExportCodeSVGModeNoSilentOverwrite(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mockAuthHandler(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"data":{"nodes":[{"id":"s:1","type":"svg","svg":{"svg_code":"<svg><rect/></svg>"},"x":0,"y":0,"width":10,"height":10}]}}`)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	path := t.TempDir() + "/out.svg"
	if err := os.WriteFile(path, []byte("ORIGINAL"), 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(overwrite string) error {
		resetSlidesCmdFlags(boardExportCodeCmd)
		t.Cleanup(func() { resetSlidesCmdFlags(boardExportCodeCmd) })
		_ = boardExportCodeCmd.Flags().Set("output-path", path)
		_ = boardExportCodeCmd.Flags().Set("overwrite", overwrite)
		_, err := captureAppsStdout(t, func() error { return boardExportCodeCmd.RunE(boardExportCodeCmd, []string{"wb_fp_test"}) })
		return err
	}
	if err := run("false"); err == nil || !strings.Contains(err.Error(), "已存在") {
		t.Fatalf("svg 模式已存在文件应报错: %v", err)
	}
	if b, _ := os.ReadFile(path); string(b) != "ORIGINAL" {
		t.Fatalf("不应改动原文件: %q", b)
	}
	if err := run("true"); err != nil {
		t.Fatalf("--overwrite 应成功: %v", err)
	}
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), "<rect/>") {
		t.Fatalf("--overwrite 后内容 = %q", b)
	}
}
