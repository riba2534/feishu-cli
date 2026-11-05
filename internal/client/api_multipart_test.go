package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

// 传输层失败标为网络错误（退出码 4，可重试）。
func TestDoAPIMultipart_NetworkErrorTagged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()
	setupTestConfig(t, url)
	_, err := DoAPIMultipart(context.Background(), http.MethodPost, "/open-apis/im/v1/files", nil, nil, MultipartFile{Field: "file", FileName: "a.txt", Content: []byte("x")}, "u-test")
	if err == nil || !clierr.HasKind(err, clierr.KindNetwork) {
		t.Fatalf("连接失败应为网络错误: %v", err)
	}
}

// 响应超过上限时显式报错，不静默截断。
func TestDoAPIMultipart_OversizedResponseRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", apiMultipartMaxResponse+10)))
	}))
	defer srv.Close()
	setupTestConfig(t, srv.URL)
	_, err := DoAPIMultipart(context.Background(), http.MethodPost, "/open-apis/im/v1/files", nil, nil, MultipartFile{Field: "file", FileName: "a.txt", Content: []byte("x")}, "u-test")
	if err == nil || !strings.Contains(err.Error(), "上限") {
		t.Fatalf("超限响应应报错: %v", err)
	}
}
