package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var calendarAttendeeCmd = &cobra.Command{
	Use:   "attendee",
	Short: "日程参与人管理",
	Long: `管理日程参与人，支持添加、移除和列出参与人（用户 / 群 / 会议室 / 外部邮箱）。

子命令:
  add     添加参与人
  remove  移除参与人
  list    列出参与人

示例:
  feishu-cli calendar attendee add CAL_ID EVENT_ID --user-ids ou_xxx,ou_yyy --room-ids omm_xxx
  feishu-cli calendar attendee remove CAL_ID EVENT_ID --attendee-ids ou_xxx
  feishu-cli calendar attendee list CAL_ID EVENT_ID --type user`,
}

// collectAttendeeRefs 汇总 --user-ids / --chat-ids / --room-ids / --attendee-ids
func collectAttendeeRefs(cmd *cobra.Command, allowAttendeeID bool) ([]*client.AttendeeRef, error) {
	var ids []string
	for _, f := range []struct{ name, prefix string }{{"user-ids", "ou_"}, {"chat-ids", "oc_"}, {"room-ids", "omm_"}} {
		raw, _ := cmd.Flags().GetString(f.name)
		for _, id := range splitAndTrim(raw) {
			if !strings.HasPrefix(id, f.prefix) {
				return nil, clierr.Usagef("--%s 只接受 %s 开头的 ID，得到 %q", f.name, f.prefix, id)
			}
			ids = append(ids, id)
		}
	}
	raw, _ := cmd.Flags().GetString("attendee-ids")
	ids = append(ids, splitAndTrim(raw)...)
	refs, err := client.ParseAttendeeRefs(ids, allowAttendeeID)
	if err != nil {
		return nil, clierr.Usage(err)
	}
	if len(refs) == 0 {
		return nil, clierr.Usagef("至少需要指定 --user-ids / --chat-ids / --room-ids / --attendee-ids 之一")
	}
	return refs, nil
}

var calendarAttendeeAddCmd = &cobra.Command{
	Use:   "add <calendar_id> <event_id>",
	Short: "添加日程参与人",
	Long: `向日程添加参与人（用户、群、会议室、外部邮箱）。

参数:
  calendar_id       日历 ID（位置参数）
  event_id          日程 ID（位置参数）
  --user-ids        用户 open_id 列表（ou_），逗号分隔
  --chat-ids        群 ID 列表（oc_），逗号分隔
  --room-ids        会议室 ID 列表（omm_），逗号分隔；可先用 calendar room-find 查空闲会议室
  --attendee-ids    混合列表，按前缀识别：ou_ 用户 / oc_ 群 / omm_ 会议室 / 含 @ 为外部邮箱
  --as              身份：auto（默认）| user | bot

示例:
  feishu-cli calendar attendee add CAL_xxx EVENT_xxx --user-ids ou_aaa,ou_bbb
  feishu-cli calendar attendee add CAL_xxx EVENT_xxx --chat-ids oc_xxx --room-ids omm_xxx`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		refs, err := collectAttendeeRefs(cmd, false)
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

		attendees := client.AttendeeRefsToEventAttendees(refs)
		if err := client.AddEventAttendees(args[0], args[1], attendees, token); err != nil {
			return err
		}

		fmt.Printf("成功添加 %d 个参与人\n", len(attendees))
		return nil
	},
}

var calendarAttendeeRemoveCmd = &cobra.Command{
	Use:   "remove <calendar_id> <event_id>",
	Short: "移除日程参与人",
	Long: `从日程移除参与人（POST .../attendees/batch_delete）。

参数:
  calendar_id       日历 ID（位置参数）
  event_id          日程 ID（位置参数）
  --user-ids / --chat-ids / --room-ids   按类型指定要移除的 ID
  --attendee-ids    混合列表：ou_ / oc_ / omm_ / 邮箱按前缀识别；其余视为 attendee list 返回的 attendee_id
  --notify          是否通知被移除的参与人（默认 true）
  --dry-run         只预览，不执行
  --as              身份：auto（默认）| user | bot

示例:
  feishu-cli calendar attendee remove CAL_xxx EVENT_xxx --user-ids ou_aaa
  feishu-cli calendar attendee remove CAL_xxx EVENT_xxx --room-ids omm_xxx --notify=false`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		refs, err := collectAttendeeRefs(cmd, true)
		if err != nil {
			return err
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}
		notify, _ := cmd.Flags().GetBool("notify")
		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			// 与真实请求共用同一构造函数，预览即实际请求体
			body, err := client.BuildRemoveEventAttendeesBody(refs, notify)
			if err != nil {
				return clierr.Usage(err)
			}
			return printDryRunPlan(cmd, "attendee remove 预览（未执行）", nil, []dryRunStep{{
				Method: "POST",
				URL:    client.RemoveEventAttendeesPath(args[0], args[1]),
				Desc:   attendeeIDFallbackNote(refs),
				Params: client.RemoveEventAttendeesParams(),
				Body:   body,
			}})
		}
		if err := config.Validate(); err != nil {
			return err
		}
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		if err := client.RemoveEventAttendees(args[0], args[1], refs, notify, token); err != nil {
			return err
		}
		fmt.Printf("成功移除 %d 个参与人\n", len(refs))
		return nil
	},
}

// attendeeIDFallbackNote 说明哪些 ID 未匹配 ou_/oc_/omm_/邮箱前缀、将按 attendee_id 处理（dry-run 提示用）。
func attendeeIDFallbackNote(refs []*client.AttendeeRef) string {
	var ids []string
	for _, r := range refs {
		if r != nil && r.Type == "" && r.AttendeeID != "" {
			ids = append(ids, r.AttendeeID)
		}
	}
	if len(ids) == 0 {
		return ""
	}
	return fmt.Sprintf("以下 ID 未匹配 ou_/oc_/omm_/邮箱前缀，按 attendee_id（calendar attendee list 返回）放入 attendee_ids: %s", strings.Join(ids, ", "))
}

var calendarAttendeeListCmd = &cobra.Command{
	Use:   "list <calendar_id> <event_id>",
	Short: "列出日程参与人",
	Long: `列出日程的参与人。

参数:
  calendar_id     日历 ID（位置参数）
  event_id        日程 ID（位置参数）
  --type          只看某类参与人：user | chat | resource（会议室）| third_party，可逗号分隔多选
  --page-all      自动翻完所有页（上限 50 页）

说明:
  群参与人（type=chat）不输出 rsvp_status：服务端对群条目恒返回 needs_action，没有意义，
  群成员各自的答复需要单独查询。

示例:
  feishu-cli calendar attendee list CAL_xxx EVENT_xxx
  feishu-cli calendar attendee list CAL_xxx EVENT_xxx --type resource -o json`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		typeFilter := map[string]bool{}
		typesRaw, _ := cmd.Flags().GetString("type")
		for _, t := range splitAndTrim(typesRaw) {
			if err := validateEnum(t, "--type", []string{"user", "chat", "resource", "third_party"}); err != nil {
				return err
			}
			typeFilter[t] = true
		}
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserTokenWithFallback(cmd)

		calendarID := args[0]
		eventID := args[1]
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		pageAll, _ := cmd.Flags().GetBool("page-all")
		output, _ := cmd.Flags().GetString("output")

		var attendees []*client.EventAttendee
		var nextPageToken string
		var hasMore bool
		for page := 0; ; page++ {
			items, next, more, err := client.ListEventAttendees(calendarID, eventID, pageSize, pageToken, token)
			if err != nil {
				return err
			}
			attendees = append(attendees, items...)
			nextPageToken, hasMore = next, more
			if !pageAll || !more || next == "" || next == pageToken || page >= 49 {
				break
			}
			pageToken = next
		}
		if len(typeFilter) > 0 {
			filtered := attendees[:0]
			for _, a := range attendees {
				if typeFilter[a.Type] {
					filtered = append(filtered, a)
				}
			}
			attendees = filtered
		}
		if hasMore {
			fmt.Fprintf(cmdErrOut(), "提示：还有更多参与人，使用 --page-token %s 或 --page-all 继续\n", nextPageToken)
		}

		if output == "json" {
			if attendees == nil {
				attendees = []*client.EventAttendee{}
			}
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
			if a.RsvpStatus != "" {
				fmt.Printf("    响应状态:    %s\n", a.RsvpStatus)
			}
			if a.UserID != "" {
				fmt.Printf("    用户 ID:     %s\n", a.UserID)
			}
			if a.ChatID != "" {
				fmt.Printf("    群 ID:       %s\n", a.ChatID)
			}
			if a.RoomID != "" {
				fmt.Printf("    会议室 ID:   %s\n", a.RoomID)
			}
			if a.ThirdPartyEmail != "" {
				fmt.Printf("    邮箱:        %s\n", a.ThirdPartyEmail)
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
	calendarAttendeeAddCmd.Flags().String("user-ids", "", "用户 open_id 列表（ou_），逗号分隔")
	calendarAttendeeAddCmd.Flags().String("chat-ids", "", "群 ID 列表（oc_），逗号分隔")
	calendarAttendeeAddCmd.Flags().String("room-ids", "", "会议室 ID 列表（omm_），逗号分隔")
	calendarAttendeeAddCmd.Flags().String("attendee-ids", "", "混合 ID 列表，按前缀识别（ou_/oc_/omm_/邮箱）")
	calendarAttendeeAddCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	addWriteAsFlag(calendarAttendeeAddCmd)

	calendarAttendeeCmd.AddCommand(calendarAttendeeRemoveCmd)
	calendarAttendeeRemoveCmd.Flags().String("user-ids", "", "用户 open_id 列表（ou_），逗号分隔")
	calendarAttendeeRemoveCmd.Flags().String("chat-ids", "", "群 ID 列表（oc_），逗号分隔")
	calendarAttendeeRemoveCmd.Flags().String("room-ids", "", "会议室 ID 列表（omm_），逗号分隔")
	calendarAttendeeRemoveCmd.Flags().String("attendee-ids", "", "混合 ID 列表（ou_/oc_/omm_/邮箱/attendee_id）")
	calendarAttendeeRemoveCmd.Flags().Bool("notify", true, "是否通知被移除的参与人")
	calendarAttendeeRemoveCmd.Flags().Bool("dry-run", false, "只预览将发出的请求，不执行")
	calendarAttendeeRemoveCmd.Flags().StringP("output", "o", "", "输出格式（json，仅 dry-run 预览）")
	calendarAttendeeRemoveCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	addWriteAsFlag(calendarAttendeeRemoveCmd)

	calendarAttendeeCmd.AddCommand(calendarAttendeeListCmd)
	calendarAttendeeListCmd.Flags().Int("page-size", 0, "每页数量")
	calendarAttendeeListCmd.Flags().String("page-token", "", "分页标记")
	calendarAttendeeListCmd.Flags().Bool("page-all", false, "自动翻完所有页（上限 50 页）")
	calendarAttendeeListCmd.Flags().String("type", "", "只看某类参与人：user | chat | resource | third_party（逗号分隔多选）")
	calendarAttendeeListCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	calendarAttendeeListCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
