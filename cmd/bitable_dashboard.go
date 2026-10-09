package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

// ==================== dashboard 仪表盘 ====================
var bitableDashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "仪表盘管理（list/copy/create/get/update/delete/arrange + block 子组）",
}

var bitableDashboardListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出仪表盘（自动翻页）",
	Long: `GET /open-apis/base/v3/bases/{base_token}/dashboards

默认自动按 page_token 翻页取全部，输出 {"items":[...],"total":N,"has_more":false}。

可选:
  --page-size    分页大小（≤100）
  --page-token   只取指定页（兼容旧用法；还有下一页时 stderr 提示续翻 token）`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pageSize, _ := cmd.Flags().GetInt("page-size")
		if pageSize < 0 || pageSize > 100 {
			return clierr.Usagef("--page-size 范围 1-100，当前 %d", pageSize)
		}
		return runBitablePageTokenList(cmd, func(bt string) bitablePageTokenList {
			return bitablePageTokenList{Method: "GET", Path: client.BaseV3Path("bases", bt, "dashboards"), Key: "items", PageSize: pageSize}
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
