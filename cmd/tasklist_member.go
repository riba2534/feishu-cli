package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var tasklistMemberCmd = &cobra.Command{
	Use:   "member",
	Short: "任务清单成员管理",
	Long: `管理任务清单成员，支持添加、移除成员。

子命令:
  add       添加成员
  remove    移除成员

示例:
  feishu-cli tasklist member add TASKLIST_GUID --members ou_xxx,ou_yyy --role editor
  feishu-cli tasklist member remove TASKLIST_GUID --members ou_xxx --role editor`,
}

var tasklistMemberAddCmd = &cobra.Command{
	Use:   "add <tasklist_guid>",
	Short: "添加清单成员",
	Long: `向任务清单添加成员。

参数:
  tasklist_guid     清单 GUID（位置参数）
  --members         成员 ID 列表，逗号分隔（必填）
  --role            角色: editor（编辑者）或 viewer（查看者），默认 editor

示例:
  feishu-cli tasklist member add TASKLIST_GUID --members ou_xxx,ou_yyy
  feishu-cli tasklist member add TASKLIST_GUID --members ou_xxx --role viewer`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token, tokenErr := resolveIdentityToken(cmd)
		if tokenErr != nil {
			return tokenErr
		}

		tasklistGuid := args[0]
		membersStr, _ := cmd.Flags().GetString("members")
		role, _ := cmd.Flags().GetString("role")
		output, _ := cmd.Flags().GetString("output")

		if role != "editor" && role != "viewer" {
			return fmt.Errorf("无效的角色: %s（仅支持 editor 或 viewer）", role)
		}

		memberIDs := splitAndTrim(membersStr)

		if len(memberIDs) == 0 {
			return fmt.Errorf("成员列表不能为空")
		}

		tl, err := client.AddTasklistMembers(tasklistGuid, memberIDs, role, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(tl)
		}

		fmt.Printf("成功添加 %d 个%s到清单「%s」\n", len(memberIDs), tasklistRoleLabel(role), tl.Name)
		return nil
	},
}

var tasklistMemberRemoveCmd = &cobra.Command{
	Use:   "remove <tasklist_guid>",
	Short: "移除清单成员",
	Long: `从任务清单中移除成员。

参数:
  tasklist_guid     清单 GUID（位置参数）
  --members         成员 ID 列表，逗号分隔（必填）
  --role            角色: editor（编辑者）或 viewer（查看者），默认 editor

示例:
  feishu-cli tasklist member remove TASKLIST_GUID --members ou_xxx
  feishu-cli tasklist member remove TASKLIST_GUID --members ou_xxx --role viewer`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token, tokenErr := resolveIdentityToken(cmd)
		if tokenErr != nil {
			return tokenErr
		}

		tasklistGuid := args[0]
		membersStr, _ := cmd.Flags().GetString("members")
		role, _ := cmd.Flags().GetString("role")
		output, _ := cmd.Flags().GetString("output")

		if role != "editor" && role != "viewer" {
			return fmt.Errorf("无效的角色: %s（仅支持 editor 或 viewer）", role)
		}

		memberIDs := splitAndTrim(membersStr)

		if len(memberIDs) == 0 {
			return fmt.Errorf("成员列表不能为空")
		}

		tl, err := client.RemoveTasklistMembers(tasklistGuid, memberIDs, role, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(tl)
		}

		fmt.Printf("成功从清单「%s」移除 %d 个%s\n", tl.Name, len(memberIDs), tasklistRoleLabel(role))
		return nil
	},
}

func tasklistRoleLabel(role string) string {
	if role == "viewer" {
		return "查看者"
	}
	return "编辑者"
}

func init() {
	tasklistCmd.AddCommand(tasklistMemberCmd)

	tasklistMemberCmd.AddCommand(tasklistMemberAddCmd)
	tasklistMemberAddCmd.Flags().String("members", "", "成员 ID 列表，逗号分隔（必填）")
	tasklistMemberAddCmd.Flags().String("role", "editor", "角色: editor/viewer")
	tasklistMemberAddCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	tasklistMemberAddCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	addWriteAsFlag(tasklistMemberAddCmd)
	mustMarkFlagRequired(tasklistMemberAddCmd, "members")

	tasklistMemberCmd.AddCommand(tasklistMemberRemoveCmd)
	tasklistMemberRemoveCmd.Flags().String("members", "", "成员 ID 列表，逗号分隔（必填）")
	tasklistMemberRemoveCmd.Flags().String("role", "editor", "角色: editor/viewer")
	tasklistMemberRemoveCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	tasklistMemberRemoveCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	addWriteAsFlag(tasklistMemberRemoveCmd)
	mustMarkFlagRequired(tasklistMemberRemoveCmd, "members")
}
