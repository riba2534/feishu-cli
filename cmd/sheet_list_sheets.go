package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/spf13/cobra"
)

var sheetListSheetsCmd = &cobra.Command{
	Use:   "list-sheets <spreadsheet_token|url>",
	Short: "列出所有工作表",
	Long:  "列出电子表格中的所有工作表（含 sheetId、标题、行列数、冻结行列、是否隐藏、子表类型 resource_type）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		output, _ := cmd.Flags().GetString("output")
		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}

		sheets, err := client.QuerySheets(client.Context(), target.Token, target.UAT)
		if err != nil {
			return err
		}

		if output == "json" {
			if err := printJSON(sheets); err != nil {
				return err
			}
		} else {
			fmt.Printf("共 %d 个工作表:\n", len(sheets))
			for i, s := range sheets {
				hidden := ""
				if s.Hidden {
					hidden = " [隐藏]"
				}
				kind := ""
				if s.ResourceType != "" && s.ResourceType != "sheet" {
					kind = " [" + s.ResourceType + "]"
				}
				fmt.Printf("  %d. %s (ID: %s, 行: %d, 列: %d)%s%s\n",
					i+1, s.Title, s.SheetID, s.RowCount, s.ColCount, hidden, kind)
			}
		}

		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetListSheetsCmd)

	sheetListSheetsCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	sheetListSheetsCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
