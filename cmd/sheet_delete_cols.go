package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

var sheetDeleteColsCmd = &cobra.Command{
	Use:   "delete-cols <spreadsheet_token> <sheet_id>",
	Short: "删除列",
	Long:  "删除指定范围的列",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken := args[0]
		sheetID := args[1]
		startIndex, _ := cmd.Flags().GetInt("start")
		endIndex, _ := cmd.Flags().GetInt("end")

		if endIndex == 0 {
			endIndex = startIndex + 1
		}

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		err := client.DeleteDimension(client.Context(), spreadsheetToken, sheetID, "COLUMNS", startIndex, endIndex, userAccessToken)
		if err != nil {
			return err
		}

		fmt.Printf("成功删除第 %d 到 %d 列\n", startIndex+1, endIndex)
		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetDeleteColsCmd)

	sheetDeleteColsCmd.Flags().Int("start", 0, "起始列号（从 0 开始，A=0）")
	sheetDeleteColsCmd.Flags().Int("end", 0, "结束列号（不包含）")
	sheetDeleteColsCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
	mustMarkFlagRequired(sheetDeleteColsCmd, "start")
}
