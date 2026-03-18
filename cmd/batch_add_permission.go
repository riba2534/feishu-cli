package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var batchAddPermissionCmd = &cobra.Command{
	Use:   "batch-add <doc_token>",
	Short: "批量添加协作者权限",
	Long: `批量为文档添加多个协作者权限。

参数:
  doc_token         文档 Token
  --doc-type        文档类型（默认: docx）
  --members-file    成员列表 JSON 文件路径（必填）
  --notification    发送通知给成员

成员列表 JSON 格式:
  [
    {"member_type": "email", "member_id": "user1@example.com", "perm": "edit"},
    {"member_type": "email", "member_id": "user2@example.com", "perm": "view"},
    {"member_type": "openid", "member_id": "ou_xxxxx", "perm": "full_access"}
  ]

权限级别:
  view          查看权限
  edit          编辑权限
  full_access   完全访问权限

示例:
  # 从文件批量添加协作者
  feishu-cli perm batch-add DOC_TOKEN --members-file members.json

  # 批量添加并发送通知
  feishu-cli perm batch-add DOC_TOKEN --members-file members.json --notification`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		docToken := args[0]
		docType, _ := cmd.Flags().GetString("doc-type")
		membersFile, _ := cmd.Flags().GetString("members-file")
		notification, _ := cmd.Flags().GetBool("notification")

		data, err := os.ReadFile(membersFile)
		if err != nil {
			return fmt.Errorf("读取成员列表文件失败: %w", err)
		}

		var members []*client.PermissionMember
		if err := json.Unmarshal(data, &members); err != nil {
			return fmt.Errorf("解析成员列表 JSON 失败: %w", err)
		}

		if len(members) == 0 {
			return fmt.Errorf("成员列表不能为空")
		}

		for i, m := range members {
			m.MemberType = normalizePermMemberType(m.MemberType)
			if m.MemberType == "" {
				return fmt.Errorf("第 %d 个成员的 member_type 不能为空", i+1)
			}
			if m.MemberID == "" {
				return fmt.Errorf("第 %d 个成员的 member_id 不能为空", i+1)
			}
			if m.Perm == "" {
				return fmt.Errorf("第 %d 个成员的 perm 不能为空", i+1)
			}
		}

		if err := client.BatchAddPermission(docToken, docType, members, notification); err != nil {
			return err
		}

		fmt.Printf("批量添加权限成功！\n")
		fmt.Printf("  文档: %s\n", docToken)
		fmt.Printf("  添加成员数: %d\n", len(members))
		return nil
	},
}

func init() {
	permCmd.AddCommand(batchAddPermissionCmd)
	batchAddPermissionCmd.Flags().String("doc-type", "docx", "文档类型（docx/sheet/bitable 等）")
	batchAddPermissionCmd.Flags().String("members-file", "", "成员列表 JSON 文件路径")
	batchAddPermissionCmd.Flags().Bool("notification", false, "发送通知给成员")
	mustMarkFlagRequired(batchAddPermissionCmd, "members-file")
}
