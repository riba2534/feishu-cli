package cmd

import (
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var sheetTableGetCmd = &cobra.Command{
	Use:   "table-get <spreadsheet_token> <sheet_id>",
	Short: "按列类型保真读取整表（table-put 的镜像，支持 round-trip）",
	Long: `把电子表格区域读回 DataFrame 形状的 JSON（columns + data + dtypes/formats），
与 table-put 输入完全对称：table-get 的输出可直接（或修改后）喂给 table-put 写回。

列类型自动推断（依据 V3 类型化单元格元素）:
  - 数字单元格          → float64（json 数字，保精度）
  - 日期单元格          → datetime64[ns]（值归一为 ISO yyyy-mm-dd）
  - 文本单元格          → string；全列值 ∈ {TRUE,FALSE} 时升级为 bool
  - 公式单元格          → 取计算结果值参与推断
  - 混合类型列          → object（逐格保留各自类型）
  - 链接/@人/@文档/图片  → 提取显示文本

输出形状:
  {"sheets":[{"name":"<sheet_id>","range":"...","columns":[...],
              "data":[[...]],"dtypes":{...},"formats":{...}}]}

参数:
  <spreadsheet_token>  电子表格 token
  <sheet_id>           子表 ID
  --range              读取区域（如 A1:D100；缺省读整张 used range，自动裁掉空行空列）
  --no-header          首行按数据处理，列名生成 col_1..col_n（默认首行为列名）

示例:
  feishu-cli sheet table-get shtcnxxx 0b12
  feishu-cli sheet table-get shtcnxxx 0b12 --range A1:D50
  # round-trip：读出 → 修改 → 写回
  feishu-cli sheet table-get shtcnxxx 0b12 > table.json
  feishu-cli sheet table-put shtcnxxx 0b12 --sheets-file table.json`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		rangeStr, _ := cmd.Flags().GetString("range")
		noHeader, _ := cmd.Flags().GetBool("no-header")
		token := resolveOptionalUserTokenWithFallback(cmd)
		result, err := client.ReadTable(client.Context(), args[0], args[1],
			unescapeSheetRange(rangeStr), noHeader, token)
		if err != nil {
			return err
		}
		return printJSON(result)
	},
}

func init() {
	sheetCmd.AddCommand(sheetTableGetCmd)
	sheetTableGetCmd.Flags().String("range", "", "读取区域（如 A1:D100，缺省读整张 used range）")
	sheetTableGetCmd.Flags().Bool("no-header", false, "首行按数据处理（默认首行为列名）")
	sheetTableGetCmd.Flags().String("user-access-token", "", "User Access Token")
}
