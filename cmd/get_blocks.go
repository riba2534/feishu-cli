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

示例:
  feishu-cli doc blocks ABC123def456
  feishu-cli doc blocks ABC123def456 --raw
  feishu-cli doc blocks ABC123def456 -o json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		documentID := args[0]
		raw, _ := cmd.Flags().GetBool("raw")

		if raw {
			content, err := client.GetRawContent(documentID)
			if err != nil {
				return err
			}
			fmt.Println(content)
			return nil
		}

		blocks, err := client.GetAllBlocks(documentID)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			data, _ := json.MarshalIndent(blocks, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("共找到 %d 个块:\n\n", len(blocks))
			for i, block := range blocks {
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

		return nil
	},
}

func init() {
	docCmd.AddCommand(getBlocksCmd)
	getBlocksCmd.Flags().Bool("raw", false, "获取原始 JSON 内容")
	getBlocksCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
}
