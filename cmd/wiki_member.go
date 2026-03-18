package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var wikiMemberCmd = &cobra.Command{
	Use:   "member",
	Short: "知识空间成员管理",
	Long: `管理知识空间成员，支持添加、列出和移除成员。

子命令:
  add       添加成员
  list      列出成员
  remove    移除成员

成员类型 (--member-type):
  openchat/chat_id      群 ID
  userid/user_id        用户 ID
  email                 邮箱
  opendepartmentid      部门 ID
  openid/open_id        Open ID

角色 (--role):
  admin    管理员
  member   成员

示例:
  feishu-cli wiki member add SPACE_ID --member-type email --member-id user@example.com --role member
  feishu-cli wiki member list SPACE_ID
  feishu-cli wiki member remove SPACE_ID --member-type email --member-id user@example.com --role member`,
}

var wikiMemberAddCmd = &cobra.Command{
	Use:   "add <space_id>",
	Short: "添加知识空间成员",
	Long: `向知识空间添加成员。

参数:
  space_id          知识空间 ID（位置参数）
  --member-type     成员类型（必填）
  --member-id       成员 ID（必填）
  --role            角色: admin/member（必填）

示例:
  feishu-cli wiki member add SPACE_ID --member-type email --member-id user@example.com --role member`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		spaceID := args[0]
		memberType, _ := cmd.Flags().GetString("member-type")
		memberID, _ := cmd.Flags().GetString("member-id")
		role, _ := cmd.Flags().GetString("role")

		memberType = normalizePermMemberType(memberType)

		if err := client.AddWikiSpaceMember(spaceID, memberType, memberID, role, resolveOptionalUserToken(cmd)); err != nil {
			return err
		}

		fmt.Printf("成功添加知识空间成员: %s (%s)\n", memberID, role)
		return nil
	},
}

var wikiMemberListCmd = &cobra.Command{
	Use:   "list <space_id>",
	Short: "列出知识空间成员",
	Long: `列出知识空间的所有成员。

参数:
  space_id    知识空间 ID（位置参数）

示例:
  feishu-cli wiki member list SPACE_ID
  feishu-cli wiki member list SPACE_ID -o json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		spaceID := args[0]
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		output, _ := cmd.Flags().GetString("output")

		members, nextPageToken, hasMore, err := client.ListWikiSpaceMembers(spaceID, pageSize, pageToken, resolveOptionalUserToken(cmd))
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(map[string]interface{}{
				"members":         members,
				"next_page_token": nextPageToken,
				"has_more":        hasMore,
			})
		}

		if len(members) == 0 {
			fmt.Println("暂无成员")
			return nil
		}

		fmt.Printf("知识空间成员（共 %d 个）:\n\n", len(members))
		for i, m := range members {
			fmt.Printf("[%d] %s\n", i+1, m.MemberID)
			fmt.Printf("    类型: %s\n", m.MemberType)
			fmt.Printf("    角色: %s\n", m.MemberRole)
			fmt.Println()
		}

		if hasMore {
			fmt.Printf("下一页 token: %s\n", nextPageToken)
		}

		return nil
	},
}

var wikiMemberRemoveCmd = &cobra.Command{
	Use:   "remove <space_id>",
	Short: "移除知识空间成员",
	Long: `从知识空间中移除成员。

参数:
  space_id          知识空间 ID（位置参数）
  --member-type     成员类型（必填）
  --member-id       成员 ID（必填）
  --role            角色: admin/member（必填）

示例:
  feishu-cli wiki member remove SPACE_ID --member-type email --member-id user@example.com --role member`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		spaceID := args[0]
		memberType, _ := cmd.Flags().GetString("member-type")
		memberID, _ := cmd.Flags().GetString("member-id")
		role, _ := cmd.Flags().GetString("role")

		memberType = normalizePermMemberType(memberType)

		if err := client.RemoveWikiSpaceMember(spaceID, memberType, memberID, role, resolveOptionalUserToken(cmd)); err != nil {
			return err
		}

		fmt.Printf("成功移除知识空间成员: %s\n", memberID)
		return nil
	},
}

func init() {
	wikiCmd.AddCommand(wikiMemberCmd)

	wikiMemberCmd.AddCommand(wikiMemberAddCmd)
	wikiMemberAddCmd.Flags().String("member-type", "", "成员类型（必填）")
	wikiMemberAddCmd.Flags().String("member-id", "", "成员 ID（必填）")
	wikiMemberAddCmd.Flags().String("role", "", "角色: admin/member（必填）")
	mustMarkFlagRequired(wikiMemberAddCmd, "member-type", "member-id", "role")
	wikiMemberAddCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问个人知识库）")

	wikiMemberCmd.AddCommand(wikiMemberListCmd)
	wikiMemberListCmd.Flags().Int("page-size", 0, "每页数量")
	wikiMemberListCmd.Flags().String("page-token", "", "分页标记")
	wikiMemberListCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	wikiMemberListCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问个人知识库）")

	wikiMemberCmd.AddCommand(wikiMemberRemoveCmd)
	wikiMemberRemoveCmd.Flags().String("member-type", "", "成员类型（必填）")
	wikiMemberRemoveCmd.Flags().String("member-id", "", "成员 ID（必填）")
	wikiMemberRemoveCmd.Flags().String("role", "", "角色: admin/member（必填）")
	mustMarkFlagRequired(wikiMemberRemoveCmd, "member-type", "member-id", "role")
	wikiMemberRemoveCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问个人知识库）")
}
