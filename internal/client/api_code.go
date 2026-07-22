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
	return apiCodePattern(code).MatchString(err.Error())
}

// apiCodePattern 构造某错误码的匹配正则（缓存无必要：调用频率极低）。
func apiCodePattern(code int) *regexp.Regexp {
	n := strconv.Itoa(code)
	// 形态一：code=<N>（本仓统一错误格式）
	// 形态二："code": <N> / "code":<N>（HTTP 错误分支透出的 raw JSON body）
	return regexp.MustCompile(fmt.Sprintf(`(code=%s\b)|("code"\s*:\s*%s\b)`, n, n))
}
