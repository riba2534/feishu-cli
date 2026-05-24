package client

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestDownloadFileWithToken_RangeFallback(t *testing.T) {
	const (
		fileToken = "boxcn_xxx"
		userToken = "u-test-token"
	)

	oldChunkSize := rangeDownloadChunkSize
	rangeDownloadChunkSize = 4
	defer func() {
		rangeDownloadChunkSize = oldChunkSize
	}()

	wantBody := []byte("large-file-body")
	var capturedRanges []string

	handler := func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			t.Fatal("显式 User Token 下载文件时不应请求 tenant token")
		}
		if got, want := r.URL.Path, "/open-apis/drive/v1/files/"+fileToken+"/download"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if r.Header.Get("Authorization") != "Bearer "+userToken {
			t.Errorf("Authorization header: got %q, want %q", r.Header.Get("Authorization"), "Bearer "+userToken)
		}

		rangeHeader := r.Header.Get("Range")
		if rangeHeader == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"code":%d,"msg":"Downloaded file size exceeds limit"}`, messageResourceFileSizeExceedsLimitCode)
			return
		}

		var start, end int
		if _, err := fmt.Sscanf(rangeHeader, "bytes=%d-%d", &start, &end); err != nil {
			t.Fatalf("Range header 格式非法: %q", rangeHeader)
		}
		if start < 0 || start >= len(wantBody) {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		if end >= len(wantBody) {
			end = len(wantBody) - 1
		}

		capturedRanges = append(capturedRanges, rangeHeader)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(wantBody)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(wantBody[start : end+1])
	}

	_, cleanup := stubFeishuServer(t, handler)
	defer cleanup()

	outputPath := t.TempDir() + "/large.txt"
	if err := DownloadFileWithToken(fileToken, outputPath, userToken); err != nil {
		t.Fatalf("DownloadFileWithToken 返回错误: %v", err)
	}
	gotBody, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("读取下载文件失败: %v", err)
	}
	if string(gotBody) != string(wantBody) {
		t.Fatalf("下载内容 = %q, want %q", gotBody, wantBody)
	}

	wantRanges := []string{"bytes=0-3", "bytes=4-7", "bytes=8-11", "bytes=12-15"}
	if strings.Join(capturedRanges, ",") != strings.Join(wantRanges, ",") {
		t.Fatalf("Range 请求序列 = %v, want %v", capturedRanges, wantRanges)
	}
}
