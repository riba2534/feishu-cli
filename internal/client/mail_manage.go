package client

import (
	"encoding/json"
	"net/http"
	"net/url"
)

// BatchModifyMailMessages 批量修改邮件：添加/移除标签、移动到指定文件夹。
// API: POST /open-apis/mail/v1/user_mailboxes/{mailbox_id}/messages/batch_modify
// body: {"message_ids":[...], "add_label_ids":[...], "remove_label_ids":[...], "add_folder":"..."}
//   - addLabelIDs / removeLabelIDs / folderID 为空的项自动省略。
//   - folderID 对应请求体的 add_folder 字段（把邮件移入该文件夹）。
//   - userIDType 非空时作为 user_id_type query 传入（该端点响应不含用户 ID，通常无需指定）。
//
// 与 mail_advanced.go 中单条 ModifyMailMessage（PUT .../messages/{id}/modify）不同，
// 本函数走批量端点，一次最多处理 20 封（由 CLI 层校验）。
func BatchModifyMailMessages(mailboxID string, messageIDs, addLabelIDs, removeLabelIDs []string, folderID, userIDType, userAccessToken string) (json.RawMessage, error) {
	if mailboxID == "" {
		mailboxID = "me"
	}
	body := map[string]any{"message_ids": messageIDs}
	if len(addLabelIDs) > 0 {
		body["add_label_ids"] = addLabelIDs
	}
	if len(removeLabelIDs) > 0 {
		body["remove_label_ids"] = removeLabelIDs
	}
	if folderID != "" {
		body["add_folder"] = folderID
	}
	apiPath := mailboxPath(mailboxID, "messages", "batch_modify")
	if userIDType != "" {
		apiPath += "?user_id_type=" + url.QueryEscape(userIDType)
	}
	return callMailAPI(http.MethodPost, apiPath, body, userAccessToken)
}

// BatchTrashMailMessages 批量软删除邮件（移入废纸篓，可在飞书邮箱内恢复）。
// API: POST /open-apis/mail/v1/user_mailboxes/{mailbox_id}/messages/batch_trash
// body: {"message_ids":[...]}
// 一次最多处理 20 封（由 CLI 层校验）。
func BatchTrashMailMessages(mailboxID string, messageIDs []string, userAccessToken string) (json.RawMessage, error) {
	if mailboxID == "" {
		mailboxID = "me"
	}
	body := map[string]any{"message_ids": messageIDs}
	return callMailAPI(http.MethodPost, mailboxPath(mailboxID, "messages", "batch_trash"), body, userAccessToken)
}
