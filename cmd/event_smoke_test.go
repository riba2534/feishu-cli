//go:build smoke

package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/event"
)

// TestSmokeEventConsume 是一个本地手动测试。
//
// 前置：必须设置 FEISHU_APP_ID + FEISHU_APP_SECRET 真实凭证；在飞书开放平台启用「事件订阅 - 长连接」
// 并订阅 im.message.receive_v1。
//
// 跑法：go test -tags=smoke ./cmd -run TestSmokeEventConsume
//
// 验证内容：
//   1. WebSocket 能成功连接（5 秒内打印 ready marker）
//   2. bus.json 注册成功
//   3. 5 秒后 ctx 取消，进程正常退出，bus.json 自动 unregister
func TestSmokeEventConsume(t *testing.T) {
	if os.Getenv("FEISHU_APP_ID") == "" || os.Getenv("FEISHU_APP_SECRET") == "" {
		t.Skip("跳过 smoke 测试：需 FEISHU_APP_ID + FEISHU_APP_SECRET 真实凭证")
	}

	// 初始化配置（读环境变量）
	if err := config.Init(""); err != nil {
		t.Fatalf("config.Init: %v", err)
	}
	cfg := config.Get()
	if cfg.AppID == "" || cfg.AppSecret == "" {
		t.Skip("跳过：env 设置但 config 读不到")
	}

	bus, err := event.NewBus(cfg.AppID)
	if err != nil {
		t.Fatalf("NewBus: %v", err)
	}

	stderr := &bytes.Buffer{}
	stdout := &bytes.Buffer{}

	runtime := event.NewRuntime(event.ConsumeOptions{
		AppID:     cfg.AppID,
		AppSecret: cfg.AppSecret,
		EventKey:  "im.message.receive_v1",
		BaseURL:   "https://open.feishu.cn",
		Out:       stdout,
		ErrOut:    io.MultiWriter(stderr, os.Stderr),
		Timeout:   5 * time.Second,
		Bus:       bus,
	})

	ctx := context.Background()
	reason, err := runtime.Run(ctx)
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if reason != "timeout" && reason != "signal" {
		t.Errorf("退出 reason 期望 timeout/signal，实际 %q", reason)
	}
	if !strings.Contains(stderr.String(), "[event] ready event_key=im.message.receive_v1") {
		t.Errorf("stderr 应包含 ready marker，实际:\n%s", stderr.String())
	}

	// bus.json 应已自动 unregister
	snap, _ := bus.Snapshot()
	for _, c := range snap.Consumers {
		if c.PID == os.Getpid() && c.EventKey == "im.message.receive_v1" {
			t.Errorf("Run 退出后 bus.json 应已 unregister 本进程，实际仍存在")
		}
	}
}
