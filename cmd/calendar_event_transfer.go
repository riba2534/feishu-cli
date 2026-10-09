package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var calendarEventTransferCmd = &cobra.Command{
	Use:   "event-transfer [calendar_id] <event_id>",
	Short: "转让日程组织者",
	Long: `把日程的组织者身份转让给另一个用户或机器人（POST .../events/{event_id}/transfer）。

转让不可撤销，会议纪要、附件等也会一并转给新组织者；需要确认（非交互环境加 --yes）。
重复日程会转让整个序列（接口不支持只转让某一次），此时必须额外传 --transfer-series 表示知情。

参数:
  calendar_id                  日历 ID（可省略，默认 primary；日程在共享日历上时须传该日历 ID）
  event_id                     日程 ID
  --to-user-id                 新组织者 open_id（ou_ 开头，可以是用户或机器人，必填）
  --remove-original-organizer  转让后把原组织者从参与人中移除（默认保留为参与人；共享日历上服务端强制移除）
  --transfer-series            确认转让整个重复序列（重复日程必填）
  --dry-run                    只预览，不执行
  --as                         身份：auto（默认）| user | bot，必须是日程当前组织者

示例:
  feishu-cli calendar event-transfer EVENT_ID --to-user-id ou_xxx --yes
  feishu-cli calendar event-transfer CAL_ID EVENT_ID --to-user-id ou_xxx --transfer-series --yes`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		calendarID, eventID, err := calendarEventArgs(args)
		if err != nil {
			return err
		}
		toUserID, _ := cmd.Flags().GetString("to-user-id")
		toUserID = strings.TrimSpace(toUserID)
		removeOriginal, _ := cmd.Flags().GetBool("remove-original-organizer")
		series, _ := cmd.Flags().GetBool("transfer-series")
		if !strings.HasPrefix(toUserID, "ou_") || toUserID == "ou_" {
			return clierr.Usagef("--to-user-id 需要 open_id（ou_ 开头），得到 %q", toUserID)
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}
		body := map[string]any{"to_user_id": toUserID, "need_remove_original_organizer": removeOriginal}
		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			steps := []dryRunStep{}
			if !series {
				steps = append(steps, dryRunStep{Method: "GET", URL: "/open-apis/calendar/v4/calendars/" + calendarID + "/events/" + eventID, Desc: "读取日程；重复日程未带 --transfer-series 时拒绝执行"})
			}
			steps = append(steps, dryRunStep{
				Method: "POST",
				URL:    "/open-apis/calendar/v4/calendars/" + calendarID + "/events/" + eventID + "/transfer",
				Params: map[string]any{"user_id_type": "open_id"},
				Body:   body,
				Desc:   "转让组织者",
			})
			return printDryRunPlan(cmd, "event-transfer 预览（未执行）", nil, steps)
		}

		if err := config.Validate(); err != nil {
			return err
		}
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		if !series {
			ev, err := client.GetEvent(calendarID, eventID, token)
			if err != nil {
				return fmt.Errorf("%w\n（读取日程以判断是否重复日程失败；确认要转让整个序列可加 --transfer-series 跳过检查）", err)
			}
			if kind := client.ClassifyEvent(ev); kind != client.RecurringKindNormal {
				return clierr.Usagef("这是重复日程（%s），转让会作用于整个序列（所有实例与例外），接口无法只转让某一次；确认后加 --transfer-series 重新执行", client.RecurringKindLabel(kind))
			}
		}
		if err := confirmDangerousAction(cmd, fmt.Sprintf("将把日程 %s 的组织者转让给 %s（不可撤销）", eventID, toUserID)); err != nil {
			return err
		}
		if err := client.TransferEvent(calendarID, eventID, toUserID, removeOriginal, token); err != nil {
			return err
		}
		result := map[string]any{
			"calendar_id":      calendarID,
			"event_id":         eventID,
			"new_organizer_id": toUserID,
		}
		if removeOriginal || calendarID == "primary" {
			result["original_organizer_removed"] = removeOriginal
		} else {
			fmt.Fprintln(cmdErrOut(), "提示：日程若在共享日历上，服务端会强制移除原组织者；主日历上原组织者保留为参与人")
		}
		if output, _ := cmd.Flags().GetString("output"); output == "json" {
			return printJSON(result)
		}
		fmt.Printf("日程组织者已转让给 %s（日程 ID: %s）\n", toUserID, eventID)
		return nil
	},
}

func init() {
	calendarCmd.AddCommand(calendarEventTransferCmd)
	calendarEventTransferCmd.Flags().String("to-user-id", "", "新组织者 open_id（ou_ 开头，必填）")
	calendarEventTransferCmd.Flags().Bool("remove-original-organizer", false, "转让后移除原组织者（默认保留为参与人）")
	calendarEventTransferCmd.Flags().Bool("transfer-series", false, "确认转让整个重复序列（重复日程必填）")
	calendarEventTransferCmd.Flags().Bool("dry-run", false, "只预览将发出的请求，不执行")
	calendarEventTransferCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	calendarEventTransferCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	addWriteAsFlag(calendarEventTransferCmd)
	mustMarkFlagRequired(calendarEventTransferCmd, "to-user-id")
}
