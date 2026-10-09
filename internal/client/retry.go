package client

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/v2/internal/runctx"
)

// 限流重置 header 名称
const rateLimitResetHeader = "x-ogw-ratelimit-reset"

// 退避上限（秒），防止等待时间过长
const maxBackoffSeconds = 30.0

// RetryConfig 重试配置
type RetryConfig struct {
	// MaxRetries 最大重试次数（不含首次调用）。
	// 当 RetryOnRateLimit=true 时，429 错误不计入此计数。
	MaxRetries int
	// MaxTotalAttempts 最大总尝试次数（含首次），防止死循环。默认 20。
	MaxTotalAttempts int
	// RetryOnRateLimit 为 true 时，429/99991400 限流错误不计入 MaxRetries，
	// 仅受 MaxTotalAttempts 约束。
	RetryOnRateLimit bool
	// IsPermanent 自定义永久性错误判断函数。返回 true 表示不应重试。
	// 为 nil 时使用 helpers.go 中的 IsPermanentError。
	IsPermanent func(error) bool
	// OnRetry 每次重试前的回调，可用于日志输出。attempt 从 1 开始。
	OnRetry func(attempt int, err error, wait time.Duration)
	// Context 用于支持外部取消。为 nil 时使用进程根 context（runctx.Root()，Ctrl-C 时取消）。
	Context context.Context
}

// RetryResult 重试执行结果
type RetryResult[T any] struct {
	Value         T
	Err           error
	Attempts      int // 总尝试次数（含首次）
	RateLimitHits int // 触发限流的次数
}

// RetryDecision 错误分类结果
type RetryDecision struct {
	ShouldRetry   bool // 是否应该重试
	IsRealFailure bool // 是否计入失败次数（RetryOnRateLimit=true 时 429 不算失败）
}

// ClassifyError 对错误进行分类，决定是否重试以及是否计入失败次数。
// 复用 helpers.go 中 IsRateLimitError/IsRetryableError/IsPermanentError 的逻辑。
func ClassifyError(err error, retryOnRateLimit bool) RetryDecision {
	if err == nil {
		return RetryDecision{ShouldRetry: false, IsRealFailure: false}
	}

	// 限流错误
	if IsRateLimitError(err) {
		return RetryDecision{
			ShouldRetry:   true,
			IsRealFailure: !retryOnRateLimit, // retryOnRateLimit=true 时不计入失败
		}
	}

	// 永久性错误（语法错误等）
	if IsPermanentError(err) {
		return RetryDecision{ShouldRetry: false, IsRealFailure: true}
	}

	// 可重试的服务端错误（5xx 等）
	if IsRetryableError(err) {
		return RetryDecision{ShouldRetry: true, IsRealFailure: true}
	}

	// 未知错误默认不重试
	return RetryDecision{ShouldRetry: false, IsRealFailure: true}
}

// maxServerWaitSeconds 服务端给出的恢复时间上限（秒），防止异常的超大值让命令长时间挂起。
// 只对服务端时间生效；超过上限时提前醒来最多再触发一次限流，由重试循环继续处理。
const maxServerWaitSeconds = 120

// GetRetryWaitDuration 计算下一次重试前的等待时间。
// 优先使用服务端给出的恢复时间（x-ogw-ratelimit-reset，其次标准 Retry-After 的秒数或 HTTP-date）：
// 秒数向上取整后只向上抖动 0~10%，保证不会早于服务端恢复时刻醒来、又避免多进程齐步重试；
// 无服务端时间时使用 full jitter：random(0, min(2^attempt, 30s))。
func GetRetryWaitDuration(headers http.Header, attempt int) time.Duration {
	if wait, ok := serverRetryWait(headers, time.Now()); ok {
		return wait
	}
	// full jitter: random(0, min(2^attempt, 30s))
	base := math.Min(math.Pow(2, float64(attempt)), maxBackoffSeconds)
	wait := rand.Float64() * base
	return time.Duration(wait * float64(time.Second))
}

// serverRetryWait 解析服务端给出的恢复时间并加上向上抖动；无有效值时返回 ok=false。
func serverRetryWait(headers http.Header, now time.Time) (time.Duration, bool) {
	seconds, ok := parseRateLimitReset(headerValue(headers, rateLimitResetHeader))
	if !ok {
		seconds, ok = parseRetryAfter(headerValue(headers, "Retry-After"), now)
	}
	if !ok {
		return 0, false
	}
	if seconds > maxServerWaitSeconds {
		seconds = maxServerWaitSeconds
	}
	base := time.Duration(seconds) * time.Second
	// 只向上抖动：[base, base*1.1]
	return base + time.Duration(rand.Float64()*0.1*float64(base)), true
}

// parseRateLimitReset 解析 x-ogw-ratelimit-reset（剩余恢复秒数，允许小数），向上取整；非正数视为无效。
func parseRateLimitReset(v string) (int64, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil || f <= 0 || math.IsInf(f, 0) || math.IsNaN(f) {
		return 0, false
	}
	return int64(math.Ceil(math.Min(f, maxServerWaitSeconds))), true
}

// parseRetryAfter 解析标准 Retry-After：delta-seconds 或 HTTP-date（RFC 9110 §10.2.3），向上取整到秒。
func parseRetryAfter(v string, now time.Time) (int64, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		if n <= 0 {
			return 0, false
		}
		return n, true
	}
	at, err := http.ParseTime(v)
	if err != nil {
		return 0, false
	}
	delay := at.Sub(now)
	if delay <= 0 {
		return 0, false
	}
	return int64(math.Ceil(delay.Seconds())), true
}

// headerValue 大小写不敏感地读取 header（兼容未规范化 key 的手工构造 header）。
func headerValue(headers http.Header, name string) string {
	if headers == nil {
		return ""
	}
	if v := headers.Get(name); v != "" {
		return v
	}
	for key, values := range headers {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

// DoWithRetry 泛型重试执行器。
// fn 返回 (结果, HTTP 响应 header, 错误)。header 可为 nil（如无法获取）。
func DoWithRetry[T any](fn func() (T, http.Header, error), cfg RetryConfig) RetryResult[T] {
	if cfg.MaxTotalAttempts <= 0 {
		cfg.MaxTotalAttempts = 20
	}
	isPermanent := cfg.IsPermanent
	if isPermanent == nil {
		isPermanent = IsPermanentError
	}

	// 未指定 Context 时使用进程根 context：Ctrl-C 能打断退避等待，而不是睡满整个间隔
	ctx := cfg.Context
	if ctx == nil {
		ctx = runctx.Root()
	}

	var zero T
	failureCount := 0
	rateLimitHits := 0

	for attempt := 0; attempt < cfg.MaxTotalAttempts; attempt++ {
		// 检查 context 是否已取消
		select {
		case <-ctx.Done():
			return RetryResult[T]{
				Value:         zero,
				Err:           fmt.Errorf("重试被取消: %w", ctx.Err()),
				Attempts:      attempt,
				RateLimitHits: rateLimitHits,
			}
		default:
		}

		value, headers, err := fn()
		if err == nil {
			return RetryResult[T]{
				Value:         value,
				Err:           nil,
				Attempts:      attempt + 1,
				RateLimitHits: rateLimitHits,
			}
		}

		// 自定义永久性错误判断
		if isPermanent(err) {
			return RetryResult[T]{
				Value:         zero,
				Err:           err,
				Attempts:      attempt + 1,
				RateLimitHits: rateLimitHits,
			}
		}

		decision := ClassifyError(err, cfg.RetryOnRateLimit)

		if IsRateLimitError(err) {
			rateLimitHits++
		}

		if decision.IsRealFailure {
			failureCount++
		}

		// 检查是否应该重试
		if !decision.ShouldRetry {
			return RetryResult[T]{
				Value:         zero,
				Err:           err,
				Attempts:      attempt + 1,
				RateLimitHits: rateLimitHits,
			}
		}

		// 检查失败次数是否超过上限
		if failureCount > cfg.MaxRetries {
			return RetryResult[T]{
				Value:         zero,
				Err:           fmt.Errorf("重试 %d 次后仍失败: %w", failureCount, err),
				Attempts:      attempt + 1,
				RateLimitHits: rateLimitHits,
			}
		}

		// 计算等待时间并休眠
		wait := GetRetryWaitDuration(headers, attempt)
		if cfg.OnRetry != nil {
			cfg.OnRetry(attempt+1, err, wait)
		}

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return RetryResult[T]{
				Value:         zero,
				Err:           fmt.Errorf("重试等待被取消: %w", ctx.Err()),
				Attempts:      attempt + 1,
				RateLimitHits: rateLimitHits,
			}
		case <-timer.C:
		}
	}

	return RetryResult[T]{
		Value:         zero,
		Err:           fmt.Errorf("达到最大总尝试次数 %d", cfg.MaxTotalAttempts),
		Attempts:      cfg.MaxTotalAttempts,
		RateLimitHits: rateLimitHits,
	}
}

// DoVoidWithRetry 无返回值版本的重试执行器。
func DoVoidWithRetry(fn func() (http.Header, error), cfg RetryConfig) RetryResult[struct{}] {
	return DoWithRetry(func() (struct{}, http.Header, error) {
		headers, err := fn()
		return struct{}{}, headers, err
	}, cfg)
}
