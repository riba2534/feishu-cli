package cmd

import (
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/spf13/cobra"
)

// ==================== 仪表盘 / 表单 分享设置（base/v3 share 端点） ====================
// 对齐官方 base +dashboard-share-get/update、+form-share-get/update（shortcuts/base/
// dashboard_share.go、form_share.go、share_common.go）：
//   GET   /open-apis/base/v3/bases/{bt}/dashboards/{dashboard_id}/share
//   PATCH /open-apis/base/v3/bases/{bt}/dashboards/{dashboard_id}/share
//   GET   /open-apis/base/v3/bases/{bt}/tables/{tid}/forms/{form_id}/share
//   PATCH /open-apis/base/v3/bases/{bt}/tables/{tid}/forms/{form_id}/share
// update 为 PATCH 语义且每次只改一个字段（官方约束）；布尔字段可用 --x=false 显式关闭。

var bitableShareAccessScopes = []string{"invite", "tenant", "anyone"}

// buildBitableShareUpdateBody 收集 share update 的字段：恰好一个 flag 被显式设置。
// common：enabled / access-scope；settingFlags：放进 body.settings 的布尔开关（flag 名 → JSON 键）。
func buildBitableShareUpdateBody(cmd *cobra.Command, settingFlags [][2]string) (map[string]any, error) {
	all := []string{"enabled", "access-scope"}
	for _, f := range settingFlags {
		all = append(all, f[0])
	}
	var changed []string
	for _, name := range all {
		if cmd.Flags().Changed(name) {
			changed = append(changed, "--"+name)
		}
	}
	if len(changed) == 0 {
		names := make([]string, 0, len(all))
		for _, n := range all {
			names = append(names, "--"+n)
		}
		return nil, clierr.Usagef("需要指定一个要修改的分享字段: %s", strings.Join(names, " / "))
	}
	if len(changed) > 1 {
		return nil, clierr.Usagef("每次只能修改一个分享字段，请把 %s 拆成多次调用", strings.Join(changed, "、"))
	}
	body := map[string]any{}
	if cmd.Flags().Changed("enabled") {
		v, _ := cmd.Flags().GetBool("enabled")
		body["enabled"] = v
	}
	if cmd.Flags().Changed("access-scope") {
		v, _ := cmd.Flags().GetString("access-scope")
		if err := validateEnum(v, "access-scope", bitableShareAccessScopes); err != nil {
			return nil, clierr.Usage(err)
		}
		body["access_scope"] = v
	}
	settings := map[string]any{}
	for _, f := range settingFlags {
		if cmd.Flags().Changed(f[0]) {
			v, _ := cmd.Flags().GetBool(f[0])
			settings[f[1]] = v
		}
	}
	if len(settings) > 0 {
		body["settings"] = settings
	}
	return body, nil
}

var bitableDashboardShareCmd = &cobra.Command{
	Use:   "share",
	Short: "仪表盘分享设置（get/update）",
}

var bitableDashboardShareGetCmd = &cobra.Command{
	Use:   "get",
	Short: "查看仪表盘分享状态与设置",
	Long:  `GET /open-apis/base/v3/bases/{base_token}/dashboards/{dashboard_id}/share（需要 base:dashboard:update 权限）`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dashboardID, _ := cmd.Flags().GetString("dashboard-id")
		if strings.TrimSpace(dashboardID) == "" {
			return clierr.Usagef("--dashboard-id 必填")
		}
		return runBitableShare(cmd, "GET", func(bt string) string {
			return client.BaseV3Path("bases", bt, "dashboards", dashboardID, "share")
		}, nil, true)
	},
}

var bitableDashboardShareUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "修改仪表盘分享设置（每次改一个字段）",
	Long: `PATCH /open-apis/base/v3/bases/{base_token}/dashboards/{dashboard_id}/share

每次只能修改一个字段（服务端 PATCH 语义）；布尔值用 --show-source=false 这种写法显式关闭。

  --enabled          开启/关闭分享
  --access-scope     分享范围: invite（仅邀请）| tenant（组织内）| anyone（互联网）
  --show-source      是否显示返回源多维表格的入口

示例:
  feishu-cli bitable dashboard share update --base-token <bt> --dashboard-id <did> --enabled
  feishu-cli bitable dashboard share update --base-token <bt> --dashboard-id <did> --access-scope tenant`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dashboardID, _ := cmd.Flags().GetString("dashboard-id")
		if strings.TrimSpace(dashboardID) == "" {
			return clierr.Usagef("--dashboard-id 必填")
		}
		body, err := buildBitableShareUpdateBody(cmd, [][2]string{{"show-source", "show_source"}})
		if err != nil {
			return err
		}
		return runBitableShare(cmd, "PATCH", func(bt string) string {
			return client.BaseV3Path("bases", bt, "dashboards", dashboardID, "share")
		}, body, true)
	},
}

var bitableFormShareCmd = &cobra.Command{
	Use:   "share",
	Short: "表单分享设置（get/update，base/v3）",
}

var bitableFormShareGetCmd = &cobra.Command{
	Use:   "get",
	Short: "查看表单分享状态与设置",
	Long:  `GET /open-apis/base/v3/bases/{base_token}/tables/{table_id}/forms/{form_id}/share（需要 base:form:update 权限）`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		formID, _ := cmd.Flags().GetString("form-id")
		if strings.TrimSpace(tableID) == "" || strings.TrimSpace(formID) == "" {
			return clierr.Usagef("--table-id 和 --form-id 必填")
		}
		return runBitableShare(cmd, "GET", func(bt string) string {
			return client.BaseV3Path("bases", bt, "tables", tableID, "forms", formID, "share")
		}, nil, false)
	},
}

var bitableFormShareUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "修改表单分享设置（每次改一个字段）",
	Long: `PATCH /open-apis/base/v3/bases/{base_token}/tables/{table_id}/forms/{form_id}/share

每次只能修改一个字段（服务端 PATCH 语义）；布尔值用 --require-login=false 这种写法显式关闭。

  --enabled           开启/关闭分享
  --access-scope      分享范围: invite | tenant | anyone
  --allow-anonymous   匿名提交（隐藏提交人身份）
  --require-login     提交前需要登录

另有 form patch --shared/--shared-limit/--submit-limit-once（bitable/v1，一次可改多个字段）。

示例:
  feishu-cli bitable form share update --base-token <bt> --table-id <tid> --form-id <fid> --enabled
  feishu-cli bitable form share update --base-token <bt> --table-id <tid> --form-id <fid> --access-scope anyone`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		formID, _ := cmd.Flags().GetString("form-id")
		if strings.TrimSpace(tableID) == "" || strings.TrimSpace(formID) == "" {
			return clierr.Usagef("--table-id 和 --form-id 必填")
		}
		body, err := buildBitableShareUpdateBody(cmd, [][2]string{
			{"allow-anonymous", "allow_anonymous"},
			{"require-login", "require_login"},
		})
		if err != nil {
			return err
		}
		return runBitableShare(cmd, "PATCH", func(bt string) string {
			return client.BaseV3Path("bases", bt, "tables", tableID, "forms", formID, "share")
		}, body, false)
	},
}

// runBitableShare 调用 share 端点；dashboard 输出去掉仍在灰度中的 settings.enable_auto_analysis（对齐官方）。
func runBitableShare(cmd *cobra.Command, method string, pathFn func(bt string) string, body map[string]any, dashboard bool) error {
	return bitableRunWithTransform(cmd, func(bt string) bitableReq {
		req := bitableReq{method: method, path: pathFn(bt)}
		if body != nil {
			req.body = body
		}
		return req
	}, func(data map[string]any) map[string]any {
		if dashboard {
			if settings, ok := data["settings"].(map[string]any); ok {
				delete(settings, "enable_auto_analysis")
			}
		}
		return data
	})
}

func init() {
	bitableDashboardCmd.AddCommand(bitableDashboardShareCmd)
	bitableDashboardShareCmd.AddCommand(bitableDashboardShareGetCmd, bitableDashboardShareUpdateCmd)
	addBitableCommonFlags(bitableDashboardShareGetCmd)
	bitableDashboardShareGetCmd.Flags().String("dashboard-id", "", "仪表盘 ID（必填）")
	addBitableWriteFlags(bitableDashboardShareUpdateCmd)
	bitableDashboardShareUpdateCmd.Flags().String("dashboard-id", "", "仪表盘 ID（必填）")
	bitableDashboardShareUpdateCmd.Flags().Bool("enabled", false, "开启/关闭分享（--enabled=false 关闭）")
	bitableDashboardShareUpdateCmd.Flags().String("access-scope", "", "分享范围: invite|tenant|anyone")
	bitableDashboardShareUpdateCmd.Flags().Bool("show-source", false, "显示返回源多维表格的入口（--show-source=false 关闭）")

	bitableFormCmd.AddCommand(bitableFormShareCmd)
	bitableFormShareCmd.AddCommand(bitableFormShareGetCmd, bitableFormShareUpdateCmd)
	for _, c := range []*cobra.Command{bitableFormShareGetCmd, bitableFormShareUpdateCmd} {
		c.Flags().String("table-id", "", "table_id（必填）")
		c.Flags().String("form-id", "", "form_id（即表单视图 view_id，必填）")
	}
	addBitableCommonFlags(bitableFormShareGetCmd)
	addBitableWriteFlags(bitableFormShareUpdateCmd)
	bitableFormShareUpdateCmd.Flags().Bool("enabled", false, "开启/关闭分享（--enabled=false 关闭）")
	bitableFormShareUpdateCmd.Flags().String("access-scope", "", "分享范围: invite|tenant|anyone")
	bitableFormShareUpdateCmd.Flags().Bool("allow-anonymous", false, "匿名提交（--allow-anonymous=false 关闭）")
	bitableFormShareUpdateCmd.Flags().Bool("require-login", false, "提交前需要登录（--require-login=false 关闭）")
}
