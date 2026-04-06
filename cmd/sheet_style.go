package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

var sheetStyleCmd = &cobra.Command{
	Use:   "style <spreadsheet_token> <range>",
	Short: "设置单元格样式",
	Long: `设置指定范围的单元格样式。

有效的数字格式（--formatter）:
  @          - 文本格式
  #,##0      - 千位分隔符（整数）
  #,##0.00   - 千位分隔符 + 两位小数
  0%         - 百分比（整数）
  0.00%      - 百分比 + 两位小数
  ¥#,##0.00  - 人民币货币格式
  $#,##0.00  - 美元货币格式
  0.00E+00   - 科学计数法
  yyyy/MM/dd - 日期格式
  yyyy-MM-dd - 日期格式

注意: 简单小数格式如 "0.00" 无效，需使用 "#,##0.00"

示例:
  # 设置背景色
  feishu-cli sheet style shtcnxxxxxx "Sheet1!A1:C3" --bg-color "#FF0000"

  # 设置字体加粗
  feishu-cli sheet style shtcnxxxxxx "Sheet1!A1:C3" --bold

  # 设置对齐方式
  feishu-cli sheet style shtcnxxxxxx "Sheet1!A1:C3" --h-align CENTER --v-align MIDDLE

  # 设置数字格式
  feishu-cli sheet style shtcnxxxxxx "Sheet1!A1:C3" --formatter "#,##0.00"

  # 清除样式
  feishu-cli sheet style shtcnxxxxxx "Sheet1!A1:C3" --clean`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken := args[0]
		rangeStr := unescapeSheetRange(args[1])

		bold, _ := cmd.Flags().GetBool("bold")
		italic, _ := cmd.Flags().GetBool("italic")
		fontSize, _ := cmd.Flags().GetString("font-size")
		hAlign, _ := cmd.Flags().GetString("h-align")
		vAlign, _ := cmd.Flags().GetString("v-align")
		bgColor, _ := cmd.Flags().GetString("bg-color")
		foreColor, _ := cmd.Flags().GetString("fore-color")
		formatter, _ := cmd.Flags().GetString("formatter")
		clean, _ := cmd.Flags().GetBool("clean")

		style := &client.CellStyle{
			HAlign:    hAlign,
			VAlign:    vAlign,
			BackColor: bgColor,
			ForeColor: foreColor,
			Formatter: formatter,
			Clean:     clean,
		}

		if bold || italic || fontSize != "" {
			style.Font = &client.FontStyle{
				Bold:     bold,
				Italic:   italic,
				FontSize: fontSize,
			}
		}

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		err := client.SetCellStyle(client.Context(), spreadsheetToken, rangeStr, style, userAccessToken)
		if err != nil {
			return err
		}

		fmt.Printf("样式设置成功！范围: %s\n", rangeStr)
		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetStyleCmd)

	sheetStyleCmd.Flags().Bool("bold", false, "加粗")
	sheetStyleCmd.Flags().Bool("italic", false, "斜体")
	sheetStyleCmd.Flags().String("font-size", "", "字体大小")
	sheetStyleCmd.Flags().String("h-align", "", "水平对齐: LEFT, CENTER, RIGHT")
	sheetStyleCmd.Flags().String("v-align", "", "垂直对齐: TOP, MIDDLE, BOTTOM")
	sheetStyleCmd.Flags().String("bg-color", "", "背景颜色（如 #FF0000）")
	sheetStyleCmd.Flags().String("fore-color", "", "字体颜色（如 #0000FF）")
	sheetStyleCmd.Flags().String("formatter", "", "数字格式（如 yyyy/MM/dd）")
	sheetStyleCmd.Flags().Bool("clean", false, "清除样式")
	sheetStyleCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
