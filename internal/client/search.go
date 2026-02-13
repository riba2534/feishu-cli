package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

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

// SearchDocsOptions 搜索文档的选项
type SearchDocsOptions struct {
	Query        string   // 搜索关键词
	DocTypes     []string // 文档类型（doc/sheet/slides/wiki_database/wiki_doc/wiki_note/wiki_sheet）
	OwnerIDs     []string // 文档所有者 ID 列表
	ChatIDs      []string // 文档所在群组 ID 列表
	CreatorIDs   []string // 文档创建者 ID 列表
	DocCreatedAt string   // 文档创建时间筛选（格式: >=yyyy-MM-dd 或 <=yyyy-MM-dd）
	DocUpdatedAt string   // 文档更新时间筛选
	PageSize     int      // 每页数量
	PageToken    string   // 分页 token
	UserIDType   string   // 用户 ID 类型
}

// DocInfo 文档信息
type DocInfo struct {
	DocToken  string // 文档 token
	DocType   string // 文档类型
	DocName   string // 文档名称
	OwnerID   string // 所有者 ID
	OwnerName string // 所有者名称
	CreatorID string // 创建者 ID
	CreateTime int64 // 创建时间
	UpdateTime int64 // 更新时间
}

// SearchDocsResult 搜索文档的结果
type SearchDocsResult struct {
	Docs      []*DocInfo // 文档列表
	PageToken string     // 分页 token
	HasMore   bool       // 是否有更多
}

// SearchDocs 搜索文档
// 注意：此 API 需要 User Access Token 和 search:docs:read 权限
func SearchDocs(opts SearchDocsOptions, userAccessToken string) (*SearchDocsResult, error) {
	httpClient := &http.Client{Timeout: 30 * time.Second}

	// 构建请求体
	reqBody := map[string]interface{}{
		"search_key": opts.Query,
	}

	// 分页参数
	if opts.PageSize > 0 {
		reqBody["count"] = opts.PageSize
	} else {
		reqBody["count"] = 20
	}

	// 解析 page_token 作为 offset
	if opts.PageToken != "" {
		var offset int
		fmt.Sscanf(opts.PageToken, "%d", &offset)
		reqBody["offset"] = offset
	} else {
		reqBody["offset"] = 0
	}

	if len(opts.DocTypes) > 0 {
		reqBody["docs_types"] = opts.DocTypes
	}
	if len(opts.OwnerIDs) > 0 {
		reqBody["owner_ids"] = opts.OwnerIDs
	}
	if len(opts.ChatIDs) > 0 {
		reqBody["chat_ids"] = opts.ChatIDs
	}
	// 注意：此 API 不支持 creator_ids、doc_created_at、doc_updated_at 参数

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	// 构建 URL - 使用云文档搜索 API
	apiURL := "https://open.feishu.cn/open-apis/suite/docs-api/search/object"

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", userAccessToken))

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("搜索文档请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	// 使用 map 解析响应
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	// 检查 code
	code, _ := result["code"].(float64)
	if code != 0 {
		msg, _ := result["msg"].(string)
		return nil, fmt.Errorf("搜索文档失败: code=%d, msg=%s", int(code), msg)
	}

	// 解析数据
	searchResult := &SearchDocsResult{
		Docs:    []*DocInfo{},
		HasMore: false,
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return searchResult, nil
	}

	// 解析 docs_entities (API 文档中的字段名)
	docsEntities, ok := data["docs_entities"].([]interface{})
	if !ok {
		return searchResult, nil
	}

	for _, item := range docsEntities {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		doc := &DocInfo{}
		// API 返回的是 docs_token, docs_type, title, owner_id
		if v, ok := itemMap["docs_token"].(string); ok {
			doc.DocToken = v
		}
		if v, ok := itemMap["docs_type"].(string); ok {
			doc.DocType = v
		}
		if v, ok := itemMap["title"].(string); ok {
			doc.DocName = v
		}
		if v, ok := itemMap["owner_id"].(string); ok {
			doc.OwnerID = v
		}

		searchResult.Docs = append(searchResult.Docs, doc)
	}

	// 解析分页信息
	if v, ok := data["has_more"].(bool); ok {
		searchResult.HasMore = v
	}
	// 计算下一页的 offset
	if searchResult.HasMore {
		totalDocs := len(searchResult.Docs)
		currentOffset := 0
		if opts.PageToken != "" {
			fmt.Sscanf(opts.PageToken, "%d", &currentOffset)
		}
		searchResult.PageToken = fmt.Sprintf("%d", currentOffset+totalDocs)
	}

	return searchResult, nil
}
