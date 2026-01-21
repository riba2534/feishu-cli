package cmd

import (
	"github.com/spf13/cobra"
)

var permCmd = &cobra.Command{
	Use:   "perm",
	Short: "权限操作命令",
	Long: `权限操作命令，用于管理文档的协作者权限。

子命令:
  add      添加协作者权限
  update   更新协作者权限

权限级别:
  view         查看权限
  edit         编辑权限
  full_access  完全访问权限

成员类型:
  email             邮箱
  openid            Open ID
  userid            用户 ID
  unionid           Union ID
  openchat          群组 ID
  opendepartmentid  部门 ID

示例:
  # 通过邮箱添加编辑权限
  feishu-cli perm add <document_id> \
    --doc-type docx \
    --member-type email \
    --member-id user@example.com \
    --perm edit

  # 添加完全访问权限并发送通知
  feishu-cli perm add <document_id> \
    --doc-type docx \
    --member-type email \
    --member-id user@example.com \
    --perm full_access \
    --notification

  # 更新用户权限
  feishu-cli perm update <document_id> \
    --doc-type docx \
    --member-type email \
    --member-id user@example.com \
    --perm edit`,
}

func init() {
	rootCmd.AddCommand(permCmd)
}
