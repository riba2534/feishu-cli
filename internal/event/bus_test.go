package event

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setupBus 在 tempdir 下构造一个 Bus，并把 HOME 重定向到 tempdir，
// 保证 EventsDir/AppDir 不污染真实 ~/.feishu-cli/。
func setupBus(t *testing.T) *Bus {
	t.Helper()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	bus, err := NewBus("cli_test_app_123")
	if err != nil {
		t.Fatalf("NewBus: %v", err)
	}
	return bus
}

func TestBus_RegisterAndSnapshot(t *testing.T) {
	bus := setupBus(t)

	entry := ConsumerEntry{
		PID:       os.Getpid(),
		EventKey:  "im.message.receive_v1",
		StartedAt: time.Now(),
		MaxEvents: 10,
	}
	if err := bus.Register(entry); err != nil {
		t.Fatalf("Register: %v", err)
	}

	snap, err := bus.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snap.AppID != "cli_test_app_123" {
		t.Errorf("AppID 期望 cli_test_app_123，实际 %q", snap.AppID)
	}
	if len(snap.Consumers) != 1 {
		t.Fatalf("Snapshot 期望 1 条 consumer，实际 %d", len(snap.Consumers))
	}
	if snap.Consumers[0].PID != entry.PID {
		t.Errorf("PID 期望 %d，实际 %d", entry.PID, snap.Consumers[0].PID)
	}
	if snap.Consumers[0].MaxEvents != 10 {
		t.Errorf("MaxEvents 期望 10，实际 %d", snap.Consumers[0].MaxEvents)
	}
}

func TestBus_UnregisterIdempotent(t *testing.T) {
	bus := setupBus(t)
	entry := ConsumerEntry{PID: os.Getpid(), EventKey: "im.chat.updated_v1", StartedAt: time.Now()}
	_ = bus.Register(entry)

	if err := bus.Unregister(entry.PID, entry.EventKey); err != nil {
		t.Fatalf("Unregister #1: %v", err)
	}
	// 第二次 Unregister 应幂等不报错
	if err := bus.Unregister(entry.PID, entry.EventKey); err != nil {
		t.Fatalf("Unregister #2 应幂等: %v", err)
	}
	snap, _ := bus.Snapshot()
	if len(snap.Consumers) != 0 {
		t.Errorf("Unregister 后 snapshot 应为空，实际 %d", len(snap.Consumers))
	}
}

func TestBus_ReplacesDuplicateRegistration(t *testing.T) {
	bus := setupBus(t)
	pid := os.Getpid()
	_ = bus.Register(ConsumerEntry{PID: pid, EventKey: "im.message.receive_v1", StartedAt: time.Now(), MaxEvents: 5})
	_ = bus.Register(ConsumerEntry{PID: pid, EventKey: "im.message.receive_v1", StartedAt: time.Now(), MaxEvents: 100})

	snap, _ := bus.Snapshot()
	if len(snap.Consumers) != 1 {
		t.Fatalf("重复注册 (同 pid + 同 key) 应替换不追加，实际 %d 条", len(snap.Consumers))
	}
	if snap.Consumers[0].MaxEvents != 100 {
		t.Errorf("重复注册后 MaxEvents 应为新值 100，实际 %d", snap.Consumers[0].MaxEvents)
	}
}

func TestBus_DifferentKeysCoexist(t *testing.T) {
	bus := setupBus(t)
	pid := os.Getpid()
	_ = bus.Register(ConsumerEntry{PID: pid, EventKey: "im.message.receive_v1", StartedAt: time.Now()})
	_ = bus.Register(ConsumerEntry{PID: pid, EventKey: "im.chat.updated_v1", StartedAt: time.Now()})

	snap, _ := bus.Snapshot()
	if len(snap.Consumers) != 2 {
		t.Errorf("不同 EventKey 应可共存，期望 2 条实际 %d", len(snap.Consumers))
	}
}

func TestBus_SnapshotCleansDeadPIDs(t *testing.T) {
	bus := setupBus(t)
	// 注册一个不存在的 PID（PID 99999999 大概率不存在）
	deadPID := 99999999
	_ = bus.Register(ConsumerEntry{PID: deadPID, EventKey: "im.message.receive_v1", StartedAt: time.Now()})
	// 同时注册一个真实存活的 PID
	livePID := os.Getpid()
	_ = bus.Register(ConsumerEntry{PID: livePID, EventKey: "im.chat.updated_v1", StartedAt: time.Now()})

	snap, err := bus.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	// Snapshot 应剔除 deadPID 那条
	for _, c := range snap.Consumers {
		if c.PID == deadPID {
			t.Errorf("Snapshot 应剔除已死 PID %d", deadPID)
		}
	}
	hasLive := false
	for _, c := range snap.Consumers {
		if c.PID == livePID {
			hasLive = true
		}
	}
	if !hasLive {
		t.Errorf("Snapshot 应保留存活 PID %d", livePID)
	}
}

func TestBus_AtomicWrite(t *testing.T) {
	bus := setupBus(t)
	_ = bus.Register(ConsumerEntry{PID: os.Getpid(), EventKey: "im.message.receive_v1", StartedAt: time.Now()})

	// bus.json 应存在且非空
	data, err := os.ReadFile(bus.StateFile())
	if err != nil {
		t.Fatalf("读 bus.json: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("bus.json 不应为空")
	}

	// tmp 文件应已被 rename 清理
	matches, _ := filepath.Glob(bus.StateFile() + ".tmp.*")
	if len(matches) != 0 {
		t.Errorf("tmp 文件 %v 应在 rename 后清理", matches)
	}
}

func TestConsumerEntry_IsAlive(t *testing.T) {
	live := ConsumerEntry{PID: os.Getpid()}
	if !live.IsAlive() {
		t.Errorf("当前进程 PID %d 应判定为存活", os.Getpid())
	}
	dead := ConsumerEntry{PID: 99999999}
	if dead.IsAlive() {
		t.Errorf("PID 99999999 大概率不应存活")
	}
	zero := ConsumerEntry{PID: 0}
	if zero.IsAlive() {
		t.Errorf("PID 0 不应判定为存活")
	}
}
