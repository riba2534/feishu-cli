package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larksearch "github.com/larksuite/oapi-sdk-go/v3/service/search/v2"
)

// SearchMessagesOptions 搜索消息的选项
type SearchMessagesOptions struct {
	Query           string   // 搜索关键词（可空，支持纯 filter 搜索）
	FromIDs         []string // 消息来自用户 ID 列表
	ChatIDs         []string // 消息所在会话 ID 列表
	MessageType     string   // 附件类型（file/image/media/video/link；media→video）
	AtChatterIDs    []string // @用户 ID 列表
	FromType        string   // 消息来自类型（bot/user）
	ExcludeFromType string   // 排除发送者类型（bot/user）→ exclude_from_types
	ChatType        string   // 会话类型（group_chat/p2p_chat 或 group/p2p）
	IsAtMe          bool     // 仅 @我
	StartTime       string   // 消息发送起始时间
	EndTime         string   // 消息发送结束时间
	PageSize        int      // 每页数量
	PageToken       string   // 分页 token
	UserIDType      string   // 已废弃：current /im/v1/messages/search 忽略
}

// SearchMessagesResult 搜索消息的结果
type SearchMessagesResult struct {
	MessageIDs []string // 消息 ID 列表
	PageToken  string   // 分页 token
	HasMore    bool     // 是否有更多
	// Notice 服务端提示（如查询词超过 50 字被截断），非空时调用方应透出给用户。
	Notice string `json:"notice,omitempty"`
}

const (
	messagesSearchDefaultPageSize = 20
	messagesSearchMaxPageSize     = 50
)

func normalizeMessagesSearchTime(s string, endOfDay bool) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	t, err := ParseTimeInput(s, endOfDay)
	if err != nil {
		return "", err
	}
	return t.Format(time.RFC3339), nil
}

func mapMessagesSearchChatType(chatType string) (string, error) {
	switch chatType {
	case "":
		return "", nil
	case "group_chat", "group":
		return "group", nil
	case "p2p_chat", "p2p":
		return "p2p", nil
	default:
		return "", fmt.Errorf("不支持的会话类型 %q（group_chat/p2p_chat 或 group/p2p）", chatType)
	}
}

func mapMessagesSearchAttachmentType(messageType string) (string, error) {
	switch messageType {
	case "":
		return "", nil
	case "media":
		return "video", nil
	case "file", "image", "video", "link":
		return messageType, nil
	default:
		return "", fmt.Errorf("不支持的消息/附件类型 %q（file/image/media/video/link）", messageType)
	}
}

func mapMessagesSearchSenderType(v, flag string) (string, error) {
	switch v {
	case "":
		return "", nil
	case "bot", "user":
		return v, nil
	default:
		return "", fmt.Errorf("%s 仅支持 bot|user，得到 %q", flag, v)
	}
}

func extractSearchMessageID(item any) string {
	switch v := item.(type) {
	case string:
		return v
	case map[string]any:
		if meta, ok := v["meta_data"].(map[string]any); ok {
			if id, _ := meta["message_id"].(string); id != "" {
				return id
			}
		}
		if id, _ := v["message_id"].(string); id != "" {
			return id
		}
	}
	return ""
}

// buildMessagesSearchBody 构造 POST /im/v1/messages/search 的 current filter body。
func buildMessagesSearchBody(opts SearchMessagesOptions) (map[string]any, error) {
	filter := map[string]any{}
	start, err := normalizeMessagesSearchTime(opts.StartTime, false)
	if err != nil {
		return nil, fmt.Errorf("搜索消息失败: %w", err)
	}
	end, err := normalizeMessagesSearchTime(opts.EndTime, true)
	if err != nil {
		return nil, fmt.Errorf("搜索消息失败: %w", err)
	}
	if start != "" && end != "" {
		st, err1 := time.Parse(time.RFC3339, start)
		et, err2 := time.Parse(time.RFC3339, end)
		if err1 == nil && err2 == nil && st.After(et) {
			return nil, fmt.Errorf("搜索消息失败: 起始时间不能晚于结束时间")
		}
	}
	if start != "" || end != "" {
		tr := map[string]any{}
		if start != "" {
			tr["start_time"] = start
		}
		if end != "" {
			tr["end_time"] = end
		}
		filter["time_range"] = tr
	}
	if len(opts.ChatIDs) > 0 {
		filter["chat_ids"] = opts.ChatIDs
	}
	if len(opts.FromIDs) > 0 {
		filter["from_ids"] = opts.FromIDs
	}
	attach, err := mapMessagesSearchAttachmentType(opts.MessageType)
	if err != nil {
		return nil, fmt.Errorf("搜索消息失败: %w", err)
	}
	if attach != "" {
		filter["include_attachment_types"] = []string{attach}
	}
	fromType, err := mapMessagesSearchSenderType(opts.FromType, "--from-type")
	if err != nil {
		return nil, fmt.Errorf("搜索消息失败: %w", err)
	}
	excludeFrom, err := mapMessagesSearchSenderType(opts.ExcludeFromType, "--exclude-from-type")
	if err != nil {
		return nil, fmt.Errorf("搜索消息失败: %w", err)
	}
	if fromType != "" && excludeFrom != "" && fromType == excludeFrom {
		return nil, fmt.Errorf("搜索消息失败: --from-type 与 --exclude-from-type 不能相同")
	}
	if fromType != "" {
		filter["from_types"] = []string{fromType}
	}
	if excludeFrom != "" {
		filter["exclude_from_types"] = []string{excludeFrom}
	}
	chatType, err := mapMessagesSearchChatType(opts.ChatType)
	if err != nil {
		return nil, fmt.Errorf("搜索消息失败: %w", err)
	}
	if chatType != "" {
		filter["chat_type"] = chatType
	}
	if opts.IsAtMe {
		filter["is_at_me"] = true
	}
	if len(opts.AtChatterIDs) > 0 {
		filter["at_chatter_ids"] = opts.AtChatterIDs
	}

	body := map[string]any{"query": opts.Query}
	if len(filter) > 0 {
		body["filter"] = filter
	}
	return body, nil
}

// ValidateSearchMessagesOptions 在发网前校验 filter / 时间 / page-size。
func ValidateSearchMessagesOptions(opts SearchMessagesOptions) error {
	if _, err := buildMessagesSearchBody(opts); err != nil {
		return err
	}
	if _, err := ResolvePageSize(opts.PageSize, messagesSearchDefaultPageSize, 1, messagesSearchMaxPageSize); err != nil {
		return fmt.Errorf("搜索消息失败: %w", err)
	}
	return nil
}

// SearchMessages 搜索消息。
// 走 current POST /open-apis/im/v1/messages/search（支持 User / Tenant 身份）。
func SearchMessages(opts SearchMessagesOptions, userAccessToken string) (*SearchMessagesResult, error) {
	if err := ValidateSearchMessagesOptions(opts); err != nil {
		return nil, err
	}
	body, err := buildMessagesSearchBody(opts)
	if err != nil {
		return nil, err
	}
	pageSize, err := ResolvePageSize(opts.PageSize, messagesSearchDefaultPageSize, 1, messagesSearchMaxPageSize)
	if err != nil {
		return nil, fmt.Errorf("搜索消息失败: %w", err)
	}

	cli, err := GetClient()
	if err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("page_size", strconv.Itoa(pageSize))
	if opts.PageToken != "" {
		q.Set("page_token", opts.PageToken)
	}
	apiPath := "/open-apis/im/v1/messages/search?" + q.Encode()

	tokenType, tokenOpts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Post(Context(), apiPath, body, tokenType, tokenOpts...)
	if err != nil {
		return nil, fmt.Errorf("搜索消息失败: %w", err)
	}
	if err := ParseAPIResponse("搜索消息", resp.StatusCode, resp.Header, resp.RawBody); err != nil {
		return nil, err
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Items         []any  `json:"items"`
			PageToken     string `json:"page_token"`
			NextPageToken string `json:"next_page_token"`
			HasMore       bool   `json:"has_more"`
			Notice        string `json:"notice"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析搜索消息响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("搜索消息失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	ids := make([]string, 0, len(apiResp.Data.Items))
	for _, item := range apiResp.Data.Items {
		if id := extractSearchMessageID(item); id != "" {
			ids = append(ids, id)
		}
	}
	pageToken := apiResp.Data.PageToken
	if pageToken == "" {
		pageToken = apiResp.Data.NextPageToken
	}
	return &SearchMessagesResult{
		MessageIDs: ids,
		PageToken:  pageToken,
		HasMore:    apiResp.Data.HasMore,
		Notice:     apiResp.Data.Notice,
	}, nil
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

// buildDocsURL 根据文档类型和 Token 拼接文档 URL（按当前品牌选择 www.feishu.cn / www.larksuite.com）
func buildDocsURL(docsType, docsToken string) string {
	path, ok := docsTypeURLPath[docsType]
	if !ok {
		path = docsType
	}
	return fmt.Sprintf("%s/%s/%s", ResourceURLBase(), path, docsToken)
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

	if err := ParseAPIResponse("搜索文档", resp.StatusCode, resp.Header, resp.RawBody); err != nil {
		return nil, err
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

// DriveSearchOptions 是 search/v2/doc_wiki/search v2 端点的扁平 filter 选项。
// 与 v1 端点（/suite/docs-api/search/object）共存：v2 提供更丰富的过滤维度
// （folder-tokens、space-ids、creator-ids、only-title、time windows）。
type DriveSearchOptions struct {
	Query        string   // 关键字（可空，纯按 filter 浏览）
	PageToken    string   // 分页 token
	PageSize     int      // 1-20，默认 15
	CreatorIDs   []string // 创建者 open_id 列表
	FolderTokens []string // 文件夹 token 列表（限定云盘内）
	SpaceIDs     []string // 知识库 space_id 列表（限定 wiki 内）
	ChatIDs      []string // 聊天 id 列表
	SharerIDs    []string // 分享者 open_id 列表
	DocTypes     []string // doc/sheet/bitable/mindnote/file/wiki/docx/folder/catalog/slides/shortcut（大写）
	OnlyTitle    bool     // 仅匹配标题
	OnlyComment  bool     // 仅搜评论
	Sort         string   // default / edit_time / edit_time_asc / open_time / create_time
}

// DriveSearchResult 是 v2 search 的精简结果视图。
type DriveSearchResult struct {
	Total     int                      `json:"total"`
	HasMore   bool                     `json:"has_more"`
	PageToken string                   `json:"page_token,omitempty"`
	Items     []map[string]interface{} `json:"items"`
}

// DriveSearchV2 调用 /open-apis/search/v2/doc_wiki/search。
// 需要 User Access Token + scope search:docs:read。
//
// v2 端点的真实 body 是「doc_filter / wiki_filter 双 filter 并列」结构：
//   - 普通搜索时两边都填同样的 filter，让 backend 同时搜云盘 + 知识库
//   - --folder-tokens 只对 doc_filter 加 folder_tokens（限定云盘文件夹）
//   - --space-ids 只对 wiki_filter 加 space_ids（限定知识库 space）
//   - sort_type 必须全大写枚举（DEFAULT 写成 DEFAULT_TYPE）
func DriveSearchV2(opts DriveSearchOptions, userAccessToken string) (*DriveSearchResult, error) {
	c, err := GetClient()
	if err != nil {
		return nil, err
	}

	body := map[string]interface{}{
		"query": opts.Query,
	}
	if opts.PageToken != "" {
		body["page_token"] = opts.PageToken
	}
	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = 15
	}
	if pageSize > 20 {
		pageSize = 20
	}
	body["page_size"] = pageSize

	// 共享 filter（除 folder_tokens / space_ids 外的字段两边都加）
	shared := map[string]interface{}{}
	if len(opts.CreatorIDs) > 0 {
		shared["creator_ids"] = opts.CreatorIDs
	}
	if len(opts.ChatIDs) > 0 {
		shared["chat_ids"] = opts.ChatIDs
	}
	if len(opts.SharerIDs) > 0 {
		shared["sharer_ids"] = opts.SharerIDs
	}
	if len(opts.DocTypes) > 0 {
		shared["doc_types"] = opts.DocTypes
	}
	if opts.OnlyTitle {
		shared["only_title"] = true
	}
	if opts.OnlyComment {
		shared["only_comment"] = true
	}
	if opts.Sort != "" {
		sortType := strings.ToUpper(opts.Sort)
		if sortType == "DEFAULT" {
			sortType = "DEFAULT_TYPE"
		}
		shared["sort_type"] = sortType
	}

	cloneFilter := func() map[string]interface{} {
		out := make(map[string]interface{}, len(shared))
		for k, v := range shared {
			out[k] = v
		}
		return out
	}

	switch {
	case len(opts.FolderTokens) > 0:
		docFilter := cloneFilter()
		docFilter["folder_tokens"] = opts.FolderTokens
		body["doc_filter"] = docFilter
	case len(opts.SpaceIDs) > 0:
		wikiFilter := cloneFilter()
		wikiFilter["space_ids"] = opts.SpaceIDs
		body["wiki_filter"] = wikiFilter
	default:
		body["doc_filter"] = cloneFilter()
		body["wiki_filter"] = cloneFilter()
	}

	resp, err := c.Post(Context(), "/open-apis/search/v2/doc_wiki/search", body,
		larkcore.AccessTokenTypeUser, UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("drive 搜索失败: %w", err)
	}
	if err := ParseAPIResponse("drive 搜索", resp.StatusCode, resp.Header, resp.RawBody); err != nil {
		return nil, err
	}

	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Total     int                      `json:"total"`
			HasMore   bool                     `json:"has_more"`
			PageToken string                   `json:"page_token"`
			ResUnits  []map[string]interface{} `json:"res_units"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &parsed); err != nil {
		return nil, fmt.Errorf("drive 搜索响应解析失败: %w", err)
	}
	if parsed.Code != 0 {
		return nil, fmt.Errorf("drive 搜索失败: code=%d, msg=%s", parsed.Code, parsed.Msg)
	}
	return &DriveSearchResult{
		Total:     parsed.Data.Total,
		HasMore:   parsed.Data.HasMore,
		PageToken: parsed.Data.PageToken,
		Items:     parsed.Data.ResUnits,
	}, nil
}
