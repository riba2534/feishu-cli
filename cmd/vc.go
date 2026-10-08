package cmd

import (
	"github.com/spf13/cobra"
)

// vcCmd 视频会议父命令
var vcCmd = &cobra.Command{
	Use:   "vc",
	Short: "视频会议操作命令",
	Long: `视频会议相关操作，包括搜索历史会议、获取会议纪要、查询会议录制等。

读命令（search/detail/recording/notes/note detail/meeting list-active）默认 User 身份，
支持 --as user|bot|auto；note transcript 仅支持 User；bot meeting-join/leave 仅支持 Bot。

子命令:
  search     搜索历史会议记录（支持 query/时间/组织者/参会者/会议室多维过滤）
  detail     聚合查询会议详情（note_id + minute_token）
  notes      获取会议纪要（支持 meeting-ids / minute-tokens / calendar-event-ids 三路径）
  recording  查询会议录制并提取 minute_token
  note       智能纪要详情 / 统一逐字稿
  meeting    进行中的会议（list-active 获取 meeting_id）
  bot        会议机器人入会/离会/会议事件

示例:
  feishu-cli vc search --query "周会" --start 2026-03-01
  feishu-cli vc notes --minute-tokens obcnxxxx --with-artifacts
  feishu-cli vc recording --meeting-ids 69xxxx`,
}

func init() {
	rootCmd.AddCommand(vcCmd)
}
