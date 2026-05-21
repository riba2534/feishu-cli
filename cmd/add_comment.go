package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var addCommentCmd = &cobra.Command{
	Use:   "add <file_token>",
	Short: "添加评论",
	Long: `为文档添加全文评论。

参数:
  file_token    文档 Token
  --type        文件类型（必填）
  --text        评论内容（必填）

身份说明:
  默认以 App Token（Bot 身份）创建；推荐 feishu-cli auth login 后传 --user-access-token，
  以用户身份创建评论。Bot 身份创建的评论只能被同一 App 自身删除。

示例:
  # 添加评论（Bot 身份）
  feishu-cli comment add doccnXXX --type docx --text "这是一条评论"

  # 添加评论（用户身份，推荐）
  feishu-cli auth login
  feishu-cli comment add doccnXXX --type docx --text "这是一条评论" \
    --user-access-token "u-xxxxx"

  # JSON 格式输出
  feishu-cli comment add doccnXXX --type docx --text "评论内容" --output json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken := args[0]
		fileType, _ := cmd.Flags().GetString("type")
		text, _ := cmd.Flags().GetString("text")
		output, _ := cmd.Flags().GetString("output")
		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		commentID, err := client.CreateComment(fileToken, fileType, text, userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			if err := printJSON(map[string]string{
				"comment_id": commentID,
			}); err != nil {
				return err
			}
		} else {
			fmt.Printf("评论添加成功！\n")
			fmt.Printf("  评论 ID: %s\n", commentID)
		}

		return nil
	},
}

func init() {
	commentCmd.AddCommand(addCommentCmd)
	addCommentCmd.Flags().String("type", "", "文件类型（必填: doc/docx/sheet/bitable）")
	addCommentCmd.Flags().String("text", "", "评论内容（必填）")
	addCommentCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mustMarkFlagRequired(addCommentCmd, "type", "text")
}
