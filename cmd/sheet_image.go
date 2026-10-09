package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

// 浮动图片命令组
var sheetImageCmd = &cobra.Command{
	Use:   "image",
	Short: "浮动图片操作",
	Long:  "工作表浮动图片相关操作",
}

var sheetImageAddCmd = &cobra.Command{
	Use:   "add <spreadsheet_token|url> <sheet_id>",
	Short: "添加浮动图片",
	Long: `在工作表中添加浮动图片。--token 为 sheet image media-upload 返回的 file_token；
--range 为锚点单元格（如 A1:A1，不带前缀时自动补上 <sheet_id>）。

示例:
  feishu-cli sheet image add shtcnxxxxxx 0b12 --token img_xxx --range "A1:A1" --width 200 --height 150`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sheetID := args[1]
		imageToken, _ := cmd.Flags().GetString("token")
		rangeStr, _ := cmd.Flags().GetString("range")
		width, _ := cmd.Flags().GetFloat64("width")
		height, _ := cmd.Flags().GetFloat64("height")
		offsetX, _ := cmd.Flags().GetFloat64("offset-x")
		offsetY, _ := cmd.Flags().GetFloat64("offset-y")
		output, _ := cmd.Flags().GetString("output")

		image := &client.FloatImage{
			FloatImageToken: imageToken,
			Range:           rangeStr,
			Width:           width,
			Height:          height,
			OffsetX:         offsetX,
			OffsetY:         offsetY,
		}

		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}
		spreadsheetToken, userAccessToken := target.Token, target.UAT
		// 锚点范围不带子表前缀时补上 <sheet_id>（接口要求带前缀，否则 1310211 Wrong Sheet Id）
		if image.Range, err = target.qualifyRange(rangeStr, sheetID, ""); err != nil {
			return err
		}

		result, err := client.CreateFloatImage(client.Context(), spreadsheetToken, sheetID, image, userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			if err := printJSON(result); err != nil {
				return err
			}
		} else {
			fmt.Printf("浮动图片添加成功！\n")
			fmt.Printf("  图片 ID: %s\n", result.FloatImageID)
			fmt.Printf("  范围: %s\n", result.Range)
		}

		return nil
	},
}

var sheetImageListCmd = &cobra.Command{
	Use:   "list <spreadsheet_token|url> <sheet_id>",
	Short: "列出浮动图片",
	Long:  "列出工作表中的所有浮动图片",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sheetID := args[1]
		output, _ := cmd.Flags().GetString("output")

		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}
		spreadsheetToken, userAccessToken := target.Token, target.UAT

		images, err := client.QueryFloatImages(client.Context(), spreadsheetToken, sheetID, userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			if err := printJSON(images); err != nil {
				return err
			}
		} else {
			if len(images) == 0 {
				fmt.Println("没有浮动图片")
				return nil
			}
			fmt.Printf("共 %d 个浮动图片:\n", len(images))
			for i, img := range images {
				fmt.Printf("  %d. ID: %s, 范围: %s, 尺寸: %.0fx%.0f\n",
					i+1, img.FloatImageID, img.Range, img.Width, img.Height)
			}
		}

		return nil
	},
}

var sheetImageDeleteCmd = &cobra.Command{
	Use:   "delete <spreadsheet_token|url> <sheet_id> <float_image_id>",
	Short: "删除浮动图片",
	Long:  "删除工作表中的浮动图片",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		sheetID := args[1]
		floatImageID := args[2]

		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}
		spreadsheetToken, userAccessToken := target.Token, target.UAT

		if err := client.DeleteFloatImage(client.Context(), spreadsheetToken, sheetID, floatImageID, userAccessToken); err != nil {
			return err
		}

		fmt.Printf("浮动图片删除成功！ID: %s\n", floatImageID)
		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetImageCmd)

	sheetImageCmd.AddCommand(sheetImageAddCmd)
	sheetImageCmd.AddCommand(sheetImageListCmd)
	sheetImageCmd.AddCommand(sheetImageDeleteCmd)

	sheetImageAddCmd.Flags().String("token", "", "图片 Token")
	sheetImageAddCmd.Flags().String("range", "", "图片左上角位置（如 A1:A1）")
	sheetImageAddCmd.Flags().Float64("width", 100, "图片宽度（像素，最小 20）")
	sheetImageAddCmd.Flags().Float64("height", 100, "图片高度（像素，最小 20）")
	sheetImageAddCmd.Flags().Float64("offset-x", 0, "水平偏移")
	sheetImageAddCmd.Flags().Float64("offset-y", 0, "垂直偏移")
	sheetImageAddCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	sheetImageAddCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
	mustMarkFlagRequired(sheetImageAddCmd, "token", "range")

	sheetImageListCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	sheetImageListCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")

	sheetImageDeleteCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
