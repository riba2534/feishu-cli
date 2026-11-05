package cmd

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/output"
	"github.com/spf13/cobra"
)

// maxAppsReleaseApplyReasonRunes --apply-reason 字符数上限（对齐官方 maxReleaseApplyReasonRunes）。
const maxAppsReleaseApplyReasonRunes = 1000

var appsReleaseCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "为妙搭应用创建一次发布（返回 release_id，可 --wait 等待结果）",
	Long: `按分支为妙搭应用创建一次发布：POST /apps/{app_id}/releases。

适用于在妙搭编辑器或云端开发里改完代码的 frontend / full_stack 应用；HTML 应用发布本地文件用
apps html-publish。发布是异步的，默认只返回 release_id / status / sync；加 --wait 会像 html-publish
一样轮询 release 状态直到终态（finished 输出 online_url，failed 输出 error_logs 并非零退出）。
--wait 只轮询状态，查询遇到限流或网络错误时退避重试；发布请求本身只发一次，不会重放。

参数:
  --app-id         应用 ID，必须 app_ 开头（必填）
  --branch         发布分支（不填由服务端用默认分支）
  --apply-reason   发布申请理由（frontend / full_stack 应用必填，≤1000 字符；HTML 应用不填）

权限: User Access Token + spark:app:write（--wait 还需 spark:app:read）

示例:
  feishu-cli apps release create --app-id app_xxx
  feishu-cli apps release create --app-id app_xxx --branch sprint/default --apply-reason "修复线上问题" --dry-run
  feishu-cli apps release create --app-id app_xxx --apply-reason "版本发布" --wait`,
	RunE: func(cmd *cobra.Command, args []string) error {
		appID := strings.TrimSpace(flagString(cmd, "app-id"))
		if appID == "" {
			return clierr.Usagef("--app-id 不能为空")
		}
		if err := validateRealAppID(appID); err != nil {
			return err
		}
		applyReason := flagString(cmd, "apply-reason")
		if cmd.Flags().Changed("apply-reason") {
			if err := validateAppsReleaseApplyReason(applyReason); err != nil {
				return err
			}
		}
		body := map[string]any{}
		if branch := strings.TrimSpace(flagString(cmd, "branch")); branch != "" {
			body["branch"] = branch
		}
		if applyReason != "" {
			body["apply_reason"] = applyReason
		}
		path := client.SparkReleaseCreatePath(appID)
		wait, _ := cmd.Flags().GetBool("wait")
		timeout, _ := cmd.Flags().GetDuration("wait-timeout")

		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			o, err := output.ParseOptions(cmd)
			if err != nil {
				return err
			}
			plan := map[string]any{
				"method":  "POST",
				"path":    path,
				"params":  nil,
				"body":    body,
				"dry_run": true,
			}
			if wait {
				plan["wait"] = map[string]any{
					"method": "GET",
					"path":   appsAppPath(appID, "/releases/<release_id>"),
					"desc":   fmt.Sprintf("--wait：每 %s 查询一次发布状态直到 finished/failed/待审批，最长 %s", appsReleasePollInterval, timeout),
				}
			}
			return output.Render(o, plan)
		}

		if err := config.Validate(); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "apps release create")
		if err != nil {
			return err
		}
		data, err := client.SparkCall("POST", path, nil, body, token)
		if err != nil {
			return appsWithHint(err, fmt.Sprintf("frontend / full_stack 应用需提供 --apply-reason；推送被拒（non-fast-forward）时先同步分支再重试；查看失败原因用 `feishu-cli apps release get --app-id %s --release-id <release_id>`", appID))
		}
		releaseID := sparkStringValue(data["release_id"])
		if wait {
			return appsWaitRelease(cmd, appID, releaseID, token, timeout, "apps release create")
		}
		// 投影为稳定字段（对齐官方 projectReleaseCreateData）
		return renderAppsResult(cmd, map[string]any{
			"release_id": releaseID,
			"status":     sparkStringValue(data["status"]),
			"sync":       data["sync"] == true,
		})
	},
}

// validateAppsReleaseApplyReason 对齐官方 validateReleaseApplyReason：非空、合法 UTF-8、
// 无控制字符与 BiDi/零宽等危险字符、≤1000 字符。
func validateAppsReleaseApplyReason(value string) error {
	if strings.TrimSpace(value) == "" {
		return clierr.Usagef("--apply-reason 不能为空")
	}
	if !utf8.ValidString(value) {
		return clierr.Usagef("--apply-reason 必须是合法的 UTF-8 文本")
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return clierr.Usagef("--apply-reason 不能包含控制字符")
		}
		if isMailDangerousRune(r) {
			return clierr.Usagef("--apply-reason 不能包含 BiDi 覆盖、零宽等危险 Unicode 字符")
		}
	}
	if n := utf8.RuneCountInString(value); n > maxAppsReleaseApplyReasonRunes {
		return clierr.Usagef("--apply-reason 最多 %d 个字符，得到 %d", maxAppsReleaseApplyReasonRunes, n)
	}
	return nil
}

func init() {
	appsReleaseCmd.AddCommand(appsReleaseCreateCmd)
	appsReleaseCreateCmd.Flags().String("app-id", "", "妙搭应用 ID，app_ 开头（必填）")
	appsReleaseCreateCmd.Flags().String("branch", "", "发布分支（不填由服务端用默认分支）")
	appsReleaseCreateCmd.Flags().String("apply-reason", "", "发布申请理由（frontend / full_stack 必填，≤1000 字符；HTML 应用不填）")
	appsReleaseCreateCmd.Flags().Bool("wait", false, "创建后轮询 release 状态直到终态，输出 online_url / error_logs（失败时非零退出）")
	appsReleaseCreateCmd.Flags().Duration("wait-timeout", 5*time.Minute, "--wait 的最长等待时间")
	addAppsWriteFlags(appsReleaseCreateCmd)
}
