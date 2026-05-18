package cmd

import (
	"github.com/spf13/cobra"
)

var calendarCmd = &cobra.Command{
	Use:     "calendar",
	Aliases: []string{"cal"},
	Short:   "日历操作命令",
	Long: `日历操作命令，包括列出日历、管理日程、智能时段建议、会议室查找、RSVP 答复等。

子命令:
  list          列出日历
  create-event  创建日程
  get-event     获取日程详情
  list-events   列出日程
  update-event  更新日程
  delete-event  删除日程
  suggestion    智能时段建议（基于参与者 freebusy 推荐可用时段）
  room-find     查找可用会议室（按城市/楼层/容量/时段过滤，支持多时段并发）
  rsvp          答复日程邀请（accept / tentative / decline）

时间格式:
  使用 RFC3339 格式，例如：2024-01-21T14:00:00+08:00

示例:
  # 列出所有日历
  feishu-cli calendar list

  # 创建日程
  feishu-cli calendar create-event --calendar-id CAL_ID --summary "会议" \
    --start 2024-01-21T14:00:00+08:00 --end 2024-01-21T15:00:00+08:00

  # 列出日程
  feishu-cli calendar list-events CAL_ID

  # 获取日程详情
  feishu-cli calendar get-event CAL_ID EVENT_ID

  # 更新日程
  feishu-cli calendar update-event CAL_ID EVENT_ID --summary "新标题"

  # 删除日程
  feishu-cli calendar delete-event CAL_ID EVENT_ID

  # 智能时段建议（推荐 60 分钟可用时段）
  feishu-cli calendar suggestion --search-start 2024-01-21T09:00:00+08:00 \
    --search-end 2024-01-21T18:00:00+08:00 --duration 60 \
    --attendee-ids ou_xxx,ou_yyy,oc_zzz

  # 查找可用会议室
  feishu-cli calendar room-find --city 北京 --min-capacity 6 \
    --slot 2024-01-21T14:00:00+08:00/2024-01-21T15:00:00+08:00

  # 答复日程邀请
  feishu-cli calendar rsvp --event-id EVENT_ID --rsvp-status accept`,
}

func init() {
	rootCmd.AddCommand(calendarCmd)
}
