package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var taskCommentCmd = &cobra.Command{
	Use:   "comment",
	Short: "任务评论管理",
	Long: `管理任务评论，支持添加和列出评论。

子命令:
  add       添加评论
  list      列出评论

示例:
  feishu-cli task comment add TASK_GUID --content "这个任务需要加快进度"
  feishu-cli task comment list TASK_GUID`,
}

var taskCommentAddCmd = &cobra.Command{
	Use:   "add <task_guid>",
	Short: "添加任务评论",
	Long: `为指定任务添加评论。

参数:
  task_guid     任务 GUID（位置参数）
  --content     评论内容（必填）
  --reply-to    回复某条评论的 ID（可选）

示例:
  feishu-cli task comment add TASK_GUID --content "进度正常"
  feishu-cli task comment add TASK_GUID --content "同意" --reply-to COMMENT_ID`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		taskGuid := args[0]
		content, _ := cmd.Flags().GetString("content")
		replyTo, _ := cmd.Flags().GetString("reply-to")
		output, _ := cmd.Flags().GetString("output")

		comment, err := client.AddTaskComment(taskGuid, content, replyTo, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(comment)
		}

		fmt.Println("评论添加成功！")
		fmt.Printf("  评论 ID: %s\n", comment.ID)
		fmt.Printf("  内容: %s\n", comment.Content)
		if comment.CreatedAt != "" {
			fmt.Printf("  创建时间: %s\n", comment.CreatedAt)
		}

		return nil
	},
}

var taskCommentListCmd = &cobra.Command{
	Use:   "list <task_guid>",
	Short: "列出任务评论",
	Long: `列出指定任务的所有评论。

参数:
  task_guid     任务 GUID（位置参数）

示例:
  feishu-cli task comment list TASK_GUID
  feishu-cli task comment list TASK_GUID -o json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserTokenWithFallback(cmd)

		taskGuid := args[0]
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		output, _ := cmd.Flags().GetString("output")

		comments, nextPageToken, hasMore, err := client.ListTaskComments(taskGuid, pageSize, pageToken, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(map[string]interface{}{
				"comments":        comments,
				"next_page_token": nextPageToken,
				"has_more":        hasMore,
			})
		}

		if len(comments) == 0 {
			fmt.Println("暂无评论")
			return nil
		}

		fmt.Printf("评论列表（共 %d 条）:\n\n", len(comments))
		for i, c := range comments {
			fmt.Printf("[%d] %s\n", i+1, c.Content)
			fmt.Printf("    ID: %s\n", c.ID)
			if c.Creator != "" {
				fmt.Printf("    创建者: %s\n", c.Creator)
			}
			if c.ReplyToCommentID != "" {
				fmt.Printf("    回复评论: %s\n", c.ReplyToCommentID)
			}
			if c.CreatedAt != "" {
				fmt.Printf("    创建时间: %s\n", c.CreatedAt)
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
	taskCmd.AddCommand(taskCommentCmd)

	taskCommentCmd.AddCommand(taskCommentAddCmd)
	taskCommentAddCmd.Flags().String("content", "", "评论内容（必填）")
	taskCommentAddCmd.Flags().String("reply-to", "", "回复评论的 ID（可选）")
	taskCommentAddCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	taskCommentAddCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	mustMarkFlagRequired(taskCommentAddCmd, "content")

	taskCommentCmd.AddCommand(taskCommentListCmd)
	taskCommentListCmd.Flags().Int("page-size", 0, "每页数量")
	taskCommentListCmd.Flags().String("page-token", "", "分页标记")
	taskCommentListCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	taskCommentListCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
