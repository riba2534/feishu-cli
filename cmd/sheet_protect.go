package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

var sheetProtectCmd = &cobra.Command{
	Use:   "protect <spreadsheet_token> <sheet_id>",
	Short: "创建保护范围",
	Long: `创建行或列的保护范围。

示例:
  # 保护前 5 行
  feishu-cli sheet protect shtcnxxxxxx 0b12 --dimension ROWS --start 0 --end 5

  # 保护 A-C 列
  feishu-cli sheet protect shtcnxxxxxx 0b12 --dimension COLUMNS --start 0 --end 3`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken := args[0]
		sheetID := args[1]
		dimension, _ := cmd.Flags().GetString("dimension")
		startIndex, _ := cmd.Flags().GetInt("start")
		endIndex, _ := cmd.Flags().GetInt("end")
		lockInfo, _ := cmd.Flags().GetString("lock-info")
		output, _ := cmd.Flags().GetString("output")

		ranges := []*client.ProtectedRange{
			{
				SheetID: sheetID,
				Dimension: &client.Dimension{
					SheetID:        sheetID,
					MajorDimension: dimension,
					StartIndex:     startIndex,
					EndIndex:       endIndex,
				},
				LockInfo: lockInfo,
			},
		}

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		protectIDs, err := client.CreateProtectedRange(client.Context(), spreadsheetToken, ranges, userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			if err := printJSON(map[string]any{
				"protect_ids": protectIDs,
			}); err != nil {
				return err
			}
		} else {
			fmt.Printf("保护范围创建成功！\n")
			fmt.Printf("  保护 ID: %v\n", protectIDs)
		}

		return nil
	},
}

var sheetUnprotectCmd = &cobra.Command{
	Use:   "unprotect <spreadsheet_token> <protect_ids...>",
	Short: "删除保护范围",
	Long: `删除指定的保护范围。

示例:
  feishu-cli sheet unprotect shtcnxxxxxx protectId1 protectId2`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		spreadsheetToken := args[0]
		protectIDs := args[1:]

		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		err := client.DeleteProtectedRange(client.Context(), spreadsheetToken, protectIDs, userAccessToken)
		if err != nil {
			return err
		}

		fmt.Printf("保护范围删除成功！删除了 %d 个保护范围\n", len(protectIDs))
		return nil
	},
}

func init() {
	sheetCmd.AddCommand(sheetProtectCmd)
	sheetCmd.AddCommand(sheetUnprotectCmd)

	sheetProtectCmd.Flags().String("dimension", "ROWS", "保护维度: ROWS, COLUMNS")
	sheetProtectCmd.Flags().Int("start", 0, "起始索引")
	sheetProtectCmd.Flags().Int("end", 0, "结束索引")
	sheetProtectCmd.Flags().String("lock-info", "", "锁定说明")
	sheetProtectCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	sheetProtectCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
	mustMarkFlagRequired(sheetProtectCmd, "end")

	sheetUnprotectCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
}
