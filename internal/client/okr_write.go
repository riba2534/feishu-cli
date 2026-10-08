package client

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strings"
)

// ============================================================================
// OKR v2 目标 / 关键结果写入与评论（对齐官方 okr +create / +patch / +comment-*）。
// v2 的 ContentBlock 使用 snake_case 键（block_element_type / paragraph_element_type / text_run），
// 与 v1 进展记录的 camelCase 结构不同，不能混用。
// ============================================================================

// OKRV2TextContent 把纯文本包装为 v2 ContentBlock（每行一个段落）
func OKRV2TextContent(text string) map[string]any {
	var blocks []any
	for _, line := range strings.Split(text, "\n") {
		blocks = append(blocks, map[string]any{
			"block_element_type": "paragraph",
			"paragraph": map[string]any{
				"elements": []any{map[string]any{
					"paragraph_element_type": "textRun",
					"text_run":               map[string]any{"text": line},
				}},
			},
		})
	}
	return map[string]any{"blocks": blocks}
}

// ParseOKRV2Content 纯文本（text）与原始 v2 ContentBlock JSON（raw）二选一；都为空返回 nil
func ParseOKRV2Content(text, raw, flagName string) (map[string]any, error) {
	text, raw = strings.TrimSpace(text), strings.TrimSpace(raw)
	if text != "" && raw != "" {
		return nil, fmt.Errorf("--%s 与 --%s-json 只能填一个", flagName, flagName)
	}
	if raw != "" {
		var v map[string]any
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, fmt.Errorf("--%s-json 不是合法 JSON 对象: %w", flagName, err)
		}
		if _, ok := v["blocks"]; !ok {
			return nil, fmt.Errorf("--%s-json 需要 v2 ContentBlock 结构 {\"blocks\":[...]}", flagName)
		}
		return v, nil
	}
	if text != "" {
		return OKRV2TextContent(text), nil
	}
	return nil, nil
}

// OKRWriteRequest 一次 OKR 写请求（dry-run 与执行共用）
type OKRWriteRequest struct {
	Method string         `json:"method"`
	Path   string         `json:"path"`
	Query  map[string]any `json:"params,omitempty"`
	Body   map[string]any `json:"body,omitempty"`
}

// BuildOKRCreateObjective 构造创建目标请求
func BuildOKRCreateObjective(cycleID string, content, notes map[string]any, categoryID, userIDType string) (*OKRWriteRequest, error) {
	if strings.TrimSpace(cycleID) == "" {
		return nil, fmt.Errorf("缺少 cycle_id（用 okr cycle list 获取用户周期 ID）")
	}
	if content == nil {
		return nil, fmt.Errorf("缺少目标内容（--content 或 --content-json）")
	}
	body := map[string]any{"content": content}
	if notes != nil {
		body["notes"] = notes
	}
	if strings.TrimSpace(categoryID) != "" {
		body["category_id"] = strings.TrimSpace(categoryID)
	}
	return &OKRWriteRequest{
		Method: "POST",
		Path:   "/open-apis/okr/v2/cycles/" + url.PathEscape(cycleID) + "/objectives",
		Query:  map[string]any{"user_id_type": okrUserIDType(userIDType)},
		Body:   body,
	}, nil
}

// BuildOKRCreateKeyResult 构造创建关键结果请求
func BuildOKRCreateKeyResult(objectiveID string, content map[string]any, userIDType string) (*OKRWriteRequest, error) {
	if strings.TrimSpace(objectiveID) == "" {
		return nil, fmt.Errorf("缺少 objective_id")
	}
	if content == nil {
		return nil, fmt.Errorf("缺少关键结果内容（--content 或 --content-json）")
	}
	return &OKRWriteRequest{
		Method: "POST",
		Path:   "/open-apis/okr/v2/objectives/" + url.PathEscape(objectiveID) + "/key_results",
		Query:  map[string]any{"user_id_type": okrUserIDType(userIDType)},
		Body:   map[string]any{"content": content},
	}, nil
}

// OKRPatchFields 目标 / 关键结果的可更新字段（nil 表示不改）
type OKRPatchFields struct {
	Content  map[string]any
	Notes    map[string]any // 仅目标
	Score    *float64       // 0-1，最多一位小数
	Deadline *int64         // 毫秒时间戳
}

// BuildOKRPatch 构造更新请求；level 为 objective | key-result
func BuildOKRPatch(level, targetID string, f OKRPatchFields, userIDType string) (*OKRWriteRequest, error) {
	if strings.TrimSpace(targetID) == "" {
		return nil, fmt.Errorf("缺少目标 ID")
	}
	body := map[string]any{}
	if f.Content != nil {
		body["content"] = f.Content
	}
	if f.Notes != nil {
		if level != "objective" {
			return nil, fmt.Errorf("--notes 只适用于目标（objective）")
		}
		body["notes"] = f.Notes
	}
	if f.Score != nil {
		s := *f.Score
		if s < 0 || s > 1 || math.Abs(s*10-math.Round(s*10)) > 1e-9 {
			return nil, fmt.Errorf("--score 必须在 0-1 之间且最多一位小数（如 0.5）")
		}
		body["score"] = s
	}
	if f.Deadline != nil {
		if *f.Deadline < 1_000_000_000_000 {
			return nil, fmt.Errorf("--deadline 需要毫秒时间戳（13 位）或日期")
		}
		body["deadline"] = fmt.Sprint(*f.Deadline)
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("没有要更新的字段（--content / --notes / --score / --deadline）")
	}
	var path string
	switch level {
	case "objective":
		path = "/open-apis/okr/v2/objectives/" + url.PathEscape(targetID)
	case "key-result":
		path = "/open-apis/okr/v2/key_results/" + url.PathEscape(targetID)
	default:
		return nil, fmt.Errorf("未知层级 %q", level)
	}
	return &OKRWriteRequest{Method: "PATCH", Path: path, Query: map[string]any{"user_id_type": okrUserIDType(userIDType)}, Body: body}, nil
}

// OKRCommentCreate 创建评论参数
type OKRCommentCreate struct {
	TargetType   string // cycle | progress | objective | key_result
	TargetID     string
	Content      map[string]any
	SelectedText string
	SelectAll    bool
	RefCommentID string
	PlainText    string // --select-all 时用于生成通配选区
}

// BuildOKRCommentCreate 构造评论请求：目标/关键结果评论必须且只能指定选区（--selected-text / --select-all / --ref-comment-id 之一）
func BuildOKRCommentCreate(c OKRCommentCreate, userIDType string) (*OKRWriteRequest, error) {
	switch c.TargetType {
	case "cycle", "progress", "objective", "key_result":
	default:
		return nil, fmt.Errorf("--target-type 仅支持 cycle | progress | objective | key_result")
	}
	if strings.TrimSpace(c.TargetID) == "" {
		return nil, fmt.Errorf("缺少 --target-id")
	}
	if c.Content == nil {
		return nil, fmt.Errorf("缺少评论内容（--content 或 --content-json）")
	}
	n := 0
	for _, set := range []bool{c.SelectedText != "", c.SelectAll, c.RefCommentID != ""} {
		if set {
			n++
		}
	}
	if c.TargetType == "objective" || c.TargetType == "key_result" {
		if n != 1 {
			return nil, fmt.Errorf("目标/关键结果评论必须且只能指定 --selected-text、--select-all、--ref-comment-id 之一")
		}
	} else if c.SelectedText != "" || c.SelectAll {
		return nil, fmt.Errorf("--selected-text / --select-all 只适用于 objective / key_result 评论")
	}
	body := map[string]any{
		"target":  map[string]any{"target_type": c.TargetType, "target_id": c.TargetID},
		"content": c.Content,
	}
	if c.SelectedText != "" {
		body["selected_text"] = c.SelectedText
	}
	if c.SelectAll {
		body["selected_text"] = strings.Repeat("*", len([]rune(c.PlainText)))
	}
	if c.RefCommentID != "" {
		body["ref_comment_id"] = c.RefCommentID
	}
	return &OKRWriteRequest{Method: "POST", Path: "/open-apis/okr/v2/comments", Query: map[string]any{"user_id_type": okrUserIDType(userIDType)}, Body: body}, nil
}

func okrUserIDType(t string) string {
	if strings.TrimSpace(t) == "" {
		return "open_id"
	}
	return t
}

// DoOKRWrite 执行写请求，返回 data 原文
func DoOKRWrite(req *OKRWriteRequest, action, userAccessToken string) (json.RawMessage, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	for k, v := range req.Query {
		q.Set(k, fmt.Sprint(v))
	}
	path := req.Path
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	var body any = req.Body
	switch req.Method {
	case "POST":
		resp, err := cli.Post(Context(), path, body, tokenType, opts...)
		if err != nil {
			return nil, fmt.Errorf("%s失败: %w", action, err)
		}
		if err := CheckAPIResponse(action, resp); err != nil {
			return nil, err
		}
		return okrDataField(resp.RawBody)
	case "PATCH":
		resp, err := cli.Patch(Context(), path, body, tokenType, opts...)
		if err != nil {
			return nil, fmt.Errorf("%s失败: %w", action, err)
		}
		if err := CheckAPIResponse(action, resp); err != nil {
			return nil, err
		}
		return okrDataField(resp.RawBody)
	}
	return nil, fmt.Errorf("不支持的方法 %s", req.Method)
}

func okrDataField(raw []byte) (json.RawMessage, error) {
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return env.Data, nil
}

// ListOKRComments 列出评论（GET /okr/v2/comments），返回原始 items 与分页
func ListOKRComments(targetType, targetID, pageToken string, pageSize int, userIDType, userAccessToken string) ([]json.RawMessage, string, bool, error) {
	q := url.Values{}
	q.Set("target_type", targetType)
	q.Set("target_id", targetID)
	q.Set("user_id_type", okrUserIDType(userIDType))
	if pageSize > 0 {
		q.Set("page_size", fmt.Sprint(pageSize))
	}
	if pageToken != "" {
		q.Set("page_token", pageToken)
	}
	var data struct {
		Items     []json.RawMessage `json:"items"`
		PageToken string            `json:"page_token"`
		HasMore   bool              `json:"has_more"`
	}
	if err := taskGet("/open-apis/okr/v2/comments", q, userAccessToken, "查询 OKR 评论", &data); err != nil {
		return nil, "", false, err
	}
	return data.Items, data.PageToken, data.HasMore, nil
}
