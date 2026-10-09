package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var authRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "强制刷新 access_token（用 refresh_token）",
	Long: `用 ~/.feishu-cli/token.json 中的 refresh_token 调用飞书 OAuth v2 token 端点，
强制获取新的 access_token，并写回 token.json。

通常情况下不需要手动调用——当 access_token 过期时，所有读取 token.json 的命令
（如 wiki/doc/search 等）会自动触发刷新。本子命令用于:
  - 长时间运行的脚本前主动续期，避免中途过期
  - 排查 token 状态时手动验证 refresh_token 是否仍有效
  - 强制让 token.json 立即更新到最新状态

示例:
  feishu-cli auth refresh                    # 文本输出
  feishu-cli auth refresh -o json            # JSON 输出（自动化场景）

退出码:
  0 - 成功
  1 - 失败（refresh_token 过期、网络错误等）`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		cfg := config.Get()
		newToken, err := auth.ForceRefreshLocalToken(cfg.AppID, cfg.AppSecret, cfg.BaseURL)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			result := map[string]any{
				"status":       "ok",
				"access_token": auth.MaskToken(newToken.AccessToken),
				"expires_at":   newToken.ExpiresAt.Format("2006-01-02 15:04:05"),
				"scope":        newToken.Scope,
			}
			if !newToken.RefreshExpiresAt.IsZero() {
				result["refresh_expires_at"] = newToken.RefreshExpiresAt.Format("2006-01-02 15:04:05")
			}
			return printJSON(result)
		}

		fmt.Println("✓ Access Token 已刷新")
		fmt.Printf("  Access Token:   %s\n", auth.MaskToken(newToken.AccessToken))
		fmt.Printf("  有效期至:        %s\n", newToken.ExpiresAt.Format("2006-01-02 15:04:05"))
		if !newToken.RefreshExpiresAt.IsZero() {
			fmt.Printf("  Refresh 有效期:  %s\n", newToken.RefreshExpiresAt.Format("2006-01-02 15:04:05"))
		} else {
			fmt.Println("  Refresh 有效期:  未知")
		}
		return nil
	},
}

func init() {
	authRefreshCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	authCmd.AddCommand(authRefreshCmd)
}
