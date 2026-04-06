package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

var sheetInsertRowsCmd = &cobra.Command{
	Use:   "insert-rows <spreadsheet_token> <sheet_id>",
	Short: "插入行",
	Long:  "在指定位置插入新行",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken := args[0]
		sheetID := args[1]
		startIndex, _ := cmd.Flags().GetInt("start")
		endIndex, _ := cmd.Flags().GetInt("end")
		inheritStyle, _ := cmd.Flags().GetString("inherit-style")

		if endIndex == 0 {
			endIndex = startIndex + 1
		}

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		err := client.InsertDimension(client.Context(), spreadsheetToken, sheetID, "ROWS", startIndex, endIndex, inheritStyle, userAccessToken)
		if err != nil {
			return err
		}

		fmt.Printf("成功在位置 %d 插入 %d 行\n", startIndex, endIndex-startIndex)
		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetInsertRowsCmd)

	sheetInsertRowsCmd.Flags().Int("start", 0, "起始行号（从 0 开始）")
	sheetInsertRowsCmd.Flags().Int("end", 0, "结束行号（不包含）")
	sheetInsertRowsCmd.Flags().String("inherit-style", "", "继承样式: BEFORE, AFTER")
	sheetInsertRowsCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
	mustMarkFlagRequired(sheetInsertRowsCmd, "start")
}
