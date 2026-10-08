//go:build !windows

package event

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"syscall"
)

// acquireInstanceLock 用 flock(LOCK_EX|LOCK_NB) 实现非阻塞单实例锁；
// 进程崩溃/被 kill 时内核自动释放，不会留下僵尸锁。
func acquireInstanceLock(path string) (*InstanceLock, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, fmt.Errorf("打开单实例锁文件失败: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrInstanceLockHeld
		}
		return nil, fmt.Errorf("获取单实例锁失败: %w", err)
	}
	// 记录持有者 PID，便于人工排查（锁语义只依赖 flock，不依赖文件内容）。
	if err := f.Truncate(0); err == nil {
		_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	}
	return &InstanceLock{
		path: path,
		release: func() {
			_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
			_ = f.Close()
		},
	}, nil
}
