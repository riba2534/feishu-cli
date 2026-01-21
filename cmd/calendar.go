package cmd

import (
	"github.com/spf13/cobra"
)

var calendarCmd = &cobra.Command{
	Use:     "calendar",
	Aliases: []string{"cal"},
	Short:   "日历操作命令",
	Long: `日历操作命令，包括列出日历、管理日程等。

子命令:
  list          列出日历
  create-event  创建日程
  get-event     获取日程详情
  list-events   列出日程
  update-event  更新日程
  delete-event  删除日程

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
  feishu-cli calendar delete-event CAL_ID EVENT_ID`,
}

func init() {
	rootCmd.AddCommand(calendarCmd)
}
