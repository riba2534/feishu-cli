package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

var sheetMergeCmd = &cobra.Command{
	Use:   "merge <spreadsheet_token> <range>",
	Short: "合并单元格",
	Long: `合并指定范围的单元格。

合并类型:
  MERGE_ALL      - 全部合并
  MERGE_ROWS     - 按行合并
  MERGE_COLUMNS  - 按列合并

示例:
  feishu-cli sheet merge shtcnxxxxxx "Sheet1!A1:C3" --type MERGE_ALL`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken := args[0]
		rangeStr := unescapeSheetRange(args[1])
		mergeType, _ := cmd.Flags().GetString("type")

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		err := client.MergeCells(client.Context(), spreadsheetToken, rangeStr, mergeType, userAccessToken)
		if err != nil {
			return err
		}

		fmt.Printf("合并成功！范围: %s, 类型: %s\n", rangeStr, mergeType)
		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetMergeCmd)

	sheetMergeCmd.Flags().String("type", "MERGE_ALL", "合并类型: MERGE_ALL, MERGE_ROWS, MERGE_COLUMNS")
	sheetMergeCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
