package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

// 下拉菜单命令组
var sheetDropdownCmd = &cobra.Command{
	Use:   "dropdown",
	Short: "下拉菜单操作",
	Long:  "工作表下拉菜单（数据验证）相关操作",
}

var sheetDropdownSetCmd = &cobra.Command{
	Use:   "set",
	Short: "设置下拉菜单",
	Long: `在指定区域设置下拉菜单（list 类型数据验证）。

range 必须带 sheetId 前缀，例如 "0b1212!A1:A100"。
options 用逗号分隔多个选项，每项 ≤ 100 字符；如需选项内出现逗号请改用 --options-json '["a","b,c"]'。

示例:
  # 简单选项
  feishu-cli sheet dropdown set --token shtcnxxxxxx --range "0b1212!A1:A100" --options "待办,处理中,已完成"

  # 多选 + 高亮
  feishu-cli sheet dropdown set --token shtcnxxxxxx --range "0b1212!B1:B100" \
      --options "P0,P1,P2" --multiple --colors "#FF4D4F,#FAAD14,#52C41A"

  # 选项含逗号
  feishu-cli sheet dropdown set --token shtcnxxxxxx --range "0b1212!C1:C100" \
      --options-json '["a, b","c"]'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken, _ := cmd.Flags().GetString("token")
		rangeStr, _ := cmd.Flags().GetString("range")
		optionsCSV, _ := cmd.Flags().GetString("options")
		optionsJSON, _ := cmd.Flags().GetString("options-json")
		colorsCSV, _ := cmd.Flags().GetString("colors")
		multiple, _ := cmd.Flags().GetBool("multiple")

		if spreadsheetToken == "" || rangeStr == "" {
			return fmt.Errorf("--token、--range 均为必填项")
		}

		if strings.TrimSpace(optionsJSON) != "" && strings.TrimSpace(optionsCSV) != "" {
			return fmt.Errorf("--options 和 --options-json 不能同时使用，请选其一")
		}

		rangeStr = unescapeSheetRange(rangeStr)

		options, err := parseDropdownOptions(optionsCSV, optionsJSON)
		if err != nil {
			return err
		}
		if len(options) == 0 {
			return fmt.Errorf("--options 或 --options-json 至少需要一个非空选项")
		}

		var colors []string
		if strings.TrimSpace(colorsCSV) != "" {
			for _, c := range strings.Split(colorsCSV, ",") {
				c = strings.TrimSpace(c)
				if c != "" {
					colors = append(colors, c)
				}
			}
		}

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		if err := client.SetDropdown(client.Context(), spreadsheetToken, rangeStr, options, multiple, colors, userAccessToken); err != nil {
			return err
		}
		fmt.Printf("下拉菜单设置成功！范围: %s，选项数: %d\n", rangeStr, len(options))
		return nil
	},
}

// parseDropdownOptions 解析下拉选项：optionsJSON 优先，否则用 CSV 拆分。
func parseDropdownOptions(csv, jsonStr string) ([]string, error) {
	if strings.TrimSpace(jsonStr) != "" {
		var arr []string
		if err := json.Unmarshal([]byte(jsonStr), &arr); err != nil {
			return nil, fmt.Errorf("--options-json 必须是字符串数组（如 '[\"a\",\"b\"]'）: %w", err)
		}
		return arr, nil
	}
	var out []string
	for _, s := range strings.Split(csv, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out, nil
}

func init() {
	sheetCmd.AddCommand(sheetDropdownCmd)

	sheetDropdownCmd.AddCommand(sheetDropdownSetCmd)

	sheetDropdownSetCmd.Flags().String("token", "", "电子表格 token（必填）")
	sheetDropdownSetCmd.Flags().String("range", "", "单元格范围，必须带 sheetId 前缀（如 0b1212!A1:A100）（必填）")
	sheetDropdownSetCmd.Flags().String("options", "", "下拉选项，逗号分隔（与 --options-json 二选一）")
	sheetDropdownSetCmd.Flags().String("options-json", "", `下拉选项 JSON 数组，如 '["a","b,c"]'（选项含逗号时使用）`)
	sheetDropdownSetCmd.Flags().Bool("multiple", false, "启用多选（默认 false）")
	sheetDropdownSetCmd.Flags().String("colors", "", "选项颜色（RGB hex，逗号分隔；数量需与选项一致，传值时自动开启高亮）")
	sheetDropdownSetCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
