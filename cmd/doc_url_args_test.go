package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// TestDocCommandsAcceptDocxURL doc 子命令的文档参数除裸 ID 外接受 /docx/ URL（与 doc export/read 一致），
// 请求落到解析出的 document_id 上。
func TestDocCommandsAcceptDocxURL(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/tenant_access_token") {
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-bot","expire":7200}`)
			return
		}
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.Path)
		mu.Unlock()
		switch {
		case r.URL.Path == "/open-apis/docx/v1/documents/DocURL123":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"document":{"document_id":"DocURL123","title":"t","revision_id":3}}}`)
		case r.URL.Path == "/open-apis/docx/v1/documents/DocURL123/blocks":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"items":[],"has_more":false}}`)
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
		}
	})
	defer cleanup()
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")

	const url = "https://example.feishu.cn/docx/DocURL123?from=share"
	if _, err := captureCmdStdout(t, func() error { return getDocumentCmd.RunE(getDocumentCmd, []string{url}) }); err != nil {
		t.Fatalf("doc get <url>: %v", err)
	}
	_ = getBlocksCmd.Flags().Set("all", "true")
	defer resetCmdFlag(getBlocksCmd, "all")
	if _, err := captureCmdStdout(t, func() error { return getBlocksCmd.RunE(getBlocksCmd, []string{url}) }); err != nil {
		t.Fatalf("doc blocks <url>: %v", err)
	}
	joined := strings.Join(paths, "\n")
	for _, want := range []string{"GET /open-apis/docx/v1/documents/DocURL123", "GET /open-apis/docx/v1/documents/DocURL123/blocks"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("应请求 %q，实际:\n%s", want, joined)
		}
	}

	// 非文档 URL 应在发请求前报错
	if err := getDocumentCmd.RunE(getDocumentCmd, []string{"https://example.feishu.cn/sheets/ShtTok"}); err == nil ||
		!strings.Contains(err.Error(), "docx") {
		t.Fatalf("sheets URL 应被拒绝: %v", err)
	}
}

// TestDocExportFileInfersTypeFromURL doc export-file 接受 URL：/sheets/ 推断 doc-type=sheet，
// 与显式 --doc-type 冲突时报错。
func TestDocExportFileInfersTypeFromURL(t *testing.T) {
	var body map[string]any
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/tenant_access_token"):
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-bot","expire":7200}`)
		case r.URL.Path == "/open-apis/drive/v1/export_tasks":
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &body)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"code":1069902,"msg":"stop here"}`)
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
		}
	})
	defer cleanup()
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")

	_ = exportFileCmd.Flags().Set("type", "xlsx")
	defer resetCmdFlag(exportFileCmd, "type", "doc-type")
	_, _ = captureCmdStdout(t, func() error {
		return exportFileCmd.RunE(exportFileCmd, []string{"https://example.feishu.cn/sheets/ShtTok123?sheet=abc"})
	})
	if body["token"] != "ShtTok123" || body["type"] != "sheet" {
		t.Fatalf("应按 URL 推断 sheet 类型与 token: %v", body)
	}

	_ = exportFileCmd.Flags().Set("doc-type", "docx")
	if err := exportFileCmd.RunE(exportFileCmd, []string{"https://example.feishu.cn/sheets/ShtTok123"}); err == nil ||
		!strings.Contains(err.Error(), "冲突") {
		t.Fatalf("显式 --doc-type 与 URL 冲突应报错: %v", err)
	}
}
