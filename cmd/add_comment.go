package cmd

import (
	"encoding/json"
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

示例:
  # 添加评论
  feishu-cli comment add doccnXXX --type docx --text "这是一条评论"

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

		commentID, err := client.CreateComment(fileToken, fileType, text)
		if err != nil {
			return err
		}

		if output == "json" {
			data, _ := json.MarshalIndent(map[string]string{
				"comment_id": commentID,
			}, "", "  ")
			fmt.Println(string(data))
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
