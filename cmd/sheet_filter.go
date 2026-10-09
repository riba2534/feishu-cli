package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/spf13/cobra"
)

// 筛选命令组
var sheetFilterCmd = &cobra.Command{
	Use:   "filter",
	Short: "筛选操作",
	Long:  "工作表筛选相关操作",
}

var sheetFilterCreateCmd = &cobra.Command{
	Use:   "create <spreadsheet_token|url> <sheet_id> <range>",
	Short: "创建筛选",
	Long: `在工作表中创建筛选（每个子表只有一个筛选，与 filter-view 筛选视图不同）。

接口要求同时提供筛选范围、筛选列与条件（只传范围会返回 99992402 col/condition is required）：
  --col           设置条件的列字母（须在范围内，如 B）
  --filter-type   multiValue | hiddenValue | number | text | color（原样透传服务端；multiValue 为多值筛选，
                  --expected 传值列表，如 '["待办","处理中"]'）
  --compare-type  比较类型（如 less、beginsWith、between；hiddenValue 不需要）
  --expected      筛选参数 JSON 字符串数组（如 '["6"]'）

示例:
  # B 列数值小于 6
  feishu-cli sheet filter create shtcnxxxxxx 0b12 "A1:C10" --col B --filter-type number --compare-type less --expected '["6"]'
  # 隐藏 C 列中值为 "已完成" 的行
  feishu-cli sheet filter create shtcnxxxxxx 0b12 "A1:C10" --col C --filter-type hiddenValue --expected '["已完成"]'`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		sheetID := args[1]
		rangeStr := unescapeSheetRange(args[2])
		cond, err := readSheetFilterCondition(cmd)
		if err != nil {
			return err
		}

		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}
		spreadsheetToken, userAccessToken := target.Token, target.UAT

		if rangeStr, err = target.qualifyRange(rangeStr, sheetID, ""); err != nil {
			return err
		}
		if err := client.CreateFilter(client.Context(), spreadsheetToken, sheetID, rangeStr, cond, userAccessToken); err != nil {
			return err
		}

		fmt.Printf("筛选创建成功！范围: %s，条件列: %s\n", rangeStr, cond.Col)
		return nil
	},
}

var sheetFilterUpdateCmd = &cobra.Command{
	Use:   "update <spreadsheet_token|url> <sheet_id>",
	Short: "更新筛选条件",
	Long: `更新已有筛选中某一列的条件（参数同 filter create）。

示例:
  feishu-cli sheet filter update shtcnxxxxxx 0b12 --col B --filter-type number --compare-type greater --expected '["10"]'`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sheetID := args[1]
		cond, err := readSheetFilterCondition(cmd)
		if err != nil {
			return err
		}
		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}
		if err := client.UpdateFilter(client.Context(), target.Token, sheetID, cond, target.UAT); err != nil {
			return err
		}
		fmt.Printf("筛选条件更新成功！条件列: %s\n", cond.Col)
		return nil
	},
}

// readSheetFilterCondition 读取 --col / --filter-type / --compare-type / --expected。
func readSheetFilterCondition(cmd *cobra.Command) (*client.SheetFilterCondition, error) {
	col := strings.ToUpper(strings.TrimSpace(flagString(cmd, "col")))
	filterType := strings.TrimSpace(flagString(cmd, "filter-type"))
	if col == "" || filterType == "" {
		return nil, clierr.Usagef("筛选需要 --col（列字母，如 B）与 --filter-type（multiValue/hiddenValue/number/text/color）")
	}
	if dr, err := client.ParseDimRange(col); err != nil || dr.Major != "COLUMNS" || dr.Start != dr.End {
		return nil, clierr.Usagef("--col 必须是单个列字母（如 B），得到 %q", col)
	}
	expected, err := parseSheetConditionExpected(flagString(cmd, "expected"))
	if err != nil {
		return nil, clierr.Usage(err)
	}
	return &client.SheetFilterCondition{
		Col:         col,
		FilterType:  filterType,
		CompareType: strings.TrimSpace(flagString(cmd, "compare-type")),
		Expected:    expected,
	}, nil
}

func addSheetFilterConditionFlags(c *cobra.Command) {
	c.Flags().String("col", "", "设置条件的列字母（如 B）（必填）")
	c.Flags().String("filter-type", "", "筛选类型: multiValue, hiddenValue, number, text, color（必填）")
	c.Flags().String("compare-type", "", "比较类型（如 less, beginsWith, between）")
	c.Flags().String("expected", "", `筛选参数 JSON 字符串数组（如 '["6"]'）`)
}

var sheetFilterGetCmd = &cobra.Command{
	Use:   "get <spreadsheet_token|url> <sheet_id>",
	Short: "获取筛选信息",
	Long:  "获取工作表的筛选信息",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sheetID := args[1]
		output, _ := cmd.Flags().GetString("output")

		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}
		spreadsheetToken, userAccessToken := target.Token, target.UAT

		info, err := client.GetFilter(client.Context(), spreadsheetToken, sheetID, userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			if err := printJSON(info); err != nil {
				return err
			}
		} else {
			fmt.Printf("筛选信息:\n")
			fmt.Printf("  范围: %s\n", info.Range)
			if len(info.FilteredRows) > 0 {
				fmt.Printf("  隐藏行: %v\n", info.FilteredRows)
			}
		}

		return nil
	},
}

var sheetFilterDeleteCmd = &cobra.Command{
	Use:   "delete <spreadsheet_token|url> <sheet_id>",
	Short: "删除筛选",
	Long:  "删除工作表的筛选",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sheetID := args[1]

		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}
		spreadsheetToken, userAccessToken := target.Token, target.UAT

		if err := client.DeleteFilter(client.Context(), spreadsheetToken, sheetID, userAccessToken); err != nil {
			return err
		}

		fmt.Println("筛选删除成功！")
		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetFilterCmd)

	sheetFilterCmd.AddCommand(sheetFilterCreateCmd)
	sheetFilterCmd.AddCommand(sheetFilterUpdateCmd)
	addSheetFilterConditionFlags(sheetFilterCreateCmd)
	addSheetFilterConditionFlags(sheetFilterUpdateCmd)
	sheetFilterUpdateCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
	sheetFilterCmd.AddCommand(sheetFilterGetCmd)
	sheetFilterCmd.AddCommand(sheetFilterDeleteCmd)

	sheetFilterGetCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")

	sheetFilterCreateCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
	sheetFilterGetCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
	sheetFilterDeleteCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
