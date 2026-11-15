package cmd

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

func newEventConsumeDryRunTestCmd(t *testing.T, as string, params ...string) *cobra.Command {
	t.Helper()
	cmd := newEventConsumeTestCmd()
	cmd.Flags().StringArrayP("param", "p", nil, "")
	cmd.Flags().String("as", "auto", "")
	cmd.Flags().Bool("dry-run", false, "")
	_ = cmd.Flags().Set("dry-run", "true")
	if as != "" {
		_ = cmd.Flags().Set("as", as)
	}
	for _, p := range params {
		_ = cmd.Flags().Set("param", p)
	}
	return cmd
}

// 无凭据下 dry-run 静态预览 exit 0、不联网；审批不传 --param 仍打印两个 subscription_type。
func TestEventConsumeDryRunApprovalNoCredentials(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("dry-run 不应发出网络请求: %s", r.URL.Path)
	})
	defer cleanup()

	var runErr error
	out := captureStdout(t, func() {
		runErr = eventConsumeCmd.RunE(newEventConsumeDryRunTestCmd(t, ""), []string{"approval.instance.status_changed_v4"})
	})
	if runErr != nil {
		t.Fatalf("dry-run 应成功: %v", runErr)
	}
	var plan struct {
		DryRun        bool   `json:"dry_run"`
		Identity      string `json:"identity"`
		Subscriptions []struct {
			Path string            `json:"path"`
			Body map[string]string `json:"body"`
		} `json:"subscriptions"`
	}
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("输出不是 JSON: %v\n%s", err, out)
	}
	if !plan.DryRun || plan.Identity != "user" || len(plan.Subscriptions) != 2 ||
		plan.Subscriptions[0].Body["subscription_type"] != "INVOLVED_APPROVAL" ||
		plan.Subscriptions[1].Body["subscription_type"] != "MANAGED_APPROVAL" {
		t.Fatalf("计划不符: %+v", plan)
	}
}

func TestEventConsumeUsageErrors(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	cases := []struct {
		name   string
		key    string
		as     string
		params []string
	}{
		{"VC 不支持 bot", "vc.note.generated_v1", "bot", nil},
		{"IM key 不支持 user", "im.message.receive_v1", "user", nil},
		{"非法 --as", "vc.note.generated_v1", "robot", nil},
		{"画板缺 whiteboard_id", "board.whiteboard.updated_v1", "", nil},
		{"未知 param", "approval.instance.status_changed_v4", "", []string{"foo=bar"}},
		{"审批类型非法", "approval.instance.status_changed_v4", "", []string{"subscription_type=X"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := eventConsumeCmd.RunE(newEventConsumeDryRunTestCmd(t, c.as, c.params...), []string{c.key})
			if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
				t.Fatalf("应为用法错误（exit 2），got %v", err)
			}
		})
	}
}

func TestEventSchemaUnknownKeyIsUsage(t *testing.T) {
	err := eventSchemaCmd.RunE(eventSchemaCmd, []string{"nope.key"})
	if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("未知 EventKey 应为用法错误，got %v", err)
	}
}
