package cmd

import (
	"fmt"
	"net/url"

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
  --dry-run     只打印将要发出的请求（不读取凭据、不联网）

身份说明:
  User 优先：--user-access-token > FEISHU_USER_ACCESS_TOKEN > auth login 保存的 token.json
  （过期自动刷新）> config.yaml 的 user_access_token；都未配置时以 App Token（Bot 身份）创建。
  已配置 User Token 但不可用（如刷新失败）时会在 stderr 告警后改用 App Token。
  Bot 身份创建的评论只能被同一 App 自身删除；App 未被加为协作者的个人文档会得到 1069303 forbidden。

示例:
  # 添加评论（已 auth login 时自动以用户身份创建）
  feishu-cli auth login
  feishu-cli comment add doccnXXX --type docx --text "这是一条评论"

  # 显式指定 User Token
  feishu-cli comment add doccnXXX --type docx --text "这是一条评论" \
    --user-access-token "u-xxxxx"

  # 预览请求
  feishu-cli comment add doccnXXX --type docx --text "评论内容" --dry-run

  # JSON 格式输出
  feishu-cli comment add doccnXXX --type docx --text "评论内容" --output json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fileToken := args[0]
		fileType, _ := cmd.Flags().GetString("type")
		text, _ := cmd.Flags().GetString("text")
		output, _ := cmd.Flags().GetString("output")
		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			return printDryRunPlan(cmd, "Create a whole-document comment", nil, []dryRunStep{{
				Method: "POST",
				URL:    "/open-apis/drive/v1/files/" + url.PathEscape(fileToken) + "/comments",
				Params: map[string]any{"file_type": fileType},
				Body: map[string]any{"reply_list": map[string]any{"replies": []map[string]any{{
					"content": map[string]any{"elements": []map[string]any{{"type": "text_run", "text_run": map[string]any{"text": text}}}},
				}}}},
			}})
		}

		if err := config.Validate(); err != nil {
			return err
		}

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
	addCommentCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求")
	mustMarkFlagRequired(addCommentCmd, "type", "text")
}
