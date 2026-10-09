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

// TestImportPartialFailureExitsNonZero 图片上传失败时 doc import 必须非零退出，
// 同时保留文档链接并在 JSON 中给出 failures 明细（此前 exit 0，只有计数）。
func TestImportPartialFailureExitsNonZero(t *testing.T) {
	var mu sync.Mutex
	next := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/blocks/doc1/children"):
			var req struct {
				Children []map[string]any `json:"children"`
			}
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &req)
			mu.Lock()
			out := make([]map[string]any, 0, len(req.Children))
			for _, c := range req.Children {
				next++
				c["block_id"] = fmt.Sprintf("blk_%d", next)
				out = append(out, c)
			}
			mu.Unlock()
			b, _ := json.Marshal(map[string]any{"code": 0, "msg": "ok", "data": map[string]any{"children": out}})
			_, _ = w.Write(b)
		case strings.HasSuffix(r.URL.Path, "/medias/upload_all"):
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"code":1061002,"msg":"params error"}`)
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "img.png"), []byte("\x89PNG\r\n\x1a\nnot-really-png"), 0o600); err != nil {
		t.Fatal(err)
	}
	mdPath := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(mdPath, []byte("段落文字\n\n![图](img.png)\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_ = importMarkdownCmd.Flags().Set("document-id", "doc1")
	_ = importMarkdownCmd.Flags().Set("output", "json")
	defer func() {
		_ = importMarkdownCmd.Flags().Set("document-id", "")
		_ = importMarkdownCmd.Flags().Set("output", "")
	}()

	var runErr error
	stdout := captureStdout(t, func() {
		runErr = importMarkdownCmd.RunE(importMarkdownCmd, []string{mdPath})
	})
	if runErr == nil {
		t.Fatal("图片上传失败时 doc import 必须非零退出")
	}
	if !strings.Contains(runErr.Error(), "部分内容导入失败") || !strings.Contains(runErr.Error(), "/docx/doc1") {
		t.Fatalf("错误应说明部分失败并保留文档链接: %v", runErr)
	}
	var out struct {
		DocumentID     string          `json:"document_id"`
		URL            string          `json:"url"`
		PartialFailure bool            `json:"partial_failure"`
		ImageFailed    int             `json:"image_failed"`
		Failures       []importFailure `json:"failures"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout 不是 JSON: %v\n%s", err, stdout)
	}
	if !out.PartialFailure || out.ImageFailed != 1 || len(out.Failures) != 1 || out.Failures[0].Kind != "image" ||
		!strings.Contains(out.Failures[0].Error, "1061002") || out.URL == "" {
		t.Fatalf("JSON 输出异常: %+v", out)
	}
}

func TestImportFailureErrorNilWhenNoFailures(t *testing.T) {
	if err := importFailureError(nil, "doc"); err != nil {
		t.Fatalf("无失败时不应报错: %v", err)
	}
	if got := importFailuresForJSON(nil); got == nil || len(got) != 0 {
		t.Fatalf("failures 应为空数组: %#v", got)
	}
	var sb strings.Builder
	printImportFailures(&sb, []importFailure{{Kind: "table", Index: 2, Error: "boom"}, {Kind: "image", Index: 1, Source: "a.png", Error: "x"}})
	if !strings.Contains(sb.String(), "图片 1 (a.png): x") || !strings.Contains(sb.String(), "表格 2: boom") {
		t.Fatalf("失败明细输出异常: %q", sb.String())
	}
}
