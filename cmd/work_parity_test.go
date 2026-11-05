package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// setupNoCredsWorkTest 在 setupWorkCmdTest 基础上清空 App 凭证：config.Validate 会返回 exit 3。
// 用于断言「参数错误在凭据校验之前返回 exit 2」与「静态 dry-run 无凭据 exit 0」。
func setupNoCredsWorkTest(t *testing.T) *workRecorder {
	t.Helper()
	rec := setupWorkCmdTest(t, "", nil)
	t.Setenv("FEISHU_APP_ID", "")
	t.Setenv("FEISHU_APP_SECRET", "")
	viper.Reset()
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Init(cfgPath); err != nil {
		t.Fatalf("config.Init: %v", err)
	}
	if err := config.Validate(); err == nil {
		t.Fatalf("期望无凭据时 Validate 失败")
	}
	return rec
}

// TestWorkParamErrorsAreUsageBeforeCredentials 参数错误为 exit 2，且先于凭据校验（无凭据时不是 exit 3）
func TestWorkParamErrorsAreUsageBeforeCredentials(t *testing.T) {
	cases := []struct {
		name  string
		cmd   *cobra.Command
		args  []string
		flags map[string]string
		want  string
	}{
		{"approval task query started", approvalTaskQueryCmd, nil, map[string]string{"topic": "started"}, "approval instance initiated"},
		{"approval task query 3", approvalTaskQueryCmd, nil, map[string]string{"topic": "3"}, "topic"},
		{"approval task query unknown", approvalTaskQueryCmd, nil, map[string]string{"topic": "nope"}, "不支持的 topic"},
		{"approval get bad code", approvalGetCmd, []string{"bad code!"}, nil, "无效的审批定义 code"},
		{"approval instance initiated bad id type", approvalInstanceInitiatedCmd, nil, map[string]string{"user-id-type": "email"}, "user_id_type"},
		{"task create bad due", createTaskCmd, nil, map[string]string{"summary": "t", "due": "notadate"}, "解析截止时间失败"},
		{"task update bad due", updateTaskCmd, []string{"g1"}, map[string]string{"due": "notadate"}, "解析截止时间失败"},
		// --as user 且未登录：若先解析身份会返回鉴权错误；参数校验在前则返回用法错误
		{"task create bad due as user", createTaskCmd, nil, map[string]string{"summary": "t", "due": "notadate", "as": "user"}, "解析截止时间失败"},
		{"task update bad due as user", updateTaskCmd, []string{"g1"}, map[string]string{"due": "notadate", "as": "user"}, "解析截止时间失败"},
		{"task get bad guid", getTaskCmd, []string{"t123"}, nil, ""},
		{"task search both", taskSearchCmd, nil, map[string]string{"completed": "true", "uncompleted": "true"}, "不能同时使用"},
		{"task search no cond", taskSearchCmd, nil, nil, "请至少提供一个搜索条件"},
		{"attendance stats no ids", attendanceUserStatsQueryCmd, nil, map[string]string{"start": "2026-01-01", "end": "2026-01-02"}, "--user-ids"},
		{"attendance task range", attendanceUserTaskQueryCmd, nil, map[string]string{"start": "2026-02-01", "end": "2026-01-01"}, "不能晚于"},
		{"calendar suggestion duration", calendarSuggestionCmd, nil, map[string]string{"duration": "0"}, "--duration"},
		{"calendar room-find no slot", calendarRoomFindCmd, nil, nil, "--slot"},
		{"calendar rsvp bad action", calendarRsvpCmd, nil, map[string]string{"event-id": "e1", "action": "maybe"}, "无效的 --action"},
		{"okr progress create both", okrProgressCreateCmd, nil, map[string]string{"objective-id": "o", "key-result-id": "k", "content": "c"}, "只能填一个"},
		{"okr progress update rate", okrProgressUpdateCmd, []string{"p1"}, map[string]string{"content": "c", "progress-status": "done"}, "--progress-percent"},
		{"okr upload-image no file", okrUploadImageCmd, nil, map[string]string{"objective-id": "o"}, "--file"},
		{"okr progress list id type", okrProgressListCmd, nil, map[string]string{"objective-id": "o", "user-id-type": "email"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := setupNoCredsWorkTest(t)
			_, err := runWorkCmd(t, c.cmd, c.args, c.flags)
			if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
				t.Fatalf("期望用法错误（exit 2），得到 %v", err)
			}
			if c.want != "" && !strings.Contains(err.Error(), c.want) {
				t.Fatalf("错误文本应包含 %q: %v", c.want, err)
			}
			if len(rec.all()) != 0 {
				t.Fatalf("参数错误不应联网: %+v", rec.all())
			}
		})
	}
}

// TestWorkDryRunWithoutCredentials 新增 dry-run 在无凭据时 exit 0、不联网；去掉 --dry-run 为 exit 3
func TestWorkDryRunWithoutCredentials(t *testing.T) {
	img := filepath.Join(t.TempDir(), "a.png")
	if err := os.WriteFile(img, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		cmd   *cobra.Command
		args  []string
		flags map[string]string
		want  string
	}{
		{calendarRsvpCmd, nil, map[string]string{"event-id": "e1", "action": "accept"}, "/open-apis/calendar/v4/calendars/primary"},
		{calendarRsvpCmd, nil, map[string]string{"event-id": "e1", "action": "decline", "calendar-id": "cal1"}, "/open-apis/calendar/v4/calendars/cal1/events/e1/reply"},
		{calendarAttendeeAddCmd, []string{"cal1", "e1"}, map[string]string{"user-ids": "ou_a"}, "/open-apis/calendar/v4/calendars/cal1/events/e1/attendees"},
		{okrUploadImageCmd, nil, map[string]string{"file": img, "objective-id": "o1"}, "/open-apis/okr/v1/images/upload"},
		{createTaskCmd, nil, map[string]string{"summary": "s", "assignee": "ou_a"}, "/open-apis/task/v2/tasks"},
		{taskCommentDeleteCmd, []string{"c1"}, nil, "/open-apis/task/v2/comments/c1"},
		{okrProgressUpdateCmd, []string{"p1"}, map[string]string{"content": "c"}, "/open-apis/okr/v1/progress_records/p1"},
	}
	for _, c := range cases {
		rec := setupNoCredsWorkTest(t)
		flags := map[string]string{"dry-run": "true"}
		for k, v := range c.flags {
			flags[k] = v
		}
		out, err := runWorkCmd(t, c.cmd, c.args, flags)
		if err != nil || !strings.Contains(out, c.want) {
			t.Fatalf("%s 无凭据 dry-run: %v\n%s", c.cmd.CommandPath(), err, out)
		}
		if len(rec.all()) != 0 {
			t.Fatalf("%s dry-run 不应联网: %+v", c.cmd.CommandPath(), rec.all())
		}
		flags["dry-run"] = "false"
		_, err = runWorkCmd(t, c.cmd, c.args, flags)
		if err == nil || !clierr.HasKind(err, clierr.KindAuth) {
			t.Fatalf("%s 去掉 --dry-run 应为凭据错误（exit 3），得到 %v", c.cmd.CommandPath(), err)
		}
	}
}
