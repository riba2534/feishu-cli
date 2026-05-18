package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ==================== 邮件模板 ====================

// MailTemplateAddr 模板的发件人/收件人地址（snake_case 与 OpenAPI 对齐）
type MailTemplateAddr struct {
	MailAddress string `json:"mail_address"`
	Name        string `json:"name,omitempty"`
}

// MailTemplateAttachment 模板附件结构
// body 是后端强制必填字段，否则会返回 99992402；对于已上传到云盘的文件，
// id 与 body 共用同一份 file_key 即可满足校验
type MailTemplateAttachment struct {
	ID             string `json:"id,omitempty"`
	Filename       string `json:"filename,omitempty"`
	CID            string `json:"cid,omitempty"`
	IsInline       bool   `json:"is_inline"`
	AttachmentType int    `json:"attachment_type,omitempty"` // 1=SMALL, 2=LARGE
	Body           string `json:"body"`
}

// MailTemplate 个人邮件模板（用于 templates.create / update / get）
type MailTemplate struct {
	TemplateID      string                   `json:"template_id,omitempty"`
	Name            string                   `json:"name"`
	Subject         string                   `json:"subject,omitempty"`
	TemplateContent string                   `json:"template_content,omitempty"`
	IsPlainTextMode bool                     `json:"is_plain_text_mode"`
	Tos             []MailTemplateAddr       `json:"tos,omitempty"`
	Ccs             []MailTemplateAddr       `json:"ccs,omitempty"`
	Bccs            []MailTemplateAddr       `json:"bccs,omitempty"`
	Attachments     []MailTemplateAttachment `json:"attachments,omitempty"`
	CreateTime      string                   `json:"create_time,omitempty"`
}

// templatePath 构造模板 API 路径
// /open-apis/mail/v1/user_mailboxes/{mailbox_id}/templates[/{template_id}]
func templatePath(mailboxID string, segments ...string) string {
	if mailboxID == "" {
		mailboxID = "me"
	}
	parts := []string{url.PathEscape(mailboxID), "templates"}
	for _, s := range segments {
		if s == "" {
			continue
		}
		parts = append(parts, url.PathEscape(s))
	}
	return mailBase + "/user_mailboxes/" + strings.Join(parts, "/")
}

// CreateMailTemplate 创建个人邮件模板
// API: POST /open-apis/mail/v1/user_mailboxes/{mailbox_id}/templates
// 请求体形如 {"template": {...}}，响应也包在 "template" 下
func CreateMailTemplate(mailboxID string, tpl *MailTemplate, userAccessToken string) (*MailTemplate, error) {
	body := map[string]any{"template": tpl}
	data, err := callMailAPI(http.MethodPost, templatePath(mailboxID), body, userAccessToken)
	if err != nil {
		return nil, err
	}
	return extractTemplate(data)
}

// UpdateMailTemplate 全量替换式更新模板（无乐观锁，last-write-wins）
// API: PUT /open-apis/mail/v1/user_mailboxes/{mailbox_id}/templates/{template_id}
func UpdateMailTemplate(mailboxID, templateID string, tpl *MailTemplate, userAccessToken string) (*MailTemplate, error) {
	body := map[string]any{"template": tpl}
	data, err := callMailAPI(http.MethodPut, templatePath(mailboxID, templateID), body, userAccessToken)
	if err != nil {
		return nil, err
	}
	return extractTemplate(data)
}

// GetMailTemplate 获取模板详情
// API: GET /open-apis/mail/v1/user_mailboxes/{mailbox_id}/templates/{template_id}
func GetMailTemplate(mailboxID, templateID, userAccessToken string) (*MailTemplate, error) {
	data, err := callMailAPI(http.MethodGet, templatePath(mailboxID, templateID), nil, userAccessToken)
	if err != nil {
		return nil, err
	}
	return extractTemplate(data)
}

// DeleteMailTemplate 删除模板
// API: DELETE /open-apis/mail/v1/user_mailboxes/{mailbox_id}/templates/{template_id}
func DeleteMailTemplate(mailboxID, templateID, userAccessToken string) error {
	_, err := callMailAPI(http.MethodDelete, templatePath(mailboxID, templateID), nil, userAccessToken)
	return err
}

// MailTemplateBrief 列表返回的精简结构（接口本身不分页，只返回 id/name）
type MailTemplateBrief struct {
	TemplateID string `json:"template_id"`
	Name       string `json:"name"`
}

// ListMailTemplates 列出指定邮箱下的全部个人邮件模板
// API: GET /open-apis/mail/v1/user_mailboxes/{mailbox_id}/templates
// 注意：接口不分页，返回所有模板的 id 与 name
func ListMailTemplates(mailboxID, userAccessToken string) ([]MailTemplateBrief, error) {
	data, err := callMailAPI(http.MethodGet, templatePath(mailboxID), nil, userAccessToken)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Templates []MailTemplateBrief `json:"templates"`
		Items     []MailTemplateBrief `json:"items"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("解析模板列表失败: %w", err)
	}
	if len(parsed.Templates) > 0 {
		return parsed.Templates, nil
	}
	return parsed.Items, nil
}

// extractTemplate 从响应里取出 "template" 包装的对象
func extractTemplate(data json.RawMessage) (*MailTemplate, error) {
	var parsed struct {
		Template *MailTemplate `json:"template"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("解析模板响应失败: %w", err)
	}
	if parsed.Template == nil {
		// 有些场景直接返回顶层对象（如自定义 mock），兜底解析
		var direct MailTemplate
		if err := json.Unmarshal(data, &direct); err == nil && direct.TemplateID != "" {
			return &direct, nil
		}
		return nil, fmt.Errorf("响应中未找到 template 字段")
	}
	return parsed.Template, nil
}

// ==================== 已读回执 ====================

// ModifyMailMessage 修改邮件（添加/移除 label）
// API: PUT /open-apis/mail/v1/user_mailboxes/{mailbox_id}/messages/{message_id}/modify
func ModifyMailMessage(mailboxID, messageID string, addLabels, removeLabels []string, userAccessToken string) error {
	if mailboxID == "" {
		mailboxID = "me"
	}
	body := map[string]any{}
	if len(addLabels) > 0 {
		body["add_label_ids"] = addLabels
	}
	if len(removeLabels) > 0 {
		body["remove_label_ids"] = removeLabels
	}
	_, err := callMailAPI(http.MethodPut, mailboxPath(mailboxID, "messages", messageID, "modify"), body, userAccessToken)
	return err
}

// ==================== 分享到聊天 ====================

// CreateMailShareToken 为邮件或会话生成分享 token
// API: POST /open-apis/mail/v1/user_mailboxes/{mailbox_id}/messages/share_token
// body 二选一：{"message_id": "xxx"} 或 {"thread_id": "xxx"}
// 返回的 card_id 用于 SendMailShareCard
func CreateMailShareToken(mailboxID, messageID, threadID, userAccessToken string) (string, error) {
	if mailboxID == "" {
		mailboxID = "me"
	}
	body := map[string]any{}
	switch {
	case threadID != "":
		body["thread_id"] = threadID
	case messageID != "":
		body["message_id"] = messageID
	default:
		return "", fmt.Errorf("message_id / thread_id 至少一个非空")
	}
	data, err := callMailAPI(http.MethodPost, mailboxPath(mailboxID, "messages", "share_token"), body, userAccessToken)
	if err != nil {
		return "", err
	}
	var parsed struct {
		CardID string `json:"card_id"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("解析 share_token 响应失败: %w", err)
	}
	if parsed.CardID == "" {
		return "", fmt.Errorf("响应中未返回 card_id")
	}
	return parsed.CardID, nil
}

// SendMailShareCard 把分享 token 投递到指定 IM 会话
// API: POST /open-apis/mail/v1/user_mailboxes/{mailbox_id}/share_tokens/{card_id}/send?receive_id_type={type}
// receiveIDType: chat_id | open_id | user_id | union_id | email
// 返回 IM message_id（投递后的卡片消息 ID）
func SendMailShareCard(mailboxID, cardID, receiveID, receiveIDType, userAccessToken string) (string, error) {
	if mailboxID == "" {
		mailboxID = "me"
	}
	if receiveIDType == "" {
		receiveIDType = "chat_id"
	}
	apiPath := mailboxPath(mailboxID, "share_tokens", cardID, "send") + "?receive_id_type=" + url.QueryEscape(receiveIDType)
	body := map[string]any{"receive_id": receiveID}
	data, err := callMailAPI(http.MethodPost, apiPath, body, userAccessToken)
	if err != nil {
		return "", err
	}
	var parsed struct {
		MessageID string `json:"message_id"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("解析 share send 响应失败: %w", err)
	}
	return parsed.MessageID, nil
}

// ==================== 邮箱事件订阅（watch） ====================

// SubscribeMailEvent 订阅邮箱事件（watch 启动前必须调用一次）
// API: POST /open-apis/mail/v1/user_mailboxes/{mailbox_id}/event/subscribe
// event_type: 1 = message_received（推送新邮件事件）
func SubscribeMailEvent(mailboxID string, eventType int, userAccessToken string) error {
	if mailboxID == "" {
		mailboxID = "me"
	}
	if eventType == 0 {
		eventType = 1
	}
	_, err := callMailAPI(http.MethodPost, mailboxPath(mailboxID, "event", "subscribe"),
		map[string]any{"event_type": eventType}, userAccessToken)
	return err
}

// UnsubscribeMailEvent 取消订阅邮箱事件（watch 退出时调用）
// API: POST /open-apis/mail/v1/user_mailboxes/{mailbox_id}/event/unsubscribe
func UnsubscribeMailEvent(mailboxID string, eventType int, userAccessToken string) error {
	if mailboxID == "" {
		mailboxID = "me"
	}
	if eventType == 0 {
		eventType = 1
	}
	_, err := callMailAPI(http.MethodPost, mailboxPath(mailboxID, "event", "unsubscribe"),
		map[string]any{"event_type": eventType}, userAccessToken)
	return err
}

// GetMailEventSubscription 查询邮箱事件订阅状态
// API: GET /open-apis/mail/v1/user_mailboxes/{mailbox_id}/event/subscription
func GetMailEventSubscription(mailboxID, userAccessToken string) (json.RawMessage, error) {
	if mailboxID == "" {
		mailboxID = "me"
	}
	return callMailAPI(http.MethodGet, mailboxPath(mailboxID, "event", "subscription"), nil, userAccessToken)
}

// ==================== CID 上传辅助（内嵌图片） ====================

// UploadMailInlineImage 上传一张内嵌图片到云盘并返回 file_token
// parent_type 固定 "email"，与模板/大附件链路对齐
// userOpenID 必填：飞书要求 email 上下文的 ParentNode 是 user open_id
//
// 复用 drive.UploadMedia（SDK v3 的 UploadAll），parent_type=email 走内嵌图片场景
func UploadMailInlineImage(filePath, fileName, userOpenID, userAccessToken string) (string, error) {
	fileToken, _, err := UploadMedia(filePath, "email", userOpenID, fileName, userAccessToken)
	if err != nil {
		return "", fmt.Errorf("上传内嵌图片失败: %w", err)
	}
	return fileToken, nil
}
