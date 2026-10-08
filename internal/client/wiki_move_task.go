package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// WikiMoveTaskResult 是 wiki move（move_docs_to_wiki）异步任务中单个文档的结果。
// status：0 成功、>0 处理中、<0 失败。
type WikiMoveTaskResult struct {
	Status    int            `json:"status"`
	StatusMsg string         `json:"status_msg,omitempty"`
	Node      map[string]any `json:"node,omitempty"`
}

// WikiMoveTaskStatus 是 GET /open-apis/wiki/v2/tasks/{task_id}?task_type=move 的归一化结果。
type WikiMoveTaskStatus struct {
	TaskID      string
	MoveResults []WikiMoveTaskResult
}

// Ready 全部文档都已成功挂载。
func (s WikiMoveTaskStatus) Ready() bool {
	if len(s.MoveResults) == 0 {
		return false
	}
	for _, r := range s.MoveResults {
		if r.Status != 0 {
			return false
		}
	}
	return true
}

// Failed 任一文档失败即视为失败。
func (s WikiMoveTaskStatus) Failed() bool {
	for _, r := range s.MoveResults {
		if r.Status < 0 {
			return true
		}
	}
	return false
}

// Primary 返回最能代表整体状态的结果：优先失败项，其次处理中，最后第一项。
func (s WikiMoveTaskStatus) Primary() *WikiMoveTaskResult {
	for i := range s.MoveResults {
		if s.MoveResults[i].Status < 0 {
			return &s.MoveResults[i]
		}
	}
	for i := range s.MoveResults {
		if s.MoveResults[i].Status > 0 {
			return &s.MoveResults[i]
		}
	}
	if len(s.MoveResults) > 0 {
		return &s.MoveResults[0]
	}
	return nil
}

// StatusLabel 人类可读状态。
func (s WikiMoveTaskStatus) StatusLabel() string {
	if p := s.Primary(); p != nil && strings.TrimSpace(p.StatusMsg) != "" {
		return p.StatusMsg
	}
	switch {
	case s.Ready():
		return "success"
	case s.Failed():
		return "failure"
	}
	return "processing"
}

// GetWikiMoveTask 查询 wiki move（move_docs_to_wiki）异步任务状态。
// 业务错误常随 HTTP 400 下发，先解析飞书信封里的 code。
func GetWikiMoveTask(taskID, userAccessToken string) (*WikiMoveTaskStatus, error) {
	c, err := GetClient()
	if err != nil {
		return nil, err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	apiPath := fmt.Sprintf("/open-apis/wiki/v2/tasks/%s?task_type=move", url.PathEscape(taskID))
	resp, err := c.Get(Context(), apiPath, nil, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("查询 wiki move 任务失败: %w", err)
	}
	if err := CheckAPIResponse("查询 wiki move 任务", resp); err != nil {
		return nil, err
	}
	var apiResp struct {
		Data struct {
			Task *struct {
				TaskID     string `json:"task_id"`
				MoveResult []struct {
					Status    int            `json:"status"`
					StatusMsg string         `json:"status_msg"`
					Node      map[string]any `json:"node"`
				} `json:"move_result"`
			} `json:"task"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析 wiki move 任务响应失败: %w", err)
	}
	if apiResp.Data.Task == nil {
		return nil, fmt.Errorf("查询 wiki move 任务失败: 响应缺少 task")
	}
	st := &WikiMoveTaskStatus{TaskID: apiResp.Data.Task.TaskID}
	if st.TaskID == "" {
		st.TaskID = taskID
	}
	for _, r := range apiResp.Data.Task.MoveResult {
		st.MoveResults = append(st.MoveResults, WikiMoveTaskResult{Status: r.Status, StatusMsg: r.StatusMsg, Node: r.Node})
	}
	return st, nil
}
