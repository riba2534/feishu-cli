package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

var sheetReplaceCmd = &cobra.Command{
	Use:   "replace <spreadsheet_token> <sheet_id> <find> <replacement>",
	Short: "替换单元格内容",
	Long: `查找并替换工作表中的内容。

示例:
  # 普通替换
  feishu-cli sheet replace shtcnxxxxxx 0b12 "旧值" "新值"

  # 在指定范围内替换
  feishu-cli sheet replace shtcnxxxxxx 0b12 "旧值" "新值" --range "A1:C10"`,
	Args: cobra.ExactArgs(4),
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken := args[0]
		sheetID := args[1]
		findStr := args[2]
		replacement := args[3]
		rangeStr, _ := cmd.Flags().GetString("range")
		matchCase, _ := cmd.Flags().GetBool("match-case")
		matchEntireCell, _ := cmd.Flags().GetBool("match-entire-cell")
		output, _ := cmd.Flags().GetString("output")

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		result, err := client.ReplaceCells(client.Context(), spreadsheetToken, sheetID, findStr, replacement, matchCase, matchEntireCell, rangeStr, userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			if err := printJSON(result); err != nil {
				return err
			}
		} else {
			fmt.Printf("替换完成:\n")
			fmt.Printf("  替换单元格: %d 个\n", len(result.MatchedCells))
			fmt.Printf("  影响行数: %d\n", result.RowsCount)
			if len(result.MatchedCells) > 0 {
				fmt.Printf("  单元格列表: %v\n", result.MatchedCells)
			}
		}

		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetReplaceCmd)

	sheetReplaceCmd.Flags().String("range", "", "替换范围（如 A1:C10）")
	sheetReplaceCmd.Flags().Bool("match-case", false, "区分大小写")
	sheetReplaceCmd.Flags().Bool("match-entire-cell", false, "完全匹配单元格")
	sheetReplaceCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	sheetReplaceCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
