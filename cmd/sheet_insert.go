package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/spf13/cobra"
)

var sheetInsertCmd = &cobra.Command{
	Use:   "insert <spreadsheet_token|url> <sheet_id> <range>",
	Short: "插入数据（V3 API）",
	Long: `使用 V3 API 在指定范围的开始位置上方插入若干行并填充数据。

范围格式:
  <sheetId>!A1:B2  - 指定工作表的范围（也可用子表名作前缀）
  A1:B2            - 不带前缀时自动补上 <sheet_id>

数据格式（三维数组）:
  简单模式（--simple）:
    [["A1", "B1"], ["A2", "B2"]]

  富文本模式:
    [
      [  // 第一行
        [  // 第一列
          {"type": "text", "text": {"text": "Hello"}}
        ]
      ]
    ]

使用限制:
  - 单次写入不超过 5,000 个单元格
  - 每个单元格不超过 50,000 字符

示例:
  # 简单模式插入（数字按原始字面量写入，1000000 不会变成 1e+06）
  feishu-cli sheet insert shtcnxxxxxx 0b12 "0b12!A1:B2" --data '[["新行1", 1000000], ["新行2", "数据2"]]' --simple

  # 从文件读取富文本数据
  feishu-cli sheet insert shtcnxxxxxx 0b12 "0b12!A1:B2" --data-file data.json`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		sheetID := args[1]
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		simple, _ := cmd.Flags().GetBool("simple")

		raw, err := readSheetDataInput(cmd)
		if err != nil {
			return err
		}
		values, err := decodeV3CellValues(raw, simple)
		if err != nil {
			return err
		}

		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}
		rangeStr, err := target.qualifyRange(args[2], sheetID, "")
		if err != nil {
			return err
		}

		if err := client.InsertCellsV3(client.Context(), target.Token, sheetID, rangeStr, values, userIDType, target.UAT); err != nil {
			return err
		}

		fmt.Printf("插入成功！\n")
		fmt.Printf("  工作表: %s\n", sheetID)
		fmt.Printf("  插入范围: %s\n", rangeStr)
		fmt.Printf("  插入行数: %d\n", len(values))

		return nil
	},
}

// decodeV3CellValues 解析 insert / append-rich 的 --data：simple 模式为二维数组（数字保留原始字面量），
// 否则为 V3 三维元素数组。
func decodeV3CellValues(raw []byte, simple bool) ([][][]*client.CellElement, error) {
	if simple {
		simpleValues, err := client.DecodeSheetValues(raw)
		if err != nil {
			return nil, clierr.Usagef("解析数据失败（需要 JSON 二维数组）: %v", err)
		}
		return client.ConvertSimpleToV3Values(simpleValues), nil
	}
	var values [][][]*client.CellElement
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, clierr.Usagef("解析数据失败（需要 V3 三维数组格式；简单二维数组请加 --simple）: %v", err)
	}
	return values, nil
}

func init() {
	sheetCmd.AddCommand(sheetInsertCmd)

	sheetInsertCmd.Flags().StringP("data", "d", "", "要插入的数据")
	sheetInsertCmd.Flags().String("data-file", "", "数据文件路径")
	sheetInsertCmd.Flags().String("user-id-type", "", "用户 ID 类型: open_id, union_id, user_id")
	sheetInsertCmd.Flags().Bool("simple", false, "使用简单模式（二维数组自动转换）")
	sheetInsertCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
