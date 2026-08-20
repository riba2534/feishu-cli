package cmd

import (
	"fmt"
	"time"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "查看当前授权状态",
	Long: `查看本地存储的 OAuth token 状态。

显示内容:
  - Access Token（脱敏）和有效期
  - Refresh Token 状态和有效期
  - 授权范围
  - 当前 profile / 生效 app_id

示例:
  feishu-cli auth status

  # JSON 格式输出（AI Agent 推荐）
  feishu-cli auth status -o json

  # 在线校验当前 token 是否仍可被服务端接受
  feishu-cli auth status --verify -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		output, _ := cmd.Flags().GetString("output")
		verify, _ := cmd.Flags().GetBool("verify")

		token, err := auth.LoadToken()
		if err != nil {
			if output == "json" {
				out := map[string]any{"logged_in": false, "error": err.Error()}
				attachProfileContext(out)
				return printJSON(out)
			}
			return fmt.Errorf("读取 token 失败: %w", err)
		}

		if token == nil {
			if output == "json" {
				out := map[string]any{
					"logged_in": false,
					"identity":  "bot",
					"note":      "未登录用户身份，仅可使用应用身份（App Token）能力",
				}
				attachProfileContext(out)
				return printJSON(out)
			}
			fmt.Println("授权状态: 未登录")
			printProfileContextHuman()
			fmt.Println("  使用 feishu-cli auth login 进行授权")
			return nil
		}

		status := token.TokenStatus()
		identity := "user"
		note := ""
		if status == "expired" {
			identity = "bot"
			note = "User Token 已过期，仅剩应用身份可用"
		}

		refreshPresent := token.RefreshToken != ""
		// health 区分三种情况：
		//   healthy              — access_token 或 refresh_token 当前有效
		//   missing_refresh_token — 登录时就没拿到 refresh_token（常因应用未开通 offline_access）
		//   needs_relogin        — 曾经有 refresh_token 但已过期，或 access_token 也已失效
		health := "healthy"
		if !refreshPresent {
			health = "missing_refresh_token"
			if note == "" {
				note = "登录时未获取到 refresh_token，Access Token 过期后需重新 auth login；常因应用未开通 offline_access scope"
			}
		} else if status == "expired" {
			health = "needs_relogin"
		}

		result := map[string]any{
			"logged_in":             true,
			"identity":              identity,
			"token_status":          status,
			"access_token":          auth.MaskToken(token.AccessToken),
			"scope":                 token.Scope,
			"expires_at":            token.ExpiresAt.Format("2006-01-02T15:04:05+08:00"),
			"access_token_valid":    token.IsAccessTokenValid(),
			"refresh_token_present": refreshPresent,
			"health":                health,
		}
		if note != "" {
			result["note"] = note
		}
		if refreshPresent {
			result["refresh_token_valid"] = token.IsRefreshTokenValid()
			if !token.RefreshExpiresAt.IsZero() {
				result["refresh_expires_at"] = token.RefreshExpiresAt.Format("2006-01-02T15:04:05+08:00")
			}
		} else {
			result["refresh_token_valid"] = false
		}
		if cache, cacheErr := auth.LoadCurrentUserCache(); cacheErr == nil && cache != nil {
			result["cached_user"] = map[string]any{
				"open_id":   cache.OpenID,
				"user_id":   cache.UserID,
				"union_id":  cache.UnionID,
				"name":      cache.Name,
				"cached_at": cache.CachedAt.Format("2006-01-02T15:04:05+08:00"),
			}
		}
		if verify {
			ok, verifyErr := verifyStoredUserToken(token)
			result["verified"] = ok
			if verifyErr != "" {
				result["verify_error"] = verifyErr
			}
		}
		// JSON 输出模式（人类可读走 printProfileContextHuman，避免重复构建 inventory）
		if output == "json" {
			attachProfileContext(result)
			return printJSON(result)
		}

		// 人类可读输出
		fmt.Printf("授权状态: 已登录（%s）\n", status)
		printProfileContextHuman()
		fmt.Printf("  Access Token:   %s\n", auth.MaskToken(token.AccessToken))

		if token.IsAccessTokenValid() {
			remaining := time.Until(token.ExpiresAt)
			fmt.Printf("  有效期至:       %s（剩余 %s）\n",
				token.ExpiresAt.Format("2006-01-02 15:04:05"),
				formatDuration(remaining))
		} else {
			fmt.Printf("  有效期至:       %s（已过期）\n", token.ExpiresAt.Format("2006-01-02 15:04:05"))
		}

		if refreshPresent {
			if token.IsRefreshTokenValid() {
				if token.RefreshExpiresAt.IsZero() {
					fmt.Println("  Refresh Token:  有效（过期时间未知）")
				} else {
					remaining := time.Until(token.RefreshExpiresAt)
					fmt.Printf("  Refresh Token:  有效（剩余 %s）\n", formatDuration(remaining))
				}
			} else {
				fmt.Println("  Refresh Token:  已过期")
			}
		} else {
			fmt.Println("  Refresh Token:  ⚠ 未获取（登录时应用可能未开通 offline_access）")
		}

		if token.Scope != "" {
			fmt.Printf("  授权范围:       %s\n", token.Scope)
		}
		if cache, ok := result["cached_user"].(map[string]any); ok {
			fmt.Printf("  当前用户:       %s (%s)\n", cache["name"], cache["open_id"])
		}
		fmt.Printf("  健康度:         %s\n", health)
		if note != "" {
			fmt.Printf("  提示:           %s\n", note)
		}
		if verify {
			if verified, _ := result["verified"].(bool); verified {
				fmt.Println("  在线校验:       通过")
			} else if verifyErr, _ := result["verify_error"].(string); verifyErr != "" {
				fmt.Printf("  在线校验:       失败（%s）\n", verifyErr)
			}
		}

		return nil
	},
}

// formatDuration 格式化时间间隔为友好显示
func formatDuration(d time.Duration) string {
	if d < 0 {
		return "已过期"
	}

	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%d 天 %d 小时", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%d 小时 %d 分", hours, minutes)
	}
	return fmt.Sprintf("%d 分钟", minutes)
}

func init() {
	authCmd.AddCommand(authStatusCmd)
	authStatusCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	authStatusCmd.Flags().Bool("verify", false, "在线校验当前 token 是否仍可被服务端接受")
}

func verifyStoredUserToken(token *auth.TokenStore) (bool, string) {
	if token == nil {
		return false, "未登录"
	}

	activeToken := token.AccessToken
	if !token.IsAccessTokenValid() {
		if !token.IsRefreshTokenValid() {
			return false, "access_token 和 refresh_token 都已过期"
		}
		cfg := config.Get()
		fresh, err := auth.RefreshAccessToken(token, cfg.AppID, cfg.AppSecret, cfg.BaseURL)
		if err != nil {
			return false, err.Error()
		}
		if err := auth.SaveToken(fresh); err != nil {
			return false, err.Error()
		}
		activeToken = fresh.AccessToken
	}

	if _, err := client.GetCurrentUserInfo(activeToken); err != nil {
		return false, err.Error()
	}
	return true, ""
}
