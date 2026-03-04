package client

import (
	"encoding/json"
	"fmt"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
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

	resp, err := client.Im.Message.Create(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
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

// ReplyMessage replies to a message
func ReplyMessage(messageID string, msgType string, content string, userAccessToken string) (string, error) {
	client, err := GetClient()
	if err != nil {
		return "", err
	}

	req := larkim.NewReplyMessageReqBuilder().
		MessageId(messageID).
		Body(larkim.NewReplyMessageReqBodyBuilder().
			MsgType(msgType).
			Content(content).
			Build()).
		Build()

	resp, err := client.Im.Message.Reply(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
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

	resp, err := client.Im.Message.Patch(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
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

	resp, err := client.Im.Message.Delete(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
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
}

// ListMessagesResult contains the result of listing messages
type ListMessagesResult struct {
	Items     []*larkim.Message
	PageToken string
	HasMore   bool
}

// ListMessages lists messages in a container (chat)
func ListMessages(containerID string, opts ListMessagesOptions, userAccessToken string) (*ListMessagesResult, error) {
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

	resp, err := client.Im.Message.List(Context(), reqBuilder.Build(), larkcore.WithUserAccessToken(userAccessToken))
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

// GetMessageResult contains the result of getting a message
type GetMessageResult struct {
	Message *larkim.Message
}

// GetMessage gets a message by message ID
func GetMessage(messageID string, userAccessToken string) (*GetMessageResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larkim.NewGetMessageReqBuilder().
		MessageId(messageID).
		Build()

	resp, err := client.Im.Message.Get(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
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

	resp, err := client.Im.Message.Forward(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
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

// SearchChats searches for chats
func SearchChats(opts SearchChatsOptions, userAccessToken string) (*SearchChatsResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	if opts.UserIDType == "" {
		opts.UserIDType = "open_id"
	}

	reqBuilder := larkim.NewListChatReqBuilder().
		UserIdType(opts.UserIDType)

	if opts.PageSize > 0 {
		reqBuilder.PageSize(opts.PageSize)
	}
	if opts.PageToken != "" {
		reqBuilder.PageToken(opts.PageToken)
	}

	resp, err := client.Im.Chat.List(Context(), reqBuilder.Build(), larkcore.WithUserAccessToken(userAccessToken))
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
		info := &ChatInfo{
			ChatID:      StringVal(chat.ChatId),
			Name:        StringVal(chat.Name),
			Description: StringVal(chat.Description),
			OwnerID:     StringVal(chat.OwnerId),
			External:    BoolVal(chat.External),
		}

		// Filter by query if specified
		if opts.Query == "" || containsIgnoreCase(info.Name, opts.Query) || containsIgnoreCase(info.Description, opts.Query) {
			result.Items = append(result.Items, info)
		}
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

	resp, err := client.Im.Message.MergeForward(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
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

	resp, err := client.Im.MessageReaction.Create(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
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

	resp, err := client.Im.MessageReaction.Delete(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
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

	resp, err := client.Im.MessageReaction.List(Context(), reqBuilder.Build(), larkcore.WithUserAccessToken(userAccessToken))
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

	resp, err := client.Im.Pin.Create(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
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

	resp, err := client.Im.Pin.Delete(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
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

	resp, err := client.Im.Pin.List(Context(), reqBuilder.Build(), larkcore.WithUserAccessToken(userAccessToken))
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

	resp, err := client.Im.Message.ReadUsers(Context(), reqBuilder.Build(), larkcore.WithUserAccessToken(userAccessToken))
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
