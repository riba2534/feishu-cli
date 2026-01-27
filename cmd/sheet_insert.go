package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

var sheetInsertCmd = &cobra.Command{
	Use:   "insert <spreadsheet_token> <sheet_id> <range>",
	Short: "插入数据（V3 API）",
	Long: `使用 V3 API 在指定范围的开始位置上方插入若干行并填充数据。

范围格式:
  SheetID!A1:B2    - 指定工作表的范围

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
  # 简单模式插入
  feishu-cli sheet insert shtcnxxxxxx 0b12 "0b12!A1:B2" --data '[["新行1", "数据1"], ["新行2", "数据2"]]' --simple

  # 从文件读取富文本数据
  feishu-cli sheet insert shtcnxxxxxx 0b12 "0b12!A1:B2" --data-file data.json`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken := args[0]
		sheetID := args[1]
		rangeStr := args[2]
		dataStr, _ := cmd.Flags().GetString("data")
		dataFile, _ := cmd.Flags().GetString("data-file")
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		simple, _ := cmd.Flags().GetBool("simple")

		// 处理 shell 转义
		rangeStr = unescapeSheetRange(rangeStr)

		// 获取数据
		var jsonData string
		if dataFile != "" {
			data, err := os.ReadFile(dataFile)
			if err != nil {
				return fmt.Errorf("读取数据文件失败: %w", err)
			}
			jsonData = string(data)
		} else if dataStr != "" {
			jsonData = dataStr
		} else {
			return fmt.Errorf("请通过 --data 或 --data-file 指定数据")
		}

		var values [][][]*client.CellElement

		if simple {
			// 简单模式：二维数组转换为三维数组
			var simpleValues [][]interface{}
			if err := json.Unmarshal([]byte(jsonData), &simpleValues); err != nil {
				return fmt.Errorf("解析数据失败（需要 JSON 二维数组）: %w", err)
			}
			values = client.ConvertSimpleToV3Values(simpleValues)
		} else {
			// 富文本模式
			if err := json.Unmarshal([]byte(jsonData), &values); err != nil {
				return fmt.Errorf("解析数据失败（需要 V3 三维数组格式）: %w", err)
			}
		}

		err := client.InsertCellsV3(client.Context(), spreadsheetToken, sheetID, rangeStr, values, userIDType)
		if err != nil {
			return err
		}

		fmt.Printf("插入成功！\n")
		fmt.Printf("  工作表: %s\n", sheetID)
		fmt.Printf("  插入范围: %s\n", rangeStr)
		fmt.Printf("  插入行数: %d\n", len(values))

		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetInsertCmd)

	sheetInsertCmd.Flags().StringP("data", "d", "", "要插入的数据")
	sheetInsertCmd.Flags().String("data-file", "", "数据文件路径")
	sheetInsertCmd.Flags().String("user-id-type", "", "用户 ID 类型: open_id, union_id, user_id")
	sheetInsertCmd.Flags().Bool("simple", false, "使用简单模式（二维数组自动转换）")
}
