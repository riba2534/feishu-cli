package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

var sheetUnmergeCmd = &cobra.Command{
	Use:   "unmerge <spreadsheet_token> <range>",
	Short: "取消合并单元格",
	Long: `取消指定范围的单元格合并。

示例:
  feishu-cli sheet unmerge shtcnxxxxxx "Sheet1!A1:C3"`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken := args[0]
		rangeStr := unescapeSheetRange(args[1])

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		err := client.UnmergeCells(client.Context(), spreadsheetToken, rangeStr, userAccessToken)
		if err != nil {
			return err
		}

		fmt.Printf("取消合并成功！范围: %s\n", rangeStr)
		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetUnmergeCmd)

	sheetUnmergeCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
