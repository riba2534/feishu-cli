package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

// sheet_dimension_cmds.go —— 新增的行列结构命令：insert-cols / update-dimension（隐藏、行高列宽）/
// move-dimension（v3 move_dimension）。索引口径换算见 sheet_dimension.go。

var sheetInsertColsCmd = &cobra.Command{
	Use:   "insert-cols <spreadsheet_token|url> <sheet_id>",
	Short: "插入列",
	Long: `在指定位置插入空白列，右侧原有列整体右移。

位置写法（二选一）:
  --range "C:D"            插入后新列所在位置（列字母，两端包含）：在原 C 列之前插入 2 列
  --start 2 --end 4        0 起始（A=0）、--end 不包含，与 insert-rows 一致

示例:
  feishu-cli sheet insert-cols shtcnxxxxxx 0b12 --range "C:D"
  feishu-cli sheet insert-cols shtcnxxxxxx 0b12 --range "B" --inherit-style BEFORE`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSheetInsertDimension(cmd, args, "COLUMNS")
	},
}

var sheetUpdateDimensionCmd = &cobra.Command{
	Use:   "update-dimension <spreadsheet_token|url> <sheet_id>",
	Short: "隐藏/显示行列、设置行高列宽",
	Long: `更新一段整行或整列的属性：隐藏 / 取消隐藏、行高 / 列宽（像素）。

--range 写法（1 起始、两端包含，与官方一致）:
  "3:5" / "3"     第 3 到 5 行 / 第 3 行
  "B:D" / "B"     B 到 D 列 / B 列

示例:
  # 隐藏第 3-5 行
  feishu-cli sheet update-dimension shtcnxxxxxx 0b12 --range "3:5" --hidden
  # 取消隐藏 B-D 列
  feishu-cli sheet update-dimension shtcnxxxxxx 0b12 --range "B:D" --hidden=false
  # 第 1 行行高设为 40 像素
  feishu-cli sheet update-dimension shtcnxxxxxx 0b12 --range "1" --size 40`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sheetID := args[1]
		raw, _ := cmd.Flags().GetString("range")
		dr, err := parseDimRangeFlag(raw, sheetID, "")
		if err != nil {
			return err
		}
		var visible *bool
		if cmd.Flags().Changed("hidden") {
			hidden, _ := cmd.Flags().GetBool("hidden")
			v := !hidden
			visible = &v
		}
		var size *int
		if cmd.Flags().Changed("size") {
			n, _ := cmd.Flags().GetInt("size")
			if n < 0 {
				return clierr.Usagef("--size 不能为负数，得到 %d", n)
			}
			size = &n
		}
		if visible == nil && size == nil {
			return clierr.Usagef("至少需要指定 --hidden 或 --size")
		}

		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}
		if err := client.UpdateDimension(client.Context(), target.Token, sheetID, dr.Major, dr.Start, dr.End, visible, size, target.UAT); err != nil {
			return err
		}
		var changes []string
		if visible != nil {
			if *visible {
				changes = append(changes, "取消隐藏")
			} else {
				changes = append(changes, "隐藏")
			}
		}
		if size != nil {
			if dr.Major == "COLUMNS" {
				changes = append(changes, fmt.Sprintf("列宽 %d 像素", *size))
			} else {
				changes = append(changes, fmt.Sprintf("行高 %d 像素", *size))
			}
		}
		fmt.Printf("已更新%s: %s\n", describeDimRange(dr), strings.Join(changes, "，"))
		return nil
	},
}

var sheetMoveDimensionCmd = &cobra.Command{
	Use:   "move-dimension <spreadsheet_token|url> <sheet_id>",
	Short: "移动行或列",
	Long: `把一段连续的行或列移动到新位置（v3 move_dimension），其他行列依次顺移。

参数（A1 写法，1 起始，与官方 +dim-move 一致）:
  --range    要移动的区间："3:5"（行）或 "B:D"（列）
  --target   目标位置：移动到「原第 N 行 / 原 X 列」之前，行号（如 10）或列字母（如 F），维度须与 --range 一致

实测语义：1..6 行执行 --range "2:3" --target 5 后顺序为 1,4,2,3,5,6；--range "5" --target 2 后为 1,5,2,3,4,6。

示例:
  # 把第 3-5 行移到原第 10 行之前
  feishu-cli sheet move-dimension shtcnxxxxxx 0b12 --range "3:5" --target 10
  # 把 B 列移到原 E 列之前
  feishu-cli sheet move-dimension shtcnxxxxxx 0b12 --range "B" --target E`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sheetID := args[1]
		raw, _ := cmd.Flags().GetString("range")
		dr, err := parseDimRangeFlag(raw, sheetID, "")
		if err != nil {
			return err
		}
		targetRaw, _ := cmd.Flags().GetString("target")
		td, err := client.ParseDimRange(strings.TrimSpace(targetRaw))
		if err != nil || td.Start != td.End {
			return clierr.Usagef("--target 应为单个行号（如 10）或列字母（如 F），得到 %q", targetRaw)
		}
		if td.Major != dr.Major {
			return clierr.Usagef("--target %q 与 --range %q 的维度不一致（行号对行、列字母对列）", targetRaw, raw)
		}

		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}
		// v3 move_dimension 为 0 起始、两端包含
		if err := client.MoveDimension(client.Context(), target.Token, sheetID, dr.Major, dr.Start-1, dr.End-1, td.Start-1, target.UAT); err != nil {
			return err
		}
		if dr.Major == "COLUMNS" {
			fmt.Printf("已将%s移动到原 %s 列之前\n", describeDimRange(dr), td.A1()[:strings.Index(td.A1(), ":")])
		} else {
			fmt.Printf("已将%s移动到原第 %d 行之前\n", describeDimRange(dr), td.Start)
		}
		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetInsertColsCmd)
	sheetInsertColsCmd.Flags().String("range", "", "插入后新列所在位置（如 \"C:D\"，1 起始、两端包含）")
	sheetInsertColsCmd.Flags().Int("start", 0, "起始列号（从 0 开始，A=0）")
	sheetInsertColsCmd.Flags().Int("end", 0, "结束列号（不包含；缺省只插入 1 列）")
	sheetInsertColsCmd.Flags().String("inherit-style", "", "继承样式: BEFORE（左侧列）, AFTER（右侧列）")
	sheetInsertColsCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")

	sheetCmd.AddCommand(sheetUpdateDimensionCmd)
	sheetUpdateDimensionCmd.Flags().String("range", "", "行列区间：\"3:5\"（行）或 \"B:D\"（列），1 起始、两端包含（必填）")
	sheetUpdateDimensionCmd.Flags().Bool("hidden", false, "隐藏（--hidden=false 取消隐藏）")
	sheetUpdateDimensionCmd.Flags().Int("size", 0, "行高 / 列宽（像素）")
	sheetUpdateDimensionCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
	mustMarkFlagRequired(sheetUpdateDimensionCmd, "range")

	sheetCmd.AddCommand(sheetMoveDimensionCmd)
	sheetMoveDimensionCmd.Flags().String("range", "", "要移动的区间：\"3:5\"（行）或 \"B:D\"（列），1 起始、两端包含（必填）")
	sheetMoveDimensionCmd.Flags().String("target", "", "目标位置：移动到原第 N 行 / 原 X 列之前，行号（如 10）或列字母（如 F）（必填）")
	sheetMoveDimensionCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
	mustMarkFlagRequired(sheetMoveDimensionCmd, "range", "target")
}
