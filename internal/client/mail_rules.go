package client

import (
	"encoding/json"
	"net/http"
)

// ==================== 收信规则 ====================

// MailRulesPath 收信规则集合路径（导出供 dry-run 预览复用）。
func MailRulesPath(mailboxID string, segments ...string) string {
	if mailboxID == "" {
		mailboxID = "me"
	}
	return mailboxPath(mailboxID, append([]string{"rules"}, segments...)...)
}

// ListMailRules 列出收信规则（服务端按执行顺序返回）。
// API: GET /open-apis/mail/v1/user_mailboxes/{mailbox_id}/rules
func ListMailRules(mailboxID, userAccessToken string) (json.RawMessage, error) {
	return callMailAPI(http.MethodGet, MailRulesPath(mailboxID), nil, userAccessToken)
}

// CreateMailRule 创建收信规则。
// API: POST /open-apis/mail/v1/user_mailboxes/{mailbox_id}/rules
func CreateMailRule(mailboxID string, body map[string]any, userAccessToken string) (json.RawMessage, error) {
	return callMailAPI(http.MethodPost, MailRulesPath(mailboxID), body, userAccessToken)
}

// UpdateMailRule 全量更新收信规则（PUT 语义，调用方需先读取当前规则再合并）。
// API: PUT /open-apis/mail/v1/user_mailboxes/{mailbox_id}/rules/{rule_id}
func UpdateMailRule(mailboxID, ruleID string, body map[string]any, userAccessToken string) (json.RawMessage, error) {
	return callMailAPI(http.MethodPut, MailRulesPath(mailboxID, ruleID), body, userAccessToken)
}

// DeleteMailRule 删除收信规则。
// API: DELETE /open-apis/mail/v1/user_mailboxes/{mailbox_id}/rules/{rule_id}
func DeleteMailRule(mailboxID, ruleID, userAccessToken string) error {
	_, err := callMailAPI(http.MethodDelete, MailRulesPath(mailboxID, ruleID), nil, userAccessToken)
	return err
}

// ReorderMailRules 按完整 rule_id 列表重排收信规则。
// API: POST /open-apis/mail/v1/user_mailboxes/{mailbox_id}/rules/reorder
func ReorderMailRules(mailboxID string, ruleIDs []string, userAccessToken string) error {
	_, err := callMailAPI(http.MethodPost, MailRulesPath(mailboxID, "reorder"), map[string]any{"rule_ids": ruleIDs}, userAccessToken)
	return err
}

// ==================== 线程整理 ====================

// MailThreadsBatchPath 线程批量操作路径（batch_modify / batch_trash）。
func MailThreadsBatchPath(mailboxID, op string) string {
	if mailboxID == "" {
		mailboxID = "me"
	}
	return mailboxPath(mailboxID, "threads", op)
}

// BatchModifyMailThreads 批量修改线程标签 / 移动文件夹（单批 ≤20，由调用方分批）。
// API: POST /open-apis/mail/v1/user_mailboxes/{mailbox_id}/threads/batch_modify
func BatchModifyMailThreads(mailboxID string, body map[string]any, userAccessToken string) (json.RawMessage, error) {
	return callMailAPI(http.MethodPost, MailThreadsBatchPath(mailboxID, "batch_modify"), body, userAccessToken)
}

// BatchTrashMailThreads 批量把线程移入废纸篓（软删除，单批 ≤20，由调用方分批）。
// API: POST /open-apis/mail/v1/user_mailboxes/{mailbox_id}/threads/batch_trash
func BatchTrashMailThreads(mailboxID string, threadIDs []string, userAccessToken string) (json.RawMessage, error) {
	return callMailAPI(http.MethodPost, MailThreadsBatchPath(mailboxID, "batch_trash"), map[string]any{"thread_ids": threadIDs}, userAccessToken)
}
