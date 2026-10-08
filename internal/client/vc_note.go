package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
)

// GetUnifiedNoteTranscript 拉取智能纪要的统一逐字稿单页。
// GET /open-apis/vc/v1/notes/{note_id}/unified_note_transcript
// params 支持 format（markdown / plain_text）、page_size、locale 与 cursor_id（翻页游标），
// 参数名对齐官方 shortcuts/note/note_transcript.go。
//
// 返回 data 对象；数字按 json.Number 保留（next_cursor_id 可能是超过 2^53 的数字，
// 解成 float64 会丢精度导致翻页游标错误）。
// 普通纪要（note_display_type=normal）服务端返回 121002 not support，错误可用 HasAPICode 识别。
func GetUnifiedNoteTranscript(noteID string, params map[string]string, userAccessToken string) (map[string]any, error) {
	cli, err := GetClient()
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

	resp, err := cli.Get(Context(), apiPath, nil, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("获取统一逐字稿失败: %w", err)
	}
	if err := CheckAPIResponse("获取统一逐字稿", resp); err != nil {
		return nil, err
	}

	var apiResp struct {
		Data map[string]any `json:"data"`
	}
	dec := json.NewDecoder(bytes.NewReader(resp.RawBody))
	dec.UseNumber()
	if err := dec.Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("获取统一逐字稿失败: 解析响应失败: %w", err)
	}
	return apiResp.Data, nil
}
