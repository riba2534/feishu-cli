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
		{"HTTP 429 with RetryOnRateLimit", fmt.Errorf("HTTP 429"), true, true, false},
		{"HTTP 429 without RetryOnRateLimit", fmt.Errorf("HTTP 429"), false, true, true},
		{"code=429 with RetryOnRateLimit", fmt.Errorf("code=429, msg=too many requests"), true, true, false},
		{"99991400 with RetryOnRateLimit", fmt.Errorf("code=99991400 frequency limit"), true, true, false},
		{"raw json body code 99991400", fmt.Errorf(`{"code": 99991400, "msg": "rate limit"}`), true, true, false},
		{"log_id contains 429 does not classify as rate limit", fmt.Errorf("code=10000, msg=failed, log_id=20260429123456"), true, false, true},
		{"token contains 429 does not classify as rate limit", fmt.Errorf("token=boxcn429abcdef error"), true, false, true},
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
	retryableTests := []error{
		fmt.Errorf("HTTP 500"),
		fmt.Errorf("HTTP 状态码 502"),
		fmt.Errorf("code=503, msg=service unavailable"),
		fmt.Errorf("code: 504, msg=gateway timeout"),
		fmt.Errorf(`{"code": 500, "msg": "internal server error"}`),
		fmt.Errorf("request failed: bad gateway"),
		fmt.Errorf("HTTP 429"),
	}

	for _, err := range retryableTests {
		d := ClassifyError(err, false)
		if !d.ShouldRetry {
			t.Errorf("可重试错误 %q 应该重试", err)
		}
		if !d.IsRealFailure {
			t.Errorf("服务端错误 %q 应计为真实失败", err)
		}
	}

	nonRetryableTests := []error{
		fmt.Errorf("创建文件夹失败: code=10000, msg=Invalid folder name, log_id=20260827123450000000000000000000"),
		fmt.Errorf("token=boxcn502abcdef upload error"),
		fmt.Errorf("token=fld503xyz not found"),
		fmt.Errorf("operation failed with code=10024, log_id=20260504123456"),
		fmt.Errorf("normal business error with log_id 500123"),
	}

	for _, err := range nonRetryableTests {
		d := ClassifyError(err, false)
		if d.ShouldRetry {
			t.Errorf("非可重试错误（如含 500/502/503 的 log_id/token）%q 不应重试", err)
		}
		if !d.IsRealFailure {
			t.Errorf("业务错误 %q 应计为真实失败", err)
		}
	}
}

func TestGetRetryWaitDuration_WithHeader(t *testing.T) {
	headers := http.Header{}
	headers.Set("x-ogw-ratelimit-reset", "5.0")

	// 只向上抖动：不得早于服务端恢复时间（5s），最多晚 10%
	for i := 0; i < 200; i++ {
		secs := GetRetryWaitDuration(headers, 0).Seconds()
		if secs < 5.0 || secs > 5.5+1e-9 {
			t.Fatalf("等待时间 %.3f 超出 [5.0, 5.5]（不得早于服务端恢复时间）", secs)
		}
	}
}

// TestGetRetryWaitDuration_ServerHints 覆盖秒数向上取整、Retry-After 两种格式、优先级与非法值回退。
func TestGetRetryWaitDuration_ServerHints(t *testing.T) {
	future := time.Now().Add(7500 * time.Millisecond).UTC().Format(http.TimeFormat)
	cases := []struct {
		name     string
		headers  http.Header
		min, max float64 // 期望范围（秒）；min<0 表示应回退到 full jitter
	}{
		{"小数秒向上取整", http.Header{"X-Ogw-Ratelimit-Reset": {"0.3"}}, 1, 1.1},
		{"未规范化的小写 key", http.Header{"x-ogw-ratelimit-reset": {"2"}}, 2, 2.2},
		{"Retry-After 秒数", http.Header{"Retry-After": {"3"}}, 3, 3.3},
		{"Retry-After HTTP-date 向上取整", http.Header{"Retry-After": {future}}, 7, 8.8},
		{"x-ogw 优先于 Retry-After", http.Header{"X-Ogw-Ratelimit-Reset": {"2"}, "Retry-After": {"9"}}, 2, 2.2},
		{"x-ogw 非法时回退 Retry-After", http.Header{"X-Ogw-Ratelimit-Reset": {"abc"}, "Retry-After": {"4"}}, 4, 4.4},
		{"超大值封顶", http.Header{"X-Ogw-Ratelimit-Reset": {"100000"}}, maxServerWaitSeconds, maxServerWaitSeconds * 1.1},
		{"0 视为无效", http.Header{"X-Ogw-Ratelimit-Reset": {"0"}}, -1, 0},
		{"过去的 HTTP-date 视为无效", http.Header{"Retry-After": {"Mon, 02 Jan 2006 15:04:05 GMT"}}, -1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i < 100; i++ {
				secs := GetRetryWaitDuration(tc.headers, 0).Seconds()
				if tc.min < 0 {
					if secs > 1.0+1e-9 { // attempt=0 → full jitter 上限 2^0 = 1s
						t.Fatalf("无效 header 应回退 full jitter（≤1s），得到 %.3f", secs)
					}
					continue
				}
				if secs < tc.min || secs > tc.max+1e-9 {
					t.Fatalf("等待时间 %.3f 超出 [%.1f, %.1f]", secs, tc.min, tc.max)
				}
			}
		})
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

	// 服务端给出的恢复时间不再被 30s 退避上限截短（否则会早于恢复时刻醒来），只受 maxServerWaitSeconds 约束
	headers := http.Header{}
	headers.Set("x-ogw-ratelimit-reset", "100.0")
	for i := 0; i < 100; i++ {
		secs := GetRetryWaitDuration(headers, 0).Seconds()
		if secs < 100 || secs > 110+1e-9 {
			t.Fatalf("header=100 时等待时间 %.2f 应在 [100, 110]", secs)
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

// TestDoWithRetry_NonIdempotentWriteSafety 验证非幂等写操作遇到包含 500/429 的普通 log_id/token 时不被误重试
func TestDoWithRetry_NonIdempotentWriteSafety(t *testing.T) {
	calls := 0
	result := DoWithRetry(func() (string, http.Header, error) {
		calls++
		// 模拟非幂等写操作返回了业务错误，但 log_id 含有 500
		return "", nil, fmt.Errorf("创建节点失败: code=10020, msg=Invalid folder, log_id=20260827123450000000000000000000")
	}, RetryConfig{
		MaxRetries: 3,
	})

	if result.Err == nil {
		t.Fatal("期望返回错误，但得到了成功")
	}
	if calls != 1 {
		t.Fatalf("非幂等错误不应因为 log_id 含有 500 而被重试，期望调用 1 次，实际调用了 %d 次", calls)
	}
}
