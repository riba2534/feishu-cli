package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var calendarAttendeeCmd = &cobra.Command{
	Use:   "attendee",
	Short: "日程参与人管理",
	Long: `管理日程参与人，支持添加和列出参与人。

子命令:
  add    添加参与人
  list   列出参与人

示例:
  feishu-cli calendar attendee add CAL_ID EVENT_ID --user-ids ou_xxx,ou_yyy
  feishu-cli calendar attendee list CAL_ID EVENT_ID`,
}

var calendarAttendeeAddCmd = &cobra.Command{
	Use:   "add <calendar_id> <event_id>",
	Short: "添加日程参与人",
	Long: `向日程添加参与人。

参数:
  calendar_id       日历 ID（位置参数）
  event_id          日程 ID（位置参数）
  --user-ids        用户 ID 列表，逗号分隔
  --chat-ids        群 ID 列表，逗号分隔

示例:
  feishu-cli calendar attendee add CAL_xxx EVENT_xxx --user-ids ou_aaa,ou_bbb
  feishu-cli calendar attendee add CAL_xxx EVENT_xxx --chat-ids oc_xxx`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		calendarID := args[0]
		eventID := args[1]
		userIDsStr, _ := cmd.Flags().GetString("user-ids")
		chatIDsStr, _ := cmd.Flags().GetString("chat-ids")

		if userIDsStr == "" && chatIDsStr == "" {
			return fmt.Errorf("至少需要指定 --user-ids 或 --chat-ids")
		}

		var attendees []*client.EventAttendee

		if userIDsStr != "" {
			for _, id := range strings.Split(userIDsStr, ",") {
				id = strings.TrimSpace(id)
				if id != "" {
					attendees = append(attendees, &client.EventAttendee{
						Type:   "user",
						UserID: id,
					})
				}
			}
		}

		if chatIDsStr != "" {
			for _, id := range strings.Split(chatIDsStr, ",") {
				id = strings.TrimSpace(id)
				if id != "" {
					attendees = append(attendees, &client.EventAttendee{
						Type:   "chat",
						ChatID: id,
					})
				}
			}
		}

		if err := client.AddEventAttendees(calendarID, eventID, attendees, token); err != nil {
			return err
		}

		fmt.Printf("成功添加 %d 个参与人\n", len(attendees))
		return nil
	},
}

var calendarAttendeeListCmd = &cobra.Command{
	Use:   "list <calendar_id> <event_id>",
	Short: "列出日程参与人",
	Long: `列出日程的所有参与人。

参数:
  calendar_id     日历 ID（位置参数）
  event_id        日程 ID（位置参数）

示例:
  feishu-cli calendar attendee list CAL_xxx EVENT_xxx
  feishu-cli calendar attendee list CAL_xxx EVENT_xxx -o json`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		calendarID := args[0]
		eventID := args[1]
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		output, _ := cmd.Flags().GetString("output")

		attendees, nextPageToken, hasMore, err := client.ListEventAttendees(calendarID, eventID, pageSize, pageToken, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(map[string]interface{}{
				"attendees":       attendees,
				"next_page_token": nextPageToken,
				"has_more":        hasMore,
			})
		}

		if len(attendees) == 0 {
			fmt.Println("暂无参与人")
			return nil
		}

		fmt.Printf("共 %d 个参与人:\n\n", len(attendees))
		for i, a := range attendees {
			fmt.Printf("[%d] %s\n", i+1, a.DisplayName)
			fmt.Printf("    类型:        %s\n", a.Type)
			fmt.Printf("    响应状态:    %s\n", a.RsvpStatus)
			if a.UserID != "" {
				fmt.Printf("    用户 ID:     %s\n", a.UserID)
			}
			if a.ChatID != "" {
				fmt.Printf("    群 ID:       %s\n", a.ChatID)
			}
			if a.IsOrganizer {
				fmt.Printf("    组织者:      是\n")
			}
			fmt.Println()
		}

		if hasMore {
			fmt.Printf("下一页 token: %s\n", nextPageToken)
		}

		return nil
	},
}

func init() {
	calendarCmd.AddCommand(calendarAttendeeCmd)

	calendarAttendeeCmd.AddCommand(calendarAttendeeAddCmd)
	calendarAttendeeAddCmd.Flags().String("user-ids", "", "用户 ID 列表，逗号分隔")
	calendarAttendeeAddCmd.Flags().String("chat-ids", "", "群 ID 列表，逗号分隔")
	calendarAttendeeAddCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")

	calendarAttendeeCmd.AddCommand(calendarAttendeeListCmd)
	calendarAttendeeListCmd.Flags().Int("page-size", 0, "每页数量")
	calendarAttendeeListCmd.Flags().String("page-token", "", "分页标记")
	calendarAttendeeListCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	calendarAttendeeListCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
