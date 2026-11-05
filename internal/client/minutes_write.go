package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// 妙记写接口的业务错误码（对齐官方 shortcuts/minutes）。
const (
	MinutesCodeSpeakerNotFound   = 2091001 // 说话人不存在
	MinutesCodeNoEditPermission  = 2091005 // 无妙记编辑权限
	MinutesCodeASRQuotaNotEnough = 2091008 // ASR/AI 配额不足
	MinutesCodeOthersEditing     = 2091110 // 文字稿正被他人编辑
	MinutesCodeWordsNotFound     = 2091111 // 替换词均未在文字稿中找到
)

// MinutesUploadPath 是由云盘文件生成妙记的接口路径。
const MinutesUploadPath = minutesBase + "/minutes/upload"

// MinutePath 返回 /open-apis/minutes/v1/minutes/{token}{suffix}（token 做 PathEscape）。
func MinutePath(minuteToken, suffix string) string {
	return fmt.Sprintf("%s/minutes/%s%s", minutesBase, url.PathEscape(minuteToken), suffix)
}

// UploadMinute 用已上传到云盘的音视频 file_token 生成妙记。
// API: POST /open-apis/minutes/v1/minutes/upload  body {"file_token": ...}，仅 User 身份（minutes:minutes.upload:write）。
func UploadMinute(fileToken, userAccessToken string) (json.RawMessage, error) {
	return vcCallAPI("生成妙记", http.MethodPost, MinutesUploadPath, map[string]any{"file_token": fileToken}, userAccessToken)
}

// UpdateMinuteTopic 修改妙记标题。API: PATCH /open-apis/minutes/v1/minutes/{token}  body {"topic": ...}
func UpdateMinuteTopic(minuteToken, topic, userAccessToken string) (json.RawMessage, error) {
	return vcCallAPI("修改妙记标题", http.MethodPatch, MinutePath(minuteToken, ""), map[string]any{"topic": topic}, userAccessToken)
}

// ReplaceMinuteSummary 替换妙记 AI 摘要。API: PUT /open-apis/minutes/v1/minutes/{token}/summary  body {"summary": ...}
func ReplaceMinuteSummary(minuteToken, summary, userAccessToken string) (json.RawMessage, error) {
	return vcCallAPI("替换妙记摘要", http.MethodPut, MinutePath(minuteToken, "/summary"), map[string]any{"summary": summary}, userAccessToken)
}

// WriteMinuteTodos 批量增改删妙记待办。API: POST /open-apis/minutes/v1/minutes/{token}/todo  body {"todo_items": [...]}
func WriteMinuteTodos(minuteToken string, items []map[string]any, userAccessToken string) (json.RawMessage, error) {
	return vcCallAPI("更新妙记待办", http.MethodPost, MinutePath(minuteToken, "/todo"), map[string]any{"todo_items": items}, userAccessToken)
}

// ReplaceMinuteSpeaker 替换文字稿说话人。
// API: PUT /open-apis/minutes/v1/minutes/{token}/transcript/speaker?user_id_type=open_id
func ReplaceMinuteSpeaker(minuteToken string, body map[string]any, userAccessToken string) (json.RawMessage, error) {
	return vcCallAPI("替换妙记说话人", http.MethodPut, MinutePath(minuteToken, "/transcript/speaker")+"?user_id_type=open_id", body, userAccessToken)
}

// ReplaceMinuteWords 批量替换文字稿中的词。
// API: PUT /open-apis/minutes/v1/minutes/{token}/transcript/word  body {"minute_token", "replace_words": [...]}
func ReplaceMinuteWords(minuteToken string, replaceWords []map[string]string, userAccessToken string) (json.RawMessage, error) {
	body := map[string]any{"minute_token": minuteToken, "replace_words": replaceWords}
	return vcCallAPI("替换妙记文字稿词语", http.MethodPut, MinutePath(minuteToken, "/transcript/word"), body, userAccessToken)
}
