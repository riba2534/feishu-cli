package cmd

import (
	"encoding/json"
	"fmt"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var addBoardCmd = &cobra.Command{
	Use:   "add-board <document_id>",
	Short: "添加画板到文档",
	Long: `向飞书文档添加画板块。

参数:
  <document_id>  文档 ID（必填）
  --parent-id    父块 ID，空表示根级别，默认空
  --index        插入位置索引，-1 表示末尾，默认 -1
  --output, -o   输出格式 (json)

说明:
  此命令会在文档中创建一个新的画板块。
  画板块创建后可以通过画板相关命令进行操作。

示例:
  # 在文档末尾添加画板
  feishu-cli doc add-board DOC_ID

  # 在指定位置添加画板
  feishu-cli doc add-board DOC_ID --parent-id BLOCK_ID --index 0

  # JSON 格式输出
  feishu-cli doc add-board DOC_ID -o json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		documentID := args[0]
		parentID, _ := cmd.Flags().GetString("parent-id")
		index, _ := cmd.Flags().GetInt("index")
		output, _ := cmd.Flags().GetString("output")

		// 如果父块 ID 为空，使用文档根节点
		if parentID == "" {
			parentID = documentID
		}

		// 构建画板块 (block_type = 43)
		blockType := 43 // Board
		boardBlock := &larkdocx.Block{
			BlockType: &blockType,
			Board:     &larkdocx.Board{},
		}

		// 创建画板块
		createdBlocks, err := client.CreateBlock(documentID, parentID, []*larkdocx.Block{boardBlock}, index)
		if err != nil {
			return err
		}

		if len(createdBlocks) == 0 {
			return fmt.Errorf("创建画板块失败：未返回块信息")
		}

		// 获取创建的画板块信息
		boardBlockID := ""
		whiteboardID := ""
		if createdBlocks[0].BlockId != nil {
			boardBlockID = *createdBlocks[0].BlockId
		}
		if createdBlocks[0].Board != nil && createdBlocks[0].Board.Token != nil {
			whiteboardID = *createdBlocks[0].Board.Token
		}

		if output == "json" {
			result := map[string]interface{}{
				"block_id":      boardBlockID,
				"whiteboard_id": whiteboardID,
				"document_id":   documentID,
			}
			data, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("画板添加成功！\n")
			fmt.Printf("  块 ID: %s\n", boardBlockID)
			if whiteboardID != "" {
				fmt.Printf("  画板 ID: %s\n", whiteboardID)
			}
		}

		return nil
	},
}

func init() {
	docCmd.AddCommand(addBoardCmd)
	addBoardCmd.Flags().String("parent-id", "", "父块 ID（默认: 文档根节点）")
	addBoardCmd.Flags().Int("index", -1, "插入位置索引（-1 表示末尾）")
	addBoardCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
}
