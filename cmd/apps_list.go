package cmd

import (
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// appsListCmd 列出当前用户的妙搭应用（游标分页）。
//
// 对标官方 lark-cli，从 --help / 补全里隐藏（Hidden），避免 Agent 把它当成枚举/搜索
// 应用的入口。人类直接调用仍可用。Agent 需要 app_id 时应让用户给妙搭应用链接
// （从 /app/ 后面的 path 段取 app_id）或直接给 app_xxx 字符串。
var appsListCmd = &cobra.Command{
	Use:    "list",
	Short:  "列出当前用户的妙搭应用（游标分页）",
	Hidden: true,
	Long: `列出调用者拥有的妙搭（Miaoda）应用，游标分页。

权限: User Access Token + spark:app:read

示例:
  feishu-cli apps list --page-size 20
  feishu-cli apps list --page-token <上一页返回的 page_token>`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		params := map[string]any{
			"page_size": flagInt(cmd, "page-size"),
		}
		if token := strings.TrimSpace(flagString(cmd, "page-token")); token != "" {
			params["page_token"] = token
		}

		userToken, err := requireUserToken(cmd, "apps list")
		if err != nil {
			return err
		}
		data, err := client.SparkCall("GET", sparkBasePath+"/apps", params, nil, userToken)
		if err != nil {
			return err
		}
		return renderAppsResult(cmd, data)
	},
}

func init() {
	appsCmd.AddCommand(appsListCmd)
	appsListCmd.Flags().Int("page-size", 20, "分页大小")
	appsListCmd.Flags().String("page-token", "", "上一页返回的分页游标")
	addAppsCommonFlags(appsListCmd)
}

// flagInt 读取整型 flag，忽略错误。
func flagInt(cmd *cobra.Command, name string) int {
	v, _ := cmd.Flags().GetInt(name)
	return v
}
