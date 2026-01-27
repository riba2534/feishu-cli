package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var createBoardNotesCmd = &cobra.Command{
	Use:   "create-notes <whiteboard_id> <nodes_json>",
	Short: "创建画板节点",
	Long: `在飞书画板上创建节点。

参数:
  <whiteboard_id>   画板唯一标识符（必填）
  <nodes_json>      节点数据 JSON 字符串或文件路径（必填）
  --source-type     源类型：file/content，默认 file
  --client-token    操作唯一标识，用于幂等更新
  --user-id-type    用户 ID 类型 (open_id/union_id/user_id)，默认 open_id
  --output, -o      输出格式 (json)

节点 JSON 格式示例:
  [
    {
      "type": "sticky_note",
      "x": 100,
      "y": 100,
      "content": "便签内容"
    }
  ]

示例:
  # 从文件创建节点
  feishu-cli board create-notes <whiteboard_id> nodes.json

  # 直接传入 JSON
  feishu-cli board create-notes <whiteboard_id> '[{"type":"sticky_note","x":100,"y":100}]' --source-type content

  # 使用幂等 token
  feishu-cli board create-notes <whiteboard_id> nodes.json --client-token abc123`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		whiteboardID := args[0]
		source := args[1]
		sourceType, _ := cmd.Flags().GetString("source-type")
		clientToken, _ := cmd.Flags().GetString("client-token")
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		output, _ := cmd.Flags().GetString("output")

		// Get nodes JSON
		var nodesJSON string
		if sourceType == "content" {
			nodesJSON = source
		} else {
			// Read from file
			data, err := os.ReadFile(source)
			if err != nil {
				return fmt.Errorf("读取节点文件失败: %w", err)
			}
			nodesJSON = string(data)
		}

		opts := client.CreateBoardNotesOptions{
			ClientToken: clientToken,
			UserIDType:  userIDType,
		}

		nodeIDs, err := client.CreateBoardNodes(whiteboardID, nodesJSON, opts)
		if err != nil {
			return err
		}

		if output == "json" {
			result := map[string]interface{}{
				"whiteboard_id": whiteboardID,
				"node_ids":      nodeIDs,
				"count":         len(nodeIDs),
			}
			data, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("画板节点创建成功！\n")
			fmt.Printf("  画板 ID: %s\n", whiteboardID)
			fmt.Printf("  创建节点数: %d\n", len(nodeIDs))
			for i, id := range nodeIDs {
				fmt.Printf("  [%d] 节点 ID: %s\n", i+1, id)
			}
		}

		return nil
	},
}

func init() {
	boardCmd.AddCommand(createBoardNotesCmd)
	createBoardNotesCmd.Flags().String("source-type", "file", "源类型 (file/content)")
	createBoardNotesCmd.Flags().String("client-token", "", "操作唯一标识（幂等）")
	createBoardNotesCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型 (open_id/union_id/user_id)")
	createBoardNotesCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
}
