package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var userListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出部门下的用户",
	Long: `列出指定部门下的用户列表。

参数:
  --department-id   部门 ID（必填）
  --user-id-type    用户 ID 类型: open_id/union_id/user_id（默认 open_id）
  --page-size       每页数量
  --page-token      分页标记

示例:
  feishu-cli user list --department-id od_xxx
  feishu-cli user list --department-id 0 -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		departmentID, _ := cmd.Flags().GetString("department-id")
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		output, _ := cmd.Flags().GetString("output")

		users, nextPageToken, hasMore, err := client.ListUsers(departmentID, userIDType, pageSize, pageToken)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(map[string]interface{}{
				"users":           users,
				"next_page_token": nextPageToken,
				"has_more":        hasMore,
			})
		}

		if len(users) == 0 {
			fmt.Println("该部门下暂无用户")
			return nil
		}

		fmt.Printf("用户列表（共 %d 个）:\n\n", len(users))
		for i, u := range users {
			fmt.Printf("[%d] %s", i+1, u.Name)
			if u.EnName != "" {
				fmt.Printf(" (%s)", u.EnName)
			}
			fmt.Println()
			if u.OpenID != "" {
				fmt.Printf("    Open ID: %s\n", u.OpenID)
			}
			if u.Email != "" {
				fmt.Printf("    邮箱: %s\n", u.Email)
			}
			if u.JobTitle != "" {
				fmt.Printf("    职位: %s\n", u.JobTitle)
			}
			if u.Status != "" {
				fmt.Printf("    状态: %s\n", u.Status)
			}
			fmt.Println()
		}

		if hasMore {
			fmt.Printf("下一页 token: %s\n", nextPageToken)
		}

		return nil
	},
}

func init() {
	userCmd.AddCommand(userListCmd)
	userListCmd.Flags().String("department-id", "", "部门 ID（必填）")
	userListCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型: open_id/union_id/user_id")
	userListCmd.Flags().Int("page-size", 0, "每页数量")
	userListCmd.Flags().String("page-token", "", "分页标记")
	userListCmd.Flags().StringP("output", "o", "", "输出格式（json）")

	mustMarkFlagRequired(userListCmd, "department-id")
}
