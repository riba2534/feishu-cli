package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

var sheetProtectCmd = &cobra.Command{
	Use:   "protect <spreadsheet_token|url> <sheet_id>",
	Short: "创建保护范围",
	Long: `创建整行或整列的保护范围（只能保护整行/整列，单次最多 5000 行或列）。

范围写法（二选一）:
  --range "1:5" / "A:C"                       1 起始、两端包含，维度由写法决定（行号=行，列字母=列）
  --dimension ROWS --start 0 --end 5          0 起始、--end 不包含（与 delete-rows 等命令一致）

CLI 会换算成接口口径（保护接口为 1 起始、两端包含）。
--users 指定除所有者外允许编辑的用户（默认 open_id，可用 --user-id-type union_id）。

示例:
  # 保护前 5 行
  feishu-cli sheet protect shtcnxxxxxx 0b12 --range "1:5"
  feishu-cli sheet protect shtcnxxxxxx 0b12 --dimension ROWS --start 0 --end 5

  # 保护 A-C 列，并允许指定用户编辑
  feishu-cli sheet protect shtcnxxxxxx 0b12 --range "A:C" --users ou_xxx,ou_yyy --lock-info "财务数据"`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sheetID := args[1]
		lockInfo, _ := cmd.Flags().GetString("lock-info")
		output, _ := cmd.Flags().GetString("output")
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		usersRaw, _ := cmd.Flags().GetString("users")

		dr, err := resolveProtectRange(cmd, sheetID)
		if err != nil {
			return err
		}
		users := splitAndTrim(usersRaw)
		userIDType = strings.TrimSpace(userIDType)
		if userIDType != "open_id" && userIDType != "union_id" {
			return clierr.Usagef("--user-id-type 仅支持 open_id / union_id，得到 %q", userIDType)
		}

		ranges := []*client.ProtectedRange{
			{
				SheetID: sheetID,
				Dimension: &client.Dimension{
					SheetID:        sheetID,
					MajorDimension: dr.Major,
					StartIndex:     dr.Start,
					EndIndex:       dr.End,
				},
				LockInfo: lockInfo,
				Users:    users,
			},
		}

		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}

		protectIDs, err := client.CreateProtectedRange(client.Context(), target.Token, ranges, userIDType, target.UAT)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(map[string]any{
				"protect_ids":     protectIDs,
				"major_dimension": dr.Major,
				"range":           dr.A1(),
				"start_index":     dr.Start,
				"end_index":       dr.End,
			})
		}
		fmt.Printf("保护范围创建成功！\n")
		fmt.Printf("  保护范围: %s\n", describeDimRange(dr))
		fmt.Printf("  保护 ID: %v\n", protectIDs)
		return nil
	},
}

// resolveProtectRange 解析 protect 的范围：--range（A1，1 起始两端包含）或 --dimension + --start/--end（0 起始、不含 end）。
func resolveProtectRange(cmd *cobra.Command, sheetID string) (client.DimRange, error) {
	if cmd.Flags().Changed("range") {
		if cmd.Flags().Changed("start") || cmd.Flags().Changed("end") {
			return client.DimRange{}, clierr.Usagef("--range 与 --start/--end 不能同时使用")
		}
		raw, _ := cmd.Flags().GetString("range")
		dr, err := parseDimRangeFlag(raw, sheetID, "")
		if err != nil {
			return client.DimRange{}, err
		}
		if cmd.Flags().Changed("dimension") {
			dim, _ := cmd.Flags().GetString("dimension")
			if !strings.EqualFold(strings.TrimSpace(dim), dr.Major) {
				return client.DimRange{}, clierr.Usagef("--dimension %s 与 --range %q 的维度（%s）不一致", dim, raw, dr.Major)
			}
		}
		return dr, nil
	}
	if !cmd.Flags().Changed("end") {
		return client.DimRange{}, clierr.Usagef("请用 --range \"1:5\" / \"A:C\"，或 --dimension + --start/--end（0 起始、--end 不包含）指定保护范围")
	}
	dim, _ := cmd.Flags().GetString("dimension")
	major := strings.ToUpper(strings.TrimSpace(dim))
	if major != "ROWS" && major != "COLUMNS" {
		return client.DimRange{}, clierr.Usagef("--dimension 仅支持 ROWS / COLUMNS，得到 %q", dim)
	}
	start, _ := cmd.Flags().GetInt("start")
	end, _ := cmd.Flags().GetInt("end")
	if start < 0 || end <= start {
		return client.DimRange{}, clierr.Usagef("--start 从 0 开始、--end 不包含且须大于 --start，得到 --start %d --end %d", start, end)
	}
	return client.DimRange{Major: major, Start: start + 1, End: end}, nil
}

var sheetUnprotectCmd = &cobra.Command{
	Use:   "unprotect <spreadsheet_token|url> <protect_ids...>",
	Short: "删除保护范围",
	Long: `删除指定的保护范围。

示例:
  feishu-cli sheet unprotect shtcnxxxxxx protectId1 protectId2`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		protectIDs := args[1:]

		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}

		if err := client.DeleteProtectedRange(client.Context(), target.Token, protectIDs, target.UAT); err != nil {
			return err
		}

		fmt.Printf("保护范围删除成功！删除了 %d 个保护范围\n", len(protectIDs))
		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetProtectCmd)
	sheetCmd.AddCommand(sheetUnprotectCmd)

	sheetProtectCmd.Flags().String("range", "", "保护范围：\"1:5\"（行）或 \"A:C\"（列），1 起始、两端包含")
	sheetProtectCmd.Flags().String("dimension", "ROWS", "保护维度: ROWS, COLUMNS（配合 --start/--end）")
	sheetProtectCmd.Flags().Int("start", 0, "起始索引（从 0 开始）")
	sheetProtectCmd.Flags().Int("end", 0, "结束索引（不包含）")
	sheetProtectCmd.Flags().String("lock-info", "", "锁定说明")
	sheetProtectCmd.Flags().String("users", "", "除所有者外允许编辑的用户 ID，逗号分隔（类型见 --user-id-type）")
	sheetProtectCmd.Flags().String("user-id-type", "open_id", "--users 的 ID 类型: open_id, union_id")
	sheetProtectCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	sheetProtectCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")

	sheetUnprotectCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
