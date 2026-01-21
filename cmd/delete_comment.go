package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var deleteCommentCmd = &cobra.Command{
	Use:   "delete <file_token> <comment_id>",
	Short: "删除评论",
	Long: `删除文档中的指定评论。

参数:
  file_token    文档 Token
  comment_id    评论 ID
  --type        文件类型（必填）

文件类型:
  doc       旧版文档
  docx      新版文档
  sheet     电子表格
  bitable   多维表格

示例:
  # 删除评论
  feishu-cli comment delete doccnXXX comment123 --type docx`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken := args[0]
		commentID := args[1]
		fileType, _ := cmd.Flags().GetString("type")

		if err := client.DeleteComment(fileToken, commentID, fileType); err != nil {
			return err
		}

		fmt.Printf("评论删除成功！\n")
		fmt.Printf("  文档 Token: %s\n", fileToken)
		fmt.Printf("  评论 ID:    %s\n", commentID)
		return nil
	},
}

func init() {
	commentCmd.AddCommand(deleteCommentCmd)
	deleteCommentCmd.Flags().String("type", "", "文件类型（必填: doc/docx/sheet/bitable）")
	_ = deleteCommentCmd.MarkFlagRequired("type")
}
