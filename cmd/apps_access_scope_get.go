package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var appsAccessScopeGetCmd = &cobra.Command{
	Use:   "access-scope-get",
	Short: "查看妙搭应用的访问范围配置",
	Long: `读取一个妙搭（Miaoda）应用当前的访问范围。

响应原样透传服务端契约：字符串 scope 枚举（All=互联网公开 / Tenant=组织内 / Range=部分人员）
+ 拆分的 users / departments / chats 数组。

权限: User Access Token + spark:app:read

示例:
  feishu-cli apps access-scope-get --app-id app_xxx`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		appID := strings.TrimSpace(flagString(cmd, "app-id"))
		if appID == "" {
			return fmt.Errorf("--app-id 不能为空")
		}

		token, err := requireUserToken(cmd, "apps access-scope-get")
		if err != nil {
			return err
		}
		data, err := client.SparkCall("GET", appsAppPath(appID, "/access-scope"), nil, nil, token)
		if err != nil {
			return err
		}
		return renderAppsResult(cmd, data)
	},
}

func init() {
	appsCmd.AddCommand(appsAccessScopeGetCmd)
	appsAccessScopeGetCmd.Flags().String("app-id", "", "妙搭应用 ID（必填）")
	addAppsCommonFlags(appsAccessScopeGetCmd)
}
