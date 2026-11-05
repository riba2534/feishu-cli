package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/output"
	"github.com/spf13/cobra"
)

var appsDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "删除妙搭应用（删除前校验应用名称，需确认）",
	Long: `删除一个妙搭（Miaoda）应用。高危操作：

  1. GET /apps/{app_id} 取应用当前名称，与 --confirm-name 精确比对，不一致时拒绝删除（exit 2）
  2. DELETE /apps/{app_id}（使用第 1 步返回的 app_id）

--app-id 也接受 meta_token（第 1 步会解析出真实 app_id）。
--confirm-name 必填，取值用 apps get 查到的名称：
  feishu-cli apps get --app-id app_xxx --jq '.app.name'

执行前需要确认：交互终端会提示 y/N；非交互环境必须追加 --yes，否则 exit 10 且不发任何请求。
--dry-run 只打印两步计划，不获取 token、不访问网络。

输出 app_id / name / deleted；服务端返回 in_trash 时一并输出：in_trash=true 表示已移入回收站可恢复，
false 表示已永久删除。

权限: User Access Token + spark:app:read + spark:app:write

示例:
  feishu-cli apps delete --app-id app_xxx --confirm-name "我的页面" --dry-run
  feishu-cli apps delete --app-id app_xxx --confirm-name "我的页面" --yes`,
	RunE: func(cmd *cobra.Command, args []string) error {
		input := strings.TrimSpace(flagString(cmd, "app-id"))
		if input == "" {
			return clierr.Usagef("--app-id 不能为空")
		}
		confirmName := strings.TrimSpace(flagString(cmd, "confirm-name"))
		if confirmName == "" {
			return clierr.Usagef("--confirm-name 不能为空（填应用当前名称，可用 `feishu-cli apps get --app-id %s --jq '.app.name'` 查询）", input)
		}
		lookupPath := appsAppPath(input, "")

		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			deletePath := lookupPath
			if !strings.HasPrefix(input, "app_") {
				deletePath = sparkBasePath + "/apps/<resolved_app_id>"
			}
			o, err := output.ParseOptions(cmd)
			if err != nil {
				return err
			}
			return output.Render(o, map[string]any{
				"dry_run": true,
				"steps": []map[string]any{
					{"method": "GET", "path": lookupPath, "desc": "取应用当前名称，与 --confirm-name 比对"},
					{"method": "DELETE", "path": deletePath, "desc": "按第 1 步返回的 app_id 删除（不再二次比对名称）"},
				},
			})
		}

		if err := config.Validate(); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "apps delete")
		if err != nil {
			return err
		}
		if err := confirmDangerousAction(cmd, fmt.Sprintf("确定删除妙搭应用 %s（名称 %q）？", input, confirmName)); err != nil {
			return err
		}

		data, err := client.SparkCall("GET", lookupPath, nil, nil, token)
		if err != nil {
			return appsWithLookupHint(err)
		}
		appID, name, err := appsResolveForDelete(data, input)
		if err != nil {
			return err
		}
		if confirmName != name {
			return clierr.Usagef("--confirm-name 与应用 %s 的当前名称不一致，未删除\n提示：用 `feishu-cli apps get --app-id %s` 核对名称，与用户确认后用准确名称重试", appID, appID)
		}

		resp, err := client.SparkCall("DELETE", appsAppPath(appID, ""), nil, nil, token)
		if err != nil {
			return appsWithDeleteHint(err)
		}
		out := map[string]any{"app_id": appID, "name": name, "deleted": true}
		if inTrash, ok := resp["in_trash"].(bool); ok {
			out["in_trash"] = inTrash
			if inTrash {
				fmt.Fprintf(os.Stderr, "已移入回收站（可恢复）：%s（%s）\n", appID, name)
			} else {
				fmt.Fprintf(os.Stderr, "已永久删除：%s（%s）\n", appID, name)
			}
		}
		return renderAppsResult(cmd, out)
	},
}

// appsResolveForDelete 从 GET 响应取 app_id / name，并校验一致性（对齐官方 appForDelete）：
// 两者非空、app_id 为 app_ 开头；输入本身是 app_id 时必须与响应一致。
func appsResolveForDelete(data map[string]any, input string) (string, string, error) {
	app, _ := data["app"].(map[string]any)
	appID := sparkStringValue(app["app_id"])
	name := sparkStringValue(app["name"])
	if appID == "" || name == "" || !strings.HasPrefix(appID, "app_") ||
		(strings.HasPrefix(input, "app_") && appID != input) {
		return "", "", fmt.Errorf("妙搭应用查询响应异常：app_id / name 缺失或与 --app-id 不一致，未删除")
	}
	return appID, name, nil
}

// appsWithLookupHint 查询应用失败：不存在（40400）提示核对 app_id；类型不支持（40002）提示去网页控制台。
func appsWithLookupHint(err error) error {
	switch {
	case client.HasAPICode(err, 40002) || client.HasAPICode(err, 3340002):
		return appsWithHint(err, "该应用类型不能用 CLI 管理，请在妙搭网页控制台删除")
	default:
		return appsWithHint(err, appsAppIDListHint)
	}
}

// appsWithDeleteHint DELETE 失败：40300 / 40400 多为应用已删除或已无权访问。
func appsWithDeleteHint(err error) error {
	for _, code := range []int{40300, 3340300, 40400, 3340400} {
		if client.HasAPICode(err, code) {
			return appsWithHint(err, "应用可能已被删除或已无权访问；用 `feishu-cli apps list` 核对 app_id")
		}
	}
	return appsWithLookupHint(err)
}

func init() {
	appsCmd.AddCommand(appsDeleteCmd)
	appsDeleteCmd.Flags().String("app-id", "", "妙搭应用 ID 或 meta_token（必填）")
	appsDeleteCmd.Flags().String("confirm-name", "", "应用当前名称（必填，须与 apps get 返回的名称完全一致，否则拒绝删除）")
	addAppsWriteFlags(appsDeleteCmd)
}
