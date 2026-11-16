package cmd

import (
	"github.com/spf13/cobra"
)

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "群聊管理命令",
	Long: `群聊管理命令，用于创建、查询、更新、解散群聊及管理群成员。

子命令:
  create    创建群聊
  create-sync  从双人私聊原生建群并同步消息（实验性）
  get       获取群聊信息
  update    更新群聊信息
  delete    解散群聊
  link      获取群分享链接
  member    群成员管理

示例:
  # 创建群聊
  feishu-cli chat create --name "测试群"

  # 获取群聊信息
  feishu-cli chat get oc_xxx

  # 更新群聊名称
  feishu-cli chat update oc_xxx --name "新群名"

  # 解散群聊
  feishu-cli chat delete oc_xxx

  # 获取群分享链接
  feishu-cli chat link oc_xxx

  # 群成员管理
  feishu-cli chat member list oc_xxx
  feishu-cli chat member add oc_xxx --id-list ou_xxx,ou_yyy
  feishu-cli chat member remove oc_xxx --id-list ou_xxx`,
}

func init() {
	rootCmd.AddCommand(chatCmd)
}
