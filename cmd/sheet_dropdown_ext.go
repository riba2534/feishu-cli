package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

// dropdown get
var sheetDropdownGetCmd = &cobra.Command{
	Use:   "get",
	Short: "获取下拉菜单设置",
	Long: `获取指定区域的下拉菜单（数据验证）设置。
range 必须带 sheetId 前缀（如 0b1212!A2:A100）。

示例:
  feishu-cli sheet dropdown get --token shtcnxxxxxx --range "0b1212!A1:A100"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken, _ := cmd.Flags().GetString("token")
		rangeStr, _ := cmd.Flags().GetString("range")

		if spreadsheetToken == "" || rangeStr == "" {
			return fmt.Errorf("--token、--range 均为必填项")
		}
		rangeStr = unescapeSheetRange(rangeStr)

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		data, err := client.GetDropdown(client.Context(), spreadsheetToken, rangeStr, userAccessToken)
		if err != nil {
			return err
		}

		// 下拉菜单 get 仅支持 json 输出（--output 默认且只接受 json），直接渲染。
		return printJSON(data)
	},
}

// dropdown update
var sheetDropdownUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "更新下拉菜单设置",
	Long: `更新下拉菜单（list 类型数据验证）。

ranges 可指定多个范围（每个需带 sheetId 前缀）；选项用 --options 逗号分隔，
或 --options-json 传字符串数组（选项含逗号时使用）。--colors 长度需与选项一致。

示例:
  feishu-cli sheet dropdown update --token shtcnxxxxxx --sheet-id 0b1212 \
      --ranges "0b1212!A1:A100" --options "待办,处理中,已完成"

  # 多范围 + 多选 + 高亮
  feishu-cli sheet dropdown update --token shtcnxxxxxx --sheet-id 0b1212 \
      --ranges "0b1212!A1:A100,0b1212!B1:B100" --options "P0,P1,P2" --multiple \
      --colors "#FF4D4F,#FAAD14,#52C41A"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken, _ := cmd.Flags().GetString("token")
		sheetID, _ := cmd.Flags().GetString("sheet-id")
		rangesStr, _ := cmd.Flags().GetString("ranges")
		optionsCSV, _ := cmd.Flags().GetString("options")
		optionsJSON, _ := cmd.Flags().GetString("options-json")
		colorsCSV, _ := cmd.Flags().GetString("colors")
		multiple, _ := cmd.Flags().GetBool("multiple")
		highlight, _ := cmd.Flags().GetBool("highlight")

		if spreadsheetToken == "" || sheetID == "" || rangesStr == "" {
			return fmt.Errorf("--token、--sheet-id、--ranges 均为必填项")
		}
		if strings.TrimSpace(optionsJSON) != "" && strings.TrimSpace(optionsCSV) != "" {
			return fmt.Errorf("--options 和 --options-json 不能同时使用，请选其一")
		}

		ranges := splitSheetCSV(unescapeSheetRange(rangesStr))
		if len(ranges) == 0 {
			return fmt.Errorf("--ranges 至少需要一个范围")
		}

		options, err := parseDropdownOptions(optionsCSV, optionsJSON)
		if err != nil {
			return err
		}
		if len(options) == 0 {
			return fmt.Errorf("--options 或 --options-json 至少需要一个非空选项")
		}

		var colors []string
		if strings.TrimSpace(colorsCSV) != "" {
			colors = splitSheetCSV(colorsCSV)
		}

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		if err := client.UpdateDropdown(client.Context(), spreadsheetToken, sheetID, ranges, options, multiple, colors, highlight, userAccessToken); err != nil {
			return err
		}
		fmt.Printf("下拉菜单更新成功！范围数: %d，选项数: %d\n", len(ranges), len(options))
		return nil
	},
}

// dropdown delete
var sheetDropdownDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "删除下拉菜单",
	Long: `删除指定范围的下拉菜单（数据验证）。
ranges 每个需带 sheetId 前缀，最多 100 个，逗号分隔。

示例:
  feishu-cli sheet dropdown delete --token shtcnxxxxxx --ranges "0b1212!A1:A100"
  feishu-cli sheet dropdown delete --token shtcnxxxxxx --ranges "0b1212!A1:A100,0b1212!B1:B100"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken, _ := cmd.Flags().GetString("token")
		rangesStr, _ := cmd.Flags().GetString("ranges")

		if spreadsheetToken == "" || rangesStr == "" {
			return fmt.Errorf("--token、--ranges 均为必填项")
		}

		ranges := splitSheetCSV(unescapeSheetRange(rangesStr))
		if len(ranges) == 0 {
			return fmt.Errorf("--ranges 至少需要一个范围")
		}

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		if err := client.DeleteDropdown(client.Context(), spreadsheetToken, ranges, userAccessToken); err != nil {
			return err
		}
		fmt.Printf("下拉菜单删除成功！范围数: %d\n", len(ranges))
		return nil
	},
}

// splitSheetCSV 按逗号拆分并去空白、去空项。
func splitSheetCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func init() {
	sheetDropdownCmd.AddCommand(sheetDropdownGetCmd)
	sheetDropdownCmd.AddCommand(sheetDropdownUpdateCmd)
	sheetDropdownCmd.AddCommand(sheetDropdownDeleteCmd)

	// get
	sheetDropdownGetCmd.Flags().String("token", "", "电子表格 token（必填）")
	sheetDropdownGetCmd.Flags().String("range", "", "单元格范围，必须带 sheetId 前缀（如 0b1212!A1:A100）（必填）")
	sheetDropdownGetCmd.Flags().StringP("output", "o", "json", "输出格式: json（默认）")
	sheetDropdownGetCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")

	// update
	sheetDropdownUpdateCmd.Flags().String("token", "", "电子表格 token（必填）")
	sheetDropdownUpdateCmd.Flags().String("sheet-id", "", "工作表 ID（必填）")
	sheetDropdownUpdateCmd.Flags().String("ranges", "", "范围，逗号分隔（每个需带 sheetId 前缀）（必填）")
	sheetDropdownUpdateCmd.Flags().String("options", "", "下拉选项，逗号分隔（与 --options-json 二选一）")
	sheetDropdownUpdateCmd.Flags().String("options-json", "", `下拉选项 JSON 数组，如 '["a","b,c"]'（选项含逗号时使用）`)
	sheetDropdownUpdateCmd.Flags().Bool("multiple", false, "启用多选（默认 false）")
	sheetDropdownUpdateCmd.Flags().Bool("highlight", false, "选项上色高亮（传 --colors 时自动开启）")
	sheetDropdownUpdateCmd.Flags().String("colors", "", "选项颜色（RGB hex，逗号分隔；数量需与选项一致）")
	sheetDropdownUpdateCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")

	// delete
	sheetDropdownDeleteCmd.Flags().String("token", "", "电子表格 token（必填）")
	sheetDropdownDeleteCmd.Flags().String("ranges", "", "范围，逗号分隔（每个需带 sheetId 前缀，最多 100 个）（必填）")
	sheetDropdownDeleteCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
