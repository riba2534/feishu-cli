package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var taskMemberCmd = &cobra.Command{
	Use:   "member",
	Short: "任务成员管理",
	Long: `管理任务成员，支持添加和移除成员。

子命令:
  add       添加成员
  remove    移除成员

示例:
  feishu-cli task member add TASK_GUID --members ou_xxx,ou_yyy --role assignee
  feishu-cli task member remove TASK_GUID --members ou_xxx --role follower`,
}

var taskMemberAddCmd = &cobra.Command{
	Use:   "add <task_guid>",
	Short: "添加任务成员",
	Long: `向任务添加成员（执行者或关注者）。

参数:
  task_guid     任务 GUID（位置参数）
  --members     成员 ID 列表，逗号分隔（必填）
  --role        角色: assignee（执行者）或 follower（关注者），默认 assignee

示例:
  feishu-cli task member add TASK_GUID --members ou_xxx,ou_yyy
  feishu-cli task member add TASK_GUID --members ou_xxx --role follower`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		taskGuid := args[0]
		membersStr, _ := cmd.Flags().GetString("members")
		role, _ := cmd.Flags().GetString("role")

		if role != "assignee" && role != "follower" {
			return fmt.Errorf("无效的角色: %s（仅支持 assignee 或 follower）", role)
		}

		var memberIDs []string
		for _, id := range strings.Split(membersStr, ",") {
			id = strings.TrimSpace(id)
			if id != "" {
				memberIDs = append(memberIDs, id)
			}
		}

		if len(memberIDs) == 0 {
			return fmt.Errorf("成员列表不能为空")
		}

		if err := client.AddTaskMembers(taskGuid, memberIDs, role, token); err != nil {
			return err
		}

		fmt.Printf("成功添加 %d 个%s\n", len(memberIDs), roleLabel(role))
		return nil
	},
}

var taskMemberRemoveCmd = &cobra.Command{
	Use:   "remove <task_guid>",
	Short: "移除任务成员",
	Long: `从任务中移除成员。

参数:
  task_guid     任务 GUID（位置参数）
  --members     成员 ID 列表，逗号分隔（必填）
  --role        角色: assignee（执行者）或 follower（关注者），默认 assignee

示例:
  feishu-cli task member remove TASK_GUID --members ou_xxx
  feishu-cli task member remove TASK_GUID --members ou_xxx --role follower`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		taskGuid := args[0]
		membersStr, _ := cmd.Flags().GetString("members")
		role, _ := cmd.Flags().GetString("role")

		if role != "assignee" && role != "follower" {
			return fmt.Errorf("无效的角色: %s（仅支持 assignee 或 follower）", role)
		}

		var memberIDs []string
		for _, id := range strings.Split(membersStr, ",") {
			id = strings.TrimSpace(id)
			if id != "" {
				memberIDs = append(memberIDs, id)
			}
		}

		if len(memberIDs) == 0 {
			return fmt.Errorf("成员列表不能为空")
		}

		if err := client.RemoveTaskMembers(taskGuid, memberIDs, role, token); err != nil {
			return err
		}

		fmt.Printf("成功移除 %d 个%s\n", len(memberIDs), roleLabel(role))
		return nil
	},
}

func roleLabel(role string) string {
	if role == "follower" {
		return "关注者"
	}
	return "执行者"
}

func init() {
	taskCmd.AddCommand(taskMemberCmd)

	taskMemberCmd.AddCommand(taskMemberAddCmd)
	taskMemberAddCmd.Flags().String("members", "", "成员 ID 列表，逗号分隔（必填）")
	taskMemberAddCmd.Flags().String("role", "assignee", "角色: assignee/follower")
	taskMemberAddCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	mustMarkFlagRequired(taskMemberAddCmd, "members")

	taskMemberCmd.AddCommand(taskMemberRemoveCmd)
	taskMemberRemoveCmd.Flags().String("members", "", "成员 ID 列表，逗号分隔（必填）")
	taskMemberRemoveCmd.Flags().String("role", "assignee", "角色: assignee/follower")
	taskMemberRemoveCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	mustMarkFlagRequired(taskMemberRemoveCmd, "members")
}
