package client

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// 通讯录搜索（对齐官方 contact +search-user / +search-bot），均只支持 User 身份：
//   - 用户：POST /open-apis/contact/v3/users/search（覆盖跨租户联系人，返回 p2p chat_id 等）
//   - 机器人：POST /open-apis/bot/v4/bot/search

const (
	MaxContactSearchPageSize = 30
	MaxContactSearchQuery    = 50
)

// SearchUsersV3Options 用户搜索参数。
type SearchUsersV3Options struct {
	Query                string
	UserIDs              []string // 限定 open_id 范围（≤100）
	HasChatted           bool     // 只看聊过天的人（has_contact）
	ExcludeExternalUsers bool     // 排除外部联系人（exclude_outer_contact）
	LeftOrganization     bool     // 只看已离职（is_resigned）
	HasEnterpriseEmail   bool
	PageSize             int
	PageToken            string
	Lang                 string // 姓名语言偏好，如 zh_cn / en_us
}

// ContactSearchUser 用户搜索结果项。name 为按语言偏好挑选的姓名。
type ContactSearchUser struct {
	OpenID          string   `json:"open_id"`
	Name            string   `json:"name"`
	Email           string   `json:"email,omitempty"`
	EnterpriseEmail string   `json:"enterprise_email,omitempty"`
	Department      string   `json:"department,omitempty"`
	P2PChatID       string   `json:"p2p_chat_id,omitempty"`
	HasChatted      bool     `json:"has_chatted"`
	IsCrossTenant   bool     `json:"is_cross_tenant"`
	IsActivated     bool     `json:"is_activated"`
	ChatRecencyHint string   `json:"chat_recency_hint,omitempty"`
	MatchSegments   []string `json:"match_segments,omitempty"`
}

// ContactSearchUsersResult 用户搜索结果。
type ContactSearchUsersResult struct {
	Users     []*ContactSearchUser `json:"users"`
	HasMore   bool                 `json:"has_more"`
	PageToken string               `json:"page_token,omitempty"`
	Notice    string               `json:"notice,omitempty"`
}

var (
	displayInfoHighlightRE = regexp.MustCompile(`<h>(.*?)</h>`)
	displayInfoRecencyRE   = regexp.MustCompile(`^\[(.+)\]$`)
	displayInfoTagRE       = regexp.MustCompile(`</?h>`)
)

// SearchUsersV3 按关键词 / open_id / 过滤条件搜索用户。
func SearchUsersV3(opts SearchUsersV3Options, userAccessToken string) (*ContactSearchUsersResult, error) {
	if userAccessToken == "" {
		return nil, fmt.Errorf("搜索用户需要 User Access Token（请先 feishu-cli auth login）")
	}
	body := map[string]any{}
	if q := strings.TrimSpace(opts.Query); q != "" {
		body["query"] = q
	}
	filter := map[string]any{}
	if len(opts.UserIDs) > 0 {
		filter["user_ids"] = opts.UserIDs
	}
	if opts.HasChatted {
		filter["has_contact"] = true
	}
	if opts.ExcludeExternalUsers {
		filter["exclude_outer_contact"] = true
	}
	if opts.LeftOrganization {
		filter["is_resigned"] = true
	}
	if opts.HasEnterpriseEmail {
		filter["has_enterprise_email"] = true
	}
	if len(filter) > 0 {
		body["filter"] = filter
	}

	var env struct {
		Data struct {
			Items []struct {
				ID          string `json:"id"`
				DisplayInfo string `json:"display_info"`
				MetaData    struct {
					I18nNames             map[string]string `json:"i18n_names"`
					MailAddress           string            `json:"mail_address"`
					EnterpriseMailAddress string            `json:"enterprise_mail_address"`
					IsRegistered          bool              `json:"is_registered"`
					ChatID                string            `json:"chat_id"`
					IsCrossTenant         bool              `json:"is_cross_tenant"`
				} `json:"meta_data"`
			} `json:"items"`
			HasMore   bool   `json:"has_more"`
			PageToken string `json:"page_token"`
			Notice    string `json:"notice"`
		} `json:"data"`
	}
	if err := postContactSearch("搜索用户", "/open-apis/contact/v3/users/search", opts.PageSize, opts.PageToken, body, userAccessToken, &env); err != nil {
		return nil, err
	}

	out := &ContactSearchUsersResult{
		Users:     []*ContactSearchUser{},
		HasMore:   env.Data.HasMore,
		PageToken: env.Data.PageToken,
		Notice:    env.Data.Notice,
	}
	for _, it := range env.Data.Items {
		segments, department, recency := parseContactDisplayInfo(it.DisplayInfo)
		out.Users = append(out.Users, &ContactSearchUser{
			OpenID:          it.ID,
			Name:            pickLocalizedName(it.MetaData.I18nNames, opts.Lang, it.ID),
			Email:           it.MetaData.MailAddress,
			EnterpriseEmail: it.MetaData.EnterpriseMailAddress,
			Department:      department,
			P2PChatID:       it.MetaData.ChatID,
			HasChatted:      it.MetaData.ChatID != "",
			IsCrossTenant:   it.MetaData.IsCrossTenant,
			IsActivated:     it.MetaData.IsRegistered,
			ChatRecencyHint: recency,
			MatchSegments:   segments,
		})
	}
	return out, nil
}

// SearchBotsOptions 机器人搜索参数。
type SearchBotsOptions struct {
	Query      string
	ChatIDs    []string // 只在这些群里找（≤100）
	HasChatted bool     // 只看聊过天的机器人
	PageSize   int
	PageToken  string
}

// ContactSearchBot 机器人搜索结果项。
type ContactSearchBot struct {
	OpenID          string   `json:"open_id"`
	Name            string   `json:"name"`
	Description     string   `json:"description,omitempty"`
	ChatID          string   `json:"chat_id,omitempty"` // 与该机器人的单聊会话（聊过天才有）
	EnableJoinGroup bool     `json:"enable_join_group"`
	IsAgent         bool     `json:"is_agent"`
	TenantID        string   `json:"tenant_id,omitempty"`
	MatchSegments   []string `json:"match_segments,omitempty"`
}

// ContactSearchBotsResult 机器人搜索结果。
type ContactSearchBotsResult struct {
	Bots      []*ContactSearchBot `json:"bots"`
	HasMore   bool                `json:"has_more"`
	PageToken string              `json:"page_token,omitempty"`
	Notice    string              `json:"notice,omitempty"`
}

// SearchBots 按关键词搜索机器人（应用）。
func SearchBots(opts SearchBotsOptions, userAccessToken string) (*ContactSearchBotsResult, error) {
	if userAccessToken == "" {
		return nil, fmt.Errorf("搜索机器人需要 User Access Token（请先 feishu-cli auth login）")
	}
	body := map[string]any{"query": strings.TrimSpace(opts.Query)}
	filter := map[string]any{}
	if len(opts.ChatIDs) > 0 {
		filter["chat_ids"] = opts.ChatIDs
	}
	if opts.HasChatted {
		filter["has_chatter"] = true
	}
	if len(filter) > 0 {
		body["filter"] = filter
	}
	var env struct {
		Data struct {
			Items []struct {
				ID          string `json:"id"`
				DisplayInfo string `json:"display_info"`
				MetaData    struct {
					TenantID        string `json:"tenant_id"`
					EnableJoinGroup bool   `json:"enable_join_group"`
					ChatID          string `json:"chat_id"`
					IsAgent         bool   `json:"is_agent"`
				} `json:"meta_data"`
			} `json:"items"`
			HasMore   bool   `json:"has_more"`
			PageToken string `json:"page_token"`
			Notice    string `json:"notice"`
		} `json:"data"`
	}
	if err := postContactSearch("搜索机器人", "/open-apis/bot/v4/bot/search", opts.PageSize, opts.PageToken, body, userAccessToken, &env); err != nil {
		return nil, err
	}
	out := &ContactSearchBotsResult{
		Bots:      []*ContactSearchBot{},
		HasMore:   env.Data.HasMore,
		PageToken: env.Data.PageToken,
		Notice:    env.Data.Notice,
	}
	for _, it := range env.Data.Items {
		lines := strings.Split(it.DisplayInfo, "\n")
		var segments []string
		for _, m := range displayInfoHighlightRE.FindAllStringSubmatch(it.DisplayInfo, -1) {
			segments = append(segments, html.UnescapeString(m[1]))
		}
		bot := &ContactSearchBot{
			OpenID:          it.ID,
			Name:            html.UnescapeString(displayInfoTagRE.ReplaceAllString(strings.TrimSpace(lines[0]), "")),
			ChatID:          it.MetaData.ChatID,
			EnableJoinGroup: it.MetaData.EnableJoinGroup,
			IsAgent:         it.MetaData.IsAgent,
			TenantID:        it.MetaData.TenantID,
			MatchSegments:   segments,
		}
		if len(lines) > 1 {
			bot.Description = html.UnescapeString(displayInfoTagRE.ReplaceAllString(strings.TrimSpace(strings.Join(lines[1:], "\n")), ""))
		}
		out.Bots = append(out.Bots, bot)
	}
	return out, nil
}

func postContactSearch(action, path string, pageSize int, pageToken string, body map[string]any, userAccessToken string, out any) error {
	cli, err := GetClient()
	if err != nil {
		return err
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	q := url.Values{}
	q.Set("page_size", strconv.Itoa(pageSize))
	if pageToken != "" {
		q.Set("page_token", pageToken)
	}
	tokenType, tokenOpts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Post(Context(), path+"?"+q.Encode(), body, tokenType, tokenOpts...)
	if err != nil {
		return fmt.Errorf("%s失败: %w", action, err)
	}
	if err := CheckAPIResponse(action, resp); err != nil {
		return err
	}
	if err := json.Unmarshal(resp.RawBody, out); err != nil {
		return fmt.Errorf("%s失败: 解析响应失败: %w", action, err)
	}
	return nil
}

// parseContactDisplayInfo 解析 display_info：首行命中高亮、第二行部门、末行 [最近联系提示]。
func parseContactDisplayInfo(raw string) (segments []string, department, recency string) {
	if raw == "" {
		return nil, "", ""
	}
	for _, m := range displayInfoHighlightRE.FindAllStringSubmatch(raw, -1) {
		segments = append(segments, html.UnescapeString(m[1]))
	}
	lines := strings.Split(raw, "\n")
	if len(lines) >= 2 {
		department = strings.TrimSpace(lines[1])
		if displayInfoRecencyRE.MatchString(department) {
			department = ""
		}
	}
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if m := displayInfoRecencyRE.FindStringSubmatch(line); m != nil {
			recency = m[1]
		}
		break
	}
	return segments, department, recency
}

// pickLocalizedName 按 --lang → zh_cn → en_us → 其他语言（字典序）→ open_id 的顺序挑选姓名。
func pickLocalizedName(i18n map[string]string, lang, fallback string) string {
	var order []string
	if lang != "" {
		order = append(order, strings.ReplaceAll(strings.ToLower(lang), "-", "_"))
	}
	order = append(order, "zh_cn", "en_us", "ja_jp", "zh_hk", "zh_tw")
	for _, loc := range order {
		if v := i18n[loc]; v != "" {
			return v
		}
	}
	keys := make([]string, 0, len(i18n))
	for k := range i18n {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if v := i18n[k]; v != "" {
			return v
		}
	}
	return fallback
}

// GetUserBasicInfo 以 User 身份查询用户基础资料（POST /contact/v3/users/basic_batch，对齐官方
// contact +get-user 的 user 分支）：权限更轻（contact:user.basic_profile:readonly），
// 只返回姓名与多语言名，不含邮箱/手机号/部门等敏感字段。
func GetUserBasicInfo(userID, userIDType, userAccessToken string) (*UserInfo, error) {
	if userAccessToken == "" {
		return nil, fmt.Errorf("以用户身份查询用户资料需要 User Access Token")
	}
	if userIDType == "" {
		userIDType = "open_id"
	}
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	tokenType, tokenOpts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Post(Context(), "/open-apis/contact/v3/users/basic_batch?user_id_type="+url.QueryEscape(userIDType),
		map[string]any{"user_ids": []string{userID}}, tokenType, tokenOpts...)
	if err != nil {
		return nil, fmt.Errorf("获取用户信息失败: %w", err)
	}
	if err := CheckAPIResponse("获取用户信息", resp); err != nil {
		return nil, err
	}
	var env struct {
		Data struct {
			Users []struct {
				UserID   string            `json:"user_id"`
				Name     string            `json:"name"`
				I18nName map[string]string `json:"i18n_name"`
			} `json:"users"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &env); err != nil {
		return nil, fmt.Errorf("获取用户信息失败: 解析响应失败: %w", err)
	}
	if len(env.Data.Users) == 0 {
		return nil, fmt.Errorf("获取用户信息失败: 未找到用户 %s（或不在可见范围内）", userID)
	}
	u := env.Data.Users[0]
	info := &UserInfo{Name: u.Name, EnName: u.I18nName["en_us"]}
	switch userIDType {
	case "union_id":
		info.UnionID = u.UserID
	case "user_id":
		info.UserID = u.UserID
	default:
		info.OpenID = u.UserID
	}
	return info, nil
}
