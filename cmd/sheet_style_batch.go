package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

var sheetBatchSetStyleCmd = &cobra.Command{
	Use:   "batch-set-style <spreadsheet_token>",
	Short: "批量设置单元格样式",
	Long: `批量为多个范围设置单元格样式。

--data 是 {ranges, style} 对象的 JSON 数组，每个 range 必须带 sheetId 前缀（如 0b1212!A1:C3）。
style 字段沿用飞书 V2 styles_batch_update 的原始结构（如 font / hAlign / vAlign / backColor / foreColor / formatter / clean）。

示例:
  feishu-cli sheet batch-set-style shtcnxxxxxx \
      --data '[{"ranges":["0b1212!A1:A2"],"style":{"font":{"bold":true},"backColor":"#FF0000"}}]'

  # 多个范围块
  feishu-cli sheet batch-set-style shtcnxxxxxx \
      --data '[{"ranges":["0b1212!A1:A2"],"style":{"font":{"bold":true}}},{"ranges":["0b1212!B1:B2"],"style":{"backColor":"#00FF00"}}]'`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken := args[0]
		dataStr, _ := cmd.Flags().GetString("data")

		if strings.TrimSpace(dataStr) == "" {
			return fmt.Errorf("--data 为必填项（{ranges, style} 对象的 JSON 数组）")
		}

		styles, err := parseSheetBatchStyleData(dataStr)
		if err != nil {
			return err
		}
		if len(styles) == 0 {
			return fmt.Errorf("--data 至少需要一个 {ranges, style} 对象")
		}

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		if err := client.SetCellStyleBatch(client.Context(), spreadsheetToken, styles, userAccessToken); err != nil {
			return err
		}
		fmt.Printf("批量样式设置成功！样式块数: %d\n", len(styles))
		return nil
	},
}

// parseSheetBatchStyleData 解析 --data 的 JSON 数组为 []map[string]any。
func parseSheetBatchStyleData(dataStr string) ([]map[string]any, error) {
	var styles []map[string]any
	if err := json.Unmarshal([]byte(dataStr), &styles); err != nil {
		return nil, fmt.Errorf("--data 必须是 {ranges, style} 对象的 JSON 数组: %w", err)
	}
	return styles, nil
}

func init() {
	sheetCmd.AddCommand(sheetBatchSetStyleCmd)

	sheetBatchSetStyleCmd.Flags().String("data", "", `{ranges, style} 对象的 JSON 数组（必填）`)
	sheetBatchSetStyleCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
	mustMarkFlagRequired(sheetBatchSetStyleCmd, "data")
}
