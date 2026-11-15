package cmd

import (
	"fmt"
	"net/url"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var boardDeleteCmd = &cobra.Command{
	Use:   "delete <whiteboard_id>",
	Short: "删除画板节点",
	Long: `批量删除画板中的节点。

删除没有确认门禁：执行即生效、不可撤销（--all 清空整个画板）。先用 --dry-run 核对要删除的节点，
或用 board nodes 回读确认后再执行。

示例:
  # 删除指定节点
  feishu-cli board delete BOARD_ID --node-ids o1:1,o1:2,o1:3

  # 删除所有节点（清空画板）
  feishu-cli board delete BOARD_ID --all

  # 预览将发出的请求（不联网、不需要凭据）
  feishu-cli board delete BOARD_ID --node-ids o1:1,o1:2 --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		whiteboardID := args[0]
		nodeIDsStr, _ := cmd.Flags().GetString("node-ids")
		deleteAll, _ := cmd.Flags().GetBool("all")
		output, _ := cmd.Flags().GetString("output")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		if !deleteAll && nodeIDsStr == "" {
			return clierr.Usagef("请指定 --node-ids 或 --all")
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}

		if dryRun {
			return printBoardDeleteDryRun(cmd, whiteboardID, deleteAll, splitAndTrim(nodeIDsStr))
		}

		if err := config.Validate(); err != nil {
			return err
		}
		userAccessToken, err := resolveVisualWriteToken(cmd)
		if err != nil {
			return err
		}

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
		} else {
			nodeIDs = splitAndTrim(nodeIDsStr)
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

// printBoardDeleteDryRun 静态预览 board delete 的请求（不联网、不解析身份）。
// --all 时节点 ID 需要执行时先读取画板才能得到，预览里用占位符表示。
func printBoardDeleteDryRun(cmd *cobra.Command, whiteboardID string, deleteAll bool, nodeIDs []string) error {
	escaped := url.PathEscape(whiteboardID)
	deletePath := fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes/batch_delete", escaped)
	var steps []dryRunStep
	extra := map[string]any{"whiteboard_id": whiteboardID, "all": deleteAll}
	if deleteAll {
		steps = append(steps,
			dryRunStep{
				Method: "GET",
				URL:    fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes", escaped),
				Desc:   "读取画板全部节点 ID（画板为空时不再发删除请求）",
			},
			dryRunStep{
				Method: "DELETE",
				URL:    deletePath,
				Desc:   "按每批 100 个删除读取到的全部节点",
				Body:   map[string]any{"ids": []string{"<all node ids>"}},
			})
	} else {
		extra["node_count"] = len(nodeIDs)
		for i := 0; i < len(nodeIDs); i += 100 {
			end := min(i+100, len(nodeIDs))
			steps = append(steps, dryRunStep{
				Method: "DELETE",
				URL:    deletePath,
				Desc:   fmt.Sprintf("删除第 %d-%d 个节点", i+1, end),
				Body:   map[string]any{"ids": nodeIDs[i:end]},
			})
		}
	}
	if label, ok := visualDryRunIdentity(cmd); ok {
		extra["identity"] = label
	}
	return printDryRunPlan(cmd, "delete whiteboard nodes", extra, steps)
}

func init() {
	boardCmd.AddCommand(boardDeleteCmd)
	boardDeleteCmd.Flags().String("node-ids", "", "要删除的节点 ID（逗号分隔）")
	boardDeleteCmd.Flags().Bool("all", false, "删除所有节点（清空画板）")
	boardDeleteCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
	boardDeleteCmd.Flags().String("user-access-token", "", "User Access Token")
	boardDeleteCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求，不联网、不需要凭据")
	addVisualWriteAsFlag(boardDeleteCmd)
}
