package profile

import (
	"testing"
	"time"
)

// CommandOverride 不得复用 writeMu：Create/Remove/Use 等写函数持锁期间若需要读覆盖值，
// 复用这把不可重入的锁会直接自锁死。
func TestCommandOverrideDoesNotShareWriteMutex(t *testing.T) {
	writeMu.Lock()
	defer writeMu.Unlock()

	done := make(chan struct{})
	go func() {
		_ = CommandOverride()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("持有 writeMu 时读 CommandOverride 卡死：两者必须使用独立 mutex")
	}
}
