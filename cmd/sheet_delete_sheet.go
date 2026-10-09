package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/spf13/cobra"
)

var sheetDeleteSheetCmd = &cobra.Command{
	Use:   "delete-sheet <spreadsheet_token|url> <sheet_id>",
	Short: "删除工作表（不可撤销）",
	Long: `删除电子表格中的指定工作表（子表）及其全部数据。删除不可撤销，建议先用 --dry-run 确认。

示例:
  feishu-cli sheet delete-sheet shtcnxxxxxx 0b12 --dry-run
  feishu-cli sheet delete-sheet shtcnxxxxxx 0b12`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sheetID := args[1]

		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			token, _, _, err := parseSpreadsheetArg(args[0], "")
			if err != nil {
				return err
			}
			return printDryRunPlan(cmd, fmt.Sprintf("删除工作表 %s（不可撤销）", sheetID), map[string]any{
				"spreadsheet_token": token,
				"sheet_id":          sheetID,
				"irreversible":      true,
			}, []dryRunStep{{
				Method: "POST",
				URL:    fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/sheets_batch_update", token),
				Body:   map[string]any{"requests": []any{map[string]any{"deleteSheet": map[string]any{"sheetId": sheetID}}}},
			}})
		}

		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}
		if err := client.DeleteSheet(client.Context(), target.Token, sheetID, target.UAT); err != nil {
			return err
		}

		fmt.Printf("删除成功！工作表 ID: %s\n", sheetID)
		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetDeleteSheetCmd)

	sheetDeleteSheetCmd.Flags().Bool("dry-run", false, "只打印将要发送的删除请求，不执行")
	sheetDeleteSheetCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
