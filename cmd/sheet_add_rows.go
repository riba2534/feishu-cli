package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

var sheetAddRowsCmd = &cobra.Command{
	Use:   "add-rows <spreadsheet_token> <sheet_id>",
	Short: "添加行",
	Long:  "在工作表末尾添加新行",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken := args[0]
		sheetID := args[1]
		count, _ := cmd.Flags().GetInt("count")

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		err := client.AddDimension(client.Context(), spreadsheetToken, sheetID, "ROWS", count, userAccessToken)
		if err != nil {
			return err
		}

		fmt.Printf("成功添加 %d 行\n", count)
		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetAddRowsCmd)

	sheetAddRowsCmd.Flags().IntP("count", "n", 1, "添加的行数")
	sheetAddRowsCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
