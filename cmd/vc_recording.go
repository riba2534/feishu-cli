package cmd

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var vcRecordingCmd = &cobra.Command{
	Use:   "recording",
	Short: "查询会议录制，提取 minute_token",
	Long: `查询会议录制信息，从 recording_url 中提取 minute_token（妙记 token）。

支持两种输入方式（互斥）：
  --meeting-ids         直接指定会议 ID 列表（逗号分隔，最多 50 条）
  --calendar-event-ids  通过日历事件实例 ID 反查会议 ID（逗号分隔，最多 50 条）

输出字段：
  meeting_id / minute_token / recording_url / duration

权限:
  - 默认 User 身份（--as user），可用 --as bot|auto 切换
  - vc:record:readonly
  - 使用 --calendar-event-ids 时额外需要 calendar:calendar:read / calendar:calendar.event:read

示例:
  feishu-cli vc recording --meeting-ids 69xxxx,70xxxx
  feishu-cli vc recording --calendar-event-ids <instance_id> -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		meetingRaw, _ := cmd.Flags().GetString("meeting-ids")
		calendarRaw, _ := cmd.Flags().GetString("calendar-event-ids")
		output, _ := cmd.Flags().GetString("output")

		if err := exactlyOneNonEmpty(
			[]string{"meeting-ids", "calendar-event-ids"},
			[]string{meetingRaw, calendarRaw},
		); err != nil {
			return err
		}

		// 入口 1：直接 meeting-ids
		meetingIDs, err := parseCSVIDs(meetingRaw, "meeting-ids")
		if err != nil {
			return err
		}
		calendarIDs, err := parseCSVIDs(calendarRaw, "calendar-event-ids")
		if err != nil {
			return err
		}

		// 本地参数校验（用法错误 exit 2）通过后再解析身份，避免被"未登录"（exit 3）遮住
		token, err := resolveVCReadIdentity(cmd)
		if err != nil {
			return err
		}

		// 入口 2：通过 calendar-event-ids 反查 meeting-ids
		var sourceCalendarEvents map[string][]string // calendar_event_id → meeting_ids
		if calendarRaw != "" {
			cal, err := client.GetPrimaryCalendar(token)
			if err != nil {
				return fmt.Errorf("获取主日历失败: %w", err)
			}
			rel, err := client.MgetInstanceRelationInfo(cal.CalendarID, calendarIDs, false, token)
			if err != nil {
				return err
			}
			sourceCalendarEvents = make(map[string][]string, len(calendarIDs))
			for _, id := range calendarIDs {
				info := rel[id]
				if info == nil {
					sourceCalendarEvents[id] = nil
					continue
				}
				sourceCalendarEvents[id] = info.MeetingInstanceIDs
				meetingIDs = append(meetingIDs, info.MeetingInstanceIDs...)
			}
			// 去重
			meetingIDs = dedupStrings(meetingIDs)
			if len(meetingIDs) > vcBatchLimit {
				return fmt.Errorf("日历事件展开后 meeting_id 数量超过 %d", vcBatchLimit)
			}
		}

		items := make([]vcBatchItem, 0, len(meetingIDs))
		for i, mid := range meetingIDs {
			if i > 0 {
				time.Sleep(vcBatchDelay)
			}
			data, err := client.GetMeetingRecording(mid, token)
			if err != nil {
				items = append(items, vcBatchItem{ID: mid, OK: false, Error: err.Error()})
				continue
			}
			parsed := parseRecordingData(data)
			parsed.MeetingID = mid
			items = append(items, vcBatchItem{ID: mid, OK: true, Data: parsed})
		}

		summary := summarizeBatch(items)

		if output == "json" {
			result := map[string]any{
				"items":   items,
				"summary": summary,
			}
			if sourceCalendarEvents != nil {
				result["source_calendar_events"] = sourceCalendarEvents
			}
			return printJSON(result)
		}

		// 文本输出
		if sourceCalendarEvents != nil {
			fmt.Printf("日历事件展开结果（共 %d 条）:\n", len(sourceCalendarEvents))
			for id, mids := range sourceCalendarEvents {
				fmt.Printf("  %s → %v\n", id, mids)
			}
			fmt.Println()
		}
		for i, it := range items {
			fmt.Printf("[%d] meeting_id=%s", i+1, it.ID)
			if !it.OK {
				fmt.Printf("  FAIL: %s\n", it.Error)
				continue
			}
			fmt.Println()
			if r, ok := it.Data.(*recordingView); ok {
				if r.MinuteToken != "" {
					fmt.Printf("    minute_token:  %s\n", r.MinuteToken)
				}
				if r.RecordingURL != "" {
					fmt.Printf("    recording_url: %s\n", r.RecordingURL)
				}
				if r.Duration != "" {
					fmt.Printf("    duration:      %s\n", r.Duration)
				}
			}
		}
		fmt.Printf("\n合计: %d / 成功 %d / 失败 %d\n", summary.Total, summary.Succeeded, summary.Failed)

		if summary.Failed > 0 && summary.Succeeded == 0 {
			return fmt.Errorf("全部 meeting_id 查询失败")
		}
		return nil
	},
}

// recordingView 会议录制视图数据
type recordingView struct {
	MeetingID    string `json:"meeting_id"`
	MinuteToken  string `json:"minute_token,omitempty"`
	RecordingURL string `json:"recording_url,omitempty"`
	Duration     string `json:"duration,omitempty"`
}

var minuteTokenInURLPattern = regexp.MustCompile(`/minutes/([A-Za-z0-9]+)`)

// parseRecordingData 从 recording API 响应 data 中提取字段
func parseRecordingData(data json.RawMessage) *recordingView {
	var parsed struct {
		Recording struct {
			URL      string `json:"url"`
			Duration string `json:"duration"`
		} `json:"recording"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return &recordingView{}
	}
	r := &recordingView{
		RecordingURL: parsed.Recording.URL,
		Duration:     parsed.Recording.Duration,
	}
	if m := minuteTokenInURLPattern.FindStringSubmatch(parsed.Recording.URL); len(m) == 2 {
		r.MinuteToken = m[1]
	}
	return r
}

func init() {
	vcCmd.AddCommand(vcRecordingCmd)
	vcRecordingCmd.Flags().String("meeting-ids", "", "会议 ID 列表，逗号分隔（最多 50 条）")
	vcRecordingCmd.Flags().String("calendar-event-ids", "", "日历事件实例 ID 列表，逗号分隔（最多 50 条）")
	vcRecordingCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	vcRecordingCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	addVCReadAsFlag(vcRecordingCmd)
}
