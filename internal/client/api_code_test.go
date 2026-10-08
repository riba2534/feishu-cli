package client

import (
	"fmt"
	"testing"
)

func TestHasAPICode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
		want bool
	}{
		{"标准 code= 形态", fmt.Errorf("获取失败: code=2091003, msg=生成中"), 2091003, true},
		{"SDK code: 形态", fmt.Errorf("code: 99991400, msg: request trigger frequency limit"), 99991400, true},
		{"包装后仍命中", fmt.Errorf("外层: %w", fmt.Errorf("code=1062507, msg=full")), 1062507, true},
		{"raw body JSON 形态", fmt.Errorf(`HTTP 400, body: {"code": 232033,"msg":"x"}`), 232033, true},
		{"log_id 同数字串不误判", fmt.Errorf(`code=99991679, msg=x, log_id=20260722091003ABC`), 2091003, false},
		{"数字是前缀不误判", fmt.Errorf("code=10625071, msg=x"), 1062507, false},
		{"nil 错误", nil, 1062507, false},
		{"无关错误", fmt.Errorf("网络超时"), 232033, false},
		{"log_id 含 429 不误判为 code 429", fmt.Errorf("code=10000, msg=fail, log_id=20260429123456"), 429, false},
		{"log_id 含 500 不误判为 code 500", fmt.Errorf("code=10000, msg=fail, log_id=20260827123450000000000000000000"), 500, false},
		// 回归：`status code: <N>` 是传输层措辞，不得当作业务码，否则永久 4xx 被判可重试
		{"body 内 status code: 500 不判为业务码 500", fmt.Errorf(`HTTP 400, body: {"code":1061045,"msg":"unexpected status code: 500 from backend"}`), 500, false},
		{"status_code=503 不判为业务码 503", fmt.Errorf(`失败: status_code=503`), 503, false},
		{"(code: N) 括号形态命中", fmt.Errorf("创建任务失败: server busy (code: 1470403)"), 1470403, true},
	}
	for _, c := range cases {
		if got := HasAPICode(c.err, c.code); got != c.want {
			t.Errorf("%s: HasAPICode(%v, %d) = %v, want %v", c.name, c.err, c.code, got, c.want)
		}
	}
}

func TestHasHTTPStatus(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		want   bool
	}{
		{"HTTP 500", fmt.Errorf("HTTP 500, body: internal error"), 500, true},
		{"HTTP 状态码 502", fmt.Errorf("下载失败: HTTP 状态码 502"), 502, true},
		{"status code: 429", fmt.Errorf("request failed: status code: 429"), 429, true},
		{"status code 503", fmt.Errorf("status code 503"), 503, true},
		{"SDK 非 JSON 429", fmt.Errorf("更新块失败: response content-type not json, response: StatusCode: 429, Header:map[], Body: <binary> len 0"), 429, true},
		{"nil 错误", nil, 500, false},
		{"log_id 含 500 不误判", fmt.Errorf("code=10000, msg=fail, log_id=20260827123450000000000000000000"), 500, false},
		{"log_id 含 429 不误判", fmt.Errorf("code=10000, msg=fail, log_id=20260429123456"), 429, false},
		{"token 含 502 不误判", fmt.Errorf("token=boxcn502abcdef"), 502, false},
	}
	for _, c := range cases {
		if got := HasHTTPStatus(c.err, c.status); got != c.want {
			t.Errorf("%s: HasHTTPStatus(%v, %d) = %v, want %v", c.name, c.err, c.status, got, c.want)
		}
	}
}

// TestIsRetryableError_NoFalseRetryOn4xx 验证携带 4xx 状态的永久错误不因 body 内容被判为可重试。
// 回归防护：`HTTP 400, body: {"msg":"unexpected status code: 500"}` 曾被判 retryable，
// 让一个永久性 400 白跑几轮退避后才暴露真实错误。
func TestIsRetryableError_NoFalseRetryOn4xx(t *testing.T) {
	notRetryable := []struct {
		name string
		err  error
	}{
		{"硬 400 + body 内含 status code: 500", fmt.Errorf(`上传失败: HTTP 400, body: {"code":1061045,"msg":"unexpected status code: 500 from backend"}`)},
		{"硬 404 + body 内含 gateway timeout", fmt.Errorf(`HTTP 404, body: {"msg":"gateway timeout 504 hint"}`)},
		{"硬 403 + body 内含 internal error", fmt.Errorf(`HTTP 403, body: {"msg":"internal error at upstream"}`)},
		{"nil", nil},
	}
	for _, c := range notRetryable {
		if IsRetryableError(c.err) {
			t.Errorf("%s: 应判为不可重试", c.name)
		}
	}

	retryable := []struct {
		name string
		err  error
	}{
		{"真 HTTP 503", fmt.Errorf(`失败: HTTP 503, body: {"msg":"unavailable"}`)},
		{"真 HTTP 500", fmt.Errorf(`失败: HTTP 500, body: internal error`)},
		{"业务码 code=502", fmt.Errorf(`调用失败: code=502, msg=bad gateway`)},
		{"限流 429", fmt.Errorf(`失败: HTTP 429, body: {"code":99991400,"msg":"request trigger frequency limit"}`)},
		{"无状态码但短语明确", fmt.Errorf(`service unavailable`)},
	}
	for _, c := range retryable {
		if !IsRetryableError(c.err) {
			t.Errorf("%s: 应判为可重试", c.name)
		}
	}
}
