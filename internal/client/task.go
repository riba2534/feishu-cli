package client

import (
	"fmt"
	"strconv"
	"time"

	larktask "github.com/larksuite/oapi-sdk-go/v3/service/task/v2"
)

// TaskInfo represents simplified task information
type TaskInfo struct {
	Guid        string `json:"guid"`
	Summary     string `json:"summary"`
	Description string `json:"description,omitempty"`
	DueTime     string `json:"due_time,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	Creator     string `json:"creator,omitempty"`
	OriginHref  string `json:"origin_href,omitempty"`
}

// CreateTaskOptions represents options for creating a task
type CreateTaskOptions struct {
	Summary        string
	Description    string
	DueTimestamp   int64  // Unix milliseconds
	OriginHref     string // URL for task origin
	OriginPlatform string // Platform name for origin
}

// UpdateTaskOptions represents options for updating a task
type UpdateTaskOptions struct {
	Summary      string
	Description  string
	DueTimestamp int64 // Unix milliseconds, 0 means no change
	CompletedAt  int64 // Unix milliseconds, 0 means not completed
	Completed    bool  // true to mark as completed
}

// CreateTask creates a new task
func CreateTask(opts CreateTaskOptions) (*TaskInfo, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	taskBuilder := larktask.NewInputTaskBuilder().
		Summary(opts.Summary)

	if opts.Description != "" {
		taskBuilder.Description(opts.Description)
	}

	if opts.DueTimestamp > 0 {
		due := larktask.NewDueBuilder().
			Timestamp(strconv.FormatInt(opts.DueTimestamp, 10)).
			IsAllDay(false).
			Build()
		taskBuilder.Due(due)
	}

	if opts.OriginHref != "" {
		href := larktask.NewHrefBuilder().
			Url(opts.OriginHref).
			Build()

		platformName := opts.OriginPlatform
		if platformName == "" {
			platformName = "feishu-cli"
		}
		i18nName := larktask.NewI18nTextBuilder().
			ZhCn(platformName).
			EnUs(platformName).
			Build()

		origin := larktask.NewOriginBuilder().
			PlatformI18nName(i18nName).
			Href(href).
			Build()
		taskBuilder.Origin(origin)
	}

	req := larktask.NewCreateTaskReqBuilder().
		UserIdType("open_id").
		InputTask(taskBuilder.Build()).
		Build()

	resp, err := client.Task.V2.Task.Create(Context(), req)
	if err != nil {
		return nil, fmt.Errorf("创建任务失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("创建任务失败: %s (code: %d)", resp.Msg, resp.Code)
	}

	return taskToInfo(resp.Data.Task), nil
}

// GetTask retrieves task details by ID
func GetTask(taskGuid string) (*TaskInfo, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larktask.NewGetTaskReqBuilder().
		TaskGuid(taskGuid).
		UserIdType("open_id").
		Build()

	resp, err := client.Task.V2.Task.Get(Context(), req)
	if err != nil {
		return nil, fmt.Errorf("获取任务失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取任务失败: %s (code: %d)", resp.Msg, resp.Code)
	}

	return taskToInfo(resp.Data.Task), nil
}

// ListTasksResult represents the result of listing tasks
type ListTasksResult struct {
	Tasks     []*TaskInfo `json:"tasks"`
	PageToken string      `json:"page_token,omitempty"`
	HasMore   bool        `json:"has_more"`
}

// ListTasks retrieves a list of tasks
func ListTasks(pageSize int, pageToken string, completed *bool) (*ListTasksResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	reqBuilder := larktask.NewListTaskReqBuilder().
		UserIdType("open_id")

	if pageSize > 0 {
		reqBuilder.PageSize(pageSize)
	}

	if pageToken != "" {
		reqBuilder.PageToken(pageToken)
	}

	if completed != nil {
		reqBuilder.Completed(*completed)
	}

	req := reqBuilder.Build()

	resp, err := client.Task.V2.Task.List(Context(), req)
	if err != nil {
		return nil, fmt.Errorf("获取任务列表失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取任务列表失败: %s (code: %d)", resp.Msg, resp.Code)
	}

	result := &ListTasksResult{
		Tasks:   make([]*TaskInfo, 0, len(resp.Data.Items)),
		HasMore: resp.Data.HasMore != nil && *resp.Data.HasMore,
	}

	if resp.Data.PageToken != nil {
		result.PageToken = *resp.Data.PageToken
	}

	for _, task := range resp.Data.Items {
		result.Tasks = append(result.Tasks, taskToInfo(task))
	}

	return result, nil
}

// UpdateTask updates an existing task
func UpdateTask(taskGuid string, opts UpdateTaskOptions) (*TaskInfo, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	taskBuilder := larktask.NewInputTaskBuilder()
	updateFields := make([]string, 0)

	if opts.Summary != "" {
		taskBuilder.Summary(opts.Summary)
		updateFields = append(updateFields, "summary")
	}

	if opts.Description != "" {
		taskBuilder.Description(opts.Description)
		updateFields = append(updateFields, "description")
	}

	if opts.DueTimestamp > 0 {
		due := larktask.NewDueBuilder().
			Timestamp(strconv.FormatInt(opts.DueTimestamp, 10)).
			IsAllDay(false).
			Build()
		taskBuilder.Due(due)
		updateFields = append(updateFields, "due")
	}

	if opts.Completed {
		// Set completed_at to current time
		now := time.Now().UnixMilli()
		taskBuilder.CompletedAt(strconv.FormatInt(now, 10))
		updateFields = append(updateFields, "completed_at")
	} else if opts.CompletedAt > 0 {
		taskBuilder.CompletedAt(strconv.FormatInt(opts.CompletedAt, 10))
		updateFields = append(updateFields, "completed_at")
	}

	if len(updateFields) == 0 {
		return nil, fmt.Errorf("没有要更新的字段")
	}

	body := larktask.NewPatchTaskReqBodyBuilder().
		Task(taskBuilder.Build()).
		UpdateFields(updateFields).
		Build()

	req := larktask.NewPatchTaskReqBuilder().
		TaskGuid(taskGuid).
		UserIdType("open_id").
		Body(body).
		Build()

	resp, err := client.Task.V2.Task.Patch(Context(), req)
	if err != nil {
		return nil, fmt.Errorf("更新任务失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("更新任务失败: %s (code: %d)", resp.Msg, resp.Code)
	}

	return taskToInfo(resp.Data.Task), nil
}

// DeleteTask deletes a task by ID
func DeleteTask(taskGuid string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	req := larktask.NewDeleteTaskReqBuilder().
		TaskGuid(taskGuid).
		Build()

	resp, err := client.Task.V2.Task.Delete(Context(), req)
	if err != nil {
		return fmt.Errorf("删除任务失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("删除任务失败: %s (code: %d)", resp.Msg, resp.Code)
	}

	return nil
}

// CompleteTask marks a task as completed
func CompleteTask(taskGuid string) (*TaskInfo, error) {
	return UpdateTask(taskGuid, UpdateTaskOptions{
		Completed: true,
	})
}

// taskToInfo converts SDK Task to TaskInfo
func taskToInfo(task *larktask.Task) *TaskInfo {
	if task == nil {
		return nil
	}

	info := &TaskInfo{}

	if task.Guid != nil {
		info.Guid = *task.Guid
	}

	if task.Summary != nil {
		info.Summary = *task.Summary
	}

	if task.Description != nil {
		info.Description = *task.Description
	}

	if task.Due != nil && task.Due.Timestamp != nil {
		ts, err := strconv.ParseInt(*task.Due.Timestamp, 10, 64)
		if err == nil {
			info.DueTime = time.UnixMilli(ts).Format("2006-01-02 15:04:05")
		}
	}

	if task.CompletedAt != nil && *task.CompletedAt != "" {
		ts, err := strconv.ParseInt(*task.CompletedAt, 10, 64)
		if err == nil {
			info.CompletedAt = time.UnixMilli(ts).Format("2006-01-02 15:04:05")
		}
	}

	if task.Creator != nil && task.Creator.Id != nil {
		info.Creator = *task.Creator.Id
	}

	if task.Origin != nil && task.Origin.Href != nil && task.Origin.Href.Url != nil {
		info.OriginHref = *task.Origin.Href.Url
	}

	return info
}
