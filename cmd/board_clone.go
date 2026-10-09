package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var boardCloneCmd = &cobra.Command{
	Use:   "clone <source_whiteboard_id> <target_whiteboard_id>",
	Short: "克隆画板内容（GET → sanitize → 分批 POST）",
	Long: `把源画板的所有节点复制到目标画板，自动处理连线 ID 重映射。

参数:
  <source_whiteboard_id>   源画板 ID
  <target_whiteboard_id>   目标画板 ID（应为已创建的空画板）

特性:
  - 移除只读字段（id/locked/children/parent_id 等）
  - 先创建形状/svg/文字节点，记录新旧 ID 映射，再创建 connector 并重写 attached_object.id
  - 分批 + 批间隔节流，避免触发限流
  - --dry-run 只列出将克隆的节点数与类型分布

示例:
  # 基础克隆
  feishu-cli board clone <src> <dst>

  # 自定义分批
  feishu-cli board clone <src> <dst> --batch-size 10 --interval 3s

  # 仅克隆 svg + composite_shape 节点
  feishu-cli board clone <src> <dst> --filter-types svg,composite_shape

  # 预览（不调用 API）
  feishu-cli board clone <src> <dst> --dry-run`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		srcID := args[0]
		dstID := args[1]
		batchSize, _ := cmd.Flags().GetInt("batch-size")
		intervalDur, _ := cmd.Flags().GetDuration("interval")
		filterTypes, _ := cmd.Flags().GetStringSlice("filter-types")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		output, _ := cmd.Flags().GetString("output")
		userAccessToken := resolveOptionalUserToken(cmd)

		if batchSize <= 0 {
			batchSize = 10
		}

		// 1. GetBoardNodes
		raw, err := client.GetBoardNodes(srcID, userAccessToken)
		if err != nil {
			return fmt.Errorf("获取源画板节点失败: %w", err)
		}
		var apiResp struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
			Data struct {
				Nodes []map[string]any `json:"nodes"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &apiResp); err != nil {
			return fmt.Errorf("解析源画板节点失败: %w", err)
		}
		if apiResp.Code != 0 {
			return fmt.Errorf("源画板返回错误: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
		}

		// 2. 类型过滤
		filterSet := map[string]bool{}
		for _, t := range filterTypes {
			if t != "" {
				filterSet[t] = true
			}
		}
		// 类型统计 + 分桶
		typeStat := map[string]int{}
		var shapeNodes []map[string]any
		var connectorNodes []map[string]any
		for _, node := range apiResp.Data.Nodes {
			t, _ := node["type"].(string)
			typeStat[t]++
			if len(filterSet) > 0 && !filterSet[t] {
				continue
			}
			if t == "connector" {
				connectorNodes = append(connectorNodes, node)
			} else {
				shapeNodes = append(shapeNodes, node)
			}
		}

		if dryRun {
			summary := map[string]any{
				"source":         srcID,
				"target":         dstID,
				"total_nodes":    len(apiResp.Data.Nodes),
				"shape_to_clone": len(shapeNodes),
				"conn_to_clone":  len(connectorNodes),
				"type_breakdown": typeStat,
				"batch_size":     batchSize,
				"interval_secs":  intervalDur.Seconds(),
				"filter_types":   filterTypes,
				"dry_run":        true,
			}
			if output == "json" {
				return printJSON(summary)
			}
			fmt.Printf("[dry-run] 将克隆 %d 个节点（形状 %d + 连线 %d），按 %d 个/批，批间隔 %s\n",
				len(shapeNodes)+len(connectorNodes), len(shapeNodes), len(connectorNodes),
				batchSize, intervalDur)
			fmt.Printf("  类型分布: %v\n", typeStat)
			return nil
		}

		// 3. 第一轮：先创建形状/svg/text 节点，建立 oldID -> newID 映射
		idMap := map[string]string{}
		sanitizedShapes := make([]map[string]any, 0, len(shapeNodes))
		oldShapeIDs := make([]string, 0, len(shapeNodes))
		for _, node := range shapeNodes {
			oldID, _ := node["id"].(string)
			oldShapeIDs = append(oldShapeIDs, oldID)
			sanitizedShapes = append(sanitizedShapes, sanitizeNode(node))
		}

		shapeCreated := 0
		for i := 0; i < len(sanitizedShapes); i += batchSize {
			end := i + batchSize
			if end > len(sanitizedShapes) {
				end = len(sanitizedShapes)
			}
			batch := sanitizedShapes[i:end]
			batchJSON, _ := json.Marshal(batch)
			newIDs, err := client.CreateBoardNodes(dstID, string(batchJSON), client.CreateBoardNotesOptions{
				UserAccessToken: userAccessToken,
			})
			if err != nil {
				return fmt.Errorf("克隆形状节点批 %d-%d 失败: %w", i, end, err)
			}
			// 建立 ID 映射（依赖批次顺序）
			for k, newID := range newIDs {
				if i+k < len(oldShapeIDs) {
					idMap[oldShapeIDs[i+k]] = newID
				}
			}
			shapeCreated += len(newIDs)
			if end < len(sanitizedShapes) && intervalDur > 0 {
				time.Sleep(intervalDur)
			}
		}

		// 4. 第二轮：处理 connector，重写 attached_object.id
		sanitizedConns := make([]map[string]any, 0, len(connectorNodes))
		connSkipped := 0
		for _, node := range connectorNodes {
			s := sanitizeNode(node)
			if !rewireConnector(s, idMap) {
				connSkipped++
				continue
			}
			sanitizedConns = append(sanitizedConns, s)
		}

		connCreated := 0
		for i := 0; i < len(sanitizedConns); i += batchSize {
			end := i + batchSize
			if end > len(sanitizedConns) {
				end = len(sanitizedConns)
			}
			batch := sanitizedConns[i:end]
			batchJSON, _ := json.Marshal(batch)
			newIDs, err := client.CreateBoardNodes(dstID, string(batchJSON), client.CreateBoardNotesOptions{
				UserAccessToken: userAccessToken,
			})
			if err != nil {
				return fmt.Errorf("克隆连线批 %d-%d 失败: %w", i, end, err)
			}
			connCreated += len(newIDs)
			if end < len(sanitizedConns) && intervalDur > 0 {
				time.Sleep(intervalDur)
			}
		}

		result := map[string]any{
			"source":            srcID,
			"target":            dstID,
			"shape_created":     shapeCreated,
			"connector_created": connCreated,
			"connector_skipped": connSkipped,
			"type_breakdown":    typeStat,
		}
		if output == "json" {
			return printJSON(result)
		}
		fmt.Printf("克隆完成：形状 %d 个 + 连线 %d 个（跳过 %d 个无法重映射的连线）\n",
			shapeCreated, connCreated, connSkipped)
		return nil
	},
}

// sanitizeNode 移除只读字段，返回干净可 POST 的节点拷贝
func sanitizeNode(node map[string]any) map[string]any {
	out := make(map[string]any, len(node))
	for k, v := range node {
		switch k {
		case "id", "locked", "children", "parent_id":
			continue
		default:
			out[k] = v
		}
	}
	return out
}

// rewireConnector 把 connector 的 attached_object.id 重写为新画板的 ID。
// 返回 false 表示无法重映射（外部节点不存在）。
func rewireConnector(conn map[string]any, idMap map[string]string) bool {
	connSection, ok := conn["connector"].(map[string]any)
	if !ok {
		return true // 没有 connector 结构（不应发生）
	}
	rewire := func(endpoint map[string]any) bool {
		obj, ok := endpoint["attached_object"].(map[string]any)
		if !ok {
			return true // 没有附着对象，端点是自由坐标
		}
		oldID, _ := obj["id"].(string)
		if oldID == "" {
			return true
		}
		newID, found := idMap[oldID]
		if !found {
			return false
		}
		obj["id"] = newID
		return true
	}
	if start, ok := connSection["start"].(map[string]any); ok {
		if !rewire(start) {
			return false
		}
	}
	if end, ok := connSection["end"].(map[string]any); ok {
		if !rewire(end) {
			return false
		}
	}
	return true
}

func init() {
	boardCmd.AddCommand(boardCloneCmd)
	boardCloneCmd.Flags().Int("batch-size", 10, "每批克隆的节点数")
	boardCloneCmd.Flags().Duration("interval", 1*time.Second, "批次间隔（避免限流）")
	boardCloneCmd.Flags().StringSlice("filter-types", nil, "只克隆指定类型（逗号分隔，如 svg,composite_shape,text_shape）")
	boardCloneCmd.Flags().Bool("dry-run", false, "预览不调用 API")
	boardCloneCmd.Flags().String("user-access-token", "", "User Access Token")
	boardCloneCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
}
