package event

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func eventPayload(eventType, eventID string) []byte {
	return []byte(`{"schema":"2.0","header":{"event_id":"` + eventID + `","event_type":"` + eventType + `","create_time":"1"},"event":{"k":"v"}}`)
}

// TestBuildDispatcherRoutesSubscribedAndAcksOthers 验证同一个 dispatcher（= 一条 WS 连接）：
//   - 订阅的多个 EventKey 都会输出；
//   - 未订阅但已知的事件类型返回 nil（ACK）且不输出；
//   - 完全未知的事件类型仍按 SDK 默认报错（不吞掉无法识别的帧）。
func TestBuildDispatcherRoutesSubscribedAndAcksOthers(t *testing.T) {
	var out, errOut concurrentBuffer
	r := NewRuntime(ConsumeOptions{
		AppID:           "cli_test",
		AppSecret:       "secret",
		EventKeys:       []string{"im.message.receive_v1", "im.message.reaction.created_v1"},
		Out:             &out,
		ErrOut:          &errOut,
		AckUnsubscribed: true,
	})
	defs := []KeyDefinition{}
	for _, k := range r.eventKeys() {
		def, ok := Lookup(k)
		if !ok {
			t.Fatalf("Lookup(%s) 失败", k)
		}
		defs = append(defs, def)
	}
	dis := r.buildDispatcher(defs)
	ctx := context.Background()

	for i, et := range []string{"im.message.receive_v1", "im.message.reaction.created_v1"} {
		if _, err := dis.Do(ctx, eventPayload(et, "ev_sub_"+string(rune('a'+i)))); err != nil {
			t.Fatalf("订阅类型 %s 处理失败: %v", et, err)
		}
	}
	// 未订阅但已知：ACK + 本地丢弃（两次只提示一次）
	for _, id := range []string{"ev_drop_1", "ev_drop_2"} {
		if _, err := dis.Do(ctx, eventPayload("im.chat.updated_v1", id)); err != nil {
			t.Fatalf("未订阅的已知类型应 ACK（返回 nil），got %v", err)
		}
	}
	if _, err := dis.Do(ctx, eventPayload("contact.department.created_v3", "ev_drop_3")); err != nil {
		t.Fatalf("官方 catch-all 列表中的类型也应 ACK，got %v", err)
	}
	// 完全未知类型
	if _, err := dis.Do(ctx, eventPayload("totally.unknown.event_v9", "ev_unknown")); err == nil {
		t.Fatal("完全未知的事件类型不应被静默 ACK")
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("应只输出 2 条订阅事件，实际 %d 行:\n%s", len(lines), out.String())
	}
	if !strings.Contains(lines[0], "im.message.receive_v1") || !strings.Contains(lines[1], "im.message.reaction.created_v1") {
		t.Fatalf("输出事件不符:\n%s", out.String())
	}
	if c := strings.Count(errOut.String(), "im.chat.updated_v1"); c != 1 {
		t.Fatalf("未订阅类型应只提示一次，实际 %d 次:\n%s", c, errOut.String())
	}
}

// TestEmitDedupByEventID 同一 event_id 的重复投递只输出一次；不同 ID 正常输出；
// schema 1.0 事件用顶层 uuid 去重。
func TestEmitDedupByEventID(t *testing.T) {
	var out concurrentBuffer
	r := NewRuntime(ConsumeOptions{EventKey: "im.message.receive_v1", Out: &out, ErrOut: io.Discard})
	dis := r.buildDispatcher([]KeyDefinition{mustLookup(t, "im.message.receive_v1")})
	ctx := context.Background()
	for _, id := range []string{"ev_1", "ev_1", "ev_2", "ev_1"} {
		if _, err := dis.Do(ctx, eventPayload("im.message.receive_v1", id)); err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.Count(out.String(), "\n"); got != 2 {
		t.Fatalf("去重后应输出 2 条，实际 %d:\n%s", got, out.String())
	}
	if r.received.Load() != 2 {
		t.Fatalf("重复事件不应计入 max-events，received=%d", r.received.Load())
	}
}

func TestDedupFilterTTLAndRing(t *testing.T) {
	now := time.Unix(1700000000, 0)
	d := newDedupFilterWithSize(2, time.Minute)
	d.now = func() time.Time { return now }
	if d.isDuplicate("a") || !d.isDuplicate("a") {
		t.Fatal("同一 ID 第二次应判重")
	}
	if d.isDuplicate("") || d.isDuplicate("") {
		t.Fatal("空 ID 不参与去重")
	}
	now = now.Add(2 * time.Minute)
	if d.isDuplicate("a") {
		t.Fatal("超过 TTL 后应重新放行")
	}
	// ring=2：b、c 进入后 a 被淘汰
	d.isDuplicate("b")
	d.isDuplicate("c")
	if len(d.seen) > 2 {
		t.Fatalf("seen 应受 ring 大小约束，got %d", len(d.seen))
	}
}

// TestMultiKeyRunUsesSingleConnectionAndReadyLabel 多 key 只启动一次 WS（一条连接），
// ready marker 以逗号连接全部 key；bus.json 为每个 key 各登记一条。
func TestMultiKeyRunUsesSingleConnectionAndReadyLabel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bus, err := NewBus("cli_multi_test")
	if err != nil {
		t.Fatal(err)
	}
	var ready concurrentBuffer
	var starts atomic.Int32
	keysSeen := make(chan []string, 1)
	r := NewRuntime(ConsumeOptions{
		AppID:     "cli_multi_test",
		AppSecret: "secret",
		EventKeys: []string{"im.message.receive_v1,im.chat.updated_v1", "im.message.receive_v1"},
		ErrOut:    io.Discard,
		ReadyOut:  &ready,
		Bus:       bus,
		StartWS: func(ctx context.Context, onHandshake func()) error {
			starts.Add(1)
			snap, _ := bus.Snapshot()
			var keys []string
			for _, c := range snap.Consumers {
				keys = append(keys, c.EventKey)
			}
			keysSeen <- keys
			onHandshake()
			<-ctx.Done()
			return ctx.Err()
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_, _ = r.Run(ctx)
		close(done)
	}()
	select {
	case keys := <-keysSeen:
		if strings.Join(keys, ",") != "im.message.receive_v1,im.chat.updated_v1" {
			t.Fatalf("bus.json 登记的 key = %v", keys)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StartWS 未被调用")
	}
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(ready.String(), "[event] ready event_key=im.message.receive_v1,im.chat.updated_v1") {
		if time.Now().After(deadline) {
			t.Fatalf("ready marker 不符: %q", ready.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	if starts.Load() != 1 {
		t.Fatalf("多 key 应只建立一条连接，StartWS 调用 %d 次", starts.Load())
	}
	snap, _ := bus.Snapshot()
	if len(snap.Consumers) != 0 {
		t.Fatalf("退出后应清理 bus.json，剩余 %+v", snap.Consumers)
	}
}

func TestAckOnlyEventTypesCoverRegistryAndCommon(t *testing.T) {
	types := ackOnlyEventTypes()
	set := map[string]bool{}
	for _, et := range types {
		if set[et] {
			t.Fatalf("重复类型 %s", et)
		}
		set[et] = true
	}
	for _, def := range ListAll() {
		if def.CardCallback {
			if set[def.EventType] {
				t.Fatalf("卡片回调 %s 不应被代答 ACK", def.EventType)
			}
			continue
		}
		if !set[def.EventType] {
			t.Fatalf("目录内事件 %s 未纳入 ACK 集合", def.EventType)
		}
	}
	if !set["im.chat.member.user.withdrawn_v1"] {
		t.Fatal("官方 catch-all 常见类型应纳入 ACK 集合")
	}
}

func TestNormalizeEventKeys(t *testing.T) {
	got := NormalizeEventKeys([]string{" a,b ", "b", "", "c,,a"})
	if strings.Join(got, ",") != "a,b,c" {
		t.Fatalf("NormalizeEventKeys = %v", got)
	}
}

// TestAcquireConsumeLockIsExclusive 同一 App 的单实例锁：持有期间再次获取返回 ErrInstanceLockHeld，释放后可再获取。
func TestAcquireConsumeLockIsExclusive(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	first, err := AcquireConsumeLock("cli_lock_test")
	if err != nil {
		t.Fatalf("首次获取锁失败: %v", err)
	}
	if _, err := AcquireConsumeLock("cli_lock_test"); !errors.Is(err, ErrInstanceLockHeld) {
		t.Fatalf("锁被持有时应返回 ErrInstanceLockHeld，got %v", err)
	}
	other, err := AcquireConsumeLock("cli_other_app")
	if err != nil {
		t.Fatalf("不同 App 的锁应互不影响: %v", err)
	}
	other.Release()
	data, _ := os.ReadFile(filepath.Clean(first.Path()))
	if !strings.Contains(string(data), "\n") {
		t.Fatalf("锁文件应记录持有者 PID，got %q", data)
	}
	first.Release()
	again, err := AcquireConsumeLock("cli_lock_test")
	if err != nil {
		t.Fatalf("释放后应可重新获取: %v", err)
	}
	again.Release()
}

func mustLookup(t *testing.T, key string) KeyDefinition {
	t.Helper()
	def, ok := Lookup(key)
	if !ok {
		t.Fatalf("Lookup(%s) 失败", key)
	}
	return def
}

// TestBuildDispatcherNoAckWhenOtherConnectionsExist 远端已有其他长连接时不代答未订阅类型：
// 返回错误让服务端可重投给其他连接，避免把别人的事件 ACK 掉。
func TestBuildDispatcherNoAckWhenOtherConnectionsExist(t *testing.T) {
	r := NewRuntime(ConsumeOptions{EventKey: "im.message.receive_v1", Out: io.Discard, ErrOut: io.Discard})
	dis := r.buildDispatcher([]KeyDefinition{mustLookup(t, "im.message.receive_v1")})
	if _, err := dis.Do(context.Background(), eventPayload("im.chat.updated_v1", "ev_x")); err == nil {
		t.Fatal("AckUnsubscribed=false 时未订阅类型不应被 ACK")
	}
	if _, err := dis.Do(context.Background(), eventPayload("im.message.receive_v1", "ev_y")); err != nil {
		t.Fatalf("订阅类型应正常处理: %v", err)
	}
}
