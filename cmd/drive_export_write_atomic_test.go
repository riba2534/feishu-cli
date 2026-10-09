package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAtomicWriteFile_PermAndOverwrite 验证导出原子写入的权限位与覆盖语义。
// 回归防护：cmd 版原子写入曾缺 Chmod（文件停留在 CreateTemp 的默认权限）、
// 缺目录 fsync、缺 Windows 覆盖兜底。
func TestAtomicWriteFile_PermAndOverwrite(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.md")

	if err := atomicWriteFile(target, []byte("first")); err != nil {
		t.Fatalf("首次写入失败: %v", err)
	}
	st, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat 失败: %v", err)
	}
	if got := st.Mode().Perm(); got != exportFilePerm {
		t.Errorf("权限位 = %o, want %o", got, exportFilePerm)
	}
	// 注：os.CreateTemp 默认也是 0600，与 exportFilePerm 相同，
	// 故上面的断言无法区分"显式 Chmod"与"依赖默认值"。
	// 下面用一个与默认值不同的权限做真实校验。
	origPerm := exportFilePermForTest
	exportFilePermForTest = 0640
	t.Cleanup(func() { exportFilePermForTest = origPerm })
	target2 := filepath.Join(dir, "perm.md")
	if err := atomicWriteFile(target2, []byte("x")); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if st2, err := os.Stat(target2); err != nil {
		t.Fatal(err)
	} else if got := st2.Mode().Perm(); got != 0640 {
		t.Errorf("显式权限未生效: %o, want 0640（说明缺 Chmod 步骤）", got)
	}

	// 覆盖既有文件
	if err := atomicWriteFile(target, []byte("second")); err != nil {
		t.Fatalf("覆盖写入失败: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "second" {
		t.Errorf("内容 = %q, want second", data)
	}

	// 不留临时文件
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("残留临时文件: %s", e.Name())
		}
	}
}

// Windows 覆盖兜底分支已迁移到 internal/safefile（见 safefile_test.go 的 TestReplaceFileWindowsFallback）。
