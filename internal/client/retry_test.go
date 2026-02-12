package client

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"testing"
	"time"
)

func TestDoWithRetry_Success(t *testing.T) {
	calls := 0
	result := DoWithRetry(func() (string, http.Header, error) {
		calls++
		return "ok", nil, nil
	}, RetryConfig{MaxRetries: 3})

	if result.Err != nil {
		t.Fatalf("期望成功，得到错误: %v", result.Err)
	}
	if result.Value != "ok" {
		t.Fatalf("期望 ok，得到 %s", result.Value)
	}
	if calls != 1 {
		t.Fatalf("期望调用 1 次，实际 %d 次", calls)
	}
	if result.Attempts != 1 {
		t.Fatalf("期望 Attempts=1，得到 %d", result.Attempts)
	}
	if result.RateLimitHits != 0 {
		t.Fatalf("期望 RateLimitHits=0，得到 %d", result.RateLimitHits)
	}
}

func TestDoWithRetry_RetryThenSuccess(t *testing.T) {
	calls := 0
	result := DoWithRetry(func() (string, http.Header, error) {
		calls++
		if calls <= 2 {
			return "", nil, fmt.Errorf("rate limit 429")
		}
		return "ok", nil, nil
	}, RetryConfig{
		MaxRetries:       3,
		RetryOnRateLimit: true,
	})

	if result.Err != nil {
		t.Fatalf("期望成功，得到错误: %v", result.Err)
	}
	if result.Value != "ok" {
		t.Fatalf("期望 ok，得到 %s", result.Value)
	}
	if calls != 3 {
		t.Fatalf("期望调用 3 次，实际 %d 次", calls)
	}
	if result.RateLimitHits != 2 {
		t.Fatalf("期望 RateLimitHits=2，得到 %d", result.RateLimitHits)
	}
}

func TestDoWithRetry_PermanentError(t *testing.T) {
	calls := 0
	result := DoWithRetry(func() (string, http.Header, error) {
		calls++
		return "", nil, fmt.Errorf("Parse error: invalid syntax")
	}, RetryConfig{MaxRetries: 5})

	if result.Err == nil {
		t.Fatal("期望错误，得到成功")
	}
	if calls != 1 {
		t.Fatalf("永久性错误不应重试，期望调用 1 次，实际 %d 次", calls)
	}
}

func TestDoWithRetry_MaxRetriesExhausted(t *testing.T) {
	calls := 0
	result := DoWithRetry(func() (string, http.Header, error) {
		calls++
		return "", nil, fmt.Errorf("internal error 500")
	}, RetryConfig{MaxRetries: 2, MaxTotalAttempts: 20})

	if result.Err == nil {
		t.Fatal("期望错误，得到成功")
	}
	// 首次调用 + 2 次重试 = 3 次，第 3 次失败后 failureCount=3 > MaxRetries=2
	if calls != 3 {
		t.Fatalf("期望调用 3 次（1 首次 + 2 重试），实际 %d 次", calls)
	}
}

func TestDoWithRetry_MaxTotalAttempts(t *testing.T) {
	calls := 0
	result := DoWithRetry(func() (string, http.Header, error) {
		calls++
		return "", nil, fmt.Errorf("rate limit 429")
	}, RetryConfig{
		MaxRetries:       100,
		MaxTotalAttempts: 5,
		RetryOnRateLimit: true,
	})

	if result.Err == nil {
		t.Fatal("期望错误，得到成功")
	}
	if calls != 5 {
		t.Fatalf("期望总尝试 5 次，实际 %d 次", calls)
	}
	if result.RateLimitHits != 5 {
		t.Fatalf("期望 RateLimitHits=5，得到 %d", result.RateLimitHits)
	}
}

func TestDoWithRetry_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	result := DoWithRetry(func() (string, http.Header, error) {
		calls++
		return "", nil, fmt.Errorf("internal error 500")
	}, RetryConfig{
		MaxRetries:       100,
		MaxTotalAttempts: 100,
		Context:          ctx,
	})

	if result.Err == nil {
		t.Fatal("期望取消错误，得到成功")
	}
	// 应该在 context 取消后很快停止
	if calls > 10 {
		t.Fatalf("context 取消后不应继续重试过多次，实际 %d 次", calls)
	}
}

func TestDoVoidWithRetry_Success(t *testing.T) {
	calls := 0
	result := DoVoidWithRetry(func() (http.Header, error) {
		calls++
		return nil, nil
	}, RetryConfig{MaxRetries: 3})

	if result.Err != nil {
		t.Fatalf("期望成功，得到错误: %v", result.Err)
	}
	if calls != 1 {
		t.Fatalf("期望调用 1 次，实际 %d 次", calls)
	}
}

func TestClassifyError_RateLimit(t *testing.T) {
	tests := []struct {
		name             string
		err              error
		retryOnRateLimit bool
		wantRetry        bool
		wantFailure      bool
	}{
		{"429 with RetryOnRateLimit", fmt.Errorf("429"), true, true, false},
		{"429 without RetryOnRateLimit", fmt.Errorf("429"), false, true, true},
		{"99991400 with RetryOnRateLimit", fmt.Errorf("99991400 frequency limit"), true, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := ClassifyError(tt.err, tt.retryOnRateLimit)
			if d.ShouldRetry != tt.wantRetry {
				t.Errorf("ShouldRetry: 期望 %v，得到 %v", tt.wantRetry, d.ShouldRetry)
			}
			if d.IsRealFailure != tt.wantFailure {
				t.Errorf("IsRealFailure: 期望 %v，得到 %v", tt.wantFailure, d.IsRealFailure)
			}
		})
	}
}

func TestClassifyError_Permanent(t *testing.T) {
	tests := []error{
		fmt.Errorf("Parse error: invalid syntax"),
		fmt.Errorf("Invalid request parameter"),
	}

	for _, err := range tests {
		d := ClassifyError(err, false)
		if d.ShouldRetry {
			t.Errorf("永久性错误 %q 不应重试", err)
		}
		if !d.IsRealFailure {
			t.Errorf("永久性错误 %q 应计为真实失败", err)
		}
	}
}

func TestClassifyError_Retryable(t *testing.T) {
	tests := []error{
		fmt.Errorf("500 internal error"),
		fmt.Errorf("502 bad gateway"),
		fmt.Errorf("503 service unavailable"),
	}

	for _, err := range tests {
		d := ClassifyError(err, false)
		if !d.ShouldRetry {
			t.Errorf("可重试错误 %q 应该重试", err)
		}
		if !d.IsRealFailure {
			t.Errorf("服务端错误 %q 应计为真实失败", err)
		}
	}
}

func TestGetRetryWaitDuration_WithHeader(t *testing.T) {
	headers := http.Header{}
	headers.Set("x-ogw-ratelimit-reset", "5.0")

	// 运行多次验证抖动范围
	for i := 0; i < 100; i++ {
		wait := GetRetryWaitDuration(headers, 0)
		secs := wait.Seconds()
		// 5.0 * 0.9 = 4.5, 5.0 * 1.1 = 5.5
		if secs < 4.4 || secs > 5.6 {
			t.Fatalf("等待时间 %.2f 超出 ±10%% 抖动范围 [4.5, 5.5]", secs)
		}
	}
}

func TestGetRetryWaitDuration_NoHeader(t *testing.T) {
	// attempt=3 → base = min(2^3, 30) = 8
	for i := 0; i < 100; i++ {
		wait := GetRetryWaitDuration(nil, 3)
		secs := wait.Seconds()
		if secs < 0 || secs > 8.1 {
			t.Fatalf("等待时间 %.2f 超出 full jitter 范围 [0, 8]", secs)
		}
	}
}

func TestGetRetryWaitDuration_Cap(t *testing.T) {
	// attempt=100 → base = min(2^100, 30) = 30
	for i := 0; i < 100; i++ {
		wait := GetRetryWaitDuration(nil, 100)
		secs := wait.Seconds()
		if secs > maxBackoffSeconds+0.1 {
			t.Fatalf("等待时间 %.2f 超过上限 %.0f 秒", secs, maxBackoffSeconds)
		}
	}

	// header 值超大也应被截断
	headers := http.Header{}
	headers.Set("x-ogw-ratelimit-reset", "100.0")
	for i := 0; i < 100; i++ {
		wait := GetRetryWaitDuration(headers, 0)
		secs := wait.Seconds()
		if secs > maxBackoffSeconds+0.1 {
			t.Fatalf("header 超大值时等待时间 %.2f 超过上限 %.0f 秒", secs, maxBackoffSeconds)
		}
	}
}

// TestDoWithRetry_CustomIsPermanent 验证自定义永久性错误判断
func TestDoWithRetry_CustomIsPermanent(t *testing.T) {
	calls := 0
	result := DoWithRetry(func() (string, http.Header, error) {
		calls++
		return "", nil, fmt.Errorf("custom fatal error")
	}, RetryConfig{
		MaxRetries: 5,
		IsPermanent: func(err error) bool {
			return err != nil && err.Error() == "custom fatal error"
		},
	})

	if result.Err == nil {
		t.Fatal("期望错误，得到成功")
	}
	if calls != 1 {
		t.Fatalf("自定义永久性错误不应重试，期望调用 1 次，实际 %d 次", calls)
	}
}

// TestDoWithRetry_OnRetryCallback 验证重试回调被正确调用
func TestDoWithRetry_OnRetryCallback(t *testing.T) {
	retryAttempts := []int{}
	calls := 0
	DoWithRetry(func() (string, http.Header, error) {
		calls++
		if calls <= 2 {
			return "", nil, fmt.Errorf("internal error 500")
		}
		return "ok", nil, nil
	}, RetryConfig{
		MaxRetries: 5,
		OnRetry: func(attempt int, err error, wait time.Duration) {
			retryAttempts = append(retryAttempts, attempt)
		},
	})

	if len(retryAttempts) != 2 {
		t.Fatalf("期望回调 2 次，实际 %d 次", len(retryAttempts))
	}
	if retryAttempts[0] != 1 || retryAttempts[1] != 2 {
		t.Fatalf("期望回调 attempt [1, 2]，得到 %v", retryAttempts)
	}
}

// TestClassifyError_Nil 验证 nil 错误的分类
func TestClassifyError_Nil(t *testing.T) {
	d := ClassifyError(nil, false)
	if d.ShouldRetry || d.IsRealFailure {
		t.Fatal("nil 错误不应重试也不应计为失败")
	}
}

// TestGetRetryWaitDuration_ExponentialGrowth 验证指数增长符合预期
func TestGetRetryWaitDuration_ExponentialGrowth(t *testing.T) {
	// 多次采样，验证 attempt 越大，最大可能等待时间越长
	maxWait := [5]float64{}
	for attempt := 0; attempt < 5; attempt++ {
		for i := 0; i < 200; i++ {
			w := GetRetryWaitDuration(nil, attempt).Seconds()
			if w > maxWait[attempt] {
				maxWait[attempt] = w
			}
		}
	}

	for attempt := 0; attempt < 4; attempt++ {
		expectedBase := math.Min(math.Pow(2, float64(attempt)), maxBackoffSeconds)
		nextBase := math.Min(math.Pow(2, float64(attempt+1)), maxBackoffSeconds)
		if nextBase > expectedBase && maxWait[attempt+1] < maxWait[attempt]*0.5 {
			t.Errorf("attempt %d 的最大等待时间 %.2f 不应明显小于 attempt %d 的 %.2f",
				attempt+1, maxWait[attempt+1], attempt, maxWait[attempt])
		}
	}
}
