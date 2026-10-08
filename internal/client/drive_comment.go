package client

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// CreateNewCommentReq 创建评论请求（V2 API，支持富文本 reply_elements）
type CreateNewCommentReq struct {
	FileToken     string           // 目标文件 token（docx/doc/sheet/slides/bitable/file）
	FileType      string           // docx / doc / sheet / slides / bitable / file
	BlockID       string           // 可选：局部评论的 anchor block_id（docx）
	ReplyElements []map[string]any // reply_elements 数组
	// Anchor 非 nil 时作为完整 anchor 对象下发（优先于 BlockID），用于 sheet 单元格
	// {block_id, sheet_col, sheet_row}、slides {block_id, slide_block_type}、
	// bitable 记录 {block_id, base_record_id, base_view_id}、file 全文评论等锚点。
	Anchor map[string]any
}

// CreateNewComment 创建新评论（V2 API，支持富文本 + 局部评论）
// API: POST /open-apis/drive/v1/files/{file_token}/new_comments
// body: {file_type, reply_elements, [anchor.block_id]}
// 返回原始 data 字段（含 comment_id、create_time、is_whole 等）
func CreateNewComment(req CreateNewCommentReq, userAccessToken string) (json.RawMessage, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	body := map[string]any{
		"file_type":      req.FileType,
		"reply_elements": req.ReplyElements,
	}
	if req.Anchor != nil {
		body["anchor"] = req.Anchor
	} else if req.BlockID != "" {
		body["anchor"] = map[string]any{
			"block_id": req.BlockID,
		}
	}

	apiPath := fmt.Sprintf("/open-apis/drive/v1/files/%s/new_comments", url.PathEscape(req.FileToken))

	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := client.Post(Context(), apiPath, body, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("创建评论失败: %w", err)
	}
	// 先解析业务信封再看 HTTP 状态：飞书业务错误（如 1069302 内容超长、1069303 无权限）常随 HTTP 400 下发
	if err := CheckAPIResponse("创建评论", resp); err != nil {
		return nil, err
	}

	var apiResp struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return apiResp.Data, nil
}
