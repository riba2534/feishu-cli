package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// AppScope 是应用在开放平台已开通的一个权限。
type AppScope struct {
	Scope       string   `json:"scope"`
	TokenTypes  []string `json:"token_types"` // user / tenant
	Description string   `json:"description,omitempty"`
	Level       int      `json:"level,omitempty"`
}

// SupportsTokenType 报告该 scope 是否对指定身份（user / tenant）开通。
func (s AppScope) SupportsTokenType(tokenType string) bool {
	for _, t := range s.TokenTypes {
		if t == tokenType {
			return true
		}
	}
	return false
}

// ApplicationScopes 是 application/v6/applications/{app_id} 中与权限相关的部分。
type ApplicationScopes struct {
	AppID   string
	AppName string
	Scopes  []AppScope
}

// ScopesFor 返回对指定身份开通的 scope 名（排序、去重）。
func (a *ApplicationScopes) ScopesFor(tokenType string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range a.Scopes {
		if s.Scope == "" || seen[s.Scope] || !s.SupportsTokenType(tokenType) {
			continue
		}
		seen[s.Scope] = true
		out = append(out, s.Scope)
	}
	sort.Strings(out)
	return out
}

// GetApplicationScopes 以应用身份（Tenant Token）查询当前应用在开放平台已开通的权限。
//
// API: GET /open-apis/application/v6/applications/{app_id}?lang=zh_cn
// 用于区分"应用没开通"（99991672，需开发者后台开通并发布）与"用户没授权"（99991679，需 auth login --scope）。
func GetApplicationScopes(appID string) (*ApplicationScopes, error) {
	if appID == "" {
		return nil, fmt.Errorf("缺少 app_id")
	}
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	req := &larkcore.ApiReq{
		HttpMethod:                "GET",
		ApiPath:                   "/open-apis/application/v6/applications/" + url.PathEscape(appID),
		PathParams:                larkcore.PathParams{},
		QueryParams:               larkcore.QueryParams{},
		SupportedAccessTokenTypes: []larkcore.AccessTokenType{larkcore.AccessTokenTypeTenant},
	}
	req.QueryParams.Set("lang", "zh_cn")
	resp, err := cli.Do(Context(), req)
	if err != nil {
		return nil, fmt.Errorf("查询应用权限失败: %w", err)
	}
	if err := CheckAPIResponse("查询应用权限", resp); err != nil {
		return nil, err
	}
	var body struct {
		Data struct {
			App struct {
				AppID   string     `json:"app_id"`
				AppName string     `json:"app_name"`
				Scopes  []AppScope `json:"scopes"`
			} `json:"app"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &body); err != nil {
		return nil, fmt.Errorf("解析应用权限响应失败: %w", err)
	}
	app := body.Data.App
	if app.AppID == "" {
		app.AppID = appID
	}
	return &ApplicationScopes{AppID: app.AppID, AppName: app.AppName, Scopes: app.Scopes}, nil
}

// AppScopeConsoleURL 返回开放平台"权限管理"页链接（与服务端 99991672 文案中的链接同形）；
// 未配置 app_id 时返回空串。
func AppScopeConsoleURL(scopes []string) string {
	return appScopeConsoleURL(scopes)
}
