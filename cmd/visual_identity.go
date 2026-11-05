package cmd

import (
	"strings"

	"github.com/spf13/cobra"
)

// slides / board 写命令的身份选择。
//
// 这两组写命令历史上默认 Bot：只有显式传 --user-access-token 或设置
// FEISHU_USER_ACCESS_TOKEN 时才切到 User Token（resolveOptionalUserToken）。
// --as 只做 additive：不传时完全保持原行为；显式传入时按 resolveIdentityToken 的统一语义
// （bot 强制 App Token；user 强制 User Token，含登录态 token.json；auto 为 User 优先、
// 未配置回退 Bot、已配置但解析/刷新失败 fail-closed）。

const visualWriteAsFlagHelp = "身份: bot(默认，App Token；不传 --as 时显式 --user-access-token / FEISHU_USER_ACCESS_TOKEN 仍生效) | user(User Token，含 auth login 登录态) | auto(User 优先；未配置回退 Bot；已配置但解析/刷新失败 fail-closed)"

// addVisualWriteAsFlag 给 slides / board 写命令注册 --as（默认 bot）。
func addVisualWriteAsFlag(cmd *cobra.Command) {
	cmd.Flags().String("as", "bot", visualWriteAsFlagHelp)
}

func visualAsChanged(cmd *cobra.Command) bool {
	f := cmd.Flags().Lookup("as")
	return f != nil && f.Changed
}

// resolveVisualWriteToken 解析 slides / board 写命令的身份。返回空串表示 App Token。
// 调用顺序：validateIdentityAs（dry-run 之前）→ config.Validate（dry-run 之后）→ 本函数。
func resolveVisualWriteToken(cmd *cobra.Command) (string, error) {
	if !visualAsChanged(cmd) {
		return resolveOptionalUserToken(cmd), nil
	}
	return resolveIdentityToken(cmd)
}

// visualDryRunIdentity 在显式传了 --as 时返回 dry-run 预览要展示的身份（不解析、不刷新 token）。
// 未传 --as 时返回 ok=false，dry-run 输出保持原结构不变。
func visualDryRunIdentity(cmd *cobra.Command) (string, bool) {
	if !visualAsChanged(cmd) {
		return "", false
	}
	as, _ := cmd.Flags().GetString("as")
	switch v := strings.ToLower(strings.TrimSpace(as)); v {
	case "bot", "tenant", "app":
		return "bot", true
	case "", "auto":
		return "auto", true
	default:
		return v, true
	}
}

// withVisualDryRunIdentity 在显式传了 --as 时给 dry-run 预览补 identity 字段；未传时原样返回。
func withVisualDryRunIdentity(cmd *cobra.Command, plan map[string]any) map[string]any {
	if label, ok := visualDryRunIdentity(cmd); ok {
		plan["identity"] = label
	}
	return plan
}
