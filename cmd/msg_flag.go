package cmd

import "github.com/spf13/cobra"

// msgFlagCmd 是 msg flag 子命令组的入口，挂载在 msg 之下。
// 飞书消息书签分两层：
//   - message 层（item_type=default, flag_type=message）：消息本身被收藏，最常见用法
//   - feed   层（item_type=thread|msg_thread, flag_type=feed）：会话/线程在侧边栏置顶
var msgFlagCmd = &cobra.Command{
	Use:   "flag",
	Short: "消息书签（收藏/取消收藏/列表）",
	Long: `消息书签，对应飞书 OpenAPI /im/v1/flags。

子命令:
  create   为消息创建书签
  list     列出当前用户的书签
  cancel   取消（删除）书签

权限要求:
  必须使用 User Access Token；list 需要 im:feed.flag:read，create/cancel 需要 im:feed.flag:write

支持的 item_type × flag_type 组合:
  default     + message  消息层（最常见）
  thread      + feed     话题群（topic-style）feed 层
  msg_thread  + feed     普通群消息线程 feed 层

示例:
  feishu-cli msg flag create om_xxx
  feishu-cli msg flag list --page-size 20
  feishu-cli msg flag cancel om_xxx
  feishu-cli msg flag cancel om_xxx --item-type thread --flag-type feed`,
}

func init() {
	msgCmd.AddCommand(msgFlagCmd)
}
