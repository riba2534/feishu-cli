package cmd

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/v2/internal/clierr"
)

// TestEstimateMermaidComplexity_RelaxedThresholds 实测服务端能渲染 par、12 个 participant、
// 3 层嵌套 alt：这些规模不再预警；只有明显超出实测范围才提示。
func TestEstimateMermaidComplexity_RelaxedThresholds(t *testing.T) {
	var b strings.Builder
	b.WriteString("sequenceDiagram\n")
	for i := 0; i < 12; i++ {
		fmt.Fprintf(&b, "  participant P%d\n", i)
	}
	b.WriteString("  par 并行\n    P0->>P1: a\n  and\n    P1->>P2: b\n  end\n")
	b.WriteString("  alt x\n    alt y\n      alt z\n        P0->>P3: deep\n      end\n    end\n  end\n")
	if warn := estimateMermaidComplexity(b.String(), "content"); warn != "" {
		t.Fatalf("par + 12 participant + 3 层嵌套 alt 已实测可渲染，不应预警: %q", warn)
	}

	var big strings.Builder
	big.WriteString("sequenceDiagram\n")
	for i := 0; i < mermaidWarnParticipants; i++ {
		fmt.Fprintf(&big, "  actor A%d\n", i)
	}
	for i := 0; i < mermaidWarnAltBlocks; i++ {
		big.WriteString("  alt c\n    A0->>A1: x\n  end\n")
	}
	warn := estimateMermaidComplexity(big.String(), "content")
	if !strings.Contains(warn, fmt.Sprintf("participant 数 %d", mermaidWarnParticipants)) || !strings.Contains(warn, fmt.Sprintf("alt 块 %d", mermaidWarnAltBlocks)) {
		t.Fatalf("超出实测范围应预警并列出全部原因: %q", warn)
	}
	if strings.Contains(warn, "par 语法") || strings.Contains(warn, "不支持") {
		t.Fatalf("par 不再视为不支持: %q", warn)
	}
}

// TestMermaidComplexityHint_SuggestsLocalEngineOnly 预警只建议 --engine local，不建议 svg-import
// （svg-import 会把整张图变成一个不可编辑节点）。
func TestMermaidComplexityHint_SuggestsLocalEngineOnly(t *testing.T) {
	if !strings.Contains(mermaidComplexityHint, "--engine local") || !strings.Contains(mermaidComplexityHint, "Parse error") {
		t.Fatalf("提示应说明遇到 Parse error 再切 --engine local: %q", mermaidComplexityHint)
	}
	if strings.Contains(mermaidComplexityHint, "svg-import") {
		t.Fatalf("提示不应建议 svg-import: %q", mermaidComplexityHint)
	}

	parseErr := clierr.Usage(errors.New("导入图表失败: code=2891001, msg=Parse error on line 3"))
	got := mermaidParseErrorHint(parseErr, "mermaid")
	if !strings.Contains(got.Error(), "--engine local") || strings.Contains(got.Error(), "svg-import") {
		t.Fatalf("Parse error 应提示 --engine local: %v", got)
	}
	if !clierr.HasKind(got, clierr.KindUsage) || !errors.Is(got, parseErr) {
		t.Fatalf("追加提示必须保留原错误链/分类: %v", clierr.Kinds(got))
	}
	if other := errors.New("internal server error"); mermaidParseErrorHint(other, "mermaid") != other {
		t.Fatal("非 Parse error 不应追加提示")
	}
	if mermaidParseErrorHint(parseErr, "plantuml") != parseErr {
		t.Fatal("非 Mermaid 不应追加 --engine local 提示")
	}
}

// TestBoardImport_ParseErrorSuggestsLocalEngine 服务端引擎返回 Parse error 时，board import 的错误里给出 --engine local。
func TestBoardImport_ParseErrorSuggestsLocalEngine(t *testing.T) {
	posts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mockAuthHandler(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/whiteboards/wb1/nodes"):
			_, _ = io.WriteString(w, `{"code":0,"data":{"nodes":[]}}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/nodes/plantuml"):
			posts++
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"code":2891001,"msg":"Parse error on line 2"}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))

	c := newBoardImportTestCmd(t, map[string]string{"syntax": "mermaid", "source-type": "content"})
	err := importDiagramCmd.RunE(c, []string{"wb1", "graph TD\nA--> {bad"})
	if err == nil || !strings.Contains(err.Error(), "--engine local") {
		t.Fatalf("Parse error 应提示 --engine local，got %v", err)
	}
	if posts != 1 {
		t.Fatalf("Parse error 是永久错误，不应重试；实际 POST %d 次", posts)
	}
}
