package runctx

import (
	"context"
	"testing"
	"time"
)

func TestDerivedContextCancelledWithRoot(t *testing.T) {
	t.Cleanup(func() { Set(nil) })
	if Root() == nil || Root().Err() != nil {
		t.Fatal("未注册时应返回未取消的 Background")
	}
	ctx, cancel := context.WithCancel(context.Background())
	Set(ctx)
	derived, stop := context.WithTimeout(Root(), time.Hour)
	defer stop()
	cancel() // 模拟收到 SIGINT
	select {
	case <-derived.Done():
	case <-time.After(time.Second):
		t.Fatal("根 context 取消后，派生的请求 context 应立即取消")
	}
	Set(nil)
	if Root().Err() != nil {
		t.Fatal("Set(nil) 应恢复为 Background")
	}
}
