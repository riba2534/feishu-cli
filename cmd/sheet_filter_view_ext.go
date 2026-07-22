package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

// filter-view get
var sheetFilterViewGetCmd = &cobra.Command{
	Use:   "get",
	Short: "获取筛选视图",
	Long: `根据 ID 获取单个筛选视图的 id、name、range。

示例:
  feishu-cli sheet filter-view get --token shtcnxxxxxx --sheet-id 0b1212 --filter-view-id pH9hbVcCXA`,
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken, err := readSheetSpreadsheetToken(cmd)
		if err != nil {
			return err
		}
		sheetID, _ := cmd.Flags().GetString("sheet-id")
		filterViewID, _ := cmd.Flags().GetString("filter-view-id")
		output, _ := cmd.Flags().GetString("output")

		if spreadsheetToken == "" || sheetID == "" || filterViewID == "" {
			return fmt.Errorf("--token、--sheet-id、--filter-view-id 均为必填项")
		}

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		fv, err := client.GetFilterView(client.Context(), spreadsheetToken, sheetID, filterViewID, userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(fv)
		}
		fmt.Printf("筛选视图信息:\n")
		fmt.Printf("  ID:    %s\n", fv.FilterViewID)
		fmt.Printf("  名称:  %s\n", fv.FilterViewName)
		fmt.Printf("  范围:  %s\n", fv.Range)
		return nil
	},
}

// filter-view update
var sheetFilterViewUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "更新筛选视图",
	Long: `更新筛选视图的名称或范围。--name / --range 至少需指定其一。

示例:
  feishu-cli sheet filter-view update --token shtcnxxxxxx --sheet-id 0b1212 --filter-view-id pH9hbVcCXA --name "新名字"
  feishu-cli sheet filter-view update --token shtcnxxxxxx --sheet-id 0b1212 --filter-view-id pH9hbVcCXA --range "0b1212!A1:H20"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken, err := readSheetSpreadsheetToken(cmd)
		if err != nil {
			return err
		}
		sheetID, _ := cmd.Flags().GetString("sheet-id")
		filterViewID, _ := cmd.Flags().GetString("filter-view-id")
		name, _ := cmd.Flags().GetString("name")
		rangeStr, _ := cmd.Flags().GetString("range")
		output, _ := cmd.Flags().GetString("output")

		if spreadsheetToken == "" || sheetID == "" || filterViewID == "" {
			return fmt.Errorf("--token、--sheet-id、--filter-view-id 均为必填项")
		}
		if name == "" && rangeStr == "" {
			return fmt.Errorf("--name 和 --range 至少需要指定一个")
		}

		rangeStr = unescapeSheetRange(rangeStr)

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		fv, err := client.UpdateFilterView(client.Context(), spreadsheetToken, sheetID, filterViewID, name, rangeStr, userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(fv)
		}
		fmt.Printf("筛选视图更新成功！\n")
		fmt.Printf("  ID:    %s\n", fv.FilterViewID)
		fmt.Printf("  名称:  %s\n", fv.FilterViewName)
		fmt.Printf("  范围:  %s\n", fv.Range)
		return nil
	},
}

func init() {
	sheetFilterViewCmd.AddCommand(sheetFilterViewGetCmd)
	sheetFilterViewCmd.AddCommand(sheetFilterViewUpdateCmd)

	// get
	sheetFilterViewGetCmd.Flags().String("token", "", "电子表格 token（必填；兼容旧名）")
	sheetFilterViewGetCmd.Flags().String("spreadsheet-token", "", "电子表格 token（兼容别名）")
	sheetFilterViewGetCmd.Flags().String("sheet-id", "", "工作表 ID（必填）")
	sheetFilterViewGetCmd.Flags().String("filter-view-id", "", "筛选视图 ID（必填）")
	sheetFilterViewGetCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	sheetFilterViewGetCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")

	// update
	sheetFilterViewUpdateCmd.Flags().String("token", "", "电子表格 token（必填；兼容旧名）")
	sheetFilterViewUpdateCmd.Flags().String("spreadsheet-token", "", "电子表格 token（兼容别名）")
	sheetFilterViewUpdateCmd.Flags().String("sheet-id", "", "工作表 ID（必填）")
	sheetFilterViewUpdateCmd.Flags().String("filter-view-id", "", "筛选视图 ID（必填）")
	sheetFilterViewUpdateCmd.Flags().String("name", "", "新名称（≤100 字符）")
	sheetFilterViewUpdateCmd.Flags().String("range", "", "新筛选范围（如 0b1212!A1:H20）")
	sheetFilterViewUpdateCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	sheetFilterViewUpdateCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
