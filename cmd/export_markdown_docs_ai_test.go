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
	"testing"

	"github.com/spf13/pflag"
)

func resetExportFlags() {
	exportMarkdownCmd.Flags().VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue); f.Changed = false })
}

func TestDocExportDocsAIEngine(t *testing.T) {
	defer resetExportFlags()
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
			return
		}
		if r.URL.Path != "/open-apis/docs_ai/v1/documents/docE/fetch" {
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
			return
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		fmt.Fprint(w, `{"code":0,"msg":"","data":{"document":{"content":"<callout emoji=\"💡\">\n提示\n</callout>","revision_id":2}}}`)
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)

	out := filepath.Join(t.TempDir(), "doc.md")
	_ = exportMarkdownCmd.Flags().Set("engine", "docs_ai")
	_ = exportMarkdownCmd.Flags().Set("output", out)
	var err error
	captureStdout(t, func() { err = exportMarkdownCmd.RunE(exportMarkdownCmd, []string{"docE"}) })
	if err != nil {
		t.Fatalf("docs_ai 导出失败: %v", err)
	}
	data, _ := os.ReadFile(out)
	if !strings.Contains(string(data), `<callout emoji="💡">`) || body["format"] != "markdown" || body["read_option"] != nil {
		t.Fatalf("导出异常: body=%#v content=%q", body, data)
	}
}

func TestDocExportEngineFlagConflicts(t *testing.T) {
	defer resetExportFlags()
	initDocUpdateTestConfig(t, "http://127.0.0.1:59997")
	_ = exportMarkdownCmd.Flags().Set("detail", "full")
	if err := exportMarkdownCmd.RunE(exportMarkdownCmd, []string{"docE"}); err == nil || !strings.Contains(err.Error(), "只用于 --engine docs_ai") {
		t.Fatalf("本地引擎使用 --detail 应报错: %v", err)
	}
	resetExportFlags()
	_ = exportMarkdownCmd.Flags().Set("engine", "docs_ai")
	_ = exportMarkdownCmd.Flags().Set("download-images", "true")
	if err := exportMarkdownCmd.RunE(exportMarkdownCmd, []string{"docE"}); err == nil || !strings.Contains(err.Error(), "本地引擎专属") {
		t.Fatalf("docs_ai 引擎使用 --download-images 应报错: %v", err)
	}
}
