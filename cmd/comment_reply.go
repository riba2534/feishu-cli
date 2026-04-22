package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var replyCmd = &cobra.Command{
	Use:   "reply",
	Short: "评论回复管理",
	Long: `评论回复管理命令，包括列出、添加、删除回复。

子命令:
  list      列出评论的回复
  add       为已有评论添加回复
  delete    删除评论回复

示例:
  # 列出评论回复
  feishu-cli comment reply list <file_token> <comment_id> --type docx

  # 添加评论回复（推荐登录后使用，以用户身份发布）
  feishu-cli comment reply add <file_token> <comment_id> --text "回复内容"

  # 删除评论回复（飞书只允许回复作者删除，需 User Token）
  feishu-cli comment reply delete <file_token> <comment_id> <reply_id> --type docx`,
}

var listReplyCmd = &cobra.Command{
	Use:   "list <file_token> <comment_id>",
	Short: "列出评论回复",
	Long: `列出指定评论的所有回复。

参数:
  file_token    文档 Token
  comment_id    评论 ID

示例:
  feishu-cli comment reply list doccnXXX 6916106822734578184 --type docx`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken := args[0]
		commentID := args[1]
		fileType, _ := cmd.Flags().GetString("type")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		output, _ := cmd.Flags().GetString("output")
		userAccessToken := resolveOptionalUserToken(cmd)

		replies, _, _, err := client.ListCommentReplies(fileToken, commentID, fileType, pageSize, "", userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(replies)
		}

		if len(replies) == 0 {
			fmt.Println("暂无回复")
			return nil
		}

		fmt.Printf("共 %d 条回复:\n\n", len(replies))
		for i, r := range replies {
			fmt.Printf("[%d] 回复 ID: %s\n", i+1, r.ReplyID)
			if r.UserID != "" {
				fmt.Printf("    用户 ID: %s\n", r.UserID)
			}
			if r.Content != "" {
				fmt.Printf("    内容:     %s\n", r.Content)
			}
			if r.CreateTime != 0 {
				fmt.Printf("    创建时间: %d\n", r.CreateTime)
			}
			fmt.Println()
		}

		return nil
	},
}

var addReplyCmd = &cobra.Command{
	Use:   "add <file_token> <comment_id>",
	Short: "为已有评论添加回复",
	Long: `为已有评论追加一条回复。

参数:
  file_token    文档 Token
  comment_id    评论 ID

建议使用 User Access Token（登录态），回复会以用户身份发出；否则以 App/Bot 身份发出，
且该回复只能被同一 App 自己删除（Bot 身份经常收到 1069303 forbidden）。

示例:
  # 登录后自动使用 User Token（推荐）
  feishu-cli auth login
  feishu-cli comment reply add doccnXXX 6916106822734578184 \
    --text "已处理，请查看最新版本。" --type docx

  # 显式传入 User Token
  feishu-cli comment reply add doccnXXX 6916106822734578184 \
    --text "回复内容" --user-access-token "u-xxxxx"`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken := args[0]
		commentID := args[1]
		fileType, _ := cmd.Flags().GetString("type")
		text, _ := cmd.Flags().GetString("text")
		output, _ := cmd.Flags().GetString("output")

		if text == "" {
			return fmt.Errorf("回复内容不能为空，请通过 --text 提供")
		}

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		reply, err := client.CreateCommentReply(fileToken, commentID, fileType, text, userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(reply)
		}

		fmt.Printf("回复添加成功！\n")
		fmt.Printf("  文档 Token: %s\n", fileToken)
		fmt.Printf("  评论 ID:    %s\n", commentID)
		fmt.Printf("  回复 ID:    %s\n", reply.ReplyID)
		if reply.UserID != "" {
			fmt.Printf("  用户 ID:    %s\n", reply.UserID)
		}

		return nil
	},
}

var deleteReplyCmd = &cobra.Command{
	Use:   "delete <file_token> <comment_id> <reply_id>",
	Short: "删除评论回复",
	Long: `删除指定的评论回复。

参数:
  file_token    文档 Token
  comment_id    评论 ID
  reply_id      回复 ID

注意：飞书 Open API 只允许回复作者本人删除；使用 App Token（Bot 身份）删除用户回复会得到
1069303 forbidden。通常需要先 feishu-cli auth login 或显式提供 --user-access-token。

示例:
  feishu-cli comment reply delete doccnXXX 6916106822734578184 6916106822734594568 --type docx`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken := args[0]
		commentID := args[1]
		replyID := args[2]
		fileType, _ := cmd.Flags().GetString("type")
		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		if err := client.DeleteCommentReply(fileToken, commentID, replyID, fileType, userAccessToken); err != nil {
			return err
		}

		fmt.Printf("回复删除成功！\n")
		fmt.Printf("  文档 Token: %s\n", fileToken)
		fmt.Printf("  评论 ID:    %s\n", commentID)
		fmt.Printf("  回复 ID:    %s\n", replyID)

		return nil
	},
}

func init() {
	commentCmd.AddCommand(replyCmd)

	replyCmd.AddCommand(listReplyCmd)
	listReplyCmd.Flags().String("type", "docx", "文件类型（doc/docx/sheet/bitable）")
	listReplyCmd.Flags().Int("page-size", 50, "每页数量")
	listReplyCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	listReplyCmd.Flags().String("user-access-token", "", "User Access Token（覆盖 App Token）")

	replyCmd.AddCommand(addReplyCmd)
	addReplyCmd.Flags().String("type", "docx", "文件类型（doc/docx/sheet/bitable）")
	addReplyCmd.Flags().String("text", "", "回复内容（必填）")
	addReplyCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	addReplyCmd.Flags().String("user-access-token", "", "User Access Token（推荐，以用户身份发布）")
	_ = addReplyCmd.MarkFlagRequired("text")

	replyCmd.AddCommand(deleteReplyCmd)
	deleteReplyCmd.Flags().String("type", "docx", "文件类型（doc/docx/sheet/bitable）")
	deleteReplyCmd.Flags().String("user-access-token", "", "User Access Token（删除用户回复时必需）")
}
