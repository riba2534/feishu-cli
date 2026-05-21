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

// TestApprovalInstanceCancelFlagsDefaults 验证 flag 注册，旧参数保持兼容但隐藏
func TestApprovalInstanceCancelFlagsDefaults(t *testing.T) {
	want := []string{"approval-code", "instance-code", "user-id", "user-id-type", "user-access-token"}
	for _, n := range want {
		if approvalInstanceCancelCmd.Flags().Lookup(n) == nil {
			t.Errorf("--%s missing", n)
		}
	}
	assertHiddenFlags(t, approvalInstanceCancelCmd, "approval-code", "user-id", "user-id-type")
}

// TestApprovalInstanceCancelRequiredFlags 验证必填
func TestApprovalInstanceCancelRequiredFlags(t *testing.T) {
	assertRequiredFlags(t, approvalInstanceCancelCmd, "instance-code")
}

// TestApprovalInstanceCcFlags 验证 cc 子命令只要求当前接口实际需要的参数
func TestApprovalInstanceCcFlags(t *testing.T) {
	assertHiddenFlags(t, approvalInstanceCcCmd, "approval-code", "user-id")
	assertRequiredFlags(t, approvalInstanceCcCmd, "instance-code", "cc-user-ids")
}

// TestApprovalTaskActionFlags 验证 approve/reject 子命令不再暴露旧参数
func TestApprovalTaskActionFlags(t *testing.T) {
	for _, cmd := range []*cobra.Command{approvalTaskApproveCmd, approvalTaskRejectCmd} {
		assertHiddenFlags(t, cmd, "approval-code", "user-id", "user-id-type")
		assertRequiredFlags(t, cmd, "instance-code", "task-id")
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
