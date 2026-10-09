package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/spf13/cobra"
)

var sheetBatchSetStyleCmd = &cobra.Command{
	Use:   "batch-set-style <spreadsheet_token|url>",
	Short: "批量设置单元格样式",
	Long: `批量为多个范围设置单元格样式。

--data 是 {ranges, style} 对象的 JSON 数组，每个 range 带 sheetId 或子表名前缀（如 0b1212!A1:C3、Sheet1!A1:C3）。
style 字段沿用飞书 V2 styles_batch_update 的原始结构（如 font / hAlign / vAlign / backColor / foreColor / formatter / clean）。

示例:
  feishu-cli sheet batch-set-style shtcnxxxxxx \
      --data '[{"ranges":["0b1212!A1:A2"],"style":{"font":{"bold":true},"backColor":"#FF0000"}}]'

  # 多个范围块
  feishu-cli sheet batch-set-style shtcnxxxxxx \
      --data '[{"ranges":["0b1212!A1:A2"],"style":{"font":{"bold":true}}},{"ranges":["0b1212!B1:B2"],"style":{"backColor":"#00FF00"}}]'`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
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

		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}
		if err := qualifyBatchStyleRanges(target, styles); err != nil {
			return err
		}

		if err := client.SetCellStyleBatch(client.Context(), target.Token, styles, target.UAT); err != nil {
			return err
		}
		fmt.Printf("批量样式设置成功！样式块数: %d\n", len(styles))
		return nil
	},
}

// qualifyBatchStyleRanges 把每个样式块 ranges 中的子表名前缀换算成 sheetId（"Sheet1!A1:C3" → "<sheetId>!A1:C3"），
// 无前缀的范围按 URL ?sheet= / 唯一子表补全。
func qualifyBatchStyleRanges(target *sheetTarget, styles []map[string]any) error {
	for i, block := range styles {
		rawRanges, ok := block["ranges"].([]any)
		if !ok {
			continue
		}
		for j, r := range rawRanges {
			rs, ok := r.(string)
			if !ok {
				continue
			}
			q, err := target.qualifyRange(rs, "", "")
			if err != nil {
				return fmt.Errorf("--data[%d].ranges[%d]: %w", i, j, err)
			}
			rawRanges[j] = q
		}
	}
	return nil
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
