package cmd

import (
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
