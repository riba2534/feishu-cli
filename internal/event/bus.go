package event

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// EventsDir 返回 feishu-cli 事件订阅状态目录：~/.feishu-cli/events/。
// 每个 App ID 一个子目录（bus.json + bus.lock），避免不同应用互相干扰。
func EventsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户目录失败: %w", err)
	}
	dir := filepath.Join(home, ".feishu-cli", "events")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("创建事件目录失败: %w", err)
	}
	return dir, nil
}

// AppDir 返回某个 AppID 的专属状态目录。
// sanitizeAppID 防止 ".."、"/" 等字符逃逸出 events/。
func AppDir(appID string) (string, error) {
	base, err := EventsDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, sanitizeAppID(appID))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("创建 App 事件目录失败: %w", err)
	}
	return dir, nil
}

// sanitizeAppID 去除可能用于路径逃逸的字符，仅保留字母/数字/下划线/连字符。
// 飞书 App ID 形如 `cli_a77d84747fa6500b`，本身已是安全字符集；本函数仅作纵深防御。
func sanitizeAppID(appID string) string {
	var b strings.Builder
	for _, r := range appID {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '_' || r == '-':
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

// ConsumerEntry 是 bus.json 中的一条 consumer 记录。
// 一个 (PID, EventKey) 元组对应一个独立的 consume 进程。
type ConsumerEntry struct {
	PID        int       `json:"pid"`
	EventKey   string    `json:"event_key"`
	StartedAt  time.Time `json:"started_at"`
	OutputDir  string    `json:"output_dir,omitempty"`
	JQExpr     string    `json:"jq_expr,omitempty"`
	MaxEvents  int       `json:"max_events,omitempty"`
	TimeoutSec int       `json:"timeout_sec,omitempty"`
}

// IsAlive 检查 PID 对应的进程是否还存活（不可移植：仅 Unix）。
// 通过 signal(0) 探活：进程不存在返回 ESRCH，存在则返回 nil 或 EPERM（无权限但确实存在）。
func (c ConsumerEntry) IsAlive() bool {
	if c.PID <= 0 {
		return false
	}
	proc, err := os.FindProcess(c.PID)
	if err != nil {
		return false
	}
	// signal(0) 不实际投递信号，只做探活
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	// EPERM = 进程存在但属于其他用户（feishu-cli 单用户场景一般不会遇到）
	return err == syscall.EPERM
}

// BusState 是 bus.json 的根结构：所有当前已知的 consumer 列表。
// 加载/保存时通过 mu 保证读写互斥；跨进程互斥通过 bus.lock 文件锁实现。
type BusState struct {
	AppID     string          `json:"app_id"`
	UpdatedAt time.Time       `json:"updated_at"`
	Consumers []ConsumerEntry `json:"consumers"`
}

// Bus 封装 bus.json 的读写 + 文件锁。
// 单进程内多 goroutine 通过 mu 串行化；跨进程通过 flock(2) on bus.lock。
type Bus struct {
	appID    string
	stateDir string
	mu       sync.Mutex
}

// NewBus 构造 Bus，确保 stateDir 存在。
func NewBus(appID string) (*Bus, error) {
	dir, err := AppDir(appID)
	if err != nil {
		return nil, err
	}
	return &Bus{appID: appID, stateDir: dir}, nil
}

// StateFile 返回 bus.json 路径。
func (b *Bus) StateFile() string {
	return filepath.Join(b.stateDir, "bus.json")
}

// LockFile 返回 bus.lock 路径（仅用作 flock 锚点，不写内容）。
func (b *Bus) LockFile() string {
	return filepath.Join(b.stateDir, "bus.lock")
}

// load 读取 bus.json；文件不存在视为空 state。
// 调用方负责持有锁（withLock 内部已加锁）。
func (b *Bus) load() (*BusState, error) {
	data, err := os.ReadFile(b.StateFile())
	if err != nil {
		if os.IsNotExist(err) {
			return &BusState{AppID: b.appID, Consumers: nil}, nil
		}
		return nil, fmt.Errorf("读取 bus.json 失败: %w", err)
	}
	var state BusState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("解析 bus.json 失败: %w", err)
	}
	if state.AppID == "" {
		state.AppID = b.appID
	}
	return &state, nil
}

// save 原子写 bus.json（tmp + os.Rename），调用方负责持有锁。
func (b *Bus) save(state *BusState) error {
	state.AppID = b.appID
	state.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 bus.json 失败: %w", err)
	}
	tmp := b.StateFile() + ".tmp." + strconv.Itoa(os.Getpid())
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("写入 bus.json 临时文件失败: %w", err)
	}
	if err := os.Rename(tmp, b.StateFile()); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("替换 bus.json 失败: %w", err)
	}
	return nil
}

// withLock 在跨进程互斥锁保护下执行 fn。
// flock(LOCK_EX) 在锁文件 fd 上加排他锁，进程退出/fd 关闭自动释放——比 PID 文件更稳健。
func (b *Bus) withLock(fn func(state *BusState) error) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	lockFD, err := os.OpenFile(b.LockFile(), os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("打开 bus.lock 失败: %w", err)
	}
	defer lockFD.Close()

	if err := syscall.Flock(int(lockFD.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("获取 bus.lock 失败: %w", err)
	}
	defer func() {
		_ = syscall.Flock(int(lockFD.Fd()), syscall.LOCK_UN)
	}()

	state, err := b.load()
	if err != nil {
		return err
	}
	return fn(state)
}

// Register 将当前进程注册为 EventKey consumer。
// 返回 entry 用于后续 Unregister；重复注册（同 PID + 同 EventKey）会替换旧条目。
func (b *Bus) Register(entry ConsumerEntry) error {
	return b.withLock(func(state *BusState) error {
		// 替换或追加
		updated := state.Consumers[:0]
		for _, c := range state.Consumers {
			if c.PID == entry.PID && c.EventKey == entry.EventKey {
				continue
			}
			updated = append(updated, c)
		}
		updated = append(updated, entry)
		state.Consumers = updated
		return b.save(state)
	})
}

// Unregister 从 bus.json 移除指定 (PID, EventKey)；幂等。
func (b *Bus) Unregister(pid int, eventKey string) error {
	return b.withLock(func(state *BusState) error {
		updated := state.Consumers[:0]
		for _, c := range state.Consumers {
			if c.PID == pid && c.EventKey == eventKey {
				continue
			}
			updated = append(updated, c)
		}
		state.Consumers = updated
		return b.save(state)
	})
}

// Snapshot 返回当前 consumer 列表的拷贝（status 查询用）。
// 同时清理已不存活的 PID 记录，保持 bus.json 不积累僵尸条目。
func (b *Bus) Snapshot() (*BusState, error) {
	var snap *BusState
	err := b.withLock(func(state *BusState) error {
		alive := state.Consumers[:0]
		changed := false
		for _, c := range state.Consumers {
			if c.IsAlive() {
				alive = append(alive, c)
			} else {
				changed = true
			}
		}
		state.Consumers = alive
		if changed {
			if err := b.save(state); err != nil {
				return err
			}
		}
		// 深拷贝返回
		copyState := *state
		copyState.Consumers = append([]ConsumerEntry(nil), state.Consumers...)
		snap = &copyState
		return nil
	})
	return snap, err
}
