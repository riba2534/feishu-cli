package cmd

import (
	"fmt"
	"time"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var authAutoCmd = &cobra.Command{
	Use:   "auto",
	Short: "自动检查并刷新 Token",
	Long: `自动检查当前 Token 状态，必要时通过 refresh_token 刷新。

两级降级策略:
  1. 检查 access_token 是否仍然有效（默认预留 5 分钟缓冲）
  2. 若过期，尝试用 refresh_token 刷新

如果两级均失败（无 refresh_token 或已过期），返回非零退出码，
提示调用方执行 auth login 重新授权。

适用于 AI Agent、定时任务等需要无人值守保持 Token 有效的场景。

示例:
  # 检查并自动刷新
  feishu-cli auth auto

  # JSON 输出（AI Agent 推荐）
  feishu-cli auth auto -o json

  # 自定义缓冲时间（默认 300 秒）
  feishu-cli auth auto --buffer 600`,
	RunE: func(cmd *cobra.Command, args []string) error {
		output, _ := cmd.Flags().GetString("output")
		buffer, _ := cmd.Flags().GetInt("buffer")

		token, err := auth.LoadToken()
		if err != nil {
			return wrapAutoResult(output, false, "", fmt.Errorf("读取 token 失败: %w", err))
		}
		if token == nil {
			return wrapAutoResult(output, false, "", fmt.Errorf("未登录，请先执行: feishu-cli auth login"))
		}

		// Level 1: access_token 有效性检查
		remaining := time.Until(token.ExpiresAt)
		if remaining > time.Duration(buffer)*time.Second {
			msg := fmt.Sprintf("Token 有效（剩余 %s）", formatDuration(remaining))
			return wrapAutoResult(output, true, msg, nil)
		}

		// Level 2: refresh_token 刷新
		if !token.IsRefreshTokenValid() {
			return wrapAutoResult(output, false, "",
				fmt.Errorf("Token 已过期且无有效的 refresh_token，请重新登录: feishu-cli auth login"))
		}

		if err := config.Validate(); err != nil {
			return wrapAutoResult(output, false, "", err)
		}
		cfg := config.Get()

		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = "https://open.feishu.cn"
		}

		newToken, err := auth.RefreshAccessToken(token.RefreshToken, cfg.AppID, cfg.AppSecret, baseURL)
		if err != nil {
			return wrapAutoResult(output, false, "",
				fmt.Errorf("刷新失败: %w\n请重新登录: feishu-cli auth login", err))
		}

		if err := auth.SaveToken(newToken); err != nil {
			return wrapAutoResult(output, false, "",
				fmt.Errorf("Token 已刷新但保存失败: %w", err))
		}

		msg := fmt.Sprintf("Token 已刷新，有效期至 %s", newToken.ExpiresAt.Format("2006-01-02 15:04:05"))
		return wrapAutoResult(output, true, msg, nil)
	},
}

// wrapAutoResult 统一处理 auth auto 的输出
func wrapAutoResult(output string, ok bool, msg string, err error) error {
	if output == "json" {
		result := map[string]any{"ok": ok}
		if msg != "" {
			result["message"] = msg
		}
		if err != nil {
			result["error"] = err.Error()
		}
		return printJSON(result)
	}

	if err != nil {
		return err
	}
	fmt.Println(msg)
	return nil
}

func init() {
	authCmd.AddCommand(authAutoCmd)
	authAutoCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	authAutoCmd.Flags().Int("buffer", 300, "Token 有效期缓冲时间（秒），剩余时间低于此值时触发刷新")
}
