package client

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// wikiMoveToDriveTaskType 是查询 move_wiki_to_docs 异步任务状态时固定的 task_type。
const wikiMoveToDriveTaskType = "move_wiki_to_docs"

// move_wiki_to_docs 任务的状态码由后端固定：0 成功、1 处理中、-1 失败。
const (
	WikiMoveToDriveStatusSuccess    = 0
	WikiMoveToDriveStatusProcessing = 1
	WikiMoveToDriveStatusFailure    = -1
)

// WikiMoveToDriveTaskStatus 表示 move_wiki_to_docs 异步任务的一次状态快照。
type WikiMoveToDriveTaskStatus struct {
	TaskID    string
	Status    int
	StatusMsg string
	ObjToken  string
	ObjType   string
	URL       string
}

// Ready 表示任务已成功完成。
func (s WikiMoveToDriveTaskStatus) Ready() bool { return s.Status == WikiMoveToDriveStatusSuccess }

// Failed 表示任务以失败状态结束（状态码 < 0）。
func (s WikiMoveToDriveTaskStatus) Failed() bool { return s.Status < WikiMoveToDriveStatusSuccess }

// MoveWikiNodeToDrive 将知识库节点移出知识空间，转存到云盘文件夹。
//
// 底层接口 POST /open-apis/wiki/v2/nodes/{node_token}/move_wiki_to_docs 始终异步执行，
// 返回 task_id 供轮询 GetMoveWikiToDriveTask。folderToken 为空表示移动到调用方个人空间根目录。
//
// SDK v3.5.3 未封装该反向接口（只有正向 MoveDocsToWiki），故直接走通用 HTTP。
func MoveWikiNodeToDrive(nodeToken, folderToken, userAccessToken string) (string, error) {
	c, err := GetClient()
	if err != nil {
		return "", err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	apiPath := fmt.Sprintf("/open-apis/wiki/v2/nodes/%s/move_wiki_to_docs", nodeToken)
	body := map[string]interface{}{}
	if folderToken != "" {
		body["folder_token"] = folderToken
	}
	resp, err := c.Post(Context(), apiPath, body, tokenType, opts...)
	if err != nil {
		return "", fmt.Errorf("移动知识库节点到云盘失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("移动知识库节点到云盘失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &parsed); err != nil {
		return "", fmt.Errorf("移动知识库节点响应解析失败: %w", err)
	}
	if parsed.Code != 0 {
		return "", fmt.Errorf("移动知识库节点到云盘失败: code=%d, msg=%s", parsed.Code, parsed.Msg)
	}
	if parsed.Data.TaskID == "" {
		return "", fmt.Errorf("移动知识库节点响应缺少 task_id")
	}
	return parsed.Data.TaskID, nil
}

// GetMoveWikiToDriveTask 查询 move_wiki_to_docs 异步任务的当前状态。
//
// 响应中 move_wiki_to_docs_result 缺省（任务尚未产出结果）时按“处理中”对待，
// 避免用零值 status(0) 误判为成功。
func GetMoveWikiToDriveTask(taskID, userAccessToken string) (*WikiMoveToDriveTaskStatus, error) {
	c, err := GetClient()
	if err != nil {
		return nil, err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	apiPath := fmt.Sprintf("/open-apis/wiki/v2/tasks/%s?task_type=%s", taskID, wikiMoveToDriveTaskType)
	resp, err := c.Get(Context(), apiPath, nil, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("查询 move_wiki_to_docs 任务失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("查询 move_wiki_to_docs 任务失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Task struct {
				TaskID string `json:"task_id"`
				Result *struct {
					Status    *int   `json:"status"`
					StatusMsg string `json:"status_msg"`
					ObjToken  string `json:"obj_token"`
					ObjType   string `json:"obj_type"`
					URL       string `json:"url"`
				} `json:"move_wiki_to_docs_result"`
			} `json:"task"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &parsed); err != nil {
		return nil, fmt.Errorf("查询 move_wiki_to_docs 任务响应解析失败: %w", err)
	}
	if parsed.Code != 0 {
		return nil, fmt.Errorf("查询 move_wiki_to_docs 任务失败: code=%d, msg=%s", parsed.Code, parsed.Msg)
	}

	taskIDOut := parsed.Data.Task.TaskID
	if taskIDOut == "" {
		taskIDOut = taskID
	}
	status := &WikiMoveToDriveTaskStatus{
		TaskID: taskIDOut,
		Status: WikiMoveToDriveStatusProcessing,
	}
	if r := parsed.Data.Task.Result; r != nil {
		if r.Status != nil {
			status.Status = *r.Status
		}
		status.StatusMsg = r.StatusMsg
		status.ObjToken = r.ObjToken
		status.ObjType = r.ObjType
		status.URL = r.URL
	}
	return status, nil
}
