package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var boardLintCmd = &cobra.Command{
	Use:   "lint <whiteboard_id>",
	Short: "几何质检：节点重叠、字号一致性、容量统计",
	Long: `本地拉取所有节点后做几何质检，给 AI 自动生成画板提供闭环依据。

检查项:
  - node_overlap         非合法嵌套的 bbox 碰撞对数
  - z_overlap_risk       高 fill_opacity 大节点遮挡风险
  - font_size_variety    使用的不同字号个数（建议 ≤ 3）
  - over_capacity        节点数是否突破 600 推荐上限
  - quality_score        综合 0-1 评分

示例:
  feishu-cli board lint <whiteboard_id>
  feishu-cli board lint <whiteboard_id> -o json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		whiteboardID := args[0]
		output, _ := cmd.Flags().GetString("output")
		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		raw, err := client.GetBoardNodes(whiteboardID, userAccessToken)
		if err != nil {
			return err
		}
		var apiResp struct {
			Code int `json:"code"`
			Data struct {
				Nodes []map[string]any `json:"nodes"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &apiResp); err != nil {
			return err
		}
		nodes := apiResp.Data.Nodes

		// 统计
		typeCount := map[string]int{}
		fontSizes := map[float64]int{}
		var rects []rect
		var overlayBigBoxes []rect
		for _, node := range nodes {
			t, _ := node["type"].(string)
			typeCount[t]++

			if t == "connector" {
				continue
			}
			x, _ := node["x"].(float64)
			y, _ := node["y"].(float64)
			w, _ := node["width"].(float64)
			h, _ := node["height"].(float64)
			if w == 0 || h == 0 {
				continue
			}
			r := rect{x: x, y: y, w: w, h: h, area: w * h}
			rects = append(rects, r)

			// 字号统计
			if textObj, ok := node["text"].(map[string]any); ok {
				if fs, ok := textObj["font_size"].(float64); ok && fs > 0 {
					fontSizes[fs]++
				}
			}

			// 大面积 + 高 opacity 遮挡风险
			if styleObj, ok := node["style"].(map[string]any); ok {
				if op, ok := styleObj["fill_opacity"].(float64); ok && op > 60 && r.area > 50000 {
					overlayBigBoxes = append(overlayBigBoxes, r)
				}
			}
		}

		// 重叠对数（O(n²) 简化算法，节点 ≤ 600 可接受）
		overlapPairs := 0
		for i := 0; i < len(rects); i++ {
			for j := i + 1; j < len(rects); j++ {
				if rectsOverlap(rects[i], rects[j]) {
					// 排除明显的"小节点在大背景里"的合法嵌套
					if rectContains(rects[i], rects[j]) || rectContains(rects[j], rects[i]) {
						continue
					}
					overlapPairs++
				}
			}
		}

		fontVariety := len(fontSizes)
		overCapacity := len(nodes) > 600
		zRisk := len(overlayBigBoxes)

		// 综合评分
		score := 1.0
		if overlapPairs > 0 {
			score -= float64(overlapPairs) * 0.02
		}
		if fontVariety > 3 {
			score -= float64(fontVariety-3) * 0.05
		}
		if overCapacity {
			score -= 0.2
		}
		if zRisk > 0 {
			score -= float64(zRisk) * 0.05
		}
		if score < 0 {
			score = 0
		}

		result := map[string]any{
			"whiteboard_id":     whiteboardID,
			"total_nodes":       len(nodes),
			"type_breakdown":    typeCount,
			"node_overlap":      overlapPairs,
			"z_overlap_risk":    zRisk,
			"font_size_variety": fontVariety,
			"font_sizes":        fontSizes,
			"over_capacity":     overCapacity,
			"quality_score":     score,
		}
		if output == "json" {
			return printJSON(result)
		}
		fmt.Printf("画板质检报告 (%s)\n", whiteboardID)
		fmt.Printf("  节点总数:       %d %s\n", len(nodes), capStat(len(nodes)))
		fmt.Printf("  类型分布:       %v\n", typeCount)
		fmt.Printf("  重叠节点对:     %d %s\n", overlapPairs, severity(overlapPairs, 0, 5))
		fmt.Printf("  字号种类:       %d %s\n", fontVariety, severity(fontVariety, 3, 6))
		fmt.Printf("  遮挡风险节点:   %d %s\n", zRisk, severity(zRisk, 0, 2))
		fmt.Printf("  质量综合分:     %.2f / 1.00\n", score)
		return nil
	},
}

type rect struct {
	x, y, w, h, area float64
}

func rectsOverlap(a, b rect) bool {
	return a.x < b.x+b.w && a.x+a.w > b.x && a.y < b.y+b.h && a.y+a.h > b.y
}

func rectContains(outer, inner rect) bool {
	return inner.x >= outer.x && inner.y >= outer.y &&
		inner.x+inner.w <= outer.x+outer.w && inner.y+inner.h <= outer.y+outer.h &&
		outer.area > inner.area*1.5
}

func severity(v, ok, warn int) string {
	if v <= ok {
		return "✓"
	}
	if v <= warn {
		return "⚠"
	}
	return "✗"
}

func capStat(n int) string {
	switch {
	case n <= 100:
		return "✓ 流畅"
	case n <= 300:
		return "✓ 正常"
	case n <= 500:
		return "⚠ 编辑略卡"
	case n <= 600:
		return "⚠ 接近上限"
	default:
		return "✗ 超出推荐上限"
	}
}

func init() {
	boardCmd.AddCommand(boardLintCmd)
	boardLintCmd.Flags().String("user-access-token", "", "User Access Token")
	boardLintCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
}
