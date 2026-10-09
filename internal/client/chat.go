package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// CreateChatOptions 创建群聊参数（POST /open-apis/im/v1/chats，仅 Bot/tenant 身份，对齐官方 #2728）。
type CreateChatOptions struct {
	Name        string
	Description string
	OwnerID     string   // open_id
	UserIDs     []string // open_id 列表
	BotIDs      []string // 机器人 app_id（cli_xxx），最多 5 个
	ChatType    string   // private / public
	ChatMode    string   // group（默认）/ topic（话题群）
}

// CreatedChat 创建群聊的返回摘要。
type CreatedChat struct {
	ChatID   string `json:"chat_id"`
	Name     string `json:"name,omitempty"`
	ChatType string `json:"chat_type,omitempty"`
	ChatMode string `json:"chat_mode,omitempty"`
	OwnerID  string `json:"owner_id,omitempty"`
	External bool   `json:"external"`
}

// CreateChat 创建群聊（兼容旧签名）。
func CreateChat(name, description, ownerID string, userIDs []string, chatType string) (string, error) {
	created, err := CreateChatWithOptions(CreateChatOptions{
		Name: name, Description: description, OwnerID: ownerID, UserIDs: userIDs, ChatType: chatType,
	})
	if err != nil {
		return "", err
	}
	return created.ChatID, nil
}

// CreateChatWithOptions 创建群聊，支持话题群（chat_mode=topic）与邀请机器人。
func CreateChatWithOptions(opts CreateChatOptions) (*CreatedChat, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	bodyBuilder := larkim.NewCreateChatReqBodyBuilder()
	if opts.Name != "" {
		bodyBuilder.Name(opts.Name)
	}
	if opts.Description != "" {
		bodyBuilder.Description(opts.Description)
	}
	if opts.OwnerID != "" {
		bodyBuilder.OwnerId(opts.OwnerID)
	}
	if len(opts.UserIDs) > 0 {
		bodyBuilder.UserIdList(opts.UserIDs)
	}
	if len(opts.BotIDs) > 0 {
		bodyBuilder.BotIdList(opts.BotIDs)
	}
	if opts.ChatType != "" {
		bodyBuilder.ChatType(opts.ChatType)
	}
	if opts.ChatMode != "" {
		bodyBuilder.ChatMode(opts.ChatMode)
	}

	req := larkim.NewCreateChatReqBuilder().
		UserIdType("open_id").
		Body(bodyBuilder.Build()).
		Build()

	resp, err := client.Im.Chat.Create(Context(), req)
	if err != nil {
		return nil, fmt.Errorf("创建群聊失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("创建群聊失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data == nil || resp.Data.ChatId == nil {
		return nil, fmt.Errorf("群聊已创建但未返回群 ID")
	}

	return &CreatedChat{
		ChatID:   StringVal(resp.Data.ChatId),
		Name:     StringVal(resp.Data.Name),
		ChatType: StringVal(resp.Data.ChatType),
		ChatMode: StringVal(resp.Data.ChatMode),
		OwnerID:  StringVal(resp.Data.OwnerId),
		External: BoolVal(resp.Data.External),
	}, nil
}

// GetChat 获取群聊信息
func GetChat(chatID string, userAccessToken string) (*larkim.GetChatRespData, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larkim.NewGetChatReqBuilder().
		ChatId(chatID).
		Build()

	resp, err := client.Im.Chat.Get(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("获取群聊信息失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取群聊信息失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data, nil
}

// UpdateChat 更新群聊信息
func UpdateChat(chatID, name, description, ownerID string, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	bodyBuilder := larkim.NewUpdateChatReqBodyBuilder()
	if name != "" {
		bodyBuilder.Name(name)
	}
	if description != "" {
		bodyBuilder.Description(description)
	}
	if ownerID != "" {
		bodyBuilder.OwnerId(ownerID)
	}

	req := larkim.NewUpdateChatReqBuilder().
		ChatId(chatID).
		Body(bodyBuilder.Build()).
		Build()

	resp, err := client.Im.Chat.Update(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return fmt.Errorf("更新群聊信息失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("更新群聊信息失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// DeleteChat 解散群聊
func DeleteChat(chatID string, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	req := larkim.NewDeleteChatReqBuilder().
		ChatId(chatID).
		Build()

	resp, err := client.Im.Chat.Delete(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return fmt.Errorf("解散群聊失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("解散群聊失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// GetChatLink 获取群分享链接
func GetChatLink(chatID string, validityPeriod string) (string, error) {
	client, err := GetClient()
	if err != nil {
		return "", err
	}

	bodyBuilder := larkim.NewLinkChatReqBodyBuilder()
	if validityPeriod != "" {
		bodyBuilder.ValidityPeriod(validityPeriod)
	}

	req := larkim.NewLinkChatReqBuilder().
		ChatId(chatID).
		Body(bodyBuilder.Build()).
		Build()

	resp, err := client.Im.Chat.Link(Context(), req)
	if err != nil {
		return "", fmt.Errorf("获取群分享链接失败: %w", err)
	}

	if !resp.Success() {
		return "", fmt.Errorf("获取群分享链接失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data.ShareLink == nil {
		return "", fmt.Errorf("获取群分享链接成功但未返回链接")
	}

	return *resp.Data.ShareLink, nil
}

// ChatMemberInfo 群成员信息（用户成员）
type ChatMemberInfo struct {
	MemberIDType string `json:"member_id_type"`
	MemberID     string `json:"member_id"`
	Name         string `json:"name"`
	TenantKey    string `json:"tenant_key"`
}

// ChatBotMemberInfo 群内机器人成员信息（/members/list 的 bots[]）
type ChatBotMemberInfo struct {
	MemberIDType string `json:"member_id_type,omitempty"`
	MemberID     string `json:"member_id"`
	Name         string `json:"name"`
	AppID        string `json:"app_id,omitempty"`
	TenantKey    string `json:"tenant_key,omitempty"`
}

// ListChatMembersResult 群成员列表结果。
//
// 底层走 GET /open-apis/im/v1/chats/{chat_id}/members/list（对齐官方 +chat-members-list）：
// 旧端点 GET /chats/{chat_id}/members 拿不到群内机器人，新端点把用户与机器人分桶返回。
// 兼容：Items 仍只含用户成员（与旧端点语义一致，避免脚本把 Bot 当人处理）；
// Users 与 Items 相同，Bots 为机器人成员；Truncations 非空表示服务端因群安全设置截断了名单。
type ListChatMembersResult struct {
	Items       []*ChatMemberInfo    `json:"items"`
	Users       []*ChatMemberInfo    `json:"users"`
	Bots        []*ChatBotMemberInfo `json:"bots"`
	Truncations []map[string]any     `json:"truncations"`
	UserTotal   *int                 `json:"user_total,omitempty"`
	BotTotal    *int                 `json:"bot_total,omitempty"`
	PageToken   string               `json:"page_token,omitempty"`
	HasMore     bool                 `json:"has_more"`
}

// ChatMembersListOptions 群成员列表查询参数。
type ChatMembersListOptions struct {
	MemberIDType string // open_id / union_id / user_id，空则 open_id
	MemberTypes  string // "user" / "bot" / "user,bot"，空表示全部
	PageSize     int    // 1-100，0 表示服务端默认
	PageToken    string
}

type chatMembersListRaw struct {
	Users []struct {
		MemberID  string `json:"member_id"`
		Name      string `json:"name"`
		TenantKey string `json:"tenant_key"`
	} `json:"users"`
	Bots []struct {
		MemberID  string `json:"member_id"`
		Name      string `json:"name"`
		AppID     string `json:"app_id"`
		TenantKey string `json:"tenant_key"`
	} `json:"bots"`
	Truncations []map[string]any `json:"truncations"`
	UserTotal   *int             `json:"user_total"`
	BotTotal    *int             `json:"bot_total"`
	HasMore     bool             `json:"has_more"`
	PageToken   string           `json:"page_token"`
}

// ListChatMembersV2 获取群成员单页（用户 + 机器人分桶）。
// userAccessToken 为空时以 Bot 身份调用（Bot 需在群内）。
func ListChatMembersV2(chatID string, opts ChatMembersListOptions, userAccessToken string) (*ListChatMembersResult, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	memberIDType := opts.MemberIDType
	if memberIDType == "" {
		memberIDType = "open_id"
	}
	q := url.Values{}
	q.Set("member_id_type", memberIDType)
	if opts.MemberTypes != "" {
		q.Set("member_types", opts.MemberTypes)
	}
	if opts.PageSize > 0 {
		q.Set("page_size", strconv.Itoa(opts.PageSize))
	}
	if opts.PageToken != "" {
		q.Set("page_token", opts.PageToken)
	}
	apiPath := "/open-apis/im/v1/chats/" + url.PathEscape(chatID) + "/members/list?" + q.Encode()

	tokenType, tokenOpts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Get(Context(), apiPath, nil, tokenType, tokenOpts...)
	if err != nil {
		return nil, fmt.Errorf("获取群成员列表失败: %w", err)
	}
	if err := CheckAPIResponse("获取群成员列表", resp); err != nil {
		return nil, err
	}
	var env struct {
		Code int                `json:"code"`
		Msg  string             `json:"msg"`
		Data chatMembersListRaw `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &env); err != nil {
		return nil, fmt.Errorf("获取群成员列表失败: 解析响应失败: %w", err)
	}

	result := &ListChatMembersResult{
		Items:       []*ChatMemberInfo{},
		Bots:        []*ChatBotMemberInfo{},
		Truncations: env.Data.Truncations,
		UserTotal:   env.Data.UserTotal,
		BotTotal:    env.Data.BotTotal,
		PageToken:   env.Data.PageToken,
		HasMore:     env.Data.HasMore,
	}
	if result.Truncations == nil {
		result.Truncations = []map[string]any{}
	}
	for _, u := range env.Data.Users {
		result.Items = append(result.Items, &ChatMemberInfo{
			MemberIDType: memberIDType,
			MemberID:     u.MemberID,
			Name:         u.Name,
			TenantKey:    u.TenantKey,
		})
	}
	for _, b := range env.Data.Bots {
		result.Bots = append(result.Bots, &ChatBotMemberInfo{
			MemberIDType: memberIDType,
			MemberID:     b.MemberID,
			Name:         b.Name,
			AppID:        b.AppID,
			TenantKey:    b.TenantKey,
		})
	}
	result.Users = result.Items
	return result, nil
}

// ListChatMembers 获取群成员列表单页（兼容旧签名，底层已切换到 /members/list）。
func ListChatMembers(chatID, memberIDType string, pageSize int, pageToken string, userAccessToken string) (*ListChatMembersResult, error) {
	return ListChatMembersV2(chatID, ChatMembersListOptions{
		MemberIDType: memberIDType,
		PageSize:     pageSize,
		PageToken:    pageToken,
	}, userAccessToken)
}

// AppendPage 把一页成员合并进汇总结果：users/bots 拼接；has_more、page_token、
// truncations、*_total 取最后一页（truncations 只在最后一页下发，取早了会漏掉截断信号）。
func (r *ListChatMembersResult) AppendPage(page *ListChatMembersResult) {
	if page == nil {
		return
	}
	r.Items = append(r.Items, page.Items...)
	r.Users = r.Items
	r.Bots = append(r.Bots, page.Bots...)
	r.Truncations = page.Truncations
	r.UserTotal = page.UserTotal
	r.BotTotal = page.BotTotal
	r.HasMore = page.HasMore
	r.PageToken = page.PageToken
}

// AddChatMembers 添加群成员
func AddChatMembers(chatID, memberIDType string, idList []string, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	reqBuilder := larkim.NewCreateChatMembersReqBuilder().
		ChatId(chatID).
		Body(larkim.NewCreateChatMembersReqBodyBuilder().
			IdList(idList).
			Build())

	if memberIDType != "" {
		reqBuilder.MemberIdType(memberIDType)
	}

	resp, err := client.Im.ChatMembers.Create(Context(), reqBuilder.Build(), UserTokenOption(userAccessToken)...)
	if err != nil {
		return fmt.Errorf("添加群成员失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("添加群成员失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// RemoveChatMembers 移除群成员
func RemoveChatMembers(chatID, memberIDType string, idList []string, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	reqBuilder := larkim.NewDeleteChatMembersReqBuilder().
		ChatId(chatID).
		Body(larkim.NewDeleteChatMembersReqBodyBuilder().
			IdList(idList).
			Build())

	if memberIDType != "" {
		reqBuilder.MemberIdType(memberIDType)
	}

	resp, err := client.Im.ChatMembers.Delete(Context(), reqBuilder.Build(), UserTokenOption(userAccessToken)...)
	if err != nil {
		return fmt.Errorf("移除群成员失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("移除群成员失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}
