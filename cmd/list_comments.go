package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var listCommentsCmd = &cobra.Command{
	Use:   "list <file_token>",
	Short: "列出文档评论",
	Long: `列出指定文档的所有评论。

参数:
  file_token    文档 Token
  --type        文件类型（必填）

示例:
  # 列出文档评论
  feishu-cli comment list doccnXXX --type docx

  # JSON 格式输出
  feishu-cli comment list doccnXXX --type docx --output json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken := args[0]
		fileType, _ := cmd.Flags().GetString("type")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		output, _ := cmd.Flags().GetString("output")

		comments, _, _, err := client.ListComments(fileToken, fileType, pageSize, "")
		if err != nil {
			return err
		}

		if output == "json" {
			data, _ := json.MarshalIndent(comments, "", "  ")
			fmt.Println(string(data))
		} else {
			if len(comments) == 0 {
				fmt.Println("该文档暂无评论")
				return nil
			}

			fmt.Printf("共找到 %d 条评论:\n\n", len(comments))
			for i, c := range comments {
				status := "未解决"
				if c.IsSolved {
					status = "已解决"
				}
				scope := "局部评论"
				if c.IsWhole {
					scope = "全文评论"
				}
				fmt.Printf("[%d] 评论 ID: %s\n", i+1, c.CommentID)
				fmt.Printf("    状态:     %s\n", status)
				fmt.Printf("    类型:     %s\n", scope)
				if c.CreateTime > 0 {
					t := time.Unix(int64(c.CreateTime), 0)
					fmt.Printf("    创建时间: %s\n", t.Format("2006-01-02 15:04:05"))
				}
				fmt.Println()
			}
		}

		return nil
	},
}

func init() {
	commentCmd.AddCommand(listCommentsCmd)
	listCommentsCmd.Flags().String("type", "", "文件类型（必填: doc/docx/sheet/bitable）")
	listCommentsCmd.Flags().Int("page-size", 50, "每页数量")
	listCommentsCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mustMarkFlagRequired(listCommentsCmd, "type")
}
