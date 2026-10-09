package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

var sheetAddColsCmd = &cobra.Command{
	Use:   "add-cols <spreadsheet_token|url> <sheet_id>",
	Short: "添加列",
	Long:  "在工作表末尾添加新列",
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

		if err := client.AddDimension(client.Context(), target.Token, sheetID, "COLUMNS", count, target.UAT); err != nil {
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
