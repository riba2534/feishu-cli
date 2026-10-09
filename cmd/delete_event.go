package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var deleteEventCmd = &cobra.Command{
	Use:   "delete-event [calendar_id] <event_id>",
	Short: "删除日程",
	Long: `删除指定日历中的日程。删除前会先读取日程，判断是普通日程还是重复日程的主体/实例/例外，
并在 stderr（以及 -o json 的 scope 字段）说明实际影响范围。

参数:
  calendar_id   日历 ID（可省略，省略时使用 primary 主日历）
  event_id      日程 ID

可选参数:
  --apply-to    重复日程的删除范围：single | all | this-and-following
  --notify      是否通知参与人（默认 true）
  --dry-run     只预览，不执行（不联网）
  --as          身份：auto（默认）| user | bot
  --output, -o  输出格式（json）

重复日程（实测语义）:
  event_id 形如 {uid}_{原始时间戳}：_0 是重复日程主体（或普通日程），其余是某一次实例。
  calendar agenda / event-search 返回的是**实例 ID**。
  - 不传 --apply-to（服务端原生语义，兼容旧行为）：
      主体 ID → 删除整条序列，但**不级联**已单独修改过的例外（例外仍留在日历上）
      实例/例外 ID → 只删除这一次
  - --apply-to single：只删这一次（实例/例外 ID）
  - --apply-to all：删除整条序列，主体与全部例外一并删除（需确认，非交互环境加 --yes）
  - --apply-to this-and-following：从该实例起删除后续所有实例，原序列截断到前一天
    （需传实例 ID；需确认，非交互环境加 --yes）

注意:
  - 删除操作不可恢复；删除后服务端保留 status=cancelled 的记录，get-event 仍可读到
  - 批量清理例外时不通知参与人，只有主体的删除/截断按 --notify 通知

示例:
  # 删除主日历上的普通日程
  feishu-cli calendar delete-event EVENT_ID

  # 删除指定日历上的日程
  feishu-cli calendar delete-event CAL_ID EVENT_ID

  # 只删除重复日程的某一次（agenda 返回的实例 ID）
  feishu-cli calendar delete-event INSTANCE_EVENT_ID --apply-to single

  # 删除整条重复序列（含例外）
  feishu-cli calendar delete-event INSTANCE_EVENT_ID --apply-to all --yes

  # 从某一次起不再重复
  feishu-cli calendar delete-event INSTANCE_EVENT_ID --apply-to this-and-following --yes`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		calendarID, eventID, err := calendarEventArgs(args)
		if err != nil {
			return err
		}
		applyTo, _ := cmd.Flags().GetString("apply-to")
		notify, _ := cmd.Flags().GetBool("notify")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		output, _ := cmd.Flags().GetString("output")
		applyTo = strings.TrimSpace(applyTo)

		if err := validateApplyToFlag(applyTo); err != nil {
			return err
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}

		if dryRun {
			return printDryRunPlan(cmd, "delete-event 预览（未执行）", map[string]any{
				"calendar_id": calendarID,
				"event_id":    eventID,
				"apply_to":    applyTo,
			}, deleteEventDryRunSteps(calendarID, eventID, applyTo, notify))
		}

		if err := config.Validate(); err != nil {
			return err
		}
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}

		current, err := client.GetEvent(calendarID, eventID, token)
		if err != nil {
			return err
		}
		kind := client.ClassifyEvent(current)
		scope, err := client.ValidateApplyTo(kind, applyTo)
		if err != nil {
			return clierr.Usage(err)
		}
		scopeText := client.DescribeDeleteScope(kind, scope)
		if scope == "" || kind != client.RecurringKindNormal {
			fmt.Fprintf(cmdErrOut(), "提示：%s，本次删除范围：%s\n", client.RecurringKindLabel(kind), scopeText)
		}
		if scope == client.ApplyToAll || scope == client.ApplyToThisAndFollowing {
			if err := confirmDangerousAction(cmd, fmt.Sprintf("将删除%s（日程 %s，不可恢复）", scopeText, eventID)); err != nil {
				return err
			}
		}

		res, err := client.DeleteEventScoped(calendarID, eventID, current, scope, notify, calendarProgress("calendar delete-event"), token)
		if err != nil {
			if res != nil && output == "json" {
				_ = printJSON(res)
			}
			return err
		}

		if output == "json" {
			return printJSON(res)
		}
		fmt.Printf("日程删除成功！（日程 ID: %s）\n", eventID)
		fmt.Printf("  影响范围:  %s\n", res.Scope)
		if res.RecurrenceTruncated != "" {
			fmt.Printf("  原序列截断: %s（主体 %s）\n", res.RecurrenceTruncated, res.MasterEventID)
		}
		printRecurringBatchSummary(res.Exceptions, true)
		return nil
	},
}

// deleteEventDryRunSteps 生成 delete-event 的预览步骤（不联网）
func deleteEventDryRunSteps(calendarID, eventID, applyTo string, notify bool) []dryRunStep {
	base := "/open-apis/calendar/v4/calendars/" + calendarID + "/events/"
	steps := []dryRunStep{{Method: "GET", URL: base + eventID, Desc: "读取日程，判断普通/重复主体/实例/例外并说明影响范围"}}
	switch applyTo {
	case client.ApplyToAll:
		steps = append(steps,
			dryRunStep{Method: "GET", URL: base + "<master_event_id>/instances", Desc: "扫描全部例外"},
			dryRunStep{Method: "DELETE", URL: base + "<exception_event_id>", Desc: "逐个销毁例外（不通知）", Params: map[string]any{"need_notification": false, "delete_exception": true}},
			dryRunStep{Method: "DELETE", URL: base + "<master_event_id>", Desc: "最后删除重复日程主体", Params: map[string]any{"need_notification": notify}},
		)
	case client.ApplyToThisAndFollowing:
		steps = append(steps,
			dryRunStep{Method: "GET", URL: base + "<master_event_id>/instances", Desc: "扫描截断点及之后的例外"},
			dryRunStep{Method: "DELETE", URL: base + "<exception_event_id>", Desc: "销毁截断点及之后的例外（不通知）", Params: map[string]any{"need_notification": false, "delete_exception": true}},
			dryRunStep{Method: "PATCH", URL: base + "<master_event_id>", Desc: "把主体 RRULE 截断到截断日前一天", Body: map[string]any{"recurrence": "<原规则 + UNTIL>", "need_notification": notify}},
		)
	default:
		steps = append(steps, dryRunStep{Method: "DELETE", URL: base + eventID, Desc: "删除该日程（主体 ID=整条序列不含例外；实例 ID=仅这一次）", Params: map[string]any{"need_notification": notify}})
	}
	return steps
}

func init() {
	calendarCmd.AddCommand(deleteEventCmd)
	deleteEventCmd.Flags().String("apply-to", "", "重复日程删除范围：single | all | this-and-following（不传=服务端原生语义）")
	deleteEventCmd.Flags().Bool("notify", true, "是否通知参与人")
	deleteEventCmd.Flags().Bool("dry-run", false, "只预览将发出的请求，不执行")
	deleteEventCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	deleteEventCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	addWriteAsFlag(deleteEventCmd)
}
