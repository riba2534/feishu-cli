package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	larktask "github.com/larksuite/oapi-sdk-go/v3/service/task/v2"
)

// ============================================================================
// 任务原始请求：task create 的成员/清单/幂等键/--data、task update --data、
// 任务评论 get/update/delete 需要发送 SDK 结构体之外的任意 JSON，统一走手写请求。
// ============================================================================

// TaskAPIBase 任务 v2 接口前缀
const TaskAPIBase = "/open-apis/task/v2"

// taskRequest 按 method 发起请求并解析业务信封到 out（data 字段）。
func taskRequest(method, apiPath string, query url.Values, body any, userAccessToken, action string, out any) error {
	cli, err := GetClient()
	if err != nil {
		return err
	}
	if len(query) > 0 {
		apiPath += "?" + query.Encode()
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	switch method {
	case http.MethodGet:
		r, e := cli.Get(Context(), apiPath, nil, tokenType, opts...)
		if e != nil {
			return fmt.Errorf("%s失败: %w", action, e)
		}
		return decodeTaskEnvelope(action, r.StatusCode, r.Header, r.RawBody, out)
	case http.MethodPost:
		r, e := cli.Post(Context(), apiPath, body, tokenType, opts...)
		if e != nil {
			return fmt.Errorf("%s失败: %w", action, e)
		}
		return decodeTaskEnvelope(action, r.StatusCode, r.Header, r.RawBody, out)
	case http.MethodPatch:
		r, e := cli.Patch(Context(), apiPath, body, tokenType, opts...)
		if e != nil {
			return fmt.Errorf("%s失败: %w", action, e)
		}
		return decodeTaskEnvelope(action, r.StatusCode, r.Header, r.RawBody, out)
	case http.MethodDelete:
		r, e := cli.Delete(Context(), apiPath, nil, tokenType, opts...)
		if e != nil {
			return fmt.Errorf("%s失败: %w", action, e)
		}
		return decodeTaskEnvelope(action, r.StatusCode, r.Header, r.RawBody, out)
	default:
		return fmt.Errorf("%s失败: 不支持的 HTTP 方法 %s", action, method)
	}
}

// decodeTaskEnvelope 先解析业务信封（飞书业务错误可能随 HTTP 400 下发），再把 data 解到 out。
func decodeTaskEnvelope(action string, status int, header http.Header, raw []byte, out any) error {
	if err := ParseAPIResponse(action, status, header, raw); err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("解析%s响应失败: %w", action, err)
	}
	if len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("解析%s响应失败: %w", action, err)
		}
	}
	return nil
}

func openIDQuery() url.Values {
	return url.Values{"user_id_type": []string{"open_id"}}
}

// CreateTaskWithBody 以任意 JSON 请求体创建任务（POST /task/v2/tasks）。
func CreateTaskWithBody(body map[string]any, userAccessToken string) (*TaskInfo, error) {
	var data struct {
		Task *larktask.Task `json:"task"`
	}
	if err := taskRequest(http.MethodPost, TaskAPIBase+"/tasks", openIDQuery(), body, userAccessToken, "创建任务", &data); err != nil {
		return nil, err
	}
	if data.Task == nil {
		return nil, fmt.Errorf("创建任务成功但未返回数据")
	}
	return taskToInfo(data.Task), nil
}

// PatchTaskWithBody 以任意 JSON 请求体更新任务（PATCH /task/v2/tasks/{guid}，body 含 task 与 update_fields）。
func PatchTaskWithBody(taskGuid string, body map[string]any, userAccessToken string) (*TaskInfo, error) {
	var data struct {
		Task *larktask.Task `json:"task"`
	}
	if err := taskRequest(http.MethodPatch, TaskAPIBase+"/tasks/"+url.PathEscape(taskGuid), openIDQuery(), body, userAccessToken, "更新任务", &data); err != nil {
		return nil, err
	}
	if data.Task == nil {
		return nil, fmt.Errorf("更新任务成功但未返回数据")
	}
	return taskToInfo(data.Task), nil
}

// GetTaskComment 获取任务评论详情（GET /task/v2/comments/{comment_id}）。
func GetTaskComment(commentID, userAccessToken string) (*CommentInfo, error) {
	var data struct {
		Comment *larktask.Comment `json:"comment"`
	}
	if err := taskRequest(http.MethodGet, TaskAPIBase+"/comments/"+url.PathEscape(commentID), openIDQuery(), nil, userAccessToken, "获取任务评论", &data); err != nil {
		return nil, err
	}
	if data.Comment == nil {
		return nil, fmt.Errorf("获取任务评论返回数据为空")
	}
	return commentToInfo(data.Comment), nil
}

// TaskCommentUpdateBody 构造评论更新请求体（当前接口只支持更新 content）。
func TaskCommentUpdateBody(content string) map[string]any {
	return map[string]any{
		"comment":       map[string]any{"content": content},
		"update_fields": []string{"content"},
	}
}

// UpdateTaskComment 更新任务评论内容（PATCH /task/v2/comments/{comment_id}）。
func UpdateTaskComment(commentID, content, userAccessToken string) (*CommentInfo, error) {
	var data struct {
		Comment *larktask.Comment `json:"comment"`
	}
	if err := taskRequest(http.MethodPatch, TaskAPIBase+"/comments/"+url.PathEscape(commentID), openIDQuery(), TaskCommentUpdateBody(content), userAccessToken, "更新任务评论", &data); err != nil {
		return nil, err
	}
	if data.Comment == nil {
		return nil, fmt.Errorf("更新任务评论成功但未返回数据")
	}
	return commentToInfo(data.Comment), nil
}

// DeleteTaskComment 删除任务评论（DELETE /task/v2/comments/{comment_id}）。
func DeleteTaskComment(commentID, userAccessToken string) error {
	return taskRequest(http.MethodDelete, TaskAPIBase+"/comments/"+url.PathEscape(commentID), nil, nil, userAccessToken, "删除任务评论", nil)
}
