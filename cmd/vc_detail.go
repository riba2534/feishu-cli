package cmd

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// meetingNoPattern 匹配 9 位纯数字会议号
var meetingNoPattern = regexp.MustCompile(`^\d{9}$`)

// meetingDetailView vc detail 单条聚合结果
// Status: ongoing（进行中）/ ended（已结束）；进行中的会议纪要与妙记尚未生成，
// 相关字段为空且用 Hint 说明，而非报错。
type meetingDetailView struct {
	MeetingID   string `json:"meeting_id"`
	MeetingNo   string `json:"meeting_no,omitempty"`
	Topic       string `json:"topic,omitempty"`
	StartTime   string `json:"start_time,omitempty"`
	EndTime     string `json:"end_time,omitempty"`
	Status      string `json:"status,omitempty"`
	NoteID      string `json:"note_id,omitempty"`
	MinuteToken string `json:"minute_token,omitempty"`
	Hint        string `json:"hint,omitempty"`
	Error       string `json:"error,omitempty"`
}

var vcDetailCmd = &cobra.Command{
	Use:   "detail <meeting_id 或会议号>",
	Short: "聚合查询会议详情（一次拿齐 note_id + minute_token）",
	Long: `聚合查询单场会议的排查入口：会议基础信息 + 关联的智能纪要 note_id + 妙记 minute_token。

一条命令串联 meeting.get 与 recording 两个端点，把后续查纪要 / 妙记所需的所有 ID 一次性拿齐，
无需再手动逐个调用。

参数:
  <meeting_id 或会议号>  会议 ID（长数字串）或 9 位会议号。传会议号时会先反查关联的会议
                        （一个会议号可能对应多场周期性会议实例，会分别输出）。

可选参数:
  --start        会议号反查时间窗口起点（YYYY-MM-DD 或 RFC3339，默认近 90 天）
  --end          会议号反查时间窗口终点（YYYY-MM-DD 或 RFC3339，默认当前时间）
  --output, -o   输出格式（json）

进行中的会议：纪要与妙记尚未生成，此时 note_id / minute_token 为空并附状态说明，不视为错误。

权限:
  - User Access Token
  - meeting.get + recording 路径: vc:meeting.meetingevent:read、vc:record:readonly
  - 会议号反查（传 9 位会议号时）额外需要: vc:meeting:readonly 或 vc:meeting.meetingid:read

示例:
  # 直接用会议 ID 查
  feishu-cli vc detail 7664881665696944183

  # 用 9 位会议号查（自动反查关联会议）
  feishu-cli vc detail 543343946

  # 会议号 + 指定时间窗口 + JSON 输出
  feishu-cli vc detail 543343946 --start 2026-06-01 --end 2026-07-22 -o json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token, err := requireUserToken(cmd, "vc detail")
		if err != nil {
			return err
		}

		input := strings.TrimSpace(args[0])
		if input == "" {
			return fmt.Errorf("请提供 meeting_id 或 9 位会议号")
		}
		startStr, _ := cmd.Flags().GetString("start")
		endStr, _ := cmd.Flags().GetString("end")
		output, _ := cmd.Flags().GetString("output")

		// 入参识别：9 位纯数字视为会议号，先反查 meeting_id；否则直接当 meeting_id
		var meetingIDs []string
		resolvedFromNo := meetingNoPattern.MatchString(input)
		if resolvedFromNo {
			ids, err := resolveMeetingIDsByNo(input, startStr, endStr, token)
			if err != nil {
				return err
			}
			if len(ids) == 0 {
				return fmt.Errorf("会议号 %s 在查询时间窗口内没有关联的会议记录（可用 --start/--end 调整窗口，默认近 90 天）", input)
			}
			meetingIDs = ids
		} else {
			meetingIDs = []string{input}
		}

		views := make([]*meetingDetailView, 0, len(meetingIDs))
		for i, mid := range meetingIDs {
			if i > 0 {
				time.Sleep(vcBatchDelay)
			}
			views = append(views, fetchMeetingDetail(mid, token))
		}

		// 全部失败判定在两种输出模式前统一计算：JSON 模式也必须以非 0 退出码收尾，
		// 否则脚本/Agent 用 $? 会把彻底失败误判成功（错误只藏在 JSON 字段里）。
		allFailed := len(views) > 0
		for _, v := range views {
			if v.Error == "" {
				allFailed = false
				break
			}
		}

		if output == "json" {
			result := map[string]any{"meetings": views}
			if resolvedFromNo {
				result["meeting_no"] = input
			}
			if err := printJSON(result); err != nil {
				return err
			}
			if allFailed {
				return fmt.Errorf("全部 %d 个会议详情查询失败（详见输出 JSON 的 error 字段）", len(views))
			}
			return nil
		}

		printMeetingDetails(views, input, resolvedFromNo)

		if allFailed {
			return fmt.Errorf("会议详情查询失败")
		}
		return nil
	},
}

// resolveMeetingIDsByNo 通过会议号反查关联的 meeting_id 列表
// 时间窗口默认近 90 天（list_by_no 仅支持 90 天内），可用 --start/--end 覆盖。
func resolveMeetingIDsByNo(meetingNo, startStr, endStr, token string) ([]string, error) {
	now := time.Now()

	var startSec int64
	if s := strings.TrimSpace(startStr); s != "" {
		rfc, err := parseVCTime(s, false)
		if err != nil {
			return nil, fmt.Errorf("解析 --start 失败: %w", err)
		}
		t, err := time.Parse(time.RFC3339, rfc)
		if err != nil {
			return nil, fmt.Errorf("解析 --start 失败: %w", err)
		}
		startSec = t.Unix()
	} else {
		startSec = now.Add(-89 * 24 * time.Hour).Unix()
	}

	var endSec int64
	if e := strings.TrimSpace(endStr); e != "" {
		rfc, err := parseVCTime(e, true)
		if err != nil {
			return nil, fmt.Errorf("解析 --end 失败: %w", err)
		}
		t, err := time.Parse(time.RFC3339, rfc)
		if err != nil {
			return nil, fmt.Errorf("解析 --end 失败: %w", err)
		}
		endSec = t.Unix()
	} else {
		endSec = now.Add(time.Hour).Unix()
	}

	if startSec > endSec {
		return nil, fmt.Errorf("--start 不能晚于 --end")
	}

	data, err := client.ListMeetingsByNo(meetingNo, startSec, endSec, token)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		MeetingBriefs []struct {
			ID string `json:"id"`
		} `json:"meeting_briefs"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("解析会议号查询结果失败: %w", err)
	}

	ids := make([]string, 0, len(parsed.MeetingBriefs))
	for _, b := range parsed.MeetingBriefs {
		if b.ID != "" {
			ids = append(ids, b.ID)
		}
	}
	return dedupStrings(ids), nil
}

// fetchMeetingDetail 聚合单场会议详情：meeting.get 拿基础信息 + note_id，
// recording 端点提取 minute_token。进行中的会议跳过 recording 并附状态说明。
func fetchMeetingDetail(meetingID, token string) *meetingDetailView {
	v := &meetingDetailView{MeetingID: meetingID}

	data, err := client.GetMeeting(meetingID, token)
	if err != nil {
		v.Error = fmt.Sprintf("获取会议详情失败: %v", err)
		return v
	}

	var parsed struct {
		Meeting struct {
			ID        string `json:"id"`
			MeetingNo string `json:"meeting_no"`
			Topic     string `json:"topic"`
			StartTime string `json:"start_time"`
			EndTime   string `json:"end_time"`
			NoteID    string `json:"note_id"`
		} `json:"meeting"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		v.Error = fmt.Sprintf("解析会议信息失败: %v", err)
		return v
	}

	m := parsed.Meeting
	if m.ID != "" {
		v.MeetingID = m.ID
	}
	v.MeetingNo = m.MeetingNo
	v.Topic = m.Topic
	v.StartTime = formatVCTime(m.StartTime)
	v.EndTime = formatVCTime(m.EndTime)
	v.NoteID = m.NoteID

	// 进行中的会议：纪要与妙记尚未生成，跳过 recording 查询，仅附状态说明
	if meetingInProgress(m.StartTime, m.EndTime) {
		v.Status = "ongoing"
		v.Hint = "会议进行中，纪要与妙记尚未生成"
		return v
	}
	v.Status = "ended"

	// 已结束：查 recording 提取 minute_token（best-effort，失败降级为 hint 不影响基础信息）
	var minuteHint string
	recData, recErr := client.GetMeetingRecording(meetingID, token)
	if recErr != nil {
		minuteHint = fmt.Sprintf("查询会议录制失败: %v", recErr)
	} else if rv := parseRecordingData(recData); rv.MinuteToken != "" {
		v.MinuteToken = rv.MinuteToken
	}

	var empties []string
	if v.NoteID == "" {
		empties = append(empties, "note_id")
	}
	if v.MinuteToken == "" && minuteHint == "" {
		empties = append(empties, "minute_token")
	}
	if len(empties) > 0 {
		v.Hint = "该会议未找到 " + strings.Join(empties, ", ")
	}
	if minuteHint != "" {
		if v.Hint != "" {
			v.Hint += "; " + minuteHint
		} else {
			v.Hint = minuteHint
		}
	}
	return v
}

// meetingInProgress 用 start/end 时间戳判断会议是否进行中：
// 有开始时间但无结束时间、或结束时间不晚于开始时间，视为进行中。
// 读取原始 Unix 时间戳字符串，"0" / 空视为缺失。
func meetingInProgress(startRaw, endRaw string) bool {
	start, hasStart := parseVCTimestamp(startRaw)
	end, hasEnd := parseVCTimestamp(endRaw)
	if !hasStart {
		return false
	}
	if !hasEnd {
		return true
	}
	return !end.After(start)
}

// printMeetingDetails 文本模式输出聚合结果
func printMeetingDetails(views []*meetingDetailView, input string, resolvedFromNo bool) {
	if resolvedFromNo {
		fmt.Printf("会议号 %s 关联 %d 场会议:\n\n", input, len(views))
	}
	for i, v := range views {
		if resolvedFromNo {
			fmt.Printf("[%d] meeting_id=%s\n", i+1, v.MeetingID)
		} else {
			fmt.Printf("meeting_id=%s\n", v.MeetingID)
		}
		if v.Error != "" {
			fmt.Printf("    FAIL: %s\n\n", v.Error)
			continue
		}
		if v.Topic != "" {
			fmt.Printf("    主题:          %s\n", v.Topic)
		}
		if v.MeetingNo != "" {
			fmt.Printf("    会议号:        %s\n", v.MeetingNo)
		}
		if v.Status != "" {
			fmt.Printf("    状态:          %s\n", v.Status)
		}
		if v.StartTime != "" {
			fmt.Printf("    开始时间:      %s\n", v.StartTime)
		}
		if v.EndTime != "" {
			fmt.Printf("    结束时间:      %s\n", v.EndTime)
		}
		if v.NoteID != "" {
			fmt.Printf("    note_id:       %s\n", v.NoteID)
		}
		if v.MinuteToken != "" {
			fmt.Printf("    minute_token:  %s\n", v.MinuteToken)
		}
		if v.Hint != "" {
			fmt.Printf("    说明:          %s\n", v.Hint)
		}
		fmt.Println()
	}
}

func init() {
	vcCmd.AddCommand(vcDetailCmd)
	vcDetailCmd.Flags().String("start", "", "会议号反查时间窗口起点（YYYY-MM-DD 或 RFC3339，默认近 90 天）")
	vcDetailCmd.Flags().String("end", "", "会议号反查时间窗口终点（YYYY-MM-DD 或 RFC3339，默认当前时间）")
	vcDetailCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	vcDetailCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
}
