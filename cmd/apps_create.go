package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// validAppTypes 应用类型枚举。当前只有 HTML，未来可能扩展（SPA / NATIVE …）。
var validAppTypes = map[string]bool{"HTML": true}

var appsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建一个 HTML 妙搭应用",
	Long: `创建一个新的妙搭（Miaoda）应用，拿到 app_id 后用 apps html-publish 发布 HTML。

权限: User Access Token + spark:app:write

示例:
  feishu-cli apps create --name "我的页面" --app-type HTML
  feishu-cli apps create --name "Dashboard" --app-type HTML --description "数据看板"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		name := strings.TrimSpace(flagString(cmd, "name"))
		appType := strings.TrimSpace(flagString(cmd, "app-type"))
		if name == "" {
			return fmt.Errorf("--name 不能为空")
		}
		if appType == "" {
			return fmt.Errorf("--app-type 不能为空")
		}
		if !validAppTypes[appType] {
			return fmt.Errorf("--app-type %q 暂不支持（当前仅支持 HTML）", appType)
		}

		body := map[string]any{"name": name, "app_type": appType}
		if desc := strings.TrimSpace(flagString(cmd, "description")); desc != "" {
			body["description"] = desc
		}
		if icon := strings.TrimSpace(flagString(cmd, "icon-url")); icon != "" {
			body["icon_url"] = icon
		}

		path := sparkBasePath + "/apps"
		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			return appsDryRun("POST", path, nil, body)
		}

		token, err := requireUserToken(cmd, "apps create")
		if err != nil {
			return err
		}
		data, err := client.SparkCall("POST", path, nil, body, token)
		if err != nil {
			return err
		}
		return renderAppsResult(cmd, data)
	},
}

func init() {
	appsCmd.AddCommand(appsCreateCmd)
	appsCreateCmd.Flags().String("name", "", "应用显示名称（必填）")
	appsCreateCmd.Flags().String("app-type", "", "应用类型（当前仅支持 HTML）")
	appsCreateCmd.Flags().String("description", "", "应用描述")
	appsCreateCmd.Flags().String("icon-url", "", "应用图标 URL（不填用默认）")
	addAppsWriteFlags(appsCreateCmd)
}

// flagString 读取字符串 flag，忽略错误（未注册 flag 返回空串）。
func flagString(cmd *cobra.Command, name string) string {
	v, _ := cmd.Flags().GetString(name)
	return v
}
