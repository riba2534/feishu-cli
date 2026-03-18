package cmd

import (
	"github.com/spf13/cobra"
)

var permCmd = &cobra.Command{
	Use:   "perm",
	Short: "权限操作命令",
	Long: `权限操作命令，用于管理文档的协作者权限。

子命令:
  add              添加协作者权限
  batch-add        批量添加协作者权限
  list             查看协作者列表
  update           更新协作者权限
  delete           删除协作者权限
  transfer-owner   转移文档所有权
  auth             判断当前用户对文档的权限
  public-get       获取公共权限设置
  public-update    更新公共权限设置
  password         文档密码管理（create/update/delete）

权限级别:
  view         查看权限
  edit         编辑权限
  full_access  完全访问权限

成员类型（也接受 open_id/user_id/union_id/chat_id 等 IM API 风格的别名）:
  email             邮箱
  openid/open_id    Open ID
  userid/user_id    用户 ID
  unionid/union_id  Union ID
  openchat/chat_id  群组 ID
  opendepartmentid  部门 ID

示例:
  # 查看文档的协作者列表
  feishu-cli perm list DOC_TOKEN

  # 通过邮箱添加编辑权限
  feishu-cli perm add DOC_TOKEN \
    --member-type email \
    --member-id user@example.com \
    --perm edit

  # 批量添加协作者
  feishu-cli perm batch-add DOC_TOKEN --members-file members.json

  # 删除协作者
  feishu-cli perm delete DOC_TOKEN \
    --member-type email \
    --member-id user@example.com

  # 获取公共权限设置
  feishu-cli perm public-get DOC_TOKEN

  # 更新公共权限设置
  feishu-cli perm public-update DOC_TOKEN --link-share-entity tenant_readable

  # 创建文档密码
  feishu-cli perm password create DOC_TOKEN

  # 判断当前用户权限
  feishu-cli perm auth DOC_TOKEN --action view`,
}

func init() {
	rootCmd.AddCommand(permCmd)
}
