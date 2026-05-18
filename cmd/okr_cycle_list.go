package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var okrCycleListCmd = &cobra.Command{
	Use:   "list",
	Short: "获取指定用户的 OKR 周期列表",
	Long: `获取指定用户的 OKR 周期列表（v2 接口，自动分页）。

参数:
  --user-id        用户 ID（必填）
  --user-id-type   用户 ID 类型：open_id（默认） / union_id / user_id
  --output, -o     输出格式：json

权限要求（User Token）:
  okr:okr:readonly 或 okr:okr.period:readonly

示例:
  # 查询用户 OKR 周期列表（默认 open_id）
  feishu-cli okr cycle list --user-id ou_xxx

  # 用 user_id 查询
  feishu-cli okr cycle list --user-id 123456 --user-id-type user_id

  # JSON 输出（适合脚本消费）
  feishu-cli okr cycle list --user-id ou_xxx --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		userID, _ := cmd.Flags().GetString("user-id")
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		output, _ := cmd.Flags().GetString("output")

		if err := validateUserIDType(userIDType); err != nil {
			return err
		}

		token := resolveOptionalUserTokenWithFallback(cmd)

		cycles, err := client.ListOKRCycles(client.ListOKRCyclesOptions{
			UserID:     userID,
			UserIDType: userIDType,
		}, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(map[string]any{
				"cycles": cycles,
				"total":  len(cycles),
			})
		}

		if len(cycles) == 0 {
			fmt.Println("未找到 OKR 周期")
			return nil
		}

		fmt.Printf("共找到 %d 个 OKR 周期\n", len(cycles))
		for idx, c := range cycles {
			fmt.Printf("[%d] %s\n", idx+1, c.ID)
			if c.StartTime != "" || c.EndTime != "" {
				fmt.Printf("    时间: %s ~ %s\n", c.StartTime, c.EndTime)
			}
			if c.CycleStatus != "" {
				fmt.Printf("    状态: %s\n", c.CycleStatus)
			}
			if c.TenantCycleID != "" {
				fmt.Printf("    租户周期 ID: %s\n", c.TenantCycleID)
			}
		}

		return nil
	},
}

// validateUserIDType 校验 user-id-type 取值
func validateUserIDType(t string) error {
	switch t {
	case "open_id", "union_id", "user_id":
		return nil
	default:
		return fmt.Errorf("不支持的 --user-id-type: %s（可选: open_id / union_id / user_id）", t)
	}
}

func init() {
	okrCycleCmd.AddCommand(okrCycleListCmd)

	okrCycleListCmd.Flags().String("user-id", "", "用户 ID（必填）")
	okrCycleListCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型：open_id / union_id / user_id")
	okrCycleListCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	okrCycleListCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌，留空则自动读取登录态）")
	mustMarkFlagRequired(okrCycleListCmd, "user-id")
}
