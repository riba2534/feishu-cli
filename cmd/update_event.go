package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var updateEventCmd = &cobra.Command{
	Use:   "update-event [calendar_id] <event_id>",
	Short: "更新日程",
	Long: `更新指定日程的信息。只更新提供的字段，未提供的字段保持不变。

参数:
  calendar_id       日历 ID（可省略，省略时使用 primary 主日历）
  event_id          日程 ID

可选参数:
  --summary, -s     日程标题
  --start           开始时间（须与 --end 同时传：服务端对只改一端的请求返回成功但不生效）
  --end             结束时间（须与 --start 同时传）
  --description, -d 日程描述
  --location, -l    地点
  --rrule           重复规则（RFC5545 RRULE），更新为重复日程或修改重复规则
  --apply-to        重复日程的修改范围：single | all | this-and-following（见下文）
  --notify          是否通知参与人（默认 true）
  --dry-run         只预览，不执行（不联网）
  --as              身份：auto（默认）| user | bot
  --output, -o      输出格式（json）

重复日程（先读取日程判断类型，再按范围处理）:
  event_id 形如 {uid}_{原始时间戳}：_0 是重复日程主体（或普通日程），其余是某一次实例。
  calendar agenda / event-search 返回的是**实例 ID**。
  - 不传 --apply-to：保持服务端原生语义——主体 ID 改整条序列（已单独修改过的例外不受影响），
    实例 ID 只改这一次；stderr 会说明实际影响范围
  - --apply-to single：只改这一次（实例/例外 ID）
  - --apply-to all：改整条序列。改了时间时先删除全部例外再改主体；未改时间时把本次传入的字段
    同步到每个例外后再改主体（需确认，非交互环境加 --yes）
  - --apply-to this-and-following：从该实例起截断原序列，并以修改后的内容新建后续序列
    （删除该次及之后的例外；需确认，非交互环境加 --yes）

时间格式:
  推荐 RFC3339，例如 2024-01-21T14:00:00+08:00；也接受 "2024-01-21 14:00"（本地时区）与 Unix 秒。

示例:
  # 更新日程标题（主日历上的日程可只传 event_id）
  feishu-cli calendar update-event EVENT_ID --summary "新标题"

  # 更新日程时间（--start/--end 必须成对）
  feishu-cli calendar update-event CAL_ID EVENT_ID \
    --start 2024-01-21T15:00:00+08:00 \
    --end 2024-01-21T16:00:00+08:00

  # 把重复日程整条序列（含例外）改名
  feishu-cli calendar update-event INSTANCE_EVENT_ID --summary "新周会" --apply-to all --yes

  # 从某次起改时间（截断原序列 + 新建后续序列）
  feishu-cli calendar update-event INSTANCE_EVENT_ID --apply-to this-and-following \
    --start 2024-02-05T15:00:00+08:00 --end 2024-02-05T16:00:00+08:00 --yes

  # 把日程改为每个工作日重复
  feishu-cli calendar update-event CAL_ID EVENT_ID \
    --rrule "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR"`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		calendarID, eventID, err := calendarEventArgs(args)
		if err != nil {
			return err
		}

		summary, _ := cmd.Flags().GetString("summary")
		startTime, _ := cmd.Flags().GetString("start")
		endTime, _ := cmd.Flags().GetString("end")
		description, _ := cmd.Flags().GetString("description")
		location, _ := cmd.Flags().GetString("location")
		rrule, _ := cmd.Flags().GetString("rrule")
		applyTo, _ := cmd.Flags().GetString("apply-to")
		notify, _ := cmd.Flags().GetBool("notify")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		output, _ := cmd.Flags().GetString("output")
		applyTo = strings.TrimSpace(applyTo)

		params := client.UpdateEventParams{
			Summary:     summary,
			StartTime:   startTime,
			EndTime:     endTime,
			Description: description,
			Location:    location,
			Recurrence:  rrule,
		}
		if !params.HasFields() {
			return clierr.Usagef("请至少提供一个要更新的字段（--summary, --start/--end, --description, --location, --rrule）")
		}
		// 实测：只传 start 或 end 时服务端返回成功却不修改时间，必须成对传
		if (startTime == "") != (endTime == "") {
			return clierr.Usagef("--start 与 --end 必须同时指定（只改一端时服务端返回成功但不生效）")
		}
		if startTime != "" {
			s, err := client.ParseEventTimeToUnix(startTime)
			if err != nil {
				return clierr.Usagef("--start: %v", err)
			}
			e, err := client.ParseEventTimeToUnix(endTime)
			if err != nil {
				return clierr.Usagef("--end: %v", err)
			}
			if e <= s {
				return clierr.Usagef("--end 必须晚于 --start")
			}
		}
		if err := validateApplyToFlag(applyTo); err != nil {
			return err
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}
		if cmd.Flags().Changed("notify") {
			params.NeedNotification = &notify
		}

		if dryRun {
			return printDryRunPlan(cmd, "update-event 预览（未执行）", map[string]any{
				"calendar_id": calendarID,
				"event_id":    eventID,
				"apply_to":    applyTo,
			}, updateEventDryRunSteps(calendarID, eventID, applyTo, params))
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
		if scope == "" {
			fmt.Fprintf(cmdErrOut(), "提示：%s，本次修改范围：%s\n", client.RecurringKindLabel(kind), client.DescribeUpdateScope(kind, scope))
		}
		if scope == client.ApplyToAll || scope == client.ApplyToThisAndFollowing {
			if err := confirmDangerousAction(cmd, fmt.Sprintf("将修改%s（日程 %s）", client.DescribeUpdateScope(kind, scope), eventID)); err != nil {
				return err
			}
		}

		res, err := client.UpdateEventScoped(calendarID, eventID, current, scope, params, calendarProgress("calendar update-event"), token)
		if err != nil {
			if res != nil && output == "json" {
				_ = printJSON(res)
			}
			return err
		}

		if output == "json" {
			if scope == "" || scope == client.ApplyToSingle {
				// 兼容旧输出：单日程更新仍直接输出日程对象
				return printJSON(res.Event)
			}
			return printJSON(res)
		}
		fmt.Println("日程更新成功！")
		fmt.Printf("  影响范围:  %s\n", res.Scope)
		if ev := res.Event; ev != nil {
			fmt.Printf("  日程 ID:   %s\n", ev.EventID)
			fmt.Printf("  标题:      %s\n", ev.Summary)
			printEventTimeLines(ev, "  ")
			if ev.Description != "" {
				fmt.Printf("  描述:      %s\n", ev.Description)
			}
			if ev.Location != "" {
				fmt.Printf("  地点:      %s\n", ev.Location)
			}
			if ev.Recurrence != "" {
				fmt.Printf("  重复规则:  %s\n", ev.Recurrence)
			}
		}
		if res.RecurrenceTruncated != "" {
			fmt.Printf("  原序列截断: %s（主体 %s）\n", res.RecurrenceTruncated, res.MasterEventID)
		}
		if res.FollowEvent != nil {
			fmt.Printf("  新序列 ID: %s（%s ~ %s）\n", res.FollowEvent.EventID, res.FollowEvent.StartTime, res.FollowEvent.EndTime)
		}
		printRecurringBatchSummary(res.Exceptions, res.ExceptionsDeleted)
		return nil
	},
}

// updateEventDryRunSteps 生成 update-event 的预览步骤（不联网，按 --apply-to 列出可能的请求）
func updateEventDryRunSteps(calendarID, eventID, applyTo string, params client.UpdateEventParams) []dryRunStep {
	body := map[string]any{}
	if params.Summary != "" {
		body["summary"] = params.Summary
	}
	if params.Description != "" {
		body["description"] = params.Description
	}
	if params.Location != "" {
		body["location"] = map[string]any{"name": params.Location}
	}
	if params.Recurrence != "" {
		body["recurrence"] = params.Recurrence
	}
	if params.StartTime != "" {
		body["start_time"] = map[string]any{"timestamp": params.StartTime}
		body["end_time"] = map[string]any{"timestamp": params.EndTime}
	}
	if params.NeedNotification != nil {
		body["need_notification"] = *params.NeedNotification
	}
	base := "/open-apis/calendar/v4/calendars/" + calendarID + "/events/"
	steps := []dryRunStep{{Method: "GET", URL: base + eventID, Desc: "读取日程，判断普通/重复主体/实例/例外"}}
	switch applyTo {
	case client.ApplyToAll:
		steps = append(steps,
			dryRunStep{Method: "GET", URL: base + "<master_event_id>/instances", Desc: "扫描全部例外"},
			dryRunStep{Method: "PATCH|DELETE", URL: base + "<exception_event_id>", Desc: "改了时间：删除每个例外；否则把本次字段同步到每个例外（不通知）"},
			dryRunStep{Method: "PATCH", URL: base + "<master_event_id>", Desc: "最后更新重复日程主体", Body: body},
		)
	case client.ApplyToThisAndFollowing:
		steps = append(steps,
			dryRunStep{Method: "GET", URL: base + "<master_event_id>/instances", Desc: "扫描截断点及之后的例外"},
			dryRunStep{Method: "DELETE", URL: base + "<exception_event_id>", Desc: "删除截断点及之后的例外（不通知）", Params: map[string]any{"delete_exception": true}},
			dryRunStep{Method: "PATCH", URL: base + "<master_event_id>", Desc: "把主体 RRULE 截断到截断日前一天", Body: map[string]any{"recurrence": "<原规则 + UNTIL>"}},
			dryRunStep{Method: "POST", URL: "/open-apis/calendar/v4/calendars/" + calendarID + "/events", Desc: "以截断点为起点新建后续序列（继承主体设置，叠加本次修改）并复制参与人", Body: body},
		)
	default:
		steps = append(steps, dryRunStep{Method: "PATCH", URL: base + eventID, Desc: "更新该日程（主体 ID=整条序列不含例外；实例 ID=仅这一次）", Body: body})
	}
	return steps
}

// printRecurringBatchSummary 打印例外批量处理汇总
func printRecurringBatchSummary(sum *client.RecurringBatchSummary, deleted bool) {
	if sum == nil {
		return
	}
	verb := "同步修改"
	if deleted {
		verb = "删除"
	}
	fmt.Printf("  例外日程:  %s %d 个，成功 %d，失败 %d\n", verb, sum.Total, sum.Succeeded, sum.Failed)
	for _, f := range sum.Failures {
		fmt.Printf("    失败 %s: %s\n", f.EventID, f.Error)
	}
}

func init() {
	calendarCmd.AddCommand(updateEventCmd)
	updateEventCmd.Flags().StringP("summary", "s", "", "日程标题")
	updateEventCmd.Flags().String("start", "", "开始时间（须与 --end 同时传）")
	updateEventCmd.Flags().String("end", "", "结束时间（须与 --start 同时传）")
	updateEventCmd.Flags().StringP("description", "d", "", "日程描述")
	updateEventCmd.Flags().StringP("location", "l", "", "地点")
	updateEventCmd.Flags().String("rrule", "", "重复规则（RFC5545 RRULE，如 FREQ=WEEKLY;BYDAY=MO）")
	updateEventCmd.Flags().String("apply-to", "", "重复日程修改范围：single | all | this-and-following（不传=服务端原生语义）")
	updateEventCmd.Flags().Bool("notify", true, "是否通知参与人")
	updateEventCmd.Flags().Bool("dry-run", false, "只预览将发出的请求，不执行")
	updateEventCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	updateEventCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	addWriteAsFlag(updateEventCmd)
}
