package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

// image get
var sheetImageGetCmd = &cobra.Command{
	Use:   "get <spreadsheet_token> <sheet_id> <float_image_id>",
	Short: "获取浮动图片",
	Long: `根据 ID 获取工作表中的单个浮动图片。

示例:
  feishu-cli sheet image get shtcnxxxxxx 0b1212 ScDmuyHm`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken := args[0]
		sheetID := args[1]
		floatImageID := args[2]
		output, _ := cmd.Flags().GetString("output")

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		img, err := client.GetFloatImage(client.Context(), spreadsheetToken, sheetID, floatImageID, userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(img)
		}
		fmt.Printf("浮动图片信息:\n")
		fmt.Printf("  ID:    %s\n", img.FloatImageID)
		fmt.Printf("  Token: %s\n", img.FloatImageToken)
		fmt.Printf("  范围:  %s\n", img.Range)
		fmt.Printf("  尺寸:  %.0fx%.0f\n", img.Width, img.Height)
		fmt.Printf("  偏移:  (%.0f, %.0f)\n", img.OffsetX, img.OffsetY)
		return nil
	},
}

// image update
var sheetImageUpdateCmd = &cobra.Command{
	Use:   "update <spreadsheet_token> <sheet_id> <float_image_id>",
	Short: "更新浮动图片",
	Long: `更新浮动图片的锚点单元格 / 尺寸 / 偏移。仅更新显式传入的字段。
--range 为新锚点单元格，必须是单个单元格（如 0b1212!B2:B2）。

示例:
  feishu-cli sheet image update shtcnxxxxxx 0b1212 ScDmuyHm --width 200 --height 150
  feishu-cli sheet image update shtcnxxxxxx 0b1212 ScDmuyHm --range "0b1212!B2:B2" --offset-x 5`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken := args[0]
		sheetID := args[1]
		floatImageID := args[2]
		rangeStr, _ := cmd.Flags().GetString("range")
		width, _ := cmd.Flags().GetFloat64("width")
		height, _ := cmd.Flags().GetFloat64("height")
		output, _ := cmd.Flags().GetString("output")

		// offset-x/offset-y 用 Changed() 区分「未传」vs「显式传 0」——0 是合法偏移值。
		var offsetX, offsetY *float64
		if cmd.Flags().Changed("offset-x") {
			v, _ := cmd.Flags().GetFloat64("offset-x")
			offsetX = &v
		}
		if cmd.Flags().Changed("offset-y") {
			v, _ := cmd.Flags().GetFloat64("offset-y")
			offsetY = &v
		}

		// 用 Changed() 判断 width/height 是否显式设置（而非值是否为 0）——否则 --width 0 会被这里
		// 当成「未指定」误报，而不是落到下面 validateFloatImageUpdate 报「不能小于 20」。
		if rangeStr == "" && !cmd.Flags().Changed("width") && !cmd.Flags().Changed("height") && offsetX == nil && offsetY == nil {
			return fmt.Errorf("至少需要指定一个待更新字段（--range/--width/--height/--offset-x/--offset-y）")
		}

		// 与 help 声明一致的边界校验：width/height 仅在用户显式设置时校验 ≥20，offset 校验 ≥0。
		if err := validateFloatImageUpdate(
			cmd.Flags().Changed("width"), width,
			cmd.Flags().Changed("height"), height,
			offsetX, offsetY,
		); err != nil {
			return err
		}

		image := &client.FloatImage{
			Range:  unescapeSheetRange(rangeStr),
			Width:  width,
			Height: height,
		}

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		result, err := client.UpdateFloatImage(client.Context(), spreadsheetToken, sheetID, floatImageID, image, offsetX, offsetY, userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(result)
		}
		fmt.Printf("浮动图片更新成功！\n")
		fmt.Printf("  ID:   %s\n", result.FloatImageID)
		fmt.Printf("  范围: %s\n", result.Range)
		fmt.Printf("  尺寸: %.0fx%.0f\n", result.Width, result.Height)
		return nil
	},
}

// image media-upload
var sheetImageMediaUploadCmd = &cobra.Command{
	Use:   "media-upload <spreadsheet_token> <file>",
	Short: "上传本地图片素材，返回 file_token",
	Long: `上传本地图片作为浮动图片素材，返回 file_token（再用于 sheet image add）。
parent_type 固定为 sheet_image，parent_node 为电子表格 token。

示例:
  feishu-cli sheet image media-upload shtcnxxxxxx ./logo.png
  feishu-cli sheet image media-upload shtcnxxxxxx ./logo.png --name banner.png -o json`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken := args[0]
		filePath := args[1]
		name, _ := cmd.Flags().GetString("name")
		output, _ := cmd.Flags().GetString("output")

		if name == "" {
			name = filepath.Base(filePath)
		}

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		fileToken, err := client.UploadSheetImageMedia(filePath, spreadsheetToken, name, userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(map[string]string{"file_token": fileToken})
		}
		fmt.Printf("素材上传成功！\n")
		fmt.Printf("  file_token: %s\n", fileToken)
		return nil
	},
}

// image write-image
var sheetImageWriteCmd = &cobra.Command{
	Use:   "write-image <spreadsheet_token> <sheet_id>",
	Short: "把本地图片写入单元格",
	Long: `将本地图片写入指定单元格（值类型为图片，非浮动图片）。目标范围起止单元格必须相同。

示例:
  feishu-cli sheet image write-image shtcnxxxxxx 0b1212 --range "0b1212!A1" --image ./logo.png
  feishu-cli sheet image write-image shtcnxxxxxx 0b1212 --range "A1" --image ./logo.png --name logo.png`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken := args[0]
		sheetID := args[1]
		rangeStr, _ := cmd.Flags().GetString("range")
		imagePath, _ := cmd.Flags().GetString("image")
		name, _ := cmd.Flags().GetString("name")

		if rangeStr == "" || imagePath == "" {
			return fmt.Errorf("--range、--image 均为必填项")
		}
		if name == "" {
			name = filepath.Base(imagePath)
		}

		normalizedRange, err := normalizeSheetWriteImageRange(unescapeSheetRange(rangeStr), sheetID)
		if err != nil {
			return err
		}
		rangeStr = normalizedRange

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		if err := client.WriteSheetImage(client.Context(), spreadsheetToken, rangeStr, imagePath, name, userAccessToken); err != nil {
			return err
		}
		fmt.Printf("图片写入成功！范围: %s\n", rangeStr)
		return nil
	},
}

// normalizeSheetWriteImageRange 把 write-image 的目标范围规整为带 sheetId 前缀、起止单元格相同的单格范围。
// 支持输入: "A1" / "<sheetId>!A1" / "<sheetId>!A1:A1"，输出统一为 "<sheetId>!A1:A1"。
func normalizeSheetWriteImageRange(rangeStr, sheetID string) (string, error) {
	body := strings.TrimSpace(rangeStr)
	prefix := sheetID
	if idx := strings.Index(body, "!"); idx >= 0 {
		prefix = body[:idx]
		body = body[idx+1:]
	}

	start := body
	end := body
	if c := strings.Index(body, ":"); c >= 0 {
		start = body[:c]
		end = body[c+1:]
	}
	start = strings.TrimSpace(start)
	end = strings.TrimSpace(end)
	if start == "" || end == "" {
		return "", fmt.Errorf("--range 必须是单个单元格，如 A1 或 %s!A1", sheetID)
	}
	if start != end {
		return "", fmt.Errorf("sheet image write-image 只支持单个单元格，不能写入多单元格范围 %q；请改用 %s", rangeStr, start)
	}
	if prefix == "" {
		return start + ":" + start, nil
	}
	return prefix + "!" + start + ":" + start, nil
}

// validateFloatImageUpdate 校验 sheet image update 的尺寸/偏移边界，与 help 声明一致：
// width/height 仅在用户显式设置时校验 ≥20（飞书浮图最小尺寸），offset-x/offset-y ≥0（不为负）。
func validateFloatImageUpdate(widthChanged bool, width float64, heightChanged bool, height float64, offsetX, offsetY *float64) error {
	if widthChanged && width < 20 {
		return fmt.Errorf("--width 不能小于 20（当前 %.0f）", width)
	}
	if heightChanged && height < 20 {
		return fmt.Errorf("--height 不能小于 20（当前 %.0f）", height)
	}
	if offsetX != nil && *offsetX < 0 {
		return fmt.Errorf("--offset-x 不能为负（当前 %.0f）", *offsetX)
	}
	if offsetY != nil && *offsetY < 0 {
		return fmt.Errorf("--offset-y 不能为负（当前 %.0f）", *offsetY)
	}
	return nil
}

func init() {
	sheetImageCmd.AddCommand(sheetImageGetCmd)
	sheetImageCmd.AddCommand(sheetImageUpdateCmd)
	sheetImageCmd.AddCommand(sheetImageMediaUploadCmd)
	sheetImageCmd.AddCommand(sheetImageWriteCmd)

	// get
	sheetImageGetCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	sheetImageGetCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")

	// update
	sheetImageUpdateCmd.Flags().String("range", "", "新锚点单元格（单格，如 0b1212!B2:B2）")
	sheetImageUpdateCmd.Flags().Float64("width", 0, "新宽度（像素，最小 20）")
	sheetImageUpdateCmd.Flags().Float64("height", 0, "新高度（像素，最小 20）")
	sheetImageUpdateCmd.Flags().Float64("offset-x", 0, "水平偏移（像素，≥0）")
	sheetImageUpdateCmd.Flags().Float64("offset-y", 0, "垂直偏移（像素，≥0）")
	sheetImageUpdateCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	sheetImageUpdateCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")

	// media-upload
	sheetImageMediaUploadCmd.Flags().String("name", "", "图片文件名（默认取文件 basename）")
	sheetImageMediaUploadCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	sheetImageMediaUploadCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")

	// write-image
	sheetImageWriteCmd.Flags().String("range", "", "目标单元格（如 A1 或 <sheetId>!A1，起止须相同）（必填）")
	sheetImageWriteCmd.Flags().String("image", "", "本地图片文件路径（必填）")
	sheetImageWriteCmd.Flags().String("name", "", "图片文件名（默认取文件 basename）")
	sheetImageWriteCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
	mustMarkFlagRequired(sheetImageWriteCmd, "range", "image")
}
