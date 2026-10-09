package cmd

import (
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

func TestNormalizeApprovalTaskTopic(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: "todo", want: "1"},
		{input: "1", want: "1"},
		{input: "done", want: "2"},
		// topic=3 已被官方 tasks 接口下线（服务端回 99992402
		// "topic is optional, options: [1,2,17,18]"），必须 fail-closed 并给出迁移提示
		{input: "started", wantErr: true},
		{input: "initiated", wantErr: true},
		{input: "3", wantErr: true},
		{input: "cc-unread", want: "17"},
		{input: "cc-read", want: "18"},
		{input: "unknown", wantErr: true},
	}

	for _, tt := range tests {
		got, err := normalizeApprovalTaskTopic(tt.input)
		if (err != nil) != tt.wantErr {
			t.Fatalf("normalizeApprovalTaskTopic(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
		}
		if err == nil && got != tt.want {
			t.Fatalf("normalizeApprovalTaskTopic(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestNormalizeApprovalTaskTopic_StartedGivesMigrationHint 验证被下线的 started
// 报错时指向可用的替代命令，而不是让用户撞服务端 400。
func TestNormalizeApprovalTaskTopic_StartedGivesMigrationHint(t *testing.T) {
	for _, in := range []string{"started", "initiated", "3"} {
		_, err := normalizeApprovalTaskTopic(in)
		if err == nil {
			t.Fatalf("normalizeApprovalTaskTopic(%q) 应报错", in)
		}
		if !strings.Contains(err.Error(), "approval instance initiated") {
			t.Errorf("normalizeApprovalTaskTopic(%q) 错误信息应指向 approval instance initiated，得到: %v", in, err)
		}
	}
}

func TestApprovalTaskTopicLabel(t *testing.T) {
	tests := map[string]string{
		"1":  "待我审批",
		"2":  "我已审批",
		"3":  "我发起的审批",
		"17": "未读抄送",
		"18": "已读抄送",
		"x":  "x",
	}

	for input, want := range tests {
		if got := approvalTaskTopicLabel(input); got != want {
			t.Fatalf("approvalTaskTopicLabel(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestResolveFlagUserTokenIgnoresEnvironment(t *testing.T) {
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "env-token")

	cmd := &cobra.Command{}
	cmd.Flags().String("user-access-token", "", "")

	if got := resolveFlagUserToken(cmd); got != "" {
		t.Fatalf("resolveFlagUserToken() = %q, want empty string", got)
	}

	if err := cmd.Flags().Set("user-access-token", "flag-token"); err != nil {
		t.Fatalf("Set flag: %v", err)
	}

	if got := resolveFlagUserToken(cmd); got != "flag-token" {
		t.Fatalf("resolveFlagUserToken() = %q, want %q", got, "flag-token")
	}
}

func TestApprovalTaskQueryDoesNotRegisterUserIDFlag(t *testing.T) {
	if flag := approvalTaskQueryCmd.Flags().Lookup("user-id"); flag != nil {
		t.Fatalf("approvalTaskQueryCmd should not register --user-id, got %q", flag.Name)
	}
}

func TestApprovalTaskQueryRequiresTopicFlag(t *testing.T) {
	flag := approvalTaskQueryCmd.Flags().Lookup("topic")
	if flag == nil {
		t.Fatal("approvalTaskQueryCmd should register --topic")
	}
	if got := flag.Annotations[cobra.BashCompOneRequiredFlag]; len(got) != 1 || got[0] != "true" {
		t.Fatalf("--topic should be marked required, got annotations %#v", flag.Annotations)
	}
}

func TestCurrentUserIDFromInfo(t *testing.T) {
	info := &client.UserInfo{
		OpenID:  "ou_test",
		UserID:  "u_test",
		UnionID: "on_test",
	}

	tests := []struct {
		userIDType string
		want       string
		wantErr    bool
	}{
		{userIDType: "open_id", want: "ou_test"},
		{userIDType: "user_id", want: "u_test"},
		{userIDType: "union_id", want: "on_test"},
		{userIDType: "employee_id", wantErr: true},
	}

	for _, tt := range tests {
		got, err := currentUserIDFromInfo(info, tt.userIDType)
		if (err != nil) != tt.wantErr {
			t.Fatalf("currentUserIDFromInfo(..., %q) error = %v, wantErr %v", tt.userIDType, err, tt.wantErr)
		}
		if err == nil && got != tt.want {
			t.Fatalf("currentUserIDFromInfo(..., %q) = %q, want %q", tt.userIDType, got, tt.want)
		}
	}
}

func TestCurrentUserIDFromInfoMissingIDDoesNotMentionRemovedUserIDFlag(t *testing.T) {
	_, err := currentUserIDFromInfo(&client.UserInfo{}, "open_id")
	if err == nil {
		t.Fatal("currentUserIDFromInfo(..., open_id) error = nil, want non-nil")
	}
	if strings.Contains(err.Error(), "--user-id") {
		t.Fatalf("error %q should not mention removed --user-id flag", err.Error())
	}
}
