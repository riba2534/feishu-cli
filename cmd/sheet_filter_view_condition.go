package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

// 筛选条件命令组（filter-view 的子资源）
var sheetFilterViewConditionCmd = &cobra.Command{
	Use:   "condition",
	Short: "筛选条件操作",
	Long:  "筛选视图的筛选条件相关操作（创建、获取、更新、删除、列出）",
}

// condition create
var sheetFilterViewConditionCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建筛选条件",
	Long: `在筛选视图上创建一个筛选条件。

condition-id 为列字母（如 E）。expected 为筛选参数 JSON 数组（如 '["6"]'）。

示例:
  feishu-cli sheet filter-view condition create --token shtcnxxxxxx --sheet-id 0b1212 \
      --filter-view-id pH9hbVcCXA --condition-id E --filter-type number --compare-type less --expected '["6"]'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSheetConditionWrite(cmd, true)
	},
}

// condition get
var sheetFilterViewConditionGetCmd = &cobra.Command{
	Use:   "get",
	Short: "获取筛选条件",
	Long: `根据列字母获取单个筛选条件。

示例:
  feishu-cli sheet filter-view condition get --token shtcnxxxxxx --sheet-id 0b1212 \
      --filter-view-id pH9hbVcCXA --condition-id E`,
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken, sheetID, filterViewID, conditionID, output, uat, err := readSheetConditionCommon(cmd, true)
		if err != nil {
			return err
		}

		c, err := client.GetFilterViewCondition(client.Context(), spreadsheetToken, sheetID, filterViewID, conditionID, uat)
		if err != nil {
			return err
		}
		if output == "json" {
			return printJSON(c)
		}
		printSheetCondition(c)
		return nil
	},
}

// condition update
var sheetFilterViewConditionUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "更新筛选条件",
	Long: `更新筛选视图上的某个筛选条件（按列字母定位）。

示例:
  feishu-cli sheet filter-view condition update --token shtcnxxxxxx --sheet-id 0b1212 \
      --filter-view-id pH9hbVcCXA --condition-id E --filter-type number --compare-type less --expected '["6"]'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSheetConditionWrite(cmd, false)
	},
}

// condition delete
var sheetFilterViewConditionDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "删除筛选条件",
	Long: `从筛选视图上删除某个筛选条件（按列字母定位）。

示例:
  feishu-cli sheet filter-view condition delete --token shtcnxxxxxx --sheet-id 0b1212 \
      --filter-view-id pH9hbVcCXA --condition-id E`,
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken, sheetID, filterViewID, conditionID, _, uat, err := readSheetConditionCommon(cmd, true)
		if err != nil {
			return err
		}
		if err := client.DeleteFilterViewCondition(client.Context(), spreadsheetToken, sheetID, filterViewID, conditionID, uat); err != nil {
			return err
		}
		fmt.Printf("筛选条件删除成功（列=%s）\n", conditionID)
		return nil
	},
}

// condition list
var sheetFilterViewConditionListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出筛选条件",
	Long: `列出筛选视图的所有筛选条件。

示例:
  feishu-cli sheet filter-view condition list --token shtcnxxxxxx --sheet-id 0b1212 --filter-view-id pH9hbVcCXA`,
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken, sheetID, filterViewID, _, output, uat, err := readSheetConditionCommon(cmd, false)
		if err != nil {
			return err
		}

		items, err := client.ListFilterViewConditions(client.Context(), spreadsheetToken, sheetID, filterViewID, uat)
		if err != nil {
			return err
		}
		if output == "json" {
			return printJSON(items)
		}
		if len(items) == 0 {
			fmt.Println("当前筛选视图没有筛选条件")
			return nil
		}
		fmt.Printf("共 %d 个筛选条件:\n", len(items))
		for i, c := range items {
			fmt.Printf("%d. 列=%s  类型=%s  比较=%s  参数=%v\n", i+1, c.ConditionID, c.FilterType, c.CompareType, c.Expected)
		}
		return nil
	},
}

// readSheetConditionCommon 读取 condition 命令的公共参数。requireConditionID 控制是否要求 --condition-id。
func readSheetConditionCommon(cmd *cobra.Command, requireConditionID bool) (token, sheetID, filterViewID, conditionID, output, uat string, err error) {
	token, err = readSheetSpreadsheetToken(cmd)
	if err != nil {
		return
	}
	sheetID, _ = cmd.Flags().GetString("sheet-id")
	filterViewID, _ = cmd.Flags().GetString("filter-view-id")
	conditionID, _ = cmd.Flags().GetString("condition-id")
	output, _ = cmd.Flags().GetString("output")

	if token == "" || sheetID == "" || filterViewID == "" {
		err = fmt.Errorf("--token、--sheet-id、--filter-view-id 均为必填项")
		return
	}
	if requireConditionID && conditionID == "" {
		err = fmt.Errorf("--condition-id 为必填项（列字母，如 E）")
		return
	}
	uat = resolveOptionalUserTokenWithFallback(cmd)
	return
}

// runSheetConditionWrite create/update 共用逻辑（两者 body 字段一致，仅端点 method 不同）。
func runSheetConditionWrite(cmd *cobra.Command, isCreate bool) error {
	spreadsheetToken, sheetID, filterViewID, conditionID, output, uat, err := readSheetConditionCommon(cmd, true)
	if err != nil {
		return err
	}
	filterType, _ := cmd.Flags().GetString("filter-type")
	compareType, _ := cmd.Flags().GetString("compare-type")
	expectedJSON, _ := cmd.Flags().GetString("expected")

	expected, err := parseSheetConditionExpected(expectedJSON)
	if err != nil {
		return err
	}

	var c *client.FilterViewConditionSummary
	if isCreate {
		c, err = client.CreateFilterViewCondition(client.Context(), spreadsheetToken, sheetID, filterViewID, conditionID, filterType, compareType, expected, uat)
	} else {
		c, err = client.UpdateFilterViewCondition(client.Context(), spreadsheetToken, sheetID, filterViewID, conditionID, filterType, compareType, expected, uat)
	}
	if err != nil {
		return err
	}

	if output == "json" {
		return printJSON(c)
	}
	if isCreate {
		fmt.Printf("筛选条件创建成功！\n")
	} else {
		fmt.Printf("筛选条件更新成功！\n")
	}
	printSheetCondition(c)
	return nil
}

// parseSheetConditionExpected 解析 expected 的 JSON 数组（空字符串返回 nil）。
func parseSheetConditionExpected(jsonStr string) ([]string, error) {
	if strings.TrimSpace(jsonStr) == "" {
		return nil, nil
	}
	var arr []string
	if err := json.Unmarshal([]byte(jsonStr), &arr); err != nil {
		return nil, fmt.Errorf("--expected 必须是字符串数组（如 '[\"6\"]'）: %w", err)
	}
	return arr, nil
}

func printSheetCondition(c *client.FilterViewConditionSummary) {
	fmt.Printf("  列(condition_id): %s\n", c.ConditionID)
	fmt.Printf("  筛选类型:         %s\n", c.FilterType)
	fmt.Printf("  比较类型:         %s\n", c.CompareType)
	fmt.Printf("  筛选参数:         %v\n", c.Expected)
}

// addSheetConditionCommonFlags 为 condition 子命令注册公共 flag。
func addSheetConditionCommonFlags(c *cobra.Command, withConditionID, withOutput bool) {
	c.Flags().String("token", "", "电子表格 token（必填；兼容旧名）")
	c.Flags().String("spreadsheet-token", "", "电子表格 token（官方 lark-cli 兼容别名）")
	c.Flags().String("sheet-id", "", "工作表 ID（必填）")
	c.Flags().String("filter-view-id", "", "筛选视图 ID（必填）")
	if withConditionID {
		c.Flags().String("condition-id", "", "筛选条件所在列字母（如 E）（必填）")
	}
	if withOutput {
		c.Flags().StringP("output", "o", "text", "输出格式: text, json")
	}
	c.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}

func init() {
	sheetFilterViewCmd.AddCommand(sheetFilterViewConditionCmd)

	sheetFilterViewConditionCmd.AddCommand(sheetFilterViewConditionCreateCmd)
	sheetFilterViewConditionCmd.AddCommand(sheetFilterViewConditionGetCmd)
	sheetFilterViewConditionCmd.AddCommand(sheetFilterViewConditionUpdateCmd)
	sheetFilterViewConditionCmd.AddCommand(sheetFilterViewConditionDeleteCmd)
	sheetFilterViewConditionCmd.AddCommand(sheetFilterViewConditionListCmd)

	// create
	addSheetConditionCommonFlags(sheetFilterViewConditionCreateCmd, true, true)
	sheetFilterViewConditionCreateCmd.Flags().String("filter-type", "", "筛选类型: hiddenValue, number, text, color")
	sheetFilterViewConditionCreateCmd.Flags().String("compare-type", "", "比较类型（如 less, beginsWith, between）")
	sheetFilterViewConditionCreateCmd.Flags().String("expected", "", `筛选参数 JSON 数组（如 '["6"]'）`)

	// get
	addSheetConditionCommonFlags(sheetFilterViewConditionGetCmd, true, true)

	// update
	addSheetConditionCommonFlags(sheetFilterViewConditionUpdateCmd, true, true)
	sheetFilterViewConditionUpdateCmd.Flags().String("filter-type", "", "筛选类型: hiddenValue, number, text, color")
	sheetFilterViewConditionUpdateCmd.Flags().String("compare-type", "", "比较类型")
	sheetFilterViewConditionUpdateCmd.Flags().String("expected", "", `筛选参数 JSON 数组（如 '["6"]'）`)

	// delete
	addSheetConditionCommonFlags(sheetFilterViewConditionDeleteCmd, true, false)

	// list
	addSheetConditionCommonFlags(sheetFilterViewConditionListCmd, false, true)
}
