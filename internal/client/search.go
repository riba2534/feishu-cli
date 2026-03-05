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
		PageToken:  StringVal(resp.Data.PageToken),
		HasMore:    BoolVal(resp.Data.HasMore),
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
		AppIDs:    resp.Data.Items,
		PageToken: StringVal(resp.Data.PageToken),
		HasMore:   BoolVal(resp.Data.HasMore),
	}

	return result, nil
}

// SearchDocWikiOptions 搜索文档和 Wiki 的选项
type SearchDocWikiOptions struct {
	Query        string   // 搜索关键词
	DocTypes     []string // 文档类型（doc/sheet/bitable/wiki/mindnote/file）
	FolderTokens []string // 文件夹 Token 列表
	SpaceIDs     []string // Wiki 空间 ID 列表
	CreatorIDs   []string // 创建者 ID 列表
	OnlyTitle    *bool    // 仅搜索标题
	SortType     string   // 排序方式（EditedTime/CreatedTime/OpenedTime）
	PageSize     int      // 每页数量
	PageToken    string   // 分页 token
}

// SearchDocWikiResult 搜索文档和 Wiki 的结果
type SearchDocWikiResult struct {
	Total     int               // 总结果数
	HasMore   bool              // 是否有更多
	ResUnits  []*DocWikiResUnit // 搜索结果列表
	PageToken string            // 分页 token
}

// DocWikiResUnit 文档搜索结果单元
type DocWikiResUnit struct {
	TitleHighlighted   string // 高亮标题
	SummaryHighlighted string // 高亮摘要
	EntityType         string // 结果类型
	URL                string // 文档 URL
	Token              string // 文档 Token
	OwnerID            string // 所有者 ID
	OwnerName          string // 所有者名称
	DocTypes           string // 文档类型
	CreateTime         int64  // 创建时间
	UpdateTime         int64  // 更新时间
}

// SearchDocWiki 搜索文档和 Wiki
// 注意：此 API 需要 User Access Token
func SearchDocWiki(opts SearchDocWikiOptions, userAccessToken string) (*SearchDocWikiResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	// 构建请求体
	bodyBuilder := larksearch.NewSearchDocWikiReqBodyBuilder().
		Query(opts.Query)

	// 如果有文档类型过滤
	if len(opts.DocTypes) > 0 || len(opts.FolderTokens) > 0 || len(opts.CreatorIDs) > 0 || opts.OnlyTitle != nil || opts.SortType != "" {
		// 判断是否包含 wiki 类型
		hasWiki := false
		for _, dt := range opts.DocTypes {
			if dt == "wiki" {
				hasWiki = true
				break
			}
		}

		if hasWiki && len(opts.SpaceIDs) > 0 {
			// 使用 WikiFilter
			wikiFilterBuilder := larksearch.NewWikiFilterBuilder()
			if len(opts.SpaceIDs) > 0 {
				wikiFilterBuilder.SpaceIds(opts.SpaceIDs)
			}
			if len(opts.CreatorIDs) > 0 {
				wikiFilterBuilder.CreatorIds(opts.CreatorIDs)
			}
			if len(opts.DocTypes) > 0 {
				wikiFilterBuilder.DocTypes(opts.DocTypes)
			}
			if opts.OnlyTitle != nil {
				wikiFilterBuilder.OnlyTitle(*opts.OnlyTitle)
			}
			if opts.SortType != "" {
				wikiFilterBuilder.SortType(opts.SortType)
			}
			bodyBuilder.WikiFilter(wikiFilterBuilder.Build())
		} else {
			// 使用 DocFilter
			docFilterBuilder := larksearch.NewDocFilterBuilder()
			if len(opts.DocTypes) > 0 {
				docFilterBuilder.DocTypes(opts.DocTypes)
			}
			if len(opts.FolderTokens) > 0 {
				docFilterBuilder.FolderTokens(opts.FolderTokens)
			}
			if len(opts.CreatorIDs) > 0 {
				docFilterBuilder.CreatorIds(opts.CreatorIDs)
			}
			if opts.OnlyTitle != nil {
				docFilterBuilder.OnlyTitle(*opts.OnlyTitle)
			}
			if opts.SortType != "" {
				docFilterBuilder.SortType(opts.SortType)
			}
			bodyBuilder.DocFilter(docFilterBuilder.Build())
		}
	}

	// 设置分页参数
	if opts.PageSize > 0 {
		bodyBuilder.PageSize(opts.PageSize)
	}
	if opts.PageToken != "" {
		bodyBuilder.PageToken(opts.PageToken)
	}

	// 构建请求
	req := larksearch.NewSearchDocWikiReqBuilder().
		Body(bodyBuilder.Build()).
		Build()

	// 使用 User Access Token 调用 API
	resp, err := client.Search.V2.DocWiki.Search(Context(), req,
		larkcore.WithUserAccessToken(userAccessToken))
	if err != nil {
		return nil, fmt.Errorf("搜索文档失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("搜索文档失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	// 解析结果
	result := &SearchDocWikiResult{
		ResUnits: make([]*DocWikiResUnit, 0),
	}

	if resp.Data != nil {
		if resp.Data.Total != nil {
			result.Total = *resp.Data.Total
		}
		if resp.Data.HasMore != nil {
			result.HasMore = *resp.Data.HasMore
		}
		if resp.Data.PageToken != nil {
			result.PageToken = *resp.Data.PageToken
		}

		// 解析每个结果单元
		for _, item := range resp.Data.ResUnits {
			unit := &DocWikiResUnit{}

			if item.TitleHighlighted != nil {
				unit.TitleHighlighted = *item.TitleHighlighted
			}
			if item.SummaryHighlighted != nil {
				unit.SummaryHighlighted = *item.SummaryHighlighted
			}
			if item.EntityType != nil {
				unit.EntityType = *item.EntityType
			}

			// 解析元数据
			if item.ResultMeta != nil {
				if item.ResultMeta.Url != nil {
					unit.URL = *item.ResultMeta.Url
				}
				if item.ResultMeta.Token != nil {
					unit.Token = *item.ResultMeta.Token
				}
				if item.ResultMeta.OwnerId != nil {
					unit.OwnerID = *item.ResultMeta.OwnerId
				}
				if item.ResultMeta.OwnerName != nil {
					unit.OwnerName = *item.ResultMeta.OwnerName
				}
				if item.ResultMeta.DocTypes != nil {
					unit.DocTypes = *item.ResultMeta.DocTypes
				}
				if item.ResultMeta.CreateTime != nil {
					unit.CreateTime = int64(*item.ResultMeta.CreateTime)
				}
				if item.ResultMeta.UpdateTime != nil {
					unit.UpdateTime = int64(*item.ResultMeta.UpdateTime)
				}
			}

			result.ResUnits = append(result.ResUnits, unit)
		}
	}

	return result, nil
}
