package cmd

import (
	"strings"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

// validateIdentityAsFlag 在 config.Validate 与身份解析之前静态校验 --as 取值，
// 错误文本与 resolveIdentityToken 的同一分支一致（只是更早报出，退出码 2）。
// 命令没有 --as flag 时直接放行。
func validateIdentityAsFlag(cmd *cobra.Command) error {
	if cmd.Flags().Lookup("as") == nil {
		return nil
	}
	as, _ := cmd.Flags().GetString("as")
	switch strings.ToLower(strings.TrimSpace(as)) {
	case "", "auto", "bot", "tenant", "app", "user":
		return nil
	}
	return clierr.Usagef("--as 仅支持 bot|user|auto，得到 %q", as)
}
