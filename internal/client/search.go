package client

import (
	"encoding/json"
	"fmt"
	"net/http"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larksearch "github.com/larksuite/oapi-sdk-go/v3/service/search/v2"
	"github.com/riba2534/feishu-cli/internal/config"
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
		UserTokenOption(userAccessToken)...)
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
		UserTokenOption(userAccessToken)...)
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
	Query    string   // 搜索关键词
	Count    int      // 返回数量（0-50）
	Offset   int      // 偏移量（offset + count < 200）
	OwnerIDs []string // 文件所有者 Open ID 列表
	ChatIDs  []string // 文件所在群 ID 列表
	DocTypes []string // 文档类型（doc/docx/sheet/slides/bitable/mindnote/file/wiki/shortcut）
}

// SearchDocWikiResult 搜索文档和 Wiki 的结果
type SearchDocWikiResult struct {
	Total    int               // 总结果数
	HasMore  bool              // 是否有更多
	ResUnits []*DocWikiResUnit // 搜索结果列表
}

// DocWikiResUnit 文档搜索结果单元
type DocWikiResUnit struct {
	DocsToken string // 文档 Token
	DocsType  string // 文档类型
	Title     string // 标题
	OwnerID   string // 所有者 ID
	URL       string // 文档 URL（根据类型和 Token 拼接）
}

// docsTypeURLPath 文档类型到 URL 路径的映射
var docsTypeURLPath = map[string]string{
	"doc":      "docx",
	"docx":     "docx",
	"sheet":    "sheets",
	"bitable":  "base",
	"mindnote": "mindnotes",
	"file":     "file",
	"slides":   "slides",
	"wiki":     "wiki",
	"shortcut": "docx",
}

// buildDocsURL 根据文档类型和 Token 拼接飞书文档 URL
func buildDocsURL(docsType, docsToken string) string {
	path, ok := docsTypeURLPath[docsType]
	if !ok {
		path = docsType
	}
	return fmt.Sprintf("%s/%s/%s", config.ResolveWebBaseURL(config.Get()), path, docsToken)
}

// SearchDocWiki 搜索云文档
// 使用 /open-apis/suite/docs-api/search/object 端点
// 注意：此 API 需要 User Access Token
func SearchDocWiki(opts SearchDocWikiOptions, userAccessToken string) (*SearchDocWikiResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	// 构建请求体
	body := map[string]any{
		"search_key": opts.Query,
	}
	if opts.Count > 0 {
		body["count"] = opts.Count
	}
	if opts.Offset > 0 {
		body["offset"] = opts.Offset
	}
	if len(opts.OwnerIDs) > 0 {
		body["owner_ids"] = opts.OwnerIDs
	}
	if len(opts.ChatIDs) > 0 {
		body["chat_ids"] = opts.ChatIDs
	}
	if len(opts.DocTypes) > 0 {
		body["docs_types"] = opts.DocTypes
	}

	apiPath := "/open-apis/suite/docs-api/search/object"

	resp, err := client.Post(Context(), apiPath, body,
		larkcore.AccessTokenTypeUser,
		UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("搜索文档失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("搜索文档失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}

	// 解析响应
	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			DocsEntities []struct {
				DocsToken string `json:"docs_token"`
				DocsType  string `json:"docs_type"`
				Title     string `json:"title"`
				OwnerID   string `json:"owner_id"`
			} `json:"docs_entities"`
			HasMore bool `json:"has_more"`
			Total   int  `json:"total"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析搜索响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("搜索文档失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	result := &SearchDocWikiResult{
		Total:    apiResp.Data.Total,
		HasMore:  apiResp.Data.HasMore,
		ResUnits: make([]*DocWikiResUnit, 0, len(apiResp.Data.DocsEntities)),
	}

	for _, entity := range apiResp.Data.DocsEntities {
		result.ResUnits = append(result.ResUnits, &DocWikiResUnit{
			DocsToken: entity.DocsToken,
			DocsType:  entity.DocsType,
			Title:     entity.Title,
			OwnerID:   entity.OwnerID,
			URL:       buildDocsURL(entity.DocsType, entity.DocsToken),
		})
	}

	return result, nil
}
