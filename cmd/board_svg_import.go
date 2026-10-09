package cmd

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var boardSVGImportCmd = &cobra.Command{
	Use:   "svg-import <whiteboard_id> <source>",
	Short: "导入 SVG 到画板（落为单个 svg 节点）",
	Long: `把整段 SVG 代码作为单个 type=svg 画板节点上传：整张图是一个整体贴图，选中只能整体移动/缩放，
不能单独改其中某个形状的颜色或文字；复杂 SVG（上千元素）还可能渲染不全。
适合 < 2KB 的图标、印章、小装饰。需要每个元素都可单独编辑时改用
board import --syntax svg（服务端把 SVG 拆成原生节点）或 svg_to_board.py 本地管道。

参数:
  <whiteboard_id>   画板唯一标识符（必填）
  <source>          SVG 文件路径 或 SVG 字符串（--source-type content）

特性:
  - 自动解析 SVG viewBox 推断默认宽高，也可用 --width/--height 覆盖
  - --dry-run 仅打印将创建的节点 JSON，不调用 API
  - --client-token 幂等键（≥10 字符）：结果未知时用同一个值重跑不会重复建节点

示例:
  # 从文件导入
  feishu-cli board svg-import <whiteboard_id> drawing.svg

  # 指定落点和尺寸
  feishu-cli board svg-import <whiteboard_id> drawing.svg --x 100 --y 100 --width 400 --height 300

  # 直接传字符串
  feishu-cli board svg-import <whiteboard_id> '<svg viewBox="0 0 100 100">...</svg>' --source-type content

  # 预览不发请求
  feishu-cli board svg-import <whiteboard_id> drawing.svg --dry-run`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		whiteboardID := args[0]
		source := args[1]
		sourceType, _ := cmd.Flags().GetString("source-type")
		x, _ := cmd.Flags().GetFloat64("x")
		y, _ := cmd.Flags().GetFloat64("y")
		widthFlag, _ := cmd.Flags().GetFloat64("width")
		heightFlag, _ := cmd.Flags().GetFloat64("height")
		zIndex, _ := cmd.Flags().GetInt("z-index")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		output, _ := cmd.Flags().GetString("output")
		clientToken, _ := cmd.Flags().GetString("client-token")
		clientToken = strings.TrimSpace(clientToken)
		if err := client.ValidateBoardClientToken(clientToken); err != nil {
			return clierr.Usage(err)
		}
		userAccessToken := resolveOptionalUserToken(cmd)

		// 读 SVG 内容
		var svgCode string
		if sourceType == "content" {
			svgCode = source
		} else {
			data, err := readLocalInputFile(source)
			if err != nil {
				return fmt.Errorf("读取 SVG 文件失败: %w", err)
			}
			svgCode = string(data)
		}

		svgCode = strings.TrimSpace(svgCode)
		if !strings.Contains(svgCode, "<svg") {
			return fmt.Errorf("内容不是有效的 SVG（未找到 <svg 标签）")
		}

		// 推断宽高
		vbW, vbH, vbErr := parseSVGDimensions(svgCode)
		width := widthFlag
		if width == 0 {
			width = vbW
		}
		height := heightFlag
		if height == 0 {
			height = vbH
		}
		if width == 0 || height == 0 {
			if vbErr != nil {
				return fmt.Errorf("解析 SVG 尺寸失败且未指定 --width/--height: %w", vbErr)
			}
			return fmt.Errorf("SVG 缺少 viewBox/width/height，请显式传 --width --height")
		}

		// 构造节点 JSON
		node := map[string]any{
			"type":    "svg",
			"x":       x,
			"y":       y,
			"width":   width,
			"height":  height,
			"angle":   0,
			"z_index": zIndex,
			"svg": map[string]any{
				"key":      "",
				"svg_code": svgCode,
				"type":     0,
			},
			"style": map[string]any{
				"border_color":            "#4e83fd",
				"border_color_type":       0,
				"border_opacity":          100,
				"border_style":            "none",
				"border_width":            "narrow",
				"fill_color_type":         0,
				"fill_opacity":            100,
				"theme_border_color_code": -1,
				"theme_fill_color_code":   -1,
			},
		}

		nodesBytes, err := json.Marshal([]map[string]any{node})
		if err != nil {
			return fmt.Errorf("序列化节点失败: %w", err)
		}

		if dryRun {
			result := map[string]any{
				"whiteboard_id": whiteboardID,
				"x":             x, "y": y, "width": width, "height": height,
				"z_index":      zIndex,
				"svg_code_len": len(svgCode),
				"node":         node,
				"dry_run":      true,
			}
			if output == "json" {
				return printJSON(result)
			}
			fmt.Printf("[dry-run] 将创建 svg 节点：位置 (%.0f, %.0f) 尺寸 %.0fx%.0f svg_code 长度 %d\n",
				x, y, width, height, len(svgCode))
			return nil
		}

		if err := config.Validate(); err != nil {
			return err
		}

		opts := client.CreateBoardNotesOptions{
			UserAccessToken: userAccessToken,
			ClientToken:     clientToken,
		}
		nodeIDs, err := client.CreateBoardNodes(whiteboardID, string(nodesBytes), opts)
		if err != nil {
			return err
		}

		if output == "json" {
			out := map[string]any{
				"whiteboard_id": whiteboardID,
				"node_ids":      nodeIDs,
				"count":         len(nodeIDs),
				"x":             x, "y": y, "width": width, "height": height,
			}
			if clientToken != "" {
				out["client_token"] = clientToken
			}
			return printJSON(out)
		}
		fmt.Printf("SVG 节点已创建：\n  画板 ID: %s\n  节点 ID: %s\n  位置: (%.0f, %.0f)  尺寸: %.0fx%.0f\n",
			whiteboardID, firstID(nodeIDs), x, y, width, height)
		return nil
	},
}

// firstID 安全返回首个节点 ID
func firstID(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

// buildSVGNode 构造单个 svg 节点（map 形式）
func buildSVGNode(svgCode string, x, y, width, height float64, zIndex int) (map[string]any, error) {
	svgCode = strings.TrimSpace(svgCode)
	if !strings.Contains(svgCode, "<svg") {
		return nil, fmt.Errorf("内容不是有效的 SVG（未找到 <svg 标签）")
	}
	if width == 0 || height == 0 {
		w, h, err := parseSVGDimensions(svgCode)
		if err != nil || w == 0 || h == 0 {
			if width == 0 {
				width = 600
			}
			if height == 0 {
				height = 400
			}
		} else {
			if width == 0 {
				width = w
			}
			if height == 0 {
				height = h
			}
		}
	}
	return map[string]any{
		"type":    "svg",
		"x":       x,
		"y":       y,
		"width":   width,
		"height":  height,
		"angle":   0,
		"z_index": zIndex,
		"svg": map[string]any{
			"key":      "",
			"svg_code": svgCode,
			"type":     0,
		},
		"style": map[string]any{
			"border_color":            "#4e83fd",
			"border_color_type":       0,
			"border_opacity":          100,
			"border_style":            "none",
			"border_width":            "narrow",
			"fill_color_type":         0,
			"fill_opacity":            100,
			"theme_border_color_code": -1,
			"theme_fill_color_code":   -1,
		},
	}, nil
}

// buildSVGNodeJSON 构造单个 svg 节点的 JSON 数组（供 Markdown 导入复用，默认落点 0,0）
func buildSVGNodeJSON(svgCode string) (string, error) {
	node, err := buildSVGNode(svgCode, 0, 0, 0, 0, 10)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal([]map[string]any{node})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// parseSVGDimensions 从 SVG 字符串解析 viewBox 或 width/height 属性
func parseSVGDimensions(svgCode string) (float64, float64, error) {
	type svgRoot struct {
		XMLName xml.Name `xml:"svg"`
		ViewBox string   `xml:"viewBox,attr"`
		Width   string   `xml:"width,attr"`
		Height  string   `xml:"height,attr"`
	}
	var root svgRoot
	decoder := xml.NewDecoder(strings.NewReader(svgCode))
	decoder.Strict = false
	decoder.AutoClose = xml.HTMLAutoClose
	decoder.Entity = xml.HTMLEntity
	if err := decoder.Decode(&root); err != nil {
		return 0, 0, err
	}
	if root.ViewBox != "" {
		parts := strings.Fields(strings.ReplaceAll(root.ViewBox, ",", " "))
		if len(parts) == 4 {
			w, errW := strconv.ParseFloat(parts[2], 64)
			h, errH := strconv.ParseFloat(parts[3], 64)
			if errW == nil && errH == nil && w > 0 && h > 0 {
				return w, h, nil
			}
		}
	}
	if root.Width != "" && root.Height != "" {
		wStr := strings.TrimSuffix(strings.TrimSuffix(root.Width, "px"), "PX")
		hStr := strings.TrimSuffix(strings.TrimSuffix(root.Height, "px"), "PX")
		w, errW := strconv.ParseFloat(wStr, 64)
		h, errH := strconv.ParseFloat(hStr, 64)
		if errW == nil && errH == nil && w > 0 && h > 0 {
			return w, h, nil
		}
	}
	return 0, 0, fmt.Errorf("SVG 无 viewBox / width / height 属性")
}

func init() {
	boardCmd.AddCommand(boardSVGImportCmd)
	boardSVGImportCmd.Flags().String("source-type", "file", "源类型 (file/content)")
	boardSVGImportCmd.Flags().Float64("x", 0, "节点 x 坐标")
	boardSVGImportCmd.Flags().Float64("y", 0, "节点 y 坐标")
	boardSVGImportCmd.Flags().Float64("width", 0, "节点宽度（默认按 SVG viewBox 推断）")
	boardSVGImportCmd.Flags().Float64("height", 0, "节点高度（默认按 SVG viewBox 推断）")
	boardSVGImportCmd.Flags().Int("z-index", 10, "节点层级")
	boardSVGImportCmd.Flags().Bool("dry-run", false, "预览节点 JSON 但不调用 API")
	boardSVGImportCmd.Flags().String("user-access-token", "", "User Access Token")
	boardSVGImportCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
	boardSVGImportCmd.Flags().String("client-token", "", "幂等键（≥10 字符；结果未知时用同一个值重跑不会重复建节点）")
}
