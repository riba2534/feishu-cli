package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var tasklistCmd = &cobra.Command{
	Use:   "tasklist",
	Short: "任务清单操作命令",
	Long: `任务清单操作命令，用于创建、查看、列出和删除任务清单。

子命令:
  create    创建任务清单
  get       获取清单详情
  list      列出所有清单
  delete    删除清单

示例:
  feishu-cli tasklist create --name "项目清单"
  feishu-cli tasklist get GUID
  feishu-cli tasklist list
  feishu-cli tasklist delete GUID`,
}

var tasklistCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建任务清单",
	Long: `创建新的任务清单。

参数:
  --name, -n    清单名称（必填）

示例:
  feishu-cli tasklist create --name "项目清单"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		name, _ := cmd.Flags().GetString("name")
		output, _ := cmd.Flags().GetString("output")

		tl, err := client.CreateTasklist(name, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(tl)
		}

		fmt.Println("任务清单创建成功！")
		fmt.Printf("  清单 ID: %s\n", tl.Guid)
		fmt.Printf("  名称: %s\n", tl.Name)
		if tl.Url != "" {
			fmt.Printf("  链接: %s\n", tl.Url)
		}

		return nil
	},
}

var tasklistGetCmd = &cobra.Command{
	Use:   "get <tasklist_guid>",
	Short: "获取清单详情",
	Long: `获取任务清单的详细信息。

示例:
  feishu-cli tasklist get GUID
  feishu-cli tasklist get GUID -o json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		tasklistGuid := args[0]
		output, _ := cmd.Flags().GetString("output")

		tl, err := client.GetTasklist(tasklistGuid, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(tl)
		}

		fmt.Printf("清单 ID:   %s\n", tl.Guid)
		fmt.Printf("名称:      %s\n", tl.Name)
		if tl.Creator != "" {
			fmt.Printf("创建者:    %s\n", tl.Creator)
		}
		if tl.Owner != "" {
			fmt.Printf("负责人:    %s\n", tl.Owner)
		}
		if tl.Url != "" {
			fmt.Printf("链接:      %s\n", tl.Url)
		}
		if tl.CreatedAt != "" {
			fmt.Printf("创建时间:  %s\n", tl.CreatedAt)
		}
		if tl.UpdatedAt != "" {
			fmt.Printf("更新时间:  %s\n", tl.UpdatedAt)
		}

		return nil
	},
}

var tasklistListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出任务清单",
	Long: `列出所有任务清单。

示例:
  feishu-cli tasklist list
  feishu-cli tasklist list -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		output, _ := cmd.Flags().GetString("output")

		lists, nextPageToken, hasMore, err := client.ListTasklists(pageSize, pageToken, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(map[string]interface{}{
				"tasklists":       lists,
				"next_page_token": nextPageToken,
				"has_more":        hasMore,
			})
		}

		if len(lists) == 0 {
			fmt.Println("暂无任务清单")
			return nil
		}

		fmt.Printf("任务清单列表（共 %d 个）:\n\n", len(lists))
		for i, tl := range lists {
			fmt.Printf("[%d] %s\n", i+1, tl.Name)
			fmt.Printf("    ID: %s\n", tl.Guid)
			if tl.CreatedAt != "" {
				fmt.Printf("    创建时间: %s\n", tl.CreatedAt)
			}
			fmt.Println()
		}

		if hasMore {
			fmt.Printf("下一页 token: %s\n", nextPageToken)
		}

		return nil
	},
}

var tasklistDeleteCmd = &cobra.Command{
	Use:   "delete <tasklist_guid>",
	Short: "删除任务清单",
	Long: `删除指定的任务清单。

示例:
  feishu-cli tasklist delete GUID`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		tasklistGuid := args[0]

		if err := client.DeleteTasklist(tasklistGuid, token); err != nil {
			return err
		}

		fmt.Println("任务清单删除成功")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(tasklistCmd)

	tasklistCmd.AddCommand(tasklistCreateCmd)
	tasklistCreateCmd.Flags().StringP("name", "n", "", "清单名称（必填）")
	tasklistCreateCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	tasklistCreateCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	mustMarkFlagRequired(tasklistCreateCmd, "name")

	tasklistCmd.AddCommand(tasklistGetCmd)
	tasklistGetCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	tasklistGetCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")

	tasklistCmd.AddCommand(tasklistListCmd)
	tasklistListCmd.Flags().Int("page-size", 0, "每页数量")
	tasklistListCmd.Flags().String("page-token", "", "分页标记")
	tasklistListCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	tasklistListCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")

	tasklistCmd.AddCommand(tasklistDeleteCmd)
	tasklistDeleteCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
