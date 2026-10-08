package safefile

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// onWindows 供测试注入，用于在非 Windows 平台验证覆盖兜底分支。
var onWindows = func() bool { return runtime.GOOS == "windows" }

// AtomicWriteFile 原子写入：同目录临时文件 → chmod → 写入 → fsync → rename → fsync 目录。
// 任一步失败都不会改动已存在的目标文件，也不会留下半截文件或临时文件。
// 语义与 cmd/drive_export_write.go、internal/auth/atomic_write.go 的既有实现一致（显式权限位、
// 目录 fsync、Windows 无法直接覆盖时的 .bak 兜底），此处提供可复用的公共版本。
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	_, err := AtomicWriteFrom(path, bytes.NewReader(data), perm)
	return err
}

// AtomicWriteFrom 流式原子写入（下载等大文件场景）：从 r 读到 EOF 后才提交到 path，
// 读取或写入中途失败（含网络中断、Ctrl-C 取消）时目标文件保持原样。返回写入字节数。
func AtomicWriteFrom(path string, r io.Reader, perm os.FileMode) (int64, error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return 0, fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(perm); err != nil {
		return 0, fmt.Errorf("设置临时文件权限失败: %w", err)
	}
	n, err := io.Copy(tmp, r)
	if err != nil {
		return n, fmt.Errorf("写入临时文件失败: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return n, fmt.Errorf("fsync 临时文件失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return n, fmt.Errorf("关闭临时文件失败: %w", err)
	}
	if err := replaceFile(tmpName, path); err != nil {
		return n, fmt.Errorf("提交文件失败: %w", err)
	}
	committed = true
	// 提交成功后 fsync 目录；此处失败不回滚已替换的文件（内容已落盘）
	_ = syncDir(dir)
	return n, nil
}

func syncDir(dir string) error {
	if onWindows() {
		return nil
	}
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

// replaceFile 提交临时文件；Windows 上 Rename 不能覆盖已存在目标，先把旧文件挪到 .bak，失败则回滚。
func replaceFile(tmp, dest string) error {
	if err := os.Rename(tmp, dest); err == nil || !onWindows() {
		return err
	}
	bak := dest + ".bak"
	_ = os.Remove(bak)
	if _, statErr := os.Stat(dest); statErr == nil {
		if err := os.Rename(dest, bak); err != nil {
			return err
		}
		if err := os.Rename(tmp, dest); err != nil {
			_ = os.Rename(bak, dest)
			return err
		}
		_ = os.Remove(bak)
		return nil
	}
	return os.Rename(tmp, dest)
}
