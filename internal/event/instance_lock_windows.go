//go:build windows

package event

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"syscall"
)

// errorSharingViolation 对应 Win32 ERROR_SHARING_VIOLATION（32）：文件已被其他进程独占打开。
const errorSharingViolation syscall.Errno = 32

// acquireInstanceLock 以 share mode=0 独占打开锁文件实现单实例锁；
// 进程退出时句柄由系统关闭，锁自动释放。
func acquireInstanceLock(path string) (*InstanceLock, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("单实例锁路径无效: %w", err)
	}
	h, err := syscall.CreateFile(p,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0, // 不共享：其他进程再次打开会得到 ERROR_SHARING_VIOLATION
		nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		if errors.Is(err, errorSharingViolation) {
			return nil, ErrInstanceLockHeld
		}
		return nil, fmt.Errorf("获取单实例锁失败: %w", err)
	}
	f := os.NewFile(uintptr(h), path)
	if err := f.Truncate(0); err == nil {
		_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	}
	return &InstanceLock{
		path: path,
		release: func() {
			_ = f.Close()
		},
	}, nil
}
