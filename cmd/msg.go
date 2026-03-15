package cmd

import (
	"github.com/spf13/cobra"
)

var msgCmd = &cobra.Command{
	Use:   "msg",
	Short: "消息操作命令",
	Long: `消息操作命令，用于向用户或群组发送、管理消息。

子命令:
  send            发送消息
  reply           回复消息
  upload-image    上传图片获取 image_key（支持直接发送）
  delete          删除消息
  list            获取消息列表
  get             获取消息详情
  forward         转发消息
  merge-forward   合并转发消息
  read-users      查询消息已读用户
  reaction        表情回复管理（add/remove/list）
  pin             置顶消息
  unpin           取消置顶消息
  pins            获取群内置顶消息列表

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

  # 回复消息
  feishu-cli msg reply om_xxx --text "收到！"

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
