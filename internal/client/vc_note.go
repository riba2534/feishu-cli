package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// GetUnifiedNoteTranscript 拉取智能纪要的统一逐字稿单页。
// GET /open-apis/vc/v1/notes/{note_id}/unified_note_transcript
// params 支持 transcript_format（markdown/text）与 cursor_id（翻页游标）。
func GetUnifiedNoteTranscript(noteID string, params map[string]string, userAccessToken string) (map[string]any, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	tokenType, opts := resolveTokenOpts(userAccessToken)
	query := url.Values{}
	for k, v := range params {
		if v != "" {
			query.Set(k, v)
		}
	}
	apiPath := fmt.Sprintf("%s/notes/%s/unified_note_transcript", vcBase, url.PathEscape(noteID))
	if encoded := query.Encode(); encoded != "" {
		apiPath += "?" + encoded
	}

	resp, err := client.Get(Context(), apiPath, nil, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("获取统一逐字稿失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取统一逐字稿失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}

	var apiResp struct {
		Code int            `json:"code"`
		Msg  string         `json:"msg"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("获取统一逐字稿失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	return apiResp.Data, nil
}
