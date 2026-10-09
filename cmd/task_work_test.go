package cmd

import (
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/riba2534/feishu-cli/v2/internal/clierr"
)

// TestTaskMyFlagsMutuallyExclusive --completed 与 --uncompleted 同时传本地报错（此前静默取 completed）
func TestTaskMyFlagsMutuallyExclusive(t *testing.T) {
	rec := setupWorkCmdTest(t, "u-test", nil)
	_, err := runWorkCmd(t, myTasksCmd, nil, map[string]string{"completed": "true", "uncompleted": "true"})
	if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("期望用法错误，得到 %v", err)
	}
	if len(rec.all()) != 0 {
		t.Fatalf("不应联网: %+v", rec.all())
	}
}

// TestTaskMyTruncatesByRune 描述按 rune 截断，中文不能被切成半个字符
func TestTaskMyTruncatesByRune(t *testing.T) {
	desc := strings.Repeat("中文描述", 20)
	setupWorkCmdTest(t, "u-test", func(w http.ResponseWriter, r *http.Request, body string) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"guid":"t1","summary":"任务","description":"` + desc + `"}],"has_more":false}}`))
	})
	out, err := runWorkCmd(t, myTasksCmd, nil, map[string]string{"uncompleted": "true"})
	if err != nil {
		t.Fatalf("task my: %v", err)
	}
	if !utf8.ValidString(out) {
		t.Fatalf("输出含非法 UTF-8（按字节截断中文）: %q", out)
	}
	if !strings.Contains(out, strings.Repeat("中文描述", 12)+"中文…") {
		t.Fatalf("应截断为 50 个字符: %s", out)
	}
}

// TestTaskWriteDefaultsToUserIdentity 任务写命令默认 auto：已登录用 User Token（此前默认 Bot，实测 1470403）
func TestTaskWriteDefaultsToUserIdentity(t *testing.T) {
	rec := setupWorkCmdTest(t, "u-test", func(w http.ResponseWriter, r *http.Request, body string) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"task":{"guid":"t1","summary":"x"}}}`))
	})
	if _, err := runWorkCmd(t, createTaskCmd, nil, map[string]string{"summary": "x"}); err != nil {
		t.Fatalf("task create: %v", err)
	}
	reqs := rec.apiReqs()
	if len(reqs) != 1 || reqs[0].Auth != "Bearer u-test" {
		t.Fatalf("默认应使用 User Token: %+v", reqs)
	}
	rec2 := setupWorkCmdTest(t, "u-test", func(w http.ResponseWriter, r *http.Request, body string) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"task":{"guid":"t1","summary":"x"}}}`))
	})
	if _, err := runWorkCmd(t, createTaskCmd, nil, map[string]string{"summary": "x", "as": "bot"}); err != nil {
		t.Fatalf("task create --as bot: %v", err)
	}
	if reqs := rec2.apiReqs(); len(reqs) != 1 || reqs[0].Auth != "Bearer t-bot" {
		t.Fatalf("--as bot 应使用 Tenant Token: %+v", reqs)
	}
}
