package cmd

import (
	"github.com/spf13/cobra"
)

var msgCmd = &cobra.Command{
	Use:   "msg",
	Short: "消息操作命令",
	Long: `消息操作命令，用于向用户或群组发送、管理消息。

子命令:
  send               发送消息
  urgent             发送加急消息
  reply              回复消息
  delete             删除消息
  list               获取消息列表
  get                获取消息详情
  mget               批量获取消息详情
  forward            转发消息
  merge-forward      合并转发消息
  read-users         查询消息已读用户
  reaction           表情回复管理（add/remove/list）
  pin                置顶消息
  unpin              取消置顶消息
  pins               获取群内置顶消息列表
  resource-download  下载消息中的资源文件
  thread-messages    获取话题/线程中的消息列表
  flag               消息书签（create/list/cancel）

接收者类型:
  email     邮箱
  open_id   Open ID
  user_id   用户 ID
  union_id  Union ID
  chat_id   群组 ID

消息类型:
  text         文本消息
  post         富文本消息
  interactive  卡片消息
  image        图片消息
  file         文件消息

示例:
  # 发送文本消息
  feishu-cli msg send \
    --receive-id-type email \
    --receive-id user@example.com \
    --text "你好，这是一条测试消息"

  # 直接发送本地文件（自动上传）
  feishu-cli msg send \
    --receive-id-type chat_id \
    --receive-id oc_xxx \
    --file /path/to/report.pdf

  # 直接发送本地图片（自动上传）
  feishu-cli msg send \
    --receive-id-type chat_id \
    --receive-id oc_xxx \
    --image /path/to/screenshot.png

  # 回复消息
  feishu-cli msg reply om_xxx --text "收到！"

  # 发送应用内加急（默认 app）
  feishu-cli msg urgent om_xxx --user-id-type open_id --user-ids ou_xxx,ou_yyy

  # 获取消息详情
  feishu-cli msg get om_xxx

  # 获取会话消息列表
  feishu-cli msg list --container-id oc_xxx

  # 转发消息
  feishu-cli msg forward om_xxx --receive-id user@example.com --receive-id-type email

  # 合并转发消息
  feishu-cli msg merge-forward --receive-id user@example.com --message-ids om_xxx,om_yyy

  # 删除消息
  feishu-cli msg delete om_xxx

  # 查询消息已读用户
  feishu-cli msg read-users om_xxx

  # 表情回复
  feishu-cli msg reaction add om_xxx --emoji-type THUMBSUP
  feishu-cli msg reaction list om_xxx

  # 置顶消息
  feishu-cli msg pin om_xxx
  feishu-cli msg unpin om_xxx
  feishu-cli msg pins --chat-id oc_xxx`,
}

func init() {
	rootCmd.AddCommand(msgCmd)
}
