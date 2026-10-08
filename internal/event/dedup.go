package event

import (
	"sync"
	"time"
)

// 事件去重参数（对齐官方 internal/event/dedup.go）：
//   - TTL 5 分钟：飞书长连接在未及时 ACK、断线重连等情况下会重投同一 event_id，
//     重投窗口远小于 5 分钟；
//   - 环形缓冲 10000：限制 map 上限，避免长时间运行时内存无界增长。
const (
	defaultDedupTTL      = 5 * time.Minute
	defaultDedupRingSize = 10000
)

// dedupFilter 按 event_id 去重。seen 是唯一判定依据；ring 只用于按 FIFO 淘汰，限制 seen 大小。
type dedupFilter struct {
	mu   sync.Mutex
	seen map[string]time.Time
	ring []string
	pos  int
	ttl  time.Duration
	now  func() time.Time // 测试注入
}

func newDedupFilter() *dedupFilter {
	return newDedupFilterWithSize(defaultDedupRingSize, defaultDedupTTL)
}

func newDedupFilterWithSize(ringSize int, ttl time.Duration) *dedupFilter {
	if ringSize <= 0 {
		ringSize = defaultDedupRingSize
	}
	return &dedupFilter{
		seen: make(map[string]time.Time),
		ring: make([]string, ringSize),
		ttl:  ttl,
		now:  time.Now,
	}
}

// isDuplicate 报告 eventID 是否在 TTL 内出现过；首次出现时登记并返回 false。
// 空 eventID 不参与去重（无法判定，宁可放行）。
func (d *dedupFilter) isDuplicate(eventID string) bool {
	if d == nil || eventID == "" {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	now := d.now()
	if ts, ok := d.seen[eventID]; ok {
		if now.Sub(ts) < d.ttl {
			return true
		}
		delete(d.seen, eventID)
	}
	d.seen[eventID] = now

	// 环形缓冲满一圈后淘汰最旧的 ID，保证 seen 最多 len(ring) 项。
	if old := d.ring[d.pos]; old != "" && old != eventID {
		delete(d.seen, old)
	}
	d.ring[d.pos] = eventID
	d.pos = (d.pos + 1) % len(d.ring)
	return false
}
