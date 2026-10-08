package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestValidateOutputPathSecurity 回归：过去按"包含 .."判断会误拒 report..v2.json，
// 且 -o ~/.ssh/x.json 会被放行；allowedDir 前缀判断无分隔符边界。
func TestValidateOutputPathSecurity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("系统目录规则仅适用于类 Unix")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	allowed := t.TempDir()

	if err := validateOutputPath("report..v2.json", ""); err != nil {
		t.Errorf("report..v2.json 应放行: %v", err)
	}
	if err := validateOutputPath(filepath.Join(home, ".ssh", "x.json"), ""); err == nil {
		t.Error("-o ~/.ssh/x.json 应被拒绝")
	} else if got := exitCodeFor(err); got != 2 {
		t.Errorf("路径校验失败退出码 = %d, want 2", got)
	}
	if err := validateOutputPath(filepath.Join(allowed, "a.md"), allowed); err != nil {
		t.Errorf("目录内路径应放行: %v", err)
	}
	if err := validateOutputPath(allowed+"-evil/a.md", allowed); err == nil {
		t.Error("同前缀兄弟目录应被拒绝（前缀判断需带分隔符边界）")
	}
}

// TestAPIDataFileRejectsSensitivePaths 回归：`api --data-file /etc/hostname` 过去可读，
// 可能把本机凭证当请求体发往远端。
func TestAPIDataFileRejectsSensitivePaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("系统目录规则仅适用于类 Unix")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	cred := filepath.Join(home, ".aws", "credentials")
	if err := os.MkdirAll(filepath.Dir(cred), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cred, []byte(`{"secret":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/etc/hostname", cred} {
		_, err := loadAPIBody("", p)
		if err == nil || !strings.Contains(err.Error(), "敏感目录") {
			t.Errorf("--data-file %s 应被拒绝，得到 %v", p, err)
		}
	}
	ok := filepath.Join(t.TempDir(), "body.json")
	if err := os.WriteFile(ok, []byte(`{"a":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if body, err := loadAPIBody("", ok); err != nil || string(body) != `{"a":1}` {
		t.Errorf("普通文件应可读: body=%q err=%v", body, err)
	}
}
