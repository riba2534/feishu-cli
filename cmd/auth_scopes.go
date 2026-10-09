package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// getApplicationScopesFn 查询应用已开通权限，测试可替换。
var getApplicationScopesFn = client.GetApplicationScopes

var authScopesCmd = &cobra.Command{
	Use:   "scopes",
	Short: "查询应用在开放平台已开通的权限（区分 user / tenant）",
	Long: `以应用身份（Tenant Token）调用 application/v6/applications/{app_id}，列出当前应用
在开放平台已开通的 scope，并区分 User 身份与 Tenant（Bot）身份。

用途：区分"应用没开通"和"用户没授权"两类缺权限错误——
  - 99991672（应用未开通）：scope 不在本命令列表中 → 到开发者后台为应用开通并发布新版本，
    重新 auth login 修不好
  - 99991679（用户未授权）：scope 已在 user_scopes 中，但本地 token 未授予 →
    执行 feishu-cli auth login --scope "<scope>"

加 --scope 时逐项诊断（diagnosis 字段）：
  ok                  应用已开通 user 身份且本地 User Token 已授权
  app_not_enabled     应用未开通该 scope（User 与 Tenant 身份都没有）
  tenant_only         应用只对 Tenant（Bot）身份开通：--as bot 可用，User 身份需在开放平台补开
  user_not_granted    应用已开通，但本地 User Token 未授权
  not_logged_in       应用已开通，但本地没有可用的 User Token

本命令只做诊断，总是以退出码 0 结束（接口调用失败除外）；业务前预检请用 auth check。
需要应用身份可调用 application/v6 应用信息接口（通常默认可用）。

示例:
  feishu-cli auth scopes
  feishu-cli auth scopes -o json
  feishu-cli auth scopes --scope "okr:okr.period:readonly,search:docs:read" -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		cfg := config.Get()
		output, _ := cmd.Flags().GetString("output")
		scopeFlag, _ := cmd.Flags().GetString("scope")

		app, err := getApplicationScopesFn(cfg.AppID)
		if err != nil {
			return err
		}
		userScopes := app.ScopesFor("user")
		tenantScopes := app.ScopesFor("tenant")

		result := map[string]any{
			"app_id":             app.AppID,
			"app_name":           app.AppName,
			"brand":              string(config.ParseBrand(cfg.BaseURL)),
			"scope_count":        len(app.Scopes),
			"user_scope_count":   len(userScopes),
			"tenant_scope_count": len(tenantScopes),
			"user_scopes":        userScopes,
			"tenant_scopes":      tenantScopes,
		}

		var checks []map[string]any
		var appMissing, userMissing []string
		if requested := auth.UniqueScopeList(scopeFlag); len(requested) > 0 {
			checks, appMissing, userMissing = diagnoseAppScopes(requested, userScopes, tenantScopes)
			result["checks"] = checks
			if len(appMissing) > 0 {
				// 应用未对 User 身份开通的 scope（含 tenant_only），需在开放平台开通并发布
				result["app_not_enabled"] = appMissing
				if link := client.AppScopeConsoleURL(appMissing); link != "" {
					result["console_url"] = link
				}
			}
			if len(userMissing) > 0 {
				result["user_not_granted"] = userMissing
				result["suggestion"] = fmt.Sprintf("feishu-cli auth login --scope %q", strings.Join(userMissing, " "))
			}
		}

		if output == "json" {
			return printJSON(result)
		}

		fmt.Printf("应用: %s（%s）\n", app.AppID, app.AppName)
		fmt.Printf("已开通 scope: %d 个（User 身份 %d，Tenant 身份 %d）\n", len(app.Scopes), len(userScopes), len(tenantScopes))
		if checks != nil {
			fmt.Println()
			for _, c := range checks {
				fmt.Printf("  %-45s %-17s user=%v tenant=%v\n", c["scope"], c["diagnosis"], c["user_enabled"], c["tenant_enabled"])
			}
			if link, _ := result["console_url"].(string); link != "" {
				fmt.Printf("\n应用未开通的 scope 请到开放平台开通并发布: %s\n", link)
			}
			if s, _ := result["suggestion"].(string); s != "" {
				fmt.Printf("用户未授权的 scope 请执行: %s\n", s)
			}
			return nil
		}
		fmt.Println("\nUser 身份:")
		for _, s := range userScopes {
			fmt.Printf("  %s\n", s)
		}
		fmt.Println("\nTenant 身份:")
		for _, s := range tenantScopes {
			fmt.Printf("  %s\n", s)
		}
		return nil
	},
}

// diagnoseAppScopes 逐项对比"应用已开通"与"本地 User Token 已授权"，返回诊断明细、
// 应用未开通（User 身份）的 scope 与用户未授权的 scope。
func diagnoseAppScopes(requested, userScopes, tenantScopes []string) (checks []map[string]any, appMissing, userMissing []string) {
	userSet := toSet(userScopes)
	tenantSet := toSet(tenantScopes)

	granted := map[string]bool{}
	loggedIn := false
	if token, err := auth.LoadToken(); err == nil && token != nil && (token.IsAccessTokenValid() || token.IsRefreshTokenValid()) {
		loggedIn = true
		granted = toSet(auth.UniqueScopeList(token.Scope))
	}

	for _, s := range requested {
		c := map[string]any{
			"scope":          s,
			"user_enabled":   userSet[s],
			"tenant_enabled": tenantSet[s],
		}
		if loggedIn {
			c["user_granted"] = granted[s]
		}
		switch {
		case !userSet[s] && tenantSet[s]:
			// 只对 Bot 身份开通：--as bot 可用；User 身份需在开放平台补开 user 权限
			c["diagnosis"] = "tenant_only"
			appMissing = append(appMissing, s)
		case !userSet[s]:
			c["diagnosis"] = "app_not_enabled"
			appMissing = append(appMissing, s)
		case !loggedIn:
			c["diagnosis"] = "not_logged_in"
			userMissing = append(userMissing, s)
		case !granted[s]:
			c["diagnosis"] = "user_not_granted"
			userMissing = append(userMissing, s)
		default:
			c["diagnosis"] = "ok"
		}
		checks = append(checks, c)
	}
	return checks, appMissing, userMissing
}

func toSet(items []string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, s := range items {
		m[s] = true
	}
	return m
}

func init() {
	authCmd.AddCommand(authScopesCmd)
	authScopesCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	authScopesCmd.Flags().String("scope", "", "逐项诊断的 scope（空格或逗号分隔）：区分应用未开通 / 用户未授权")
}
