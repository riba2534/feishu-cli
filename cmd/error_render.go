package cmd

import (
	"errors"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/apidiag"
	"github.com/riba2534/feishu-cli/v2/internal/client"
)

// errorHinter 由命令返回的错误实现，提供领域专属的修复建议；
// 根命令在诊断信息之后打印（跨领域的鉴权 / 权限建议优先）。
type errorHinter interface {
	Hint() string
}

// renderErrorDiagnostics 返回附加在错误主文本之后的诊断行（写 stderr）：
//   - 飞书错误响应里的缺失 scope、字段校验、详情、log_id、排查链接；
//   - 按业务码给出的修复建议（如 99991672 指向开放平台开通 scope，而不是重新登录）。
//
// 诊断来源：错误链上的 *client.APIError；否则用错误文本里的 code=N 匹配
// 传输层记录的最近错误响应（apidiag），覆盖仍以 "code=%d, msg=%s" 格式化错误的旧调用点。
func renderErrorDiagnostics(err error) []string {
	if err == nil {
		return nil
	}
	msg := err.Error()
	var lines []string
	info, ok := diagnosticsFor(err)
	if ok {
		for _, line := range info.Lines(msg) {
			lines = append(lines, "  "+line)
		}
	}
	hint := ""
	if ok {
		hint = client.APIErrorHint(info)
	}
	if hint == "" {
		var h errorHinter
		if errors.As(err, &h) {
			hint = h.Hint()
		}
	}
	if hint != "" && !strings.Contains(msg, hint) {
		lines = append(lines, hint)
	}
	return lines
}

func diagnosticsFor(err error) (apidiag.Info, bool) {
	if apiErr, ok := client.AsAPIError(err); ok {
		return apiErr.Info, true
	}
	return apidiag.Lookup(func(info apidiag.Info) bool {
		return client.HasAPICode(err, info.Code)
	})
}
