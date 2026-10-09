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
