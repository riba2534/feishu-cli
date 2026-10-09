package cmd

import (
	"fmt"
	"net/url"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/output"
	"github.com/spf13/cobra"
)

// appsCmd 妙搭（Miaoda）应用父命令：秒搭 HTML 应用 + 一键发布 + 访问范围管理。
var appsCmd = &cobra.Command{
	Use:   "apps",
	Short: "妙搭（Miaoda）应用：创建 / 发布 HTML / 发布状态 / 访问范围管理",
	Long: `妙搭（Miaoda）低代码应用平台操作 —— 把一份 HTML 秒级发布成一个可分享的飞书应用。

所有 apps 子命令均需 User Access Token，且需要妙搭权限 scope：
  feishu-cli auth check --scope "spark:app:read spark:app:write"   # 先预检
  feishu-cli auth login --scope "spark:app:read spark:app:write"
auth login 是增量授权：多次登录申请的 scope 在服务端累积，补授 spark scope 不会丢掉已有授权。

子命令:
  create            创建妙搭应用（html / frontend / full_stack）
  get               查看单个应用详情（app_type / 发布状态等）
  list              按名称定位应用 app_id（--keyword）
  html-publish      把 HTML 文件/目录按官方三段协议打包发布，返回 release_id（--wait 等待完成拿 online_url）
  release get       按 release_id 查询发布状态 / online_url / error_logs
  release list      发布历史
  update            修改应用名称 / 描述
  access-scope-get  查看应用访问范围
  access-scope-set  设置应用访问范围（specific / public / tenant）

典型流程:
  feishu-cli apps create --name "我的页面" --app-type html                 # 拿 app_id
  feishu-cli apps html-publish --app-id app_xxx --path ./site --wait       # 发布并等待 online_url
  feishu-cli apps access-scope-set --app-id app_xxx --scope tenant`,
}

// sparkBasePath 妙搭 OpenAPI 前缀（与 client.SparkBasePath 同源）。
const sparkBasePath = client.SparkBasePath

func init() {
	rootCmd.AddCommand(appsCmd)
}

// addAppsCommonFlags 给 apps 子命令注册通用 flag：--user-access-token + --format/--jq。
func addAppsCommonFlags(cmd *cobra.Command) {
	cmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	output.AddFormatFlags(cmd)
}

// addAppsWriteFlags 在通用 flag 基础上追加 --dry-run（mutating 操作预览）。
func addAppsWriteFlags(cmd *cobra.Command) {
	addAppsCommonFlags(cmd)
	output.AddDryRunFlag(cmd)
}

// renderAppsResult 渲染妙搭返回的 data：注册了 --format → 走 output 包（支持 --jq/--format）；
// 否则回退 printJSON。
func renderAppsResult(cmd *cobra.Command, data any) error {
	if cmd.Flags().Lookup("format") != nil {
		o, err := output.ParseOptions(cmd)
		if err != nil {
			return err
		}
		return output.Render(o, data)
	}
	return printJSON(data)
}

// appsDryRun 打印写命令将要发出的请求（不实际调用）。
// dry-run 预览同样尊重 --format/--jq（与实调路径 renderAppsResult 一致），
// 避免 help 列了 --format/--jq 却在 dry-run 时静默失效（对齐 bitable dry-run 行为）。
func appsDryRun(cmd *cobra.Command, method, path string, params map[string]any, body any) error {
	o, err := output.ParseOptions(cmd)
	if err != nil {
		return err
	}
	return output.Render(o, map[string]any{
		"method":  method,
		"path":    path,
		"params":  params,
		"body":    body,
		"dry_run": true,
	})
}

// appsAppPath 构造 /open-apis/spark/v1/apps/{app_id}{suffix}，对 app_id 做 path 转义。
func appsAppPath(appID, suffix string) string {
	return fmt.Sprintf("%s/apps/%s%s", sparkBasePath, url.PathEscape(appID), suffix)
}
