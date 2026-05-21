//go:build windows

package event

import (
	"fmt"
	"os"
	"time"
)

const (
	busLockPollInterval = 20 * time.Millisecond
	busLockTimeout      = 5 * time.Second
	busLockStaleAfter   = time.Minute
)

func withBusFileLock(path string, fn func() error) error {
	lockDir := path + ".lockdir"
	deadline := time.Now().Add(busLockTimeout)

	for {
		err := os.Mkdir(lockDir, 0700)
		if err == nil {
			_ = touchBusLockFile(path)
			defer os.Remove(lockDir)
			return fn()
		}
		if !os.IsExist(err) {
			return fmt.Errorf("获取 bus.lock 失败: %w", err)
		}
		if info, statErr := os.Stat(lockDir); statErr == nil && time.Since(info.ModTime()) > busLockStaleAfter {
			_ = os.Remove(lockDir)
			continue
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("获取 bus.lock 超时")
		}
		time.Sleep(busLockPollInterval)
	}
}

func touchBusLockFile(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	return f.Close()
}
