package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var createDocumentCmd = &cobra.Command{
	Use:   "create",
	Short: "创建新文档",
	Long: `创建新的飞书云文档。

参数:
  --title, -t     文档标题（必填）
  --folder, -f    目标文件夹 token（可选）
  --output, -o    输出格式，可选 json

示例:
  # 创建空白文档
  feishu-cli doc create --title "我的文档"

  # 在指定文件夹创建
  feishu-cli doc create --title "项目文档" --folder FOLDER_TOKEN

  # JSON 格式输出
  feishu-cli doc create --title "测试" --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		title, _ := cmd.Flags().GetString("title")
		folder, _ := cmd.Flags().GetString("folder")

		doc, err := client.CreateDocument(title, folder)
		if err != nil {
			return err
		}

		documentID := ""
		docTitle := ""
		var revisionID int
		if doc.DocumentId != nil {
			documentID = *doc.DocumentId
		}
		if doc.Title != nil {
			docTitle = *doc.Title
		}
		if doc.RevisionId != nil {
			revisionID = *doc.RevisionId
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			if err := printJSON(map[string]interface{}{
				"document_id": documentID,
				"title":       docTitle,
				"revision_id": revisionID,
			}); err != nil {
				return err
			}
		} else {
			fmt.Printf("文档创建成功！\n")
			fmt.Printf("  文档 ID: %s\n", documentID)
			fmt.Printf("  标题: %s\n", docTitle)
			fmt.Printf("  版本: %d\n", revisionID)
			fmt.Printf("  链接: https://feishu.cn/docx/%s\n", documentID)
		}

		return nil
	},
}

func init() {
	docCmd.AddCommand(createDocumentCmd)
	createDocumentCmd.Flags().StringP("title", "t", "", "文档标题（必填）")
	createDocumentCmd.Flags().StringP("folder", "f", "", "目标文件夹 token")
	createDocumentCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	_ = createDocumentCmd.MarkFlagRequired("title")
}
