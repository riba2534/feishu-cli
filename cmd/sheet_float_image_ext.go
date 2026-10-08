package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/safefile"
	"github.com/spf13/cobra"
)

// image get
var sheetImageGetCmd = &cobra.Command{
	Use:   "get <spreadsheet_token|url> <sheet_id> <float_image_id>",
	Short: "获取浮动图片",
	Long: `根据 ID 获取工作表中的单个浮动图片。

示例:
  feishu-cli sheet image get shtcnxxxxxx 0b1212 ScDmuyHm`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		sheetID := args[1]
		floatImageID := args[2]
		output, _ := cmd.Flags().GetString("output")

		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}
		spreadsheetToken, userAccessToken := target.Token, target.UAT

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
	Use:   "update <spreadsheet_token|url> <sheet_id> <float_image_id>",
	Short: "更新浮动图片",
	Long: `更新浮动图片的锚点单元格 / 尺寸 / 偏移。仅更新显式传入的字段。
--range 为新锚点单元格，必须是单个单元格（如 0b1212!B2:B2）。

示例:
  feishu-cli sheet image update shtcnxxxxxx 0b1212 ScDmuyHm --width 200 --height 150
  feishu-cli sheet image update shtcnxxxxxx 0b1212 ScDmuyHm --range "0b1212!B2:B2" --offset-x 5`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
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

		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}
		spreadsheetToken, userAccessToken := target.Token, target.UAT
		if rangeStr != "" {
			if rangeStr, err = target.qualifyRange(rangeStr, sheetID, ""); err != nil {
				return err
			}
		}
		image := &client.FloatImage{
			Range:  rangeStr,
			Width:  width,
			Height: height,
		}

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
	Use:   "media-upload <spreadsheet_token|url> <file>",
	Short: "上传本地图片素材，返回 file_token",
	Long: `上传本地图片作为浮动图片素材，返回 file_token（再用于 sheet image add）。
parent_type 按表格类型自动选择：原生飞书表格用 sheet_image，导入型 office 表格
（token 以 fake_office_/local_office_ 开头，或长度 ≥25 且第 5/10/15/20/25 位依次为 OFL0X）用 office_sheet_file；parent_node 为电子表格 token。
文件 ≤20MB 走 medias/upload_all，超过 20MB 自动改用分片上传（upload_prepare/upload_part/upload_finish）；
注意实测浮动图片接口不接受超过 20MB 的图片（1310245），作浮动图片前请先压缩。

示例:
  feishu-cli sheet image media-upload shtcnxxxxxx ./logo.png
  feishu-cli sheet image media-upload shtcnxxxxxx ./logo.png --name banner.png -o json`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[1]
		name, _ := cmd.Flags().GetString("name")
		output, _ := cmd.Flags().GetString("output")

		if name == "" {
			name = filepath.Base(filePath)
		}
		if err := safefile.ValidateInputPath(filePath); err != nil {
			return err
		}

		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}
		spreadsheetToken, userAccessToken := target.Token, target.UAT

		// >20MB 自动走分片上传（upload_all 对超限文件只回 1061002 params error）
		if client.SheetImageUsesMultipart(filePath) {
			fmt.Fprintln(os.Stderr, "提示：文件超过 20MB，改用分片上传。实测浮动图片接口（sheet image add）不接受超过 20MB 的图片"+
				"（返回 1310245 Wrong Float Image Token），如需作为浮动图片使用请先压缩到 20MB 以内")
		}
		fileToken, err := client.UploadSheetImageMediaAuto(filePath, spreadsheetToken, name, userAccessToken)
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
	Use:   "write-image <spreadsheet_token|url> <sheet_id>",
	Short: "把网络或本地图片写入单元格",
	Long: `将网络图片（HTTPS URL）或本地图片写入指定单元格（原生图片单元格，非浮动图片），并通过 V3 read-rich 回读验证。
目标范围起止单元格必须相同（单格）。

写入规则与说明:
  - --image 接受 HTTPS 网络图片 URL 或本地图片文件路径。
  - --range 必须是单个单元格（如 A1 或 0b1212!A1:A1）。
  - 网络图片下载后会校验响应状态、图片格式和大小（默认 ≤20 MiB）。
  - 严禁用 =IMAGE(...) 公式或 Markdown 图片语法替代；必须写入原生图片单元格以保证持久渲染。
  - JPEG/PNG/GIF 直接写入；BMP/TIFF/WebP 自动转 PNG，原文件不变。
  - HEIC/BPG 原样提交，能否写入取决于服务端支持，失败时返回非零退出码。
  - 文件名缺少有效图片后缀时按实际格式补齐；转码图片统一使用 .png 后缀。
  - 写入完成后自动通过 V3 read-rich 回读验证原生 image_token。

示例:
  # 写入网络图片并回读验证
  feishu-cli sheet image write-image shtcnxxxxxx 0b1212 --range "A1" --image https://example.com/logo.png

  # 写入本地图片
  feishu-cli sheet image write-image shtcnxxxxxx 0b1212 --range "0b1212!B2" --image ./logo.png --name logo.png -o json`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sheetID := args[1]
		rangeStr, _ := cmd.Flags().GetString("range")
		imagePath, _ := cmd.Flags().GetString("image")
		name, _ := cmd.Flags().GetString("name")
		allowPrivate, _ := cmd.Flags().GetBool("allow-private-net")
		output, _ := cmd.Flags().GetString("output")
		if output != "" && output != "text" && output != "json" {
			return fmt.Errorf("不支持的输出格式 %q，仅支持 text, json", output)
		}

		if rangeStr == "" || imagePath == "" {
			return fmt.Errorf("--range、--image 均为必填项")
		}

		rangeStr = unescapeSheetRange(rangeStr)
		prefix, rest, hasPrefix := client.SplitSheetRangePrefix(rangeStr)
		if !hasPrefix || prefix == sheetID {
			// 前缀缺省或就是 sheetId：联网前先做本地校验
			if _, err := normalizeSheetWriteImageRange(rangeStr, sheetID); err != nil {
				return err
			}
		}

		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}
		spreadsheetToken, userAccessToken := target.Token, target.UAT
		if hasPrefix && prefix != sheetID {
			// 前缀是子表名：换算成 sheetId 后再校验（与 <sheet_id> 不是同一子表时照常报错）
			id, err := target.resolveSheetRef(prefix)
			if err != nil {
				return err
			}
			rangeStr = id + "!" + rest
		}
		normalizedRange, err := normalizeSheetWriteImageRange(rangeStr, sheetID)
		if err != nil {
			return err
		}

		item := client.BatchWriteSheetImageItem{
			Cell: normalizedRange,
			Name: name,
		}
		trimmedImage := strings.TrimSpace(imagePath)
		lowerImage := strings.ToLower(trimmedImage)
		if strings.HasPrefix(lowerImage, "https://") || strings.HasPrefix(lowerImage, "http://") {
			item.URL = trimmedImage
		} else {
			item.Path = trimmedImage
		}

		opts := client.BatchWriteSheetImageOptions{
			Workers:         1,
			MaxBytes:        client.DefaultSheetImageBatchMaxBytes,
			AllowPrivateNet: allowPrivate,
		}

		result, runErr := client.BatchWriteSheetImages(cmd.Context(), spreadsheetToken, sheetID, []client.BatchWriteSheetImageItem{item}, opts, userAccessToken)
		if output == "json" {
			if len(result.Outcomes) > 0 {
				if err := printJSON(result.Outcomes[0]); err != nil {
					return err
				}
			} else {
				if err := printJSON(result); err != nil {
					return err
				}
			}
		} else {
			if runErr == nil && len(result.Outcomes) > 0 && result.Outcomes[0].Status == "verified" {
				fmt.Fprintf(cmd.OutOrStdout(), "图片写入成功！范围: %s，图片 Token: %s\n", normalizedRange, result.Outcomes[0].ImageToken)
			}
		}
		if runErr != nil {
			if len(result.Outcomes) > 0 && result.Outcomes[0].Error != "" {
				return fmt.Errorf("图片写入失败: %s", result.Outcomes[0].Error)
			}
			return runErr
		}
		return nil
	},
}

// normalizeSheetWriteImageRange 把 write-image 的目标范围规整为带 sheetId 前缀、起止单元格相同的单格范围。
// 支持输入: "A1" / "<sheetId>!A1" / "<sheetId>!A1:A1"，输出统一为 "<sheetId>!A1:A1"。
func normalizeSheetWriteImageRange(rangeStr, sheetID string) (string, error) {
	body := strings.TrimSpace(rangeStr)
	prefix := sheetID
	if idx := strings.LastIndex(body, "!"); idx >= 0 {
		p := strings.Trim(body[:idx], "'")
		if p != sheetID {
			return "", fmt.Errorf("单元格范围的工作表 ID %q 与目标 %q 不一致（飞书接口要求使用 sheet_id 而非工作表标题）", p, sheetID)
		}
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
	if !strings.EqualFold(start, end) {
		return "", fmt.Errorf("sheet image write-image 只支持单个单元格，不能写入多单元格范围 %q；请改用 %s", rangeStr, start)
	}
	if !sheetCellRE.MatchString(start) {
		return "", fmt.Errorf("--range 必须是有效的单个单元格地址（当前 %q）", start)
	}
	start = strings.ToUpper(start)
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
	sheetImageWriteCmd.Flags().String("image", "", "图片文件路径或 HTTPS URL（必填）")
	sheetImageWriteCmd.Flags().String("name", "", "图片文件名（默认取文件 basename 或 URL 资源名）")
	sheetImageWriteCmd.Flags().Bool("allow-private-net", false, "允许从私有网络或内网 IP 下载图片（用于企业内网 CDN/对象存储）")
	sheetImageWriteCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	sheetImageWriteCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
	mustMarkFlagRequired(sheetImageWriteCmd, "range", "image")
}
