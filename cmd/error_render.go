package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/internal/apidiag"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
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
	if hint := errorHint(err, info, ok); hint != "" && !strings.Contains(msg, hint) {
		lines = append(lines, hint)
	}
	return lines
}

// errorHint 返回修复建议：按业务码的跨领域建议优先，其次是错误自带的领域建议。
func errorHint(err error, info apidiag.Info, hasInfo bool) string {
	hint := ""
	if hasInfo {
		hint = client.APIErrorHint(info)
	}
	if hint == "" {
		var h errorHinter
		if errors.As(err, &h) {
			hint = h.Hint()
		}
	}
	return hint
}

func diagnosticsFor(err error) (apidiag.Info, bool) {
	if apiErr, ok := client.AsAPIError(err); ok {
		return apiErr.Info, true
	}
	return apidiag.Lookup(func(info apidiag.Info) bool {
		return client.HasAPICode(err, info.Code)
	})
}

// ---------------- --error-format（opt-in 机器可读错误信封） ----------------

const (
	errorFormatText = "text"
	errorFormatJSON = "json"
)

// errorFormatValue 是全局 --error-format 的取值；Set 拒绝非法值，cobra 会按用法错误（退出码 2）报出。
type errorFormatValue string

func (v *errorFormatValue) String() string { return string(*v) }
func (v *errorFormatValue) Type() string   { return "string" }
func (v *errorFormatValue) Set(s string) error {
	switch s = strings.ToLower(strings.TrimSpace(s)); s {
	case errorFormatText, errorFormatJSON:
		*v = errorFormatValue(s)
		return nil
	}
	return fmt.Errorf("仅支持 text|json")
}

var errorFormat = errorFormatValue(errorFormatText)

// flagsParsed 由 Execute 设置：本次执行的命令是否完成了 flag 解析。未完成（未知命令、flag 解析失败）时
// pflag 可能没解析到 --error-format，需要回头扫描原始参数。
var flagsParsed bool

// flagParseError 由 flagSuggestionErrorFunc 置位：pflag 在 Parse 一开始就标记 Parsed，解析失败时也为 true，
// 需要单独记录失败。Execute 开始时复位。
var flagParseError bool

func init() {
	rootCmd.PersistentFlags().Var(&errorFormat, "error-format",
		"错误输出格式：text（默认，中文文本）| json（stderr 输出一行 JSON 错误信封，stdout 不受影响）")
}

// errorFormatIsJSON 判断本次是否输出 JSON 错误信封。
// flag 解析失败（如未知 flag 出现在 --error-format 之前）时 pflag 不会再解析后续 flag，
// 因此再扫描一遍原始参数兜底；"--" 之后的参数属于位置参数，不参与判断。
func errorFormatIsJSON(args []string) bool {
	if errorFormat == errorFormatJSON {
		return true
	}
	// flag 正常解析过时以解析结果为准：原始参数里的 "--error-format" 可能只是别的 flag 的取值（如 --text --error-format=json）
	if flagsParsed {
		return false
	}
	for i, a := range args {
		if a == "--" {
			break
		}
		if v, ok := strings.CutPrefix(a, "--error-format="); ok {
			return strings.EqualFold(strings.TrimSpace(v), errorFormatJSON)
		}
		if a == "--error-format" && i+1 < len(args) {
			return strings.EqualFold(strings.TrimSpace(args[i+1]), errorFormatJSON)
		}
	}
	return false
}

// writeCommandError 把命令失败写到 stderr。默认文本格式与历史输出逐字节一致：
// 错误主文本一行 + renderErrorDiagnostics 的诊断行；--error-format json 时改为一行 JSON 信封。
func writeCommandError(w io.Writer, err error) {
	if err == nil {
		return
	}
	if errorFormatIsJSON(os.Args[1:]) {
		raw, mErr := json.Marshal(map[string]any{"error": errorEnvelope(err)})
		if mErr == nil {
			fmt.Fprintln(w, string(raw))
			return
		}
	}
	if msg := err.Error(); msg != "" {
		fmt.Fprintln(w, msg)
	}
	for _, line := range renderErrorDiagnostics(err) {
		fmt.Fprintln(w, line)
	}
}

// errorEnvelope 构造 JSON 错误信封的 error 对象。字段只取自已有的结构化信息：
// exit_code/kind 来自 exitCodeFor 与 clierr 分类；code/log_id/missing_scopes 只取错误链上的
// *client.APIError（不从文本猜）；hint 与文本模式的修复建议同源；取不到的字段省略。
func errorEnvelope(err error) map[string]any {
	code := exitCodeFor(err)
	env := map[string]any{
		"message":   err.Error(),
		"exit_code": code,
		"kind":      errorKindName(err, code),
		"retryable": code == clierr.ExitNetwork || client.IsRateLimitError(err),
	}
	apiErr, hasAPI := client.AsAPIError(err)
	var info apidiag.Info
	if hasAPI {
		info = apiErr.Info
		if info.Code != 0 {
			env["code"] = info.Code
		}
		if info.LogID != "" {
			env["log_id"] = info.LogID
		}
		if len(info.MissingScopes) > 0 {
			env["missing_scopes"] = info.MissingScopes
		}
	}
	if hint := errorHint(err, info, hasAPI); hint != "" {
		env["hint"] = hint
	}
	return env
}

// errorKindName 返回与退出码对应的稳定分类名（usage/auth/network/confirmation_required/cancelled/interrupted/general）。
func errorKindName(err error, code int) string {
	switch code {
	case clierr.ExitUsage:
		return clierr.KindUsage.String()
	case clierr.ExitAuth:
		return clierr.KindAuth.String()
	case clierr.ExitNetwork:
		return clierr.KindNetwork.String()
	case clierr.ExitConfirmationRequired:
		return clierr.KindConfirmationRequired.String()
	case clierr.ExitInterrupted:
		return "interrupted"
	}
	for _, k := range clierr.Kinds(err) {
		if k == clierr.KindCancelled {
			return k.String()
		}
	}
	return clierr.KindGeneral.String()
}
