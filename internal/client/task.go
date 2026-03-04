package client

import (
	"fmt"
	"strconv"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
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
func CreateTask(opts CreateTaskOptions, userAccessToken string) (*TaskInfo, error) {
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

	resp, err := client.Task.V2.Task.Create(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
	if err != nil {
		return nil, fmt.Errorf("创建任务失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("创建任务失败: %s (code: %d)", resp.Msg, resp.Code)
	}

	if resp.Data == nil {
		return nil, fmt.Errorf("创建任务成功但未返回数据")
	}

	return taskToInfo(resp.Data.Task), nil
}

// GetTask retrieves task details by ID
func GetTask(taskGuid string, userAccessToken string) (*TaskInfo, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larktask.NewGetTaskReqBuilder().
		TaskGuid(taskGuid).
		UserIdType("open_id").
		Build()

	resp, err := client.Task.V2.Task.Get(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
	if err != nil {
		return nil, fmt.Errorf("获取任务失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取任务失败: %s (code: %d)", resp.Msg, resp.Code)
	}

	if resp.Data == nil {
		return nil, fmt.Errorf("获取任务返回数据为空")
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
func ListTasks(pageSize int, pageToken string, completed *bool, userAccessToken string) (*ListTasksResult, error) {
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

	resp, err := client.Task.V2.Task.List(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
	if err != nil {
		return nil, fmt.Errorf("获取任务列表失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取任务列表失败: %s (code: %d)", resp.Msg, resp.Code)
	}

	if resp.Data == nil {
		return &ListTasksResult{Tasks: make([]*TaskInfo, 0)}, nil
	}

	result := &ListTasksResult{
		Tasks:     make([]*TaskInfo, 0, len(resp.Data.Items)),
		HasMore:   BoolVal(resp.Data.HasMore),
		PageToken: StringVal(resp.Data.PageToken),
	}

	for _, task := range resp.Data.Items {
		result.Tasks = append(result.Tasks, taskToInfo(task))
	}

	return result, nil
}

// UpdateTask updates an existing task
func UpdateTask(taskGuid string, opts UpdateTaskOptions, userAccessToken string) (*TaskInfo, error) {
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

	resp, err := client.Task.V2.Task.Patch(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
	if err != nil {
		return nil, fmt.Errorf("更新任务失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("更新任务失败: %s (code: %d)", resp.Msg, resp.Code)
	}

	if resp.Data == nil {
		return nil, fmt.Errorf("更新任务成功但未返回数据")
	}

	return taskToInfo(resp.Data.Task), nil
}

// DeleteTask deletes a task by ID
func DeleteTask(taskGuid string, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	req := larktask.NewDeleteTaskReqBuilder().
		TaskGuid(taskGuid).
		Build()

	resp, err := client.Task.V2.Task.Delete(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
	if err != nil {
		return fmt.Errorf("删除任务失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("删除任务失败: %s (code: %d)", resp.Msg, resp.Code)
	}

	return nil
}

// CompleteTask marks a task as completed
func CompleteTask(taskGuid string, userAccessToken string) (*TaskInfo, error) {
	return UpdateTask(taskGuid, UpdateTaskOptions{
		Completed: true,
	}, userAccessToken)
}

// TasklistInfo 任务清单信息
type TasklistInfo struct {
	Guid      string `json:"guid"`
	Name      string `json:"name"`
	Creator   string `json:"creator,omitempty"`
	Owner     string `json:"owner,omitempty"`
	Url       string `json:"url,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// CreateSubtask 创建子任务
func CreateSubtask(taskGuid, summary string, userAccessToken string) (*TaskInfo, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	inputTask := larktask.NewInputTaskBuilder().
		Summary(summary).
		Build()

	req := larktask.NewCreateTaskSubtaskReqBuilder().
		TaskGuid(taskGuid).
		UserIdType("open_id").
		InputTask(inputTask).
		Build()

	resp, err := client.Task.V2.TaskSubtask.Create(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
	if err != nil {
		return nil, fmt.Errorf("创建子任务失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("创建子任务失败: %s (code: %d)", resp.Msg, resp.Code)
	}

	if resp.Data == nil {
		return nil, fmt.Errorf("创建子任务成功但未返回数据")
	}

	return taskToInfo(resp.Data.Subtask), nil
}

// ListSubtasks 列出子任务
func ListSubtasks(taskGuid string, pageSize int, pageToken string, userAccessToken string) ([]*TaskInfo, string, bool, error) {
	client, err := GetClient()
	if err != nil {
		return nil, "", false, err
	}

	reqBuilder := larktask.NewListTaskSubtaskReqBuilder().
		TaskGuid(taskGuid).
		UserIdType("open_id")

	if pageSize > 0 {
		reqBuilder.PageSize(pageSize)
	}
	if pageToken != "" {
		reqBuilder.PageToken(pageToken)
	}

	resp, err := client.Task.V2.TaskSubtask.List(Context(), reqBuilder.Build(), larkcore.WithUserAccessToken(userAccessToken))
	if err != nil {
		return nil, "", false, fmt.Errorf("获取子任务列表失败: %w", err)
	}

	if !resp.Success() {
		return nil, "", false, fmt.Errorf("获取子任务列表失败: %s (code: %d)", resp.Msg, resp.Code)
	}

	var tasks []*TaskInfo
	if resp.Data != nil {
		for _, task := range resp.Data.Items {
			tasks = append(tasks, taskToInfo(task))
		}
	}

	var nextPageToken string
	var hasMore bool
	if resp.Data != nil {
		nextPageToken = StringVal(resp.Data.PageToken)
		hasMore = BoolVal(resp.Data.HasMore)
	}

	return tasks, nextPageToken, hasMore, nil
}

// AddTaskMembers 添加任务成员
func AddTaskMembers(taskGuid string, memberIDs []string, memberRole string, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	var members []*larktask.Member
	for _, id := range memberIDs {
		member := larktask.NewMemberBuilder().
			Id(id).
			Type("user").
			Role(memberRole).
			Build()
		members = append(members, member)
	}

	body := larktask.NewAddMembersTaskReqBodyBuilder().
		Members(members).
		Build()

	req := larktask.NewAddMembersTaskReqBuilder().
		TaskGuid(taskGuid).
		UserIdType("open_id").
		Body(body).
		Build()

	resp, err := client.Task.V2.Task.AddMembers(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
	if err != nil {
		return fmt.Errorf("添加任务成员失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("添加任务成员失败: %s (code: %d)", resp.Msg, resp.Code)
	}

	return nil
}

// RemoveTaskMembers 移除任务成员
func RemoveTaskMembers(taskGuid string, memberIDs []string, memberRole string, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	var members []*larktask.Member
	for _, id := range memberIDs {
		member := larktask.NewMemberBuilder().
			Id(id).
			Type("user").
			Role(memberRole).
			Build()
		members = append(members, member)
	}

	body := larktask.NewRemoveMembersTaskReqBodyBuilder().
		Members(members).
		Build()

	req := larktask.NewRemoveMembersTaskReqBuilder().
		TaskGuid(taskGuid).
		UserIdType("open_id").
		Body(body).
		Build()

	resp, err := client.Task.V2.Task.RemoveMembers(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
	if err != nil {
		return fmt.Errorf("移除任务成员失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("移除任务成员失败: %s (code: %d)", resp.Msg, resp.Code)
	}

	return nil
}

// AddTaskReminders 添加任务提醒
func AddTaskReminders(taskGuid string, relativeFireMinute int, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	reminder := larktask.NewReminderBuilder().
		RelativeFireMinute(relativeFireMinute).
		Build()

	body := larktask.NewAddRemindersTaskReqBodyBuilder().
		Reminders([]*larktask.Reminder{reminder}).
		Build()

	req := larktask.NewAddRemindersTaskReqBuilder().
		TaskGuid(taskGuid).
		UserIdType("open_id").
		Body(body).
		Build()

	resp, err := client.Task.V2.Task.AddReminders(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
	if err != nil {
		return fmt.Errorf("添加任务提醒失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("添加任务提醒失败: %s (code: %d)", resp.Msg, resp.Code)
	}

	return nil
}

// RemoveTaskReminders 移除任务提醒
func RemoveTaskReminders(taskGuid string, reminderIDs []string, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	body := larktask.NewRemoveRemindersTaskReqBodyBuilder().
		ReminderIds(reminderIDs).
		Build()

	req := larktask.NewRemoveRemindersTaskReqBuilder().
		TaskGuid(taskGuid).
		UserIdType("open_id").
		Body(body).
		Build()

	resp, err := client.Task.V2.Task.RemoveReminders(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
	if err != nil {
		return fmt.Errorf("移除任务提醒失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("移除任务提醒失败: %s (code: %d)", resp.Msg, resp.Code)
	}

	return nil
}

// CreateTasklist 创建任务清单
func CreateTasklist(name string, userAccessToken string) (*TasklistInfo, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	inputTasklist := larktask.NewInputTasklistBuilder().
		Name(name).
		Build()

	req := larktask.NewCreateTasklistReqBuilder().
		UserIdType("open_id").
		InputTasklist(inputTasklist).
		Build()

	resp, err := client.Task.V2.Tasklist.Create(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
	if err != nil {
		return nil, fmt.Errorf("创建任务清单失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("创建任务清单失败: %s (code: %d)", resp.Msg, resp.Code)
	}

	if resp.Data == nil {
		return nil, fmt.Errorf("创建任务清单成功但未返回数据")
	}

	return tasklistToInfo(resp.Data.Tasklist), nil
}

// GetTasklist 获取任务清单详情
func GetTasklist(tasklistGuid string, userAccessToken string) (*TasklistInfo, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larktask.NewGetTasklistReqBuilder().
		TasklistGuid(tasklistGuid).
		UserIdType("open_id").
		Build()

	resp, err := client.Task.V2.Tasklist.Get(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
	if err != nil {
		return nil, fmt.Errorf("获取任务清单失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取任务清单失败: %s (code: %d)", resp.Msg, resp.Code)
	}

	if resp.Data == nil {
		return nil, fmt.Errorf("获取任务清单返回数据为空")
	}

	return tasklistToInfo(resp.Data.Tasklist), nil
}

// ListTasklists 列出任务清单
func ListTasklists(pageSize int, pageToken string, userAccessToken string) ([]*TasklistInfo, string, bool, error) {
	client, err := GetClient()
	if err != nil {
		return nil, "", false, err
	}

	reqBuilder := larktask.NewListTasklistReqBuilder().
		UserIdType("open_id")

	if pageSize > 0 {
		reqBuilder.PageSize(pageSize)
	}
	if pageToken != "" {
		reqBuilder.PageToken(pageToken)
	}

	resp, err := client.Task.V2.Tasklist.List(Context(), reqBuilder.Build(), larkcore.WithUserAccessToken(userAccessToken))
	if err != nil {
		return nil, "", false, fmt.Errorf("获取任务清单列表失败: %w", err)
	}

	if !resp.Success() {
		return nil, "", false, fmt.Errorf("获取任务清单列表失败: %s (code: %d)", resp.Msg, resp.Code)
	}

	var lists []*TasklistInfo
	if resp.Data != nil {
		for _, item := range resp.Data.Items {
			lists = append(lists, tasklistToInfo(item))
		}
	}

	var nextPageToken string
	var hasMore bool
	if resp.Data != nil {
		nextPageToken = StringVal(resp.Data.PageToken)
		hasMore = BoolVal(resp.Data.HasMore)
	}

	return lists, nextPageToken, hasMore, nil
}

// DeleteTasklist 删除任务清单
func DeleteTasklist(tasklistGuid string, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	req := larktask.NewDeleteTasklistReqBuilder().
		TasklistGuid(tasklistGuid).
		Build()

	resp, err := client.Task.V2.Tasklist.Delete(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
	if err != nil {
		return fmt.Errorf("删除任务清单失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("删除任务清单失败: %s (code: %d)", resp.Msg, resp.Code)
	}

	return nil
}

// tasklistToInfo 转换 SDK Tasklist 为 TasklistInfo
func tasklistToInfo(tl *larktask.Tasklist) *TasklistInfo {
	if tl == nil {
		return nil
	}
	info := &TasklistInfo{
		Guid: StringVal(tl.Guid),
		Name: StringVal(tl.Name),
		Url:  StringVal(tl.Url),
	}
	if tl.Creator != nil {
		info.Creator = StringVal(tl.Creator.Id)
	}
	if tl.Owner != nil {
		info.Owner = StringVal(tl.Owner.Id)
	}
	if createdAt := StringVal(tl.CreatedAt); createdAt != "" {
		if ts, err := strconv.ParseInt(createdAt, 10, 64); err == nil {
			info.CreatedAt = time.UnixMilli(ts).Format("2006-01-02 15:04:05")
		}
	}
	if updatedAt := StringVal(tl.UpdatedAt); updatedAt != "" {
		if ts, err := strconv.ParseInt(updatedAt, 10, 64); err == nil {
			info.UpdatedAt = time.UnixMilli(ts).Format("2006-01-02 15:04:05")
		}
	}
	return info
}

// taskToInfo converts SDK Task to TaskInfo
func taskToInfo(task *larktask.Task) *TaskInfo {
	if task == nil {
		return nil
	}

	info := &TaskInfo{
		Guid:        StringVal(task.Guid),
		Summary:     StringVal(task.Summary),
		Description: StringVal(task.Description),
	}

	if task.Due != nil {
		if ts, err := strconv.ParseInt(StringVal(task.Due.Timestamp), 10, 64); err == nil && ts > 0 {
			info.DueTime = time.UnixMilli(ts).Format("2006-01-02 15:04:05")
		}
	}

	if completedAt := StringVal(task.CompletedAt); completedAt != "" {
		if ts, err := strconv.ParseInt(completedAt, 10, 64); err == nil {
			info.CompletedAt = time.UnixMilli(ts).Format("2006-01-02 15:04:05")
		}
	}

	if task.Creator != nil {
		info.Creator = StringVal(task.Creator.Id)
	}

	if task.Origin != nil && task.Origin.Href != nil {
		info.OriginHref = StringVal(task.Origin.Href.Url)
	}

	return info
}
