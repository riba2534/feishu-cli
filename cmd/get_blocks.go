package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var getBlocksCmd = &cobra.Command{
	Use:   "blocks <document_id>",
	Short: "获取文档所有块",
	Long: `获取飞书文档中的所有块信息。

参数:
  <document_id>              文档 ID（必填）
  --all                      获取所有块（自动处理分页）
  --raw                      获取原始 JSON 内容
  --page-size                分页大小（默认 500）
  --page-token               分页标记
  --document-revision-id     文档版本 ID，-1 表示最新
  --user-id-type             用户 ID 类型，默认 open_id
  --output, -o               输出格式 (json)

示例:
  feishu-cli doc blocks ABC123def456
  feishu-cli doc blocks ABC123def456 --all
  feishu-cli doc blocks ABC123def456 --raw
  feishu-cli doc blocks ABC123def456 -o json
  feishu-cli doc blocks ABC123def456 --page-size 100 --page-token xxx`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		documentID := args[0]
		raw, _ := cmd.Flags().GetBool("raw")
		all, _ := cmd.Flags().GetBool("all")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		output, _ := cmd.Flags().GetString("output")

		if raw {
			content, err := client.GetRawContent(documentID)
			if err != nil {
				return err
			}
			fmt.Println(content)
			return nil
		}

		// Get blocks
		var blocks interface{}
		var nextPageToken string

		if all {
			// Get all blocks with automatic pagination
			allBlocks, err := client.GetAllBlocks(documentID)
			if err != nil {
				return err
			}
			blocks = allBlocks

			if output == "json" {
				data, _ := json.MarshalIndent(blocks, "", "  ")
				fmt.Println(string(data))
			} else {
				fmt.Printf("共找到 %d 个块:\n\n", len(allBlocks))
				for i, block := range allBlocks {
					blockType := "未知"
					if block.BlockType != nil {
						blockType = fmt.Sprintf("%d", *block.BlockType)
					}
					blockID := ""
					if block.BlockId != nil {
						blockID = *block.BlockId
					}
					fmt.Printf("[%d] 类型: %s, ID: %s\n", i+1, blockType, blockID)
				}
			}
		} else {
			// Get blocks with pagination
			blockList, nextToken, err := client.ListBlocks(documentID, pageToken, pageSize)
			if err != nil {
				return err
			}
			blocks = blockList
			nextPageToken = nextToken

			if output == "json" {
				result := map[string]interface{}{
					"items":      blockList,
					"page_token": nextPageToken,
					"has_more":   nextPageToken != "",
				}
				data, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(data))
			} else {
				fmt.Printf("共找到 %d 个块:\n\n", len(blockList))
				for i, block := range blockList {
					blockType := "未知"
					if block.BlockType != nil {
						blockType = fmt.Sprintf("%d", *block.BlockType)
					}
					blockID := ""
					if block.BlockId != nil {
						blockID = *block.BlockId
					}
					fmt.Printf("[%d] 类型: %s, ID: %s\n", i+1, blockType, blockID)
				}
				if nextPageToken != "" {
					fmt.Printf("\n还有更多块，使用 --page-token %s 获取下一页\n", nextPageToken)
				}
			}
		}

		return nil
	},
}

func init() {
	docCmd.AddCommand(getBlocksCmd)
	getBlocksCmd.Flags().Bool("raw", false, "获取原始 JSON 内容")
	getBlocksCmd.Flags().Bool("all", false, "获取所有块（自动处理分页）")
	getBlocksCmd.Flags().Int("page-size", 500, "分页大小")
	getBlocksCmd.Flags().String("page-token", "", "分页标记")
	getBlocksCmd.Flags().Int("document-revision-id", -1, "文档版本 ID（-1 表示最新）")
	getBlocksCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型")
	getBlocksCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
}
