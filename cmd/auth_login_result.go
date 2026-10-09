package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/v2/internal/auth"
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

func buildAuthorizationCompleteEvent(token *auth.TokenStore, summary *loginScopeSummary) map[string]any {
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
		warnings = append(warnings, "未获取到 refresh_token，Access Token 过期后需要重新登录")
		hints = append(hints, "请在飞书开放平台应用权限管理页面开通 offline_access 后重新 feishu-cli auth login")
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

func printTokenSuccess(token *auth.TokenStore, summary *loginScopeSummary) {
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
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  ⚠ 警告: 未获取到 refresh_token")
		fmt.Fprintln(os.Stderr, "    Access Token 过期（约 2 小时）后需要重新 feishu-cli auth login")
		fmt.Fprintln(os.Stderr, "    常见原因: 应用在飞书开放平台未开通 offline_access scope")
		fmt.Fprintln(os.Stderr, "    排查步骤:")
		fmt.Fprintln(os.Stderr, "      1. 登录飞书开放平台应用权限管理页面，确认已开通 offline_access")
		fmt.Fprintln(os.Stderr, "      2. 执行 feishu-cli auth logout && feishu-cli auth login 重新授权")
		fmt.Fprintln(os.Stderr, "      3. 仍失败请反馈: https://github.com/riba2534/feishu-cli/issues/94")
	}
}

func emptyIfNil(items []string) []string {
	if items == nil {
		return []string{}
	}
	return items
}
