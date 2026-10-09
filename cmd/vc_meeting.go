package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

// vcMeetingCmd 会中会议相关的纯分组命令（无 RunE，未知子命令由 command_guard 统一拦截）
var vcMeetingCmd = &cobra.Command{
	Use:   "meeting",
	Short: "进行中的会议（查询当前参加的会议）",
	Long: `进行中的会议相关操作。

子命令:
  list-active   列出当前身份（或 Bot 身份下指定用户）正在参加的会议，获取 meeting_id

meeting_id 可继续用于 vc bot meeting-events 查询会中事件。`,
}

var vcMeetingListActiveCmd = &cobra.Command{
	Use:   "list-active",
	Short: "列出正在参加的会议（获取 meeting_id）",
	Long: `列出当前身份正在参加的会议（GET /open-apis/vc/v1/bots/user_active_meeting）。

常用于 vc bot meeting-events 之前定位 meeting_id：同时在多个会议中时，先让用户选定一个 meeting_id。

身份:
  --as user（默认）  查询当前登录用户正在参加的会议
  --as bot           查询 --user-id 指定用户正在参加的会议（Bot 身份必须传 --user-id，open_id 以 ou_ 开头）
  --as auto          User 优先；未配置 User 时回退 Bot（此时同样需要 --user-id）

可选:
  --user-id      目标用户 open_id（仅 Bot 身份使用）
  --dry-run      只打印将要发送的请求，不实际调用
  --output, -o   输出格式（json）

权限:
  - User 身份: vc:meeting.meetingevent:read
  - Bot 身份:  vc:meeting.meetingevent:read 与 vc:meeting.bot.join:write 任一（应用身份权限，开通其一即可）

示例:
  feishu-cli vc meeting list-active
  feishu-cli vc meeting list-active --as bot --user-id ou_xxx -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}
		userID, _ := cmd.Flags().GetString("user-id")
		userID = strings.TrimSpace(userID)
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		output, _ := cmd.Flags().GetString("output")
		as, _ := cmd.Flags().GetString("as")
		as = strings.ToLower(strings.TrimSpace(as))

		if userID != "" && !strings.HasPrefix(userID, "ou_") {
			return clierr.Usagef("--user-id 需要 open_id（以 ou_ 开头），得到 %q", userID)
		}
		// 显式 --as bot 时前置校验 --user-id，不联网
		if isBotAs(as) && userID == "" {
			return clierr.Usagef("--as bot 需要 --user-id 指定目标用户 open_id（ou_ 开头）")
		}

		if dryRun {
			preview := map[string]any{
				"method": "GET",
				"path":   "/open-apis/vc/v1/bots/user_active_meeting",
				"as":     as,
			}
			if isBotAs(as) {
				preview["query"] = map[string]any{"user_id": userID}
			}
			return printJSON(preview)
		}

		token, err := resolveVCReadIdentity(cmd)
		if err != nil {
			return err
		}
		queryUserID := ""
		if token == "" {
			// Bot 身份（含 auto 回退）：接口要求 user_id
			if userID == "" {
				return clierr.Usagef("当前为 Bot 身份（未登录 User），需要 --user-id 指定目标用户 open_id（ou_ 开头）")
			}
			queryUserID = userID
		}

		data, err := client.ListActiveMeetings(queryUserID, token)
		if err != nil {
			return err
		}
		if output == "json" {
			return printJSON(json.RawMessage(data))
		}
		printActiveMeetingsText(data)
		return nil
	},
}

// isBotAs 判断 --as 取值是否为 Bot 身份
func isBotAs(as string) bool {
	switch as {
	case "bot", "tenant", "app":
		return true
	}
	return false
}

// printActiveMeetingsText 文本输出进行中的会议
func printActiveMeetingsText(data json.RawMessage) {
	var parsed struct {
		Meetings []struct {
			MeetingID    string `json:"meeting_id"`
			MeetingNo    string `json:"meeting_no"`
			MeetingTitle string `json:"meeting_title"`
		} `json:"meetings"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		fmt.Println(string(data))
		return
	}
	if len(parsed.Meetings) == 0 {
		fmt.Println("当前没有进行中的会议")
		return
	}
	fmt.Printf("进行中的会议（共 %d 个）:\n\n", len(parsed.Meetings))
	for i, m := range parsed.Meetings {
		title := strings.TrimSpace(m.MeetingTitle)
		if title == "" {
			title = "(无标题)"
		}
		fmt.Printf("[%d] %s\n", i+1, title)
		if m.MeetingID != "" {
			fmt.Printf("    meeting_id:  %s\n", m.MeetingID)
		}
		if m.MeetingNo != "" {
			fmt.Printf("    会议号:      %s\n", m.MeetingNo)
		}
	}
	if len(parsed.Meetings) > 1 {
		fmt.Println("\n同时在多个会议中：请先确认要查询的 meeting_id，再调用 vc bot meeting-events。")
	}
}

func init() {
	vcCmd.AddCommand(vcMeetingCmd)
	vcMeetingCmd.AddCommand(vcMeetingListActiveCmd)
	addVCReadAsFlag(vcMeetingListActiveCmd)
	vcMeetingListActiveCmd.Flags().String("user-id", "", "目标用户 open_id（ou_ 开头，仅 Bot 身份使用）")
	vcMeetingListActiveCmd.Flags().Bool("dry-run", false, "只打印请求，不实际调用")
	vcMeetingListActiveCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	vcMeetingListActiveCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
}
