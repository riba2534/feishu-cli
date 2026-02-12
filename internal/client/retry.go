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
	// Context 用于支持外部取消。为 nil 时不检查取消。
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

// GetRetryWaitDuration 计算下一次重试前的等待时间。
// 优先使用服务端返回的限流重置时间（±10% 抖动）；
// 否则使用 full jitter：random(0, min(2^attempt, 30s))。
func GetRetryWaitDuration(headers http.Header, attempt int) time.Duration {
	// 尝试从 header 读取限流重置时间
	if headers != nil {
		for key, values := range headers {
			if strings.EqualFold(key, rateLimitResetHeader) && len(values) > 0 {
				if resetSec, err := strconv.ParseFloat(values[0], 64); err == nil && resetSec >= 0 {
					// 对服务端提供的等待时间做 ±10% 抖动，避免齐步醒来
					jittered := resetSec * (0.9 + rand.Float64()*0.2)
					jittered = math.Min(jittered, maxBackoffSeconds)
					return time.Duration(jittered * float64(time.Second))
				}
			}
		}
	}

	// full jitter: random(0, min(2^attempt, 30s))
	base := math.Min(math.Pow(2, float64(attempt)), maxBackoffSeconds)
	wait := rand.Float64() * base
	return time.Duration(wait * float64(time.Second))
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

	var zero T
	failureCount := 0
	rateLimitHits := 0

	for attempt := 0; attempt < cfg.MaxTotalAttempts; attempt++ {
		// 检查 context 是否已取消
		if cfg.Context != nil {
			select {
			case <-cfg.Context.Done():
				return RetryResult[T]{
					Value:         zero,
					Err:           fmt.Errorf("重试被取消: %w", cfg.Context.Err()),
					Attempts:      attempt,
					RateLimitHits: rateLimitHits,
				}
			default:
			}
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

		if cfg.Context != nil {
			timer := time.NewTimer(wait)
			select {
			case <-cfg.Context.Done():
				timer.Stop()
				return RetryResult[T]{
					Value:         zero,
					Err:           fmt.Errorf("重试等待被取消: %w", cfg.Context.Err()),
					Attempts:      attempt + 1,
					RateLimitHits: rateLimitHits,
				}
			case <-timer.C:
			}
		} else {
			time.Sleep(wait)
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
