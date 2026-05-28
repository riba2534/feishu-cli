package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

// ==================== dashboard 仪表盘 ====================
var bitableDashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "仪表盘管理（list/copy）",
}

var bitableDashboardListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出仪表盘",
	Long: `GET /open-apis/base/v3/bases/{base_token}/dashboards

可选:
  --page-size    分页大小（≤100）
  --page-token   下一页 token`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		params := map[string]any{}
		if pageSize > 0 {
			params["page_size"] = pageSize
		}
		if pageToken != "" {
			params["page_token"] = pageToken
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "GET", path: client.BaseV3Path("bases", bt, "dashboards"), params: params}
		})
	},
}

var bitableDashboardCopyCmd = &cobra.Command{
	Use:   "copy",
	Short: "复制仪表盘（base/v3 无此端点，走 bitable/v1）",
	Long: `POST /open-apis/bitable/v1/apps/{app_token}/dashboards/{dashboard_id}/copy

base/v3 无仪表盘复制端点，本命令走 bitable/v1（app_token 即 base_token）。

必填:
  --dashboard-id  要复制的仪表盘 ID
  --name          新仪表盘名称`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dashboardID, _ := cmd.Flags().GetString("dashboard-id")
		name, _ := cmd.Flags().GetString("name")
		if dashboardID == "" {
			return fmt.Errorf("--dashboard-id 必填")
		}
		if name == "" {
			return fmt.Errorf("--name 必填")
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{
				method: "POST",
				path:   client.BitableV1Path("apps", bt, "dashboards", dashboardID, "copy"),
				body:   map[string]any{"name": name},
				useV1:  true,
			}
		})
	},
}

func init() {
	bitableCmd.AddCommand(bitableDashboardCmd)

	bitableDashboardCmd.AddCommand(bitableDashboardListCmd)
	addBitableCommonFlags(bitableDashboardListCmd)
	bitableDashboardListCmd.Flags().Int("page-size", 0, "分页大小（≤100）")
	bitableDashboardListCmd.Flags().String("page-token", "", "分页 token")

	bitableDashboardCmd.AddCommand(bitableDashboardCopyCmd)
	addBitableWriteFlags(bitableDashboardCopyCmd)
	bitableDashboardCopyCmd.Flags().String("dashboard-id", "", "仪表盘 ID（必填）")
	bitableDashboardCopyCmd.Flags().String("name", "", "新仪表盘名称（必填）")
}
