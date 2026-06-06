package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var appsAccessScopeAllowedTargetTypes = map[string]bool{
	"user":       true,
	"department": true,
	"chat":       true,
}

// appsScopeToServerEnum 把 CLI 友好的 scope 字符串映射成后端字符串枚举。
// 后端语义：All=互联网公开 / Tenant=组织内 / Range=部分人员。
var appsScopeToServerEnum = map[string]string{
	"public":   "All",
	"tenant":   "Tenant",
	"specific": "Range",
}

var appsAccessScopeSetCmd = &cobra.Command{
	Use:   "access-scope-set",
	Short: "设置妙搭应用的访问范围（specific / public / tenant）",
	Long: `设置一个妙搭（Miaoda）应用的访问范围。

--scope:
  specific  部分人员 —— 必须配 --targets；可选 --apply-enabled / --approver
  public    互联网公开 —— 必须显式给 --require-login（true/false）
  tenant    组织内可见 —— 不接受其它 flag

--targets 是统一格式的 JSON 数组，发请求时会拆成后端的 users/departments/chats：
  [{"type":"user|department|chat","id":"..."}, ...]

权限: User Access Token + spark:app:write

示例:
  feishu-cli apps access-scope-set --app-id app_xxx --scope tenant
  feishu-cli apps access-scope-set --app-id app_xxx --scope public --require-login=true
  feishu-cli apps access-scope-set --app-id app_xxx --scope specific \
    --targets '[{"type":"user","id":"ou_xxx"},{"type":"chat","id":"oc_xxx"}]'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		appID := strings.TrimSpace(flagString(cmd, "app-id"))
		if appID == "" {
			return fmt.Errorf("--app-id 不能为空")
		}
		if err := validateAppsAccessScopeFlags(cmd); err != nil {
			return err
		}

		body, err := buildAppsAccessScopeBody(cmd)
		if err != nil {
			return err
		}

		path := appsAppPath(appID, "/access-scope")
		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			return appsDryRun("PUT", path, nil, body)
		}

		token, err := requireUserToken(cmd, "apps access-scope-set")
		if err != nil {
			return err
		}
		data, err := client.SparkCall("PUT", path, nil, body, token)
		if err != nil {
			return err
		}
		return renderAppsResult(cmd, data)
	},
}

// validateAppsAccessScopeFlags 按 scope 校验 flag 组合，规则对齐官方 lark-cli。
func validateAppsAccessScopeFlags(cmd *cobra.Command) error {
	scope := flagString(cmd, "scope")
	targets := strings.TrimSpace(flagString(cmd, "targets"))
	applyEnabled, _ := cmd.Flags().GetBool("apply-enabled")
	approver := strings.TrimSpace(flagString(cmd, "approver"))
	requireLogin, _ := cmd.Flags().GetBool("require-login")

	switch scope {
	case "specific":
		if targets == "" {
			return fmt.Errorf("--scope=specific 时必须提供 --targets")
		}
		if err := validateAppsTargetsJSON(targets); err != nil {
			return err
		}
		if approver != "" && !applyEnabled {
			return fmt.Errorf("--approver 需要配合 --apply-enabled")
		}
		if requireLogin {
			return fmt.Errorf("--scope=specific 时不允许 --require-login")
		}
	case "public":
		if targets != "" {
			return fmt.Errorf("--scope=public 时不允许 --targets")
		}
		if applyEnabled {
			return fmt.Errorf("--scope=public 时不允许 --apply-enabled")
		}
		if approver != "" {
			return fmt.Errorf("--scope=public 时不允许 --approver")
		}
		if !cmd.Flags().Changed("require-login") {
			return fmt.Errorf("--scope=public 时必须显式给 --require-login（true 或 false，不要依赖默认值）")
		}
	case "tenant":
		if targets != "" || applyEnabled || approver != "" || requireLogin {
			return fmt.Errorf("--scope=tenant 时不允许其它 flag")
		}
	default:
		return fmt.Errorf("--scope 必须是 specific / public / tenant")
	}
	return nil
}

func validateAppsTargetsJSON(targetsJSON string) error {
	var items []map[string]any
	if err := json.Unmarshal([]byte(targetsJSON), &items); err != nil {
		return fmt.Errorf("--targets 不是合法 JSON: %w", err)
	}
	if len(items) == 0 {
		return fmt.Errorf("--targets 至少要有一项；specific 范围需要具体的 user/department/chat id")
	}
	for i, t := range items {
		typ, _ := t["type"].(string)
		if !appsAccessScopeAllowedTargetTypes[typ] {
			return fmt.Errorf("--targets[%d].type %q 必须是 user / department / chat 之一", i, typ)
		}
		if id, _ := t["id"].(string); strings.TrimSpace(id) == "" {
			return fmt.Errorf("--targets[%d].id 为空", i)
		}
	}
	return nil
}

func buildAppsAccessScopeBody(cmd *cobra.Command) (map[string]any, error) {
	scope := flagString(cmd, "scope")
	enum, ok := appsScopeToServerEnum[scope]
	if !ok {
		return nil, fmt.Errorf("--scope 必须是 specific / public / tenant，得到 %q", scope)
	}
	body := map[string]any{"scope": enum}

	switch scope {
	case "specific":
		var targets []map[string]any
		if err := json.Unmarshal([]byte(flagString(cmd, "targets")), &targets); err != nil {
			return nil, fmt.Errorf("--targets 不是合法 JSON: %w", err)
		}
		users, departments, chats := splitAppsAccessScopeTargets(targets)
		if len(users) > 0 {
			body["users"] = users
		}
		if len(departments) > 0 {
			body["departments"] = departments
		}
		if len(chats) > 0 {
			body["chats"] = chats
		}
		if applyEnabled, _ := cmd.Flags().GetBool("apply-enabled"); applyEnabled {
			applyConfig := map[string]any{"enabled": true}
			if approver := strings.TrimSpace(flagString(cmd, "approver")); approver != "" {
				applyConfig["approvers"] = []string{approver}
			}
			body["apply_config"] = applyConfig
		}
	case "public":
		requireLogin, _ := cmd.Flags().GetBool("require-login")
		body["require_login"] = requireLogin
	}
	return body, nil
}

// splitAppsAccessScopeTargets 把统一 [{type,id}] 形态拆成后端要求的 users/departments/chats 三个数组。
func splitAppsAccessScopeTargets(targets []map[string]any) (users, departments, chats []string) {
	for _, t := range targets {
		typ, _ := t["type"].(string)
		id, _ := t["id"].(string)
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		switch typ {
		case "user":
			users = append(users, id)
		case "department":
			departments = append(departments, id)
		case "chat":
			chats = append(chats, id)
		}
	}
	return
}

func init() {
	appsCmd.AddCommand(appsAccessScopeSetCmd)
	appsAccessScopeSetCmd.Flags().String("app-id", "", "妙搭应用 ID（必填）")
	appsAccessScopeSetCmd.Flags().String("scope", "", "访问范围: specific | public | tenant（必填）")
	appsAccessScopeSetCmd.Flags().String("targets", "", `目标 JSON 数组: [{"type":"user|department|chat","id":"..."}, ...]`)
	appsAccessScopeSetCmd.Flags().Bool("apply-enabled", false, "允许申请访问（scope=specific）")
	appsAccessScopeSetCmd.Flags().String("approver", "", "审批人 open_id（配合 --apply-enabled，后端只允许一个）")
	appsAccessScopeSetCmd.Flags().Bool("require-login", false, "是否要求登录（scope=public 时必填）")
	addAppsWriteFlags(appsAccessScopeSetCmd)
}
