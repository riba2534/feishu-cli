package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var updateBlockCmd = &cobra.Command{
	Use:   "update <document_id> <block_id>",
	Short: "更新文档中的块",
	Long: `更新飞书文档中已有的块内容。

内容应为 JSON 格式的更新请求体。

示例:
  feishu-cli doc update DOC_ID BLOCK_ID --content '{"update_text_elements":{"elements":[{"text_run":{"content":"已更新"}}]}}'
  feishu-cli doc update DOC_ID BLOCK_ID --content-file update.json`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		documentID := args[0]
		blockID := args[1]
		contentStr, _ := cmd.Flags().GetString("content")
		contentFile, _ := cmd.Flags().GetString("content-file")

		// Get content from file or flag
		var contentJSON string
		if contentFile != "" {
			data, err := os.ReadFile(contentFile)
			if err != nil {
				return fmt.Errorf("读取内容文件失败: %w", err)
			}
			contentJSON = string(data)
		} else if contentStr != "" {
			contentJSON = contentStr
		} else {
			return fmt.Errorf("必须指定 --content 或 --content-file")
		}

		// Parse content JSON
		var updateContent map[string]interface{}
		if err := json.Unmarshal([]byte(contentJSON), &updateContent); err != nil {
			return fmt.Errorf("解析内容 JSON 失败: %w", err)
		}

		if err := client.UpdateBlock(documentID, blockID, updateContent); err != nil {
			return err
		}

		fmt.Printf("块 %s 更新成功!\n", blockID)
		return nil
	},
}

func init() {
	docCmd.AddCommand(updateBlockCmd)
	updateBlockCmd.Flags().StringP("content", "c", "", "更新内容 (JSON 格式)")
	updateBlockCmd.Flags().String("content-file", "", "包含更新内容的 JSON 文件")
}
