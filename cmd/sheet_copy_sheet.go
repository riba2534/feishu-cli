package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

var sheetCopySheetCmd = &cobra.Command{
	Use:   "copy-sheet <spreadsheet_token> <source_sheet_id>",
	Short: "复制工作表",
	Long:  "复制电子表格中的指定工作表",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken := args[0]
		sourceSheetID := args[1]
		newTitle, _ := cmd.Flags().GetString("title")
		output, _ := cmd.Flags().GetString("output")

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		info, err := client.CopySheet(client.Context(), spreadsheetToken, sourceSheetID, newTitle, userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			if err := printJSON(info); err != nil {
				return err
			}
		} else {
			fmt.Printf("复制成功！\n")
			fmt.Printf("  新工作表 ID: %s\n", info.SheetID)
			fmt.Printf("  标题: %s\n", info.Title)
		}

		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetCopySheetCmd)

	sheetCopySheetCmd.Flags().StringP("title", "t", "", "新工作表标题（可选）")
	sheetCopySheetCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	sheetCopySheetCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
