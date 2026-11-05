package cmd

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

// dryRunBodies 解析 printDryRunPlan 的 JSON 输出，返回各步骤
func dryRunSteps(t *testing.T, out string) []dryRunStep {
	t.Helper()
	var plan struct {
		DryRun bool         `json:"dry_run"`
		API    []dryRunStep `json:"api"`
	}
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("dry-run 输出不是 JSON: %v\n%s", err, out)
	}
	if !plan.DryRun || len(plan.API) == 0 {
		t.Fatalf("dry-run 输出缺少 api: %s", out)
	}
	return plan.API
}

// TestTaskCreateInvalidDueIsUsageBeforeAuth --due 非法为用法错误（exit 2），且在凭据解析前返回、不联网
func TestTaskCreateInvalidDueIsUsageBeforeAuth(t *testing.T) {
	for _, c := range []*cobra.Command{createTaskCmd, updateTaskCmd} {
		rec := setupWorkCmdTest(t, "u-test", nil)
		var args []string
		flags := map[string]string{"due": "notadate"}
		if c == createTaskCmd {
			flags["summary"] = "t"
		} else {
			args = []string{"guid-1"}
		}
		_, err := runWorkCmd(t, c, args, flags)
		if err == nil || !clierr.HasKind(err, clierr.KindUsage) || !strings.Contains(err.Error(), "解析截止时间失败") {
			t.Fatalf("%s: 期望用法错误，得到 %v", c.Name(), err)
		}
		if len(rec.all()) != 0 {
			t.Fatalf("%s: 参数错误不应联网（含 token 获取）: %+v", c.Name(), rec.all())
		}
	}
}

// TestTaskCreateDryRunExtendedBody 新 flag 写入请求体：成员、清单 applink、幂等键、--data 合并且显式 flag 优先
func TestTaskCreateDryRunExtendedBody(t *testing.T) {
	rec := setupWorkCmdTest(t, "", nil)
	out, err := runWorkCmd(t, createTaskCmd, nil, map[string]string{
		"summary":         "显式标题",
		"assignee":        "ou_a,ou_b",
		"follower":        "cli_app",
		"tasklist-id":     "https://applink.feishu.cn/client/todo/task_list?guid=tl-guid",
		"idempotency-key": "k-1",
		"due":             "2026-12-01",
		"data":            `{"summary":"被覆盖","custom_fields":[{"guid":"cf1","number_value":"3"}]}`,
		"dry-run":         "true",
	})
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if len(rec.all()) != 0 {
		t.Fatalf("dry-run 不应联网: %+v", rec.all())
	}
	steps := dryRunSteps(t, out)
	if steps[0].Method != "POST" || steps[0].URL != "/open-apis/task/v2/tasks" {
		t.Fatalf("端点不对: %+v", steps[0])
	}
	b := steps[0].Body
	if b["summary"] != "显式标题" || b["client_token"] != "k-1" {
		t.Fatalf("summary/client_token 不对: %+v", b)
	}
	members, _ := b["members"].([]any)
	if len(members) != 3 {
		t.Fatalf("应有 3 个成员: %+v", b["members"])
	}
	last := members[2].(map[string]any)
	if last["id"] != "cli_app" || last["type"] != "app" || last["role"] != "follower" {
		t.Fatalf("cli_ 应识别为应用关注人: %+v", last)
	}
	tl := b["tasklists"].([]any)[0].(map[string]any)
	if tl["tasklist_guid"] != "tl-guid" {
		t.Fatalf("清单 applink 应解析出 guid: %+v", tl)
	}
	if _, ok := b["custom_fields"]; !ok {
		t.Fatalf("--data 字段应保留: %+v", b)
	}
	if due := b["due"].(map[string]any); due["is_all_day"] != false || due["timestamp"] == "" {
		t.Fatalf("due 结构不对: %+v", due)
	}
}

// TestBuildTaskCreateBodyValidation 本地校验：成员前缀、--data 格式、--data 文件
func TestBuildTaskCreateBodyValidation(t *testing.T) {
	cases := []taskCreateInput{
		{Summary: "x", Assignee: "u123"},
		{Summary: "x", Follower: "ou_ok,on_bad"},
		{Summary: "x", Data: "[1,2]"},
		{Summary: "x", Data: "{bad"},
		{Summary: "x", TasklistID: "https://applink.feishu.cn/client/todo/task_list"},
		{Data: `{"description":"no summary"}`},
	}
	for i, in := range cases {
		if _, _, err := buildTaskCreateBody(in); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
			t.Fatalf("case %d 期望用法错误，得到 %v", i, err)
		}
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "d.json")
	if err := os.WriteFile(p, []byte(`{"summary":"来自文件","mode":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	body, _, err := buildTaskCreateBody(taskCreateInput{Data: "@" + p})
	if err != nil || body["summary"] != "来自文件" || body["mode"] != float64(2) {
		t.Fatalf("@文件 读取失败: %v %+v", err, body)
	}
	// 不带新 flag 时请求体与旧 SDK 构造一致
	body, _, err = buildTaskCreateBody(taskCreateInput{Summary: "s", OriginHref: "https://example.com"})
	if err != nil || len(body) != 2 || body["origin"].(map[string]any)["platform_i18n_name"].(map[string]any)["zh_cn"] != "feishu-cli" {
		t.Fatalf("默认请求体不对: %v %+v", err, body)
	}
}

// TestTaskCreateExtendedSendsBody 用到新 flag 时按构造的请求体真实发送
func TestTaskCreateExtendedSendsBody(t *testing.T) {
	rec := setupWorkCmdTest(t, "u-test", func(w http.ResponseWriter, r *http.Request, body string) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"task":{"guid":"t1","summary":"x","members":[{"id":"ou_a","type":"user","role":"assignee"}]}}}`))
	})
	out, err := runWorkCmd(t, createTaskCmd, nil, map[string]string{"summary": "x", "assignee": "ou_a", "idempotency-key": "k", "output": "json"})
	if err != nil {
		t.Fatalf("task create: %v", err)
	}
	reqs := rec.apiReqs()
	if len(reqs) != 1 || reqs[0].Method != "POST" || reqs[0].Path != "/open-apis/task/v2/tasks" || !strings.Contains(reqs[0].Query, "user_id_type=open_id") {
		t.Fatalf("请求不对: %+v", reqs)
	}
	if !strings.Contains(reqs[0].Body, `"client_token":"k"`) || !strings.Contains(reqs[0].Body, `"role":"assignee"`) {
		t.Fatalf("请求体缺少新字段: %s", reqs[0].Body)
	}
	if !strings.Contains(out, `"guid": "t1"`) || !strings.Contains(out, `"ou_a"`) {
		t.Fatalf("输出不对: %s", out)
	}
}

// TestTaskUpdateDataUpdateFields --data 的顶层键计入 update_fields，显式 flag 覆盖同名字段
func TestTaskUpdateDataUpdateFields(t *testing.T) {
	body, err := buildTaskUpdateBody(taskUpdateInput{Summary: "新", Data: `{"summary":"旧","custom_fields":[]}`})
	if err != nil {
		t.Fatal(err)
	}
	fields := body["update_fields"].([]string)
	if strings.Join(fields, ",") != "custom_fields,summary" {
		t.Fatalf("update_fields 不对: %v", fields)
	}
	if body["task"].(map[string]any)["summary"] != "新" {
		t.Fatalf("显式 flag 应覆盖 --data: %+v", body)
	}
	if _, err := buildTaskUpdateBody(taskUpdateInput{}); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("无字段应为用法错误: %v", err)
	}
	rec := setupWorkCmdTest(t, "u-test", func(w http.ResponseWriter, r *http.Request, b string) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"task":{"guid":"g1","summary":"s"}}}`))
	})
	if _, err := runWorkCmd(t, updateTaskCmd, []string{"g1"}, map[string]string{"data": `{"mode":2}`}); err != nil {
		t.Fatalf("task update --data: %v", err)
	}
	reqs := rec.apiReqs()
	if len(reqs) != 1 || reqs[0].Method != "PATCH" || !strings.Contains(reqs[0].Body, `"update_fields":["mode"]`) {
		t.Fatalf("PATCH 请求不对: %+v", reqs)
	}
}

// TestTaskWriteCommandsDryRunNoNetwork 任务/清单写命令 --dry-run 只打印计划，不联网
func TestTaskWriteCommandsDryRunNoNetwork(t *testing.T) {
	cases := []struct {
		cmd    *cobra.Command
		args   []string
		flags  map[string]string
		method string
		url    string
	}{
		{createTaskCmd, nil, map[string]string{"summary": "s"}, "POST", "/open-apis/task/v2/tasks"},
		{updateTaskCmd, []string{"g1"}, map[string]string{"summary": "s"}, "PATCH", "/open-apis/task/v2/tasks/g1"},
		{deleteTaskCmd, []string{"g1"}, nil, "DELETE", "/open-apis/task/v2/tasks/g1"},
		{completeTaskCmd, []string{"g1"}, nil, "GET", "/open-apis/task/v2/tasks/g1"},
		{reopenTaskCmd, []string{"g1"}, nil, "PATCH", "/open-apis/task/v2/tasks/g1"},
		{taskCommentAddCmd, []string{"g1"}, map[string]string{"content": "c"}, "POST", "/open-apis/task/v2/comments"},
		{taskCommentUpdateCmd, []string{"c1"}, map[string]string{"content": "c"}, "PATCH", "/open-apis/task/v2/comments/c1"},
		{taskCommentDeleteCmd, []string{"c1"}, nil, "DELETE", "/open-apis/task/v2/comments/c1"},
		{taskSubtaskCreateCmd, []string{"g1"}, map[string]string{"summary": "s"}, "POST", "/open-apis/task/v2/tasks/g1/subtasks"},
		{taskMemberAddCmd, []string{"g1"}, map[string]string{"members": "ou_a"}, "POST", "/open-apis/task/v2/tasks/g1/add_members"},
		{taskMemberRemoveCmd, []string{"g1"}, map[string]string{"members": "ou_a"}, "POST", "/open-apis/task/v2/tasks/g1/remove_members"},
		{taskReminderAddCmd, []string{"g1"}, map[string]string{"minutes": "30"}, "POST", "/open-apis/task/v2/tasks/g1/add_reminders"},
		{taskReminderRemoveCmd, []string{"g1"}, map[string]string{"ids": "r1"}, "POST", "/open-apis/task/v2/tasks/g1/remove_reminders"},
		{taskSetAncestorCmd, []string{"g1"}, map[string]string{"ancestor-id": "g0"}, "POST", "/open-apis/task/v2/tasks/g1/set_ancestor_task"},
		{tasklistCreateCmd, nil, map[string]string{"name": "n"}, "POST", "/open-apis/task/v2/tasklists"},
		{tasklistDeleteCmd, []string{"tl"}, nil, "DELETE", "/open-apis/task/v2/tasklists/tl"},
		{tasklistTaskAddCmd, []string{"tl"}, map[string]string{"task-ids": "g1", "section-guid": "sec"}, "POST", "/open-apis/task/v2/tasks/g1/add_tasklist"},
		{tasklistTaskRemoveCmd, []string{"tl"}, map[string]string{"task-ids": "g1"}, "POST", "/open-apis/task/v2/tasks/g1/remove_tasklist"},
		{tasklistMemberAddCmd, []string{"tl"}, map[string]string{"members": "ou_a"}, "POST", "/open-apis/task/v2/tasklists/tl/add_members"},
		{tasklistMemberRemoveCmd, []string{"tl"}, map[string]string{"members": "ou_a"}, "POST", "/open-apis/task/v2/tasklists/tl/remove_members"},
	}
	for _, c := range cases {
		rec := setupWorkCmdTest(t, "", nil)
		flags := map[string]string{"dry-run": "true"}
		for k, v := range c.flags {
			flags[k] = v
		}
		out, err := runWorkCmd(t, c.cmd, c.args, flags)
		if err != nil {
			t.Fatalf("%s --dry-run: %v", c.cmd.CommandPath(), err)
		}
		steps := dryRunSteps(t, out)
		if steps[0].Method != c.method || steps[0].URL != c.url {
			t.Fatalf("%s 计划不对: %+v", c.cmd.CommandPath(), steps[0])
		}
		if len(rec.all()) != 0 {
			t.Fatalf("%s --dry-run 不应联网: %+v", c.cmd.CommandPath(), rec.all())
		}
		if c.cmd == tasklistTaskAddCmd && steps[0].Body["section_guid"] != "sec" {
			t.Fatalf("task-add 应带 section_guid: %+v", steps[0].Body)
		}
	}
}

// TestTasklistTaskAddSendsSectionGuid --section-guid 真实请求写入 section_guid
func TestTasklistTaskAddSendsSectionGuid(t *testing.T) {
	rec := setupWorkCmdTest(t, "u-test", func(w http.ResponseWriter, r *http.Request, body string) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"task":{"guid":"g1","summary":"s"}}}`))
	})
	if _, err := runWorkCmd(t, tasklistTaskAddCmd, []string{"tl"}, map[string]string{"task-ids": "g1", "section-guid": "sec"}); err != nil {
		t.Fatalf("task-add: %v", err)
	}
	reqs := rec.apiReqs()
	if len(reqs) != 1 || !strings.Contains(reqs[0].Body, `"section_guid":"sec"`) {
		t.Fatalf("应带 section_guid: %+v", reqs)
	}
}

// TestTaskCommentDeleteNeedsConfirmation 非交互且未带 --yes → exit 10，不发 DELETE
func TestTaskCommentDeleteNeedsConfirmation(t *testing.T) {
	rec := setupWorkCmdTest(t, "u-test", func(w http.ResponseWriter, r *http.Request, body string) {
		_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
	})
	_, err := runWorkCmd(t, taskCommentDeleteCmd, []string{"c1"}, nil)
	if err == nil || !clierr.HasKind(err, clierr.KindConfirmationRequired) {
		t.Fatalf("期望需要确认，得到 %v", err)
	}
	if len(rec.apiReqs()) != 0 {
		t.Fatalf("未确认不应删除: %+v", rec.apiReqs())
	}
}

// TestTaskCommentGetUpdate get/update 端点与请求体
func TestTaskCommentGetUpdate(t *testing.T) {
	rec := setupWorkCmdTest(t, "u-test", func(w http.ResponseWriter, r *http.Request, body string) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"comment":{"id":"c1","content":"新内容","creator":{"id":"ou_a"}}}}`))
	})
	out, err := runWorkCmd(t, taskCommentGetCmd, []string{"c1"}, map[string]string{"output": "json"})
	if err != nil || !strings.Contains(out, `"content": "新内容"`) {
		t.Fatalf("comment get: %v %s", err, out)
	}
	if _, err := runWorkCmd(t, taskCommentUpdateCmd, []string{"c1"}, map[string]string{"content": "新内容"}); err != nil {
		t.Fatalf("comment update: %v", err)
	}
	reqs := rec.apiReqs()
	if len(reqs) != 2 || reqs[0].Method != "GET" || reqs[0].Path != "/open-apis/task/v2/comments/c1" {
		t.Fatalf("get 请求不对: %+v", reqs)
	}
	if reqs[1].Method != "PATCH" || !strings.Contains(reqs[1].Body, `"update_fields":["content"]`) || !strings.Contains(reqs[1].Body, `"comment":{"content":"新内容"}`) {
		t.Fatalf("update 请求不对: %+v", reqs[1])
	}
	if _, err := runWorkCmd(t, taskCommentUpdateCmd, []string{"c1"}, map[string]string{"content": "  "}); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("空内容应为用法错误: %v", err)
	}
}

// TestTaskMemberInvalidRoleIsUsage 角色非法为用法错误
func TestTaskMemberInvalidRoleIsUsage(t *testing.T) {
	rec := setupWorkCmdTest(t, "u-test", nil)
	for _, c := range []*cobra.Command{taskMemberAddCmd, tasklistMemberAddCmd} {
		_, err := runWorkCmd(t, c, []string{"g1"}, map[string]string{"members": "ou_a", "role": "owner"})
		if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
			t.Fatalf("%s 期望用法错误，得到 %v", c.CommandPath(), err)
		}
	}
	if len(rec.all()) != 0 {
		t.Fatalf("不应联网: %+v", rec.all())
	}
}
