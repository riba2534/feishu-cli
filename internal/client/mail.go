package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// Mail API 基础路径
const mailBase = "/open-apis/mail/v1"

// mailboxPath 构造 mailbox 相关的 API path
// mailboxID 可以是 "me" 或具体 email 地址
func mailboxPath(mailboxID string, segments ...string) string {
	parts := make([]string, 0, 1+len(segments))
	parts = append(parts, url.PathEscape(mailboxID))
	for _, seg := range segments {
		if seg != "" {
			parts = append(parts, url.PathEscape(seg))
		}
	}
	return mailBase + "/user_mailboxes/" + strings.Join(parts, "/")
}

// callMailAPI 统一包装 mail API 调用
// method: GET/POST/PUT/DELETE
// path: API 完整路径（含 query string）
// body: 请求体（nil 表示无）
// 返回 data 字段原始 JSON
func callMailAPI(method, apiPath string, body any, userAccessToken string) (json.RawMessage, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)

	var resp *larkcore.ApiResp
	switch method {
	case http.MethodGet:
		resp, err = client.Get(Context(), apiPath, body, tokenType, opts...)
	case http.MethodPost:
		resp, err = client.Post(Context(), apiPath, body, tokenType, opts...)
	case http.MethodPut:
		resp, err = client.Put(Context(), apiPath, body, tokenType, opts...)
	case http.MethodDelete:
		resp, err = client.Delete(Context(), apiPath, body, tokenType, opts...)
	default:
		return nil, fmt.Errorf("不支持的 HTTP 方法: %s", method)
	}
	if err != nil {
		return nil, fmt.Errorf("mail API %s %s 失败: %w", method, apiPath, err)
	}

	// 先按飞书业务信封解析再看 HTTP 状态：邮箱的大量业务错误（如 4038 folder 非法、权限不足）随 HTTP 400 下发，
	// 先判状态码会丢掉业务码与 log_id，按 HasAPICode 分支的提示也走不到。
	if err := CheckAPIResponse("mail API "+method+" "+apiPathWithoutQuery(apiPath), resp); err != nil {
		return nil, err
	}

	var apiResp struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("mail API 解析响应失败: %w", err)
	}
	return apiResp.Data, nil
}

// apiPathWithoutQuery 去掉 query，避免错误信息里带上 page_token 等长参数。
func apiPathWithoutQuery(p string) string {
	if i := strings.IndexByte(p, '?'); i >= 0 {
		return p[:i]
	}
	return p
}

// MailboxProfile mailbox profile 信息
type MailboxProfile struct {
	PrimaryEmailAddress string `json:"primary_email_address"`
	UserMailboxID       string `json:"user_mailbox_id"`
	Name                string `json:"name"`
}

// GetMailboxProfile 获取 mailbox profile（用于解析当前用户邮箱地址）
// API: GET /open-apis/mail/v1/user_mailboxes/{mailbox_id}/profile
func GetMailboxProfile(mailboxID, userAccessToken string) (*MailboxProfile, error) {
	if mailboxID == "" {
		mailboxID = "me"
	}
	data, err := callMailAPI(http.MethodGet, mailboxPath(mailboxID, "profile"), nil, userAccessToken)
	if err != nil {
		return nil, err
	}
	var profile MailboxProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("解析 mailbox profile 失败: %w", err)
	}
	return &profile, nil
}

// ==================== 邮件查询 ====================

// GetMailMessage 获取单封邮件
// API: GET /open-apis/mail/v1/user_mailboxes/{mailbox_id}/messages/{message_id}
// format: "full"（含 HTML） / "plain_text_full"（纯文本） / "raw"（原始 EML）
func GetMailMessage(mailboxID, messageID, format, userAccessToken string) (json.RawMessage, error) {
	if mailboxID == "" {
		mailboxID = "me"
	}
	if format == "" {
		format = "full"
	}
	apiPath := mailboxPath(mailboxID, "messages", messageID) + "?format=" + url.QueryEscape(format)
	return callMailAPI(http.MethodGet, apiPath, nil, userAccessToken)
}

// BatchGetMailMessages 批量获取邮件（单批最多 20 条，自动分块，严格按请求顺序保序，输出包含 total 与 unavailable_message_ids）
// API: POST /open-apis/mail/v1/user_mailboxes/{mailbox_id}/messages/batch_get
func BatchGetMailMessages(mailboxID string, messageIDs []string, format, userAccessToken string) (json.RawMessage, error) {
	if mailboxID == "" {
		mailboxID = "me"
	}
	if format == "" {
		format = "full"
	}
	if len(messageIDs) == 0 {
		return json.Marshal(map[string]any{
			"messages": []any{},
			"total":    0,
		})
	}

	const batchSize = 20
	var allCollected []json.RawMessage
	for i := 0; i < len(messageIDs); i += batchSize {
		end := i + batchSize
		if end > len(messageIDs) {
			end = len(messageIDs)
		}
		chunk := messageIDs[i:end]
		body := map[string]any{
			"message_ids": chunk,
			"format":      format,
		}
		data, err := callMailAPI(http.MethodPost, mailboxPath(mailboxID, "messages", "batch_get"), body, userAccessToken)
		if err != nil {
			return nil, err
		}
		var resp struct {
			Messages []json.RawMessage `json:"messages"`
		}
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, fmt.Errorf("解析 batch_get 响应失败: %w", err)
		}
		allCollected = append(allCollected, resp.Messages...)
	}

	// 收集所有已获取的 message，以 message_id 为 key 建立映射
	type idHolder struct {
		MessageID string `json:"message_id"`
	}
	msgMap := make(map[string]json.RawMessage, len(allCollected))
	for _, raw := range allCollected {
		var holder idHolder
		if err := json.Unmarshal(raw, &holder); err == nil && holder.MessageID != "" {
			msgMap[holder.MessageID] = raw
		}
	}

	// 严格按照请求 messageIDs 的顺序构建结果，重复 ID 确定性保留，缺失 ID 收集到 unavailable_message_ids
	ordered := make([]json.RawMessage, 0, len(messageIDs))
	var unavailableIDs []string

	for _, id := range messageIDs {
		if raw, ok := msgMap[id]; ok {
			ordered = append(ordered, raw)
		} else {
			unavailableIDs = append(unavailableIDs, id)
		}
	}

	out := map[string]any{
		"messages": ordered,
		"total":    len(ordered),
	}
	if len(unavailableIDs) > 0 {
		out["unavailable_message_ids"] = unavailableIDs
	}

	return json.Marshal(out)
}

// GetMailThread 获取线程并按时间升序实际排序（保留 thread 及其内部全部未知字段，使用 UseNumber 保持大整数精度）
// API: GET /open-apis/mail/v1/user_mailboxes/{mailbox_id}/threads/{thread_id}
func GetMailThread(mailboxID, threadID, format, userAccessToken string) (json.RawMessage, error) {
	if mailboxID == "" {
		mailboxID = "me"
	}
	if format == "" {
		format = "full"
	}
	apiPath := mailboxPath(mailboxID, "threads", threadID) + "?format=" + url.QueryEscape(format)
	data, err := callMailAPI(http.MethodGet, apiPath, nil, userAccessToken)
	if err != nil {
		return nil, err
	}
	return sortThreadMessages(data)
}

func sortThreadMessages(data json.RawMessage) (json.RawMessage, error) {
	// 使用 json.NewDecoder + UseNumber 解析，以完整保留顶层和 thread 内部的所有未知字段与 >2^53 大整数精度
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var topMap map[string]any
	if err := dec.Decode(&topMap); err != nil {
		return data, nil
	}
	threadRaw, ok := topMap["thread"]
	if !ok {
		return data, nil
	}
	threadMap, ok := threadRaw.(map[string]any)
	if !ok {
		return data, nil
	}
	messagesRaw, ok := threadMap["messages"]
	if !ok {
		return data, nil
	}
	messagesList, ok := messagesRaw.([]any)
	if !ok || len(messagesList) <= 1 {
		return data, nil
	}

	type msgEntry struct {
		item any
		date int64
	}
	entries := make([]msgEntry, len(messagesList))
	for i, it := range messagesList {
		var dateVal int64
		if m, ok := it.(map[string]any); ok {
			if d, exists := m["internal_date"]; exists {
				switch v := d.(type) {
				case string:
					dateVal, _ = strconv.ParseInt(v, 10, 64)
				case json.Number:
					dateVal, _ = v.Int64()
				case float64:
					dateVal = int64(v)
				}
			}
		}
		entries[i] = msgEntry{item: it, date: dateVal}
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].date < entries[j].date
	})

	sortedList := make([]any, len(entries))
	for i, e := range entries {
		sortedList[i] = e.item
	}

	// 仅替换 thread 内的 messages，其它所有字段及顶层结构保持原样
	threadMap["messages"] = sortedList
	topMap["thread"] = threadMap

	return json.Marshal(topMap)
}

// 邮件分页大小约束（对齐官方 shortcuts/mail：list 端点上限 20、search 端点上限 15）。
// 两个端点都强制要求 page_size，缺失会返回 99992402 field validation failed。
const (
	mailListPageSizeDefault   = 20
	mailListPageSizeMax       = 20
	mailSearchPageSizeDefault = 15
	mailSearchPageSizeMax     = 15
)

// normalizeMailListPageSize 归一化 messages 列表端点的 page_size
func normalizeMailListPageSize(pageSize int) int {
	if pageSize <= 0 {
		return mailListPageSizeDefault
	}
	if pageSize > mailListPageSizeMax {
		return mailListPageSizeMax
	}
	return pageSize
}

// normalizeMailSearchPageSize 归一化 search 端点的 page_size
func normalizeMailSearchPageSize(pageSize int) int {
	if pageSize <= 0 {
		return mailSearchPageSizeDefault
	}
	if pageSize > mailSearchPageSizeMax {
		return mailSearchPageSizeMax
	}
	return pageSize
}

// ListMailMessagesParams 邮件列表参数
type ListMailMessagesParams struct {
	MailboxID  string
	FolderID   string // INBOX / SENT / SPAM / ARCHIVED / STRANGER 或自定义 folder_id
	LabelID    string // 标签 id
	UnreadOnly bool
	PageSize   int
	PageToken  string
	AfterTime  int64 // Unix 毫秒
	BeforeTime int64 // Unix 毫秒
}

// ListMailMessages 列出邮件（按 folder/label/未读过滤；无 label 默认 INBOX）
// API: GET /open-apis/mail/v1/user_mailboxes/{mailbox_id}/messages
// 关键词搜索请使用 SearchMailMessages（走专用 /search 端点）
func ListMailMessages(params ListMailMessagesParams, userAccessToken string) (json.RawMessage, error) {
	mailboxID := params.MailboxID
	if mailboxID == "" {
		mailboxID = "me"
	}
	folderID := params.FolderID
	if folderID == "" && params.LabelID == "" {
		folderID = "INBOX"
	}
	q := url.Values{}
	if folderID != "" {
		q.Set("folder_id", folderID)
	}
	if params.LabelID != "" {
		q.Set("label_id", params.LabelID)
	}
	if params.UnreadOnly {
		q.Set("only_unread", "true")
	}
	// page_size 是该端点的必填参数（缺失时服务端返回 99992402 field validation failed）。
	// 对齐官方：始终发送，未指定时取默认值，超过上限则截断。
	q.Set("page_size", fmt.Sprintf("%d", normalizeMailListPageSize(params.PageSize)))
	if params.PageToken != "" {
		q.Set("page_token", params.PageToken)
	}
	if params.AfterTime > 0 {
		q.Set("after_time", fmt.Sprintf("%d", params.AfterTime))
	}
	if params.BeforeTime > 0 {
		q.Set("before_time", fmt.Sprintf("%d", params.BeforeTime))
	}
	apiPath := mailboxPath(mailboxID, "messages")
	if encoded := q.Encode(); encoded != "" {
		apiPath += "?" + encoded
	}
	return callMailAPI(http.MethodGet, apiPath, nil, userAccessToken)
}

// ==================== 草稿管理 ====================

// CreateMailDraft 创建草稿（raw EML base64url 编码）
// API: POST /open-apis/mail/v1/user_mailboxes/{mailbox_id}/drafts
// body: {"raw": "base64url_encoded_eml"}
// 返回 draft_id
func CreateMailDraft(mailboxID, rawEMLBase64URL, userAccessToken string) (string, error) {
	if mailboxID == "" {
		mailboxID = "me"
	}
	data, err := callMailAPI(http.MethodPost, mailboxPath(mailboxID, "drafts"),
		map[string]any{"raw": rawEMLBase64URL}, userAccessToken)
	if err != nil {
		return "", err
	}
	return extractMailDraftID(data), nil
}

// UpdateMailDraft 更新草稿
// API: PUT /open-apis/mail/v1/user_mailboxes/{mailbox_id}/drafts/{draft_id}
func UpdateMailDraft(mailboxID, draftID, rawEMLBase64URL, userAccessToken string) error {
	if mailboxID == "" {
		mailboxID = "me"
	}
	_, err := callMailAPI(http.MethodPut, mailboxPath(mailboxID, "drafts", draftID),
		map[string]any{"raw": rawEMLBase64URL}, userAccessToken)
	return err
}

// SendMailDraft 发送草稿
// API: POST /open-apis/mail/v1/user_mailboxes/{mailbox_id}/drafts/{draft_id}/send
// 返回响应原始 data（含 message_id、thread_id 等）
func SendMailDraft(mailboxID, draftID, userAccessToken string) (json.RawMessage, error) {
	if mailboxID == "" {
		mailboxID = "me"
	}
	return callMailAPI(http.MethodPost, mailboxPath(mailboxID, "drafts", draftID, "send"), nil, userAccessToken)
}

// GetMailDraftRaw 获取草稿原始 EML（base64url 编码）
// API: GET /open-apis/mail/v1/user_mailboxes/{mailbox_id}/drafts/{draft_id}?format=raw
// 实测响应形如 {"draft":{"id":"...","message":{"message_id":"...","raw":"<base64url EML>"}}}；
// 兼容 data.raw / data.draft.raw 等旧形态。
func GetMailDraftRaw(mailboxID, draftID, userAccessToken string) (string, error) {
	if mailboxID == "" {
		mailboxID = "me"
	}
	apiPath := mailboxPath(mailboxID, "drafts", draftID) + "?format=raw"
	data, err := callMailAPI(http.MethodGet, apiPath, nil, userAccessToken)
	if err != nil {
		return "", err
	}
	var parsed struct {
		Draft struct {
			Raw     string `json:"raw"`
			Message struct {
				Raw string `json:"raw"`
			} `json:"message"`
		} `json:"draft"`
		Message struct {
			Raw string `json:"raw"`
		} `json:"message"`
		Raw string `json:"raw"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("解析草稿响应失败: %w", err)
	}
	for _, raw := range []string{parsed.Draft.Message.Raw, parsed.Draft.Raw, parsed.Message.Raw, parsed.Raw} {
		if raw != "" {
			return raw, nil
		}
	}
	return "", fmt.Errorf("草稿 %s 的响应中没有 raw 字段（format=raw）", draftID)
}

func extractMailDraftID(data json.RawMessage) string {
	var parsed struct {
		DraftID string `json:"draft_id"`
		ID      string `json:"id"`
		Draft   struct {
			DraftID string `json:"draft_id"`
			ID      string `json:"id"`
		} `json:"draft"`
	}
	_ = json.Unmarshal(data, &parsed)
	if parsed.DraftID != "" {
		return parsed.DraftID
	}
	if parsed.ID != "" {
		return parsed.ID
	}
	if parsed.Draft.DraftID != "" {
		return parsed.Draft.DraftID
	}
	return parsed.Draft.ID
}

// ==================== 文件夹和标签 ====================

type mailNamedItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

var systemLabelAliases = map[string]string{
	"important": "IMPORTANT",
	"priority":  "IMPORTANT",
	"重要邮件":      "IMPORTANT",
	"flagged":   "FLAGGED",
	"已加旗标":      "FLAGGED",
	"other":     "OTHER",
	"其他邮件":      "OTHER",
}

var systemLabelSearchName = map[string]string{
	"FLAGGED":   "flagged",
	"IMPORTANT": "priority",
	"OTHER":     "other",
}

var folderSystemIDToAlias = map[string]string{
	"INBOX":    "inbox",
	"SENT":     "sent",
	"DRAFT":    "draft",
	"TRASH":    "trash",
	"SPAM":     "spam",
	"ARCHIVED": "archive",
}

var folderSystemAliases = map[string]string{
	"inbox":    "INBOX",
	"收件箱":      "INBOX",
	"sent":     "SENT",
	"已发送":      "SENT",
	"draft":    "DRAFT",
	"drafts":   "DRAFT",
	"草稿箱":      "DRAFT",
	"trash":    "TRASH",
	"已删除":      "TRASH",
	"废纸篓":      "TRASH",
	"spam":     "SPAM",
	"垃圾邮件":     "SPAM",
	"archive":  "ARCHIVED",
	"archived": "ARCHIVED",
	"归档":       "ARCHIVED",
}

var searchOnlyFolderNames = map[string]bool{
	"scheduled": true,
}

// resolveSystemLabel 检查输入是否为系统标签别名（important/flagged/other）
func resolveSystemLabel(input string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(input))
	if id, ok := systemLabelAliases[lower]; ok {
		return id, true
	}
	switch strings.ToUpper(strings.TrimSpace(input)) {
	case "IMPORTANT", "FLAGGED", "OTHER":
		return strings.ToUpper(strings.TrimSpace(input)), true
	}
	return "", false
}

// resolveFolderSystemAliasOrID 检查输入是否为系统文件夹别名（INBOX/SENT/DRAFT/TRASH/SPAM/ARCHIVED）
func resolveFolderSystemAliasOrID(input string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(input))
	if id, ok := folderSystemAliases[lower]; ok {
		return id, true
	}
	upper := strings.ToUpper(strings.TrimSpace(input))
	if _, ok := folderSystemIDToAlias[upper]; ok {
		return upper, true
	}
	return "", false
}

// parseFilterInt 从 filter 值中解析整数（兼容 int / json.Number / float64 / 字符串）
func parseFilterInt(v any) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case int64:
		return int(val), true
	case float64:
		return int(val), true
	case json.Number:
		if n, err := val.Int64(); err == nil {
			return int(n), true
		}
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
			return n, true
		}
	}
	return 0, false
}

func parseFilterStrings(v any) []string {
	var out []string
	switch val := v.(type) {
	case string:
		if s := strings.TrimSpace(val); s != "" {
			out = append(out, s)
		}
	case []string:
		for _, s := range val {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				out = append(out, trimmed)
			}
		}
	case []any:
		for _, item := range val {
			if s := strings.TrimSpace(fmt.Sprintf("%v", item)); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

func resolveCustomFolderName(mailboxID, input string, userAccessToken string, cachedFolders *[]mailNamedItem) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", nil
	}
	// 检查 search-only folder (如 scheduled)
	if searchOnlyFolderNames[strings.ToLower(input)] {
		return strings.ToLower(input), nil
	}
	// 检查系统 folder 别名
	if sysID, ok := resolveFolderSystemAliasOrID(input); ok {
		return folderSystemIDToAlias[sysID], nil
	}

	// 自定义文件夹：延迟拉取一次 folders 列表
	if cachedFolders != nil && len(*cachedFolders) == 0 {
		raw, err := ListMailFolders(mailboxID, userAccessToken)
		if err != nil {
			return "", fmt.Errorf("查询文件夹列表失败: %w", err)
		}
		var resp struct {
			Items   []mailNamedItem `json:"items"`
			Folders []mailNamedItem `json:"folders"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return "", fmt.Errorf("解析文件夹列表失败: %w", err)
		}
		all := append(resp.Items, resp.Folders...)
		*cachedFolders = all
	}

	items := []mailNamedItem{}
	if cachedFolders != nil {
		items = *cachedFolders
	}

	// exact ID 优先
	for _, item := range items {
		if item.ID == input && item.Name != "" {
			return item.Name, nil
		}
	}

	// exact Name 次之（防同名歧义）
	var matchedNames []string
	for _, item := range items {
		if item.Name == input {
			matchedNames = append(matchedNames, item.Name)
		}
	}
	if len(matchedNames) == 1 {
		return matchedNames[0], nil
	}
	if len(matchedNames) > 1 {
		return "", fmt.Errorf("文件夹名称 %q 存在多个匹配项（名称歧义），请使用明确的文件夹 ID", input)
	}

	return "", fmt.Errorf("未找到文件夹 %q（既非系统文件夹，也非有效自定义文件夹 ID 或名称）", input)
}

func resolveCustomLabelName(mailboxID, input string, userAccessToken string, cachedLabels *[]mailNamedItem) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", nil
	}

	// 注意：禁止调用 checkSystemFolder / resolveFolderSystemAliasOrID！
	// --label inbox 或 --label scheduled 绝不是系统 label，必须按自定义 label 处理

	// 自定义 label：延迟拉取一次 labels 列表
	if cachedLabels != nil && len(*cachedLabels) == 0 {
		raw, err := ListMailLabels(mailboxID, userAccessToken)
		if err != nil {
			return "", fmt.Errorf("查询标签列表失败: %w", err)
		}
		var resp struct {
			Items  []mailNamedItem `json:"items"`
			Labels []mailNamedItem `json:"labels"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return "", fmt.Errorf("解析标签列表失败: %w", err)
		}
		all := append(resp.Items, resp.Labels...)
		*cachedLabels = all
	}

	items := []mailNamedItem{}
	if cachedLabels != nil {
		items = *cachedLabels
	}

	// exact ID 优先
	for _, item := range items {
		if item.ID == input && item.Name != "" {
			return item.Name, nil
		}
	}

	// exact Name 次之
	var matchedNames []string
	for _, item := range items {
		if item.Name == input {
			matchedNames = append(matchedNames, item.Name)
		}
	}
	if len(matchedNames) == 1 {
		return matchedNames[0], nil
	}
	if len(matchedNames) > 1 {
		return "", fmt.Errorf("标签名称 %q 存在多个匹配项（名称歧义），请使用明确的标签 ID", input)
	}

	return "", fmt.Errorf("未找到标签 %q（既非系统标签，也非有效自定义标签 ID 或名称）", input)
}

// SearchMailMessages 通过专用 search 端点搜索邮件
// API: POST /open-apis/mail/v1/user_mailboxes/{mailbox_id}/search?page_size=xx&page_token=yy
// body: {"query": "关键词", "filter": {"folder": ["inbox"], "label": ["xxx"], "is_unread": true}}
// 用于 mail triage --query 的真实搜索（对齐官方 resolveSearchFilter：系统标签覆盖 folder、清除 label、零列表请求；自定义值精确查表）
func SearchMailMessages(mailboxID, query string, filter map[string]any, userAccessToken string) (json.RawMessage, error) {
	if mailboxID == "" {
		mailboxID = "me"
	}
	q := url.Values{}
	normalizedFilter := make(map[string]any)

	var rawFolderInput []string
	var rawLabelInput []string
	searchPageSize := 0

	for k, v := range filter {
		switch k {
		case "page_size":
			if n, ok := parseFilterInt(v); ok {
				searchPageSize = n
			}
		case "page_token":
			if s := fmt.Sprintf("%v", v); s != "" {
				q.Set("page_token", s)
			}
		case "folder", "folder_id":
			rawFolderInput = append(rawFolderInput, parseFilterStrings(v)...)
		case "label", "label_id":
			rawLabelInput = append(rawLabelInput, parseFilterStrings(v)...)
		case "only_unread", "is_unread":
			if b, ok := v.(bool); ok {
				normalizedFilter["is_unread"] = b
			} else if s := fmt.Sprintf("%v", v); s == "true" {
				normalizedFilter["is_unread"] = true
			}
		default:
			normalizedFilter[k] = v
		}
	}

	// page_size 是 search 端点的必填参数（缺失时服务端返回 99992402 field validation failed）。
	// 对齐官方：始终发送，未指定时取默认值，超过上限则截断。
	q.Set("page_size", fmt.Sprintf("%d", normalizeMailSearchPageSize(searchPageSize)))

	// Step 1: Check if folder or label contains a system label (IMPORTANT/FLAGGED/OTHER).
	// 官方语义：系统标签（important/flagged/other）在 search API 中作为 folder 发送；
	// 一旦检测到系统标签，立即清空 label 字段，设置 folder 为该系统标签名（覆盖原 folder），并立即返回！零列表网络请求。
	var systemLabelFolder string
	for _, f := range rawFolderInput {
		if id, ok := resolveSystemLabel(f); ok {
			systemLabelFolder = systemLabelSearchName[id]
			break
		}
	}
	if systemLabelFolder == "" {
		for _, l := range rawLabelInput {
			if id, ok := resolveSystemLabel(l); ok {
				systemLabelFolder = systemLabelSearchName[id]
				break
			}
		}
	}

	if systemLabelFolder != "" {
		normalizedFilter["folder"] = []string{systemLabelFolder}
		delete(normalizedFilter, "label")

		body := map[string]any{"query": query}
		if len(normalizedFilter) > 0 {
			body["filter"] = normalizedFilter
		}
		apiPath := mailboxPath(mailboxID, "search")
		if encoded := q.Encode(); encoded != "" {
			apiPath += "?" + encoded
		}
		return callMailAPI(http.MethodPost, apiPath, body, userAccessToken)
	}

	// Step 2: 普通文件夹解析（System or Custom Folders）
	var cachedFolders []mailNamedItem
	var resolvedFolders []string
	for _, item := range rawFolderInput {
		resolved, err := resolveCustomFolderName(mailboxID, item, userAccessToken, &cachedFolders)
		if err != nil {
			return nil, err
		}
		if resolved != "" {
			resolvedFolders = append(resolvedFolders, resolved)
		}
	}
	if len(resolvedFolders) > 0 {
		normalizedFilter["folder"] = resolvedFolders
	}

	// Step 3: 普通标签解析（Custom Labels Only，禁止将 inbox/sent/scheduled 当成系统标签）
	var cachedLabels []mailNamedItem
	var resolvedLabels []string
	for _, item := range rawLabelInput {
		resolved, err := resolveCustomLabelName(mailboxID, item, userAccessToken, &cachedLabels)
		if err != nil {
			return nil, err
		}
		if resolved != "" {
			resolvedLabels = append(resolvedLabels, resolved)
		}
	}
	if len(resolvedLabels) > 0 {
		normalizedFilter["label"] = resolvedLabels
	}

	body := map[string]any{"query": query}
	if len(normalizedFilter) > 0 {
		body["filter"] = normalizedFilter
	}
	apiPath := mailboxPath(mailboxID, "search")
	if encoded := q.Encode(); encoded != "" {
		apiPath += "?" + encoded
	}
	return callMailAPI(http.MethodPost, apiPath, body, userAccessToken)
}

// ListMailSignatures 列出邮箱签名
// API: GET /open-apis/mail/v1/user_mailboxes/{mailbox_id}/settings/signatures
// 权限: User Access Token + mail:user_mailbox:readonly
//
//	（飞书无 mail:user_mailbox.settings:read 这个 scope，settings 路径下端点复用 mailbox 读权限）
//
// 返回 data 字段原始 JSON（含 signatures 列表 + usages 使用信息）
func ListMailSignatures(mailboxID, userAccessToken string) (json.RawMessage, error) {
	if mailboxID == "" {
		mailboxID = "me"
	}
	return callMailAPI(http.MethodGet, mailboxPath(mailboxID, "settings", "signatures"), nil, userAccessToken)
}

// ListMailFolders 列出邮箱文件夹
// API: GET /open-apis/mail/v1/user_mailboxes/{mailbox_id}/folders
func ListMailFolders(mailboxID, userAccessToken string) (json.RawMessage, error) {
	if mailboxID == "" {
		mailboxID = "me"
	}
	return callMailAPI(http.MethodGet, mailboxPath(mailboxID, "folders"), nil, userAccessToken)
}

// ListMailLabels 列出邮箱标签
// API: GET /open-apis/mail/v1/user_mailboxes/{mailbox_id}/labels
func ListMailLabels(mailboxID, userAccessToken string) (json.RawMessage, error) {
	if mailboxID == "" {
		mailboxID = "me"
	}
	return callMailAPI(http.MethodGet, mailboxPath(mailboxID, "labels"), nil, userAccessToken)
}

// ResolveMailFolderID 把 --folder 输入解析为 messages 列表端点可用的 folder_id。
// 规则（对齐官方 resolveFolderName）：系统文件夹别名/ID（inbox/收件箱/INBOX…）本地解析；
// 其余按"精确 ID 优先、名称（大小写不敏感）其次"在文件夹列表中查找，名称重复或不存在时报错。
// 修复：列表端点只接受 folder_id，此前直接透传 "inbox" 被服务端以 4038 拒绝。
func ResolveMailFolderID(mailboxID, input, userAccessToken string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", nil
	}
	if id, ok := resolveFolderSystemAliasOrID(value); ok {
		return id, nil
	}
	raw, err := ListMailFolders(mailboxID, userAccessToken)
	if err != nil {
		return "", fmt.Errorf("查询文件夹列表失败: %w", err)
	}
	return resolveMailNamedID("文件夹", "--list-folders", value, raw, "folders")
}

// ResolveMailLabelID 把 --label 输入解析为 messages 列表端点可用的 label_id。
// 系统标签（important/flagged/other 及其大写 ID）本地解析；其余在标签列表中按 ID / 名称查找。
func ResolveMailLabelID(mailboxID, input, userAccessToken string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", nil
	}
	if id, ok := resolveSystemLabel(value); ok {
		return id, nil
	}
	if strings.EqualFold(value, "UNREAD") {
		return "UNREAD", nil
	}
	raw, err := ListMailLabels(mailboxID, userAccessToken)
	if err != nil {
		return "", fmt.Errorf("查询标签列表失败: %w", err)
	}
	return resolveMailNamedID("标签", "--list-labels", value, raw, "labels")
}

func resolveMailNamedID(kind, listFlag, value string, raw json.RawMessage, altKey string) (string, error) {
	var resp map[string][]mailNamedItem
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("解析%s列表失败: %w", kind, err)
	}
	items := append(resp["items"], resp[altKey]...)
	for _, it := range items {
		if it.ID != "" && it.ID == value {
			return it.ID, nil
		}
	}
	var matches []string
	for _, it := range items {
		if it.ID != "" && strings.EqualFold(strings.TrimSpace(it.Name), value) {
			matches = append(matches, it.ID)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("未找到%s %q；可先用 `feishu-cli mail triage %s` 查看可用的 ID 与名称", kind, value, listFlag)
	default:
		return "", fmt.Errorf("%s名称 %q 对应多个 ID（%s），请改用 ID", kind, value, strings.Join(matches, ","))
	}
}
