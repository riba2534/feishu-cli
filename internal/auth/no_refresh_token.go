package auth

import (
	"fmt"
	"net/url"

	"github.com/riba2534/feishu-cli/internal/config"
)

// NoRefreshTokenCause 说明登录拿不到 refresh_token 的真实原因（对齐官方 internal/auth/no_refresh_token.go）。
//
// 只申请 offline_access 不够：应用必须在开发者后台「安全设置」里开启「刷新 user_access_token」，
// 并发布包含该改动的应用版本，token 端点才会下发 refresh_token；在此之前授予的 scope 里仍会回显 offline_access。
const NoRefreshTokenCause = "应用未在开发者后台「安全设置」中开启「刷新 user_access_token」，或开启后尚未发布应用版本（只申请 offline_access 不会下发 refresh_token）"

// NoRefreshTokenSettingsURL 返回开启「刷新 user_access_token」的应用安全设置页。
// 品牌由 Open API base URL 推断（larksuite.com → open.larksuite.com，否则 open.feishu.cn）；appID 为空时返回空串。
func NoRefreshTokenSettingsURL(baseURL, appID string) string {
	if appID == "" {
		return ""
	}
	return fmt.Sprintf("%s/app/%s/safe", config.OfficialOpenBase(config.ParseBrand(baseURL)), url.PathEscape(appID))
}

// NoRefreshTokenFix 给出让后续登录可自动续期的处理步骤（含安全设置页链接，appID 已知时）。
func NoRefreshTokenFix(baseURL, appID string) string {
	fix := "在开发者后台「安全设置」中开启「刷新 user_access_token」，创建并发布新版本，然后重新执行 feishu-cli auth login"
	if settingsURL := NoRefreshTokenSettingsURL(baseURL, appID); settingsURL != "" {
		fix += "（" + settingsURL + "）"
	}
	return fix
}

// expiredWithoutRefreshTokenError 是 token.json 的 access_token 已过期、且登录时从未下发 refresh_token 时的错误。
// 与「refresh_token 也已过期」区分：后者重新登录即可，前者不修应用设置会每 2 小时重复失效。
func expiredWithoutRefreshTokenError(baseURL, appID string) error {
	return fmt.Errorf("User Access Token 已过期，且登录时未下发 refresh_token，无法自动续期。\n"+
		"原因：%s。\n"+
		"处理：%s。",
		NoRefreshTokenCause, NoRefreshTokenFix(baseURL, appID))
}
