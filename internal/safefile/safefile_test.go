package safefile

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

func setupHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, d := range []string{".ssh", ".aws", ".feishu-cli", ".config/gcloud"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func TestValidateOutputPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("系统目录规则仅适用于类 Unix")
	}
	home := setupHome(t)
	work := t.TempDir()
	link := filepath.Join(work, "innocent")
	if err := os.Symlink(filepath.Join(home, ".ssh"), link); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		path    string
		wantErr string // 空表示应放行
	}{
		{name: "文件名含双点放行", path: "report..v2.json"},
		{name: "目录名以双点开头放行", path: "a/..b/c.json"},
		{name: "绝对路径 /tmp 放行", path: filepath.Join(os.TempDir(), "out.json")},
		{name: "绝对路径内 .. 经 Clean 后放行", path: filepath.Join(work, "a", "..", "b.json")},
		{name: "相近名字不误伤（分隔符边界）", path: filepath.Join(home, ".sshx", "a.json")},
		{name: "/etcetera 不误伤", path: "/etcetera/x.json"},
		{name: "/dev/null 放行", path: "/dev/null"},
		{name: "/dev/stdout 放行", path: "/dev/stdout"},
		{name: "相对路径越出当前目录", path: "../x.json", wantErr: "越出当前目录"},
		{name: "多级相对越界", path: "./dir/../../../root", wantErr: "越出当前目录"},
		{name: "~/.ssh 拒绝", path: filepath.Join(home, ".ssh", "x.json"), wantErr: "~/.ssh"},
		{name: "~/.ssh 目录本身拒绝", path: filepath.Join(home, ".ssh"), wantErr: "~/.ssh"},
		{name: "~/.feishu-cli 拒绝", path: filepath.Join(home, ".feishu-cli", "token.json"), wantErr: "~/.feishu-cli"},
		{name: "~/.config/gcloud 拒绝", path: filepath.Join(home, ".config", "gcloud", "x"), wantErr: "~/.config/gcloud"},
		{name: "尚不存在的 ~/.aws 子路径拒绝", path: filepath.Join(home, ".aws", "new", "credentials"), wantErr: "~/.aws"},
		{name: "符号链接指向 ~/.ssh 拒绝", path: filepath.Join(link, "authorized_keys"), wantErr: "~/.ssh"},
		{name: "/etc 拒绝", path: "/etc/x.json", wantErr: "/etc"},
		{name: "/proc 拒绝", path: "/proc/self/environ", wantErr: "/proc"},
		{name: "/dev 块设备拒绝", path: "/dev/sda", wantErr: "/dev"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateOutputPath(tc.path)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("应放行 %q，得到 %v", tc.path, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("应拒绝 %q 且提示含 %q，得到 %v", tc.path, tc.wantErr, err)
			}
			if !clierr.HasKind(err, clierr.KindUsage) {
				t.Fatalf("路径校验失败应是用法错误（退出码 2）: %v", err)
			}
		})
	}
}

func TestValidateInputPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("系统目录规则仅适用于类 Unix")
	}
	home := setupHome(t)
	ok := filepath.Join(t.TempDir(), "body.json")
	if err := os.WriteFile(ok, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateInputPath(ok); err != nil {
		t.Fatalf("普通文件应放行: %v", err)
	}
	for _, p := range []string{"/etc/hostname", filepath.Join(home, ".aws", "credentials"), filepath.Join(home, ".feishu-cli", "config.yaml")} {
		if err := ValidateInputPath(p); err == nil || !strings.Contains(err.Error(), "拒绝读取") {
			t.Errorf("应拒绝读取 %q，得到 %v", p, err)
		}
	}
	if err := ValidateInputPath("/dev/stdin"); err != nil {
		t.Errorf("/dev/stdin 应放行: %v", err)
	}
}

func TestIsWithin(t *testing.T) {
	cases := []struct {
		path, dir string
		want      bool
	}{
		{"/etc", "/etc", true},
		{"/etc/passwd", "/etc", true},
		{"/etcetera", "/etc", false},
		{"/home/u/.ssh2/x", "/home/u/.ssh", false},
		{"/home/u", "/home/u/.ssh", false},
	}
	for _, tc := range cases {
		if got := IsWithin(tc.path, tc.dir); got != tc.want {
			t.Errorf("IsWithin(%q, %q) = %v, want %v", tc.path, tc.dir, got, tc.want)
		}
	}
}

func TestIsWithinResolvedFollowsSymlink(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(allowed, "escape")); err != nil {
		t.Fatal(err)
	}
	if ok, err := IsWithinResolved(filepath.Join(allowed, "sub", "a.md"), allowed); err != nil || !ok {
		t.Fatalf("目录内路径应判定在内: ok=%v err=%v", ok, err)
	}
	if ok, err := IsWithinResolved(filepath.Join(allowed, "escape", "a.md"), allowed); err != nil || ok {
		t.Fatalf("经符号链接逃逸的路径应判定在外: ok=%v err=%v", ok, err)
	}
	if ok, _ := IsWithinResolved(allowed+"-sibling/a.md", allowed); ok {
		t.Fatal("同前缀的兄弟目录不应判定在内")
	}
}

type failingReader struct{ n int }

func (r *failingReader) Read(p []byte) (int, error) {
	if r.n > 0 {
		r.n--
		return copy(p, "partial"), nil
	}
	return 0, errors.New("网络中断")
}

func TestAtomicWriteFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.json")
	if err := AtomicWriteFile(target, []byte("v1"), 0o640); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if st, _ := os.Stat(target); st.Mode().Perm() != 0o640 {
		t.Fatalf("权限位 = %o, want 0640", st.Mode().Perm())
	}
	if err := AtomicWriteFile(target, []byte("v2"), 0o640); err != nil {
		t.Fatalf("覆盖失败: %v", err)
	}
	if b, _ := os.ReadFile(target); string(b) != "v2" {
		t.Fatalf("内容 = %q, want v2", b)
	}

	// 流式写入中途失败：原文件保持不变，不留临时文件
	if _, err := AtomicWriteFrom(target, &failingReader{n: 2}, 0o640); err == nil {
		t.Fatal("读取失败时应返回错误")
	}
	if b, _ := os.ReadFile(target); string(b) != "v2" {
		t.Fatalf("失败后原文件被破坏: %q", b)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("不应残留临时文件: %v", names)
	}
}

func TestReplaceFileWindowsFallback(t *testing.T) {
	orig := onWindows
	onWindows = func() bool { return true }
	t.Cleanup(func() { onWindows = orig })
	dir := t.TempDir()
	target := filepath.Join(dir, "out.md")
	for _, v := range []string{"v1", "v2"} {
		if err := AtomicWriteFile(target, []byte(v), 0o600); err != nil {
			t.Fatalf("写入 %s 失败: %v", v, err)
		}
	}
	if b, _ := os.ReadFile(target); string(b) != "v2" {
		t.Fatalf("内容 = %q, want v2", b)
	}
	if _, err := os.Stat(target + ".bak"); err == nil {
		t.Fatal("成功路径不应残留 .bak")
	}
}
