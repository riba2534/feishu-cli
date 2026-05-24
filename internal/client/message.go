package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/riba2534/feishu-cli/internal/config"
)

// 卡片消息内容返回格式枚举值（OAPI 参数 card_msg_content_type 的取值）
//
// 当查询消息（list/get/mget）命中 interactive 卡片时，OAPI 默认返回的是渲染后的
// 文本（形如 `<card title="...">...</card>`）。通过传 card_msg_content_type 参数
// 可拿到原始 JSON：
//   - user_card_content：返回 userDSL（开发者构建卡片时的 schema 2.0 JSON，便于偷师/调试）
//   - raw_card_content： 返回 cardDSL（平台内部完整描述，含默认补全字段）
const (
	CardMsgContentTypeUser = "user_card_content"
	CardMsgContentTypeRaw  = "raw_card_content"
)

// SendMessage sends a message to a user or chat
func SendMessage(receiveIDType string, receiveID string, msgType string, content string, userAccessToken string) (string, error) {
	client, err := GetClient()
	if err != nil {
		return "", err
	}

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(receiveIDType).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(receiveID).
			MsgType(msgType).
			Content(content).
			Build()).
		Build()

	resp, err := client.Im.Message.Create(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return "", fmt.Errorf("发送消息失败: %w", err)
	}

	if !resp.Success() {
		return "", fmt.Errorf("发送消息失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data.MessageId == nil {
		return "", fmt.Errorf("消息已发送但未返回消息 ID")
	}

	return *resp.Data.MessageId, nil
}

// ReplyMessage replies to a message.
// 当 replyInThread 为 true 时，以话题（thread）形式回复；
// 若目标群聊本身就是话题模式，该参数会自动回复到消息所在话题。
func ReplyMessage(messageID string, msgType string, content string, replyInThread bool, userAccessToken string) (string, error) {
	client, err := GetClient()
	if err != nil {
		return "", err
	}

	bodyBuilder := larkim.NewReplyMessageReqBodyBuilder().
		MsgType(msgType).
		Content(content)
	if replyInThread {
		bodyBuilder.ReplyInThread(true)
	}

	req := larkim.NewReplyMessageReqBuilder().
		MessageId(messageID).
		Body(bodyBuilder.Build()).
		Build()

	resp, err := client.Im.Message.Reply(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return "", fmt.Errorf("回复消息失败: %w", err)
	}

	if !resp.Success() {
		return "", fmt.Errorf("回复消息失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data.MessageId == nil {
		return "", fmt.Errorf("回复已发送但未返回消息 ID")
	}

	return *resp.Data.MessageId, nil
}

// UpdateMessage updates a message content
func UpdateMessage(messageID string, content string, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	req := larkim.NewPatchMessageReqBuilder().
		MessageId(messageID).
		Body(larkim.NewPatchMessageReqBodyBuilder().
			Content(content).
			Build()).
		Build()

	resp, err := client.Im.Message.Patch(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return fmt.Errorf("更新消息失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("更新消息失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// CreateTextMessageContent creates content for a text message.
// json.Marshal 对 map[string]string 不会失败，因此忽略错误。
func CreateTextMessageContent(text string) string {
	content := map[string]string{"text": text}
	data, _ := json.Marshal(content)
	return string(data)
}

// CreateRichTextMessageContent creates content for a rich text (post) message.
func CreateRichTextMessageContent(title string, content [][]map[string]any) string {
	post := map[string]any{
		"zh_cn": map[string]any{
			"title":   title,
			"content": content,
		},
	}
	data, _ := json.Marshal(post)
	return string(data)
}

// CreateInteractiveCardContent creates content for an interactive card message.
func CreateInteractiveCardContent(card map[string]any) string {
	data, _ := json.Marshal(card)
	return string(data)
}

// DeleteMessage deletes a message by message ID
func DeleteMessage(messageID string, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	req := larkim.NewDeleteMessageReqBuilder().
		MessageId(messageID).
		Build()

	resp, err := client.Im.Message.Delete(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return fmt.Errorf("删除消息失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("删除消息失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// ListMessagesOptions contains options for listing messages
type ListMessagesOptions struct {
	ContainerIDType string
	StartTime       string
	EndTime         string
	SortType        string
	PageSize        int
	PageToken       string
	// CardContentType 控制 interactive 卡片的返回格式（取值 user_card_content / raw_card_content / 空）。
	// 详见 CardMsgContentTypeUser/CardMsgContentTypeRaw 注释。
	CardContentType string
}

// ListMessagesResult contains the result of listing messages
type ListMessagesResult struct {
	Items     []*larkim.Message
	PageToken string
	HasMore   bool
	// MergeForwardSubMessages 当 Items 中存在 msg_type=merge_forward 容器时，
	// 此 map 以容器 message_id 为 key，value 是该容器递归展开后的平铺子消息列表
	// （每条子消息带 upper_message_id 可重建嵌套树）。无展开时为 nil。
	MergeForwardSubMessages map[string][]*larkim.Message
	// ThreadReplies 当 ExpandThreadReplies 被调用后，以 thread_id 为 key，value
	// 是该话题在 ascending 顺序下的回复列表（不含根消息本身）。无展开时为 nil。
	// 配合 ThreadHasMore 表示是否有更多回复未拉完。
	ThreadReplies map[string][]*larkim.Message
	// ThreadHasMore key 为 thread_id，value 为 true 表示该话题在 perThread 限额
	// 内未拉完，还有更多回复存在。可结合 msg thread-messages 命令翻页继续拉。
	ThreadHasMore map[string]bool
}

// ListMessages lists messages in a container (chat).
// Note: The current Feishu Go SDK typed builder declares the List Messages API as
// tenant_access_token only, but the API actually supports user_access_token as well.
// When a user access token is provided, we use a raw HTTP request to bypass the SDK's
// client-side token type validation. See: https://open.feishu.cn/document/server-docs/im-v1/message/list
//
// 当 opts.CardContentType 非空时，SDK builder 不识别该字段，统一走 raw HTTP / SDK raw request
// 路径，把 card_msg_content_type 加到 query params。
func ListMessages(containerID string, opts ListMessagesOptions, userAccessToken string) (result *ListMessagesResult, err error) {
	// merge_forward 自动并发展开：在所有 fetch 路径成功返回后统一处理。
	// 命名返回值 + defer 让三个分支共享展开逻辑且改动最小。
	defer func() {
		if err != nil || result == nil {
			return
		}
		if subMap := expandMergeForwardForContainers(result.Items, userAccessToken); len(subMap) > 0 {
			result.MergeForwardSubMessages = subMap
		}
	}()

	// When user access token is provided, use raw HTTP to bypass SDK token type restriction
	if userAccessToken != "" {
		return listMessagesWithUserToken(containerID, opts, userAccessToken)
	}

	// 当 SDK builder 不支持的参数（card_msg_content_type）需要传时，走 SDK raw request。
	if opts.CardContentType != "" {
		return listMessagesViaRawRequest(containerID, opts, "")
	}

	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	reqBuilder := larkim.NewListMessageReqBuilder().
		ContainerIdType(opts.ContainerIDType).
		ContainerId(containerID)

	if opts.StartTime != "" {
		reqBuilder.StartTime(opts.StartTime)
	}
	if opts.EndTime != "" {
		reqBuilder.EndTime(opts.EndTime)
	}
	if opts.SortType != "" {
		reqBuilder.SortType(opts.SortType)
	}
	if opts.PageSize > 0 {
		reqBuilder.PageSize(opts.PageSize)
	}
	if opts.PageToken != "" {
		reqBuilder.PageToken(opts.PageToken)
	}

	resp, err := client.Im.Message.List(Context(), reqBuilder.Build())
	if err != nil {
		return nil, fmt.Errorf("获取消息列表失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取消息列表失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return &ListMessagesResult{
		Items:     resp.Data.Items,
		PageToken: StringVal(resp.Data.PageToken),
		HasMore:   BoolVal(resp.Data.HasMore),
	}, nil
}

// listMessagesViaRawRequest calls /im/v1/messages via SDK raw request so that
// query params (e.g. card_msg_content_type) unsupported by the typed SDK builder
// can still be sent. tenant_access_token is auto-managed by the SDK; pass an
// empty userAccessToken for tenant mode.
func listMessagesViaRawRequest(containerID string, opts ListMessagesOptions, userAccessToken string) (*ListMessagesResult, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := &larkcore.ApiReq{
		HttpMethod:  http.MethodGet,
		ApiPath:     "/open-apis/im/v1/messages",
		QueryParams: larkcore.QueryParams{},
		SupportedAccessTokenTypes: []larkcore.AccessTokenType{
			larkcore.AccessTokenTypeTenant,
			larkcore.AccessTokenTypeUser,
		},
	}
	if opts.ContainerIDType != "" {
		req.QueryParams.Set("container_id_type", opts.ContainerIDType)
	}
	req.QueryParams.Set("container_id", containerID)
	if opts.StartTime != "" {
		req.QueryParams.Set("start_time", opts.StartTime)
	}
	if opts.EndTime != "" {
		req.QueryParams.Set("end_time", opts.EndTime)
	}
	if opts.SortType != "" {
		req.QueryParams.Set("sort_type", opts.SortType)
	}
	if opts.PageSize > 0 {
		req.QueryParams.Set("page_size", strconv.Itoa(opts.PageSize))
	}
	if opts.PageToken != "" {
		req.QueryParams.Set("page_token", opts.PageToken)
	}
	if opts.CardContentType != "" {
		req.QueryParams.Set("card_msg_content_type", opts.CardContentType)
	}

	apiResp, err := cli.Do(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("获取消息列表失败: %w", err)
	}
	if apiResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取消息列表失败: HTTP %d, body: %s", apiResp.StatusCode, string(apiResp.RawBody))
	}

	var resp listMessagesRawResponse
	if err := json.Unmarshal(apiResp.RawBody, &resp); err != nil {
		return nil, fmt.Errorf("获取消息列表失败: 解析响应失败: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("获取消息列表失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return &ListMessagesResult{
		Items:     resp.Data.Items,
		PageToken: StringVal(resp.Data.PageToken),
		HasMore:   BoolVal(resp.Data.HasMore),
	}, nil
}

// listMessagesRawResponse represents the raw API response for list messages
type listMessagesRawResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Items     []*larkim.Message `json:"items"`
		HasMore   *bool             `json:"has_more"`
		PageToken *string           `json:"page_token"`
	} `json:"data"`
}

// listMessagesWithUserToken calls the List Messages API directly via HTTP,
// bypassing the SDK's token type validation that incorrectly rejects user_access_token.
func listMessagesWithUserToken(containerID string, opts ListMessagesOptions, userAccessToken string) (*ListMessagesResult, error) {
	cfg := config.Get()
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://open.feishu.cn"
	}

	params := url.Values{}
	params.Set("container_id_type", opts.ContainerIDType)
	params.Set("container_id", containerID)
	if opts.StartTime != "" {
		params.Set("start_time", opts.StartTime)
	}
	if opts.EndTime != "" {
		params.Set("end_time", opts.EndTime)
	}
	if opts.SortType != "" {
		params.Set("sort_type", opts.SortType)
	}
	if opts.PageSize > 0 {
		params.Set("page_size", strconv.Itoa(opts.PageSize))
	}
	if opts.PageToken != "" {
		params.Set("page_token", opts.PageToken)
	}
	if opts.CardContentType != "" {
		params.Set("card_msg_content_type", opts.CardContentType)
	}

	reqURL := fmt.Sprintf("%s/open-apis/im/v1/messages?%s", baseURL, params.Encode())
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("获取消息列表失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+userAccessToken)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("获取消息列表失败: %w", err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("获取消息列表失败: 读取响应失败: %w", err)
	}

	var resp listMessagesRawResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("获取消息列表失败: 解析响应失败: %w", err)
	}

	if resp.Code != 0 {
		return nil, fmt.Errorf("获取消息列表失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return &ListMessagesResult{
		Items:     resp.Data.Items,
		PageToken: StringVal(resp.Data.PageToken),
		HasMore:   BoolVal(resp.Data.HasMore),
	}, nil
}

// LoadAllChatMembers 拉取群完整成员列表（自动翻页，最多 50 页 = 5000 人）。
// 失败（外部群 232033 / Bot 不在群 / 网络异常等）时返回 (nil, err)，调用方一般静默降级。
//
// **重要**：此函数对内部群非常有用（成员 ID 可用于 mention、@、统计等），但**外部群下
// 拿到的 member_id 跟 message API 拿到的 sender_id 是不同的 ID namespace**，不能用
// member.member_id 去 lookup msg.sender.id。这是飞书的设计：外部群内 Bot 看到的
// 成员列表是"群通讯录视图"，而消息接口看到的 sender 是"消息系统视图"，两套 ID 独立。
// 因此本 helper 只负责取数据，由调用方决定怎么使用（建议作为独立字段返回，不要混入
// sender_names 字典）。
func LoadAllChatMembers(chatID, userAccessToken string) ([]*ChatMemberInfo, error) {
	if chatID == "" || !strings.HasPrefix(chatID, "oc_") {
		return nil, nil
	}

	all := make([]*ChatMemberInfo, 0)
	pageToken := ""
	// 防御性：最多翻 50 页（5000 人），避免异常响应造成死循环
	for i := 0; i < 50; i++ {
		res, err := ListChatMembers(chatID, "open_id", 100, pageToken, userAccessToken)
		if err != nil {
			if i == 0 {
				return nil, err
			}
			break // 后续页失败时返回已拿到的部分
		}
		all = append(all, res.Items...)
		if !res.HasMore || res.PageToken == "" {
			break
		}
		pageToken = res.PageToken
	}
	return all, nil
}

// ResolveSenderNames 为消息列表补齐每个 user 发送者的显示名字，返回 open_id → name 的映射。
// 两步解析（对齐官方 lark-cli 的 ResolveSenderNames）：
//  1. 从每条消息的 mentions 里抽已有的 {id, name}（免费，无需 API 调用）
//  2. 剩余未解的 user 发送者 open_id 走 POST /open-apis/contact/v3/users/basic_batch 批量补齐。
//
// 该函数对网络错误是容错的：任一步失败只返回当前累积的映射，调用方仍可得到部分结果。
// App sender（id_type=app_id / sender_type=app）跳过，不会发起查询。
//
// 群聊场景额外想拿"群完整成员名单（含群昵称）"用 LoadAllChatMembers 单独取，结果应独立于
// sender_names 字段使用——外部群下 member_id 和 sender_id 是不同 namespace，不能互查。
func ResolveSenderNames(messages []*larkim.Message, userAccessToken string) map[string]string {
	nameMap := make(map[string]string)

	// Step 1: 从 mentions 抽映射
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		for _, mention := range msg.Mentions {
			if mention == nil {
				continue
			}
			id := StringVal(mention.Id)
			name := StringVal(mention.Name)
			if id != "" && name != "" && strings.HasPrefix(id, "ou_") {
				nameMap[id] = name
			}
		}
	}

	// Step 2: 收集剩余未解的 user sender open_id
	seen := make(map[string]bool)
	var missing []string
	for _, msg := range messages {
		if msg == nil || msg.Sender == nil {
			continue
		}
		senderType := StringVal(msg.Sender.SenderType)
		if senderType != "" && senderType != "user" {
			continue
		}
		id := StringVal(msg.Sender.Id)
		if id == "" || !strings.HasPrefix(id, "ou_") || seen[id] || nameMap[id] != "" {
			continue
		}
		seen[id] = true
		missing = append(missing, id)
	}
	if len(missing) == 0 || userAccessToken == "" {
		return nameMap
	}

	resolved, err := BatchGetUsersBasic(missing, userAccessToken)
	if err != nil {
		// 容错：拿到多少算多少
		for k, v := range resolved {
			nameMap[k] = v
		}
		return nameMap
	}
	for k, v := range resolved {
		nameMap[k] = v
	}
	return nameMap
}

// ResolveP2PChatID 通过对方的 open_id 反查 P2P 私聊的 chat_id（oc_xxx）。
// 拿到 chat_id 后即可像读群聊一样使用 `container_id_type=chat` 读取私聊消息。
// 底层调用 POST /open-apis/im/v1/chat_p2p/batch_query，必须 User Token；
// SDK 未封装此端点，所以走 raw HTTP。
func ResolveP2PChatID(openID, userAccessToken string) (string, error) {
	if userAccessToken == "" {
		return "", fmt.Errorf("反查 P2P chat_id 需要 User Access Token")
	}
	if openID == "" {
		return "", fmt.Errorf("反查 P2P chat_id 必须提供 open_id")
	}

	cfg := config.Get()
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://open.feishu.cn"
	}

	reqURL := fmt.Sprintf("%s/open-apis/im/v1/chat_p2p/batch_query?chatter_id_type=open_id", baseURL)
	bodyBytes, err := json.Marshal(map[string]any{"chatter_ids": []string{openID}})
	if err != nil {
		return "", fmt.Errorf("反查 P2P chat_id 失败: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("反查 P2P chat_id 失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+userAccessToken)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("反查 P2P chat_id 失败: %w", err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return "", fmt.Errorf("反查 P2P chat_id 失败: 读取响应失败: %w", err)
	}

	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			P2PChats []struct {
				ChatID string `json:"chat_id"`
			} `json:"p2p_chats"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("反查 P2P chat_id 失败: 解析响应失败: %w", err)
	}

	if resp.Code != 0 {
		return "", fmt.Errorf("反查 P2P chat_id 失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	for _, c := range resp.Data.P2PChats {
		if c.ChatID != "" {
			return c.ChatID, nil
		}
	}
	return "", fmt.Errorf("尚未和该用户有过私聊（open_id=%s）", openID)
}

// GetMessageResult contains the result of getting a message
type GetMessageResult struct {
	Message *larkim.Message
	// SubMessages 仅当 Message.MsgType == "merge_forward" 且自动展开成功时填充。
	// 平铺所有递归展开的子消息，每条带 upper_message_id 可重建父子树。
	SubMessages []*larkim.Message
}

// GetMessage gets a message by message ID.
//
// Note: The SDK incorrectly declares this API as tenant_access_token only,
// but it actually supports user_access_token. When a user token is provided,
// we use raw HTTP to bypass the SDK's client-side token type validation.
//
// 当 cardContentType 非空时（user_card_content / raw_card_content），SDK builder 不识别该
// 字段，统一走 raw 路径把 card_msg_content_type 加到 query params。
func GetMessage(messageID, userAccessToken, cardContentType string) (result *GetMessageResult, err error) {
	// merge_forward 自动展开：在所有 fetch 路径成功返回后统一处理。
	// 命名返回值 + defer 让三个分支共享展开逻辑且改动最小。
	defer func() {
		if err == nil {
			applyMergeForwardExpansion(result, messageID, userAccessToken)
		}
	}()

	if userAccessToken != "" {
		return getMessageWithUserToken(messageID, userAccessToken, cardContentType)
	}

	if cardContentType != "" {
		return getMessageViaRawRequest(messageID, cardContentType, "")
	}

	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larkim.NewGetMessageReqBuilder().
		MessageId(messageID).
		Build()

	resp, err := client.Im.Message.Get(Context(), req)
	if err != nil {
		return nil, fmt.Errorf("获取消息详情失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取消息详情失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if len(resp.Data.Items) == 0 {
		return nil, fmt.Errorf("消息不存在")
	}

	return &GetMessageResult{
		Message: resp.Data.Items[0],
	}, nil
}

// getMessageWithUserToken calls the Get Message API via raw HTTP,
// bypassing the SDK's token type validation.
//
// cardContentType 透传到 query params；空字符串则不传，保持向后兼容。
func getMessageWithUserToken(messageID, userAccessToken, cardContentType string) (*GetMessageResult, error) {
	cfg := config.Get()
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://open.feishu.cn"
	}

	reqURL := fmt.Sprintf("%s/open-apis/im/v1/messages/%s", baseURL, messageID)
	if cardContentType != "" {
		params := url.Values{}
		params.Set("card_msg_content_type", cardContentType)
		reqURL = reqURL + "?" + params.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("获取消息详情失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+userAccessToken)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("获取消息详情失败: %w", err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("获取消息详情失败: 读取响应失败: %w", err)
	}

	var resp listMessagesRawResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("获取消息详情失败: 解析响应失败: %w", err)
	}

	if resp.Code != 0 {
		return nil, fmt.Errorf("获取消息详情失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if len(resp.Data.Items) == 0 {
		return nil, fmt.Errorf("消息不存在")
	}

	return &GetMessageResult{
		Message: resp.Data.Items[0],
	}, nil
}

// getMessageViaRawRequest calls /im/v1/messages/:message_id via SDK raw request.
// 用于 tenant 模式 + 需要传 SDK builder 不识别的字段（card_msg_content_type）的场景。
func getMessageViaRawRequest(messageID, cardContentType, userAccessToken string) (*GetMessageResult, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := &larkcore.ApiReq{
		HttpMethod:  http.MethodGet,
		ApiPath:     "/open-apis/im/v1/messages/:message_id",
		PathParams:  larkcore.PathParams{},
		QueryParams: larkcore.QueryParams{},
		SupportedAccessTokenTypes: []larkcore.AccessTokenType{
			larkcore.AccessTokenTypeTenant,
			larkcore.AccessTokenTypeUser,
		},
	}
	req.PathParams.Set("message_id", messageID)
	if cardContentType != "" {
		req.QueryParams.Set("card_msg_content_type", cardContentType)
	}

	apiResp, err := cli.Do(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("获取消息详情失败: %w", err)
	}
	if apiResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取消息详情失败: HTTP %d, body: %s", apiResp.StatusCode, string(apiResp.RawBody))
	}

	var resp listMessagesRawResponse
	if err := json.Unmarshal(apiResp.RawBody, &resp); err != nil {
		return nil, fmt.Errorf("获取消息详情失败: 解析响应失败: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("获取消息详情失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}
	if len(resp.Data.Items) == 0 {
		return nil, fmt.Errorf("消息不存在")
	}
	return &GetMessageResult{
		Message: resp.Data.Items[0],
	}, nil
}

// ForwardMessage forwards a message to another recipient
func ForwardMessage(messageID string, receiveID string, receiveIDType string, userAccessToken string) (string, error) {
	client, err := GetClient()
	if err != nil {
		return "", err
	}

	req := larkim.NewForwardMessageReqBuilder().
		MessageId(messageID).
		ReceiveIdType(receiveIDType).
		Body(larkim.NewForwardMessageReqBodyBuilder().
			ReceiveId(receiveID).
			Build()).
		Build()

	resp, err := client.Im.Message.Forward(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return "", fmt.Errorf("转发消息失败: %w", err)
	}

	if !resp.Success() {
		return "", fmt.Errorf("转发消息失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data.MessageId == nil {
		return "", fmt.Errorf("转发成功但未返回消息 ID")
	}

	return *resp.Data.MessageId, nil
}

// ReadUser represents a user who has read a message
type ReadUser struct {
	UserIDType string
	UserID     string
	Timestamp  string
	TenantKey  string
}

// ReadUsersResult contains the result of getting read users
type ReadUsersResult struct {
	Items     []*ReadUser
	PageToken string
	HasMore   bool
}

// ChatInfo contains chat information
type ChatInfo struct {
	ChatID      string `json:"chat_id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	OwnerID     string `json:"owner_id,omitempty"`
	External    bool   `json:"external,omitempty"`
}

// SearchChatsOptions contains options for searching chats
type SearchChatsOptions struct {
	UserIDType string
	Query      string
	PageToken  string
	PageSize   int
}

// SearchChatsResult contains the result of searching chats
type SearchChatsResult struct {
	Items     []*ChatInfo
	PageToken string
	HasMore   bool
}

// SearchChats searches for chats.
// When query is provided, uses the Search API (server-side filtering).
// When query is empty, falls back to List API with client-side filtering.
func SearchChats(opts SearchChatsOptions, userAccessToken string) (*SearchChatsResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	if opts.UserIDType == "" {
		opts.UserIDType = "open_id"
	}

	if opts.Query != "" {
		return searchChatsWithSearchAPI(client, opts, userAccessToken)
	}
	return searchChatsWithListAPI(client, opts, userAccessToken)
}

// searchChatsWithSearchAPI uses GET /open-apis/im/v1/chats/search for server-side query filtering.
func searchChatsWithSearchAPI(client *lark.Client, opts SearchChatsOptions, userAccessToken string) (*SearchChatsResult, error) {
	reqBuilder := larkim.NewSearchChatReqBuilder().
		UserIdType(opts.UserIDType).
		Query(opts.Query)

	if opts.PageSize > 0 {
		reqBuilder.PageSize(opts.PageSize)
	}
	if opts.PageToken != "" {
		reqBuilder.PageToken(opts.PageToken)
	}

	resp, err := client.Im.Chat.Search(Context(), reqBuilder.Build(), UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("搜索群聊失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("搜索群聊失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	result := &SearchChatsResult{
		PageToken: StringVal(resp.Data.PageToken),
		HasMore:   BoolVal(resp.Data.HasMore),
	}
	for _, chat := range resp.Data.Items {
		result.Items = append(result.Items, &ChatInfo{
			ChatID:      StringVal(chat.ChatId),
			Name:        StringVal(chat.Name),
			Description: StringVal(chat.Description),
			OwnerID:     StringVal(chat.OwnerId),
			External:    BoolVal(chat.External),
		})
	}

	return result, nil
}

// searchChatsWithListAPI uses GET /open-apis/im/v1/chats to list all chats (no query).
func searchChatsWithListAPI(client *lark.Client, opts SearchChatsOptions, userAccessToken string) (*SearchChatsResult, error) {
	reqBuilder := larkim.NewListChatReqBuilder().
		UserIdType(opts.UserIDType)

	if opts.PageSize > 0 {
		reqBuilder.PageSize(opts.PageSize)
	}
	if opts.PageToken != "" {
		reqBuilder.PageToken(opts.PageToken)
	}

	resp, err := client.Im.Chat.List(Context(), reqBuilder.Build(), UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("搜索群聊失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("搜索群聊失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	result := &SearchChatsResult{
		PageToken: StringVal(resp.Data.PageToken),
		HasMore:   BoolVal(resp.Data.HasMore),
	}
	for _, chat := range resp.Data.Items {
		result.Items = append(result.Items, &ChatInfo{
			ChatID:      StringVal(chat.ChatId),
			Name:        StringVal(chat.Name),
			Description: StringVal(chat.Description),
			OwnerID:     StringVal(chat.OwnerId),
			External:    BoolVal(chat.External),
		})
	}

	return result, nil
}

// containsIgnoreCase checks if s contains substr (case insensitive)
func containsIgnoreCase(s, substr string) bool {
	if substr == "" {
		return true
	}
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsIgnoreCaseHelper(s, substr))
}

func containsIgnoreCaseHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if equalIgnoreCase(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

func equalIgnoreCase(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// MergeForwardMessage 合并转发多条消息
func MergeForwardMessage(receiveID, receiveIDType string, messageIDs []string, userAccessToken string) (string, error) {
	client, err := GetClient()
	if err != nil {
		return "", err
	}

	req := larkim.NewMergeForwardMessageReqBuilder().
		ReceiveIdType(receiveIDType).
		Body(larkim.NewMergeForwardMessageReqBodyBuilder().
			ReceiveId(receiveID).
			MessageIdList(messageIDs).
			Build()).
		Build()

	resp, err := client.Im.Message.MergeForward(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return "", fmt.Errorf("合并转发消息失败: %w", err)
	}

	if !resp.Success() {
		return "", fmt.Errorf("合并转发消息失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data.Message == nil || resp.Data.Message.MessageId == nil {
		return "", fmt.Errorf("合并转发成功但未返回消息 ID")
	}

	return *resp.Data.Message.MessageId, nil
}

// CreateReaction 给消息添加表情回复
func CreateReaction(messageID, emojiType string, userAccessToken string) (string, error) {
	client, err := GetClient()
	if err != nil {
		return "", err
	}

	req := larkim.NewCreateMessageReactionReqBuilder().
		MessageId(messageID).
		Body(larkim.NewCreateMessageReactionReqBodyBuilder().
			ReactionType(larkim.NewEmojiBuilder().EmojiType(emojiType).Build()).
			Build()).
		Build()

	resp, err := client.Im.MessageReaction.Create(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return "", fmt.Errorf("添加表情回复失败: %w", err)
	}

	if !resp.Success() {
		return "", fmt.Errorf("添加表情回复失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data.ReactionId == nil {
		return "", fmt.Errorf("添加表情回复成功但未返回 reaction ID")
	}

	return *resp.Data.ReactionId, nil
}

// DeleteReaction 删除消息的表情回复
func DeleteReaction(messageID, reactionID string, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	req := larkim.NewDeleteMessageReactionReqBuilder().
		MessageId(messageID).
		ReactionId(reactionID).
		Build()

	resp, err := client.Im.MessageReaction.Delete(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return fmt.Errorf("删除表情回复失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("删除表情回复失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// ListReactionsResult 表情回复列表结果
type ListReactionsResult struct {
	Items     []*larkim.MessageReaction `json:"items"`
	PageToken string                    `json:"page_token,omitempty"`
	HasMore   bool                      `json:"has_more"`
}

// UrgentMessageResult 消息加急结果
type UrgentMessageResult struct {
	InvalidUserIDList []string `json:"invalid_user_id_list,omitempty"`
}

// urgentCall 封装加急 API 调用，返回 (invalidUserIDs, error)
type urgentCall func() ([]string, error)

// UrgentMessage 对指定消息发送加急提醒（应用内/电话/短信）。
func UrgentMessage(messageID, urgentType, userIDType string, userIDs []string) (*UrgentMessageResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	receivers := larkim.NewUrgentReceiversBuilder().
		UserIdList(userIDs).
		Build()

	// 根据加急类型构建对应的调用闭包
	var call urgentCall
	var label string

	switch urgentType {
	case "app":
		label = "应用内"
		call = func() ([]string, error) {
			req := larkim.NewUrgentAppMessageReqBuilder().
				MessageId(messageID).UserIdType(userIDType).UrgentReceivers(receivers).Build()
			resp, err := client.Im.Message.UrgentApp(Context(), req)
			if err != nil {
				return nil, err
			}
			if !resp.Success() {
				return nil, fmt.Errorf("code=%d, msg=%s", resp.Code, resp.Msg)
			}
			if resp.Data != nil {
				return resp.Data.InvalidUserIdList, nil
			}
			return nil, nil
		}
	case "phone":
		label = "电话"
		call = func() ([]string, error) {
			req := larkim.NewUrgentPhoneMessageReqBuilder().
				MessageId(messageID).UserIdType(userIDType).UrgentReceivers(receivers).Build()
			resp, err := client.Im.Message.UrgentPhone(Context(), req)
			if err != nil {
				return nil, err
			}
			if !resp.Success() {
				return nil, fmt.Errorf("code=%d, msg=%s", resp.Code, resp.Msg)
			}
			if resp.Data != nil {
				return resp.Data.InvalidUserIdList, nil
			}
			return nil, nil
		}
	case "sms":
		label = "短信"
		call = func() ([]string, error) {
			req := larkim.NewUrgentSmsMessageReqBuilder().
				MessageId(messageID).UserIdType(userIDType).UrgentReceivers(receivers).Build()
			resp, err := client.Im.Message.UrgentSms(Context(), req)
			if err != nil {
				return nil, err
			}
			if !resp.Success() {
				return nil, fmt.Errorf("code=%d, msg=%s", resp.Code, resp.Msg)
			}
			if resp.Data != nil {
				return resp.Data.InvalidUserIdList, nil
			}
			return nil, nil
		}
	default:
		return nil, fmt.Errorf("不支持的加急类型: %s，可选值: app, phone, sms", urgentType)
	}

	invalidIDs, err := call()
	if err != nil {
		return nil, fmt.Errorf("发送%s加急失败: %w", label, err)
	}

	return &UrgentMessageResult{InvalidUserIDList: invalidIDs}, nil
}

// ListReactions 获取消息的表情回复列表
func ListReactions(messageID, emojiType string, pageSize int, pageToken string, userAccessToken string) (*ListReactionsResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	reqBuilder := larkim.NewListMessageReactionReqBuilder().
		MessageId(messageID)

	if emojiType != "" {
		reqBuilder.ReactionType(emojiType)
	}
	if pageSize > 0 {
		reqBuilder.PageSize(pageSize)
	}
	if pageToken != "" {
		reqBuilder.PageToken(pageToken)
	}

	resp, err := client.Im.MessageReaction.List(Context(), reqBuilder.Build(), UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("获取表情回复列表失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取表情回复列表失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return &ListReactionsResult{
		Items:     resp.Data.Items,
		PageToken: StringVal(resp.Data.PageToken),
		HasMore:   BoolVal(resp.Data.HasMore),
	}, nil
}

// PinMessage 置顶消息
func PinMessage(messageID string, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	req := larkim.NewCreatePinReqBuilder().
		Body(larkim.NewCreatePinReqBodyBuilder().
			MessageId(messageID).
			Build()).
		Build()

	resp, err := client.Im.Pin.Create(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return fmt.Errorf("置顶消息失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("置顶消息失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// UnpinMessage 取消置顶消息
func UnpinMessage(messageID string, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	req := larkim.NewDeletePinReqBuilder().
		MessageId(messageID).
		Build()

	resp, err := client.Im.Pin.Delete(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return fmt.Errorf("取消置顶消息失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("取消置顶消息失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// ListPinsResult 置顶消息列表结果
type ListPinsResult struct {
	Items     []*larkim.Pin `json:"items"`
	PageToken string        `json:"page_token,omitempty"`
	HasMore   bool          `json:"has_more"`
}

// ListPins 获取群内置顶消息列表
func ListPins(chatID string, startTime, endTime, pageToken string, pageSize int, userAccessToken string) (*ListPinsResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	reqBuilder := larkim.NewListPinReqBuilder().
		ChatId(chatID)

	if startTime != "" {
		reqBuilder.StartTime(startTime)
	}
	if endTime != "" {
		reqBuilder.EndTime(endTime)
	}
	if pageSize > 0 {
		reqBuilder.PageSize(pageSize)
	}
	if pageToken != "" {
		reqBuilder.PageToken(pageToken)
	}

	resp, err := client.Im.Pin.List(Context(), reqBuilder.Build(), UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("获取置顶消息列表失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取置顶消息列表失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return &ListPinsResult{
		Items:     resp.Data.Items,
		PageToken: StringVal(resp.Data.PageToken),
		HasMore:   BoolVal(resp.Data.HasMore),
	}, nil
}

// DownloadMessageResource 下载消息中的资源文件（图片/文件）
func DownloadMessageResource(messageID, fileKey, resourceType, outputPath, userAccessToken string, timeout ...time.Duration) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	req := larkim.NewGetMessageResourceReqBuilder().
		MessageId(messageID).
		FileKey(fileKey).
		Type(resourceType).
		Build()

	t := resolveTimeout(downloadTimeout, timeout)
	resp, err := client.Im.MessageResource.Get(ContextWithTimeout(t), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return fmt.Errorf("下载消息资源失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("下载消息资源失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if err := resp.WriteFile(outputPath); err != nil {
		return fmt.Errorf("保存文件失败: %w", err)
	}

	return nil
}

// BatchGetMessagesResult 是 BatchGetMessages 的返回值。
//
// Messages 保持入参顺序，与 messageIDs 一一对应。
// MergeForwardSubMessages 收集本批所有 merge_forward 容器的递归展开子消息，
// 以容器 message_id 为 key；无展开时为 nil。
type BatchGetMessagesResult struct {
	Messages                []*larkim.Message
	MergeForwardSubMessages map[string][]*larkim.Message
}

// BatchGetMessages 批量获取消息详情，并发调用 GetMessage（限 5 并发，保持入参顺序）。
//
// cardContentType 透传给每次 GetMessage 调用；空字符串则维持原渲染版返回。
// 每次 GetMessage 内部会自动展开 merge_forward 子消息，结果聚合到返回值的 map 中。
func BatchGetMessages(messageIDs []string, userAccessToken, cardContentType string) (*BatchGetMessagesResult, error) {
	const batchGetMessagesConcurrency = 5
	results := make([]*larkim.Message, len(messageIDs))
	errs := make([]error, len(messageIDs))
	subMap := make(map[string][]*larkim.Message)
	var subMu sync.Mutex

	sem := make(chan struct{}, batchGetMessagesConcurrency)
	var wg sync.WaitGroup
	for i, id := range messageIDs {
		i, id := i, id
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			msgResult, err := GetMessage(id, userAccessToken, cardContentType)
			if err != nil {
				errs[i] = fmt.Errorf("获取消息 %s 失败: %w", id, err)
				return
			}
			results[i] = msgResult.Message
			if msgResult.Message != nil && len(msgResult.SubMessages) > 0 {
				containerID := StringVal(msgResult.Message.MessageId)
				if containerID != "" {
					subMu.Lock()
					subMap[containerID] = msgResult.SubMessages
					subMu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	out := &BatchGetMessagesResult{Messages: results}
	if len(subMap) > 0 {
		out.MergeForwardSubMessages = subMap
	}
	return out, nil
}

// ListThreadMessages 列出线程/话题中的消息
func ListThreadMessages(threadID string, opts ListMessagesOptions, userAccessToken string) (*ListMessagesResult, error) {
	// 设置 container_id_type 为 thread
	opts.ContainerIDType = "thread"

	// 复用 ListMessages 逻辑（支持 user token 绕过 SDK 限制）
	return ListMessages(threadID, opts, userAccessToken)
}

// 自动展开线程回复的默认上限。与官方 lark-cli 保持一致：
//   - perThread 默认 50：单 thread 最多拉 50 条回复（也是 OpenAPI 单页上限）
//   - totalLimit 默认 500：所有 thread 累计拉到的回复总数上限，防止极端话题群打爆 QPS
const (
	ExpandThreadRepliesDefaultPerThread  = 50
	ExpandThreadRepliesDefaultTotalLimit = 500
)

// ExpandThreadReplies 对 result.Items 中带 thread_id 的消息逐个自动展开线程回复，
// 结果存入 result.ThreadReplies / result.ThreadHasMore。重复 thread_id 只拉一次。
// 任一 thread 拉失败时静默跳过（不中断整体流程），由调用方决定是否提示。
//
// 与 lark-cli `shortcuts/im/convert_lib/thread.go` 行为对齐：按 ByCreateTimeAsc 拉取，
// 累计达到 totalLimit 时提前停止。回复列表**不含**根消息本身（根消息已在 Items 里）。
//
// 设计要点：
//   - perThread <= 0 时用默认 50；> 50 会被截断为 50（OpenAPI 限制）
//   - totalLimit <= 0 时用默认 500
//   - userAccessToken 为空时仍可工作（走 App Token，前提是 Bot 在群里）
func ExpandThreadReplies(result *ListMessagesResult, userAccessToken string, perThread, totalLimit int) {
	if result == nil || len(result.Items) == 0 {
		return
	}
	if perThread <= 0 {
		perThread = ExpandThreadRepliesDefaultPerThread
	}
	if perThread > 50 {
		perThread = 50
	}
	if totalLimit <= 0 {
		totalLimit = ExpandThreadRepliesDefaultTotalLimit
	}

	if result.ThreadReplies == nil {
		result.ThreadReplies = make(map[string][]*larkim.Message)
	}
	if result.ThreadHasMore == nil {
		result.ThreadHasMore = make(map[string]bool)
	}

	totalFetched := 0
	seen := make(map[string]bool)
	for _, msg := range result.Items {
		if totalFetched >= totalLimit {
			return
		}
		if msg == nil || msg.ThreadId == nil {
			continue
		}
		tid := StringVal(msg.ThreadId)
		if tid == "" || seen[tid] {
			continue
		}
		seen[tid] = true

		limit := perThread
		if remaining := totalLimit - totalFetched; limit > remaining {
			limit = remaining
		}

		opts := ListMessagesOptions{
			ContainerIDType: "thread",
			SortType:        "ByCreateTimeAsc",
			PageSize:        limit,
		}
		sub, err := ListMessages(tid, opts, userAccessToken)
		if err != nil || sub == nil {
			continue
		}

		// 过滤掉与根消息 ID 相同的项（thread API 把根消息也返回了，避免重复）。
		rootID := StringVal(msg.MessageId)
		replies := sub.Items
		if rootID != "" {
			filtered := replies[:0]
			for _, r := range sub.Items {
				if StringVal(r.MessageId) != rootID {
					filtered = append(filtered, r)
				}
			}
			replies = filtered
		}

		if len(replies) > 0 {
			result.ThreadReplies[tid] = replies
			totalFetched += len(replies)
		}
		if sub.HasMore {
			result.ThreadHasMore[tid] = true
		}
	}
}

// GetReadUsers gets the list of users who have read a message
func GetReadUsers(messageID string, userIDType string, pageSize int, pageToken string, userAccessToken string) (*ReadUsersResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	reqBuilder := larkim.NewReadUsersMessageReqBuilder().
		MessageId(messageID).
		UserIdType(userIDType)

	if pageSize > 0 {
		reqBuilder.PageSize(pageSize)
	}
	if pageToken != "" {
		reqBuilder.PageToken(pageToken)
	}

	resp, err := client.Im.Message.ReadUsers(Context(), reqBuilder.Build(), UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("查询消息已读用户失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("查询消息已读用户失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	result := &ReadUsersResult{
		PageToken: StringVal(resp.Data.PageToken),
		HasMore:   BoolVal(resp.Data.HasMore),
	}
	for _, item := range resp.Data.Items {
		result.Items = append(result.Items, &ReadUser{
			UserIDType: StringVal(item.UserIdType),
			UserID:     StringVal(item.UserId),
			Timestamp:  StringVal(item.Timestamp),
			TenantKey:  StringVal(item.TenantKey),
		})
	}

	return result, nil
}
