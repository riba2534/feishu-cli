package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

// interruptGracePeriod 收到第一次 SIGINT/SIGTERM 后，等待命令借 context 取消自行收尾的时长。
// 超时仍未退出（某段代码没有使用 context）则强制以 130 退出。
// 需大于 event consume 的优雅关闭耗时（WS 关闭 2s + 退订 HTTP 5s）。
const interruptGracePeriod = 10 * time.Second

// rootInterruptedFlag 记录本进程是否因 SIGINT/SIGTERM 被取消，退出码据此映射为 130。
var rootInterruptedFlag atomic.Bool

func rootInterrupted() bool { return rootInterruptedFlag.Load() }

// interruptExit 供测试注入，替代 os.Exit。
var interruptExit = os.Exit

// newSignalContext 返回在 SIGINT/SIGTERM 时取消的根 context 与清理函数。
//
// 第一次信号：取消 context（进行中的 HTTP 请求、重试退避、轮询都会立即返回），
// 并恢复默认信号处理——再次 Ctrl-C 直接终止进程；若 grace 内命令仍未退出，强制以 130 退出。
// 命令正常结束时调用清理函数，不会误判为中断。
func newSignalContext(parent context.Context, grace time.Duration, errOut io.Writer) (context.Context, func()) {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		select {
		case <-done:
			return
		case <-ctx.Done():
		}
		// 清理函数会先关闭 done 再 stop()，两者同时就绪时以 done 为准，避免把正常结束当成中断
		select {
		case <-done:
			return
		default:
		}
		if parent.Err() != nil {
			return // 父 context 取消不是信号中断
		}
		rootInterruptedFlag.Store(true)
		stop()
		fmt.Fprintln(errOut, "\n收到中断信号，正在取消进行中的操作（再次 Ctrl-C 立即退出）...")
		timer := time.NewTimer(grace)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
			fmt.Fprintln(errOut, "等待收尾超时，强制退出")
			interruptExit(clierr.ExitInterrupted)
		}
	}()
	var once sync.Once
	return ctx, func() {
		once.Do(func() {
			close(done)
			stop()
		})
	}
}
