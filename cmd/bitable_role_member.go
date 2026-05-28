package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

// ==================== role member 协作者 ====================
// base/v3 未暴露 roles/{id}/members 子资源，统一走 bitable/v1（app_token 即 base_token）。
var bitableRoleMemberCmd = &cobra.Command{
	Use:   "member",
	Short: "角色协作者管理（list/create/delete/batch-create/batch-delete）",
}

var memberIDTypes = []string{"open_id", "union_id", "user_id", "chat_id", "department_id", "open_department_id"}

func roleMemberPath(appToken, roleID string, extra ...string) string {
	parts := []string{"apps", appToken, "roles", roleID, "members"}
	parts = append(parts, extra...)
	return client.BitableV1Path(parts...)
}

func requireRoleID(cmd *cobra.Command) (string, error) {
	roleID, _ := cmd.Flags().GetString("role-id")
	if roleID == "" {
		return "", fmt.Errorf("--role-id 必填")
	}
	return roleID, nil
}

var bitableRoleMemberListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出角色协作者",
	RunE: func(cmd *cobra.Command, args []string) error {
		roleID, err := requireRoleID(cmd)
		if err != nil {
			return err
		}
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		params := map[string]any{}
		if pageSize > 0 {
			params["page_size"] = pageSize
		}
		if pageToken != "" {
			params["page_token"] = pageToken
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "GET", path: roleMemberPath(bt, roleID), params: params, useV1: true}
		})
	},
}

var bitableRoleMemberCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "新增角色协作者",
	Long: `POST /open-apis/bitable/v1/apps/{app_token}/roles/{role_id}/members

必填:
  --role-id        角色 ID
  --member-id      协作者 ID
可选:
  --member-id-type ID 类型（默认 open_id；open_id|union_id|user_id|chat_id|department_id|open_department_id）`,
	RunE: func(cmd *cobra.Command, args []string) error {
		roleID, err := requireRoleID(cmd)
		if err != nil {
			return err
		}
		memberID, _ := cmd.Flags().GetString("member-id")
		if memberID == "" {
			return fmt.Errorf("--member-id 必填")
		}
		memberIDType, _ := cmd.Flags().GetString("member-id-type")
		params := map[string]any{}
		if memberIDType != "" {
			if err := validateEnum(memberIDType, "member-id-type", memberIDTypes); err != nil {
				return err
			}
			params["member_id_type"] = memberIDType
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "POST", path: roleMemberPath(bt, roleID), params: params, body: map[string]any{"member_id": memberID}, useV1: true}
		})
	},
}

var bitableRoleMemberDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "删除角色协作者",
	RunE: func(cmd *cobra.Command, args []string) error {
		roleID, err := requireRoleID(cmd)
		if err != nil {
			return err
		}
		memberID, _ := cmd.Flags().GetString("member-id")
		if memberID == "" {
			return fmt.Errorf("--member-id 必填")
		}
		memberIDType, _ := cmd.Flags().GetString("member-id-type")
		params := map[string]any{}
		if memberIDType != "" {
			if err := validateEnum(memberIDType, "member-id-type", memberIDTypes); err != nil {
				return err
			}
			params["member_id_type"] = memberIDType
		}
		return bitableRun(cmd, func(bt string) bitableReq {
			return bitableReq{method: "DELETE", path: roleMemberPath(bt, roleID, memberID), params: params, useV1: true}
		})
	},
}

var bitableRoleMemberBatchCreateCmd = &cobra.Command{
	Use:   "batch-create",
	Short: "批量新增角色协作者",
	Long: `POST /open-apis/bitable/v1/apps/{app_token}/roles/{role_id}/members/batch_create

必填:
  --role-id      角色 ID
  --member-ids   协作者 ID 列表（逗号分隔，≤100）
可选:
  --member-id-type  member_list 各项的 type（默认 open_id；与单条命令一致）`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRoleMemberBatch(cmd, "batch_create")
	},
}

var bitableRoleMemberBatchDeleteCmd = &cobra.Command{
	Use:   "batch-delete",
	Short: "批量删除角色协作者",
	Long:  `POST /open-apis/bitable/v1/apps/{app_token}/roles/{role_id}/members/batch_delete`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRoleMemberBatch(cmd, "batch_delete")
	},
}

// runRoleMemberBatch 共享 batch_create / batch_delete 逻辑：从 --member-ids 构造 member_list。
func runRoleMemberBatch(cmd *cobra.Command, action string) error {
	roleID, err := requireRoleID(cmd)
	if err != nil {
		return err
	}
	idsStr, _ := cmd.Flags().GetString("member-ids")
	ids := splitAndTrim(idsStr)
	if len(ids) == 0 {
		return fmt.Errorf("--member-ids 必填（逗号分隔）")
	}
	memberType, _ := cmd.Flags().GetString("member-id-type")
	if memberType == "" {
		memberType = "open_id"
	}
	if err := validateEnum(memberType, "member-id-type", memberIDTypes); err != nil {
		return err
	}
	memberList := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		memberList = append(memberList, map[string]any{"type": memberType, "id": id})
	}
	body := map[string]any{"member_list": memberList}
	return bitableRun(cmd, func(bt string) bitableReq {
		return bitableReq{method: "POST", path: roleMemberPath(bt, roleID, action), body: body, useV1: true}
	})
}

func init() {
	bitableRoleCmd.AddCommand(bitableRoleMemberCmd)

	bitableRoleMemberCmd.AddCommand(bitableRoleMemberListCmd)
	addBitableCommonFlags(bitableRoleMemberListCmd)
	bitableRoleMemberListCmd.Flags().String("role-id", "", "role_id（必填）")
	bitableRoleMemberListCmd.Flags().Int("page-size", 0, "分页大小（≤100）")
	bitableRoleMemberListCmd.Flags().String("page-token", "", "分页 token")

	bitableRoleMemberCmd.AddCommand(bitableRoleMemberCreateCmd)
	addBitableWriteFlags(bitableRoleMemberCreateCmd)
	bitableRoleMemberCreateCmd.Flags().String("role-id", "", "role_id（必填）")
	bitableRoleMemberCreateCmd.Flags().String("member-id", "", "协作者 ID（必填）")
	bitableRoleMemberCreateCmd.Flags().String("member-id-type", "", "ID 类型（默认 open_id）")

	bitableRoleMemberCmd.AddCommand(bitableRoleMemberDeleteCmd)
	addBitableWriteFlags(bitableRoleMemberDeleteCmd)
	bitableRoleMemberDeleteCmd.Flags().String("role-id", "", "role_id（必填）")
	bitableRoleMemberDeleteCmd.Flags().String("member-id", "", "协作者 ID（必填）")
	bitableRoleMemberDeleteCmd.Flags().String("member-id-type", "", "ID 类型（默认 open_id）")

	bitableRoleMemberCmd.AddCommand(bitableRoleMemberBatchCreateCmd)
	addBitableWriteFlags(bitableRoleMemberBatchCreateCmd)
	bitableRoleMemberBatchCreateCmd.Flags().String("role-id", "", "role_id（必填）")
	bitableRoleMemberBatchCreateCmd.Flags().String("member-ids", "", "协作者 ID 列表（逗号分隔，必填）")
	bitableRoleMemberBatchCreateCmd.Flags().String("member-id-type", "", "member_list 各项 type（默认 open_id，与单条命令一致）")

	bitableRoleMemberCmd.AddCommand(bitableRoleMemberBatchDeleteCmd)
	addBitableWriteFlags(bitableRoleMemberBatchDeleteCmd)
	bitableRoleMemberBatchDeleteCmd.Flags().String("role-id", "", "role_id（必填）")
	bitableRoleMemberBatchDeleteCmd.Flags().String("member-ids", "", "协作者 ID 列表（逗号分隔，必填）")
	bitableRoleMemberBatchDeleteCmd.Flags().String("member-id-type", "", "member_list 各项 type（默认 open_id，与单条命令一致）")
}
