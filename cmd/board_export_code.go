package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/safefile"
	"github.com/spf13/cobra"
)

var boardExportCodeCmd = &cobra.Command{
	Use:   "export-code <whiteboard_id>",
	Short: "提取画板中的 SVG 代码，或用 --source 取回 Mermaid/PlantUML 源码",
	Long: `默认从画板中提取所有 svg 节点的 svg_code，按 z_index 顺序拼接，输出到文件或 stdout。

--source 模式：取回经服务端导入的 Mermaid / PlantUML 图表源码（节点上的 syntax.code），
便于"导出源码 → 本地修改 → board import --overwrite 写回"。画板上有多个图表时用 --node-id 指定其一；
--output-path 不带扩展名时按语法补 .mmd / .puml。

适用场景:
  - 把 AI 生成 + 落板后的 SVG 拉回本地，便于版本管理和二次编辑
  - 把多个 svg 节点的代码导出为单一 SVG 文件
  - 取回文档里 Mermaid/PlantUML 图表的源码继续编辑

示例:
  feishu-cli board export-code <id>                # stdout
  feishu-cli board export-code <id> --output-path design.svg
  feishu-cli board export-code <id> --output-path design.svg --merge
  feishu-cli board export-code <id> --source                       # 打印 Mermaid/PlantUML 源码
  feishu-cli board export-code <id> --source --node-id t1:2 --output-path diagram`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		whiteboardID := args[0]
		outputPath, _ := cmd.Flags().GetString("output-path")
		legacyOutputPath, _ := cmd.Flags().GetString("output")
		if outputPath == "" {
			outputPath = legacyOutputPath
		}
		merge, _ := cmd.Flags().GetBool("merge")
		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		raw, err := client.GetBoardNodes(whiteboardID, userAccessToken)
		if err != nil {
			return err
		}
		var apiResp struct {
			Code int `json:"code"`
			Data struct {
				Nodes json.RawMessage `json:"nodes"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &apiResp); err != nil {
			return err
		}
		nodes, err := parseBoardExportNodes(apiResp.Data.Nodes)
		if err != nil {
			return err
		}
		if source, _ := cmd.Flags().GetBool("source"); source {
			nodeID, _ := cmd.Flags().GetString("node-id")
			return exportBoardDiagramSource(nodes, strings.TrimSpace(nodeID), outputPath)
		}

		type svgItem struct {
			zIndex  float64
			x, y, w float64
			h       float64
			code    string
		}
		var items []svgItem
		for _, node := range nodes {
			if t, _ := node["type"].(string); t != "svg" {
				continue
			}
			svgObj, ok := node["svg"].(map[string]any)
			if !ok {
				continue
			}
			code, _ := svgObj["svg_code"].(string)
			if code == "" {
				continue
			}
			z, _ := node["z_index"].(float64)
			x, _ := node["x"].(float64)
			y, _ := node["y"].(float64)
			w, _ := node["width"].(float64)
			h, _ := node["height"].(float64)
			items = append(items, svgItem{zIndex: z, x: x, y: y, w: w, h: h, code: code})
		}

		if len(items) == 0 {
			return fmt.Errorf("画板中没有 svg 节点")
		}

		sort.SliceStable(items, func(i, j int) bool { return items[i].zIndex < items[j].zIndex })

		var content string
		if merge {
			// 合并为单一 SVG：计算包围盒，每个 sub-svg 用 <g transform> 平移
			var minX, minY = items[0].x, items[0].y
			var maxX, maxY = items[0].x + items[0].w, items[0].y + items[0].h
			for _, it := range items[1:] {
				if it.x < minX {
					minX = it.x
				}
				if it.y < minY {
					minY = it.y
				}
				if it.x+it.w > maxX {
					maxX = it.x + it.w
				}
				if it.y+it.h > maxY {
					maxY = it.y + it.h
				}
			}
			totalW := maxX - minX
			totalH := maxY - minY
			var sb strings.Builder
			fmt.Fprintf(&sb, `<svg viewBox="0 0 %.0f %.0f" width="%.0f" height="%.0f" xmlns="http://www.w3.org/2000/svg">`+"\n",
				totalW, totalH, totalW, totalH)
			for _, it := range items {
				inner := stripSVGWrapper(it.code)
				fmt.Fprintf(&sb, `  <g transform="translate(%.2f %.2f)">%s</g>`+"\n",
					it.x-minX, it.y-minY, inner)
			}
			sb.WriteString("</svg>\n")
			content = sb.String()
		} else {
			// 仅拼接（按 z_index 顺序）
			var sb strings.Builder
			for i, it := range items {
				fmt.Fprintf(&sb, "<!-- node #%d z=%v pos=(%.0f,%.0f) size=%.0fx%.0f -->\n",
					i+1, it.zIndex, it.x, it.y, it.w, it.h)
				sb.WriteString(it.code)
				sb.WriteString("\n\n")
			}
			content = sb.String()
		}

		if outputPath == "" {
			fmt.Print(content)
		} else {
			if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
				return fmt.Errorf("写文件失败: %w", err)
			}
			fmt.Printf("提取 %d 个 svg 节点 → %s（合并: %v）\n", len(items), outputPath, merge)
		}
		return nil
	},
}

// boardSourceBlock 画板中保留了图表源码的节点（服务端 import 的 Mermaid/PlantUML 图表会在 section 节点上留 syntax.code）。
type boardSourceBlock struct {
	NodeID string
	Syntax string
	Code   string
}

var boardSourceSyntaxNames = map[int]string{1: "plantuml", 2: "mermaid"}
var boardSourceSyntaxExt = map[string]string{"plantuml": ".puml", "mermaid": ".mmd"}

// collectBoardSourceBlocks 提取 syntax.code（syntax_type 1=PlantUML 2=Mermaid），按节点 ID 排序。
func collectBoardSourceBlocks(nodes []map[string]any) []boardSourceBlock {
	var blocks []boardSourceBlock
	for _, n := range nodes {
		syn, ok := n["syntax"].(map[string]any)
		if !ok {
			continue
		}
		code, _ := syn["code"].(string)
		st, _ := syn["syntax_type"].(float64)
		name, known := boardSourceSyntaxNames[int(st)]
		if strings.TrimSpace(code) == "" || !known {
			continue
		}
		id, _ := n["id"].(string)
		blocks = append(blocks, boardSourceBlock{NodeID: id, Syntax: name, Code: code})
	}
	sort.SliceStable(blocks, func(i, j int) bool { return blocks[i].NodeID < blocks[j].NodeID })
	return blocks
}

// exportBoardDiagramSource 取回画板上 Mermaid/PlantUML 图表的源码（对齐官方 whiteboard +export --output-type source）。
// 只有一个源码块时直接输出；多个时要求 --node-id 指定，避免把多张图拼成无法解析的文件。
func exportBoardDiagramSource(nodes []map[string]any, nodeID, outputPath string) error {
	blocks := collectBoardSourceBlocks(nodes)
	if len(blocks) == 0 {
		return fmt.Errorf("画板中没有可取回源码的 Mermaid/PlantUML 图表（只有经 board import 服务端引擎或文档导入生成的图表保留 syntax.code；SVG/手绘节点请用 board export-code 或 board svg-export）")
	}
	if nodeID != "" {
		var picked []boardSourceBlock
		for _, b := range blocks {
			if b.NodeID == nodeID {
				picked = append(picked, b)
			}
		}
		if len(picked) == 0 {
			return clierr.Usagef("--node-id %s 不是带源码的图表节点；可选: %s", nodeID, boardSourceBlockList(blocks))
		}
		blocks = picked
	}
	if len(blocks) > 1 {
		return clierr.Usagef("画板中有 %d 个图表源码块，请用 --node-id 指定其一: %s", len(blocks), boardSourceBlockList(blocks))
	}
	b := blocks[0]
	if outputPath == "" {
		fmt.Print(b.Code)
		if !strings.HasSuffix(b.Code, "\n") {
			fmt.Println()
		}
		return nil
	}
	if filepath.Ext(outputPath) == "" {
		outputPath += boardSourceSyntaxExt[b.Syntax]
	}
	if err := safefile.ValidateOutputPath(outputPath); err != nil {
		return err
	}
	if err := safefile.AtomicWriteFile(outputPath, []byte(b.Code), 0o644); err != nil {
		return fmt.Errorf("写文件失败: %w", err)
	}
	fmt.Printf("已导出 %s 源码（节点 %s）→ %s\n", b.Syntax, b.NodeID, outputPath)
	return nil
}

func boardSourceBlockList(blocks []boardSourceBlock) string {
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		parts = append(parts, fmt.Sprintf("%s(%s)", b.NodeID, b.Syntax))
	}
	return strings.Join(parts, ", ")
}

func parseBoardExportNodes(raw json.RawMessage) ([]map[string]any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	var nodeArray []map[string]any
	if err := json.Unmarshal(raw, &nodeArray); err == nil {
		return nodeArray, nil
	}

	var nodeMap map[string]map[string]any
	if err := json.Unmarshal(raw, &nodeMap); err == nil {
		nodes := make([]map[string]any, 0, len(nodeMap))
		for id, node := range nodeMap {
			if node == nil {
				continue
			}
			if _, ok := node["id"]; !ok && id != "" {
				node["id"] = id
			}
			nodes = append(nodes, node)
		}
		return nodes, nil
	}

	return nil, fmt.Errorf("解析画板节点失败：nodes 既不是数组也不是对象")
}

// stripSVGWrapper 去除最外层 <svg ...> 和 </svg>，只留内部内容（用于 merge）
func stripSVGWrapper(svgCode string) string {
	s := strings.TrimSpace(svgCode)
	openIdx := strings.Index(s, ">")
	if openIdx < 0 {
		return s
	}
	closeIdx := strings.LastIndex(s, "</svg>")
	if closeIdx < 0 {
		return s
	}
	return strings.TrimSpace(s[openIdx+1 : closeIdx])
}

func init() {
	boardCmd.AddCommand(boardExportCodeCmd)
	boardExportCodeCmd.Flags().String("output-path", "", "输出文件路径（不指定则打印到 stdout）")
	boardExportCodeCmd.Flags().String("output", "", "输出文件路径（兼容旧参数，请改用 --output-path）")
	_ = boardExportCodeCmd.Flags().MarkDeprecated("output", "请改用 --output-path")
	boardExportCodeCmd.Flags().Bool("merge", false, "合并所有 svg 为单一 SVG（带 viewBox 自动包围）")
	boardExportCodeCmd.Flags().Bool("source", false, "取回 Mermaid/PlantUML 图表源码（节点 syntax.code）而不是 svg 代码")
	boardExportCodeCmd.Flags().String("node-id", "", "--source 模式下指定图表节点（画板有多个图表时必填）")
	boardExportCodeCmd.Flags().String("user-access-token", "", "User Access Token")
}
