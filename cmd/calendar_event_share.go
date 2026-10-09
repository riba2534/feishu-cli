package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var calendarEventShareCmd = &cobra.Command{
	Use:   "event-share [calendar_id] <event_id>",
	Short: "获取日程分享链接",
	Long: `获取日程分享链接（POST /calendars/{calendar_id}/events/{event_id}/share_info）。

把日程分享给某人、某个群或粘贴到文档，需要的是这个分享链接；get-event 的 app_link
带查看者本人的 calendarId，只能本人打开，不要拿来分享或自行拼接。

参数:
  calendar_id   日历 ID（可省略，省略时使用 primary 主日历）
  event_id      日程 ID
  --as          身份：auto（默认）| user | bot
  --output, -o  输出格式（json）

示例:
  feishu-cli calendar event-share EVENT_ID
  feishu-cli calendar event-share CAL_ID EVENT_ID -o json`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		calendarID, eventID, err := calendarEventArgs(args)
		if err != nil {
			return err
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}
		if err := config.Validate(); err != nil {
			return err
		}
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		link, err := client.GetEventShareLink(calendarID, eventID, token)
		if err != nil {
			return err
		}
		if output, _ := cmd.Flags().GetString("output"); output == "json" {
			return printJSON(map[string]any{
				"calendar_id": calendarID,
				"event_id":    eventID,
				"share_link":  link,
			})
		}
		fmt.Println(link)
		return nil
	},
}

func init() {
	calendarCmd.AddCommand(calendarEventShareCmd)
	calendarEventShareCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	calendarEventShareCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	addWriteAsFlag(calendarEventShareCmd)
}
