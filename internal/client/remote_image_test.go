package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func testPNGBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{G: 255, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestParseRemoteImageURL(t *testing.T) {
	for _, ok := range []string{"https://example.com/a.png", "http://example.com/a?x=1", " https://example.com/a.png "} {
		if _, err := ParseRemoteImageURL(ok); err != nil {
			t.Errorf("%q 应合法: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "ftp://example.com/a.png", "//example.com/a.png", "./a.png", "https://user:pw@example.com/a.png", "https:///a.png", "file:///etc/passwd"} {
		if _, err := ParseRemoteImageURL(bad); err == nil {
			t.Errorf("%q 应被拒绝", bad)
		}
	}
}

func TestValidateRemoteImageURLBlocksInternal(t *testing.T) {
	orig := remoteImageLookupIP
	t.Cleanup(func() { remoteImageLookupIP = orig })
	remoteImageLookupIP = func(_ context.Context, _, host string) ([]net.IP, error) {
		switch host {
		case "public.test":
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		case "mixed.test":
			return []net.IP{net.ParseIP("93.184.216.34"), net.ParseIP("10.1.2.3")}, nil
		}
		return nil, errors.New("no such host")
	}
	if err := ValidateRemoteImageURL(context.Background(), "https://public.test/a.png"); err != nil {
		t.Fatalf("公网地址应放行: %v", err)
	}
	for _, bad := range []string{
		"http://localhost/a.png", "http://a.localhost/a.png", "http://127.0.0.1/a.png", "http://10.0.0.1/a.png",
		"http://172.16.5.4/a.png", "http://192.168.1.1/a.png", "http://169.254.169.254/latest", "http://100.64.0.1/a",
		"http://198.18.0.1/a", "http://0.0.0.0/a", "http://[::1]/a", "http://[fc00::1]/a", "http://[fe80::1]/a",
		"https://mixed.test/a.png", "https://unknown.test/a.png",
	} {
		if err := ValidateRemoteImageURL(context.Background(), bad); err == nil {
			t.Errorf("%s 应被拒绝", bad)
		}
	}
	// 建连时的对端 IP 校验（防 DNS rebinding）
	if err := checkRemoteImageDialAddr("10.0.0.8:443"); !errors.Is(err, errRemoteImageBlocked) {
		t.Fatalf("内网对端应在建连时被拒绝: %v", err)
	}
	if err := checkRemoteImageDialAddr("93.184.216.34:443"); err != nil {
		t.Fatalf("公网对端应放行: %v", err)
	}
}

func TestDownloadRemoteImage(t *testing.T) {
	orig := remoteImageAllowLoopback
	remoteImageAllowLoopback = true
	t.Cleanup(func() { remoteImageAllowLoopback = orig })
	pngData := testPNGBytes(t, 40, 20)

	mux := http.NewServeMux()
	mux.HandleFunc("/ok.png", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png; charset=binary")
		_, _ = w.Write(pngData)
	})
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ok.png", http.StatusFound)
	})
	var loopHits atomic.Int32
	mux.HandleFunc("/loop", func(w http.ResponseWriter, r *http.Request) {
		loopHits.Add(1)
		http.Redirect(w, r, "/loop", http.StatusFound)
	})
	mux.HandleFunc("/to-internal", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://10.0.0.1/a.png", http.StatusFound)
	})
	mux.HandleFunc("/html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html></html>"))
	})
	mux.HandleFunc("/fake-jpeg", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(pngData)
	})
	mux.HandleFunc("/empty", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
	})
	mux.HandleFunc("/huge", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", fmt.Sprint(RemoteImageMaxBytes+1))
		_, _ = w.Write(pngData)
	})
	mux.HandleFunc("/503", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
	mux.HandleFunc("/404", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	for _, path := range []string{"/ok.png", "/redirect"} {
		img, err := DownloadRemoteImage(context.Background(), srv.URL+path)
		if err != nil {
			t.Fatalf("%s 下载失败: %v", path, err)
		}
		if img.FileName != "image.png" || img.Width != 40 || img.Height != 20 || !bytes.Equal(img.Content, pngData) {
			t.Fatalf("%s 结果异常: %+v", path, img)
		}
	}
	cases := []struct {
		path      string
		want      string
		retryable bool
	}{
		{"/loop", "重定向次数超过 5 次", false},
		{"/to-internal", "重定向目标", false},
		{"/html", "不受支持", false},
		{"/fake-jpeg", "实际内容是 png", false},
		{"/empty", "响应为空", false},
		{"/huge", "20MiB", false},
		{"/503", "HTTP 503", true},
		{"/404", "HTTP 404", false},
	}
	for _, tc := range cases {
		_, err := DownloadRemoteImage(context.Background(), srv.URL+tc.path)
		if err == nil || !strings.Contains(err.Error(), tc.want) || IsRetryableRemoteImageError(err) != tc.retryable {
			t.Errorf("%s 期望错误含 %q（retryable=%v），得到 %v", tc.path, tc.want, tc.retryable, err)
		}
	}

	if n := loopHits.Load(); n != remoteImageMaxRedirects {
		t.Fatalf("最多跟随 %d 次请求后停止，实际 %d 次", remoteImageMaxRedirects, n)
	}

	// 不允许 loopback 时，请求前即拒绝
	remoteImageAllowLoopback = false
	if _, err := DownloadRemoteImage(context.Background(), srv.URL+"/ok.png"); err == nil || IsRetryableRemoteImageError(err) {
		t.Fatalf("loopback 地址应被拒绝且不可重试: %v", err)
	}
}
