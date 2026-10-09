package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

// errCheckFailed 是一个 sentinel error，让 cobra 把退出码映射为非零但不打印任何错误信息。
// auth check 的 JSON 结果通过 stdout 输出，错误信息不应进入 stderr。
var errCheckFailed = errors.New("")

var authCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "检查当前 token 是否包含所需 scope",
	Long: `检查 ~/.feishu-cli/token.json 中的 User Access Token 是否满足指定 scope 需求。

用于 AI Agent 在执行搜索、消息互动、审批查询等命令前预检。
输出 JSON 到 stdout，退出码 0 表示满足，非 0 表示缺少或未登录。

输出字段:
  ok          布尔值，true 表示所有 required scope 都已授权
  granted     已包含的 scope 列表
  missing     缺失的 scope 列表
  error       失败原因（not_logged_in / token_expired，仅在未登录或过期时出现）
  suggestion  修复建议（仅在 ok=false 时出现）

--scope 可用空格或逗号分隔（可混用），重复项自动去重；suggestion 中的
auth login 命令统一输出空格分隔（OAuth 线上格式，服务端拒绝 "a,b" 形式）。

示例:
  feishu-cli auth check --scope "search:docs:read"
  feishu-cli auth check --scope "search:docs:read im:message:readonly"
  feishu-cli auth check --scope "search:docs:read,im:message:readonly"`,
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		scopeFlag, _ := cmd.Flags().GetString("scope")
		// 逗号与空白都视为分隔符：服务端只认空格分隔，"a,b" 会被当成一个不存在的 scope
		required := auth.UniqueScopeList(scopeFlag)
		if len(required) == 0 {
			return clierr.Usagef("必须通过 --scope 指定至少一个 scope（空格或逗号分隔）")
		}

		result, ok := performAuthCheck(required)
		if err := printJSON(result); err != nil {
			return err
		}
		if !ok {
			// 缺 scope / 未登录 / token 过期：鉴权类失败（退出码 3），结果已在 stdout
			return clierr.Auth(errCheckFailed)
		}
		return nil
	},
}

func performAuthCheck(required []string) (result map[string]any, ok bool) {
	token, err := auth.LoadToken()
	if err != nil || token == nil {
		return errorResult("not_logged_in", required), false
	}

	if !token.IsAccessTokenValid() && !token.IsRefreshTokenValid() {
		return errorResult("token_expired", required), false
	}

	granted, missing := auth.PartitionScopes(token.Scope, required)
	out := map[string]any{
		"ok":      len(missing) == 0,
		"granted": granted,
		"missing": missing,
	}
	if len(missing) > 0 {
		out["suggestion"] = fmt.Sprintf(
			"确认飞书开放平台已开通后，执行 feishu-cli auth login --scope %q 重新授权",
			strings.Join(missing, " "),
		)
	}
	return out, len(missing) == 0
}

func errorResult(errCode string, required []string) map[string]any {
	return map[string]any{
		"ok":         false,
		"error":      errCode,
		"missing":    required,
		"suggestion": fmt.Sprintf("feishu-cli auth login --scope %q", strings.Join(required, " ")),
	}
}

func init() {
	authCmd.AddCommand(authCheckCmd)
	authCheckCmd.Flags().String("scope", "", "待检查的 scope（空格或逗号分隔，如 \"search:docs:read im:message:readonly\"）")
	mustMarkFlagRequired(authCheckCmd, "scope")
}
