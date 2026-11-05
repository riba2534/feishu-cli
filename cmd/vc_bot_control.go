package cmd

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/safefile"
	"github.com/spf13/cobra"
)

// 会中控制命令（对齐官方 vc +meeting-countdown / +meeting-message-send / +meeting-screenshot /
// +meeting-invite / +meeting-end）。countdown / message-send / screenshot 支持 User 与 Bot（--as 默认 auto，
// 规则同 meeting-events）；invite / end 仅 Bot。

const (
	vcCountdownSet          = "set"
	vcCountdownProlong      = "prolong"
	vcCountdownEndInAdvance = "end_in_advance"
	vcCountdownCloseWindow  = "close_window"

	vcMessageMaxTextBytes = 48 * 1024
	vcMessageMaxUUIDBytes = 128
	vcInviteeLimit        = 200
)

var vcCountdownActions = []string{vcCountdownSet, vcCountdownProlong, vcCountdownEndInAdvance, vcCountdownCloseWindow}

// vcCountdownInput 是 meeting-countdown 的参数；*Set 表示对应 flag 是否显式传入。
type vcCountdownInput struct {
	MeetingID    string
	Action       string
	Duration     int
	DurationSet  bool
	Reminder     int
	ReminderSet  bool
	PlayAudio    bool
	PlayAudioSet bool
}

// buildVCCountdownBody 校验并构造倒计时请求体（规则对齐官方 buildMeetingCountdownBody）。
func buildVCCountdownBody(in vcCountdownInput) (map[string]any, error) {
	action := strings.ToLower(strings.TrimSpace(in.Action))
	if action == "" {
		return nil, clierr.Usagef("--action 必填，可选值: %s", strings.Join(vcCountdownActions, ", "))
	}
	if err := validateEnum(action, "--action", vcCountdownActions); err != nil {
		return nil, err
	}
	switch action {
	case vcCountdownSet, vcCountdownProlong:
		if !in.DurationSet || in.Duration <= 0 {
			return nil, clierr.Usagef("--action %s 时 --duration 必须是正整数（分钟）", action)
		}
	default:
		if in.DurationSet {
			return nil, clierr.Usagef("--duration 仅在 --action set / prolong 时有效")
		}
	}
	if in.ReminderSet {
		if action != vcCountdownSet {
			return nil, clierr.Usagef("--reminder-before-end 仅在 --action set 时有效")
		}
		if in.Reminder <= 0 {
			return nil, clierr.Usagef("--reminder-before-end 必须是正整数（分钟）")
		}
		if in.Reminder >= in.Duration {
			return nil, clierr.Usagef("--reminder-before-end 必须小于 --duration")
		}
	}
	if in.PlayAudioSet && action != vcCountdownSet {
		return nil, clierr.Usagef("--need-play-audio-at-end 仅在 --action set 时有效")
	}
	body := map[string]any{"meeting_id": in.MeetingID, "action": action}
	if action == vcCountdownSet || action == vcCountdownProlong {
		body["duration"] = in.Duration
	}
	if action == vcCountdownSet {
		if in.PlayAudioSet {
			body["need_play_audio_at_end"] = in.PlayAudio
		}
		if in.ReminderSet {
			body["reminder_before_end"] = in.Reminder
		}
	}
	return body, nil
}

// buildVCMessageBody 校验并构造会中消息请求体（规则对齐官方 buildMeetingMessageSendBody）：
// 未传 --msg-type 时按 --text / --emoji-type 二者其一推断。
func buildVCMessageBody(meetingID, msgType, text, emoji, uuid string) (map[string]any, error) {
	msgType = strings.ToLower(strings.TrimSpace(msgType))
	text = strings.TrimSpace(text)
	emoji = strings.TrimSpace(emoji)
	uuid = strings.TrimSpace(uuid)
	if msgType == "" {
		switch {
		case text != "" && emoji == "":
			msgType = "text"
		case text == "" && emoji != "":
			msgType = "reaction"
		default:
			return nil, clierr.Usagef("--text 与 --emoji-type 需且只能传一个；同时传或都不传时请用 --msg-type text|reaction 指明")
		}
	}
	body := map[string]any{"meeting_id": meetingID, "msg_type": msgType}
	switch msgType {
	case "text":
		if text == "" {
			return nil, clierr.Usagef("--msg-type text 时 --text 必填")
		}
		if emoji != "" {
			return nil, clierr.Usagef("--msg-type text 时不能传 --emoji-type")
		}
		if len(text) > vcMessageMaxTextBytes {
			return nil, clierr.Usagef("--text 过长：最多 %d 字节，当前 %d 字节", vcMessageMaxTextBytes, len(text))
		}
		body["content"] = text
	case "reaction":
		if emoji == "" {
			return nil, clierr.Usagef("--msg-type reaction 时 --emoji-type 必填（如 THUMBSUP、LOVE）")
		}
		if text != "" {
			return nil, clierr.Usagef("--msg-type reaction 时不能传 --text")
		}
		body["content"] = emoji
	default:
		return nil, clierr.Usagef("--msg-type 只能是 text 或 reaction，得到 %q", msgType)
	}
	if len(uuid) > vcMessageMaxUUIDBytes {
		return nil, clierr.Usagef("--uuid 过长：最多 %d 字节", vcMessageMaxUUIDBytes)
	}
	if uuid != "" {
		body["uuid"] = uuid
	}
	return body, nil
}

// buildVCInviteBody 校验并构造邀请请求体：all_suggested → invite_type=1；selected → invite_type=2 + invitees。
func buildVCInviteBody(meetingID, inviteType string, openIDs []string) (map[string]any, error) {
	inviteType = strings.ToLower(strings.TrimSpace(inviteType))
	seen := map[string]bool{}
	var ids []string
	for _, id := range openIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	body := map[string]any{"meeting_id": meetingID}
	switch inviteType {
	case "all_suggested":
		if len(ids) > 0 {
			return nil, clierr.Usagef("--type all_suggested 时不能传 --open-ids")
		}
		body["invite_type"] = 1
	case "selected":
		if len(ids) == 0 {
			return nil, clierr.Usagef("--type selected 时 --open-ids 必填")
		}
		if len(ids) > vcInviteeLimit {
			return nil, clierr.Usagef("--open-ids 最多 %d 个，当前 %d 个", vcInviteeLimit, len(ids))
		}
		invitees := make([]map[string]any, 0, len(ids))
		for _, id := range ids {
			if !strings.HasPrefix(id, "ou_") {
				return nil, clierr.Usagef("--open-ids 只接受用户 open_id（ou_ 开头），得到 %q", id)
			}
			invitees = append(invitees, map[string]any{"id": id, "user_type": 1})
		}
		body["invite_type"] = 2
		body["invitees"] = invitees
	case "":
		return nil, clierr.Usagef("--type 必填：all_suggested 或 selected")
	default:
		return nil, clierr.Usagef("--type 只能是 all_suggested 或 selected，得到 %q", inviteType)
	}
	return body, nil
}

// vcScreenshotNow 便于测试固定默认文件名中的时间戳。
var vcScreenshotNow = time.Now

// vcScreenshotOutputPath 返回截图输出路径：未传 --output 时为 meeting-screenshots/<meeting_id>-<UTC 时间>.jpg。
func vcScreenshotOutputPath(output, meetingID string) string {
	if p := strings.TrimSpace(output); p != "" {
		return p
	}
	stamp := vcScreenshotNow().UTC().Format("20060102T150405.000Z")
	return filepath.Join("meeting-screenshots", fmt.Sprintf("%s-%s.jpg", meetingID, stamp))
}

func printVCBotDryRun(cmd *cobra.Command, path string, params map[string]any, body any, identity string, extra map[string]any) error {
	preview := map[string]any{
		"dry_run": true,
		"method":  "POST",
		"path":    path,
		"body":    body,
		"as":      identity,
	}
	if len(params) > 0 {
		preview["query"] = params
	}
	for k, v := range extra {
		preview[k] = v
	}
	return printJSON(preview)
}

func printVCBotResult(output string, data json.RawMessage, text string) error {
	if output == "json" {
		if len(data) == 0 || string(data) == "null" {
			data = json.RawMessage("{}")
		}
		return printJSON(data)
	}
	fmt.Println(text)
	if len(data) > 0 && string(data) != "null" && string(data) != "{}" {
		fmt.Println(string(data))
	}
	return nil
}

var vcBotCountdownCmd = &cobra.Command{
	Use:   "meeting-countdown",
	Short: "操作会中倒计时（设置 / 延长 / 提前结束 / 关闭窗口）",
	Long: `操作会中倒计时（POST /open-apis/vc/v1/bots/countdown）。

必填:
  --meeting-id   长数字 meeting_id（不是 9 位会议号；可用 vc meeting list-active 获取）
  --action       set（设置）/ prolong（延长）/ end_in_advance（提前结束）/ close_window（关闭倒计时窗口）

可选:
  --duration                倒计时时长（分钟，正整数）；set / prolong 时必填，其余 action 不能传
  --reminder-before-end     结束前提醒（分钟，须小于 --duration）；仅 set
  --need-play-audio-at-end  倒计时结束时播放提示音；仅 set
  --as                      bot | user | auto（默认 auto，规则同 vc bot meeting-events）
  --dry-run                 只打印请求，不调用（不读取凭据、不联网）
  --output, -o              输出格式（json）

权限: vc:meeting.interaction:write

示例:
  feishu-cli vc bot meeting-countdown --meeting-id 6911188411932033028 --action set --duration 10 --reminder-before-end 2
  feishu-cli vc bot meeting-countdown --meeting-id 6911188411932033028 --action prolong --duration 5 --as bot
  feishu-cli vc bot meeting-countdown --meeting-id 6911188411932033028 --action close_window --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		meetingID, _ := cmd.Flags().GetString("meeting-id")
		meetingID = strings.TrimSpace(meetingID)
		if err := validateVCEventsMeetingID(meetingID); err != nil {
			return err
		}
		in := vcCountdownInput{MeetingID: meetingID}
		in.Action, _ = cmd.Flags().GetString("action")
		in.Duration, _ = cmd.Flags().GetInt("duration")
		in.DurationSet = cmd.Flags().Changed("duration")
		in.Reminder, _ = cmd.Flags().GetInt("reminder-before-end")
		in.ReminderSet = cmd.Flags().Changed("reminder-before-end")
		in.PlayAudio, _ = cmd.Flags().GetBool("need-play-audio-at-end")
		in.PlayAudioSet = cmd.Flags().Changed("need-play-audio-at-end")
		body, err := buildVCCountdownBody(in)
		if err != nil {
			return err
		}
		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			identity, err := peekVCBotEventsIdentity(cmd)
			if err != nil {
				return err
			}
			return printVCBotDryRun(cmd, "/open-apis/vc/v1/bots/countdown", nil, body, identity, nil)
		}
		if err := config.Validate(); err != nil {
			return err
		}
		token, _, err := resolveVCBotEventsIdentity(cmd)
		if err != nil {
			return err
		}
		data, err := client.VCBotCountdown(body, token)
		if err != nil {
			return err
		}
		output, _ := cmd.Flags().GetString("output")
		return printVCBotResult(output, data, fmt.Sprintf("会中倒计时已操作（action=%s）。", body["action"]))
	},
}

var vcBotMessageSendCmd = &cobra.Command{
	Use:   "meeting-message-send",
	Short: "在会中发送文本消息或表情",
	Long: `在会中发送文本消息或表情（POST /open-apis/vc/v1/bots/message）。

必填:
  --meeting-id   长数字 meeting_id（不是 9 位会议号）
  --text 或 --emoji-type 二选一（未传 --msg-type 时按此推断类型）

可选:
  --msg-type     text | reaction（同时传或都不传 --text/--emoji-type 时必填）
  --text         文本内容（最多 48KB）
  --emoji-type   表情 key，如 THUMBSUP、LOVE、VC_NoSound
  --uuid         幂等键（≤128 字节；同一 uuid 重复发送不会产生多条）
  --as           bot | user | auto（默认 auto，规则同 vc bot meeting-events）
  --dry-run      只打印请求，不调用（不读取凭据、不联网）
  --output, -o   输出格式（json）

权限: vc:meeting.message:write

示例:
  feishu-cli vc bot meeting-message-send --meeting-id 6911188411932033028 --text "会议还剩 5 分钟" --uuid remind-5min
  feishu-cli vc bot meeting-message-send --meeting-id 6911188411932033028 --emoji-type THUMBSUP --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		meetingID, _ := cmd.Flags().GetString("meeting-id")
		meetingID = strings.TrimSpace(meetingID)
		if err := validateVCEventsMeetingID(meetingID); err != nil {
			return err
		}
		msgType, _ := cmd.Flags().GetString("msg-type")
		text, _ := cmd.Flags().GetString("text")
		emoji, _ := cmd.Flags().GetString("emoji-type")
		uuid, _ := cmd.Flags().GetString("uuid")
		body, err := buildVCMessageBody(meetingID, msgType, text, emoji, uuid)
		if err != nil {
			return err
		}
		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			identity, err := peekVCBotEventsIdentity(cmd)
			if err != nil {
				return err
			}
			return printVCBotDryRun(cmd, "/open-apis/vc/v1/bots/message", nil, body, identity, nil)
		}
		if err := config.Validate(); err != nil {
			return err
		}
		token, _, err := resolveVCBotEventsIdentity(cmd)
		if err != nil {
			return err
		}
		data, err := client.VCBotMessageSend(body, token)
		if err != nil {
			return err
		}
		output, _ := cmd.Flags().GetString("output")
		return printVCBotResult(output, data, fmt.Sprintf("会中消息已发送（msg_type=%s）。", body["msg_type"]))
	},
}

var vcBotScreenshotCmd = &cobra.Command{
	Use:   "meeting-screenshot",
	Short: "获取会中截图（JPEG）并保存到本地",
	Long: `获取会中画面截图（POST /open-apis/vc/v1/bots/screenshot），响应为 JPEG 原始字节，保存到本地文件。

必填:
  --meeting-id   长数字 meeting_id（不是 9 位会议号）

可选:
  --output       JPEG 保存路径（默认 meeting-screenshots/<meeting_id>-<UTC 时间>.jpg，父目录自动创建）；
                 在任何网络请求之前校验，敏感目录（~/.ssh 等）直接拒绝（exit 2）
  --overwrite    目标文件已存在时覆盖（不传则拒绝覆盖，exit 2）
  --as           bot | user | auto（默认 auto，规则同 vc bot meeting-events）
  --dry-run      只打印请求与输出路径，不调用（不读取凭据、不联网）

输出: JSON {meeting_id, path, size_bytes, content_type, sha256}

权限: vc:meeting.realtime:read

示例:
  feishu-cli vc bot meeting-screenshot --meeting-id 6911188411932033028 --output ./shot.jpg
  feishu-cli vc bot meeting-screenshot --meeting-id 6911188411932033028 --as bot --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		meetingID, _ := cmd.Flags().GetString("meeting-id")
		meetingID = strings.TrimSpace(meetingID)
		if err := validateVCEventsMeetingID(meetingID); err != nil {
			return err
		}
		output, _ := cmd.Flags().GetString("output")
		overwrite, _ := cmd.Flags().GetBool("overwrite")
		outPath := vcScreenshotOutputPath(output, meetingID)
		// 输出路径必须在首个网络请求（含 token 刷新）之前校验
		if err := safefile.ValidateOutputPath(outPath); err != nil {
			return err
		}
		if info, err := os.Stat(outPath); err == nil {
			if info.IsDir() {
				return clierr.Usagef("--output %s 是目录，请指定 .jpg 文件路径", outPath)
			}
			if !overwrite {
				return clierr.Usagef("输出文件已存在: %s（使用 --overwrite 覆盖）", outPath)
			}
		}
		body := map[string]any{"meeting_id": meetingID}
		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			identity, err := peekVCBotEventsIdentity(cmd)
			if err != nil {
				return err
			}
			return printVCBotDryRun(cmd, "/open-apis/vc/v1/bots/screenshot", nil, body, identity, map[string]any{"output": outPath})
		}
		if err := config.Validate(); err != nil {
			return err
		}
		token, _, err := resolveVCBotEventsIdentity(cmd)
		if err != nil {
			return err
		}
		image, contentType, err := client.VCBotScreenshot(meetingID, token)
		if err != nil {
			return err
		}
		if dir := filepath.Dir(outPath); dir != "." && dir != "" {
			if err := safefile.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		if err := safefile.AtomicWriteFile(outPath, image, 0o600); err != nil {
			return fmt.Errorf("写入截图失败: %w", err)
		}
		abs, absErr := filepath.Abs(outPath)
		if absErr != nil {
			abs = outPath
		}
		sum := sha256.Sum256(image)
		return printJSON(map[string]any{
			"meeting_id":   meetingID,
			"path":         abs,
			"size_bytes":   len(image),
			"content_type": contentType,
			"sha256":       fmt.Sprintf("%x", sum),
		})
	},
}

var vcBotInviteCmd = &cobra.Command{
	Use:   "meeting-invite",
	Short: "机器人邀请参会人（仅 Bot）",
	Long: `以应用机器人身份邀请用户入会（POST /open-apis/vc/v1/bots/invite?user_id_type=open_id，仅 Bot 身份）。

必填:
  --meeting-id   长数字 meeting_id（不是 9 位会议号）
  --type         all_suggested（邀请全部建议参会人）/ selected（邀请 --open-ids 指定的用户）

可选:
  --open-ids     逗号分隔的用户 open_id（ou_ 开头，去重后最多 200 个）；仅 --type selected，且此时必填
  --dry-run      只打印请求，不调用（不读取凭据、不联网）
  --output, -o   输出格式（json）

身份: 仅 Bot；传 --user-access-token 直接报用法错误（exit 2）。
权限: vc:meeting.bot.join:write

示例:
  feishu-cli vc bot meeting-invite --meeting-id 6911188411932033028 --type selected --open-ids ou_xxx,ou_yyy
  feishu-cli vc bot meeting-invite --meeting-id 6911188411932033028 --type all_suggested --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := rejectVCBotUserToken(cmd, "meeting-invite"); err != nil {
			return err
		}
		meetingID, _ := cmd.Flags().GetString("meeting-id")
		meetingID = strings.TrimSpace(meetingID)
		if err := validateVCEventsMeetingID(meetingID); err != nil {
			return err
		}
		inviteType, _ := cmd.Flags().GetString("type")
		openIDs, _ := cmd.Flags().GetStringSlice("open-ids")
		body, err := buildVCInviteBody(meetingID, inviteType, openIDs)
		if err != nil {
			return err
		}
		params := map[string]any{"user_id_type": "open_id"}
		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			return printVCBotDryRun(cmd, "/open-apis/vc/v1/bots/invite", params, body, "bot", nil)
		}
		if err := config.Validate(); err != nil {
			return err
		}
		data, err := client.VCBotInvite(body)
		if err != nil {
			return err
		}
		output, _ := cmd.Flags().GetString("output")
		return printVCBotResult(output, data, "邀请请求已发送。")
	},
}

var vcBotEndCmd = &cobra.Command{
	Use:   "meeting-end",
	Short: "机器人结束整场会议（仅 Bot，高危）",
	Long: `以主持人机器人身份结束整场会议（POST /open-apis/vc/v1/bots/end，仅 Bot 身份）。
所有参会人都会被移出会议；只让机器人自己离开请用 vc bot meeting-leave。

必填:
  --meeting-id   长数字 meeting_id（不是 9 位会议号）

可选:
  --dry-run      只打印请求，不调用（不读取凭据、不联网）
  --output, -o   输出格式（json）
  --yes          跳过确认（非交互环境未带 --yes 时 exit 10，不发请求）

身份: 仅 Bot（机器人须为当前主持人）；传 --user-access-token 直接报用法错误（exit 2）。
权限: vc:meeting.bot.manage:write

示例:
  feishu-cli vc bot meeting-end --meeting-id 6911188411932033028 --dry-run
  feishu-cli vc bot meeting-end --meeting-id 6911188411932033028 --yes`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := rejectVCBotUserToken(cmd, "meeting-end"); err != nil {
			return err
		}
		meetingID, _ := cmd.Flags().GetString("meeting-id")
		meetingID = strings.TrimSpace(meetingID)
		if err := validateVCEventsMeetingID(meetingID); err != nil {
			return err
		}
		body := map[string]any{"meeting_id": meetingID}
		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			return printVCBotDryRun(cmd, "/open-apis/vc/v1/bots/end", nil, body, "bot", nil)
		}
		if err := config.Validate(); err != nil {
			return err
		}
		if err := confirmDangerousAction(cmd, fmt.Sprintf("确定要结束会议 %s 吗？所有参会人都会被移出会议", meetingID)); err != nil {
			return err
		}
		data, err := client.VCBotEndMeeting(meetingID)
		if err != nil {
			return err
		}
		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			out := map[string]any{}
			if len(data) > 0 && string(data) != "null" {
				_ = json.Unmarshal(data, &out)
			}
			out["meeting_id"] = meetingID
			return printJSON(out)
		}
		fmt.Printf("会议 %s 已结束。\n", meetingID)
		return nil
	},
}

func init() {
	vcBotCmd.AddCommand(vcBotCountdownCmd, vcBotMessageSendCmd, vcBotScreenshotCmd, vcBotInviteCmd, vcBotEndCmd)

	f := vcBotCountdownCmd.Flags()
	f.String("meeting-id", "", "会议 ID（长数字 meeting_id，必填）")
	f.String("action", "", "set / prolong / end_in_advance / close_window（必填）")
	f.Int("duration", 0, "倒计时时长（分钟）；set / prolong 时必填")
	f.Int("reminder-before-end", 0, "结束前提醒（分钟，须小于 --duration）；仅 set")
	f.Bool("need-play-audio-at-end", false, "倒计时结束时播放提示音；仅 set")
	addVCBotSharedFlags(vcBotCountdownCmd, true)
	mustMarkFlagRequired(vcBotCountdownCmd, "meeting-id", "action")

	f = vcBotMessageSendCmd.Flags()
	f.String("meeting-id", "", "会议 ID（长数字 meeting_id，必填）")
	f.String("msg-type", "", "text | reaction（可按 --text / --emoji-type 推断）")
	f.String("text", "", "文本内容（最多 48KB）")
	f.String("emoji-type", "", "表情 key，如 THUMBSUP、LOVE")
	f.String("uuid", "", "幂等键（≤128 字节）")
	addVCBotSharedFlags(vcBotMessageSendCmd, true)
	mustMarkFlagRequired(vcBotMessageSendCmd, "meeting-id")

	f = vcBotScreenshotCmd.Flags()
	f.String("meeting-id", "", "会议 ID（长数字 meeting_id，必填）")
	f.String("output", "", "JPEG 保存路径（默认 meeting-screenshots/<meeting_id>-<时间>.jpg）")
	f.Bool("overwrite", false, "目标文件已存在时覆盖")
	f.Bool("dry-run", false, "只打印请求与输出路径，不实际调用")
	f.String("as", "auto", "身份：bot | user | auto（默认 auto）")
	f.String("user-access-token", "", "User Access Token（仅 --as user/auto 使用）")
	mustMarkFlagRequired(vcBotScreenshotCmd, "meeting-id")

	f = vcBotInviteCmd.Flags()
	f.String("meeting-id", "", "会议 ID（长数字 meeting_id，必填）")
	f.String("type", "", "all_suggested | selected（必填）")
	f.StringSlice("open-ids", nil, "用户 open_id 列表（逗号分隔，仅 --type selected）")
	addVCBotSharedFlags(vcBotInviteCmd, false)
	mustMarkFlagRequired(vcBotInviteCmd, "meeting-id", "type")

	f = vcBotEndCmd.Flags()
	f.String("meeting-id", "", "会议 ID（长数字 meeting_id，必填）")
	addVCBotSharedFlags(vcBotEndCmd, false)
	mustMarkFlagRequired(vcBotEndCmd, "meeting-id")
}

// addVCBotSharedFlags 注册会中控制命令共用的 --dry-run / -o 与身份 flag；
// withAs=false 的命令仅 Bot：--user-access-token 只用于报错提示。
func addVCBotSharedFlags(c *cobra.Command, withAs bool) {
	c.Flags().Bool("dry-run", false, "只打印请求，不实际调用")
	c.Flags().StringP("output", "o", "", "输出格式（json）")
	if withAs {
		c.Flags().String("as", "auto", "身份：bot | user | auto（默认 auto）")
		c.Flags().String("user-access-token", "", "User Access Token（仅 --as user/auto 使用）")
		return
	}
	c.Flags().String("user-access-token", "", "该接口仅支持 Bot 身份，传入将报错")
}
