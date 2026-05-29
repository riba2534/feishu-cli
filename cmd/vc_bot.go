package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// vcStartAfterEnd 判断 start 是否晚于 end（按 Unix 秒数值比较，而非字符串字典序）。
// 字符串字典序在位数不同时（如 "999999999" vs "1000000000"）≠ 数值序，会误判先后，
// 故用 strconv.ParseInt 转 int64 再比。任一为空（未传）视为无需比较，返回 false。
func vcStartAfterEnd(startSec, endSec string) (bool, error) {
	if startSec == "" || endSec == "" {
		return false, nil
	}
	s, err := strconv.ParseInt(startSec, 10, 64)
	if err != nil {
		return false, fmt.Errorf("解析 --start 秒数失败: %w", err)
	}
	e, err := strconv.ParseInt(endSec, 10, 64)
	if err != nil {
		return false, fmt.Errorf("解析 --end 秒数失败: %w", err)
	}
	return s > e, nil
}

// validateVCPageSize 校验 meeting-events 的 page-size：取值范围 20-100（lark/help 声明），
// 0 表示未传（回落默认 20），故只在非 0 时检查下限/上限。
func validateVCPageSize(pageSize int) error {
	if pageSize != 0 && (pageSize < 20 || pageSize > 100) {
		return fmt.Errorf("--page-size 取值范围 20-100（当前 %d）", pageSize)
	}
	return nil
}

// vcBotCmd 会议机器人父命令组
var vcBotCmd = &cobra.Command{
	Use:   "bot",
	Short: "会议机器人入会/离会/事件",
	Long: `会议机器人相关操作（vc bots 域）。

子命令:
  meeting-join    机器人按会议号加入会议（POST /open-apis/vc/v1/bots/join）
  meeting-leave   机器人离开会议（POST /open-apis/vc/v1/bots/leave）
  meeting-events  查询机器人会议事件（GET /open-apis/vc/v1/bots/events）

权限:
  - meeting-join 需要 vc:meeting.bot.join:write
  - meeting-leave 需要 vc:meeting.bot.leave:write
  - meeting-events 需要 vc 读权限

示例:
  feishu-cli vc bot meeting-join --meeting-number 123456789
  feishu-cli vc bot meeting-leave --meeting-id 6911188411932033028
  feishu-cli vc bot meeting-events --meeting-id 6911188411932033028 --start 2026-03-01 --end 2026-03-31`,
}

// vcParseTimeToUnixSec 把用户输入的时间字符串解析为 Unix 秒（字符串）。
// 纯整数按 Unix 秒原样透传（与 --start/--end help 宣传一致）；其余走 parseVCTime
// 解析日期/RFC3339 再转秒。空输入返回空字符串。
func vcParseTimeToUnixSec(input string, isEnd bool) (string, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", nil
	}
	// 纯整数视为 Unix 秒直接透传。strconv.ParseInt 严格模式会拒绝含 '-'/'T'/':' 的
	// 日期或 RFC3339 串（如 2026-03-01），故不会把日期误吞成时间戳。
	if sec, err := strconv.ParseInt(s, 10, 64); err == nil {
		if sec <= 0 {
			return "", fmt.Errorf("Unix 秒须为正整数（当前 %q）", s)
		}
		return strconv.FormatInt(sec, 10), nil
	}
	rfc, err := parseVCTime(s, isEnd)
	if err != nil {
		return "", err
	}
	t, err := time.Parse(time.RFC3339, rfc)
	if err != nil {
		return "", fmt.Errorf("时间转换失败: %w", err)
	}
	return strconv.FormatInt(t.Unix(), 10), nil
}

var vcBotJoinCmd = &cobra.Command{
	Use:   "meeting-join",
	Short: "机器人按会议号加入会议",
	Long: `机器人按会议号加入会议。

使用飞书 POST /open-apis/vc/v1/bots/join API。

必填:
  --meeting-number   要加入的会议号

可选:
  --password         会议密码（如会议设了密码）
  --dry-run          只打印将要发送的请求体，不实际调用
  --output, -o       输出格式（json）

权限:
  vc:meeting.bot.join:write

示例:
  feishu-cli vc bot meeting-join --meeting-number 123456789
  feishu-cli vc bot meeting-join --meeting-number 123456789 --password 1234 --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		meetingNo, _ := cmd.Flags().GetString("meeting-number")
		password, _ := cmd.Flags().GetString("password")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		output, _ := cmd.Flags().GetString("output")

		if strings.TrimSpace(meetingNo) == "" {
			return fmt.Errorf("--meeting-number 必填")
		}

		if dryRun {
			// 复用 client 端 body 构造器，保证预览与真实请求同源（join_type/join_identify 不漏）。
			body := client.BuildVCBotJoinBody(client.VCBotJoinReq{MeetingNo: meetingNo, Password: password})
			return printJSON(map[string]any{
				"method": "POST",
				"path":   "/open-apis/vc/v1/bots/join",
				"body":   body,
			})
		}

		token, err := requireUserToken(cmd, "vc bot meeting-join")
		if err != nil {
			return err
		}

		data, err := client.VCBotJoinMeeting(client.VCBotJoinReq{
			MeetingNo: meetingNo,
			Password:  password,
		}, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(json.RawMessage(data))
		}
		fmt.Println("机器人入会成功。")
		if len(data) > 0 && string(data) != "null" {
			fmt.Println(string(data))
		}
		return nil
	},
}

var vcBotLeaveCmd = &cobra.Command{
	Use:   "meeting-leave",
	Short: "机器人离开会议",
	Long: `机器人离开会议。

使用飞书 POST /open-apis/vc/v1/bots/leave API。

必填:
  --meeting-id   要离开的会议 ID

可选:
  --dry-run      只打印将要发送的请求体，不实际调用
  --output, -o   输出格式（json）

权限:
  vc:meeting.bot.leave:write

示例:
  feishu-cli vc bot meeting-leave --meeting-id 6911188411932033028`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		meetingID, _ := cmd.Flags().GetString("meeting-id")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		output, _ := cmd.Flags().GetString("output")

		if strings.TrimSpace(meetingID) == "" {
			return fmt.Errorf("--meeting-id 必填")
		}

		if dryRun {
			return printJSON(map[string]any{
				"method": "POST",
				"path":   "/open-apis/vc/v1/bots/leave",
				"body":   map[string]any{"meeting_id": meetingID},
			})
		}

		token, err := requireUserToken(cmd, "vc bot meeting-leave")
		if err != nil {
			return err
		}

		data, err := client.VCBotLeaveMeeting(meetingID, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(json.RawMessage(data))
		}
		fmt.Println("机器人离会成功。")
		if len(data) > 0 && string(data) != "null" {
			fmt.Println(string(data))
		}
		return nil
	},
}

var vcBotEventsCmd = &cobra.Command{
	Use:   "meeting-events",
	Short: "查询机器人会议事件",
	Long: `按会议 ID 查询机器人会议事件。

使用飞书 GET /open-apis/vc/v1/bots/events API。

必填:
  --meeting-id   要查询的会议 ID

可选:
  --start        起始时间（YYYY-MM-DD / RFC3339 / Unix 秒）
  --end          结束时间（YYYY-MM-DD / RFC3339 / Unix 秒；纯日期对齐到 23:59:59）
  --page-size    每页数量（20-100；不传或 0 = 用默认 20）
  --page-token   分页标记
  --dry-run      只打印将要发送的请求参数，不实际调用
  --output, -o   输出格式（json）

示例:
  feishu-cli vc bot meeting-events --meeting-id 6911188411932033028
  feishu-cli vc bot meeting-events --meeting-id 6911188411932033028 --start 2026-03-01 --end 2026-03-31 -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		meetingID, _ := cmd.Flags().GetString("meeting-id")
		startStr, _ := cmd.Flags().GetString("start")
		endStr, _ := cmd.Flags().GetString("end")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		output, _ := cmd.Flags().GetString("output")

		if strings.TrimSpace(meetingID) == "" {
			return fmt.Errorf("--meeting-id 必填")
		}

		startSec, err := vcParseTimeToUnixSec(startStr, false)
		if err != nil {
			return fmt.Errorf("解析 --start 失败: %w", err)
		}
		endSec, err := vcParseTimeToUnixSec(endStr, true)
		if err != nil {
			return fmt.Errorf("解析 --end 失败: %w", err)
		}
		after, err := vcStartAfterEnd(startSec, endSec)
		if err != nil {
			return err
		}
		if after {
			return fmt.Errorf("--start 不能晚于 --end")
		}

		if err := validateVCPageSize(pageSize); err != nil {
			return err
		}
		if pageSize == 0 {
			pageSize = 20
		}

		req := client.VCBotEventsReq{
			MeetingID:    meetingID,
			StartTimeSec: startSec,
			EndTimeSec:   endSec,
			PageSize:     pageSize,
			PageToken:    pageToken,
		}

		if dryRun {
			// 预览只放真实请求会带上的参数（与 client 端 set 逻辑一致：空值不发）。
			query := map[string]any{
				"meeting_id": req.MeetingID,
				"page_size":  req.PageSize,
			}
			if req.StartTimeSec != "" {
				query["start_time"] = req.StartTimeSec
			}
			if req.EndTimeSec != "" {
				query["end_time"] = req.EndTimeSec
			}
			if req.PageToken != "" {
				query["page_token"] = req.PageToken
			}
			return printJSON(map[string]any{
				"method": "GET",
				"path":   "/open-apis/vc/v1/bots/events",
				"query":  query,
			})
		}

		token, err := requireUserToken(cmd, "vc bot meeting-events")
		if err != nil {
			return err
		}

		data, err := client.VCBotMeetingEvents(req, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(json.RawMessage(data))
		}

		var parsed struct {
			MeetingEventList []json.RawMessage `json:"meeting_event_list"`
			HasMore          bool              `json:"has_more"`
			PageToken        string            `json:"page_token"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			fmt.Println(string(data))
			return nil
		}
		fmt.Printf("机器人会议事件（共 %d 条）:\n\n", len(parsed.MeetingEventList))
		for i, ev := range parsed.MeetingEventList {
			fmt.Printf("[%d] %s\n", i+1, string(ev))
		}
		if parsed.HasMore {
			fmt.Printf("\n还有更多，可用 --page-token %s 获取下一页\n", parsed.PageToken)
		}
		return nil
	},
}

func init() {
	vcCmd.AddCommand(vcBotCmd)
	vcBotCmd.AddCommand(vcBotJoinCmd)
	vcBotCmd.AddCommand(vcBotLeaveCmd)
	vcBotCmd.AddCommand(vcBotEventsCmd)

	vcBotJoinCmd.Flags().String("meeting-number", "", "要加入的会议号（必填）")
	vcBotJoinCmd.Flags().String("password", "", "会议密码（可选）")
	vcBotJoinCmd.Flags().Bool("dry-run", false, "只打印请求体，不实际调用")
	vcBotJoinCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	vcBotJoinCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(vcBotJoinCmd, "meeting-number")

	vcBotLeaveCmd.Flags().String("meeting-id", "", "要离开的会议 ID（必填）")
	vcBotLeaveCmd.Flags().Bool("dry-run", false, "只打印请求体，不实际调用")
	vcBotLeaveCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	vcBotLeaveCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(vcBotLeaveCmd, "meeting-id")

	vcBotEventsCmd.Flags().String("meeting-id", "", "要查询的会议 ID（必填）")
	vcBotEventsCmd.Flags().String("start", "", "起始时间（YYYY-MM-DD / RFC3339 / Unix 秒）")
	vcBotEventsCmd.Flags().String("end", "", "结束时间（YYYY-MM-DD / RFC3339 / Unix 秒）")
	vcBotEventsCmd.Flags().Int("page-size", 20, "每页数量（20-100；不传或 0 = 用默认 20）")
	vcBotEventsCmd.Flags().String("page-token", "", "分页标记")
	vcBotEventsCmd.Flags().Bool("dry-run", false, "只打印请求参数，不实际调用")
	vcBotEventsCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	vcBotEventsCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(vcBotEventsCmd, "meeting-id")
}
