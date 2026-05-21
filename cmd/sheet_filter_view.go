package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

// 筛选视图命令组
var sheetFilterViewCmd = &cobra.Command{
	Use:   "filter-view",
	Short: "筛选视图操作",
	Long:  "工作表筛选视图相关操作（创建、列出、删除）",
}

var sheetFilterViewCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建筛选视图",
	Long: `在工作表中创建筛选视图。

示例:
  feishu-cli sheet filter-view create --token shtcnxxxxxx --sheet-id 0b1212 --range "0b1212!A1:H14" --name "我的视图"
  feishu-cli sheet filter-view create --token shtcnxxxxxx --sheet-id 0b1212 --range "A1:H14"   # range 不带 sheetId 前缀时自动补全`,
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken, _ := cmd.Flags().GetString("token")
		sheetID, _ := cmd.Flags().GetString("sheet-id")
		rangeStr, _ := cmd.Flags().GetString("range")
		name, _ := cmd.Flags().GetString("name")
		filterViewID, _ := cmd.Flags().GetString("filter-view-id")
		output, _ := cmd.Flags().GetString("output")

		if spreadsheetToken == "" || sheetID == "" || rangeStr == "" {
			return fmt.Errorf("--token、--sheet-id、--range 均为必填项")
		}

		rangeStr = unescapeSheetRange(rangeStr)

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		fv, err := client.CreateFilterView(client.Context(), spreadsheetToken, sheetID, rangeStr, name, filterViewID, userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(fv)
		}
		fmt.Printf("筛选视图创建成功！\n")
		fmt.Printf("  ID:    %s\n", fv.FilterViewID)
		fmt.Printf("  名称:  %s\n", fv.FilterViewName)
		fmt.Printf("  范围:  %s\n", fv.Range)
		return nil
	},
}

var sheetFilterViewListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出筛选视图",
	Long:  "列出指定工作表的所有筛选视图",
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken, _ := cmd.Flags().GetString("token")
		sheetID, _ := cmd.Flags().GetString("sheet-id")
		output, _ := cmd.Flags().GetString("output")

		if spreadsheetToken == "" || sheetID == "" {
			return fmt.Errorf("--token、--sheet-id 均为必填项")
		}

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		items, err := client.ListFilterViews(client.Context(), spreadsheetToken, sheetID, userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(items)
		}
		if len(items) == 0 {
			fmt.Println("当前工作表没有筛选视图")
			return nil
		}
		fmt.Printf("共 %d 个筛选视图:\n", len(items))
		for i, fv := range items {
			fmt.Printf("%d. ID=%s  名称=%s  范围=%s\n", i+1, fv.FilterViewID, fv.FilterViewName, fv.Range)
		}
		return nil
	},
}

var sheetFilterViewDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "删除筛选视图",
	Long: `删除指定的筛选视图。

示例:
  feishu-cli sheet filter-view delete --token shtcnxxxxxx --sheet-id 0b1212 --filter-view-id pH9hbVcCXA`,
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken, _ := cmd.Flags().GetString("token")
		sheetID, _ := cmd.Flags().GetString("sheet-id")
		filterViewID, _ := cmd.Flags().GetString("filter-view-id")

		if spreadsheetToken == "" || sheetID == "" || filterViewID == "" {
			return fmt.Errorf("--token、--sheet-id、--filter-view-id 均为必填项")
		}

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		if err := client.DeleteFilterView(client.Context(), spreadsheetToken, sheetID, filterViewID, userAccessToken); err != nil {
			return err
		}
		fmt.Printf("筛选视图删除成功（ID=%s）\n", filterViewID)
		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetFilterViewCmd)

	sheetFilterViewCmd.AddCommand(sheetFilterViewCreateCmd)
	sheetFilterViewCmd.AddCommand(sheetFilterViewListCmd)
	sheetFilterViewCmd.AddCommand(sheetFilterViewDeleteCmd)

	// create
	sheetFilterViewCreateCmd.Flags().String("token", "", "电子表格 token（必填）")
	sheetFilterViewCreateCmd.Flags().String("sheet-id", "", "工作表 ID（必填）")
	sheetFilterViewCreateCmd.Flags().String("range", "", "筛选范围，例如 \"<sheetId>!A1:H14\"（必填）")
	sheetFilterViewCreateCmd.Flags().String("name", "", "筛选视图名称（≤100 字符，可选）")
	sheetFilterViewCreateCmd.Flags().String("filter-view-id", "", "自定义视图 ID（10 位字母数字，可选）")
	sheetFilterViewCreateCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	sheetFilterViewCreateCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")

	// list
	sheetFilterViewListCmd.Flags().String("token", "", "电子表格 token（必填）")
	sheetFilterViewListCmd.Flags().String("sheet-id", "", "工作表 ID（必填）")
	sheetFilterViewListCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	sheetFilterViewListCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")

	// delete
	sheetFilterViewDeleteCmd.Flags().String("token", "", "电子表格 token（必填）")
	sheetFilterViewDeleteCmd.Flags().String("sheet-id", "", "工作表 ID（必填）")
	sheetFilterViewDeleteCmd.Flags().String("filter-view-id", "", "筛选视图 ID（必填）")
	sheetFilterViewDeleteCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
