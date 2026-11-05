package client

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 传输层失败保留 net.OpError 错误链，交给 exitCodeFor 归为网络错误（退出码 4）。
func TestDoAPIMultipart_NetworkErrorTagged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()
	setupTestConfig(t, url)
	_, err := DoAPIMultipart(context.Background(), http.MethodPost, "/open-apis/im/v1/files", nil, nil, MultipartFile{Field: "file", FileName: "a.txt", Content: []byte("x")}, "u-test")
	var opErr *net.OpError
	if err == nil || !errors.As(err, &opErr) {
		t.Fatalf("连接失败应保留 net.OpError: %v", err)
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
