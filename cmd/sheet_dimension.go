package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

// sheet_dimension.go —— 行列操作的口径换算与新增的行列命令（insert-cols / update-dimension / move-dimension）。
//
// 飞书 v2 行列接口的索引口径并不统一（均已实测）：
//   - 删除 DELETE dimension_range、更新 PUT dimension_range、保护 protected_dimension：
//     从 1 开始、两端包含（startIndex=2,endIndex=3 删除第 2、3 行；startIndex=0 报 90202）；
//   - 插入 insert_dimension_range：从 0 开始、不含 endIndex（startIndex=1,endIndex=2 在第 1、2 行之间插 1 行）；
//   - v3 move_dimension：从 0 开始、两端包含。
//
// 命令层对外只暴露两种写法：
//   - 旧 flag --start/--end：0 起始、--end 不包含（保持原有帮助文档语义）；
//   - 新 flag --range："3:5"（行）/ "B:D"（列），1 起始、两端包含，与官方 lark-cli 一致。
// 进入 client 之前统一换算成接口口径。

// dimLabel 返回维度的中文名。
func dimLabel(major string) string {
	if major == "COLUMNS" {
		return "列"
	}
	return "行"
}

// parseDimRangeFlag 解析 --range（可带与 sheetID 一致的子表前缀），并校验维度。
func parseDimRangeFlag(raw, sheetID, wantMajor string) (client.DimRange, error) {
	raw = strings.TrimSpace(unescapeSheetRange(raw))
	if prefix, rest, ok := client.SplitSheetRangePrefix(raw); ok {
		if prefix != sheetID {
			return client.DimRange{}, clierr.Usagef("--range 的子表前缀 %q 与 <sheet_id> %q 不一致；--range 只需写 \"3:5\" 或 \"B:D\"", prefix, sheetID)
		}
		raw = rest
	}
	dr, err := client.ParseDimRange(raw)
	if err != nil {
		return client.DimRange{}, clierr.Usagef("--range 解析失败: %v", err)
	}
	if wantMajor != "" && dr.Major != wantMajor {
		if wantMajor == "ROWS" {
			return client.DimRange{}, clierr.Usagef("--range %q 是列区间，该命令操作行，请写成 \"3:5\" 这样的行号区间", raw)
		}
		return client.DimRange{}, clierr.Usagef("--range %q 是行区间，该命令操作列，请写成 \"B:D\" 这样的列字母区间", raw)
	}
	return dr, nil
}

// legacyStartEnd 读取旧 flag --start（0 起始）/ --end（不包含；缺省或 0 表示只操作 1 行/列），
// 返回 1 起始、两端包含的区间。
func legacyStartEnd(cmd *cobra.Command, major string) (client.DimRange, error) {
	start, _ := cmd.Flags().GetInt("start")
	end, _ := cmd.Flags().GetInt("end")
	if start < 0 {
		return client.DimRange{}, clierr.Usagef("--start 从 0 开始，不能为负数，得到 %d", start)
	}
	if end == 0 {
		end = start + 1
	}
	if end <= start {
		return client.DimRange{}, clierr.Usagef("--end（不包含）必须大于 --start，得到 --start %d --end %d", start, end)
	}
	// 0 起始 [start, end) == 1 起始 [start+1, end]
	return client.DimRange{Major: major, Start: start + 1, End: end}, nil
}

// resolveDimRangeFlags 统一处理 --range 与 --start/--end 两种写法（二选一）。
func resolveDimRangeFlags(cmd *cobra.Command, sheetID, major string) (client.DimRange, error) {
	hasRange := cmd.Flags().Changed("range")
	hasStart := cmd.Flags().Changed("start") || cmd.Flags().Changed("end")
	switch {
	case hasRange && hasStart:
		return client.DimRange{}, clierr.Usagef("--range 与 --start/--end 不能同时使用")
	case hasRange:
		raw, _ := cmd.Flags().GetString("range")
		return parseDimRangeFlag(raw, sheetID, major)
	case cmd.Flags().Changed("start"):
		return legacyStartEnd(cmd, major)
	default:
		if major == "COLUMNS" {
			return client.DimRange{}, clierr.Usagef("请用 --range \"B:D\"（1 起始、两端包含）或 --start/--end（0 起始、--end 不包含）指定列范围")
		}
		return client.DimRange{}, clierr.Usagef("请用 --range \"3:5\"（1 起始、两端包含）或 --start/--end（0 起始、--end 不包含）指定行范围")
	}
}

// describeDimRange 生成人类可读的区间描述，如「第 3 到 5 行（3:5，共 3 行）」。
func describeDimRange(dr client.DimRange) string {
	label := dimLabel(dr.Major)
	if dr.Start == dr.End {
		return fmt.Sprintf("第 %d %s（%s）", dr.Start, label, dr.A1())
	}
	return fmt.Sprintf("第 %d 到 %d %s（%s，共 %d %s）", dr.Start, dr.End, label, dr.A1(), dr.Count(), label)
}

// dimensionRangeURL 是 v2 dimension_range 的接口路径（dry-run 展示用）。
func dimensionRangeURL(token string) string {
	return fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/dimension_range", token)
}

// runSheetDeleteDimension 是 delete-rows / delete-cols 的公共实现。
func runSheetDeleteDimension(cmd *cobra.Command, args []string, major string) error {
	sheetID := args[1]
	dr, err := resolveDimRangeFlags(cmd, sheetID, major)
	if err != nil {
		return err
	}
	desc := describeDimRange(dr)

	if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
		token, _, isWiki, err := parseSpreadsheetArg(args[0], "")
		if err != nil {
			return err
		}
		extra := map[string]any{
			"spreadsheet_token": token,
			"sheet_id":          sheetID,
			"major_dimension":   dr.Major,
			"range":             dr.A1(),
			"count":             dr.Count(),
			"irreversible":      true,
		}
		if isWiki {
			extra["wiki_node_token"] = token
		}
		return printDryRunPlan(cmd, "删除"+desc+"（不可撤销）", extra, []dryRunStep{{
			Method: "DELETE",
			URL:    dimensionRangeURL(token),
			Desc:   "接口口径：startIndex/endIndex 从 1 开始、两端包含",
			Body: map[string]any{"dimension": map[string]any{
				"sheetId": sheetID, "majorDimension": dr.Major, "startIndex": dr.Start, "endIndex": dr.End,
			}},
		}})
	}

	target, err := newSheetTarget(cmd, args[0])
	if err != nil {
		return err
	}
	if err := client.DeleteDimension(client.Context(), target.Token, sheetID, dr.Major, dr.Start, dr.End, target.UAT); err != nil {
		return err
	}
	fmt.Printf("成功删除%s\n", desc)
	return nil
}

// runSheetInsertDimension 是 insert-rows / insert-cols 的公共实现。
// --range 表示插入后新行/列所在的位置（1 起始、两端包含）；--start/--end 为 0 起始、--end 不包含。
// 二者都换算成插入接口口径（0 起始、不含 endIndex）。
func runSheetInsertDimension(cmd *cobra.Command, args []string, major string) error {
	sheetID := args[1]
	inheritStyle, _ := cmd.Flags().GetString("inherit-style")
	inheritStyle = strings.ToUpper(strings.TrimSpace(inheritStyle))
	if inheritStyle != "" && inheritStyle != "BEFORE" && inheritStyle != "AFTER" {
		return clierr.Usagef("--inherit-style 仅支持 BEFORE / AFTER，得到 %q", inheritStyle)
	}
	dr, err := resolveDimRangeFlags(cmd, sheetID, major)
	if err != nil {
		return err
	}

	target, err := newSheetTarget(cmd, args[0])
	if err != nil {
		return err
	}
	if err := client.InsertDimension(client.Context(), target.Token, sheetID, major, dr.Start-1, dr.End, inheritStyle, target.UAT); err != nil {
		return err
	}
	fmt.Printf("成功插入 %d %s，新%s为%s\n", dr.Count(), dimLabel(major), dimLabel(major), describeDimRange(dr))
	return nil
}
