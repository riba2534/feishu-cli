package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/auth"
)

type loginScopeSummary struct {
	Requested []string
	Granted   []string
	Missing   []string
}

func buildLoginScopeSummary(requestedScope, grantedScope string) *loginScopeSummary {
	requested := auth.UniqueScopeList(requestedScope)
	if len(requested) == 0 {
		return &loginScopeSummary{
			Requested: []string{},
			Granted:   auth.UniqueScopeList(grantedScope),
			Missing:   []string{},
		}
	}
	granted, missing := auth.PartitionScopes(grantedScope, requested)
	return &loginScopeSummary{
		Requested: requested,
		Granted:   granted,
		Missing:   missing,
	}
}

// buildAuthorizationCompleteEvent 构造 auth login --json 的 authorization_complete 事件。
// appID/baseURL 用于在未下发 refresh_token 时拼应用安全设置页链接。
func buildAuthorizationCompleteEvent(token *auth.TokenStore, summary *loginScopeSummary, appID, baseURL string) map[string]any {
	refreshPresent := token.RefreshToken != ""
	event := map[string]any{
		"event":                 "authorization_complete",
		"expires_at":            token.ExpiresAt.Format(time.RFC3339),
		"scope":                 token.Scope,
		"requested_scopes":      emptyIfNil(summary.Requested),
		"granted_scopes":        emptyIfNil(summary.Granted),
		"missing_scopes":        emptyIfNil(summary.Missing),
		"refresh_token_present": refreshPresent,
	}
	if !token.RefreshExpiresAt.IsZero() {
		event["refresh_expires_at"] = token.RefreshExpiresAt.Format(time.RFC3339)
	}
	if len(summary.Requested) > 0 {
		event["requested_scope"] = strings.Join(summary.Requested, " ")
	}
	if token.StatusMessage != "" {
		event["status_message"] = token.StatusMessage
	}
	warnings := []string{}
	hints := []string{}
	if !refreshPresent {
		appID = auth.NoRefreshTokenAppID(token, appID)
		warnings = append(warnings, "未获取到 refresh_token，Access Token 过期后需要重新登录")
		hints = append(hints, auth.NoRefreshTokenFix(baseURL, appID))
		// 结构化归因（additive）：只申请 offline_access 不够，需开启应用安全设置里的「刷新 user_access_token」并发版
		event["no_refresh_token"] = noRefreshTokenInfo(baseURL, appID)
	}
	if len(summary.Missing) > 0 {
		warnings = append(warnings, fmt.Sprintf("以下 scope 未授予: %s", strings.Join(summary.Missing, " ")))
		hints = append(hints, fmt.Sprintf("确认飞书开放平台已开通后，执行 feishu-cli auth login --scope %q 重新授权", strings.Join(summary.Missing, " ")))
	}
	if len(warnings) > 0 {
		event["warnings"] = warnings
		// 兼容：历史字段 warning/hint 仍保留首条，避免旧消费方解析失败
		event["warning"] = warnings[0]
		if len(hints) > 0 {
			event["hint"] = hints[0]
			event["hints"] = hints
		}
	}
	return event
}

func printTokenSuccess(token *auth.TokenStore, summary *loginScopeSummary, appID, baseURL string) {
	path, _ := auth.TokenPath()
	fmt.Fprintln(os.Stderr, "\n✓ 授权成功！")
	fmt.Fprintf(os.Stderr, "  Token 已保存到 %s\n", path)
	fmt.Fprintf(os.Stderr, "  Access Token 有效期至: %s\n", token.ExpiresAt.Format("2006-01-02 15:04:05"))
	if token.RefreshToken != "" && !token.RefreshExpiresAt.IsZero() {
		fmt.Fprintf(os.Stderr, "  Refresh Token 有效期至: %s\n", token.RefreshExpiresAt.Format("2006-01-02 15:04:05"))
	}
	if token.Scope != "" {
		fmt.Fprintf(os.Stderr, "  实际授予: %s\n", token.Scope)
	}
	if summary != nil && len(summary.Requested) > 0 {
		fmt.Fprintf(os.Stderr, "  请求范围: %s\n", strings.Join(summary.Requested, " "))
	}
	if token.StatusMessage != "" {
		fmt.Fprintf(os.Stderr, "  服务端提示: %s\n", token.StatusMessage)
	}
	if summary != nil && len(summary.Missing) > 0 {
		fmt.Fprintf(os.Stderr, "  未授予:   %s\n", strings.Join(summary.Missing, " "))
		fmt.Fprintf(os.Stderr, "  提示:     确认飞书开放平台已开通后，执行 feishu-cli auth login --scope %q 重新授权\n", strings.Join(summary.Missing, " "))
	}
	if token.RefreshToken == "" {
		appID = auth.NoRefreshTokenAppID(token, appID)
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  ⚠ 警告: 未获取到 refresh_token")
		fmt.Fprintln(os.Stderr, "    Access Token 过期（约 2 小时）后需要重新 feishu-cli auth login")
		fmt.Fprintf(os.Stderr, "    原因: %s\n", auth.NoRefreshTokenCause)
		fmt.Fprintln(os.Stderr, "    处理步骤:")
		fmt.Fprintln(os.Stderr, "      1. 在开发者后台「安全设置」中开启「刷新 user_access_token」")
		if u := auth.NoRefreshTokenSettingsURL(baseURL, appID); u != "" {
			fmt.Fprintf(os.Stderr, "         安全设置页面: %s\n", u)
		}
		fmt.Fprintln(os.Stderr, "      2. 创建并发布新的应用版本（开关改动发版后才生效）")
		fmt.Fprintln(os.Stderr, "      3. 执行 feishu-cli auth login 重新授权")
		fmt.Fprintln(os.Stderr, "      4. 仍失败请反馈: https://github.com/riba2534/feishu-cli/issues/94")
	}
}

// noRefreshTokenInfo 构造 no_refresh_token{cause,fix,settings_url}；app_id 未知时省略 settings_url。
func noRefreshTokenInfo(baseURL, appID string) map[string]any {
	info := map[string]any{
		"cause": auth.NoRefreshTokenCause,
		"fix":   auth.NoRefreshTokenFix(baseURL, appID),
	}
	if u := auth.NoRefreshTokenSettingsURL(baseURL, appID); u != "" {
		info["settings_url"] = u
	}
	return info
}

func emptyIfNil(items []string) []string {
	if items == nil {
		return []string{}
	}
	return items
}
