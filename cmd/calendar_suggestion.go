package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var calendarSuggestionCmd = &cobra.Command{
	Use:   "suggestion",
	Short: "智能推荐可用会议时段",
	Long: `调用飞书 freebusy/suggestion 智能时段建议接口，根据参与者列表和会议时长，
自动避开冲突推荐可用时段。AI Agent 安排会议时常用。

参数:
  --attendee-ids        参与者 ID 列表，逗号分隔（ou_xxx 用户 / oc_xxx 群聊混合）
  --duration            会议时长，支持 Go duration（30m / 1h30m）或纯分钟数（90）；
                        服务端单位为分钟，最大 1440（24h）
  --start               搜索窗口起点（RFC3339，默认当前时间）
  --end                 搜索窗口终点（RFC3339，默认 start 当天 23:59:59）
  --timezone            时区（例如 Asia/Shanghai）
  --event-rrule         周期性规则（rrule 字符串）
  --exclude             需要排除的时段，格式 start~end，多段逗号分隔
                        例：2024-01-21T10:00:00+08:00~2024-01-21T11:00:00+08:00

权限:
  calendar:calendar.free_busy:read（User Token 或 App Token 均可）

示例:
  # 给两个同事和我自己排 30 分钟，今天的范围内推荐
  feishu-cli calendar suggestion \
    --attendee-ids ou_aaa,ou_bbb \
    --duration 30m

  # 指定明天 9-18 点窗口，时长 1 小时，排除午休 12-13 点
  feishu-cli calendar suggestion \
    --attendee-ids ou_aaa,oc_groupid \
    --duration 1h \
    --start 2024-01-22T09:00:00+08:00 \
    --end   2024-01-22T18:00:00+08:00 \
    --exclude 2024-01-22T12:00:00+08:00~2024-01-22T13:00:00+08:00

  # JSON 输出便于 AI Agent 解析
  feishu-cli calendar suggestion --attendee-ids ou_aaa --duration 30m -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		attendeeIDs, _ := cmd.Flags().GetString("attendee-ids")
		durationStr, _ := cmd.Flags().GetString("duration")
		startStr, _ := cmd.Flags().GetString("start")
		endStr, _ := cmd.Flags().GetString("end")
		timezone, _ := cmd.Flags().GetString("timezone")
		eventRrule, _ := cmd.Flags().GetString("event-rrule")
		excludeStr, _ := cmd.Flags().GetString("exclude")
		output, _ := cmd.Flags().GetString("output")

		// 解析参与者
		userIDs, chatIDs, err := client.SplitAttendeeIDs(attendeeIDs)
		if err != nil {
			return err
		}

		// 解析时长（分钟）
		durationMinutes, err := parseDurationMinutes(durationStr)
		if err != nil {
			return err
		}

		// 默认 start 用当前时间；end 用 start 当天 23:59:59
		startTime, endTime, err := resolveSearchWindow(startStr, endStr)
		if err != nil {
			return err
		}

		// 排除时段
		excludedTimes, err := parseExcludedTimes(excludeStr)
		if err != nil {
			return err
		}

		req := &client.SuggestionRequest{
			SearchStartTime:    startTime,
			SearchEndTime:      endTime,
			Timezone:           timezone,
			EventRrule:         eventRrule,
			DurationMinutes:    durationMinutes,
			AttendeeUserIDs:    userIDs,
			AttendeeChatIDs:    chatIDs,
			ExcludedEventTimes: excludedTimes,
		}

		result, err := client.SuggestFreebusy(req, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(result)
		}

		if result == nil || len(result.Suggestions) == 0 {
			fmt.Println("未找到合适的时段")
			if result != nil && result.AiActionGuidance != "" {
				fmt.Printf("\n建议: %s\n", result.AiActionGuidance)
			}
			return nil
		}

		fmt.Printf("推荐时段（共 %d 个）:\n\n", len(result.Suggestions))
		for i, s := range result.Suggestions {
			fmt.Printf("[%d] %s ~ %s\n", i+1, s.EventStartTime, s.EventEndTime)
			if s.RecommendReason != "" {
				fmt.Printf("    理由: %s\n", s.RecommendReason)
			}
		}
		if result.AiActionGuidance != "" {
			fmt.Printf("\n建议: %s\n", result.AiActionGuidance)
		}

		return nil
	},
}

// parseDurationMinutes 解析时长，支持 "30m" / "1h30m" 或纯分钟 "90"
// 0 或空字符串视为不限制；范围 1-1440
func parseDurationMinutes(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}

	// 纯数字按分钟处理
	if isAllDigits(s) {
		mins := 0
		for _, c := range s {
			mins = mins*10 + int(c-'0')
		}
		if mins < 1 || mins > 1440 {
			return 0, fmt.Errorf("--duration 必须在 1-1440 分钟之间，当前: %d", mins)
		}
		return mins, nil
	}

	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("解析 --duration 失败: %w（支持 30m / 1h30m / 90）", err)
	}
	mins := int(d / time.Minute)
	if mins < 1 || mins > 1440 {
		return 0, fmt.Errorf("--duration 必须在 1-1440 分钟之间，当前: %d 分钟", mins)
	}
	return mins, nil
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// resolveSearchWindow 解析 start / end，缺省 start=now，end=start 当天 23:59:59
// 返回 RFC3339 字符串。
func resolveSearchWindow(startStr, endStr string) (string, string, error) {
	var start time.Time
	if startStr == "" {
		start = time.Now()
	} else {
		t, err := time.Parse(time.RFC3339, startStr)
		if err != nil {
			return "", "", fmt.Errorf("解析 --start 失败: %w（RFC3339 格式，如 2024-01-21T14:00:00+08:00）", err)
		}
		start = t
	}

	var end time.Time
	if endStr == "" {
		end = time.Date(start.Year(), start.Month(), start.Day(), 23, 59, 59, 0, start.Location())
	} else {
		t, err := time.Parse(time.RFC3339, endStr)
		if err != nil {
			return "", "", fmt.Errorf("解析 --end 失败: %w（RFC3339 格式）", err)
		}
		end = t
	}

	if !end.After(start) {
		return "", "", fmt.Errorf("--end 必须晚于 --start")
	}

	return start.Format(time.RFC3339), end.Format(time.RFC3339), nil
}

// parseExcludedTimes 解析 --exclude，多段逗号分隔，单段 start~end
func parseExcludedTimes(s string) ([]*client.SuggestionEventTime, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}

	var out []*client.SuggestionEventTime
	for _, raw := range strings.Split(s, ",") {
		r := strings.TrimSpace(raw)
		if r == "" {
			continue
		}
		parts := strings.Split(r, "~")
		if len(parts) != 2 {
			return nil, fmt.Errorf("无效的 --exclude 段 %q，应为 start~end 格式", r)
		}
		startT, err := time.Parse(time.RFC3339, strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, fmt.Errorf("--exclude 起始时间 %q 解析失败: %w", parts[0], err)
		}
		endT, err := time.Parse(time.RFC3339, strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("--exclude 结束时间 %q 解析失败: %w", parts[1], err)
		}
		if !endT.After(startT) {
			return nil, fmt.Errorf("--exclude 段 %q 的结束时间必须晚于起始时间", r)
		}
		out = append(out, &client.SuggestionEventTime{
			EventStartTime: startT.Format(time.RFC3339),
			EventEndTime:   endT.Format(time.RFC3339),
		})
	}
	return out, nil
}

func init() {
	calendarCmd.AddCommand(calendarSuggestionCmd)
	calendarSuggestionCmd.Flags().String("attendee-ids", "", "参与者 ID 列表，逗号分隔（ou_xxx / oc_xxx）")
	calendarSuggestionCmd.Flags().String("duration", "", "会议时长（30m / 1h30m / 90）")
	calendarSuggestionCmd.Flags().String("start", "", "搜索起点（RFC3339，默认当前时间）")
	calendarSuggestionCmd.Flags().String("end", "", "搜索终点（RFC3339，默认 start 当天 23:59:59）")
	calendarSuggestionCmd.Flags().String("timezone", "", "时区（例：Asia/Shanghai）")
	calendarSuggestionCmd.Flags().String("event-rrule", "", "周期性规则 rrule 字符串")
	calendarSuggestionCmd.Flags().String("exclude", "", "排除的时段，多段逗号分隔，单段 start~end")
	calendarSuggestionCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	calendarSuggestionCmd.Flags().String("user-access-token", "", "User Access Token（可选，默认 App Token）")

	mustMarkFlagRequired(calendarSuggestionCmd, "attendee-ids", "duration")
}
