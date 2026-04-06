package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

var sheetDeleteSheetCmd = &cobra.Command{
	Use:   "delete-sheet <spreadsheet_token> <sheet_id>",
	Short: "删除工作表",
	Long:  "删除电子表格中的指定工作表",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken := args[0]
		sheetID := args[1]

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		err := client.DeleteSheet(client.Context(), spreadsheetToken, sheetID, userAccessToken)
		if err != nil {
			return err
		}

		fmt.Printf("删除成功！工作表 ID: %s\n", sheetID)
		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetDeleteSheetCmd)

	sheetDeleteSheetCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
