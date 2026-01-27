package client

import (
	"encoding/json"
	"fmt"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// SendMessage sends a message to a user or chat
func SendMessage(receiveIDType string, receiveID string, msgType string, content string) (string, error) {
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

	resp, err := client.Im.Message.Create(Context(), req)
	if err != nil {
		return "", fmt.Errorf("failed to send message: %w", err)
	}

	if !resp.Success() {
		return "", fmt.Errorf("failed to send message: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data.MessageId == nil {
		return "", fmt.Errorf("message sent but no message ID returned")
	}

	return *resp.Data.MessageId, nil
}

// ReplyMessage replies to a message
func ReplyMessage(messageID string, msgType string, content string) (string, error) {
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

	resp, err := client.Im.Message.Reply(Context(), req)
	if err != nil {
		return "", fmt.Errorf("failed to reply message: %w", err)
	}

	if !resp.Success() {
		return "", fmt.Errorf("failed to reply message: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data.MessageId == nil {
		return "", fmt.Errorf("reply sent but no message ID returned")
	}

	return *resp.Data.MessageId, nil
}

// UpdateMessage updates a message content
func UpdateMessage(messageID string, content string) error {
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

	resp, err := client.Im.Message.Patch(Context(), req)
	if err != nil {
		return fmt.Errorf("failed to update message: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("failed to update message: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// CreateTextMessageContent creates content for a text message
func CreateTextMessageContent(text string) string {
	content := map[string]string{"text": text}
	data, _ := json.Marshal(content)
	return string(data)
}

// CreateRichTextMessageContent creates content for a rich text (post) message
func CreateRichTextMessageContent(title string, content [][]map[string]interface{}) string {
	post := map[string]interface{}{
		"zh_cn": map[string]interface{}{
			"title":   title,
			"content": content,
		},
	}
	data, _ := json.Marshal(post)
	return string(data)
}

// CreateInteractiveCardContent creates content for an interactive card message
func CreateInteractiveCardContent(card map[string]interface{}) string {
	data, _ := json.Marshal(card)
	return string(data)
}

// DeleteMessage deletes a message by message ID
func DeleteMessage(messageID string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	req := larkim.NewDeleteMessageReqBuilder().
		MessageId(messageID).
		Build()

	resp, err := client.Im.Message.Delete(Context(), req)
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
func ListMessages(containerID string, opts ListMessagesOptions) (*ListMessagesResult, error) {
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

	result := &ListMessagesResult{
		Items: resp.Data.Items,
	}
	if resp.Data.PageToken != nil {
		result.PageToken = *resp.Data.PageToken
	}
	if resp.Data.HasMore != nil {
		result.HasMore = *resp.Data.HasMore
	}

	return result, nil
}

// GetMessageResult contains the result of getting a message
type GetMessageResult struct {
	Message *larkim.Message
}

// GetMessage gets a message by message ID
func GetMessage(messageID string) (*GetMessageResult, error) {
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

	// Get returns a list but we want the first message
	if len(resp.Data.Items) == 0 {
		return nil, fmt.Errorf("消息不存在")
	}

	return &GetMessageResult{
		Message: resp.Data.Items[0],
	}, nil
}

// ForwardMessage forwards a message to another recipient
func ForwardMessage(messageID string, receiveID string, receiveIDType string) (string, error) {
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

	resp, err := client.Im.Message.Forward(Context(), req)
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
func SearchChats(opts SearchChatsOptions) (*SearchChatsResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	// Default user ID type
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

	resp, err := client.Im.Chat.List(Context(), reqBuilder.Build())
	if err != nil {
		return nil, fmt.Errorf("搜索群聊失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("搜索群聊失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	result := &SearchChatsResult{}
	for _, chat := range resp.Data.Items {
		info := &ChatInfo{}
		if chat.ChatId != nil {
			info.ChatID = *chat.ChatId
		}
		if chat.Name != nil {
			info.Name = *chat.Name
		}
		if chat.Description != nil {
			info.Description = *chat.Description
		}
		if chat.OwnerId != nil {
			info.OwnerID = *chat.OwnerId
		}
		if chat.External != nil {
			info.External = *chat.External
		}

		// Filter by query if specified
		if opts.Query == "" || containsIgnoreCase(info.Name, opts.Query) || containsIgnoreCase(info.Description, opts.Query) {
			result.Items = append(result.Items, info)
		}
	}

	if resp.Data.PageToken != nil {
		result.PageToken = *resp.Data.PageToken
	}
	if resp.Data.HasMore != nil {
		result.HasMore = *resp.Data.HasMore
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

// GetReadUsers gets the list of users who have read a message
func GetReadUsers(messageID string, userIDType string, pageSize int, pageToken string) (*ReadUsersResult, error) {
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

	resp, err := client.Im.Message.ReadUsers(Context(), reqBuilder.Build())
	if err != nil {
		return nil, fmt.Errorf("查询消息已读用户失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("查询消息已读用户失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	result := &ReadUsersResult{}
	for _, item := range resp.Data.Items {
		user := &ReadUser{}
		if item.UserIdType != nil {
			user.UserIDType = *item.UserIdType
		}
		if item.UserId != nil {
			user.UserID = *item.UserId
		}
		if item.Timestamp != nil {
			user.Timestamp = *item.Timestamp
		}
		if item.TenantKey != nil {
			user.TenantKey = *item.TenantKey
		}
		result.Items = append(result.Items, user)
	}

	if resp.Data.PageToken != nil {
		result.PageToken = *resp.Data.PageToken
	}
	if resp.Data.HasMore != nil {
		result.HasMore = *resp.Data.HasMore
	}

	return result, nil
}
