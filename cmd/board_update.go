package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/riba2534/feishu-cli/v2/internal/safefile"
	"github.com/spf13/cobra"
)

var boardUpdateCmd = &cobra.Command{
	Use:   "update <whiteboard_id> [nodes_json_file]",
	Short: "更新画板内容（支持覆盖）",
	Long: `更新画板节点内容。支持从文件或 stdin 读取节点 JSON。

--overwrite 模式会通过服务端 overwrite: true 参数原子清空并写入新节点。
--dry-run 模式仅预览，不实际执行。
--client-token 幂等键（≥10 字符）：网络超时等结果未知时，用同一个值重跑不会重复建节点
（服务端对同一 client_token 直接返回首次写入的节点 ID）。

示例:
  # 从文件更新（追加模式）
  feishu-cli board update BOARD_ID nodes.json

  # 从 stdin 管道更新（原子覆盖模式）
  cat nodes.json | feishu-cli board update BOARD_ID --stdin --overwrite

  # 预览覆盖操作
  feishu-cli board update BOARD_ID nodes.json --overwrite --dry-run

  # 带幂等键写入（结果未知时原样重跑）
  feishu-cli board update BOARD_ID nodes.json --client-token fp-nodes-20260101-001`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		whiteboardID := args[0]
		useStdin, _ := cmd.Flags().GetBool("stdin")
		overwrite, _ := cmd.Flags().GetBool("overwrite")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		snapshotPath, _ := cmd.Flags().GetString("snapshot")
		output, _ := cmd.Flags().GetString("output")
		clientToken, _ := cmd.Flags().GetString("client-token")
		clientToken = strings.TrimSpace(clientToken)
		if err := client.ValidateBoardClientToken(clientToken); err != nil {
			return clierr.Usage(err)
		}
		if snapshotPath != "" {
			// 快照路径在任何网络请求之前校验，避免先改远端画板再发现本地写不了
			if err := validateOutputPath(snapshotPath, ""); err != nil {
				return err
			}
		}
		userAccessToken := resolveOptionalUserToken(cmd)

		// 1. 读取节点 JSON（从文件或 stdin）
		var nodesJSON string
		if useStdin {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("读取标准输入失败: %w", err)
			}
			nodesJSON = string(data)
		} else if len(args) >= 2 {
			data, err := readLocalInputFile(args[1])
			if err != nil {
				return fmt.Errorf("读取节点文件失败: %w", err)
			}
			nodesJSON = string(data)
		} else {
			return fmt.Errorf("请提供节点 JSON 文件路径，或使用 --stdin 从标准输入读取")
		}

		// 2. dry-run 模式：仅预览
		if dryRun {
			var nodes []json.RawMessage
			if err := json.Unmarshal([]byte(nodesJSON), &nodes); err != nil {
				return fmt.Errorf("解析节点 JSON 失败（需要 JSON 数组格式）: %w", err)
			}
			if overwrite {
				fmt.Fprintf(os.Stderr, "[dry-run] 将以原子覆盖模式 (overwrite: true) 写入 %d 个节点到画板 %s\n", len(nodes), whiteboardID)
			} else {
				fmt.Fprintf(os.Stderr, "[dry-run] 将追加写入 %d 个节点到画板 %s\n", len(nodes), whiteboardID)
			}
			return nil
		}

		// 3. --snapshot：在执行 overwrite 之前先把旧节点导出到本地（只读备份）
		if overwrite && snapshotPath != "" {
			raw, err := client.GetBoardNodes(whiteboardID, userAccessToken)
			if err != nil {
				return fmt.Errorf("快照导出失败: %w", err)
			}
			if err := safefile.AtomicWriteFile(snapshotPath, raw, 0o644); err != nil {
				return fmt.Errorf("写入快照文件失败: %w", err)
			}
			fmt.Fprintf(os.Stderr, "已导出旧画板快照 → %s（用于本地备份）\n", snapshotPath)
		}

		// 4. 调用 CreateBoardNodes（服务端原子覆盖）
		newNodeIDs, err := client.CreateBoardNodes(whiteboardID, nodesJSON, client.CreateBoardNotesOptions{
			UserAccessToken: userAccessToken,
			Overwrite:       overwrite,
			ClientToken:     clientToken,
		})
		if err != nil {
			return fmt.Errorf("更新画板节点失败: %w", err)
		}

		// 5. 输出结果
		if output == "json" {
			result := map[string]any{
				"whiteboard_id": whiteboardID,
				"new_node_ids":  newNodeIDs,
				"created_count": len(newNodeIDs),
				"overwrite":     overwrite,
			}
			if clientToken != "" {
				result["client_token"] = clientToken
			}
			return printJSON(result)
		}

		if overwrite {
			fmt.Printf("画板原子覆盖更新成功！\n")
		} else {
			fmt.Printf("画板节点创建成功！\n")
		}
		fmt.Printf("  画板 ID: %s\n", whiteboardID)
		fmt.Printf("  创建节点数: %d\n", len(newNodeIDs))
		for i, id := range newNodeIDs {
			fmt.Printf("  [%d] 节点 ID: %s\n", i+1, id)
		}

		return nil
	},
}

// extractBoardNodeIDs 从画板获取所有节点 ID
func extractBoardNodeIDs(whiteboardID, userAccessToken string) ([]string, error) {
	rawJSON, err := client.GetBoardNodes(whiteboardID, userAccessToken)
	if err != nil {
		return nil, err
	}

	// 解析响应，提取节点 ID
	// API 返回格式: {"code":0,"data":{"nodes":{"id1":{...},"id2":{...}}}}
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Nodes json.RawMessage `json:"nodes"`
		} `json:"data"`
	}

	if err := json.Unmarshal(rawJSON, &resp); err != nil {
		return nil, fmt.Errorf("解析节点响应失败: %w", err)
	}

	if resp.Code != 0 {
		return nil, fmt.Errorf("获取节点失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return parseBoardNodeIDs(resp.Data.Nodes)
}

// parseBoardNodeIDs 从 data.nodes 中提取节点 ID。
// 空画板的响应是 {"code":0,"data":{}}——没有 nodes 字段（官方 483043c8 同样按空处理），
// 此时 RawMessage 为空，必须直接返回空列表，不能交给 json.Unmarshal（会报 unexpected end of JSON input）。
func parseBoardNodeIDs(raw json.RawMessage) ([]string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return []string{}, nil
	}

	// nodes 可能是 map[string]any 格式（key 就是 node ID）
	var nodesMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &nodesMap); err != nil {
		// 也可能是数组格式，尝试解析为数组
		var nodesArray []struct {
			ID string `json:"id"`
		}
		if err2 := json.Unmarshal(raw, &nodesArray); err2 != nil {
			return nil, fmt.Errorf("解析节点数据失败: map 解析=%w, 数组解析=%v", err, err2)
		}
		ids := make([]string, 0, len(nodesArray))
		for _, n := range nodesArray {
			if n.ID != "" {
				ids = append(ids, n.ID)
			}
		}
		return ids, nil
	}

	ids := make([]string, 0, len(nodesMap))
	for id := range nodesMap {
		ids = append(ids, id)
	}
	return ids, nil
}

func init() {
	boardCmd.AddCommand(boardUpdateCmd)
	boardUpdateCmd.Flags().Bool("stdin", false, "从标准输入读取节点 JSON")
	boardUpdateCmd.Flags().Bool("overwrite", false, "原子覆盖模式（服务端 overwrite: true）")
	boardUpdateCmd.Flags().Bool("dry-run", false, "仅预览，不实际执行")
	boardUpdateCmd.Flags().String("snapshot", "", "执行 --overwrite 前把旧节点导出到此路径（用于本地备份）")
	boardUpdateCmd.Flags().String("client-token", "", "幂等键（≥10 字符；结果未知时用同一个值重跑不会重复建节点）")
	boardUpdateCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
	boardUpdateCmd.Flags().String("user-access-token", "", "User Access Token")
}
