package client

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestBuildImportDiagramBody_SyntaxWhitelist 回归：--syntax svg 过去落到 switch default 被当成 PlantUML(1)；
// 现在 svg 映射 syntax_type=3，未知取值报错。
func TestBuildImportDiagramBody_SyntaxWhitelist(t *testing.T) {
	cases := []struct {
		syntax     string
		wantType   int
		wantErr    bool
		wantLayout bool // 是否携带 style_type / diagram_type
	}{
		{"", 1, false, true},
		{"plantuml", 1, false, true},
		{"PlantUML", 1, false, true},
		{"mermaid", 2, false, true},
		{"svg", 3, false, false},
		{"SVG", 3, false, false},
		{"graphviz", 0, true, false},
		{"dot", 0, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.syntax, func(t *testing.T) {
			body, err := BuildImportDiagramBody("code", ImportDiagramOptions{Syntax: tc.syntax})
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "--syntax") {
					t.Fatalf("syntax=%q 应报错并点名 --syntax，got body=%v err=%v", tc.syntax, body, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("syntax=%q unexpected err: %v", tc.syntax, err)
			}
			if body["syntax_type"] != tc.wantType {
				t.Fatalf("syntax=%q syntax_type=%v, want %d", tc.syntax, body["syntax_type"], tc.wantType)
			}
			_, hasStyle := body["style_type"]
			_, hasDiagram := body["diagram_type"]
			if hasStyle != tc.wantLayout || hasDiagram != tc.wantLayout {
				t.Fatalf("syntax=%q style_type/diagram_type 存在=%v/%v, want %v", tc.syntax, hasStyle, hasDiagram, tc.wantLayout)
			}
		})
	}
}

func TestValidateImportDiagramOptions_RejectsUnknownEnums(t *testing.T) {
	bad := []ImportDiagramOptions{
		{Syntax: "svgz"},
		{Style: "fancy"},
		{DiagramType: "gantt"},
		{ClientToken: "short"},
	}
	for _, o := range bad {
		if err := ValidateImportDiagramOptions(o); err == nil {
			t.Errorf("ValidateImportDiagramOptions(%+v) 应报错", o)
		}
	}
	good := ImportDiagramOptions{Syntax: "svg", Style: "classic", DiagramType: "Flowchart", ClientToken: "0123456789"}
	if err := ValidateImportDiagramOptions(good); err != nil {
		t.Fatalf("合法取值不应报错: %v", err)
	}
}

// TestImportDiagram_SVGRequestAndDegraded 验证 svg 走 syntax_type=3、client_token 进 query、
// 并解析 data.extra.degradedAttributes。
func TestImportDiagram_SVGRequestAndDegraded(t *testing.T) {
	var gotBody map[string]any
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "tenant_access_token") {
			_, _ = io.WriteString(w, `{"code":0,"tenant_access_token":"t-x","expire":7200}`)
			return
		}
		if r.URL.Path != "/open-apis/board/v1/whiteboards/wb1/nodes/plantuml" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"data":{"node_id":"o1:1","extra":{"degradedAttributes":["fill=url(#g) - not support"]}}}`)
	}))
	t.Cleanup(srv.Close)
	setupTestConfig(t, srv.URL)

	res, _, err := ImportDiagram("wb1", `<svg viewBox="0 0 10 10"><rect width="5" height="5"/></svg>`, ImportDiagramOptions{
		SourceType:      "content",
		Syntax:          "svg",
		ClientToken:     "fp-test-token-123",
		UserAccessToken: "u-test",
	})
	if err != nil {
		t.Fatalf("ImportDiagram: %v", err)
	}
	if gotBody["syntax_type"] != float64(3) {
		t.Fatalf("syntax_type = %v, want 3", gotBody["syntax_type"])
	}
	if _, ok := gotBody["style_type"]; ok {
		t.Fatalf("svg 请求不应携带 style_type: %v", gotBody)
	}
	if !strings.Contains(gotQuery, "client_token=fp-test-token-123") {
		t.Fatalf("client_token 应进 query，got %q", gotQuery)
	}
	if res.TicketID != "o1:1" || len(res.DegradedAttributes) != 1 {
		t.Fatalf("result = %+v", res)
	}
}

// TestImportDiagram_BusinessErrorOnHTTP400 验证随 HTTP 400 下发的业务码被解析成 code=N（而不是 HTTP 400 body），
// 并保留 HTTP 状态供重试分类判定为不可重试。
func TestImportDiagram_BusinessErrorOnHTTP400(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "tenant_access_token") {
			_, _ = io.WriteString(w, `{"code":0,"tenant_access_token":"t-x","expire":7200}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"code":2890002,"msg":"internal error: Parse error near line 1"}`)
	}))
	t.Cleanup(srv.Close)
	setupTestConfig(t, srv.URL)

	_, _, err := ImportDiagram("wb1", "graph TD\nA-->B", ImportDiagramOptions{SourceType: "content", Syntax: "mermaid"})
	if err == nil {
		t.Fatal("期望报错")
	}
	if !HasAPICode(err, 2890002) {
		t.Fatalf("应能按业务码 2890002 分支，got %v", err)
	}
	if _, ok := AsAPIError(err); !ok {
		t.Fatalf("应是 *APIError，got %T %v", err, err)
	}
	if IsRetryableError(err) {
		t.Fatalf("HTTP 400 业务错误不应被判为可重试（即使 msg 含 internal error）: %v", err)
	}
	if !IsPermanentError(err) {
		t.Fatalf("Parse error 应仍被判为永久错误: %v", err)
	}
}
