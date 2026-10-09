package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

// ==================== role 子命令组 ====================
var bitableRoleCmd = &cobra.Command{
	Use:   "role",
	Short: "角色管理（list/get/create/update/delete）+ member 子组（协作者增删）",
}

func bitableRolePath(baseToken string, extra ...string) string {
	parts := []string{"bases", baseToken, "roles"}
	parts = append(parts, extra...)
	return client.BaseV3Path(parts...)
}

var bitableRoleListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出角色",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBaseV3Role(cmd, "GET", func(bt string) string {
			return bitableRolePath(bt)
		}, nil)
	},
}

var bitableRoleGetCmd = &cobra.Command{
	Use:   "get",
	Short: "获取角色",
	RunE: func(cmd *cobra.Command, args []string) error {
		roleID, _ := cmd.Flags().GetString("role-id")
		if roleID == "" {
			return fmt.Errorf("--role-id 必填")
		}
		return runBaseV3Role(cmd, "GET", func(bt string) string {
			return bitableRolePath(bt, roleID)
		}, nil)
	},
}

var bitableRoleCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建角色",
	Long:  `通过 --config/--config-file 传入完整 role 定义`,
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := loadBitableJSONBody(cmd)
		if err != nil {
			return err
		}
		return runBaseV3Role(cmd, "POST", func(bt string) string {
			return bitableRolePath(bt)
		}, body)
	},
}

var bitableRoleUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "更新角色",
	RunE: func(cmd *cobra.Command, args []string) error {
		roleID, _ := cmd.Flags().GetString("role-id")
		if roleID == "" {
			return fmt.Errorf("--role-id 必填")
		}
		body, err := loadBitableJSONBody(cmd)
		if err != nil {
			return err
		}
		return runBaseV3Role(cmd, "PUT", func(bt string) string {
			return bitableRolePath(bt, roleID)
		}, body)
	},
}

var bitableRoleDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "删除角色",
	RunE: func(cmd *cobra.Command, args []string) error {
		roleID, _ := cmd.Flags().GetString("role-id")
		if roleID == "" {
			return fmt.Errorf("--role-id 必填")
		}
		return runBaseV3Role(cmd, "DELETE", func(bt string) string {
			return bitableRolePath(bt, roleID)
		}, nil)
	},
}

// runBaseV3Role 调用角色接口并解开多层响应（外层 data 里再套一层二次序列化的 data，
// 内层可能带自己的 code/message——外层 code=0 但内层失败时必须报错，见 client.UnwrapBaseRoleData）。
func runBaseV3Role(cmd *cobra.Command, method string, pathFn func(baseToken string) string, body any) error {
	if err := config.Validate(); err != nil {
		return err
	}
	token, err := resolveIdentityToken(cmd)
	if err != nil {
		return err
	}
	baseToken, err := resolveBaseToken(cmd)
	if err != nil {
		return err
	}
	data, err := client.BaseV3Call(method, pathFn(baseToken), nil, body, token)
	if err != nil {
		return err
	}
	inner, err := client.UnwrapBaseRoleData(data)
	if err != nil {
		return err
	}
	return printJSON(inner)
}

// loadBitableJSONBody 读取 --config/--config-file 为任意 JSON 值。
func loadBitableJSONBody(cmd *cobra.Command) (any, error) {
	configJSON, _ := cmd.Flags().GetString("config")
	configFile, _ := cmd.Flags().GetString("config-file")
	raw, err := loadJSONInput(configJSON, configFile, "config", "config-file", "请求体")
	if err != nil {
		return nil, err
	}
	var body any
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return nil, fmt.Errorf("解析 --config 失败: %w", err)
	}
	return body, nil
}

// ==================== advperm（高级权限） ====================
var bitableAdvpermCmd = &cobra.Command{
	Use:   "advperm",
	Short: "高级权限开关（enable/disable）",
}

var bitableAdvpermEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "启用高级权限",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 官方 base/v3: PUT /bases/{base_token}/advperm/enable?enable=true
		return runBaseV3Simple(cmd, "PUT", func(bt string) string {
			return client.BaseV3Path("bases", bt, "advperm", "enable")
		}, map[string]any{"enable": "true"})
	},
}

var bitableAdvpermDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "禁用高级权限",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBaseV3Simple(cmd, "PUT", func(bt string) string {
			return client.BaseV3Path("bases", bt, "advperm", "enable")
		}, map[string]any{"enable": "false"})
	},
}

// ==================== data-query ====================
var bitableDataQueryCmd = &cobra.Command{
	Use:   "data-query",
	Short: "数据聚合查询（LiteQuery DSL：分组、聚合、筛选、排序）",
	Long: `POST /open-apis/base/v3/bases/{base_token}/data/query

数据查询端点在 base 级别（不需要 --table-id），数据表通过 DSL 的 datasource 指定。
--config/--config-file 传入 LiteQuery DSL（实测可用）:

  {
    "datasource": {"type": "table", "table": {"tableId": "tblxxx"}},   // 或 {"tableName": "销售数据"}
    "dimensions": [{"field_name": "城市", "alias": "dim_city"}],
    "measures":   [{"field_name": "金额", "aggregation": "sum", "alias": "total_amount"}],
    "filters":    {"type": 1, "conjunction": "and",
                   "conditions": [{"field_name": "城市", "operator": "isNot", "value": [""]}]},
    "sort":       [{"field_name": "total_amount", "order": "desc"}],
    "pagination": {"limit": 100},
    "shaper":     {"format": "flat"}
  }

要点:
  - datasource 必填；dimensions 与 measures 至少一个
  - 字段用 field_name（字段名，区分大小写），不是 field_id；alias 只能用英文且全局唯一
  - aggregation: sum|avg|min|max|count|count_all|distinct_count
  - filters 的 operator: is|isNot|contains|doesNotContain|isEmpty|isNotEmpty|isGreater|
    isGreaterEqual|isLess|isLessEqual（与 record list 的 tuple DSL 不同）
  - pagination.limit 最大 5000，不支持 offset；结果在 main_data 数组，每格是 {"value": ...}
  - 高级权限多维表格需要完全访问（FA）权限

示例:
  feishu-cli bitable data-query --base-token bscnxxx --config-file query.json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := loadBitableJSONBody(cmd)
		if err != nil {
			return err
		}
		if err := validateDataQueryDSL(body); err != nil {
			return err
		}
		return runBaseV3WithBody(cmd, "POST", func(bt string) string {
			// 注意：用两段 "data", "query" 而不是 "data/query"，
			// 因为 BaseV3Path 会对每段做 url.PathEscape
			return client.BaseV3Path("bases", bt, "data", "query")
		}, body)
	},
}

// validateDataQueryDSL 本地校验 LiteQuery DSL 的必填结构，给出比服务端 800004006 更直接的提示。
func validateDataQueryDSL(body any) error {
	m, ok := body.(map[string]any)
	if !ok {
		return clierr.Usagef("data-query 的 DSL 必须是 JSON 对象")
	}
	if _, ok := m["datasource"]; !ok {
		return clierr.Usagef(`DSL 缺少 datasource（服务端会报 800004006）；示例: "datasource":{"type":"table","table":{"tableId":"tblxxx"}}，或用 "tableName" 指定表名`)
	}
	_, hasDim := m["dimensions"]
	_, hasMeas := m["measures"]
	if !hasDim && !hasMeas {
		return clierr.Usagef("DSL 至少需要 dimensions 或 measures 之一")
	}
	return nil
}

// ==================== workflow ====================
var bitableWorkflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "工作流管理（list/get/create/update/enable/disable）",
}

var bitableWorkflowListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出工作流（自动翻页）",
	Long: `POST /open-apis/base/v3/bases/{base_token}/workflows/list

默认自动按 page_token 翻页取全部，输出 {"items":[...],"total":N,"has_more":false}。

可选:
  --page-size    每页大小（1-100）
  --page-token   只取指定页（兼容旧用法；还有下一页时 stderr 提示续翻 token）
  --status       enabled / disabled 过滤`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pageSize, _ := cmd.Flags().GetInt("page-size")
		status, _ := cmd.Flags().GetString("status")
		if pageSize < 0 || pageSize > 100 {
			return clierr.Usagef("--page-size 范围 1-100，当前 %d", pageSize)
		}
		params := map[string]any{}
		if status != "" {
			if err := validateEnum(status, "status", []string{"enabled", "disabled"}); err != nil {
				return clierr.Usage(err)
			}
			params["status"] = status
		}
		return runBitablePageTokenList(cmd, func(bt string) bitablePageTokenList {
			return bitablePageTokenList{Method: "POST", Path: client.BaseV3Path("bases", bt, "workflows", "list"), Key: "items", Params: params, PageSize: pageSize}
		})
	},
}

func init() {
	// role
	bitableCmd.AddCommand(bitableRoleCmd)
	roleSubs := []*cobra.Command{
		bitableRoleListCmd, bitableRoleGetCmd, bitableRoleCreateCmd,
		bitableRoleUpdateCmd, bitableRoleDeleteCmd,
	}
	for _, c := range roleSubs {
		bitableRoleCmd.AddCommand(c)
		addBaseTokenFlag(c)
		c.Flags().String("user-access-token", "", "User Access Token")
	}
	bitableRoleGetCmd.Flags().String("role-id", "", "role_id（必填）")
	bitableRoleCreateCmd.Flags().String("config", "", "JSON 请求体")
	bitableRoleCreateCmd.Flags().String("config-file", "", "JSON 请求体文件")
	bitableRoleUpdateCmd.Flags().String("role-id", "", "role_id（必填）")
	bitableRoleUpdateCmd.Flags().String("config", "", "JSON 请求体")
	bitableRoleUpdateCmd.Flags().String("config-file", "", "JSON 请求体文件")
	bitableRoleDeleteCmd.Flags().String("role-id", "", "role_id（必填）")

	// advperm
	bitableCmd.AddCommand(bitableAdvpermCmd)
	bitableAdvpermCmd.AddCommand(bitableAdvpermEnableCmd)
	addBaseTokenFlag(bitableAdvpermEnableCmd)
	bitableAdvpermEnableCmd.Flags().String("user-access-token", "", "User Access Token")

	bitableAdvpermCmd.AddCommand(bitableAdvpermDisableCmd)
	addBaseTokenFlag(bitableAdvpermDisableCmd)
	bitableAdvpermDisableCmd.Flags().String("user-access-token", "", "User Access Token")

	// data-query（官方 base/v3 端点在 base 级，无 table-id）
	bitableCmd.AddCommand(bitableDataQueryCmd)
	addBaseTokenFlag(bitableDataQueryCmd)
	bitableDataQueryCmd.Flags().String("config", "", "LiteQuery DSL JSON（与 --config-file 二选一）")
	bitableDataQueryCmd.Flags().String("config-file", "", "LiteQuery DSL JSON 文件")
	bitableDataQueryCmd.Flags().String("user-access-token", "", "User Access Token")

	// workflow
	bitableCmd.AddCommand(bitableWorkflowCmd)
	bitableWorkflowCmd.AddCommand(bitableWorkflowListCmd)
	bitableWorkflowListCmd.Flags().Int("page-size", 0, "分页大小")
	bitableWorkflowListCmd.Flags().String("page-token", "", "分页 token")
	bitableWorkflowListCmd.Flags().String("status", "", "过滤状态: enabled/disabled")
	addBaseTokenFlag(bitableWorkflowListCmd)
	bitableWorkflowListCmd.Flags().String("user-access-token", "", "User Access Token")
}
