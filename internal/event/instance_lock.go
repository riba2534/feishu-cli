package event

import (
	"errors"
	"path/filepath"
)

// ErrInstanceLockHeld 表示同一 App 在本机已有 event consume 进程持有单实例锁。
var ErrInstanceLockHeld = errors.New("同一 App 已有 event consume 进程在运行")

// InstanceLock 是 event consume 的单实例锁句柄（进程退出时由操作系统自动释放）。
//
// 为什么需要：飞书长连接会把同一 App 的事件随机分发给该 App 的任意一条 WebSocket 连接。
// 同一台机器上开多个 consume 进程（哪怕订阅不同 EventKey）就是多条连接互相抢事件，
// 每个进程都只能收到一部分。因此同一 App 同一机器只允许一个 consume 进程，
// 需要多个 EventKey 时在一个进程里一起订阅：feishu-cli event consume k1 k2。
type InstanceLock struct {
	path    string
	release func()
}

// Path 返回锁文件路径（排查用）。
func (l *InstanceLock) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// Release 释放锁；幂等。
func (l *InstanceLock) Release() {
	if l == nil || l.release == nil {
		return
	}
	l.release()
	l.release = nil
}

// AcquireConsumeLock 以非阻塞方式获取 App 级 consume 单实例锁。
// 已被其他进程持有时返回 ErrInstanceLockHeld。
func AcquireConsumeLock(appID string) (*InstanceLock, error) {
	dir, err := AppDir(appID)
	if err != nil {
		return nil, err
	}
	return acquireInstanceLock(filepath.Join(dir, "consume.lock"))
}
