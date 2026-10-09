package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/v2/internal/selfupdate"
)

// fakeRelease 模拟 GitHub release：302 跳转 + checksums.txt + 规范命名的 tar.gz。
type fakeRelease struct {
	tag          string
	archive      []byte
	checksums    string
	noChecksums  bool
	downloadHits int
}

func (f *fakeRelease) handler(t *testing.T) http.Handler {
	asset := fmt.Sprintf("feishu-cli_%s_%s-%s.tar.gz", f.tag, runtime.GOOS, runtime.GOARCH)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/riba2534/feishu-cli/releases/latest":
			w.Header().Set("Location", "/riba2534/feishu-cli/releases/tag/"+f.tag)
			w.WriteHeader(http.StatusFound)
		case "/riba2534/feishu-cli/releases/download/" + f.tag + "/checksums.txt":
			if f.noChecksums {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(f.checksums))
		case "/riba2534/feishu-cli/releases/download/" + f.tag + "/" + asset:
			f.downloadHits++
			_, _ = w.Write(f.archive)
		default:
			t.Logf("unexpected request %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func fakeBinaryScript(version string) string {
	return "#!/bin/sh\necho \"feishu-cli version " + version + " (built test)\"\n"
}

func newFakeRelease(t *testing.T, tag, binary string) *fakeRelease {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	dir := fmt.Sprintf("feishu-cli_%s_%s-%s", tag, runtime.GOOS, runtime.GOARCH)
	_ = tw.WriteHeader(&tar.Header{Name: dir + "/", Mode: 0o755, Typeflag: tar.TypeDir})
	_ = tw.WriteHeader(&tar.Header{Name: dir + "/feishu-cli", Mode: 0o755, Size: int64(len(binary)), Typeflag: tar.TypeReg})
	_, _ = tw.Write([]byte(binary))
	_ = tw.Close()
	_ = gz.Close()
	sum := sha256.Sum256(buf.Bytes())
	return &fakeRelease{
		tag:       tag,
		archive:   buf.Bytes(),
		checksums: fmt.Sprintf("%s  %s.tar.gz\n", hex.EncodeToString(sum[:]), dir),
	}
}

func useFakeRelease(t *testing.T, rel *fakeRelease) {
	t.Helper()
	srv := httptest.NewServer(rel.handler(t))
	t.Cleanup(srv.Close)
	prev := newUpdateClient
	newUpdateClient = func() *selfupdate.Client {
		return &selfupdate.Client{HTTP: srv.Client(), WebBase: srv.URL, APIBase: srv.URL + "/api", Repo: selfupdate.DefaultRepo}
	}
	t.Cleanup(func() { newUpdateClient = prev })
}

func writeFakeTarget(t *testing.T, version string) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "bin", "feishu-cli")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(fakeBinaryScript(version)), 0o755); err != nil {
		t.Fatal(err)
	}
	return target
}

func TestUpdateReplacesTargetBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("测试用 shell 脚本模拟二进制")
	}
	if _, err := selfupdate.AssetName("v9.9.9", runtime.GOOS, runtime.GOARCH); err != nil {
		t.Skip(err)
	}
	newBinary := fakeBinaryScript("v9.9.9")
	rel := newFakeRelease(t, "v9.9.9", newBinary)
	useFakeRelease(t, rel)
	target := writeFakeTarget(t, "v1.0.0")

	out, err := runSkillsCLI(t, "update", "--check", "--json", "--target", target)
	if err != nil {
		t.Fatal(err)
	}
	var st updateStatus
	if err := json.Unmarshal([]byte(out), &st); err != nil || st.CurrentVersion != "v1.0.0" || st.LatestVersion != "v9.9.9" || !st.UpdateAvailable {
		t.Fatalf("--check = %s (%v)", out, err)
	}

	out, err = runSkillsCLI(t, "update", "--dry-run", "--target", target)
	if err != nil || !strings.Contains(out, "[dry-run]") || rel.downloadHits != 0 {
		t.Fatalf("--dry-run = %v hits=%d\n%s", err, rel.downloadHits, out)
	}
	if b, _ := os.ReadFile(target); string(b) != fakeBinaryScript("v1.0.0") {
		t.Fatal("dry-run must not touch target")
	}

	out, err = runSkillsCLI(t, "update", "--target", target)
	if err != nil {
		t.Fatalf("update: %v\n%s", err, out)
	}
	if !strings.Contains(out, "v1.0.0 → v9.9.9") || !strings.Contains(out, "skills install") {
		t.Fatalf("update 输出不对:\n%s", out)
	}
	if b, _ := os.ReadFile(target); string(b) != newBinary {
		t.Fatalf("target not replaced: %q", b)
	}
	if info, _ := os.Stat(target); info.Mode().Perm()&0o100 == 0 {
		t.Fatal("replaced binary must be executable")
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(target), ".feishu-cli*"))
	if len(leftovers) != 0 {
		t.Fatalf("staging files leaked: %v", leftovers)
	}

	// 已是最新：不下载
	hits := rel.downloadHits
	out, err = runSkillsCLI(t, "update", "--target", target)
	if err != nil || !strings.Contains(out, "已是最新版本") || rel.downloadHits != hits {
		t.Fatalf("up-to-date = %v\n%s", err, out)
	}
}

func TestUpdateRejectsBadChecksumAndMissingChecksums(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("测试用 shell 脚本模拟二进制")
	}
	if _, err := selfupdate.AssetName("v9.9.9", runtime.GOOS, runtime.GOARCH); err != nil {
		t.Skip(err)
	}
	rel := newFakeRelease(t, "v9.9.9", fakeBinaryScript("v9.9.9"))
	rel.checksums = strings.Repeat("0", 64) + "  " + fmt.Sprintf("feishu-cli_v9.9.9_%s-%s.tar.gz\n", runtime.GOOS, runtime.GOARCH)
	useFakeRelease(t, rel)
	target := writeFakeTarget(t, "v1.0.0")
	if _, err := runSkillsCLI(t, "update", "--target", target); err == nil || !strings.Contains(err.Error(), "sha256 不匹配") {
		t.Fatalf("bad checksum err = %v", err)
	}
	if b, _ := os.ReadFile(target); string(b) != fakeBinaryScript("v1.0.0") {
		t.Fatal("target must stay untouched on checksum failure")
	}

	rel.noChecksums = true
	if _, err := runSkillsCLI(t, "update", "--target", target); err == nil || !strings.Contains(err.Error(), "checksums.txt") {
		t.Fatalf("missing checksums err = %v", err)
	}
}

func TestUpdateRejectsWrongVersionBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("测试用 shell 脚本模拟二进制")
	}
	if _, err := selfupdate.AssetName("v9.9.9", runtime.GOOS, runtime.GOARCH); err != nil {
		t.Skip(err)
	}
	// 安装包里实际是 v8.0.0：自检必须拦截，不能替换
	useFakeRelease(t, newFakeRelease(t, "v9.9.9", fakeBinaryScript("v8.0.0")))
	target := writeFakeTarget(t, "v1.0.0")
	if _, err := runSkillsCLI(t, "update", "--target", target); err == nil || !strings.Contains(err.Error(), "不一致") {
		t.Fatalf("err = %v", err)
	}
	if b, _ := os.ReadFile(target); string(b) != fakeBinaryScript("v1.0.0") {
		t.Fatal("target must stay untouched when self-check fails")
	}
}

func TestUpdateDevBuildRequiresForce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("测试用 shell 脚本模拟二进制")
	}
	useFakeRelease(t, newFakeRelease(t, "v9.9.9", fakeBinaryScript("v9.9.9")))
	target := writeFakeTarget(t, "v1.0.0")
	if err := os.WriteFile(target, []byte("#!/bin/sh\necho 'feishu-cli version dev (built unknown)'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// dev 构建的 --version 输出没有版本号 → --target 无法识别，直接报错
	if _, err := runSkillsCLI(t, "update", "--target", target); err == nil {
		t.Fatal("unrecognizable target version should fail")
	}
	if _, err := runSkillsCLI(t, "update", "--check", "--dry-run"); err == nil {
		t.Fatal("--check 与 --dry-run 互斥")
	}
}

func TestUpdateSkipsConfigInit(t *testing.T) {
	if !shouldSkipConfigInit(updateCmd) {
		t.Fatal("update 不应依赖 config.yaml")
	}
}
