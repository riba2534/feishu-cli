package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var boardDeleteCmd = &cobra.Command{
	Use:   "delete <whiteboard_id>",
	Short: "删除画板节点",
	Long: `批量删除画板中的节点。

示例:
  # 删除指定节点
  feishu-cli board delete BOARD_ID --node-ids o1:1,o1:2,o1:3

  # 删除所有节点（清空画板）
  feishu-cli board delete BOARD_ID --all`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		whiteboardID := args[0]
		nodeIDsStr, _ := cmd.Flags().GetString("node-ids")
		deleteAll, _ := cmd.Flags().GetBool("all")
		output, _ := cmd.Flags().GetString("output")
		userAccessToken := resolveOptionalUserToken(cmd)

		var nodeIDs []string
		if deleteAll {
			// 获取所有节点 ID
			ids, err := extractBoardNodeIDs(whiteboardID, userAccessToken)
			if err != nil {
				return fmt.Errorf("获取画板节点失败: %w", err)
			}
			if len(ids) == 0 {
				if output == "json" {
					return printJSON(map[string]any{
						"whiteboard_id": whiteboardID,
						"deleted_ids":   []string{},
						"deleted_count": 0,
					})
				}
				fmt.Println("画板中没有节点，无需删除")
				return nil
			}
			nodeIDs = ids
		} else if nodeIDsStr != "" {
			nodeIDs = splitAndTrim(nodeIDsStr)
		} else {
			return fmt.Errorf("请指定 --node-ids 或 --all")
		}

		if err := client.DeleteBoardNodes(whiteboardID, nodeIDs, userAccessToken); err != nil {
			return err
		}

		if output == "json" {
			result := map[string]any{
				"whiteboard_id": whiteboardID,
				"deleted_ids":   nodeIDs,
				"deleted_count": len(nodeIDs),
			}
			return printJSON(result)
		}

		fmt.Printf("画板节点删除成功！\n")
		fmt.Printf("  画板 ID: %s\n", whiteboardID)
		fmt.Printf("  删除节点数: %d\n", len(nodeIDs))

		return nil
	},
}

func init() {
	boardCmd.AddCommand(boardDeleteCmd)
	boardDeleteCmd.Flags().String("node-ids", "", "要删除的节点 ID（逗号分隔）")
	boardDeleteCmd.Flags().Bool("all", false, "删除所有节点（清空画板）")
	boardDeleteCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
	boardDeleteCmd.Flags().String("user-access-token", "", "User Access Token")
}
