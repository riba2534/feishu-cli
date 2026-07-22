package client

import (
	"fmt"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// ChatListItem 群列表项（当前身份加入的群）
type ChatListItem struct {
	ChatID      string `json:"chat_id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	OwnerID     string `json:"owner_id,omitempty"`
	OwnerIDType string `json:"owner_id_type,omitempty"`
	External    bool   `json:"external"`
	TenantKey   string `json:"tenant_key,omitempty"`
	ChatStatus  string `json:"chat_status,omitempty"`
	Avatar      string `json:"avatar,omitempty"`
}

// ListChatsResult 群列表结果（单页）
type ListChatsResult struct {
	Items     []*ChatListItem `json:"items"`
	PageToken string          `json:"page_token,omitempty"`
	HasMore   bool            `json:"has_more"`
}

// ListChats 列出当前身份（User 或 Tenant）加入的群，单页返回。
// userAccessToken 为空时走 App/Tenant Token（列 Bot 加入的群），非空时列该用户加入的群。
// sortType 支持 ByCreateTimeAsc（按创建时间升序）/ ByActiveTimeDesc（按活跃时间降序）。
func ListChats(userIDType, sortType string, pageSize int, pageToken string, userAccessToken string) (*ListChatsResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	reqBuilder := larkim.NewListChatReqBuilder()
	if userIDType != "" {
		reqBuilder.UserIdType(userIDType)
	}
	if sortType != "" {
		reqBuilder.SortType(sortType)
	}
	if pageSize > 0 {
		reqBuilder.PageSize(pageSize)
	}
	if pageToken != "" {
		reqBuilder.PageToken(pageToken)
	}

	resp, err := client.Im.Chat.List(Context(), reqBuilder.Build(), UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("获取群列表失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取群列表失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	result := &ListChatsResult{
		PageToken: StringVal(resp.Data.PageToken),
		HasMore:   BoolVal(resp.Data.HasMore),
	}
	for _, item := range resp.Data.Items {
		result.Items = append(result.Items, &ChatListItem{
			ChatID:      StringVal(item.ChatId),
			Name:        StringVal(item.Name),
			Description: StringVal(item.Description),
			OwnerID:     StringVal(item.OwnerId),
			OwnerIDType: StringVal(item.OwnerIdType),
			External:    BoolVal(item.External),
			TenantKey:   StringVal(item.TenantKey),
			ChatStatus:  StringVal(item.ChatStatus),
			Avatar:      StringVal(item.Avatar),
		})
	}

	return result, nil
}

// ChatMembersPage 群成员列表单页结果，额外带上服务端返回的成员总数（member_total）。
// member_total 用于识别"安全设置截断"：当翻页结束（HasMore=false）后仍有
// MemberTotal > 已取回条数时，说明群配置限制了成员可见性，返回的名单不完整。
type ChatMembersPage struct {
	Items       []*ChatMemberInfo
	PageToken   string
	HasMore     bool
	MemberTotal int
}

// ListChatMembersPage 获取群成员列表单页，复用 ChatMemberInfo，并额外返回 member_total。
// 供 `chat member list --page-all` 自动翻页 + 截断检测使用。
func ListChatMembersPage(chatID, memberIDType string, pageSize int, pageToken string, userAccessToken string) (*ChatMembersPage, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	reqBuilder := larkim.NewGetChatMembersReqBuilder().
		ChatId(chatID)

	if memberIDType != "" {
		reqBuilder.MemberIdType(memberIDType)
	}
	if pageSize > 0 {
		reqBuilder.PageSize(pageSize)
	}
	if pageToken != "" {
		reqBuilder.PageToken(pageToken)
	}

	resp, err := client.Im.ChatMembers.Get(Context(), reqBuilder.Build(), UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("获取群成员列表失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取群成员列表失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	page := &ChatMembersPage{
		PageToken:   StringVal(resp.Data.PageToken),
		HasMore:     BoolVal(resp.Data.HasMore),
		MemberTotal: IntVal(resp.Data.MemberTotal),
	}
	for _, item := range resp.Data.Items {
		page.Items = append(page.Items, &ChatMemberInfo{
			MemberIDType: StringVal(item.MemberIdType),
			MemberID:     StringVal(item.MemberId),
			Name:         StringVal(item.Name),
			TenantKey:    StringVal(item.TenantKey),
		})
	}

	return page, nil
}
