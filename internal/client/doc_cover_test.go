package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

func TestGetAndUpdateDocumentCover(t *testing.T) {
	var mu sync.Mutex
	var patchBodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.Method == http.MethodGet && r.URL.Path == "/open-apis/docx/v1/documents/docWith":
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"document":{"document_id":"docWith","cover":{"token":"boxCover","offset_ratio_x":0.2,"offset_ratio_y":-0.5}}}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/open-apis/docx/v1/documents/docEmpty":
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"document":{"document_id":"docEmpty"}}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/open-apis/docx/v1/documents/docGone":
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"code":1770003,"msg":"document deleted","error":{"log_id":"log123"}}`)
		case r.Method == http.MethodPatch && r.URL.Path == "/open-apis/docx/v1/documents/docWith":
			b, _ := io.ReadAll(r.Body)
			patchBodies = append(patchBodies, string(b))
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{}}`)
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	setupTestConfig(t, server.URL)

	cover, err := GetDocumentCover("docWith", "")
	if err != nil {
		t.Fatalf("GetDocumentCover: %v", err)
	}
	if cover.Token != "boxCover" || cover.OffsetRatioX == nil || *cover.OffsetRatioX != 0.2 || cover.OffsetRatioY == nil || *cover.OffsetRatioY != -0.5 {
		t.Fatalf("cover = %+v", cover)
	}
	empty, err := GetDocumentCover("docEmpty", "")
	if err != nil || empty.Token != "" {
		t.Fatalf("无封面应返回空 token: %+v, %v", empty, err)
	}
	if _, err := GetDocumentCover("docGone", ""); !HasAPICode(err, 1770003) {
		t.Fatalf("应透出业务码 1770003（HTTP 404 也先解析信封），得到 %v", err)
	}

	x := 0.0
	if err := UpdateDocumentCover("docWith", &DocumentCover{Token: "boxNew", OffsetRatioX: &x}, ""); err != nil {
		t.Fatalf("UpdateDocumentCover: %v", err)
	}
	if err := UpdateDocumentCover("docWith", nil, ""); err != nil {
		t.Fatalf("删除封面: %v", err)
	}
	if len(patchBodies) != 2 {
		t.Fatalf("PATCH 次数 = %d", len(patchBodies))
	}
	// 显式 0 偏移必须下发（指针非 nil），未设置的 y 不下发
	if patchBodies[0] != `{"update_cover":{"cover":{"token":"boxNew","offset_ratio_x":0}}}` {
		t.Fatalf("更新请求体 = %s", patchBodies[0])
	}
	// 删除封面必须显式发送 "cover": null（与官方一致），不能省略 cover 字段
	if patchBodies[1] != `{"update_cover":{"cover":null}}` {
		t.Fatalf("删除请求体 = %s", patchBodies[1])
	}
}

func TestOpenMediaPreviewDownload(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\nfake-preview")
	var gotQuery, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.URL.Path == "/open-apis/drive/v1/medias/boxTok/preview_download":
			gotQuery, gotAuth = r.URL.RawQuery, r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		case r.URL.Path == "/open-apis/drive/v1/medias/boxDenied/preview_download":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"code":1061004,"msg":"forbidden","error":{"log_id":"logX"}}`)
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	setupTestConfig(t, server.URL)
	resetTenantTokenCacheForTest()
	t.Cleanup(resetTenantTokenCacheForTest)

	d, err := OpenMediaPreviewDownload("boxTok", "u-user", 0)
	if err != nil {
		t.Fatalf("OpenMediaPreviewDownload: %v", err)
	}
	data, err := io.ReadAll(d)
	d.Close()
	if err != nil || string(data) != string(png) {
		t.Fatalf("预览内容 = %q, %v", data, err)
	}
	if gotQuery != "preview_type=16" {
		t.Fatalf("preview_type 应固定为 16（源文件），得到 %q", gotQuery)
	}
	if gotAuth != "Bearer u-user" {
		t.Fatalf("显式 User Token 应作为 Bearer，得到 %q", gotAuth)
	}
	if d.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("Header 未透出: %v", d.Header())
	}

	// HTTP 200 + JSON 业务错误信封必须报错，不能当成文件内容
	if _, err := OpenMediaPreviewDownload("boxDenied", "", 0); !HasAPICode(err, 1061004) {
		t.Fatalf("应识别 200 + 业务错误信封，得到 %v", err)
	}
}

func TestUploadDocMediaBytes(t *testing.T) {
	payload := "\x89PNG\r\n\x1a\nclipboard-bytes"
	var form map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case "/open-apis/drive/v1/medias/upload_all":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("解析表单失败: %v", err)
			}
			f, _, _ := r.FormFile("file")
			data, _ := io.ReadAll(f)
			form = map[string]string{
				"file_name":   r.FormValue("file_name"),
				"parent_type": r.FormValue("parent_type"),
				"parent_node": r.FormValue("parent_node"),
				"size":        r.FormValue("size"),
				"extra":       r.FormValue("extra"),
				"file":        string(data),
			}
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"file_token":"boxImg"}}`)
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	setupTestConfig(t, server.URL)

	token, err := UploadDocMediaBytes([]byte(payload), "docx_image", "docABC", "clipboard.png", "docABC", "")
	if err != nil || token != "boxImg" {
		t.Fatalf("UploadDocMediaBytes = (%q, %v)", token, err)
	}
	want := map[string]string{
		"file_name": "clipboard.png", "parent_type": "docx_image", "parent_node": "docABC",
		"size": fmt.Sprint(len(payload)), "extra": `{"drive_route_token":"docABC"}`, "file": payload,
	}
	for k, v := range want {
		if form[k] != v {
			t.Fatalf("表单 %s = %q，期望 %q", k, form[k], v)
		}
	}
	if _, err := UploadDocMediaBytes(nil, "docx_image", "doc", "a.png", "doc", ""); err == nil {
		t.Fatal("空内容应报错")
	}
}

// TestUploadDocMediaBytesMultipart 内存素材超过单次上限时同样走分片上传。
func TestUploadDocMediaBytesMultipart(t *testing.T) {
	payload := strings.Repeat("A", 10) + "BB"
	orig := maxSingleUploadSize
	maxSingleUploadSize = len(payload) - 1
	defer func() { maxSingleUploadSize = orig }()

	var parts []string
	var prepare map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case "/open-apis/drive/v1/medias/upload_prepare":
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &prepare)
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"upload_id":"up1","block_size":10,"block_num":2}}`)
		case "/open-apis/drive/v1/medias/upload_part":
			_ = r.ParseMultipartForm(1 << 20)
			f, _, _ := r.FormFile("file")
			data, _ := io.ReadAll(f)
			parts = append(parts, r.FormValue("seq")+":"+string(data))
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{}}`)
		case "/open-apis/drive/v1/medias/upload_finish":
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"file_token":"boxBig"}}`)
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	setupTestConfig(t, server.URL)

	token, err := UploadDocMediaBytes([]byte(payload), "docx_image", "docABC", "cover.png", "docABC", "")
	if err != nil || token != "boxBig" {
		t.Fatalf("UploadDocMediaBytes = (%q, %v)", token, err)
	}
	if prepare["extra"] != `{"drive_route_token":"docABC"}` || prepare["parent_node"] != "docABC" {
		t.Fatalf("prepare = %#v", prepare)
	}
	if strings.Join(parts, "|") != "0:AAAAAAAAAA|1:BB" {
		t.Fatalf("分片 = %v", parts)
	}
}

func TestIsUnsafeCoverIP(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1":       true,
		"10.1.2.3":        true,
		"172.16.0.1":      true,
		"172.31.255.255":  true,
		"192.168.1.1":     true,
		"169.254.169.254": true,
		"100.64.0.1":      true,
		"198.18.0.1":      true,
		"0.1.2.3":         true,
		"240.0.0.1":       true,
		"224.0.0.1":       true,
		"::1":             true,
		"fc00::1":         true,
		"fe80::1":         true,
		"8.8.8.8":         false,
		"172.32.0.1":      false,
		"100.128.0.1":     false,
		"2001:4860::8888": false,
	}
	for ip, want := range cases {
		if got := isUnsafeCoverIP(net.ParseIP(ip)); got != want {
			t.Errorf("isUnsafeCoverIP(%s) = %v，期望 %v", ip, got, want)
		}
	}
}

func TestValidateCoverImageURL(t *testing.T) {
	bad := []string{"http://example.com/a.png", "https://user:pass@example.com/a.png", "https:///a.png", "ftp://example.com/a.png", "://bad"}
	for _, raw := range bad {
		if err := ValidateCoverImageURL(raw); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
			t.Errorf("%q 应为用法错误，得到 %v", raw, err)
		}
	}
	if err := ValidateCoverImageURL("https://example.com/a.png"); err != nil {
		t.Fatalf("合法 https URL 不应报错: %v", err)
	}
}

// stubCoverURLNet 让测试服务器（127.0.0.1、自签证书）可被访问：受限 IP 判定只拦截 blocked 中的地址，
// hosts 为请求前 DNS 校验使用的虚构解析结果。
func stubCoverURLNet(t *testing.T, server *httptest.Server, hosts map[string][]string, blocked ...string) {
	t.Helper()
	oldLookup, oldUnsafe, oldTLS := coverURLLookupIP, coverURLUnsafeIP, coverURLTLSConfig
	t.Cleanup(func() { coverURLLookupIP, coverURLUnsafeIP, coverURLTLSConfig = oldLookup, oldUnsafe, oldTLS })
	coverURLLookupIP = func(ctx context.Context, host string) ([]net.IP, error) {
		var ips []net.IP
		for _, ip := range hosts[host] {
			ips = append(ips, net.ParseIP(ip))
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("no such host %s", host)
		}
		return ips, nil
	}
	coverURLUnsafeIP = func(ip net.IP) bool {
		for _, b := range blocked {
			if ip.Equal(net.ParseIP(b)) {
				return true
			}
		}
		return false
	}
	// httptest 证书只签发给 example.com / 127.0.0.1：固定校验名，避免依赖测试里的地址写法
	tlsCfg := server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	tlsCfg.ServerName = "example.com"
	coverURLTLSConfig = tlsCfg
}

func TestFetchCoverImageURL(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\ncover")
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		port := srv.URL[strings.LastIndex(srv.URL, ":")+1:]
		switch r.URL.Path {
		case "/img/logo":
			w.Header().Set("Content-Type", "image/png; charset=binary")
			_, _ = w.Write(png)
		case "/img/photo.jpeg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte{0xff, 0xd8, 0xff})
		case "/redirect":
			http.Redirect(w, r, "https://127.0.0.1:"+port+"/img/logo", http.StatusFound)
		case "/redirect-internal":
			http.Redirect(w, r, "https://127.0.0.2:"+port+"/img/logo", http.StatusFound)
		case "/redirect-internal-host":
			http.Redirect(w, r, "https://internal.example.test:"+port+"/img/logo", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		case "/svg":
			w.Header().Set("Content-Type", "image/svg+xml")
			fmt.Fprint(w, "<svg/>")
		case "/html":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, "<html/>")
		case "/big":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(make([]byte, CoverURLMaxBytes+1))
		case "/boom":
			w.WriteHeader(http.StatusBadGateway)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	port := srv.URL[strings.LastIndex(srv.URL, ":")+1:]
	base := "https://127.0.0.1:" + port
	stubCoverURLNet(t, srv, map[string][]string{"internal.example.test": {"10.0.0.8"}}, "127.0.0.2", "10.0.0.8")

	img, err := FetchCoverImageURL(base + "/img/logo")
	if err != nil {
		t.Fatalf("FetchCoverImageURL: %v", err)
	}
	if string(img.Data) != string(png) || img.FileName != "logo.png" || img.ContentType != "image/png" {
		t.Fatalf("结果 = %+v（文件名应按 Content-Type 补 .png）", img)
	}
	if img, err := FetchCoverImageURL(base + "/img/photo.jpeg"); err != nil || img.FileName != "photo.jpeg" {
		t.Fatalf("已有扩展名不应改动: %+v, %v", img, err)
	}
	if img, err := FetchCoverImageURL(base + "/redirect"); err != nil || img.FileName != "logo.png" {
		t.Fatalf("合法跳转应跟随: %+v, %v", img, err)
	}

	cases := []struct {
		path string
		kind clierr.Kind
		want string
	}{
		{"/redirect-internal", clierr.KindUsage, "内网地址"},
		{"/redirect-internal-host", clierr.KindUsage, "10.0.0.8"},
		{"/loop", clierr.KindUsage, "重定向超过 3 次"},
		{"/svg", clierr.KindUsage, "image/svg+xml"},
		{"/html", clierr.KindUsage, "text/html"},
		{"/big", clierr.KindUsage, "20MiB"},
		{"/boom", clierr.KindNetwork, "HTTP 502"},
	}
	for _, c := range cases {
		_, err := FetchCoverImageURL(base + c.path)
		if err == nil || !strings.Contains(err.Error(), c.want) || !clierr.HasKind(err, c.kind) {
			t.Errorf("%s: 期望 %v 且含 %q，得到 %v (kinds=%v)", c.path, c.kind, c.want, err, clierr.Kinds(err))
		}
	}
	// 404 不是可重试的网络错误
	if _, err := FetchCoverImageURL(base + "/missing"); err == nil || !strings.Contains(err.Error(), "HTTP 404") || clierr.HasKind(err, clierr.KindNetwork) {
		t.Errorf("404 应为一般错误，得到 %v", err)
	}
	// 主机名解析到受限地址：请求前拒绝
	if _, err := FetchCoverImageURL("https://internal.example.test:" + port + "/img/logo"); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Errorf("内网主机应在请求前拒绝，得到 %v", err)
	}
}

// TestValidateCoverURLHostAnyUnsafe 任一解析结果落在受限网段即拒绝；解析失败归为网络错误。
func TestValidateCoverURLHostAnyUnsafe(t *testing.T) {
	old := coverURLLookupIP
	t.Cleanup(func() { coverURLLookupIP = old })
	coverURLLookupIP = func(ctx context.Context, host string) ([]net.IP, error) {
		switch host {
		case "mixed.example.test":
			return []net.IP{net.ParseIP("93.184.216.34"), net.ParseIP("192.168.1.10")}, nil
		case "public.example.test":
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		}
		return nil, fmt.Errorf("no such host %s", host)
	}
	ctx := context.Background()
	if err := validateCoverURLHost(ctx, "mixed.example.test"); err == nil || !strings.Contains(err.Error(), "192.168.1.10") {
		t.Fatalf("混合解析结果应拒绝，得到 %v", err)
	}
	if err := validateCoverURLHost(ctx, "public.example.test"); err != nil {
		t.Fatalf("公网地址应放行: %v", err)
	}
	if err := validateCoverURLHost(ctx, "nx.example.test"); err == nil || !clierr.HasKind(err, clierr.KindNetwork) {
		t.Fatalf("解析失败应为网络错误，得到 %v", err)
	}
}

// TestFetchCoverImageURLDefaultGuard 不注入判定时，真实的受限 IP 规则会拒绝回环、链路本地与 localhost。
func TestFetchCoverImageURLDefaultGuard(t *testing.T) {
	for _, raw := range []string{"https://127.0.0.1/a.png", "https://localhost/a.png", "https://[::1]/a.png", "https://169.254.169.254/latest"} {
		if _, err := FetchCoverImageURL(raw); err == nil || !clierr.HasKind(err, clierr.KindUsage) || !strings.Contains(err.Error(), "内网地址") {
			t.Errorf("%s 应被拒绝，得到 %v", raw, err)
		}
	}
}

// TestCoverURLDialControlBlocksRebinding 请求前校验通过、但拨号时实际地址受限（DNS 被替换）仍会被拦截。
func TestCoverURLDialControlBlocksRebinding(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("\x89PNG\r\n\x1a\n"))
	}))
	defer srv.Close()
	stubCoverURLNet(t, srv, nil)
	calls := 0
	coverURLUnsafeIP = func(ip net.IP) bool {
		calls++
		return calls > 1 // 第一次（请求前校验）放行，之后（拨号阶段）视为受限
	}
	_, err := FetchCoverImageURL(srv.URL + "/a.png")
	if err == nil || !clierr.HasKind(err, clierr.KindUsage) || !strings.Contains(err.Error(), "实际连接的地址 127.0.0.1") {
		t.Fatalf("拨号阶段应拦截受限地址，得到 %v", err)
	}
}

func TestCoverURLDialControl(t *testing.T) {
	control := coverURLDialControl(map[string]struct{}{"127.0.0.1:7890": {}})
	if err := control("tcp", "127.0.0.1:7890", nil); err != nil {
		t.Fatalf("代理地址应放行: %v", err)
	}
	if err := control("tcp", "127.0.0.1:443", nil); err == nil {
		t.Fatal("回环地址应拦截")
	}
	if err := control("tcp6", "[fd00::1]:443", nil); err == nil {
		t.Fatal("IPv6 私有地址应拦截")
	}
	if err := control("tcp", "93.184.216.34:443", nil); err != nil {
		t.Fatalf("公网地址应放行: %v", err)
	}
}

func TestEnvProxyDialAddrs(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://10.1.2.3:3128")
	t.Setenv("https_proxy", "")
	t.Setenv("HTTP_PROXY", "socks5://10.0.0.1")
	t.Setenv("http_proxy", "[::1]:8080")
	got := envProxyDialAddrs()
	for _, want := range []string{"10.1.2.3:3128", "10.0.0.1:1080", "[::1]:8080"} {
		if _, ok := got[want]; !ok {
			t.Errorf("缺少代理地址 %s: %v", want, got)
		}
	}
}

func TestCoverURLFileName(t *testing.T) {
	cases := map[string]string{
		"https://a.test/":                   "cover.png",
		"https://a.test/path/%E5%B0%81.png": "封.png",
		"https://a.test/x/..%2F..%2Fetc":    "etc.png",
		"https://a.test/logo":               "logo.png",
	}
	for raw, want := range cases {
		u, _ := url.Parse(raw)
		if got := coverURLFileName(u, ".png"); got != want {
			t.Errorf("coverURLFileName(%s) = %q，期望 %q", raw, got, want)
		}
	}
}
