package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var getDocumentCmd = &cobra.Command{
	Use:   "get <document_id>",
	Short: "获取文档信息",
	Long: `获取飞书文档的基本信息，包括文档ID、标题、版本号等。

示例:
  feishu-cli doc get ABC123def456
  feishu-cli doc get ABC123def456 -o json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		docID := args[0]
		doc, err := client.GetDocument(docID)
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
			data, _ := json.MarshalIndent(doc, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("文档信息:\n")
			fmt.Printf("  文档ID: %s\n", documentID)
			fmt.Printf("  标题: %s\n", docTitle)
			fmt.Printf("  版本: %d\n", revisionID)
			fmt.Printf("  链接: https://feishu.cn/docx/%s\n", documentID)
		}

		return nil
	},
}

func init() {
	docCmd.AddCommand(getDocumentCmd)
	getDocumentCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
}
