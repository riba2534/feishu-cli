package client

import (
	"fmt"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larksearch "github.com/larksuite/oapi-sdk-go/v3/service/search/v2"
)

// SearchMessagesOptions 搜索消息的选项
type SearchMessagesOptions struct {
	Query        string   // 搜索关键词
	FromIDs      []string // 消息来自用户 ID 列表
	ChatIDs      []string // 消息所在会话 ID 列表
	MessageType  string   // 消息类型（file/image/media）
	AtChatterIDs []string // @用户 ID 列表
	FromType     string   // 消息来自类型（bot/user）
	ChatType     string   // 会话类型（group_chat/p2p_chat）
	StartTime    string   // 消息发送起始时间
	EndTime      string   // 消息发送结束时间
	PageSize     int      // 每页数量
	PageToken    string   // 分页 token
	UserIDType   string   // 用户 ID 类型（open_id/union_id/user_id）
}

// SearchMessagesResult 搜索消息的结果
type SearchMessagesResult struct {
	MessageIDs []string // 消息 ID 列表
	PageToken  string   // 分页 token
	HasMore    bool     // 是否有更多
}

// SearchMessages 搜索消息
// 注意：此 API 需要 User Access Token
func SearchMessages(opts SearchMessagesOptions, userAccessToken string) (*SearchMessagesResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	bodyBuilder := larksearch.NewCreateMessageReqBodyBuilder().
		Query(opts.Query)

	if len(opts.FromIDs) > 0 {
		bodyBuilder.FromIds(opts.FromIDs)
	}
	if len(opts.ChatIDs) > 0 {
		bodyBuilder.ChatIds(opts.ChatIDs)
	}
	if opts.MessageType != "" {
		bodyBuilder.MessageType(opts.MessageType)
	}
	if len(opts.AtChatterIDs) > 0 {
		bodyBuilder.AtChatterIds(opts.AtChatterIDs)
	}
	if opts.FromType != "" {
		bodyBuilder.FromType(opts.FromType)
	}
	if opts.ChatType != "" {
		bodyBuilder.ChatType(opts.ChatType)
	}
	if opts.StartTime != "" {
		bodyBuilder.StartTime(opts.StartTime)
	}
	if opts.EndTime != "" {
		bodyBuilder.EndTime(opts.EndTime)
	}

	reqBuilder := larksearch.NewCreateMessageReqBuilder().
		Body(bodyBuilder.Build())

	if opts.PageSize > 0 {
		reqBuilder.PageSize(opts.PageSize)
	}
	if opts.PageToken != "" {
		reqBuilder.PageToken(opts.PageToken)
	}
	if opts.UserIDType != "" {
		reqBuilder.UserIdType(opts.UserIDType)
	}

	// 使用 User Access Token 调用 API
	resp, err := client.Search.Message.Create(Context(), reqBuilder.Build(),
		larkcore.WithUserAccessToken(userAccessToken))
	if err != nil {
		return nil, fmt.Errorf("搜索消息失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("搜索消息失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	result := &SearchMessagesResult{
		MessageIDs: resp.Data.Items,
	}
	if resp.Data.PageToken != nil {
		result.PageToken = *resp.Data.PageToken
	}
	if resp.Data.HasMore != nil {
		result.HasMore = *resp.Data.HasMore
	}

	return result, nil
}

// SearchAppsOptions 搜索应用的选项
type SearchAppsOptions struct {
	Query      string // 搜索关键词
	PageSize   int    // 每页数量
	PageToken  string // 分页 token
	UserIDType string // 用户 ID 类型
}

// SearchAppsResult 搜索应用的结果
type SearchAppsResult struct {
	AppIDs    []string // 应用 ID 列表
	PageToken string   // 分页 token
	HasMore   bool     // 是否有更多
}

// SearchApps 搜索应用
// 注意：此 API 需要 User Access Token
func SearchApps(opts SearchAppsOptions, userAccessToken string) (*SearchAppsResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	reqBuilder := larksearch.NewCreateAppReqBuilder().
		Body(larksearch.NewCreateAppReqBodyBuilder().
			Query(opts.Query).
			Build())

	if opts.PageSize > 0 {
		reqBuilder.PageSize(opts.PageSize)
	}
	if opts.PageToken != "" {
		reqBuilder.PageToken(opts.PageToken)
	}
	if opts.UserIDType != "" {
		reqBuilder.UserIdType(opts.UserIDType)
	}

	// 使用 User Access Token 调用 API
	resp, err := client.Search.App.Create(Context(), reqBuilder.Build(),
		larkcore.WithUserAccessToken(userAccessToken))
	if err != nil {
		return nil, fmt.Errorf("搜索应用失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("搜索应用失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	result := &SearchAppsResult{
		AppIDs: resp.Data.Items,
	}
	if resp.Data.PageToken != nil {
		result.PageToken = *resp.Data.PageToken
	}
	if resp.Data.HasMore != nil {
		result.HasMore = *resp.Data.HasMore
	}

	return result, nil
}
