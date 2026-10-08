package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const minutesBase = "/open-apis/minutes/v1"

// GetMinute 获取妙记基础信息
// API: GET /open-apis/minutes/v1/minutes/{minute_token}
// 返回 data 字段原始 JSON（包含 minute.title / minute.url / minute.create_time / minute.owner_id / minute.note_id 等）
func GetMinute(minuteToken, userAccessToken string) (json.RawMessage, error) {
	apiPath := fmt.Sprintf("%s/minutes/%s", minutesBase, url.PathEscape(minuteToken))

	return vcCallAPI("获取妙记信息", http.MethodGet, apiPath, nil, userAccessToken)
}

// GetMinuteArtifacts 获取妙记 AI 产物（summary / minute_todos / minute_chapters）
// API: GET /open-apis/minutes/v1/minutes/{minute_token}/artifacts
func GetMinuteArtifacts(minuteToken, userAccessToken string) (json.RawMessage, error) {
	apiPath := fmt.Sprintf("%s/minutes/%s/artifacts", minutesBase, url.PathEscape(minuteToken))

	return vcCallAPI("获取妙记 AI 产物", http.MethodGet, apiPath, nil, userAccessToken)
}

// GetMinuteTranscript 获取妙记文字稿（txt 格式，含说话人和时间戳）
// API: GET /open-apis/minutes/v1/minutes/{minute_token}/transcript
// 返回原始字节，调用方负责写文件。
// 成功时服务端直接返回文件字节；失败时返回飞书 JSON 信封（可能随 HTTP 4xx 下发），
// 先按业务信封解析，保证 2091005 等业务码能被 HasAPICode 识别。
func GetMinuteTranscript(minuteToken, userAccessToken string) ([]byte, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	tokenType, opts := resolveTokenOpts(userAccessToken)
	apiPath := fmt.Sprintf("%s/minutes/%s/transcript?need_speaker=true&need_timestamp=true&file_format=txt",
		minutesBase, url.PathEscape(minuteToken))

	resp, err := client.Get(Context(), apiPath, nil, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("获取妙记文字稿失败: %w", err)
	}
	if err := CheckAPIResponse("获取妙记文字稿", resp); err != nil {
		return nil, err
	}
	if len(resp.RawBody) == 0 {
		return nil, fmt.Errorf("获取妙记文字稿失败: 响应体为空")
	}
	return resp.RawBody, nil
}

// GetMinuteMediaURL 获取妙记媒体文件的预签名下载 URL
// API: GET /open-apis/minutes/v1/minutes/{minute_token}/media
// 返回 data.download_url
func GetMinuteMediaURL(minuteToken, userAccessToken string) (string, error) {
	apiPath := fmt.Sprintf("%s/minutes/%s/media", minutesBase, url.PathEscape(minuteToken))
	data, err := vcCallAPI("获取妙记媒体下载链接", http.MethodGet, apiPath, nil, userAccessToken)
	if err != nil {
		return "", err
	}
	var parsed struct {
		DownloadURL string `json:"download_url"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("获取妙记媒体下载链接失败: 解析响应失败: %w", err)
	}
	if parsed.DownloadURL == "" {
		return "", fmt.Errorf("获取妙记媒体下载链接失败: 响应中未包含 download_url")
	}
	return parsed.DownloadURL, nil
}
