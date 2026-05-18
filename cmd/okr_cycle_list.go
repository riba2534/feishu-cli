package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var okrCycleListCmd = &cobra.Command{
	Use:   "list",
	Short: "获取当前租户的 OKR 周期列表",
	Long: `获取当前租户的 OKR 周期列表（/open-apis/okr/v1/periods，自动分页）。

注意：飞书 v1/periods 是租户级全局周期列表，不按用户过滤，所有成员看到的周期一致。

参数:
  --output, -o     输出格式：json

权限要求（应用 Token / Tenant，实测 user token 被服务端拒：99991668）:
  okr:okr:readonly 或 okr:okr.period:readonly

示例:
  # 查询当前租户所有 OKR 周期
  feishu-cli okr cycle list

  # JSON 输出（适合脚本消费）
  feishu-cli okr cycle list --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")

		// OKR API 服务端拒 user access token（99991668 实测），强制 App Token
		token := ""

		cycles, err := client.ListOKRCycles(client.ListOKRCyclesOptions{}, token)
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
			name := c.ZhName
			if name == "" {
				name = c.EnName
			}
			if name == "" {
				fmt.Printf("[%d] %s\n", idx+1, c.ID)
			} else {
				fmt.Printf("[%d] %s (%s)\n", idx+1, name, c.ID)
			}
			if c.StartTime != "" || c.EndTime != "" {
				fmt.Printf("    时间: %s ~ %s\n", c.StartTime, c.EndTime)
			}
			if c.CycleStatus != "" {
				fmt.Printf("    状态: %s\n", c.CycleStatus)
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

	okrCycleListCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	okrCycleListCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌，留空则自动读取登录态）")
}
