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
