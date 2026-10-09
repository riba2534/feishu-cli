package cmd

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"unicode"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

// 妙搭发布状态（release-get 的 status 字段）。
const (
	sparkReleasePublishing = "publishing"
	sparkReleasePending    = "pending"
	sparkReleaseFinished   = "finished"
	sparkReleaseFailed     = "failed"
)

// validateRealAppID 要求 --app-id 是 app_ 开头的真实应用 ID（官方 validateRealAppID）。
// meta_token 或 /page/<token>/ 链接需先用 apps get 换出 app_id。
func validateRealAppID(appID string) error {
	if !strings.HasPrefix(appID, "app_") {
		return clierr.Usagef("--app-id 必须是 app_ 开头的应用 ID，得到 %q\n提示：手上是 meta_token 或 /page/<token>/ 链接时，先执行 `feishu-cli apps get --app-id <meta_token> --jq '.app.app_id'` 取得 app_id", appID)
	}
	return nil
}

var appsGetCmd = &cobra.Command{
	Use:   "get",
	Short: "查看单个妙搭应用详情（app_type / 名称 / 发布状态等）",
	Long: `按 app_id（或 meta_token）查询单个妙搭应用详情。

返回 data.app：app_id、meta_token、app_type（HTML / FRONTEND / FULL_STACK / MODERN_HTML）、name、
description、icon_url、created_at、updated_at、is_published。
is_published=true 只说明应用历史上发布过，不代表最新内容已部署；要确认某次发布是否上线，看
apps release get 返回的 status=finished。

权限: User Access Token + spark:app:read

示例:
  feishu-cli apps get --app-id app_xxx
  feishu-cli apps get --app-id app_xxx --jq '.app.app_type'
  feishu-cli apps get --app-id <meta_token> --jq '.app.app_id'   # meta_token 换 app_id`,
	RunE: func(cmd *cobra.Command, args []string) error {
		appID := strings.TrimSpace(flagString(cmd, "app-id"))
		if appID == "" {
			return clierr.Usagef("--app-id 不能为空")
		}
		path := appsAppPath(appID, "")
		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			return appsDryRun(cmd, "GET", path, nil, nil)
		}
		if err := config.Validate(); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "apps get")
		if err != nil {
			return err
		}
		data, err := client.SparkCall("GET", path, nil, nil, token)
		if err != nil {
			return appsWithHint(err, "确认 app_id（妙搭应用链接 /app/ 后面那段，形如 app_xxx）与 spark:app:read 授权；只有应用名时用 `feishu-cli apps list --keyword <名称>` 查找")
		}
		return renderAppsResult(cmd, data)
	},
}

var appsReleaseCmd = &cobra.Command{
	Use:   "release",
	Short: "妙搭应用发布记录：查看单次发布状态 / 发布历史",
	Long: `查询妙搭应用的发布（release）状态与历史。

子命令:
  get    按 release_id 查询单次发布详情（status / online_url / error_logs / 审批节点）
  list   列出发布历史（最近的在前）

发布是异步的：html-publish 只返回 release_id，status=finished 后才能把 online_url 当作本次
发布的访问链接；status=failed 时 error_logs 给出失败步骤。`,
}

var appsReleaseGetCmd = &cobra.Command{
	Use:   "get",
	Short: "按 release_id 查询单次发布详情",
	Long: `按 release_id 查询单次发布的状态与详情。

输出（已把 data.release 展平，并把 current_node_info 中服务端可能返回的 camelCase 字段统一成 snake_case）:
  release_id / status / created_at / updated_at / commit_id
  online_url         status=finished 时为本次发布的线上访问链接（默认仅创建者可见，交付前按需 access-scope-set）
  error_logs         status=failed 时的失败步骤（step / error_log）
  current_node_info  审批节点：current_node / current_status / result.approval_url / submitted_by

status 取值与处理:
  finished    发布成功，读取 online_url（没有就只报告完成，不要编造链接）
  failed      发布失败，读取 error_logs
  publishing  进行中：约每 20 秒查询一次同一 release_id，总计约 5 分钟（或 html-publish --wait）
  pending / current_node_info.current_status=PENDING
              正在等待服务端配置的审批负责人处理，不是失败；停止轮询，审批后再查同一 release_id，
              不要重新发布。submitted_by 是申请人不是审批人

权限: User Access Token + spark:app:read

示例:
  feishu-cli apps release get --app-id app_xxx --release-id <release_id>
  feishu-cli apps release get --app-id app_xxx --release-id <release_id> --jq '.status'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		appID := strings.TrimSpace(flagString(cmd, "app-id"))
		releaseID := strings.TrimSpace(flagString(cmd, "release-id"))
		if appID == "" {
			return clierr.Usagef("--app-id 不能为空")
		}
		if err := validateRealAppID(appID); err != nil {
			return err
		}
		if releaseID == "" {
			return clierr.Usagef("--release-id 不能为空（来自 apps html-publish 或 apps release list）")
		}
		path := client.SparkReleaseGetPath(appID, releaseID)
		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			return appsDryRun(cmd, "GET", path, nil, nil)
		}
		if err := config.Validate(); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "apps release get")
		if err != nil {
			return err
		}
		data, err := client.SparkCall("GET", path, nil, nil, token)
		if err != nil {
			return appsWithHint(err, fmt.Sprintf("release_id 不确定时先列出发布历史：`feishu-cli apps release list --app-id %s`", appID))
		}
		return renderAppsResult(cmd, projectSparkRelease(data))
	},
}

var appsReleaseListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出妙搭应用的发布历史（最近的在前）",
	Long: `列出妙搭应用的发布历史，用于找回 release_id 或查看上次为什么失败。

参数:
  --app-id       应用 ID（必填）
  --status       按状态过滤：publishing / finished / failed
  --page-size    每页条数（默认 20，最大 500）
  --page-token   上一页返回的分页游标（has_more=true 时 stderr 会提示）

权限: User Access Token + spark:app:read

示例:
  feishu-cli apps release list --app-id app_xxx
  feishu-cli apps release list --app-id app_xxx --status failed --page-size 5
  feishu-cli apps release list --app-id app_xxx --jq '.releases[].release_id'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		appID := strings.TrimSpace(flagString(cmd, "app-id"))
		if appID == "" {
			return clierr.Usagef("--app-id 不能为空")
		}
		status := strings.TrimSpace(flagString(cmd, "status"))
		switch status {
		case "", sparkReleasePublishing, sparkReleaseFinished, sparkReleaseFailed:
		default:
			return clierr.Usagef("--status 取值 %q 不受支持，可选: publishing / finished / failed", status)
		}
		pageSize := flagInt(cmd, "page-size")
		if pageSize < 1 || pageSize > 500 {
			return clierr.Usagef("--page-size 取值范围 1-500，得到 %d", pageSize)
		}
		params := map[string]any{"page_size": pageSize}
		if status != "" {
			params["status"] = status
		}
		if pt := strings.TrimSpace(flagString(cmd, "page-token")); pt != "" {
			params["page_token"] = pt
		}
		path := client.SparkReleaseListPath(appID)
		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			return appsDryRun(cmd, "GET", path, params, nil)
		}
		if err := config.Validate(); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "apps release list")
		if err != nil {
			return err
		}
		data, err := client.SparkCall("GET", path, params, nil, token)
		if err != nil {
			return appsWithHint(err, "确认 app_id 与 spark:app:read 授权")
		}
		noteAppsNextPage(data, "apps release list --app-id "+appID)
		return renderAppsResult(cmd, data)
	},
}

// noteAppsNextPage has_more=true 时在 stderr 提示续翻游标（stdout 保持纯 JSON）。
func noteAppsNextPage(data map[string]any, command string) {
	hasMore, _ := data["has_more"].(bool)
	if !hasMore {
		return
	}
	token := sparkStringValue(data["page_token"])
	if token == "" {
		token = sparkStringValue(data["next_page_token"])
	}
	if token != "" {
		fmt.Fprintf(os.Stderr, "提示：还有更多结果，继续翻页：feishu-cli %s --page-token %s\n", command, token)
	} else {
		fmt.Fprintln(os.Stderr, "提示：还有更多结果（has_more=true），但响应未返回分页游标")
	}
}

func sparkStringValue(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

// appsWithHint 在错误后追加一行可执行的恢复建议（保留错误链与业务码）。
func appsWithHint(err error, hint string) error {
	if err == nil || hint == "" {
		return err
	}
	return fmt.Errorf("%w\n提示：%s", err, hint)
}

// projectSparkRelease 把 release 详情展平为稳定输出（对齐官方 projectReleaseDetail）：
//   - data.release 存在时以它为根，外层的 error_logs / current_node_info 优先（新响应放在 release 旁边）
//   - current_node_info 中的 camelCase 字段统一成 snake_case，保留未知字段
func projectSparkRelease(data map[string]any) map[string]any {
	root := data
	if rel, ok := data["release"].(map[string]any); ok {
		root = rel
	}
	out := make(map[string]any, len(root)+2)
	for k, v := range root {
		out[k] = v
	}
	delete(out, "release")
	for _, key := range []string{"error_logs", "current_node_info"} {
		if v, ok := data[key]; ok {
			out[key] = v
		} else if v, ok := root[key]; ok {
			out[key] = v
		}
	}
	if node, ok := out["current_node_info"].(map[string]any); ok {
		out["current_node_info"] = normalizeSparkCurrentNode(node)
	}
	return out
}

func normalizeSparkCurrentNode(node map[string]any) map[string]any {
	n := cloneAnyMap(node)
	renameSparkAlias(n, node, "current_node", "currentNode")
	renameSparkAlias(n, node, "current_status", "currentStatus")
	renameSparkAlias(n, node, "created_at", "createdAt")
	renameSparkAlias(n, node, "submitted_by", "submittedBy")
	if result, ok := n["result"].(map[string]any); ok {
		r := cloneAnyMap(result)
		renameSparkAlias(r, result, "approval_url", "approvalURL")
		n["result"] = r
	}
	if sub, ok := n["submitted_by"].(map[string]any); ok {
		s := cloneAnyMap(sub)
		renameSparkAlias(s, sub, "open_id", "openID")
		n["submitted_by"] = s
	}
	return n
}

// renameSparkAlias 规范名有非空值时优先；否则用兼容名的值；兼容名键总是删除。
func renameSparkAlias(dst, src map[string]any, canonical, compat string) {
	cv, hasCanonical := src[canonical]
	av, hasCompat := src[compat]
	delete(dst, compat)
	nonEmpty := func(v any) bool {
		if s, ok := v.(string); ok {
			return s != ""
		}
		return v != nil
	}
	switch {
	case hasCanonical && nonEmpty(cv):
		dst[canonical] = cv
	case hasCompat && nonEmpty(av):
		dst[canonical] = av
	case hasCanonical:
		dst[canonical] = cv
	case hasCompat:
		dst[canonical] = av
	}
}

func cloneAnyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// sparkReleasePendingApproval 尚未进入终态且 current_node_info.current_status=PENDING：等待审批负责人处理。
func sparkReleasePendingApproval(rel map[string]any) bool {
	status := sparkStringValue(rel["status"])
	if status == sparkReleaseFinished || status == sparkReleaseFailed {
		return false
	}
	node, _ := rel["current_node_info"].(map[string]any)
	return strings.EqualFold(sparkStringValue(node["current_status"]), "PENDING")
}

// sparkApprovalURL 返回可展示的审批链接：只接受带 host 的绝对 https URL，其余视为无效。
func sparkApprovalURL(rel map[string]any) string {
	node, _ := rel["current_node_info"].(map[string]any)
	result, _ := node["result"].(map[string]any)
	raw := sparkStringValue(result["approval_url"])
	u, err := url.Parse(raw)
	if raw == "" || err != nil || u.Scheme != "https" || u.Host == "" {
		return ""
	}
	return raw
}

// sparkAgentName 读取发起方 Agent 名（创建应用时作为 source_agent 上报）：
// FEISHU_CLI_AGENT_NAME 优先，兼容官方 LARKSUITE_CLI_AGENT_NAME；超长或含控制字符的值丢弃。
func sparkAgentName() string {
	for _, key := range []string{"FEISHU_CLI_AGENT_NAME", "LARKSUITE_CLI_AGENT_NAME"} {
		v := strings.TrimSpace(os.Getenv(key))
		if v == "" || len(v) > 128 {
			continue
		}
		clean := true
		for _, r := range v {
			if unicode.IsControl(r) {
				clean = false
				break
			}
		}
		if clean {
			return v
		}
	}
	return ""
}

func init() {
	appsCmd.AddCommand(appsGetCmd)
	appsGetCmd.Flags().String("app-id", "", "妙搭应用 ID 或 meta_token（必填）")
	appsGetCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求")
	addAppsCommonFlags(appsGetCmd)

	appsCmd.AddCommand(appsReleaseCmd)
	appsReleaseCmd.AddCommand(appsReleaseGetCmd)
	appsReleaseGetCmd.Flags().String("app-id", "", "妙搭应用 ID，app_ 开头（必填）")
	appsReleaseGetCmd.Flags().String("release-id", "", "发布 ID（html-publish 返回的 release_id，必填）")
	appsReleaseGetCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求")
	addAppsCommonFlags(appsReleaseGetCmd)

	appsReleaseCmd.AddCommand(appsReleaseListCmd)
	appsReleaseListCmd.Flags().String("app-id", "", "妙搭应用 ID（必填）")
	appsReleaseListCmd.Flags().String("status", "", "按状态过滤: publishing / finished / failed")
	appsReleaseListCmd.Flags().Int("page-size", 20, "每页条数（1-500）")
	appsReleaseListCmd.Flags().String("page-token", "", "上一页返回的分页游标")
	appsReleaseListCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求")
	addAppsCommonFlags(appsReleaseListCmd)
}
