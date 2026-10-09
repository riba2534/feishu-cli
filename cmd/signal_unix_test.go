//go:build !windows

package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/runctx"
)

func resetInterruptState(t *testing.T) {
	t.Helper()
	origExit := interruptExit
	t.Cleanup(func() {
		interruptExit = origExit
		rootInterruptedFlag.Store(false)
		runctx.Set(nil)
	})
}

// TestSignalCancelsRootContext 验证 SIGTERM 会取消根 context，client.Context() 派生的请求 context 随之取消，
// 退出码映射为 130；命令迟迟不退出时在 grace 后强制退出。
func TestSignalCancelsRootContext(t *testing.T) {
	resetInterruptState(t)
	var exitCode atomic.Int32
	exitCode.Store(-1)
	exited := make(chan struct{})
	interruptExit = func(code int) {
		exitCode.Store(int32(code))
		close(exited)
	}
	var errOut bytes.Buffer
	ctx, cleanup := newSignalContext(context.Background(), 50*time.Millisecond, &errOut)
	defer cleanup()
	runctx.Set(ctx)
	reqCtx := client.Context() // 模拟进行中的 API 请求

	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("发送信号失败: %v", err)
	}
	select {
	case <-reqCtx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("收到 SIGTERM 后 client.Context() 应被取消")
	}
	select {
	case <-exited:
	case <-time.After(2 * time.Second):
		t.Fatal("grace 超时后应强制退出")
	}
	if got := exitCode.Load(); got != 130 {
		t.Fatalf("强制退出码 = %d, want 130", got)
	}
	if got := exitCodeFor(fmt.Errorf("请求失败: %w", context.Canceled)); got != 130 {
		t.Fatalf("中断后失败的退出码 = %d, want 130", got)
	}
}

func TestSignalContextNormalCompletionIsNotInterrupt(t *testing.T) {
	resetInterruptState(t)
	called := make(chan int, 1)
	interruptExit = func(code int) { called <- code }
	var errOut bytes.Buffer
	ctx, cleanup := newSignalContext(context.Background(), 20*time.Millisecond, &errOut)
	cleanup()
	<-ctx.Done()
	time.Sleep(60 * time.Millisecond)
	if rootInterrupted() {
		t.Fatal("正常结束不应标记为中断")
	}
	select {
	case code := <-called:
		t.Fatalf("正常结束不应强制退出，得到 %d", code)
	default:
	}
	if got := exitCodeFor(errors.New("普通错误")); got != 1 {
		t.Fatalf("未中断时普通错误退出码 = %d, want 1", got)
	}
}
