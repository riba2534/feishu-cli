package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/config"
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
  --client-token    操作唯一标识，用于幂等更新（长度至少 10 字符，建议使用 UUID）
  --overwrite       是否覆盖画板已有内容（服务端 overwrite: true 原子替换）
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

  # 使用幂等 token（至少 10 字符）
  feishu-cli board create-notes <whiteboard_id> nodes.json --client-token 6d99f59c-4d7d-4452-98d6-3d0556393cf6

  # 覆盖创建
  feishu-cli board create-notes <whiteboard_id> nodes.json --overwrite`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		whiteboardID := args[0]
		source := args[1]
		sourceType, _ := cmd.Flags().GetString("source-type")
		clientToken, _ := cmd.Flags().GetString("client-token")
		overwrite, _ := cmd.Flags().GetBool("overwrite")
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		output, _ := cmd.Flags().GetString("output")
		userAccessToken := resolveOptionalUserToken(cmd)

		if clientToken != "" && len(clientToken) < 10 {
			return fmt.Errorf("--client-token 长度至少为 10 个字符: %s", clientToken)
		}

		// Get nodes JSON
		var nodesJSON string
		if sourceType == "content" {
			nodesJSON = source
		} else {
			// Read from file
			data, err := readLocalInputFile(source)
			if err != nil {
				return fmt.Errorf("读取节点文件失败: %w", err)
			}
			nodesJSON = string(data)
		}

		opts := client.CreateBoardNotesOptions{
			ClientToken:     clientToken,
			UserIDType:      userIDType,
			Overwrite:       overwrite,
			UserAccessToken: userAccessToken,
		}

		nodeIDs, err := client.CreateBoardNodes(whiteboardID, nodesJSON, opts)
		if err != nil {
			return err
		}

		if output == "json" {
			result := map[string]any{
				"whiteboard_id": whiteboardID,
				"node_ids":      nodeIDs,
				"count":         len(nodeIDs),
			}
			if overwrite {
				result["overwrite"] = true
			}
			if err := printJSON(result); err != nil {
				return err
			}
		} else {
			if overwrite {
				fmt.Printf("画板节点覆盖创建成功！\n")
			} else {
				fmt.Printf("画板节点创建成功！\n")
			}
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
	createBoardNotesCmd.Flags().String("client-token", "", "操作唯一标识（幂等，至少 10 字符）")
	createBoardNotesCmd.Flags().Bool("overwrite", false, "是否覆盖画板已有内容（服务端 overwrite: true）")
	createBoardNotesCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型 (open_id/union_id/user_id)")
	createBoardNotesCmd.Flags().String("user-access-token", "", "User Access Token")
	createBoardNotesCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
}
