package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var getEventCmd = &cobra.Command{
	Use:   "get-event [calendar_id] <event_id>",
	Short: "获取日程详情",
	Long: `获取指定日程的详细信息。

参数:
  calendar_id   日历 ID（可省略，省略时使用 primary 主日历）
  event_id      日程 ID（也接受重复日程的实例 ID）
  --share-link  额外获取日程分享链接（events/share_info）。分享日程给他人/群/文档请用它，
                不要用 app_link（app_link 带查看者本人的 calendarId，只能本人打开）

输出字段（-o json）:
  start_time / end_time  普通日程为 RFC3339；全天日程为 YYYY-MM-DD 且 is_all_day=true，
                         end_time 已换算为包含端日期（服务端 end.date 是排他的次日）
  self_rsvp_status       当前身份的答复状态（accept / decline / tentative / needs_action）
  vchat.meeting_url      视频会议链接
  reminders              提醒（开始前 N 分钟）
  recurrence / is_exception / recurring_event_id  重复日程信息

日程状态（status）:
  tentative     暂定
  confirmed     确认
  cancelled     取消

可见性（visibility）:
  default       默认
  public        公开
  private       私密

示例:
  # 获取日程详情
  feishu-cli calendar get-event CAL_ID EVENT_ID

  # 主日历上的日程可只传 event_id，并获取分享链接
  feishu-cli calendar get-event EVENT_ID --share-link

  # JSON 格式输出
  feishu-cli calendar get-event CAL_ID EVENT_ID --output json`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		calendarID, eventID, err := calendarEventArgs(args)
		if err != nil {
			return err
		}
		token := resolveOptionalUserTokenWithFallback(cmd)
		output, _ := cmd.Flags().GetString("output")
		withShare, _ := cmd.Flags().GetBool("share-link")

		event, err := client.GetEvent(calendarID, eventID, token)
		if err != nil {
			return err
		}
		if withShare {
			link, err := client.GetEventShareLink(calendarID, eventID, token)
			if err != nil {
				return err
			}
			event.ShareLink = link
		}

		if output == "json" {
			if err := printJSON(event); err != nil {
				return err
			}
		} else {
			fmt.Println("日程详情:")
			fmt.Printf("  日程 ID:     %s\n", event.EventID)
			fmt.Printf("  标题:        %s\n", event.Summary)
			if event.IsAllDay {
				fmt.Printf("  时间:        全天日程 %s ~ %s\n", event.StartTime, event.EndTime)
			} else {
				fmt.Printf("  开始时间:    %s\n", event.StartTime)
				fmt.Printf("  结束时间:    %s\n", event.EndTime)
			}
			if event.TimeZone != "" && !event.IsAllDay {
				fmt.Printf("  时区:        %s\n", event.TimeZone)
			}
			if event.Description != "" {
				fmt.Printf("  描述:        %s\n", event.Description)
			}
			if event.Location != "" {
				fmt.Printf("  地点:        %s\n", event.Location)
			}
			if event.Status != "" {
				fmt.Printf("  状态:        %s\n", event.Status)
			}
			if event.Visibility != "" {
				fmt.Printf("  可见性:      %s\n", event.Visibility)
			}
			if event.OrganizerID != "" {
				fmt.Printf("  组织者日历:  %s\n", event.OrganizerID)
			}
			if event.CreateTime != "" {
				fmt.Printf("  创建时间:    %s\n", event.CreateTime)
			}
			if event.RecurringID != "" {
				fmt.Printf("  重复日程 ID: %s\n", event.RecurringID)
			}
			if event.Recurrence != "" {
				fmt.Printf("  重复规则:    %s\n", event.Recurrence)
			}
			if event.IsException {
				fmt.Printf("  是否例外:    是\n")
			}
			if event.SelfRSVPStatus != "" {
				fmt.Printf("  我的答复:    %s\n", event.SelfRSVPStatus)
			}
			if event.FreeBusyStatus != "" {
				fmt.Printf("  忙闲:        %s\n", event.FreeBusyStatus)
			}
			if event.Vchat != nil && event.Vchat.MeetingURL != "" {
				fmt.Printf("  会议链接:    %s\n", event.Vchat.MeetingURL)
			}
			if len(event.Reminders) > 0 {
				parts := make([]string, 0, len(event.Reminders))
				for _, r := range event.Reminders {
					parts = append(parts, fmt.Sprintf("%d 分钟", r.Minutes))
				}
				fmt.Printf("  提醒:        开始前 %s\n", strings.Join(parts, " / "))
			}
			if event.ShareLink != "" {
				fmt.Printf("  分享链接:    %s\n", event.ShareLink)
			}
		}

		return nil
	},
}

func init() {
	calendarCmd.AddCommand(getEventCmd)
	getEventCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	getEventCmd.Flags().Bool("share-link", false, "额外获取日程分享链接（用于分享给他人/群/文档）")
	getEventCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
