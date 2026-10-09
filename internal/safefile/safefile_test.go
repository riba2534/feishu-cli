package safefile

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/v2/internal/clierr"
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

// TestAtomicWriteRejectsSensitiveDirs 兜底层：用户输出经 AtomicWriteFile / AtomicWriteFrom 写出时
// 不会落进敏感目录（返回用法错误，不留任何文件）；CLI 自管路径用 Trusted 变体照常写入。
func TestAtomicWriteRejectsSensitiveDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("系统目录规则仅适用于类 Unix")
	}
	home := setupHome(t)
	for _, target := range []string{
		filepath.Join(home, ".ssh", "authorized_keys"),
		filepath.Join(home, ".feishu-cli", "fp-out.md"),
		filepath.Join(home, ".aws", "new", "credentials"),
	} {
		if err := AtomicWriteFile(target, []byte("x"), 0o600); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
			t.Fatalf("AtomicWriteFile(%q) 应返回用法错误，得到 %v", target, err)
		}
		if _, err := AtomicWriteFrom(target, strings.NewReader("x"), 0o600); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
			t.Fatalf("AtomicWriteFrom(%q) 应返回用法错误，得到 %v", target, err)
		}
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatalf("被拒绝的路径不应生成文件: %s", target)
		}
	}
	for _, d := range []string{filepath.Join(home, ".ssh"), filepath.Join(home, ".feishu-cli")} {
		entries, _ := os.ReadDir(d)
		if len(entries) != 0 {
			t.Fatalf("%s 下不应残留临时文件: %d 个", d, len(entries))
		}
	}

	// 内部写入：~/.feishu-cli 下的配置 / token 等由 CLI 自己决定位置，走 Trusted 变体不受拦截
	internal := filepath.Join(home, ".feishu-cli", "config.yaml")
	if err := AtomicWriteFileTrusted(internal, []byte("app_id: cli_xxx\n"), 0o600); err != nil {
		t.Fatalf("Trusted 写入 CLI 自管路径失败: %v", err)
	}
	if n, err := AtomicWriteFromTrusted(internal, strings.NewReader("v2"), 0o600); err != nil || n != 2 {
		t.Fatalf("AtomicWriteFromTrusted 失败: n=%d err=%v", n, err)
	}
	if b, _ := os.ReadFile(internal); string(b) != "v2" {
		t.Fatalf("Trusted 写入内容 = %q", b)
	}
}

func TestMkdirAllRejectsSensitiveDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("系统目录规则仅适用于类 Unix")
	}
	home := setupHome(t)
	bad := filepath.Join(home, ".ssh", "assets")
	if err := MkdirAll(bad, 0o755); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("MkdirAll(%q) 应返回用法错误，得到 %v", bad, err)
	}
	if _, err := os.Stat(bad); !os.IsNotExist(err) {
		t.Fatal("被拒绝的目录不应被创建")
	}
	ok := filepath.Join(t.TempDir(), "a", "b")
	if err := MkdirAll(ok, 0o755); err != nil {
		t.Fatalf("普通目录应可创建: %v", err)
	}
}

// TestInputFileHelpers 输入文件：敏感目录 / 不存在 / 是目录 / 无权限读取均为带路径的用法错误；普通文件正常读取。
func TestInputFileHelpers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("系统目录规则仅适用于类 Unix")
	}
	home := setupHome(t)
	secret := filepath.Join(home, ".ssh", "id_test")
	if err := os.WriteFile(secret, []byte("k"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	ok := filepath.Join(dir, "in.md")
	if err := os.WriteFile(ok, []byte("# hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	noPerm := filepath.Join(dir, "noperm.md")
	if err := os.WriteFile(noPerm, []byte("x"), 0o000); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name, path, want string
	}{
		{"敏感目录", secret, "拒绝读取"},
		{"不存在", filepath.Join(dir, "missing.md"), "不存在"},
		{"是目录", dir, "是目录"},
	}
	if os.Geteuid() != 0 { // root 无视权限位
		cases = append(cases, struct{ name, path, want string }{"无权限", noPerm, "无权限读取"})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ReadInputFile(tc.path); err == nil || !clierr.HasKind(err, clierr.KindUsage) ||
				!strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), tc.path) {
				t.Fatalf("ReadInputFile(%q) 应为含 %q 与路径的用法错误，得到 %v", tc.path, tc.want, err)
			}
			if f, _, err := OpenInputFile(tc.path); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
				if f != nil {
					_ = f.Close()
				}
				t.Fatalf("OpenInputFile(%q) 应为用法错误，得到 %v", tc.path, err)
			}
		})
	}

	data, err := ReadInputFile(ok)
	if err != nil || string(data) != "# hi" {
		t.Fatalf("普通文件应可读取: %q %v", data, err)
	}
	f, info, err := OpenInputFile(ok)
	if err != nil || info.Size() != 4 {
		t.Fatalf("OpenInputFile 普通文件失败: %v", err)
	}
	_ = f.Close()
	if err := InputFileError(ok, errors.New("磁盘坏道")); clierr.HasKind(err, clierr.KindUsage) {
		t.Fatal("非参数类 I/O 错误不应归为用法错误")
	}
}
