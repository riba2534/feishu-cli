package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// validAppTypes 应用类型枚举（对齐官方小写 html / frontend / full_stack）。
var validAppTypes = map[string]bool{"html": true, "frontend": true, "full_stack": true}

// legacyAppTypes 旧版大写写法（HTML 等）保留为兼容别名：归一为小写发送，并在 stderr 提示一次。
var legacyAppTypes = map[string]string{"HTML": "html", "FRONTEND": "frontend", "FULL_STACK": "full_stack"}

// normalizeAppsCreateType 校验并归一 --app-type，返回发送给服务端的小写取值。
func normalizeAppsCreateType(appType string) (string, error) {
	if validAppTypes[appType] {
		return appType, nil
	}
	if lower, ok := legacyAppTypes[appType]; ok {
		fmt.Fprintf(os.Stderr, "提示：--app-type %s 是旧写法，已按 %s 处理；请改用小写 html / frontend / full_stack\n", appType, lower)
		return lower, nil
	}
	return "", clierr.Usagef("--app-type %q 不受支持，可选: html / frontend / full_stack", appType)
}

// buildAppsCreateBody 构造创建请求体；FEISHU_CLI_AGENT_NAME（兼容 LARKSUITE_CLI_AGENT_NAME）非空时附带 source_agent。
func buildAppsCreateBody(name, appType, description, iconURL string) map[string]any {
	body := map[string]any{"name": name, "app_type": appType}
	if description != "" {
		body["description"] = description
	}
	if iconURL != "" {
		body["icon_url"] = iconURL
	}
	if agent := sparkAgentName(); agent != "" {
		body["source_agent"] = agent
	}
	return body
}

var appsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建一个妙搭应用（html / frontend / full_stack）",
	Long: `创建一个新的妙搭（Miaoda）应用，拿到 app_id。HTML 应用随后用 apps html-publish 发布。

--app-type 取值（对齐官方小写枚举）:
  html         静态 HTML 应用（apps html-publish 可直接发布）
  frontend     纯前端交互应用
  full_stack   带数据库的全栈应用
  旧写法 HTML / FRONTEND / FULL_STACK 仍兼容，会归一为小写并在 stderr 提示。

设置环境变量 FEISHU_CLI_AGENT_NAME（兼容 LARKSUITE_CLI_AGENT_NAME）时，请求体附带 source_agent，
标记应用由哪个 Agent 创建。

权限: User Access Token + spark:app:write

示例:
  feishu-cli apps create --name "我的页面" --app-type html
  feishu-cli apps create --name "Dashboard" --app-type html --description "数据看板"
  feishu-cli apps create --name "审批系统" --app-type full_stack --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.TrimSpace(flagString(cmd, "name"))
		appType := strings.TrimSpace(flagString(cmd, "app-type"))
		if name == "" {
			return clierr.Usagef("--name 不能为空")
		}
		if appType == "" {
			return clierr.Usagef("--app-type 不能为空（html / frontend / full_stack）")
		}
		normalized, err := normalizeAppsCreateType(appType)
		if err != nil {
			return err
		}
		body := buildAppsCreateBody(name, normalized, strings.TrimSpace(flagString(cmd, "description")), strings.TrimSpace(flagString(cmd, "icon-url")))

		path := sparkBasePath + "/apps"
		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			return appsDryRun(cmd, "POST", path, nil, body)
		}

		if err := config.Validate(); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "apps create")
		if err != nil {
			return err
		}
		data, err := client.SparkCall("POST", path, nil, body, token)
		if err != nil {
			return appsWithHint(err, "确认 --app-type 为 html / frontend / full_stack、--name 非空；权限错误时确认账号可创建妙搭应用并已授权 spark:app:write")
		}
		return renderAppsResult(cmd, data)
	},
}

func init() {
	appsCmd.AddCommand(appsCreateCmd)
	appsCreateCmd.Flags().String("name", "", "应用显示名称（必填）")
	appsCreateCmd.Flags().String("app-type", "", "应用类型: html / frontend / full_stack（必填）")
	appsCreateCmd.Flags().String("description", "", "应用描述")
	appsCreateCmd.Flags().String("icon-url", "", "应用图标 URL（不填用默认）")
	addAppsWriteFlags(appsCreateCmd)
}
