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
	Use:   "add <spreadsheet_token> <sheet_id>",
	Short: "添加浮动图片",
	Long: `在工作表中添加浮动图片。

示例:
  feishu-cli sheet image add shtcnxxxxxx 0b12 --token img_xxx --range "A1:A1" --width 200 --height 150`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken := args[0]
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

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

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
	Use:   "list <spreadsheet_token> <sheet_id>",
	Short: "列出浮动图片",
	Long:  "列出工作表中的所有浮动图片",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken := args[0]
		sheetID := args[1]
		output, _ := cmd.Flags().GetString("output")

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

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
	Use:   "delete <spreadsheet_token> <sheet_id> <float_image_id>",
	Short: "删除浮动图片",
	Long:  "删除工作表中的浮动图片",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken := args[0]
		sheetID := args[1]
		floatImageID := args[2]

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		err := client.DeleteFloatImage(client.Context(), spreadsheetToken, sheetID, floatImageID, userAccessToken)
		if err != nil {
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
