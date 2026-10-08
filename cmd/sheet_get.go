package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

var sheetGetCmd = &cobra.Command{
	Use:   "get <spreadsheet_token|url>",
	Short: "获取电子表格信息",
	Long:  "获取电子表格的基本信息",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		output, _ := cmd.Flags().GetString("output")
		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}

		info, err := client.GetSpreadsheet(client.Context(), target.Token, target.UAT)
		if err != nil {
			return err
		}

		if output == "json" {
			if err := printJSON(info); err != nil {
				return err
			}
		} else {
			fmt.Printf("电子表格信息:\n")
			fmt.Printf("  Token: %s\n", info.SpreadsheetToken)
			fmt.Printf("  标题: %s\n", info.Title)
			if info.URL != "" {
				fmt.Printf("  URL: %s\n", info.URL)
			}
			if info.OwnerID != "" {
				fmt.Printf("  所有者: %s\n", info.OwnerID)
			}
		}

		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetGetCmd)

	sheetGetCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	sheetGetCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
