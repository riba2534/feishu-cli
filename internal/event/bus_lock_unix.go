//go:build !windows

package event

import (
	"fmt"
	"os"
	"syscall"
)

func withBusFileLock(path string, fn func() error) error {
	lockFD, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("打开 bus.lock 失败: %w", err)
	}
	defer lockFD.Close()

	if err := syscall.Flock(int(lockFD.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("获取 bus.lock 失败: %w", err)
	}
	defer func() {
		_ = syscall.Flock(int(lockFD.Fd()), syscall.LOCK_UN)
	}()

	return fn()
}
