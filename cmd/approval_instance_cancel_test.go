package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestApprovalInstanceCancelCmdRegistered 验证 cancel 子命令注册
func TestApprovalInstanceCancelCmdRegistered(t *testing.T) {
	if approvalInstanceCancelCmd.Use != "cancel" {
		t.Fatalf("Use = %q, want cancel", approvalInstanceCancelCmd.Use)
	}
	if approvalInstanceCancelCmd.Short == "" {
		t.Fatal("Short should not be empty")
	}
	found := false
	for _, sub := range approvalInstanceCmd.Commands() {
		if sub == approvalInstanceCancelCmd {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("approvalInstanceCancelCmd should be child of approvalInstanceCmd")
	}
}

// TestApprovalInstanceCancelFlagsDefaults 验证正式接口所需 flag 已注册。
func TestApprovalInstanceCancelFlagsDefaults(t *testing.T) {
	want := []string{"instance-code", "user-access-token"}
	for _, n := range want {
		if approvalInstanceCancelCmd.Flags().Lookup(n) == nil {
			t.Errorf("--%s missing", n)
		}
	}
}

// TestApprovalInstanceCancelRequiredFlags 验证必填
func TestApprovalInstanceCancelRequiredFlags(t *testing.T) {
	assertRequiredFlags(t, approvalInstanceCancelCmd, "instance-code")
}

// TestApprovalInstanceCcFlags 验证 cc 子命令要求正式接口实际需要的参数
func TestApprovalInstanceCcFlags(t *testing.T) {
	assertRequiredFlags(t, approvalInstanceCcCmd, "instance-code", "cc-user-ids")
}

func TestApprovalInstanceGetFlags(t *testing.T) {
	assertRequiredFlags(t, approvalInstanceGetCmd, "instance-code")
}

// TestApprovalTaskActionFlags 验证 approve/reject 子命令要求正式接口实际需要的参数
func TestApprovalTaskActionFlags(t *testing.T) {
	for _, cmd := range []*cobra.Command{approvalTaskApproveCmd, approvalTaskRejectCmd} {
		assertRequiredFlags(t, cmd, "instance-code", "task-id")
	}
}

func TestApprovalTaskTransferFlags(t *testing.T) {
	assertRequiredFlags(t, approvalTaskTransferCmd, "instance-code", "task-id", "transfer-user-id")
}

func TestApprovalResourceAliases(t *testing.T) {
	if len(approvalInstanceCmd.Aliases) == 0 || approvalInstanceCmd.Aliases[0] != "instances" {
		t.Fatalf("approval instance aliases = %#v, want instances", approvalInstanceCmd.Aliases)
	}
	if len(approvalTaskCmd.Aliases) == 0 || approvalTaskCmd.Aliases[0] != "tasks" {
		t.Fatalf("approval task aliases = %#v, want tasks", approvalTaskCmd.Aliases)
	}
}

func assertHiddenFlags(t *testing.T, cmd *cobra.Command, names ...string) {
	t.Helper()
	for _, n := range names {
		f := cmd.Flags().Lookup(n)
		if f == nil {
			t.Fatalf("--%s missing", n)
		}
		if !f.Hidden {
			t.Errorf("--%s should be hidden compatibility flag", n)
		}
	}
}

func assertRequiredFlags(t *testing.T, cmd *cobra.Command, names ...string) {
	t.Helper()
	for _, n := range names {
		f := cmd.Flags().Lookup(n)
		if f == nil {
			t.Fatalf("--%s missing", n)
		}
		ann := f.Annotations["cobra_annotation_bash_completion_one_required_flag"]
		if len(ann) == 0 || ann[0] != "true" {
			t.Errorf("--%s should be required, ann=%v", n, ann)
		}
	}
}
