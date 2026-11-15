package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var taskCommentCmd = &cobra.Command{
	Use:   "comment",
	Short: "任务评论管理",
	Long: `管理任务评论：添加、列出、查看、修改与删除。

子命令:
  add       添加评论
  list      列出评论
  get       查看单条评论
  update    修改评论内容
  delete    删除评论（需确认）

示例:
  feishu-cli task comment add TASK_GUID --content "这个任务需要加快进度"
  feishu-cli task comment list TASK_GUID
  feishu-cli task comment update COMMENT_ID --content "更正"
  feishu-cli task comment delete COMMENT_ID --yes`,
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
		taskGuid, guidErr := parseTaskGUIDArg(args[0])
		if guidErr != nil {
			return guidErr
		}
		content, _ := cmd.Flags().GetString("content")
		replyTo, _ := cmd.Flags().GetString("reply-to")
		output, _ := cmd.Flags().GetString("output")
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}
		if taskDryRunRequested(cmd) {
			body := map[string]any{"content": content, "resource_type": "task", "resource_id": taskGuid}
			if replyTo != "" {
				body["reply_to_comment_id"] = replyTo
			}
			return printDryRunPlan(cmd, "task comment add 预览（未执行）", nil, []dryRunStep{{
				Method: "POST", URL: taskAPIPath("comments"), Desc: "添加任务评论", Params: taskOpenIDParams(), Body: body,
			}})
		}

		if err := config.Validate(); err != nil {
			return err
		}
		token, tokenErr := resolveIdentityToken(cmd)
		if tokenErr != nil {
			return tokenErr
		}

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

		taskGuid, guidErr := parseTaskGUIDArg(args[0])
		if guidErr != nil {
			return guidErr
		}
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

// printTaskComment 输出单条评论（文本或 JSON）
func printTaskComment(c *client.CommentInfo, output, title string) error {
	if output == "json" {
		return printJSON(c)
	}
	fmt.Println(title)
	fmt.Printf("  评论 ID: %s\n", c.ID)
	fmt.Printf("  内容: %s\n", c.Content)
	if c.Creator != "" {
		fmt.Printf("  创建者: %s\n", c.Creator)
	}
	if c.ReplyToCommentID != "" {
		fmt.Printf("  回复评论: %s\n", c.ReplyToCommentID)
	}
	if c.CreatedAt != "" {
		fmt.Printf("  创建时间: %s\n", c.CreatedAt)
	}
	if c.UpdatedAt != "" {
		fmt.Printf("  更新时间: %s\n", c.UpdatedAt)
	}
	return nil
}

// parseTaskCommentIDArg 校验评论 ID 位置参数
func parseTaskCommentIDArg(raw string) (string, error) {
	id := strings.TrimSpace(raw)
	if id == "" {
		return "", clierr.Usagef("comment_id 不能为空")
	}
	// 评论 ID 为纯数字或字母数字串；含路径分隔符、空白、? # 等字符的一律拒绝，避免拼进请求路径
	if strings.ContainsAny(id, "/\\?#%& \t\n") {
		return "", clierr.Usagef("comment_id 格式无效: %q", id)
	}
	return id, nil
}

var taskCommentGetCmd = &cobra.Command{
	Use:   "get <comment_id>",
	Short: "获取任务评论详情",
	Long: `按评论 ID 获取任务评论详情（GET /task/v2/comments/{comment_id}）。

参数:
  comment_id    评论 ID（位置参数，来自 task comment list）

身份: 与 task comment list 一致，User 优先、未配置回退 Bot。

示例:
  feishu-cli task comment get COMMENT_ID
  feishu-cli task comment get COMMENT_ID -o json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		commentID, err := parseTaskCommentIDArg(args[0])
		if err != nil {
			return err
		}
		output, _ := cmd.Flags().GetString("output")
		if err := config.Validate(); err != nil {
			return err
		}
		token := resolveOptionalUserTokenWithFallback(cmd)
		c, err := client.GetTaskComment(commentID, token)
		if err != nil {
			return err
		}
		return printTaskComment(c, output, "评论详情:")
	},
}

var taskCommentUpdateCmd = &cobra.Command{
	Use:   "update <comment_id>",
	Short: "更新任务评论",
	Long: `更新任务评论内容（PATCH /task/v2/comments/{comment_id}，update_fields=["content"]）。
只能修改调用身份自己发表的评论。

参数:
  comment_id    评论 ID（位置参数）
  --content     新的评论内容（必填，最长 3000 字符）
  --dry-run     只预览请求，不执行（不联网、不读凭据）
  --as          身份：auto（默认）| user | bot

示例:
  feishu-cli task comment update COMMENT_ID --content "更正：周五上线"
  feishu-cli task comment update COMMENT_ID --content "x" --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		commentID, err := parseTaskCommentIDArg(args[0])
		if err != nil {
			return err
		}
		content, _ := cmd.Flags().GetString("content")
		output, _ := cmd.Flags().GetString("output")
		if strings.TrimSpace(content) == "" {
			return clierr.Usagef("--content 不能为空")
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}
		if taskDryRunRequested(cmd) {
			return printDryRunPlan(cmd, "task comment update 预览（未执行）", nil, []dryRunStep{{
				Method: "PATCH", URL: taskAPIPath("comments", commentID), Desc: "更新评论内容",
				Params: taskOpenIDParams(), Body: client.TaskCommentUpdateBody(content),
			}})
		}
		if err := config.Validate(); err != nil {
			return err
		}
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		c, err := client.UpdateTaskComment(commentID, content, token)
		if err != nil {
			return err
		}
		return printTaskComment(c, output, "评论更新成功！")
	},
}

var taskCommentDeleteCmd = &cobra.Command{
	Use:   "delete <comment_id>",
	Short: "删除任务评论",
	Long: `删除任务评论（DELETE /task/v2/comments/{comment_id}），不可恢复。
只能删除调用身份自己发表的评论。

确认门禁: 交互终端会询问确认；非交互环境（AI Agent / 管道）未带 --yes 时退出码 10、不执行。

参数:
  comment_id    评论 ID（位置参数）
  --dry-run     只预览请求，不执行（不联网、不读凭据）
  --as          身份：auto（默认）| user | bot

示例:
  feishu-cli task comment delete COMMENT_ID --dry-run
  feishu-cli task comment delete COMMENT_ID --yes`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		commentID, err := parseTaskCommentIDArg(args[0])
		if err != nil {
			return err
		}
		output, _ := cmd.Flags().GetString("output")
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}
		if taskDryRunRequested(cmd) {
			return printDryRunPlan(cmd, "task comment delete 预览（未执行）", nil, []dryRunStep{{
				Method: "DELETE", URL: taskAPIPath("comments", commentID), Desc: "删除评论（不可恢复）",
			}})
		}
		if err := config.Validate(); err != nil {
			return err
		}
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		if err := confirmDangerousAction(cmd, fmt.Sprintf("确认删除任务评论 %s？删除后不可恢复", commentID)); err != nil {
			return err
		}
		if err := client.DeleteTaskComment(commentID, token); err != nil {
			return err
		}
		if output == "json" {
			return printJSON(map[string]any{"success": true, "comment_id": commentID})
		}
		fmt.Printf("评论已删除: %s\n", commentID)
		return nil
	},
}

func init() {
	taskCmd.AddCommand(taskCommentCmd)

	taskCommentCmd.AddCommand(taskCommentGetCmd)
	taskCommentGetCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	taskCommentGetCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")

	taskCommentCmd.AddCommand(taskCommentUpdateCmd)
	taskCommentUpdateCmd.Flags().String("content", "", "新的评论内容（必填）")
	taskCommentUpdateCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	taskCommentUpdateCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	addWriteAsFlag(taskCommentUpdateCmd)
	addTaskDryRunFlag(taskCommentUpdateCmd)
	mustMarkFlagRequired(taskCommentUpdateCmd, "content")

	taskCommentCmd.AddCommand(taskCommentDeleteCmd)
	taskCommentDeleteCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	taskCommentDeleteCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	addWriteAsFlag(taskCommentDeleteCmd)
	addTaskDryRunFlag(taskCommentDeleteCmd)

	taskCommentCmd.AddCommand(taskCommentAddCmd)
	taskCommentAddCmd.Flags().String("content", "", "评论内容（必填）")
	taskCommentAddCmd.Flags().String("reply-to", "", "回复评论的 ID（可选）")
	taskCommentAddCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	taskCommentAddCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	addWriteAsFlag(taskCommentAddCmd)
	addTaskDryRunFlag(taskCommentAddCmd)
	mustMarkFlagRequired(taskCommentAddCmd, "content")

	taskCommentCmd.AddCommand(taskCommentListCmd)
	taskCommentListCmd.Flags().Int("page-size", 0, "每页数量")
	taskCommentListCmd.Flags().String("page-token", "", "分页标记")
	taskCommentListCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	taskCommentListCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
