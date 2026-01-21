package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var addContentCmd = &cobra.Command{
	Use:   "add <document_id>",
	Short: "向文档添加内容块",
	Long: `向飞书文档添加内容块。

内容应为 JSON 格式的块对象数组。

示例:
  feishu-cli doc add DOC_ID --content '[{"block_type":2,"text":{"elements":[{"text_run":{"content":"你好"}}]}}]'
  feishu-cli doc add DOC_ID --content-file blocks.json --block-id PARENT_BLOCK_ID
  feishu-cli doc add DOC_ID --content-file blocks.json --index 0`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		documentID := args[0]
		contentStr, _ := cmd.Flags().GetString("content")
		contentFile, _ := cmd.Flags().GetString("content-file")
		blockID, _ := cmd.Flags().GetString("block-id")
		index, _ := cmd.Flags().GetInt("index")

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
		var blocks []*larkdocx.Block
		if err := json.Unmarshal([]byte(contentJSON), &blocks); err != nil {
			return fmt.Errorf("解析内容 JSON 失败: %w", err)
		}

		// If no block ID specified, use document root
		if blockID == "" {
			blockID = documentID
		}

		createdBlocks, err := client.CreateBlock(documentID, blockID, blocks, index)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			data, _ := json.MarshalIndent(createdBlocks, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("成功添加 %d 个块!\n", len(createdBlocks))
			for i, block := range createdBlocks {
				if block.BlockId != nil {
					fmt.Printf("  [%d] 块ID: %s\n", i+1, *block.BlockId)
				}
			}
		}

		return nil
	},
}

func init() {
	docCmd.AddCommand(addContentCmd)
	addContentCmd.Flags().StringP("content", "c", "", "要添加的块内容 (JSON 格式)")
	addContentCmd.Flags().String("content-file", "", "包含块内容的 JSON 文件")
	addContentCmd.Flags().StringP("block-id", "b", "", "父块ID (默认: 文档根节点)")
	addContentCmd.Flags().IntP("index", "i", -1, "插入位置索引 (-1 表示末尾)")
	addContentCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
}
