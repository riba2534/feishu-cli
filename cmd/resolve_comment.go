package cmd

import (
	"fmt"
	"net/url"

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
  --dry-run     只打印将要发出的请求（不读取凭据、不联网）

示例:
  feishu-cli comment resolve doccnXXX 6916106822734578184 --type docx
  feishu-cli comment resolve doccnXXX 6916106822734578184 --type docx --dry-run`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		fileToken := args[0]
		commentID := args[1]
		fileType, _ := cmd.Flags().GetString("type")
		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			return printDryRunPlan(cmd, "Set comment is_solved=true", nil, []dryRunStep{{
				Method: "PATCH",
				URL:    "/open-apis/drive/v1/files/" + url.PathEscape(fileToken) + "/comments/" + url.PathEscape(commentID),
				Params: map[string]any{"file_type": fileType},
				Body:   map[string]any{"is_solved": true},
			}})
		}

		if err := config.Validate(); err != nil {
			return err
		}

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
  --dry-run     只打印将要发出的请求（不读取凭据、不联网）

示例:
  feishu-cli comment unresolve doccnXXX 6916106822734578184 --type docx
  feishu-cli comment unresolve doccnXXX 6916106822734578184 --type docx --dry-run`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		fileToken := args[0]
		commentID := args[1]
		fileType, _ := cmd.Flags().GetString("type")
		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			return printDryRunPlan(cmd, "Set comment is_solved=false", nil, []dryRunStep{{
				Method: "PATCH",
				URL:    "/open-apis/drive/v1/files/" + url.PathEscape(fileToken) + "/comments/" + url.PathEscape(commentID),
				Params: map[string]any{"file_type": fileType},
				Body:   map[string]any{"is_solved": false},
			}})
		}

		if err := config.Validate(); err != nil {
			return err
		}

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
	resolveCommentCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求")

	commentCmd.AddCommand(unresolveCommentCmd)
	unresolveCommentCmd.Flags().String("type", "docx", "文件类型（doc/docx/sheet/bitable）")
	unresolveCommentCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求")
}
