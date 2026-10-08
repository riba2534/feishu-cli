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
  list            列出日历
  primary         获取当前身份的主日历
  agenda          查看日程（展开重复日程为实例）
  event-search    搜索日程
  create-event    创建日程（可带参与人/会议室/视频会议）
  get-event       获取日程详情
  list-events     列出日程
  update-event    更新日程（重复日程支持 --apply-to）
  delete-event    删除日程（重复日程支持 --apply-to）
  event-share     获取日程分享链接
  event-transfer  转让日程组织者
  attendee        参与人管理（add / remove / list）
  freebusy        查询忙闲（单人/多人/共同空闲）
  suggestion      智能时段建议（基于参与者 freebusy 推荐可用时段）
  room-find       查找可用会议室（按城市/楼层/容量/时段过滤，支持多时段并发）
  rsvp            答复日程邀请（accept / tentative / decline）
  event-reply     答复日程邀请（位置参数版，与 rsvp 等价）

身份:
  读命令优先 User Token；写命令（create/update/delete-event、attendee add/remove、event-transfer、
  event-share）支持 --as，默认 auto：已登录用本人身份，未配置回退 Bot，已配置但不可用时报错。
  rsvp / event-reply 必需 User Token。

时间格式:
  推荐 RFC3339，例如：2024-01-21T14:00:00+08:00

示例:
  # 列出所有日历
  feishu-cli calendar list

  # 在本人主日历创建日程
  feishu-cli calendar create-event --summary "会议" \
    --start 2024-01-21T14:00:00+08:00 --end 2024-01-21T15:00:00+08:00

  # 列出日程
  feishu-cli calendar list-events CAL_ID

  # 获取日程详情
  feishu-cli calendar get-event CAL_ID EVENT_ID

  # 更新日程
  feishu-cli calendar update-event CAL_ID EVENT_ID --summary "新标题"

  # 删除日程（重复日程删整个序列加 --apply-to all --yes）
  feishu-cli calendar delete-event CAL_ID EVENT_ID

  # 智能时段建议（推荐 60 分钟可用时段）
  feishu-cli calendar suggestion --start 2024-01-21T09:00:00+08:00 \
    --end 2024-01-21T18:00:00+08:00 --duration 60 \
    --attendee-ids ou_xxx,ou_yyy,oc_zzz

  # 查找可用会议室
  feishu-cli calendar room-find --city 北京 --min-capacity 6 \
    --slot 2024-01-21T14:00:00+08:00~2024-01-21T15:00:00+08:00

  # 答复日程邀请
  feishu-cli calendar rsvp --event-id EVENT_ID --action accept`,
}

func init() {
	rootCmd.AddCommand(calendarCmd)
}
