package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var appsUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "修改妙搭应用的名称 / 描述（部分更新）",
	Long: `部分更新一个妙搭（Miaoda）应用，只发送提供的字段（--name / --description 至少一个）。

权限: User Access Token + spark:app:write

示例:
  feishu-cli apps update --app-id app_xxx --name "新名字"
  feishu-cli apps update --app-id app_xxx --description "更新后的描述"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		appID := strings.TrimSpace(flagString(cmd, "app-id"))
		if appID == "" {
			return fmt.Errorf("--app-id 不能为空")
		}

		body := map[string]any{}
		if v := strings.TrimSpace(flagString(cmd, "name")); v != "" {
			body["name"] = v
		}
		if v := strings.TrimSpace(flagString(cmd, "description")); v != "" {
			body["description"] = v
		}
		if len(body) == 0 {
			return fmt.Errorf("至少提供 --name 或 --description 之一")
		}

		path := appsAppPath(appID, "")
		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			return appsDryRun(cmd, "PATCH", path, nil, body)
		}

		token, err := requireUserToken(cmd, "apps update")
		if err != nil {
			return err
		}
		data, err := client.SparkCall("PATCH", path, nil, body, token)
		if err != nil {
			return err
		}
		return renderAppsResult(cmd, data)
	},
}

func init() {
	appsCmd.AddCommand(appsUpdateCmd)
	appsUpdateCmd.Flags().String("app-id", "", "妙搭应用 ID（必填）")
	appsUpdateCmd.Flags().String("name", "", "新的应用显示名称")
	appsUpdateCmd.Flags().String("description", "", "新的应用描述")
	addAppsWriteFlags(appsUpdateCmd)
}
