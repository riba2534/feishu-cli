package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

// calendarWriteAsHelp 日历/任务写命令的 --as 说明（默认 auto 是 v1.42 起的行为变更）
const calendarWriteAsHelp = "身份: auto(默认；已登录用 User Token，未配置回退 Bot，已配置但不可用 fail-closed) | user | bot（操作应用日历/无人值守时显式传 --as bot）"

// addWriteAsFlag 为日历/任务写命令注册 --as（默认 auto）。
//
// 历史上这些命令默认 Bot 身份，而 agenda/event-search 默认 auto：用户从 agenda 拿到自己日程的 ID
// 再用 Bot 去改/删大概率失败（Bot 不在日程里）。对齐官方后默认 auto。
func addWriteAsFlag(cmd *cobra.Command) {
	cmd.Flags().String("as", "auto", calendarWriteAsHelp)
}

// calendarEventArgs 解析 "[calendar_id] <event_id>" 形式的位置参数：只给一个参数时视为 event_id，
// 日历取 primary（当前身份的主日历）。
func calendarEventArgs(args []string) (calendarID, eventID string, err error) {
	switch len(args) {
	case 1:
		calendarID, eventID = "primary", strings.TrimSpace(args[0])
	case 2:
		calendarID, eventID = strings.TrimSpace(args[0]), strings.TrimSpace(args[1])
	default:
		return "", "", clierr.Usagef("需要 [calendar_id] <event_id>（只传 event_id 时使用主日历）")
	}
	if calendarID == "" {
		calendarID = "primary"
	}
	if eventID == "" {
		return "", "", clierr.Usagef("event_id 不能为空")
	}
	return calendarID, eventID, nil
}

// validateApplyToFlag 只做 --apply-to 取值校验（不联网，可在 dry-run 前调用）
func validateApplyToFlag(applyTo string) error {
	applyTo = strings.TrimSpace(applyTo)
	if applyTo == "" {
		return nil
	}
	for _, v := range client.ApplyToValues {
		if v == applyTo {
			return nil
		}
	}
	return clierr.Usagef("--apply-to 仅支持 %s，得到 %q", strings.Join(client.ApplyToValues, " | "), applyTo)
}

// parseAttendeeIDsFlag 解析 --attendee-ids / --*-ids 列表（按前缀识别 ou_/oc_/omm_/邮箱）
func parseAttendeeIDsFlag(raw string, flagName string, allowAttendeeID bool) ([]*client.AttendeeRef, error) {
	refs, err := client.ParseAttendeeRefs(splitAndTrim(raw), allowAttendeeID)
	if err != nil {
		return nil, clierr.Usagef("--%s: %v", flagName, err)
	}
	return refs, nil
}

// workStderr 日历/任务/审批命令的提示输出（stderr，不污染 stdout 的 JSON）；测试可替换
var workStderr io.Writer = os.Stderr

func cmdErrOut() io.Writer { return workStderr }

// calendarProgress 把重复日程批处理进度写到 stderr（不污染 stdout 的 JSON）
func calendarProgress(prefix string) func(string) {
	return func(msg string) {
		fmt.Fprintf(cmdErrOut(), "[%s] %s\n", prefix, msg)
	}
}

// printEventTimeLines 统一打印开始/结束时间（全天日程标注）
func printEventTimeLines(ev *client.CalendarEvent, indent string) {
	if ev == nil {
		return
	}
	if ev.IsAllDay {
		fmt.Printf("%s时间:      全天日程 %s ~ %s\n", indent, ev.StartTime, ev.EndTime)
		return
	}
	fmt.Printf("%s开始时间:  %s\n", indent, ev.StartTime)
	fmt.Printf("%s结束时间:  %s\n", indent, ev.EndTime)
}
