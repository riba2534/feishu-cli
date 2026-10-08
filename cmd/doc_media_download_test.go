package cmd

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocMediaDownloadContextFlags(t *testing.T) {
	for _, name := range []string{"doc-token", "doc-type", "extra"} {
		if docMediaDownloadCmd.Flags().Lookup(name) == nil {
			t.Fatalf("doc media-download missing --%s flag", name)
		}
	}

	docType, err := docMediaDownloadCmd.Flags().GetString("doc-type")
	if err != nil {
		t.Fatalf("get doc-type flag: %v", err)
	}
	if docType != "docx" {
		t.Fatalf("doc-type default = %q, want docx", docType)
	}
}

func TestSniffMediaExtension(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	if got := sniffMediaExtension(write("a", png)); got != ".png" {
		t.Errorf("png => %q", got)
	}
	if got := sniffMediaExtension(write("b", []byte("%PDF-1.4\n..."))); got != ".pdf" {
		t.Errorf("pdf => %q", got)
	}
	if got := sniffMediaExtension(write("c", []byte("hello 纯文本\n"))); got != ".txt" {
		t.Errorf("txt => %q", got)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range []string{"[Content_Types].xml", "word/document.xml"} {
		w, _ := zw.Create(n)
		_, _ = w.Write([]byte("<x/>"))
	}
	_ = zw.Close()
	if got := sniffMediaExtension(write("d", buf.Bytes())); got != ".docx" {
		t.Errorf("docx => %q", got)
	}
}

// TestMediaDownloadRefusesOverwriteAndInfersExt 无扩展名时按内容补扩展名；目标已存在且未加 --overwrite 时拒绝。
func TestMediaDownloadRefusesOverwriteAndInfersExt(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDRfake")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case strings.HasSuffix(r.URL.Path, "/batch_get_tmp_download_url"):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"code":1061004,"msg":"forbidden"}`)
		case strings.HasSuffix(r.URL.Path, "/medias/boxTok/download"):
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)

	dir := t.TempDir()
	out := filepath.Join(dir, "pic")
	reset := func() {
		for _, k := range []string{"output", "overwrite"} {
			f := docMediaDownloadCmd.Flags().Lookup(k)
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		}
	}
	defer reset()
	_ = docMediaDownloadCmd.Flags().Set("output", out)
	captureStdout(t, func() {
		if err := docMediaDownloadCmd.RunE(docMediaDownloadCmd, []string{"boxTok"}); err != nil {
			t.Fatalf("首次下载失败: %v", err)
		}
	})
	data, err := os.ReadFile(out + ".png")
	if err != nil || !bytes.Equal(data, png) {
		t.Fatalf("应保存为 pic.png（按内容补扩展名）: err=%v", err)
	}
	// 再次下载：目标已存在 → 拒绝
	var runErr error
	captureStdout(t, func() { runErr = docMediaDownloadCmd.RunE(docMediaDownloadCmd, []string{"boxTok"}) })
	if runErr == nil || !strings.Contains(runErr.Error(), "已存在") {
		t.Fatalf("目标已存在时应拒绝覆盖，得到 %v", runErr)
	}
	// --overwrite 放行
	_ = docMediaDownloadCmd.Flags().Set("overwrite", "true")
	captureStdout(t, func() { runErr = docMediaDownloadCmd.RunE(docMediaDownloadCmd, []string{"boxTok"}) })
	if runErr != nil {
		t.Fatalf("--overwrite 应允许覆盖: %v", runErr)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".part") {
			t.Fatalf("不应残留临时文件: %s", e.Name())
		}
	}
}

func TestWithMediaDownloadHint403(t *testing.T) {
	err := withMediaDownloadHint(fmt.Errorf("下载失败: HTTP 状态码 403"), "media")
	if !strings.Contains(err.Error(), "--doc-token") {
		t.Fatalf("403 应提示 --doc-token: %v", err)
	}
}
