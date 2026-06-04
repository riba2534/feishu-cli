package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
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
		return runBaseV3Simple(cmd, "GET", func(bt string) string {
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
		return runBaseV3Simple(cmd, "GET", func(bt string) string {
			return bitableRolePath(bt, roleID)
		}, nil)
	},
}

var bitableRoleCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建角色",
	Long:  `通过 --config/--config-file 传入完整 role 定义`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBaseV3WithJSON(cmd, "POST", func(bt string) string {
			return bitableRolePath(bt)
		})
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
		return runBaseV3WithJSON(cmd, "PUT", func(bt string) string {
			return bitableRolePath(bt, roleID)
		})
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
		return runBaseV3Simple(cmd, "DELETE", func(bt string) string {
			return bitableRolePath(bt, roleID)
		}, nil)
	},
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
	Short: "数据聚合查询（LiteQuery DSL）",
	Long: `POST /open-apis/base/v3/bases/{base_token}/data/query

官方 base/v3 的数据查询端点在 base 级别，不含 table_id。通过 --config 传入
完整的 LiteQuery DSL（dimensions / measures / filters 等）。

示例:
  feishu-cli bitable data-query --base-token bscnxxx --config-file query.json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBaseV3WithJSON(cmd, "POST", func(bt string) string {
			// 注意：用两段 "data", "query" 而不是 "data/query"，
			// 因为 BaseV3Path 会对每段做 url.PathEscape
			return client.BaseV3Path("bases", bt, "data", "query")
		})
	},
}

// ==================== workflow ====================
var bitableWorkflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "工作流管理（list/get/create/update/enable/disable）",
}

var bitableWorkflowListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出工作流",
	Long: `POST /open-apis/base/v3/bases/{base_token}/workflows/list

可选:
  --page-size    分页大小
  --page-token   下一页 token
  --status       enabled / disabled 过滤`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		status, _ := cmd.Flags().GetString("status")

		body := map[string]any{}
		if pageSize > 0 {
			body["page_size"] = pageSize
		}
		if pageToken != "" {
			body["page_token"] = pageToken
		}
		if status != "" {
			body["status"] = status
		}
		return runBaseV3WithBody(cmd, "POST", func(bt string) string {
			return client.BaseV3Path("bases", bt, "workflows", "list")
		}, body)
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
