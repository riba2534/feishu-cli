package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

var sheetAddColsCmd = &cobra.Command{
	Use:   "add-cols <spreadsheet_token> <sheet_id>",
	Short: "添加列",
	Long:  "在工作表末尾添加新列",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken := args[0]
		sheetID := args[1]
		count, _ := cmd.Flags().GetInt("count")

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		err := client.AddDimension(client.Context(), spreadsheetToken, sheetID, "COLUMNS", count, userAccessToken)
		if err != nil {
			return err
		}

		fmt.Printf("成功添加 %d 列\n", count)
		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetAddColsCmd)

	sheetAddColsCmd.Flags().IntP("count", "n", 1, "添加的列数")
	sheetAddColsCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
