package cmd

import (
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var appsFeaturesCmd = &cobra.Command{
	Use:   "features",
	Short: "查看妙搭应用的业务功能开关（如 database）",
	Long: `只读查询一个妙搭（Miaoda）应用的业务功能（目前为 database）及是否已开启：
GET /apps/{app_id}/features。

响应原样输出：app_id、features[]（name / enabled），传 --operation-ref 时还有该次开启操作的
operation（ref / state / error_reason / retryable）。
enabled=true 只说明应用的业务开关已开，不代表本地开发环境已就绪。
开启功能（feature-enable）不在本 CLI 范围内，请在妙搭云端开发中操作。

参数:
  --app-id          应用 ID，必须 app_ 开头（必填）
  --operation-ref   之前某次开启操作的 idempotent_key，用于查看该操作的状态（自动转小写）

权限: User Access Token + spark:app:read

示例:
  feishu-cli apps features --app-id app_xxx
  feishu-cli apps features --app-id app_xxx --jq '.features[] | select(.name=="database") | .enabled'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		appID := strings.TrimSpace(flagString(cmd, "app-id"))
		if appID == "" {
			return clierr.Usagef("--app-id 不能为空")
		}
		if err := validateRealAppID(appID); err != nil {
			return err
		}
		var params map[string]any
		// 服务端按小写保存 idempotent_key（对齐官方 canonicalIdempotentKey）
		if ref := strings.ToLower(strings.TrimSpace(flagString(cmd, "operation-ref"))); ref != "" {
			params = map[string]any{"operation_ref": ref}
		}
		path := client.SparkAppFeaturesPath(appID)
		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			return appsDryRun(cmd, "GET", path, params, nil)
		}

		if err := config.Validate(); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "apps features")
		if err != nil {
			return err
		}
		data, err := client.SparkCall("GET", path, params, nil, token)
		if err != nil {
			// 2200：网关内部错误，部署中尚未提供应用功能接口时也是这个答复
			if client.HasAPICode(err, 2200) || client.HasHTTPStatus(err, 404) {
				return appsWithHint(err, "当前服务端可能尚未提供应用功能接口；确认 app_id 无误后不要反复重试，可在妙搭云端开发中查看数据库状态")
			}
			return appsWithHint(err, appsAppIDListHint)
		}
		return renderAppsResult(cmd, data)
	},
}

func init() {
	appsCmd.AddCommand(appsFeaturesCmd)
	appsFeaturesCmd.Flags().String("app-id", "", "妙搭应用 ID，app_ 开头（必填）")
	appsFeaturesCmd.Flags().String("operation-ref", "", "之前某次开启操作的 idempotent_key（查看该操作状态）")
	addAppsCommonFlags(appsFeaturesCmd)
	appsFeaturesCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求")
}
