package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
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
		return clierr.Usagef("--page-size 取值范围 20-100（当前 %d）", pageSize)
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
  - meeting-leave 需要 vc:meeting.bot.join:write（与入会同一 scope；官方无独立 leave scope）
  - meeting-events：User 身份需要 vc:meeting.meetingevent:read；
    Bot 身份需要 vc:meeting.meetingevent:read 与 vc:meeting.bot.join:write 任一（应用身份权限，开通其一即可）

身份:
  meeting-join / meeting-leave 仅支持 Bot 身份（默认使用 Bot/Tenant Access Token；对齐官方，
  传 --user-access-token 会直接报错，不会以用户身份入会/离会）。
  meeting-events 必须显式用 --as bot|user|auto 选择身份，禁止静默回落：
    --as user  强制 User Token（缺失即失败）
    --as bot   强制 Bot/Tenant Token（即使已登录也不改用 User）
    --as auto  已登录用 User，未登录用 Bot（默认）
  读取身份必须与 meeting_id 来源一致；Bot 身份要求机器人在会中。
  meeting_id 可用 feishu-cli vc meeting list-active 获取当前进行中的会议。

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
	Long: `机器人按会议号加入会议（仅 Bot 身份）。

使用飞书 POST /open-apis/vc/v1/bots/join API。

必填:
  --meeting-number   要加入的会议号（9 位数字）

可选:
  --password         会议密码（如会议设了密码）
  --call-id          邀请事件透传的关联 ID（call_id）
  --action           join（默认，加入会议）/ start（发起日程会议）
  --dry-run          只打印将要发送的请求体，不实际调用
  --output, -o       输出格式（json）

身份:
  仅支持 Bot 身份，默认使用 Bot/Tenant 身份；传 --user-access-token 会直接报错（对齐官方）。

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
		callID, _ := cmd.Flags().GetString("call-id")
		action, _ := cmd.Flags().GetString("action")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		output, _ := cmd.Flags().GetString("output")

		if err := rejectVCBotUserToken(cmd, "meeting-join"); err != nil {
			return err
		}
		meetingNo = strings.TrimSpace(meetingNo)
		if !meetingNoPattern.MatchString(meetingNo) {
			return clierr.Usagef("--meeting-number 必须是 9 位数字会议号，得到 %q", meetingNo)
		}
		action = strings.ToLower(strings.TrimSpace(action))
		if action == "" {
			action = "join"
		}
		if action != "join" && action != "start" {
			return clierr.Usagef("--action 仅支持 join / start，得到 %q", action)
		}

		req := client.VCBotJoinReq{
			MeetingNo: meetingNo,
			Password:  strings.TrimSpace(password),
			CallID:    strings.TrimSpace(callID),
			Start:     action == "start",
		}

		if dryRun {
			// 复用 client 端 body 构造器，保证预览与真实请求同源（join_type/join_identify 不漏）。
			return printJSON(map[string]any{
				"method": "POST",
				"path":   "/open-apis/vc/v1/bots/join",
				"body":   client.BuildVCBotJoinBody(req),
				"as":     "bot",
			})
		}

		// 仅 Bot 身份：空 token 即 App/Tenant Token
		data, err := client.VCBotJoinMeeting(req, "")
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
	Long: `机器人离开会议（仅 Bot 身份）。

使用飞书 POST /open-apis/vc/v1/bots/leave API。

必填:
  --meeting-id   要离开的会议 ID

可选:
  --dry-run      只打印将要发送的请求体，不实际调用
  --output, -o   输出格式（json）

身份:
  仅支持 Bot 身份，默认使用 Bot/Tenant 身份；传 --user-access-token 会直接报错（对齐官方）。

权限:
  vc:meeting.bot.join:write

示例:
  feishu-cli vc bot meeting-leave --meeting-id 6911188411932033028`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		meetingID, _ := cmd.Flags().GetString("meeting-id")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		output, _ := cmd.Flags().GetString("output")

		if err := rejectVCBotUserToken(cmd, "meeting-leave"); err != nil {
			return err
		}
		meetingID = strings.TrimSpace(meetingID)
		if meetingID == "" {
			return clierr.Usagef("--meeting-id 必填")
		}

		if dryRun {
			return printJSON(map[string]any{
				"method": "POST",
				"path":   "/open-apis/vc/v1/bots/leave",
				"body":   map[string]any{"meeting_id": meetingID},
				"as":     "bot",
			})
		}

		// 仅 Bot 身份：空 token 即 App/Tenant Token
		data, err := client.VCBotLeaveMeeting(meetingID, "")
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

使用飞书 GET /open-apis/vc/v1/bots/events API（事件列表字段为 events）。

必填:
  --meeting-id   要查询的会议 ID（长数字 meeting_id，不是 9 位会议号；
                 可用 vc meeting list-active 或 vc detail <会议号> 获取）

可选:
  --start        起始时间（YYYY-MM-DD / RFC3339 / Unix 秒）
  --end          结束时间（YYYY-MM-DD / RFC3339 / Unix 秒；纯日期对齐到 23:59:59）
  --page-size    每页数量（20-100；不传或 0 = 用默认 20）
  --page-token   分页标记
  --page-all     自动翻页拉取全部事件（每页 100，最多 200 页，重复游标即停止）
  --dry-run      只打印将要发送的请求参数，不实际调用
  --output, -o   输出格式（json）
  --as           身份：bot | user | auto（默认 auto）。必须与 meeting_id 来源一致；
                 --as user 缺 Token 失败；--as bot 即使已登录也走 Bot
  --user-access-token 覆盖登录态（仅 --as user/auto 使用）

权限:
  - User 身份: vc:meeting.meetingevent:read
  - Bot 身份:  vc:meeting.meetingevent:read 与 vc:meeting.bot.join:write 任一（应用身份权限，开通其一即可；机器人须在会中）

示例:
  feishu-cli vc bot meeting-events --meeting-id 6911188411932033028 --as user
  feishu-cli vc bot meeting-events --meeting-id 6911188411932033028 --as bot --dry-run
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
		pageAll, _ := cmd.Flags().GetBool("page-all")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		output, _ := cmd.Flags().GetString("output")

		meetingID = strings.TrimSpace(meetingID)
		if err := validateVCEventsMeetingID(meetingID); err != nil {
			return err
		}

		startSec, err := vcParseTimeToUnixSec(startStr, false)
		if err != nil {
			return clierr.Usagef("解析 --start 失败: %v", err)
		}
		endSec, err := vcParseTimeToUnixSec(endStr, true)
		if err != nil {
			return clierr.Usagef("解析 --end 失败: %v", err)
		}
		after, err := vcStartAfterEnd(startSec, endSec)
		if err != nil {
			return err
		}
		if after {
			return clierr.Usagef("--start 不能晚于 --end")
		}

		if err := validateVCPageSize(pageSize); err != nil {
			return err
		}
		if pageSize == 0 {
			pageSize = 20
		}
		if pageAll {
			// 自动翻页取上限 100，减少往返（对齐官方 --page-all）
			pageSize = vcEventsMaxPageSize
		}

		req := client.VCBotEventsReq{
			MeetingID:    meetingID,
			StartTimeSec: startSec,
			EndTimeSec:   endSec,
			PageSize:     pageSize,
			PageToken:    pageToken,
		}

		if dryRun {
			identity, err := peekVCBotEventsIdentity(cmd)
			if err != nil {
				return err
			}
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
			preview := map[string]any{
				"method": "GET",
				"path":   "/open-apis/vc/v1/bots/events",
				"query":  query,
				"as":     identity,
			}
			if pageAll {
				preview["page_all"] = true
			}
			return printJSON(preview)
		}

		token, _, err := resolveVCBotEventsIdentity(cmd)
		if err != nil {
			return err
		}

		var data json.RawMessage
		if pageAll {
			data, err = fetchAllVCBotEvents(req, token)
		} else {
			data, err = client.VCBotMeetingEvents(req, token)
		}
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(json.RawMessage(data))
		}
		return printVCBotEventsText(data)
	},
}

// vcEventsMaxPageSize meeting-events 单页上限；vcEventsMaxPages 自动翻页页数上限（对齐官方 200 页）
const (
	vcEventsMaxPageSize = 100
	vcEventsMaxPages    = 200
)

// validateVCEventsMeetingID meeting-events 只收长数字 meeting_id：
// 9 位会议号（meeting_no）是常见误传，服务端只会返回难懂的错误，这里前置拦截并给出获取 meeting_id 的方法。
func validateVCEventsMeetingID(meetingID string) error {
	if meetingID == "" {
		return clierr.Usagef("--meeting-id 必填")
	}
	if meetingNoPattern.MatchString(meetingID) {
		return clierr.Usagef("--meeting-id 需要长数字 meeting_id，%q 是 9 位会议号；"+
			"请用 `feishu-cli vc meeting list-active` 或 `feishu-cli vc detail %s` 获取 meeting_id", meetingID, meetingID)
	}
	if v, err := strconv.ParseInt(meetingID, 10, 64); err != nil || v <= 0 {
		return clierr.Usagef("--meeting-id 必须是正整数 meeting_id，得到 %q", meetingID)
	}
	return nil
}

// fetchAllVCBotEvents 自动翻页拉取全部会议事件：合并 events，最后一页的 has_more / page_token 原样保留。
// 游标为空或重复即停止；达到页数上限仍 has_more 时在 stderr 告警（page_token 留在输出中可续翻）。
func fetchAllVCBotEvents(req client.VCBotEventsReq, token string) (json.RawMessage, error) {
	var (
		all      []json.RawMessage
		last     map[string]json.RawMessage
		hasMore  bool
		nextTok  string
		seenTok  = map[string]bool{}
		finished bool
	)
	for page := 0; page < vcEventsMaxPages; page++ {
		data, err := client.VCBotMeetingEvents(req, token)
		if err != nil {
			return nil, err
		}
		var parsed map[string]json.RawMessage
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, fmt.Errorf("解析会议事件失败: %w", err)
		}
		last = parsed
		all = append(all, vcBotEventItems(data)...)
		hasMore = false
		nextTok = ""
		_ = json.Unmarshal(parsed["has_more"], &hasMore)
		_ = json.Unmarshal(parsed["page_token"], &nextTok)
		if !hasMore || nextTok == "" || seenTok[nextTok] {
			finished = true
			break
		}
		seenTok[nextTok] = true
		req.PageToken = nextTok
	}
	if !finished && hasMore {
		fmt.Fprintf(os.Stderr, "警告：已达到自动翻页上限 %d 页，仍有更多事件；可用 --page-token %s 继续\n", vcEventsMaxPages, nextTok)
	}
	if last == nil {
		last = map[string]json.RawMessage{}
	}
	if all == nil {
		all = []json.RawMessage{}
	}
	eventsJSON, _ := json.Marshal(all)
	last["events"] = eventsJSON
	delete(last, "meeting_event_list")
	hasMoreJSON, _ := json.Marshal(hasMore)
	last["has_more"] = hasMoreJSON
	tokJSON, _ := json.Marshal(nextTok)
	last["page_token"] = tokJSON
	return json.Marshal(last)
}

// vcBotEventItems 取事件列表：官方与服务端字段为 events；兼容历史误用的 meeting_event_list。
func vcBotEventItems(data json.RawMessage) []json.RawMessage {
	var parsed struct {
		Events           []json.RawMessage `json:"events"`
		MeetingEventList []json.RawMessage `json:"meeting_event_list"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil
	}
	if parsed.Events != nil {
		return parsed.Events
	}
	return parsed.MeetingEventList
}

// printVCBotEventsText 文本模式输出会议事件：时间 + 类型 + 事件原文（紧凑 JSON）
func printVCBotEventsText(data json.RawMessage) error {
	var meta struct {
		HasMore   bool   `json:"has_more"`
		PageToken string `json:"page_token"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		fmt.Println(string(data))
		return nil
	}
	events := vcBotEventItems(data)
	fmt.Printf("机器人会议事件（共 %d 条）:\n\n", len(events))
	for i, ev := range events {
		var head struct {
			EventType string          `json:"event_type"`
			Type      string          `json:"type"`
			EventTime json.RawMessage `json:"event_time"`
		}
		_ = json.Unmarshal(ev, &head)
		evType := head.EventType
		if evType == "" {
			evType = head.Type
		}
		evTime := strings.Trim(string(head.EventTime), `"`)
		if evTime != "" && evTime != "null" {
			evTime = formatVCTime(evTime)
		} else {
			evTime = ""
		}
		label := strings.TrimSpace(evTime + " " + evType)
		if label != "" {
			fmt.Printf("[%d] %s\n    %s\n", i+1, label, string(ev))
		} else {
			fmt.Printf("[%d] %s\n", i+1, string(ev))
		}
	}
	if meta.HasMore {
		fmt.Printf("\n还有更多，可用 --page-token %s 获取下一页（或 --page-all 自动翻页）\n", meta.PageToken)
	}
	return nil
}

// rejectVCBotUserToken meeting-join / meeting-leave 仅支持 Bot 身份（官方 #2570）：
// 显式传 --user-access-token 时报用法错误，绝不静默改成以 Bot 身份入会（入会主体变化是可见副作用）。
func rejectVCBotUserToken(cmd *cobra.Command, name string) error {
	if strings.TrimSpace(resolveFlagUserToken(cmd)) != "" {
		return clierr.Usagef("%s 仅支持 Bot 身份（接口只接受应用身份），请去掉 --user-access-token 后重试", name)
	}
	return nil
}

func init() {
	vcCmd.AddCommand(vcBotCmd)
	vcBotCmd.AddCommand(vcBotJoinCmd)
	vcBotCmd.AddCommand(vcBotLeaveCmd)
	vcBotCmd.AddCommand(vcBotEventsCmd)

	vcBotJoinCmd.Flags().String("meeting-number", "", "要加入的会议号（9 位数字，必填）")
	vcBotJoinCmd.Flags().String("password", "", "会议密码（可选）")
	vcBotJoinCmd.Flags().String("call-id", "", "邀请事件透传的关联 ID（可选）")
	vcBotJoinCmd.Flags().String("action", "join", "join（加入会议）/ start（发起日程会议）")
	vcBotJoinCmd.Flags().Bool("dry-run", false, "只打印请求体，不实际调用")
	vcBotJoinCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	vcBotJoinCmd.Flags().String("user-access-token", "", "已废弃：该接口仅支持 Bot 身份（默认 Bot/Tenant 身份），传入将报错")
	mustMarkFlagRequired(vcBotJoinCmd, "meeting-number")

	vcBotLeaveCmd.Flags().String("meeting-id", "", "要离开的会议 ID（必填）")
	vcBotLeaveCmd.Flags().Bool("dry-run", false, "只打印请求体，不实际调用")
	vcBotLeaveCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	vcBotLeaveCmd.Flags().String("user-access-token", "", "已废弃：该接口仅支持 Bot 身份（默认 Bot/Tenant 身份），传入将报错")
	mustMarkFlagRequired(vcBotLeaveCmd, "meeting-id")

	vcBotEventsCmd.Flags().String("meeting-id", "", "要查询的会议 ID（必填）")
	vcBotEventsCmd.Flags().String("start", "", "起始时间（YYYY-MM-DD / RFC3339 / Unix 秒）")
	vcBotEventsCmd.Flags().String("end", "", "结束时间（YYYY-MM-DD / RFC3339 / Unix 秒）")
	vcBotEventsCmd.Flags().Int("page-size", 20, "每页数量（20-100；不传或 0 = 用默认 20）")
	vcBotEventsCmd.Flags().String("page-token", "", "分页标记")
	vcBotEventsCmd.Flags().Bool("page-all", false, "自动翻页拉取全部事件（每页 100，最多 200 页）")
	vcBotEventsCmd.Flags().Bool("dry-run", false, "只打印请求参数，不实际调用")
	vcBotEventsCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	vcBotEventsCmd.Flags().String("as", "auto", "身份：bot | user | auto（默认 auto；须与 meeting_id 来源一致）")
	vcBotEventsCmd.Flags().String("user-access-token", "", "User Access Token（仅 --as user/auto 使用；--as bot 忽略）")
	mustMarkFlagRequired(vcBotEventsCmd, "meeting-id")
}

// peekVCBotEventsIdentity 供 dry-run：只静态探测身份，不刷新、不联网、不写 token。
func peekVCBotEventsIdentity(cmd *cobra.Command) (identity string, err error) {
	as, _ := cmd.Flags().GetString("as")
	switch strings.ToLower(strings.TrimSpace(as)) {
	case "", "auto":
		flagToken, _ := cmd.Flags().GetString("user-access-token")
		if auth.HasUserTokenConfigured(flagToken, config.Get().UserAccessToken) {
			return "user", nil
		}
		return "bot", nil
	case "bot", "tenant", "app":
		return "bot", nil
	case "user":
		flagToken, _ := cmd.Flags().GetString("user-access-token")
		if !auth.HasUserTokenConfigured(flagToken, config.Get().UserAccessToken) {
			// 与实调一致：缺 User Token 属鉴权类错误（exit 3）
			return "", clierr.Authf("--as user 需要 User Access Token（请先 `feishu-cli auth login`，或改用 --as bot）")
		}
		return "user", nil
	default:
		return "", clierr.Usagef("--as 仅支持 bot|user|auto，得到 %q", as)
	}
}

// resolveVCBotEventsIdentity 解析 meeting-events 实调身份。
// --as auto 走 resolveAutoUserToken：未配置 User 才回落 Bot；刷新/读 token 失败 fail-closed，禁止静默切 Bot。
func resolveVCBotEventsIdentity(cmd *cobra.Command) (token string, identity string, err error) {
	as, _ := cmd.Flags().GetString("as")
	switch strings.ToLower(strings.TrimSpace(as)) {
	case "", "auto":
		token, err = resolveAutoUserToken(cmd)
		if err != nil {
			return "", "", err
		}
		if token != "" {
			return token, "user", nil
		}
		return "", "bot", nil
	case "bot", "tenant", "app":
		return "", "bot", nil
	case "user":
		token, err = resolveRequiredUserToken(cmd)
		if err != nil {
			return "", "", fmt.Errorf("--as user 需要 User Access Token（请先 `feishu-cli auth login`，或改用 --as bot）: %w", err)
		}
		return token, "user", nil
	default:
		return "", "", clierr.Usagef("--as 仅支持 bot|user|auto，得到 %q", as)
	}
}
