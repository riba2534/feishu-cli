package client

import (
	"fmt"
	"regexp"
	"strconv"
)

// HasAPICode 判断 err 的错误链文本中是否携带指定的飞书业务错误码。
//
// 本仓 client 层错误统一以 "code=<N>" 形态携带业务码（fmt.Errorf("...: code=%d, msg=%s", ...)），
// 本 helper 按 **词边界** 匹配 `code=<N>` 或 `code": <N>`（raw body 透出场景），
// 而不是裸 substring——裸搜数字会命中 log_id/token/body 里的无关同数字串造成误判。
//
// 供 cmd 层做特定错误码的分支处理（如 1062507 目录已满、2091003 妙记生成中、232033 外部群）。
func HasAPICode(err error, code int) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if !apiCodePattern(code).MatchString(msg) {
		return false
	}
	// 去掉 `status code: <N>` 这类传输层措辞后若不再命中，说明原命中来自该短语，不是业务码
	if stripped := statusCodePhrasePattern(code).ReplaceAllString(msg, " "); !apiCodePattern(code).MatchString(stripped) {
		return false
	}
	return true
}

// apiCodePattern 构造某错误码的匹配正则（缓存无必要：调用频率极低）。
//
// 匹配「业务码字段本身」的三种真实形态：
//   - code=<N>：本仓统一错误格式 fmt.Errorf("...: code=%d, msg=%s", ...)
//   - (code: <N>)：task.go 等处的 fmt.Errorf("...: %s (code: %d)", ...)
//   - "code": <N>：HTTP 错误分支透出的 raw JSON body 字段
//
// 关键：显式排除 `status code: <N>`（Go regexp 不支持负向后顾，故先匹配再排除）。
// 否则 `HTTP 400, body: {"msg":"unexpected status code: 500"}` 会被当成业务码 500，
// 使永久错误被 IsRetryableError 判成可重试而白跑几轮退避——与 log_id 同数字串误判同类。
func apiCodePattern(code int) *regexp.Regexp {
	n := strconv.Itoa(code)
	return regexp.MustCompile(fmt.Sprintf(`(?:^|[^\w])code\s*[:=]\s*%s\b|"code"\s*:\s*%s\b`, n, n))
}

// statusCodePhrasePattern 匹配 `status code: <N>` / `status_code=<N>` 这类
// **传输层**措辞，用于把它们从业务码判定中排除。
func statusCodePhrasePattern(code int) *regexp.Regexp {
	n := strconv.Itoa(code)
	return regexp.MustCompile(fmt.Sprintf(`(?i)\bstatus[\s_-]*code\s*[:=]?\s*%s\b`, n))
}

// HasHTTPStatus 判断 err 的错误链文本中是否携带指定的 HTTP 状态码（词边界安全）。
//
// 匹配形态：
//   - HTTP <N> / HTTP 状态码 <N> / HTTP status <N>（本仓统一格式）
//   - status code: <N> / status code <N>（SDK 与三方库措辞）
//
// 刻意不匹配裸 `status: <N>`——响应正文里的 `{"status": 500}` 与传输层状态无关。
func HasHTTPStatus(err error, status int) bool {
	if err == nil {
		return false
	}
	return httpStatusPattern(status).MatchString(err.Error())
}

func httpStatusPattern(status int) *regexp.Regexp {
	n := strconv.Itoa(status)
	// "status\s*code" 同时覆盖 "status code: 429" 与 SDK 非 JSON 响应错误里的 "StatusCode: 429"
	// （限流网关返回空 body 的 429 时，SDK 报 "response content-type not json, response: StatusCode: 429"）
	return regexp.MustCompile(fmt.Sprintf(`(?i)\bHTTP\s*(?:状态码|status(?:\s*code)?)?\s*[:=]?\s*%s\b|\bstatus\s*code\s*[:=]?\s*%s\b`, n, n))
}
