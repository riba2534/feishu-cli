package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ============================================================================
// 任务 P1 能力：分组（sections）、相关任务、设置父任务、清单搜索
// 对齐官方 lark-task 的 sections / +get-related-tasks / +set-ancestor / +tasklist-search。
// ============================================================================

// TaskSection 任务分组
type TaskSection struct {
	Guid         string `json:"guid"`
	Name         string `json:"name"`
	IsDefault    bool   `json:"is_default,omitempty"`
	ResourceType string `json:"resource_type,omitempty"`
	ResourceID   string `json:"resource_id,omitempty"`
}

// TaskPage 通用任务分页结果
type TaskPage struct {
	Tasks     []*TaskInfo `json:"tasks"`
	PageToken string      `json:"page_token,omitempty"`
	HasMore   bool        `json:"has_more"`
}

// taskGet 发起 GET 并解析业务信封到 out（data 字段）
func taskGet(apiPath string, query url.Values, userAccessToken, action string, out any) error {
	cli, err := GetClient()
	if err != nil {
		return err
	}
	if len(query) > 0 {
		apiPath += "?" + query.Encode()
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Get(Context(), apiPath, nil, tokenType, opts...)
	if err != nil {
		return fmt.Errorf("%s失败: %w", action, err)
	}
	if err := CheckAPIResponse(action, resp); err != nil {
		return err
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &env); err != nil {
		return fmt.Errorf("解析%s响应失败: %w", action, err)
	}
	if out != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("解析%s响应失败: %w", action, err)
		}
	}
	return nil
}

// taskPost 发起 POST 并解析业务信封到 out（data 字段）
func taskPost(apiPath string, body any, userAccessToken, action string, out any) error {
	cli, err := GetClient()
	if err != nil {
		return err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Post(Context(), apiPath, body, tokenType, opts...)
	if err != nil {
		return fmt.Errorf("%s失败: %w", action, err)
	}
	if err := CheckAPIResponse(action, resp); err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &env); err != nil {
		return fmt.Errorf("解析%s响应失败: %w", action, err)
	}
	if len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("解析%s响应失败: %w", action, err)
		}
	}
	return nil
}

// rawTaskSummary 列表接口返回的任务摘要（字段子集）
type rawTaskSummary struct {
	Guid        string `json:"guid"`
	Summary     string `json:"summary"`
	CompletedAt string `json:"completed_at"`
	Due         *struct {
		Timestamp string `json:"timestamp"`
		IsAllDay  bool   `json:"is_all_day"`
	} `json:"due"`
	Creator *struct {
		ID string `json:"id"`
	} `json:"creator"`
	Members []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Role string `json:"role"`
	} `json:"members"`
	URL            string `json:"url"`
	ParentTaskGuid string `json:"parent_task_guid"`
}

func (r *rawTaskSummary) toInfo() *TaskInfo {
	info := &TaskInfo{Guid: r.Guid, Summary: r.Summary, URL: r.URL, ParentTaskGuid: r.ParentTaskGuid}
	if r.Due != nil {
		if ts, err := strconv.ParseInt(r.Due.Timestamp, 10, 64); err == nil && ts > 0 {
			if r.Due.IsAllDay {
				info.DueTime = time.UnixMilli(ts).UTC().Format("2006-01-02")
				info.DueIsAllDay = true
			} else {
				info.DueTime = time.UnixMilli(ts).Format("2006-01-02 15:04:05")
			}
		}
	}
	if r.CompletedAt != "" && r.CompletedAt != "0" {
		if ts, err := strconv.ParseInt(r.CompletedAt, 10, 64); err == nil && ts > 0 {
			info.CompletedAt = time.UnixMilli(ts).Format("2006-01-02 15:04:05")
		}
	}
	if r.Creator != nil {
		info.Creator = r.Creator.ID
	}
	for _, m := range r.Members {
		info.Members = append(info.Members, TaskMemberInfo{ID: m.ID, Type: m.Type, Role: m.Role})
	}
	return info
}

// ListTaskSections 列出分组（GET /task/v2/sections）。resourceType: my_tasks | tasklist
func ListTaskSections(resourceType, resourceID, pageToken string, pageSize int, userAccessToken string) ([]*TaskSection, string, bool, error) {
	q := url.Values{}
	q.Set("resource_type", resourceType)
	if resourceID != "" {
		q.Set("resource_id", resourceID)
	}
	q.Set("user_id_type", "open_id")
	if pageSize > 0 {
		q.Set("page_size", strconv.Itoa(pageSize))
	}
	if pageToken != "" {
		q.Set("page_token", pageToken)
	}
	var data struct {
		Items     []*TaskSection `json:"items"`
		PageToken string         `json:"page_token"`
		HasMore   bool           `json:"has_more"`
	}
	if err := taskGet("/open-apis/task/v2/sections", q, userAccessToken, "列出任务分组", &data); err != nil {
		return nil, "", false, err
	}
	for _, s := range data.Items {
		if s != nil && s.ResourceType == "" {
			s.ResourceType = resourceType
		}
	}
	return data.Items, data.PageToken, data.HasMore, nil
}

// ListSectionTasks 列出分组内的任务（GET /task/v2/sections/{guid}/tasks）
func ListSectionTasks(sectionGuid string, completed *bool, pageToken string, pageSize int, userAccessToken string) (*TaskPage, error) {
	q := url.Values{}
	q.Set("user_id_type", "open_id")
	if pageSize > 0 {
		q.Set("page_size", strconv.Itoa(pageSize))
	}
	if pageToken != "" {
		q.Set("page_token", pageToken)
	}
	if completed != nil {
		q.Set("completed", strconv.FormatBool(*completed))
	}
	var data struct {
		Items     []*rawTaskSummary `json:"items"`
		PageToken string            `json:"page_token"`
		HasMore   bool              `json:"has_more"`
	}
	if err := taskGet("/open-apis/task/v2/sections/"+url.PathEscape(sectionGuid)+"/tasks", q, userAccessToken, "列出分组任务", &data); err != nil {
		return nil, err
	}
	page := &TaskPage{Tasks: make([]*TaskInfo, 0, len(data.Items)), PageToken: data.PageToken, HasMore: data.HasMore}
	for _, it := range data.Items {
		if it != nil {
			page.Tasks = append(page.Tasks, it.toInfo())
		}
	}
	return page, nil
}

// ListRelatedTasks 列出与当前用户相关的任务（GET /task/v2/task_v2/list_related_task，仅 User 身份）。
// page_token 是任务 updated_at 的微秒游标。
func ListRelatedTasks(includeCompleted bool, pageToken string, pageSize int, userAccessToken string) (*TaskPage, error) {
	q := url.Values{}
	q.Set("user_id_type", "open_id")
	if pageSize > 0 {
		q.Set("page_size", strconv.Itoa(pageSize))
	}
	if !includeCompleted {
		q.Set("completed", "false")
	}
	if pageToken != "" {
		q.Set("page_token", pageToken)
	}
	var data struct {
		Items     []*rawTaskSummary `json:"items"`
		PageToken string            `json:"page_token"`
		HasMore   bool              `json:"has_more"`
	}
	if err := taskGet("/open-apis/task/v2/task_v2/list_related_task", q, userAccessToken, "列出相关任务", &data); err != nil {
		return nil, err
	}
	page := &TaskPage{Tasks: make([]*TaskInfo, 0, len(data.Items)), PageToken: data.PageToken, HasMore: data.HasMore}
	for _, it := range data.Items {
		if it != nil {
			page.Tasks = append(page.Tasks, it.toInfo())
		}
	}
	return page, nil
}

// SetTaskAncestor 设置父任务（POST /task/v2/tasks/{guid}/set_ancestor_task）；ancestorGuid 为空表示解除父任务
func SetTaskAncestor(taskGuid, ancestorGuid string, userAccessToken string) error {
	body := map[string]any{}
	if strings.TrimSpace(ancestorGuid) != "" {
		body["ancestor_guid"] = strings.TrimSpace(ancestorGuid)
	}
	return taskPost("/open-apis/task/v2/tasks/"+url.PathEscape(taskGuid)+"/set_ancestor_task?user_id_type=open_id", body, userAccessToken, "设置父任务", nil)
}

// TasklistSearchOptions 清单搜索条件
type TasklistSearchOptions struct {
	Query      string
	CreatorIDs []string
	PageToken  string
	PageSize   int
}

// SearchTasklists 搜索清单（POST /task/v2/tasklists/search），命中后逐个拉清单详情
func SearchTasklists(opts TasklistSearchOptions, userAccessToken string) ([]*TasklistInfo, string, bool, error) {
	if strings.TrimSpace(opts.Query) == "" && len(opts.CreatorIDs) == 0 {
		return nil, "", false, fmt.Errorf("至少提供关键词或创建人之一")
	}
	body := map[string]any{"query": opts.Query}
	if len(opts.CreatorIDs) > 0 {
		body["filter"] = map[string]any{"user_id": opts.CreatorIDs}
	}
	q := url.Values{}
	if opts.PageSize > 0 {
		q.Set("page_size", strconv.Itoa(opts.PageSize))
	}
	if opts.PageToken != "" {
		q.Set("page_token", opts.PageToken)
	}
	apiPath := "/open-apis/task/v2/tasklists/search"
	if len(q) > 0 {
		apiPath += "?" + q.Encode()
	}
	var data struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		PageToken string `json:"page_token"`
		HasMore   bool   `json:"has_more"`
	}
	if err := taskPost(apiPath, body, userAccessToken, "搜索任务清单", &data); err != nil {
		return nil, "", false, err
	}
	out := make([]*TasklistInfo, 0, len(data.Items))
	for _, it := range data.Items {
		if it.ID == "" {
			continue
		}
		tl, err := GetTasklist(it.ID, userAccessToken)
		if err != nil || tl == nil {
			// 详情不可见时降级为只给 GUID，不中断搜索
			out = append(out, &TasklistInfo{Guid: it.ID})
			continue
		}
		out = append(out, tl)
	}
	return out, data.PageToken, data.HasMore, nil
}

// ParseTaskGUID 接受任务 GUID 或任务 applink（取 guid= 参数）；界面上的任务编号（如 t123456）不是 GUID
func ParseTaskGUID(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("任务 ID 为空")
	}
	lower := strings.ToLower(input)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		u, err := url.Parse(input)
		if err != nil {
			return "", fmt.Errorf("无效的任务链接: %v", err)
		}
		if guid := strings.TrimSpace(u.Query().Get("guid")); guid != "" {
			return guid, nil
		}
		return "", fmt.Errorf("任务链接中没有 guid 参数；请传任务 GUID 或带 guid= 的任务 applink")
	}
	if isTaskDisplayNumber(input) {
		return "", fmt.Errorf("%q 是任务界面编号，不是任务 GUID；请用 task search / task get 的 guid 或任务 applink", input)
	}
	return input, nil
}

// isTaskDisplayNumber 界面编号形如 t123456（字母 t + 纯数字）
func isTaskDisplayNumber(s string) bool {
	if len(s) < 2 || (s[0] != 't' && s[0] != 'T') {
		return false
	}
	for _, r := range s[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
