package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

var sheetUpdateSheetCmd = &cobra.Command{
	Use:   "update-sheet <spreadsheet_token|url> <sheet_id>",
	Short: "修改工作表属性（改名/隐藏/移动/冻结）",
	Long: `修改子表属性：标题、位置、隐藏状态、冻结行列数。只修改显式传入的字段。

示例:
  # 改名
  feishu-cli sheet update-sheet shtcnxxxxxx 0b12 --title "2024 汇总"
  # 隐藏 / 取消隐藏
  feishu-cli sheet update-sheet shtcnxxxxxx 0b12 --hidden
  feishu-cli sheet update-sheet shtcnxxxxxx 0b12 --hidden=false
  # 移到第一个位置（0 起始）
  feishu-cli sheet update-sheet shtcnxxxxxx 0b12 --index 0
  # 冻结首行和首列（0 表示取消冻结）
  feishu-cli sheet update-sheet shtcnxxxxxx 0b12 --frozen-rows 1 --frozen-cols 1`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		props := &client.SheetPropertiesUpdate{SheetID: args[1]}
		var changes []string
		if cmd.Flags().Changed("title") {
			title, _ := cmd.Flags().GetString("title")
			if strings.TrimSpace(title) == "" {
				return clierr.Usagef("--title 不能为空")
			}
			props.Title = &title
			changes = append(changes, fmt.Sprintf("标题=%q", title))
		}
		if cmd.Flags().Changed("index") {
			index, _ := cmd.Flags().GetInt("index")
			if index < 0 {
				return clierr.Usagef("--index 从 0 开始，不能为负数")
			}
			props.Index = &index
			changes = append(changes, fmt.Sprintf("位置=%d", index))
		}
		if cmd.Flags().Changed("hidden") {
			hidden, _ := cmd.Flags().GetBool("hidden")
			props.Hidden = &hidden
			changes = append(changes, fmt.Sprintf("隐藏=%v", hidden))
		}
		if err := setFrozenProps(cmd, props, "frozen-rows", "frozen-cols", &changes); err != nil {
			return err
		}
		if len(changes) == 0 {
			return clierr.Usagef("至少需要指定一个待修改属性（--title / --index / --hidden / --frozen-rows / --frozen-cols）")
		}
		return runUpdateSheetProps(cmd, args[0], props, changes)
	},
}

var sheetFreezeCmd = &cobra.Command{
	Use:   "freeze <spreadsheet_token|url> <sheet_id>",
	Short: "冻结行列",
	Long: `冻结子表的前 N 行 / 前 M 列（0 表示取消冻结）。等价于 update-sheet --frozen-rows/--frozen-cols。

示例:
  feishu-cli sheet freeze shtcnxxxxxx 0b12 --rows 1
  feishu-cli sheet freeze shtcnxxxxxx 0b12 --rows 2 --cols 1
  feishu-cli sheet freeze shtcnxxxxxx 0b12 --rows 0 --cols 0   # 取消冻结`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		props := &client.SheetPropertiesUpdate{SheetID: args[1]}
		var changes []string
		if err := setFrozenProps(cmd, props, "rows", "cols", &changes); err != nil {
			return err
		}
		if len(changes) == 0 {
			return clierr.Usagef("至少需要指定 --rows 或 --cols")
		}
		return runUpdateSheetProps(cmd, args[0], props, changes)
	},
}

// setFrozenProps 读取冻结行列 flag（Changed 才写，允许显式 0 取消冻结）。
func setFrozenProps(cmd *cobra.Command, props *client.SheetPropertiesUpdate, rowsFlag, colsFlag string, changes *[]string) error {
	if cmd.Flags().Changed(rowsFlag) {
		n, _ := cmd.Flags().GetInt(rowsFlag)
		if n < 0 {
			return clierr.Usagef("--%s 不能为负数", rowsFlag)
		}
		props.FrozenRowCount = &n
		*changes = append(*changes, fmt.Sprintf("冻结行=%d", n))
	}
	if cmd.Flags().Changed(colsFlag) {
		n, _ := cmd.Flags().GetInt(colsFlag)
		if n < 0 {
			return clierr.Usagef("--%s 不能为负数", colsFlag)
		}
		props.FrozenColCount = &n
		*changes = append(*changes, fmt.Sprintf("冻结列=%d", n))
	}
	return nil
}

func runUpdateSheetProps(cmd *cobra.Command, rawToken string, props *client.SheetPropertiesUpdate, changes []string) error {
	target, err := newSheetTarget(cmd, rawToken)
	if err != nil {
		return err
	}
	if err := client.UpdateSheetProperties(client.Context(), target.Token, props, target.UAT); err != nil {
		return err
	}
	fmt.Printf("工作表 %s 已更新: %s\n", props.SheetID, strings.Join(changes, "，"))
	return nil
}

func init() {
	sheetCmd.AddCommand(sheetUpdateSheetCmd)
	sheetUpdateSheetCmd.Flags().String("title", "", "新标题")
	sheetUpdateSheetCmd.Flags().Int("index", 0, "新位置（0 起始）")
	sheetUpdateSheetCmd.Flags().Bool("hidden", false, "隐藏子表（--hidden=false 取消隐藏）")
	sheetUpdateSheetCmd.Flags().Int("frozen-rows", 0, "冻结前 N 行（0 取消）")
	sheetUpdateSheetCmd.Flags().Int("frozen-cols", 0, "冻结前 N 列（0 取消）")
	sheetUpdateSheetCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")

	sheetCmd.AddCommand(sheetFreezeCmd)
	sheetFreezeCmd.Flags().Int("rows", 0, "冻结前 N 行（0 取消冻结行）")
	sheetFreezeCmd.Flags().Int("cols", 0, "冻结前 N 列（0 取消冻结列）")
	sheetFreezeCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
