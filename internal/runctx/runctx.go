// Package runctx 保存进程级根 context。
//
// 根命令在启动时把接了 SIGINT/SIGTERM 的 context 注册进来；client.Context()、
// token 刷新、分片下载等各层都从它派生带超时的 context，从而在 Ctrl-C 时能立即取消
// 进行中的 HTTP 请求、重试等待与轮询，而不是只能等待超时或被强杀。
// 未注册时（单元测试、库方式调用）退化为 context.Background()。
package runctx

import (
	"context"
	"sync"
)

var (
	mu   sync.RWMutex
	root context.Context = context.Background()
)

// Set 注册进程级根 context；传入 nil 时恢复为 context.Background()。
func Set(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	mu.Lock()
	root = ctx
	mu.Unlock()
}

// Root 返回进程级根 context（未注册时为 context.Background()）。
func Root() context.Context {
	mu.RLock()
	defer mu.RUnlock()
	return root
}
