package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testClient(srv *httptest.Server) *Client {
	return &Client{HTTP: srv.Client(), WebBase: srv.URL, APIBase: srv.URL + "/api", Repo: DefaultRepo, UserAgent: "test-agent"}
}

func TestLatestVersionViaRedirect(t *testing.T) {
	var sawUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawUA = r.Header.Get("User-Agent")
		if r.Method == http.MethodHead && r.URL.Path == "/riba2534/feishu-cli/releases/latest" {
			w.Header().Set("Location", "https://github.com/riba2534/feishu-cli/releases/tag/v1.42.0")
			w.WriteHeader(http.StatusFound)
			return
		}
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusTeapot)
	}))
	defer srv.Close()
	got, err := testClient(srv).LatestVersion(context.Background())
	if err != nil || got != "v1.42.0" {
		t.Fatalf("LatestVersion = %q, %v", got, err)
	}
	if sawUA != "test-agent" {
		t.Fatalf("User-Agent = %q", sawUA)
	}
}

func TestLatestVersionFallsBackToAPI(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/riba2534/feishu-cli/releases/latest":
			w.WriteHeader(http.StatusOK) // 没有跳转
		case "/api/repos/riba2534/feishu-cli/releases/latest":
			auth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"tag_name":"v1.43.0"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := testClient(srv)
	c.Token = "ghp_test_placeholder"
	got, err := c.LatestVersion(context.Background())
	if err != nil || got != "v1.43.0" {
		t.Fatalf("LatestVersion = %q, %v", got, err)
	}
	if auth != "Bearer ghp_test_placeholder" {
		t.Fatalf("API fallback should send token, got %q", auth)
	}
}

func TestLatestVersionErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Location", "https://github.com/riba2534/feishu-cli/releases/tag/<script>")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()
	_, err := testClient(srv).LatestVersion(context.Background())
	if err == nil || !strings.Contains(err.Error(), "版本号格式异常") || !strings.Contains(err.Error(), "限流") {
		t.Fatalf("err = %v", err)
	}
}

func TestTagFromLocation(t *testing.T) {
	cases := map[string]string{
		"https://github.com/o/r/releases/tag/v1.2.3":         "v1.2.3",
		"/o/r/releases/tag/v1.2.3-rc.1?x=1":                  "v1.2.3-rc.1",
		"https://github.com/o/r/releases/tag/v1.2.3/extra":   "v1.2.3",
		"https://github.com/o/r/releases/tag/v1.2.3%2Dbeta1": "v1.2.3-beta1",
	}
	for loc, want := range cases {
		if got, err := tagFromLocation(loc); err != nil || got != want {
			t.Errorf("tagFromLocation(%q) = %q, %v", loc, got, err)
		}
	}
	for _, bad := range []string{"", "https://github.com/o/r/releases", "https://github.com/o/r/releases/tag/latest"} {
		if _, err := tagFromLocation(bad); err == nil {
			t.Errorf("tagFromLocation(%q) should fail", bad)
		}
	}
}

func TestAssetNameAndBinaryName(t *testing.T) {
	got, err := AssetName("v1.2.3", "linux", "amd64")
	if err != nil || got != "feishu-cli_v1.2.3_linux-amd64.tar.gz" {
		t.Fatalf("AssetName = %q, %v", got, err)
	}
	if _, err := AssetName("v1.2.3", "windows", "arm64"); err == nil {
		t.Fatal("unsupported platform should fail")
	}
	if BinaryName("windows") != "feishu-cli.exe" || BinaryName("darwin") != "feishu-cli" {
		t.Fatal("BinaryName mismatch")
	}
	c := &Client{WebBase: "https://github.com", Repo: DefaultRepo}
	if u := c.DownloadURL("v1.2.3", "checksums.txt"); u != "https://github.com/riba2534/feishu-cli/releases/download/v1.2.3/checksums.txt" {
		t.Fatalf("DownloadURL = %s", u)
	}
}

func TestParseChecksums(t *testing.T) {
	h := strings.Repeat("a", 64)
	data := h + "  feishu-cli_v1_linux-amd64.tar.gz\n" + strings.Repeat("B", 64) + " *feishu-cli_v1_windows-amd64.tar.gz\nnot a line\nzz  bad\n"
	sums := ParseChecksums([]byte(data))
	if len(sums) != 2 || sums["feishu-cli_v1_linux-amd64.tar.gz"] != h || sums["feishu-cli_v1_windows-amd64.tar.gz"] != strings.Repeat("b", 64) {
		t.Fatalf("sums = %v", sums)
	}
}

func makeTarGz(t *testing.T, entries map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range entries {
		hdr := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if strings.HasSuffix(name, "/") {
			hdr = &tar.Header{Name: name, Mode: 0o755, Typeflag: tar.TypeDir}
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if hdr.Typeflag == tar.TypeReg {
			_, _ = tw.Write([]byte(body))
		}
	}
	_ = tw.Close()
	_ = gz.Close()
	p := filepath.Join(t.TempDir(), "pkg.tar.gz")
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExtractBinary(t *testing.T) {
	archive := makeTarGz(t, map[string]string{
		"feishu-cli_v1_linux-amd64/":           "",
		"../feishu-cli":                        "evil",
		"a/b/feishu-cli":                       "too deep",
		"feishu-cli_v1_linux-amd64/feishu-cli": "real-binary",
	})
	var out bytes.Buffer
	if err := ExtractBinary(archive, "feishu-cli", &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "real-binary" {
		t.Fatalf("extracted %q", out.String())
	}
	if err := ExtractBinary(archive, "feishu-cli.exe", &out); err == nil {
		t.Fatal("missing binary should fail")
	}
	bad := filepath.Join(t.TempDir(), "bad.tar.gz")
	_ = os.WriteFile(bad, []byte("not gzip"), 0o644)
	if err := ExtractBinary(bad, "feishu-cli", &out); err == nil {
		t.Fatal("invalid gzip should fail")
	}
}

func TestParseAndCompareVersions(t *testing.T) {
	parse := func(s string) Version {
		v, ok := ParseVersion(s)
		if !ok {
			t.Fatalf("ParseVersion(%q) failed", s)
		}
		return v
	}
	if v := parse("v1.41.0-12-g745592c-dirty"); !v.Dev || v.Minor != 41 {
		t.Fatalf("git describe = %+v", v)
	}
	if v := parse("1.2.3-rc.1"); v.Dev || v.Pre != "rc.1" {
		t.Fatalf("prerelease = %+v", v)
	}
	for _, bad := range []string{"dev", "", "v1.2", "latest"} {
		if _, ok := ParseVersion(bad); ok {
			t.Errorf("ParseVersion(%q) should fail", bad)
		}
	}
	cases := []struct {
		a, b string
		want int
	}{
		{"v1.40.0", "v1.41.0", -1},
		{"v1.41.0", "v1.41.0", 0},
		{"v1.41.0", "1.41.0", 0},
		{"v2.0.0", "v1.99.99", 1},
		{"v1.41.0-rc1", "v1.41.0", -1},
		{"v1.41.0-3-gabc1234", "v1.41.0", 1},
		{"v1.41.0-dirty", "v1.41.0", 1},
		{"v1.40.9-3-gabc1234", "v1.41.0", -1},
	}
	for _, c := range cases {
		if got := Compare(parse(c.a), parse(c.b)); got != c.want {
			t.Errorf("Compare(%s,%s) = %d want %d", c.a, c.b, got, c.want)
		}
	}
	if ExtractVersion("feishu-cli version v1.41.0 (built 2026-09-22_10:35:00)") != "v1.41.0" {
		t.Fatal("ExtractVersion failed")
	}
	if !SameVersion("v1.2.3", "1.2.3") || SameVersion("v1.2.3", "v1.2.4") {
		t.Fatal("SameVersion mismatch")
	}
}

func TestReplaceExecutable(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "feishu-cli")
	staged := filepath.Join(dir, ".staged")
	_ = os.WriteFile(target, []byte("old"), 0o755)
	_ = os.WriteFile(staged, []byte("new"), 0o755)
	if old, err := ReplaceExecutable(target, staged, "linux"); err != nil || old != "" {
		t.Fatalf("replace = %q %v", old, err)
	}
	if b, _ := os.ReadFile(target); string(b) != "new" {
		t.Fatalf("target = %q", b)
	}

	// Windows 路径：旧文件改名为 .old
	_ = os.WriteFile(staged, []byte("newer"), 0o755)
	old, err := ReplaceExecutable(target, staged, "windows")
	if err != nil || old != target+".old" {
		t.Fatalf("windows replace = %q %v", old, err)
	}
	if b, _ := os.ReadFile(old); string(b) != "new" {
		t.Fatalf("old = %q", b)
	}
	if b, _ := os.ReadFile(target); string(b) != "newer" {
		t.Fatalf("target = %q", b)
	}

	other := filepath.Join(t.TempDir(), "x")
	_ = os.WriteFile(other, []byte("x"), 0o755)
	if _, err := ReplaceExecutable(target, other, "linux"); err == nil {
		t.Fatal("cross-directory replace must be rejected")
	}
}
