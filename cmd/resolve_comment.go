package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var resolveCommentCmd = &cobra.Command{
	Use:   "resolve <file_token> <comment_id>",
	Short: "标记评论为已解决",
	Long: `将指定评论标记为已解决状态。

参数:
  file_token    文档 Token
  comment_id    评论 ID

示例:
  feishu-cli comment resolve doccnXXX 6916106822734578184 --type docx`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken := args[0]
		commentID := args[1]
		fileType, _ := cmd.Flags().GetString("type")
		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		if err := client.PatchComment(fileToken, commentID, fileType, true, userAccessToken); err != nil {
			return err
		}

		fmt.Printf("评论已标记为已解决！\n")
		fmt.Printf("  文档 Token: %s\n", fileToken)
		fmt.Printf("  评论 ID:    %s\n", commentID)

		return nil
	},
}

var unresolveCommentCmd = &cobra.Command{
	Use:   "unresolve <file_token> <comment_id>",
	Short: "标记评论为未解决",
	Long: `将指定评论标记为未解决状态。

参数:
  file_token    文档 Token
  comment_id    评论 ID

示例:
  feishu-cli comment unresolve doccnXXX 6916106822734578184 --type docx`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken := args[0]
		commentID := args[1]
		fileType, _ := cmd.Flags().GetString("type")
		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		if err := client.PatchComment(fileToken, commentID, fileType, false, userAccessToken); err != nil {
			return err
		}

		fmt.Printf("评论已标记为未解决！\n")
		fmt.Printf("  文档 Token: %s\n", fileToken)
		fmt.Printf("  评论 ID:    %s\n", commentID)

		return nil
	},
}

func init() {
	commentCmd.AddCommand(resolveCommentCmd)
	resolveCommentCmd.Flags().String("type", "docx", "文件类型（doc/docx/sheet/bitable）")

	commentCmd.AddCommand(unresolveCommentCmd)
	unresolveCommentCmd.Flags().String("type", "docx", "文件类型（doc/docx/sheet/bitable）")
}
