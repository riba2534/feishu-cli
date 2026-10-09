package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// assertHelpListsSubcommands 命令组 Long 必须列出全部可见子命令（以 "<路径> <子命令>" 形式），
// 防止新增子命令后 help 列表遗漏。
func assertHelpListsSubcommands(t *testing.T, group *cobra.Command, long string, prefix string) {
	t.Helper()
	for _, child := range group.Commands() {
		if !child.IsAvailableCommand() {
			continue
		}
		want := prefix + " " + child.Name()
		if !strings.Contains(long, want+"）") && !strings.Contains(long, want+")") {
			t.Errorf("%s 的 help 缺少子命令 %q", group.CommandPath(), want)
		}
	}
}

// TestApprovalHelpListsAllTaskSubcommands approval 与 approval task 的 help 列出全部任务子命令
// （含 rollback/add-sign/remind），且不再出现已下线的 --topic started 示例。
func TestApprovalHelpListsAllTaskSubcommands(t *testing.T) {
	assertHelpListsSubcommands(t, approvalTaskCmd, approvalCmd.Long, "approval task")
	assertHelpListsSubcommands(t, approvalTaskCmd, approvalTaskCmd.Long, "approval task")
	assertHelpListsSubcommands(t, approvalInstanceCmd, approvalCmd.Long, "approval instance")
	for _, name := range []string{"rollback", "add-sign", "remind"} {
		if !strings.Contains(approvalCmd.Long, "approval task "+name) {
			t.Errorf("approval --help 缺少 %s", name)
		}
	}
	for _, c := range []*cobra.Command{approvalCmd, approvalTaskCmd, approvalTaskQueryCmd} {
		if strings.Contains(c.Long, "--topic started") {
			t.Errorf("%s --help 仍包含已下线的 --topic started 示例", c.CommandPath())
		}
	}
	if !strings.Contains(approvalTaskCmd.Long, "approval instance initiated") {
		t.Error("approval task --help 应指向 approval instance initiated 查询我发起的审批")
	}
}

// TestOKRHelpScopesMatchVerifiedBehavior OKR help 的身份与 scope 与实测一致：
// cycle detail 只接受 okr:okr.content:readonly，progress list/get 只接受 okr:okr.progress:readonly，
// progress list/get/create 接受 User Token（缺 scope 为 99991679），不再声称 User Token 被 99991668 拒绝。
func TestOKRHelpScopesMatchVerifiedBehavior(t *testing.T) {
	for _, c := range []*cobra.Command{okrCmd, okrProgressListCmd, okrProgressCreateCmd, okrProgressGetCmd, okrCycleDetailCmd} {
		if strings.Contains(c.Long, "99991668") {
			t.Errorf("%s --help 仍声称 User Token 被 99991668 拒绝", c.CommandPath())
		}
	}
	lineOf := func(long, key string) string {
		for _, line := range strings.Split(long, "\n") {
			if strings.Contains(line, key) {
				return line
			}
		}
		return ""
	}
	if line := lineOf(okrCmd.Long, "cycle detail "); !strings.Contains(line, "okr:okr.content:readonly") || strings.Contains(strings.ReplaceAll(line, "okr:okr:readonly 不生效", ""), "okr:okr:readonly") {
		t.Errorf("okr --help 的 cycle detail scope 应只有 okr:okr.content:readonly: %q", line)
	}
	if line := lineOf(okrCmd.Long, "progress list / get"); !strings.Contains(line, "okr:okr.progress:readonly") || strings.Contains(line, "okr:okr:readonly 或") {
		t.Errorf("okr --help 的 progress list/get scope 应只有 okr:okr.progress:readonly: %q", line)
	}
	if line := lineOf(okrCycleDetailCmd.Long, "权限要求"); !strings.Contains(line, "okr:okr.content:readonly") {
		t.Errorf("okr cycle detail --help scope 应为 okr:okr.content:readonly: %q", line)
	}
	for _, c := range []*cobra.Command{okrProgressListCmd, okrProgressGetCmd} {
		if !strings.Contains(c.Long, "okr:okr.progress:readonly") || strings.Contains(c.Long, "okr:okr:readonly 或") {
			t.Errorf("%s --help scope 应只有 okr:okr.progress:readonly", c.CommandPath())
		}
	}
	for _, c := range []*cobra.Command{okrProgressListCmd, okrProgressCreateCmd} {
		if !strings.Contains(c.Long, "User/Bot 均可") || !strings.Contains(c.Long, "99991679") {
			t.Errorf("%s --help 应说明 User/Bot 均可调用、user 缺 scope 时 99991679", c.CommandPath())
		}
	}
}

// TestCalendarListPageSizeRange calendar list 的 --page-size 低于服务端最小值 50 时本地用法错误（exit 2）、
// 不发请求；合法值透传；help 示例不再使用非法的 20。
func TestCalendarListPageSizeRange(t *testing.T) {
	if strings.Contains(listCalendarsCmd.Long, "--page-size 20") {
		t.Fatal("calendar list --help 示例不应使用低于服务端最小值 50 的 --page-size 20")
	}
	rec := setupWorkCmdTest(t, "u-test", func(w http.ResponseWriter, r *http.Request, body string) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"calendar_list":[],"has_more":false}}`))
	})
	for _, size := range []string{"20", "49", "1001"} {
		_, err := runWorkCmd(t, listCalendarsCmd, nil, map[string]string{"page-size": size})
		if err == nil || exitCodeFor(err) != 2 || !strings.Contains(err.Error(), "50–1000") {
			t.Fatalf("--page-size %s 应为用法错误（exit 2）: %v", size, err)
		}
	}
	if n := len(rec.apiReqs()); n != 0 {
		t.Fatalf("非法 --page-size 不应发出请求，实际 %d 个", n)
	}
	if _, err := runWorkCmd(t, listCalendarsCmd, nil, map[string]string{"page-size": "100", "output": "json"}); err != nil {
		t.Fatalf("--page-size 100 应合法: %v", err)
	}
	reqs := rec.apiReqs()
	if len(reqs) != 1 || !strings.Contains(reqs[0].Query, "page_size=100") {
		t.Fatalf("合法 page_size 应透传: %+v", reqs)
	}
}

// TestCalendarAttendeeRemoveDryRunMatchesRealRequest attendee remove 的 dry-run 预览请求体与真实请求体一致
// （delete_ids / attendee_ids），不再输出 attendees + 空 type；未识别前缀的 ID 在 desc 中明确说明按 attendee_id 处理。
func TestCalendarAttendeeRemoveDryRunMatchesRealRequest(t *testing.T) {
	rec := setupWorkCmdTest(t, "u-test", func(w http.ResponseWriter, r *http.Request, body string) {
		_, _ = w.Write([]byte(`{"code":0,"msg":"ok","data":{}}`))
	})
	flags := map[string]string{"attendee-ids": "ou_fake1,oc_fake2,omm_fake3,a@example.com,user_fakeattendee", "notify": "false"}

	dryFlags := map[string]string{"dry-run": "true"}
	for k, v := range flags {
		dryFlags[k] = v
	}
	out, err := runWorkCmd(t, calendarAttendeeRemoveCmd, []string{"CAL_fake", "EVT_fake"}, dryFlags)
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if n := len(rec.apiReqs()); n != 0 {
		t.Fatalf("dry-run 不应发请求，实际 %d 个", n)
	}
	var plan struct {
		API []struct {
			URL    string         `json:"url"`
			Desc   string         `json:"desc"`
			Params map[string]any `json:"params"`
			Body   map[string]any `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal([]byte(out), &plan); err != nil || len(plan.API) != 1 {
		t.Fatalf("dry-run 输出不是预期 JSON: %v\n%s", err, out)
	}
	if strings.Contains(out, `"type": ""`) || strings.Contains(out, `"attendees"`) {
		t.Fatalf("dry-run 不应输出 attendees/空 type:\n%s", out)
	}
	if !strings.Contains(plan.API[0].Desc, "user_fakeattendee") || !strings.Contains(plan.API[0].Desc, "attendee_id") {
		t.Fatalf("dry-run 应说明未识别前缀的 ID 按 attendee_id 处理: %q", plan.API[0].Desc)
	}

	// runWorkCmd 只在测试结束时复位 flag，这里显式关闭 dry-run
	flags["dry-run"] = "false"
	if _, err := runWorkCmd(t, calendarAttendeeRemoveCmd, []string{"CAL_fake", "EVT_fake"}, flags); err != nil {
		t.Fatalf("真实请求（mock）: %v", err)
	}
	reqs := rec.apiReqs()
	if len(reqs) != 1 {
		t.Fatalf("应发出 1 个请求，实际 %d 个", len(reqs))
	}
	if reqs[0].Path != plan.API[0].URL || !strings.Contains(reqs[0].Query, "user_id_type=open_id") || plan.API[0].Params["user_id_type"] != "open_id" {
		t.Fatalf("dry-run URL/params 与真实请求不一致: dry=%s %v real=%s?%s", plan.API[0].URL, plan.API[0].Params, reqs[0].Path, reqs[0].Query)
	}
	var real map[string]any
	if err := json.Unmarshal([]byte(reqs[0].Body), &real); err != nil {
		t.Fatalf("真实请求体不是 JSON: %v", err)
	}
	dryJSON, _ := json.Marshal(plan.API[0].Body)
	realJSON, _ := json.Marshal(real)
	if string(dryJSON) != string(realJSON) {
		t.Fatalf("dry-run 请求体与真实请求体不一致:\ndry=%s\nreal=%s", dryJSON, realJSON)
	}
	if real["attendee_ids"] == nil || real["delete_ids"] == nil {
		t.Fatalf("真实请求体应同时含 delete_ids 与 attendee_ids: %s", realJSON)
	}
}

// TestTasklistListDefaultPageSize tasklist list 默认显式传 page_size=100（不传时服务端报 1470500），
// 越界值本地用法错误（exit 2）且不发请求。
func TestTasklistListDefaultPageSize(t *testing.T) {
	rec := setupWorkCmdTest(t, "u-test", func(w http.ResponseWriter, r *http.Request, body string) {
		_, _ = w.Write([]byte(`{"code":0,"msg":"ok","data":{"items":[],"has_more":false}}`))
	})
	if _, err := runWorkCmd(t, tasklistListCmd, nil, map[string]string{"output": "json"}); err != nil {
		t.Fatalf("tasklist list: %v", err)
	}
	reqs := rec.apiReqs()
	if len(reqs) != 1 || !strings.Contains(reqs[0].Query, "page_size=100") {
		t.Fatalf("默认应显式传 page_size=100: %+v", reqs)
	}
	for _, size := range []string{"0", "101"} {
		_, err := runWorkCmd(t, tasklistListCmd, nil, map[string]string{"page-size": size})
		if err == nil || exitCodeFor(err) != 2 {
			t.Fatalf("--page-size %s 应为用法错误（exit 2）: %v", size, err)
		}
	}
	if n := len(rec.apiReqs()); n != 1 {
		t.Fatalf("越界 --page-size 不应发请求，实际共 %d 个", n)
	}
}
