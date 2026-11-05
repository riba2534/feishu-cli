package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

// ============================================================================
// 任务写命令的请求体构造与 --dry-run 公共逻辑。
// 构造函数只做本地解析与校验（不联网、不读凭据），dry-run 预览与真实执行共用同一份请求体。
// ============================================================================

const taskDryRunHelp = "只预览将发出的请求，不执行（不联网、不读凭据）"

// addTaskDryRunFlag 为任务/清单写命令注册 --dry-run
func addTaskDryRunFlag(c *cobra.Command) {
	c.Flags().Bool("dry-run", false, taskDryRunHelp)
}

// taskDryRunRequested 读取 --dry-run
func taskDryRunRequested(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("dry-run")
	return v
}

// taskAPIPath 拼接任务 v2 接口路径（各段做 PathEscape）
func taskAPIPath(segments ...string) string {
	var b strings.Builder
	b.WriteString(client.TaskAPIBase)
	for _, s := range segments {
		b.WriteString("/")
		b.WriteString(url.PathEscape(s))
	}
	return b.String()
}

// taskOpenIDParams dry-run 中展示的 query 参数
func taskOpenIDParams() map[string]any {
	return map[string]any{"user_id_type": "open_id"}
}

// parseJSONObjectFlag 解析 JSON 对象类 flag：支持内联 JSON 或 @path（本地文件，拒绝敏感目录）。
// 空字符串返回 nil, nil；非对象或格式错误为用法错误。
func parseJSONObjectFlag(raw, flagName string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	data := []byte(raw)
	if strings.HasPrefix(raw, "@") {
		path := strings.TrimSpace(strings.TrimPrefix(raw, "@"))
		if path == "" {
			return nil, clierr.Usagef("--%s 的 @ 后需要文件路径", flagName)
		}
		b, err := readLocalInputFile(path)
		if err != nil {
			return nil, err
		}
		data = b
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil || obj == nil {
		if err == nil {
			err = fmt.Errorf("不是 JSON 对象")
		}
		return nil, clierr.Usagef("--%s 必须是 JSON 对象（内联或 @文件）: %v", flagName, err)
	}
	return obj, nil
}

// splitCSV 按逗号切分并去掉空白与空项
func splitCSV(raw string) []string {
	var out []string
	for _, s := range strings.Split(raw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// buildTaskMembers 把 --assignee/--follower 的 ID 列表转成任务成员；ou_ 为用户，cli_ 为应用。
func buildTaskMembers(raw, role, flagName string) ([]any, error) {
	var members []any
	for _, id := range splitCSV(raw) {
		var typ string
		switch {
		case strings.HasPrefix(id, "ou_"):
			typ = "user"
		case strings.HasPrefix(id, "cli_"):
			typ = "app"
		default:
			return nil, clierr.Usagef("--%s 仅支持 open_id（ou_ 开头）或应用 ID（cli_ 开头），得到 %q", flagName, id)
		}
		members = append(members, map[string]any{"id": id, "type": typ, "role": role})
	}
	return members, nil
}

// extractTasklistGUID 从清单 GUID 或清单 applink（?guid=...）中取 GUID
func extractTasklistGUID(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", clierr.Usagef("无效的清单链接: %v", err)
		}
		guid := strings.TrimSpace(u.Query().Get("guid"))
		if guid == "" {
			return "", clierr.Usagef("清单链接缺少 guid 参数")
		}
		return guid, nil
	}
	return raw, nil
}

// parseTaskDueFlag 解析 --due（本地校验，参数错误为 exit 2）
func parseTaskDueFlag(dueStr string) (int64, error) {
	if dueStr == "" {
		return 0, nil
	}
	dueTime, err := parseTime(dueStr)
	if err != nil {
		return 0, clierr.Usagef("解析截止时间失败: %v", err)
	}
	return dueTime.UnixMilli(), nil
}

// taskDueBody 截止时间请求体（与 SDK 构造一致：毫秒时间戳字符串，非全天）
func taskDueBody(ms int64) map[string]any {
	return map[string]any{"timestamp": strconv.FormatInt(ms, 10), "is_all_day": false}
}

// taskCreateInput task create 的 flag 输入
type taskCreateInput struct {
	Summary        string
	Description    string
	Due            string
	OriginHref     string
	OriginPlatform string
	Assignee       string
	Follower       string
	TasklistID     string
	IdempotencyKey string
	Data           string
}

// extended 报告是否用到了 SDK 结构体之外的新能力（成员、清单、幂等键、--data）
func (in taskCreateInput) extended() bool {
	return in.Assignee != "" || in.Follower != "" || in.TasklistID != "" || in.IdempotencyKey != "" || strings.TrimSpace(in.Data) != ""
}

// buildTaskCreateBody 构造 POST /task/v2/tasks 请求体：先取 --data，再由显式 flag 覆盖同名字段。
// 返回的 dueMs 供兼容路径（SDK）复用。
func buildTaskCreateBody(in taskCreateInput) (body map[string]any, dueMs int64, err error) {
	body, err = parseJSONObjectFlag(in.Data, "data")
	if err != nil {
		return nil, 0, err
	}
	if body == nil {
		body = map[string]any{}
	}
	if dueMs, err = parseTaskDueFlag(in.Due); err != nil {
		return nil, 0, err
	}
	if in.Summary != "" {
		body["summary"] = in.Summary
	}
	if s, _ := body["summary"].(string); strings.TrimSpace(s) == "" {
		return nil, 0, clierr.Usagef("--summary 不能为空")
	}
	if in.Description != "" {
		body["description"] = in.Description
	}
	if dueMs > 0 {
		body["due"] = taskDueBody(dueMs)
	}
	if in.OriginHref != "" {
		platform := in.OriginPlatform
		if platform == "" {
			platform = "feishu-cli"
		}
		body["origin"] = map[string]any{
			"platform_i18n_name": map[string]any{"zh_cn": platform, "en_us": platform},
			"href":               map[string]any{"url": in.OriginHref},
		}
	}
	assignees, err := buildTaskMembers(in.Assignee, "assignee", "assignee")
	if err != nil {
		return nil, 0, err
	}
	followers, err := buildTaskMembers(in.Follower, "follower", "follower")
	if err != nil {
		return nil, 0, err
	}
	if members := append(assignees, followers...); len(members) > 0 {
		body["members"] = members
	}
	if in.TasklistID != "" {
		guid, err := extractTasklistGUID(in.TasklistID)
		if err != nil {
			return nil, 0, err
		}
		body["tasklists"] = []any{map[string]any{"tasklist_guid": guid}}
	}
	if in.IdempotencyKey != "" {
		body["client_token"] = in.IdempotencyKey
	}
	return body, dueMs, nil
}

// taskMemberDryRunList dry-run 展示的成员列表（与 client 构造一致：type 固定 user）
func taskMemberDryRunList(ids []string, role string) []any {
	out := make([]any, 0, len(ids))
	for _, id := range ids {
		out = append(out, map[string]any{"id": id, "type": "user", "role": role})
	}
	return out
}

// taskUpdateInput task update 的 flag 输入
type taskUpdateInput struct {
	Summary     string
	Description string
	Due         string
	Completed   bool
	Data        string
	NowMs       int64 // --completed 时写入的完成时间（毫秒）
}

// buildTaskUpdateBody 构造 PATCH /task/v2/tasks/{guid} 请求体：--data 的每个顶层键都计入 update_fields，
// 显式 flag 覆盖同名字段。没有任何可更新字段时返回用法错误（文本与旧版一致）。
func buildTaskUpdateBody(in taskUpdateInput) (map[string]any, error) {
	taskObj, err := parseJSONObjectFlag(in.Data, "data")
	if err != nil {
		return nil, err
	}
	dueMs, err := parseTaskDueFlag(in.Due)
	if err != nil {
		return nil, err
	}
	if taskObj == nil {
		taskObj = map[string]any{}
	}
	var fields []string
	seen := map[string]bool{}
	add := func(k string) {
		if !seen[k] {
			seen[k] = true
			fields = append(fields, k)
		}
	}
	// --data 的键按字典序计入，保证 dry-run 输出稳定
	keys := make([]string, 0, len(taskObj))
	for k := range taskObj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		add(k)
	}
	if in.Summary != "" {
		taskObj["summary"] = in.Summary
		add("summary")
	}
	if in.Description != "" {
		taskObj["description"] = in.Description
		add("description")
	}
	if dueMs > 0 {
		taskObj["due"] = taskDueBody(dueMs)
		add("due")
	}
	if in.Completed {
		taskObj["completed_at"] = strconv.FormatInt(in.NowMs, 10)
		add("completed_at")
	}
	if len(fields) == 0 {
		return nil, clierr.Usagef("请指定要更新的字段（--summary, --description, --due 或 --completed）")
	}
	return map[string]any{"task": taskObj, "update_fields": fields}, nil
}
