package client

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadInlineImageBytes_RejectPathTraversal 验证路径遍历被拒绝
// 攻击场景：<img src="../../../../etc/passwd"> 会让 builder 读敏感文件附进 EML 发到外部
func TestLoadInlineImageBytes_RejectPathTraversal(t *testing.T) {
	cases := []string{
		"../etc/passwd",
		"./foo/../../bar.png",
		"a/../../b.png",
		`..\windows\system32\config\sam`,
	}
	for _, p := range cases {
		ref := &MailInlineImageRef{LocalPath: p}
		err := LoadInlineImageBytes(ref)
		if err == nil {
			t.Errorf("路径 %q 应被拒绝但通过了", p)
			continue
		}
		if !strings.Contains(err.Error(), "..") && !strings.Contains(err.Error(), "路径遍历") {
			t.Errorf("路径 %q 的错误信息未提到 `..`/路径遍历: %v", p, err)
		}
	}
}

// TestLoadInlineImageBytes_RejectOutsideSafeRoots 验证 abs 在 cwd/home 之外被拒绝
func TestLoadInlineImageBytes_RejectOutsideSafeRoots(t *testing.T) {
	// /etc/hosts 通常不在 cwd 或 home 子树
	ref := &MailInlineImageRef{LocalPath: "/etc/hosts"}
	err := LoadInlineImageBytes(ref)
	if err == nil {
		t.Fatal("/etc/hosts 应该被 safe-roots 拒绝")
	}
	if !strings.Contains(err.Error(), "当前目录") && !strings.Contains(err.Error(), "home") {
		t.Errorf("错误信息未提到 safe-roots 约束: %v", err)
	}
}

// TestLoadInlineImageBytes_AllowInCwd 验证 cwd 内的合法路径可以读到
func TestLoadInlineImageBytes_AllowInCwd(t *testing.T) {
	dir := t.TempDir()
	// 临时切到 dir 让它作为 cwd
	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	fp := filepath.Join(dir, "x.png")
	if err := os.WriteFile(fp, []byte{0x89, 0x50, 0x4e, 0x47}, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	ref := &MailInlineImageRef{LocalPath: "x.png"}
	if err := LoadInlineImageBytes(ref); err != nil {
		t.Fatalf("合法 cwd 路径被拒绝: %v", err)
	}
	if len(ref.Bytes) != 4 {
		t.Errorf("Bytes 长度异常: %d", len(ref.Bytes))
	}
	if ref.MIME != "image/png" {
		t.Errorf("MIME 未按扩展名兜底: %s", ref.MIME)
	}
}

// TestScanInlineImagePaths_WindowsDrive 验证 Windows 驱动器路径不被 scheme 正则误判
func TestScanInlineImagePaths_WindowsDrive(t *testing.T) {
	html := `<img src="C:\photos\a.png"><img src="d:/x.jpg"><img src="http://cdn/y.png">`
	got := ScanInlineImagePaths(html)
	if len(got) != 2 {
		t.Fatalf("got %d items %v, want 2 (Windows 驱动器路径)", len(got), got)
	}
	if got[0] != `C:\photos\a.png` {
		t.Errorf("[0] got %q, want C:\\photos\\a.png", got[0])
	}
	if got[1] != "d:/x.jpg" {
		t.Errorf("[1] got %q, want d:/x.jpg", got[1])
	}
}
