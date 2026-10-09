package client

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

func stubRemoteImageProbe(t *testing.T, resp *http.Response, respErr error) *http.Request {
	t.Helper()
	origDo, origLookup := remoteImageProbeDo, remoteImageLookupIP
	t.Cleanup(func() { remoteImageProbeDo, remoteImageLookupIP = origDo, origLookup })
	remoteImageLookupIP = func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	}
	captured := &http.Request{}
	remoteImageProbeDo = func(_ *http.Client, req *http.Request) (*http.Response, error) {
		*captured = *req
		if respErr != nil {
			return nil, respErr
		}
		if resp.Body == nil {
			resp.Body = io.NopCloser(strings.NewReader(""))
		}
		return resp, nil
	}
	return captured
}

func probeKind(t *testing.T, err error) (RemoteImageProbeKind, string) {
	t.Helper()
	var probeErr *RemoteImageProbeError
	if !errors.As(err, &probeErr) {
		t.Fatalf("应返回 *RemoteImageProbeError: %T %v", err, err)
	}
	return probeErr.Kind, probeErr.Reason
}

func TestProbeRemoteImageUsesRangedGET(t *testing.T) {
	req := stubRemoteImageProbe(t, &http.Response{StatusCode: http.StatusPartialContent, Header: http.Header{"Content-Type": {"image/png"}}, ContentLength: 1}, nil)
	if err := ProbeRemoteImage(context.Background(), "https://img.example.com/a.png"); err != nil {
		t.Fatalf("探测应通过: %v", err)
	}
	if req.Method != http.MethodGet || req.Header.Get("Range") != "bytes=0-0" || req.Header.Get("Authorization") != "" {
		t.Fatalf("应为不带凭证的 Range GET: method=%s range=%q", req.Method, req.Header.Get("Range"))
	}
}

func TestProbeRemoteImageClassifiesFailures(t *testing.T) {
	tests := []struct {
		name       string
		resp       *http.Response
		respErr    error
		wantKind   RemoteImageProbeKind
		wantReason string
	}{
		{name: "404", resp: &http.Response{StatusCode: 404, Header: http.Header{"Content-Type": {"image/png"}}}, wantKind: RemoteImageUnavailable, wantReason: "HTTP 404"},
		{name: "500", resp: &http.Response{StatusCode: 500, Header: http.Header{}}, wantKind: RemoteImageUnavailable, wantReason: "HTTP 500"},
		{name: "html", resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/html; charset=utf-8"}}}, wantKind: RemoteImageFormat, wantReason: `"text/html"`},
		{name: "bad content type", resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"image/;;"}}}, wantKind: RemoteImageFormat, wantReason: "Content-Type 非法"},
		{name: "too large", resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"image/jpeg"}}, ContentLength: RemoteImageMaxBytes + 1}, wantKind: RemoteImageTooLarge, wantReason: "20MiB"},
		{name: "network", respErr: errors.New("connection reset"), wantKind: RemoteImageUnavailable, wantReason: "可用性探测失败"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubRemoteImageProbe(t, tt.resp, tt.respErr)
			err := ProbeRemoteImage(context.Background(), "https://img.example.com/a.png")
			kind, reason := probeKind(t, err)
			if kind != tt.wantKind || !strings.Contains(reason, tt.wantReason) {
				t.Fatalf("kind=%s reason=%q，期望 %s / %q", kind, reason, tt.wantKind, tt.wantReason)
			}
		})
	}
}

func TestProbeRemoteImageAcceptsMissingContentType(t *testing.T) {
	stubRemoteImageProbe(t, &http.Response{StatusCode: 200, Header: http.Header{}}, nil)
	if err := ProbeRemoteImage(context.Background(), "https://img.example.com/a"); err != nil {
		t.Fatalf("缺少 Content-Type 时视为通过: %v", err)
	}
}

func TestValidateRemoteImageSourceRejectsInternalTargets(t *testing.T) {
	origLookup := remoteImageLookupIP
	t.Cleanup(func() { remoteImageLookupIP = origLookup })
	remoteImageLookupIP = func(_ context.Context, host string) ([]net.IP, error) {
		switch host {
		case "intranet.example.com":
			return []net.IP{net.ParseIP("93.184.216.34"), net.ParseIP("10.1.2.3")}, nil
		case "public.example.com":
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		}
		return nil, errors.New("no such host")
	}
	for _, raw := range []string{
		"http://127.0.0.1/a.png", "http://localhost/a.png", "http://foo.localhost/a.png", "http://[::1]/a.png",
		"http://169.254.169.254/latest", "http://10.0.0.1/a.png", "http://192.168.1.1/a.png", "http://100.64.0.1/a.png",
		"http://198.18.0.1/a.png", "http://0.0.0.0/a.png", "http://240.0.0.1/a.png", "http://[fd00::1]/a.png",
		"https://intranet.example.com/a.png", "https://unknown.example.com/a.png",
		"ftp://public.example.com/a.png", "file:///etc/passwd", "https://user:pass@public.example.com/a.png", "https:///a.png",
	} {
		err := ValidateRemoteImageSource(context.Background(), raw)
		if kind, _ := probeKind(t, err); kind != RemoteImageSourceDisallowed {
			t.Fatalf("%s 应被拒绝为 source_disallowed: %v", raw, err)
		}
	}
	if err := ValidateRemoteImageSource(context.Background(), "https://public.example.com/a.png"); err != nil {
		t.Fatalf("公网地址应放行: %v", err)
	}
	// 内网地址在发起请求前就被拒绝，不调用 HTTP
	origDo := remoteImageProbeDo
	t.Cleanup(func() { remoteImageProbeDo = origDo })
	remoteImageProbeDo = func(*http.Client, *http.Request) (*http.Response, error) {
		t.Fatal("受限地址不应发起请求")
		return nil, nil
	}
	if kind, reason := probeKind(t, ProbeRemoteImage(context.Background(), "http://127.0.0.1/a.png")); kind != RemoteImageSourceDisallowed || reason != "不允许访问本地/内网地址" {
		t.Fatalf("kind=%s reason=%q", kind, reason)
	}
}

func TestRemoteImageProbeRedirectPolicy(t *testing.T) {
	origLookup := remoteImageLookupIP
	t.Cleanup(func() { remoteImageLookupIP = origLookup })
	remoteImageLookupIP = func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	}
	httpClient, closeIdle := newRemoteImageProbeClient(context.Background())
	defer closeIdle()
	mk := func(raw string) *http.Request {
		req, _ := http.NewRequest(http.MethodGet, raw, nil)
		return req
	}
	httpsVia := []*http.Request{mk("https://public.example.com/a.png")}
	if err := httpClient.CheckRedirect(mk("http://public.example.com/b.png"), httpsVia); err == nil {
		t.Fatal("HTTPS→HTTP 降级应被拒绝")
	}
	if err := httpClient.CheckRedirect(mk("https://127.0.0.1/b.png"), httpsVia); err == nil {
		t.Fatal("重定向到内网应被拒绝")
	}
	if err := httpClient.CheckRedirect(mk("https://cdn.example.com/b.png"), httpsVia); err != nil {
		t.Fatalf("重定向到公网应放行: %v", err)
	}
	many := make([]*http.Request, remoteImageProbeMaxRedirects)
	for i := range many {
		many[i] = mk("https://public.example.com/a.png")
	}
	if err := httpClient.CheckRedirect(mk("https://cdn.example.com/b.png"), many); err == nil {
		t.Fatal("超过重定向上限应报错")
	}
}

func TestRemoteImageProbeDialGuardBlocksRestrictedIP(t *testing.T) {
	for _, key := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy"} {
		t.Setenv(key, "")
	}
	httpClient, closeIdle := newRemoteImageProbeClient(context.Background())
	defer closeIdle()
	transport := httpClient.Transport.(*http.Transport)
	// DNS 预校验之后若解析结果变为内网（DNS rebinding），建立连接前再拦一次
	_, err := transport.DialContext(context.Background(), "tcp", "127.0.0.1:9")
	if kind, _ := probeKind(t, err); kind != RemoteImageSourceDisallowed {
		t.Fatalf("连接 127.0.0.1 应被拦截: %v", err)
	}
}
