package cmd

import (
	"fmt"
	"time"

	"github.com/riba2534/feishu-cli/v2/internal/auth"
	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/riba2534/feishu-cli/v2/internal/registry"
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
				out := map[string]any{"logged_in": false, "error": err.Error(), "catalog": registry.Status()}
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
					"catalog":   registry.Status(),
				}
				attachProfileContext(out)
				return printJSON(out)
			}
			fmt.Println("授权状态: 未登录")
			printProfileContextHuman()
			printCatalogHuman()
			fmt.Println("  使用 feishu-cli auth login 进行授权")
			return nil
		}

		// --verify 先在线校验：access 过期时与业务命令一样走加锁刷新（含 App 绑定校验），
		// 后续展示的就是刷新后的 token 状态，而不是刷新前的旧快照。
		var verified bool
		var verifyErr string
		if verify {
			cfg := config.Get()
			var fresh *auth.TokenStore
			fresh, verified, verifyErr = verifyStoredUserToken(token, cfg.AppID, cfg.AppSecret, cfg.BaseURL)
			if fresh != nil {
				token = fresh
			} else if reloaded, err := auth.LoadToken(); err == nil && reloaded != nil {
				// 校验失败时 token.json 可能刚被写入终态刷新失败标记，重读以展示最新状态
				token = reloaded
			}
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
		if token.RefreshFailure != nil {
			// 终态刷新失败标记：refresh_token 已被服务端判定失效，access 过期后无法续期
			health = "needs_relogin"
			note = token.RefreshFailure.Err().Error()
		}

		result := map[string]any{
			"logged_in":             true,
			"identity":              identity,
			"token_status":          status,
			"access_token":          auth.MaskToken(token.AccessToken),
			"scope":                 token.Scope,
			"expires_at":            token.ExpiresAt.Format(time.RFC3339),
			"access_token_valid":    token.IsAccessTokenValid(),
			"refresh_token_present": refreshPresent,
			"health":                health,
			"catalog":               registry.Status(),
		}
		if note != "" {
			result["note"] = note
		}
		if f := token.RefreshFailure; f != nil {
			failure := map[string]any{"at": f.At.Format(time.RFC3339)}
			if f.Code != 0 {
				failure["code"] = f.Code
			}
			if f.Error != "" {
				failure["error"] = f.Error
			}
			if f.Description != "" {
				failure["description"] = f.Description
			}
			result["refresh_failure"] = failure
		}
		if refreshPresent {
			result["refresh_token_valid"] = token.IsRefreshTokenValid()
			if !token.RefreshExpiresAt.IsZero() {
				result["refresh_expires_at"] = token.RefreshExpiresAt.Format(time.RFC3339)
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
				"cached_at": cache.CachedAt.Format(time.RFC3339),
			}
		}
		if verify {
			result["verified"] = verified
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
			if f := token.RefreshFailure; f != nil {
				fmt.Printf("  Refresh Token:  已失效（code=%d，%s 刷新被拒绝）\n", f.Code, f.At.Format("2006-01-02 15:04:05"))
			} else if token.IsRefreshTokenValid() {
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
		printCatalogHuman()
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

func printCatalogHuman() {
	info := registry.Status()
	fmt.Printf("  Catalog:        source=%s version=%s services=%d methods=%d\n",
		info.Source, info.RuntimeVersion, info.ServiceCount, info.MethodCount)
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

// verifyUserInfoFn 在线校验 User Token（调 authen/v1/user_info），测试可替换。
var verifyUserInfoFn = func(userAccessToken string) error {
	_, err := client.GetCurrentUserInfo(userAccessToken)
	return err
}

// verifyStoredUserToken 校验 token.json 中的 User Token 能否被服务端接受。
//
// 与业务命令共用 auth.EnsureFreshLocalToken：先校验 token 已绑定当前 App（未绑定的旧 token
// 不会被静默绑定），access 过期时走跨进程加锁 + 代际校验的刷新路径，避免与并发进程
// 重复消耗同一 refresh_token（20073）。返回刷新后的 token（未刷新时为入参）供展示。
func verifyStoredUserToken(token *auth.TokenStore, appID, appSecret, baseURL string) (*auth.TokenStore, bool, string) {
	if token == nil {
		return nil, false, "未登录"
	}
	fresh, _, err := auth.EnsureFreshLocalToken(appID, appSecret, baseURL, token)
	if err != nil {
		return nil, false, err.Error()
	}
	if err := verifyUserInfoFn(fresh.AccessToken); err != nil {
		return fresh, false, err.Error()
	}
	return fresh, true, ""
}
