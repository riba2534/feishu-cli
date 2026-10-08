package cmd

import (
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// appsListCmd 列出当前用户可见的妙搭应用（游标分页），主要用途是按应用名定位 app_id。
var appsListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出可见的妙搭应用，按名称定位 app_id（游标分页）",
	Long: `列出当前用户可见的妙搭（Miaoda）应用，游标分页。

用途：下游命令需要 app_id、而用户只给了应用名时，用 --keyword 定位；用户已给出 app_xxx 或妙搭链接
（https://miaoda.feishu.cn/app/app_xxx）时直接提取，不必再 list。避免无目的的全量枚举。

参数:
  --keyword      按应用名模糊搜索
  --ownership    all（默认：我创建的 + 共享给我的）/ mine / shared
  --app-type     html / frontend / full_stack
  --page-size    每页条数（默认 20）
  --page-token   上一页返回的分页游标（has_more=true 时 stderr 会提示）

输出 data.items[]：app_id、name、description、is_published、online_url、updated_at 等。
is_published / online_url 只是发布态快照，不能证明最新内容已部署（看 apps release get 的 finished）。

权限: User Access Token + spark:app:read

示例:
  feishu-cli apps list --keyword "审批"
  feishu-cli apps list --ownership mine --app-type html
  feishu-cli apps list --page-token <上一页返回的 page_token>`,
	RunE: func(cmd *cobra.Command, args []string) error {
		params := map[string]any{
			"page_size": flagInt(cmd, "page-size"),
		}
		if token := strings.TrimSpace(flagString(cmd, "page-token")); token != "" {
			params["page_token"] = token
		}
		if kw := strings.TrimSpace(flagString(cmd, "keyword")); kw != "" {
			params["keyword"] = kw
		}
		if ownership := strings.TrimSpace(flagString(cmd, "ownership")); ownership != "" {
			if ownership != "all" && ownership != "mine" && ownership != "shared" {
				return clierr.Usagef("--ownership 取值 %q 不受支持，可选: all / mine / shared", ownership)
			}
			params["ownership"] = ownership
		}
		if appType := strings.TrimSpace(flagString(cmd, "app-type")); appType != "" {
			if !validAppTypes[appType] {
				return clierr.Usagef("--app-type 取值 %q 不受支持，可选: html / frontend / full_stack", appType)
			}
			params["app_type"] = appType
		}
		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			return appsDryRun(cmd, "GET", sparkBasePath+"/apps", params, nil)
		}

		if err := config.Validate(); err != nil {
			return err
		}
		userToken, err := requireUserToken(cmd, "apps list")
		if err != nil {
			return err
		}
		data, err := client.SparkCall("GET", sparkBasePath+"/apps", params, nil, userToken)
		if err != nil {
			return err
		}
		noteAppsNextPage(data, "apps list")
		return renderAppsResult(cmd, data)
	},
}

func init() {
	appsCmd.AddCommand(appsListCmd)
	appsListCmd.Flags().Int("page-size", 20, "分页大小")
	appsListCmd.Flags().String("page-token", "", "上一页返回的分页游标")
	appsListCmd.Flags().String("keyword", "", "按应用名模糊搜索")
	appsListCmd.Flags().String("ownership", "", "归属过滤: all / mine / shared（默认 all）")
	appsListCmd.Flags().String("app-type", "", "类型过滤: html / frontend / full_stack")
	appsListCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求")
	addAppsCommonFlags(appsListCmd)
}
