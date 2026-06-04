package client

import (
	"context"
	"sync"
	"time"
)

// 飞书 docx 写类 API（CreateBlock/UpdateBlock/BatchUpdate/InsertTableRow/DeleteBlocks 等）
// 受单文档 3 QPS 限制：https://open.feishu.cn/document/server-docs/docs/docs/docx-v1/document-block/batch_update
// 多个 worker（图表 / 表格 / 媒体）并发写同一文档时，必须共享同一个 limiter 才能避免 99991400 / HTTP 429。
//
// 这里实现一个最朴素的 per-documentID token bucket：每秒发 N 个 token，桶容量 N（允许小爆发）。
// 故意不引入 golang.org/x/time/rate，避免新依赖。
const (
	defaultDocWriteQPS    = 3
	defaultDocWriteBurst  = 3
	docLimiterIdleEvict   = 10 * time.Minute // 长时间空闲的 limiter 自动回收
	docLimiterEvictPeriod = 5 * time.Minute  // 后台清理周期（仅在调用时触发，无独立 goroutine）
	// DocWriteSlotAcquireTimeout 等待文档写入配额（限流排队）的最长时间，刻意 >> 单次 HTTP 的 30s：
	// 限流排队不应与 HTTP 截止时间耦合，否则重度并发（大量 per-cell 写抢同一文档 3 QPS 桶）排队超 30s 时
	// acquire 会 deadline exceeded，把本该重试的写直接判死。5 分钟远超任何合理排队（实测千次 acquire 最大 ~30s）。
	DocWriteSlotAcquireTimeout = 5 * time.Minute
)

type docWriteLimiter struct {
	qps    int
	burst  int
	mu     sync.Mutex
	tokens float64
	last   time.Time
	idleAt time.Time // 最后一次 acquire 的时间
}

func newDocWriteLimiter(qps, burst int) *docWriteLimiter {
	if qps <= 0 {
		qps = defaultDocWriteQPS
	}
	if burst <= 0 {
		burst = defaultDocWriteBurst
	}
	return &docWriteLimiter{
		qps:    qps,
		burst:  burst,
		tokens: float64(burst),
		last:   time.Now(),
	}
}

// acquire 等待直到拿到一个 token，或 ctx 被取消。
func (l *docWriteLimiter) acquire(ctx context.Context) error {
	for {
		wait, ok := l.tryTake()
		if ok {
			return nil
		}
		if ctx == nil {
			time.Sleep(wait)
			continue
		}
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}

// tryTake 尝试取一个 token；返回距离下个可用 token 的等待时长（仅当 ok=false 时有效）。
func (l *docWriteLimiter) tryTake() (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if elapsed := now.Sub(l.last).Seconds(); elapsed > 0 {
		l.tokens = min(l.tokens+elapsed*float64(l.qps), float64(l.burst))
		l.last = now
	}
	l.idleAt = now

	if l.tokens >= 1 {
		l.tokens--
		return 0, true
	}
	deficit := 1 - l.tokens
	wait := max(
		time.Duration(deficit/float64(l.qps)*float64(time.Second)),
		time.Millisecond,
	)
	return wait, false
}

// 全局注册表：documentID -> *docWriteLimiter
var (
	docWriteLimiters     sync.Map
	docLimiterLastEvict  time.Time
	docLimiterEvictMutex sync.Mutex
)

// docLimiterQPSOverride 由 SetDocWriteQPS 修改，覆盖后续新建 limiter 的 QPS。
// 0 表示使用默认 defaultDocWriteQPS。仅供测试用。
var docLimiterQPSOverride int

// AcquireDocWriteSlot 等待获取一次写文档的配额。documentID 为空时直接返回（不限速）。
//
// 调用约定：CreateBlock / UpdateBlock / DeleteBlocks / BatchUpdateBlocks 4 个底层
// docx 写函数自带 acquire（见 docx.go），所有间接调用者（InsertTableRow、
// AppendTableRows、ReplaceImage、fillCell* 等）都自动受限，无需重复调用。
//
// 与 retry 的关系：DoVoidWithRetry 在 429 触发时会按 x-ogw-ratelimit-reset header
// 做一次 backoff sleep，然后再次执行用户闭包，闭包再调底层写函数 → 又过一次 acquire。
// 两段串联会产生顺序叠加，但 limiter 桶按 3 token/s 速率回补，retry sleep 期间
// 通常已经回满，第二次 acquire 多在 0~50ms 内返回，单次开销可接受。
//
// 跨进程：limiter 仅约束本进程；多 CLI 实例并发写同一文档时仍可能触发服务端 429，
// 因此 retry-on-429 必须保留作为兜底。
func AcquireDocWriteSlot(ctx context.Context, documentID string) error {
	if documentID == "" {
		return nil
	}
	maybeEvictIdleLimiters()
	if v, ok := docWriteLimiters.Load(documentID); ok {
		return v.(*docWriteLimiter).acquire(ctx)
	}
	qps := defaultDocWriteQPS
	if docLimiterQPSOverride > 0 {
		qps = docLimiterQPSOverride
	}
	v, _ := docWriteLimiters.LoadOrStore(documentID, newDocWriteLimiter(qps, qps))
	return v.(*docWriteLimiter).acquire(ctx)
}

// SetDocWriteQPS 调整后续新建 limiter 的 QPS（已存在的不变）。
// 仅供测试 / 高级用户使用。传入非正值时无效。
func SetDocWriteQPS(qps int) {
	if qps <= 0 {
		return
	}
	docLimiterQPSOverride = qps
}

// ResetDocWriteLimiters 清空所有已注册的 limiter，仅供单测使用。
func ResetDocWriteLimiters() {
	docWriteLimiters.Range(func(k, _ any) bool {
		docWriteLimiters.Delete(k)
		return true
	})
}

// maybeEvictIdleLimiters 在调用 AcquireDocWriteSlot 时顺手清理空闲过久的 limiter，
// 避免长跑进程对短期高频写入大量文档时常驻内存。
func maybeEvictIdleLimiters() {
	docLimiterEvictMutex.Lock()
	if time.Since(docLimiterLastEvict) < docLimiterEvictPeriod {
		docLimiterEvictMutex.Unlock()
		return
	}
	docLimiterLastEvict = time.Now()
	docLimiterEvictMutex.Unlock()

	cutoff := time.Now().Add(-docLimiterIdleEvict)
	docWriteLimiters.Range(func(k, v any) bool {
		l, ok := v.(*docWriteLimiter)
		if !ok {
			docWriteLimiters.Delete(k)
			return true
		}
		l.mu.Lock()
		idleAt := l.idleAt
		l.mu.Unlock()
		if !idleAt.IsZero() && idleAt.Before(cutoff) {
			docWriteLimiters.Delete(k)
		}
		return true
	})
}
