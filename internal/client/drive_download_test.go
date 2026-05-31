package client

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDownloadFileWithToken_RawHTTP200JSONErrorDoesNotWriteFile(t *testing.T) {
	const (
		fileToken = "boxcn_xxx"
		userToken = "u-test-token"
	)

	handler := func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			t.Fatal("显式 User Token 下载文件时不应请求 tenant token")
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"code":234040,"msg":"The file is invisible to the operator."}`)
	}

	_, cleanup := stubFeishuServer(t, handler)
	defer cleanup()

	outputPath := t.TempDir() + "/error.json"
	err := DownloadFileWithToken(fileToken, outputPath, userToken)
	if err == nil {
		t.Fatal("HTTP 200 JSON 业务错误应返回 error")
	}
	if !strings.Contains(err.Error(), "234040") {
		t.Fatalf("error = %q, want code 234040", err)
	}
	if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
		t.Fatalf("业务错误不应落盘，stat err = %v", statErr)
	}
}

func TestDownloadFileWithToken_RawJSONAttachmentIsSaved(t *testing.T) {
	const (
		fileToken = "boxcn_xxx"
		userToken = "u-test-token"
		body      = `{"code":234040,"msg":"this is normal file content"}`
	)

	handler := func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			t.Fatal("显式 User Token 下载文件时不应请求 tenant token")
		}
		w.Header().Set("Content-Disposition", `attachment; filename="data.json"`)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, body)
	}

	_, cleanup := stubFeishuServer(t, handler)
	defer cleanup()

	outputPath := t.TempDir() + "/data.json"
	if err := DownloadFileWithToken(fileToken, outputPath, userToken); err != nil {
		t.Fatalf("真实 JSON 附件不应被误判为 API 错误: %v", err)
	}
	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("读取输出文件失败: %v", err)
	}
	if string(got) != body {
		t.Fatalf("输出内容 = %q, want %q", string(got), body)
	}
}

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
			w.WriteHeader(http.StatusOK)
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

func TestDownloadBearerURLByRange_TimeoutCoversWholeDownload(t *testing.T) {
	const userToken = "u-test-token"

	oldChunkSize := rangeDownloadChunkSize
	rangeDownloadChunkSize = 1
	defer func() {
		rangeDownloadChunkSize = oldChunkSize
	}()

	body := []byte("abc")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+userToken {
			t.Errorf("Authorization header: got %q, want %q", r.Header.Get("Authorization"), "Bearer "+userToken)
		}
		time.Sleep(70 * time.Millisecond)

		var start, end int
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
			t.Fatalf("Range header 格式非法: %q", r.Header.Get("Range"))
		}
		if end >= len(body) {
			end = len(body) - 1
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(body)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(body[start : end+1])
	}))
	defer srv.Close()

	outputPath := t.TempDir() + "/timeout.bin"
	err := downloadBearerURLByRange("下载文件", srv.URL, outputPath, userToken, 120*time.Millisecond)
	if err == nil {
		t.Fatal("Range 总耗时超过 timeout 时应返回 error")
	}
	if !strings.Contains(err.Error(), "context deadline") {
		t.Fatalf("error = %q, want context deadline exceeded", err)
	}
	if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
		t.Fatalf("Range 超时后不应保留部分文件，stat err = %v", statErr)
	}
}
