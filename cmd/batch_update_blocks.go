package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var batchUpdateBlocksCmd = &cobra.Command{
	Use:   "batch-update <document_id> <requests>",
	Short: "批量更新文档块",
	Long: `批量更新飞书文档中的多个块。

参数:
  <document_id>             文档 ID（必填）
  <requests>                更新请求列表，JSON 格式（必填）
  --source-type             源类型：file/content，默认 file
  --document-revision-id    文档版本 ID，-1 表示最新
  --client-token            UUIDv4，用于幂等更新
  --user-id-type            用户 ID 类型，默认 open_id
  --output, -o              输出格式 (json)

请求格式示例:
  [
    {
      "block_id": "xxx",
      "update_text_elements": {
        "elements": [{"text_run": {"content": "新内容"}}]
      }
    }
  ]

示例:
  # 从文件批量更新
  feishu-cli doc batch-update DOC_ID requests.json

  # 直接传入 JSON
  feishu-cli doc batch-update DOC_ID '[{"block_id":"xxx","update_text_elements":{"elements":[{"text_run":{"content":"新内容"}}]}}]' --source-type content

  # 使用幂等 token
  feishu-cli doc batch-update DOC_ID requests.json --client-token abc123`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		documentID := args[0]
		source := args[1]
		sourceType, _ := cmd.Flags().GetString("source-type")
		documentRevisionID, _ := cmd.Flags().GetInt("document-revision-id")
		clientToken, _ := cmd.Flags().GetString("client-token")
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		output, _ := cmd.Flags().GetString("output")

		// Get requests JSON
		var requestsJSON string
		if sourceType == "content" {
			requestsJSON = source
		} else {
			// Read from file
			data, err := os.ReadFile(source)
			if err != nil {
				return fmt.Errorf("读取请求文件失败: %w", err)
			}
			requestsJSON = string(data)
		}

		opts := client.BatchUpdateBlocksOptions{
			DocumentRevisionID: documentRevisionID,
			ClientToken:        clientToken,
			UserIDType:         userIDType,
		}

		result, err := client.BatchUpdateBlocks(documentID, requestsJSON, opts)
		if err != nil {
			return err
		}

		if output == "json" {
			data, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("批量更新成功！\n")
			fmt.Printf("  文档 ID: %s\n", documentID)
			fmt.Printf("  更新块数: %d\n", len(result.BlockIDs))
			for i, id := range result.BlockIDs {
				fmt.Printf("  [%d] 块 ID: %s\n", i+1, id)
			}
		}

		return nil
	},
}

func init() {
	docCmd.AddCommand(batchUpdateBlocksCmd)
	batchUpdateBlocksCmd.Flags().String("source-type", "file", "源类型 (file/content)")
	batchUpdateBlocksCmd.Flags().Int("document-revision-id", -1, "文档版本 ID（-1 表示最新）")
	batchUpdateBlocksCmd.Flags().String("client-token", "", "操作唯一标识（幂等）")
	batchUpdateBlocksCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型")
	batchUpdateBlocksCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
}
