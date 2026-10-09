package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

var sheetAddRowsCmd = &cobra.Command{
	Use:   "add-rows <spreadsheet_token|url> <sheet_id>",
	Short: "添加行",
	Long:  "在工作表末尾添加新行",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sheetID := args[1]
		count, _ := cmd.Flags().GetInt("count")
		if count < 1 {
			return clierr.Usagef("--count 必须 ≥ 1，得到 %d", count)
		}

		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}

		if err := client.AddDimension(client.Context(), target.Token, sheetID, "ROWS", count, target.UAT); err != nil {
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
