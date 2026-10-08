package cmd

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// minuteSearchHighlightPattern 匹配搜索结果 display_info 里的高亮标签（<h></h> / <b></b> 等）
var minuteSearchHighlightPattern = regexp.MustCompile(`</?[a-zA-Z]+>`)

var minutesSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "按关键词 / 所有者 / 参与者 / 时间搜索妙记",
	Long: `搜索妙记列表，支持关键词、所有者、参与者、创建时间范围多条件过滤。

使用飞书 POST /open-apis/minutes/v1/minutes/search API，结果固定按创建时间倒序
（sorter=create_time_desc），保证 --page-token 翻页不漏条。至少指定一个过滤条件：
--query / --owner-ids / --participant-ids / --start / --end。

可选参数（参数名对齐官方 minutes +search）:
  --query            搜索关键词（1-50 字符；--keyword 为别名）
  --owner-ids        所有者 open_id 列表，逗号分隔（任一匹配；me = 当前登录用户）
  --participant-ids  参与者 open_id 列表，逗号分隔（任一匹配；me = 当前登录用户）
  --start            创建时间起点（YYYY-MM-DD 或 RFC3339）
  --end              创建时间终点（YYYY-MM-DD 或 RFC3339；纯日期对齐到 23:59:59）
  --page-size        每页数量（1-30，默认 15）
  --page-token       分页标记
  --output, -o       输出格式（json）

旧参数名继续可用（已废弃）：--owner-id → --owner-ids，--start-time → --start，--end-time → --end。

权限:
  默认 User 身份（--as user），可用 --as bot|auto 切换；需要 minutes:minutes.search:read 权限
  me 需要已登录 User（解析当前用户 open_id）

示例:
  # 按关键词搜索
  feishu-cli minutes search --query "预算复盘"

  # 按时间范围搜索
  feishu-cli minutes search --start 2026-03-10 --end 2026-03-17

  # 我参与过的妙记
  feishu-cli minutes search --participant-ids me --start 2026-03-01

  # 组合过滤 + JSON 输出
  feishu-cli minutes search --query "周会" --owner-ids ou_xxx -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		query, _ := cmd.Flags().GetString("query")
		keyword, _ := cmd.Flags().GetString("keyword")
		ownerRaw, _ := cmd.Flags().GetString("owner-ids")
		legacyOwner, _ := cmd.Flags().GetString("owner-id")
		participantRaw, _ := cmd.Flags().GetString("participant-ids")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		output, _ := cmd.Flags().GetString("output")

		query = strings.TrimSpace(query)
		if kw := strings.TrimSpace(keyword); kw != "" {
			if query != "" && query != kw {
				return clierr.Usagef("--query 与 --keyword（别名）不能同时指定不同的值")
			}
			query = kw
		}
		startStr, err := pickRenamedFlag(cmd, "start", "start-time")
		if err != nil {
			return err
		}
		endStr, err := pickRenamedFlag(cmd, "end", "end-time")
		if err != nil {
			return err
		}
		// --owner-id（旧，单值）与 --owner-ids（新，多值）合并
		ownerList := splitAndTrim(ownerRaw)
		if s := strings.TrimSpace(legacyOwner); s != "" {
			ownerList = append(ownerList, s)
		}
		participantList := splitAndTrim(participantRaw)

		// 至少一个过滤条件
		if query == "" && len(ownerList) == 0 && len(participantList) == 0 &&
			strings.TrimSpace(startStr) == "" && strings.TrimSpace(endStr) == "" {
			return clierr.Usagef("请至少指定一个过滤条件（--query / --owner-ids / --participant-ids / --start / --end）")
		}

		if l := len([]rune(query)); l > 50 {
			return clierr.Usagef("--query 长度不能超过 50 字符（当前 %d）", l)
		}

		startRFC, err := parseVCTime(startStr, false)
		if err != nil {
			return clierr.Usagef("解析 --start 失败: %v", err)
		}
		endRFC, err := parseVCTime(endStr, true)
		if err != nil {
			return clierr.Usagef("解析 --end 失败: %v", err)
		}
		if startRFC != "" && endRFC != "" && startRFC > endRFC {
			return clierr.Usagef("--start 不能晚于 --end")
		}

		if pageSize < 0 || pageSize > 30 {
			return clierr.Usagef("--page-size 取值范围 1-30（当前 %d）", pageSize)
		}
		if pageSize == 0 {
			pageSize = 15
		}

		ownerIDs, err := resolveMinutesUserIDs(cmd, "--owner-ids", ownerList)
		if err != nil {
			return err
		}
		participantIDs, err := resolveMinutesUserIDs(cmd, "--participant-ids", participantList)
		if err != nil {
			return err
		}

		token, err := resolveVCReadIdentity(cmd)
		if err != nil {
			return err
		}

		req := client.SearchMinutesReq{
			Query:          query,
			OwnerIDs:       ownerIDs,
			ParticipantIDs: participantIDs,
			StartRFC3339:   startRFC,
			EndRFC3339:     endRFC,
			PageSize:       pageSize,
			PageToken:      pageToken,
		}

		data, err := client.SearchMinutes(req, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(json.RawMessage(data))
		}

		var parsed struct {
			Items []struct {
				Token       string `json:"token"`
				DisplayInfo string `json:"display_info"`
				MetaData    struct {
					Description string `json:"description"`
					AppLink     string `json:"app_link"`
				} `json:"meta_data"`
			} `json:"items"`
			HasMore   bool   `json:"has_more"`
			PageToken string `json:"page_token"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			fmt.Println(string(data))
			return nil
		}

		if len(parsed.Items) == 0 {
			fmt.Println("未找到匹配的妙记")
			return nil
		}

		fmt.Printf("妙记列表（共 %d 条）:\n\n", len(parsed.Items))
		for i, it := range parsed.Items {
			title := stripMinuteHighlight(it.DisplayInfo)
			if title == "" {
				title = "(无标题)"
			}
			fmt.Printf("[%d] %s\n", i+1, title)
			fmt.Printf("    token:    %s\n", it.Token)
			if desc := strings.TrimSpace(it.MetaData.Description); desc != "" {
				fmt.Printf("    信息:     %s\n", desc)
			}
			if link := strings.TrimSpace(it.MetaData.AppLink); link != "" {
				fmt.Printf("    链接:     %s\n", link)
			}
			fmt.Println()
		}
		if parsed.HasMore {
			fmt.Printf("还有更多妙记，可用 --page-token %s 获取下一页\n", parsed.PageToken)
		}
		return nil
	},
}

// pickRenamedFlag 读取更名后的 flag：新旧名都可用；同时指定且取值不同时报用法错误。
// 旧名的废弃提示由 cobra MarkDeprecated 在 stderr 统一输出。
func pickRenamedFlag(cmd *cobra.Command, newName, oldName string) (string, error) {
	newVal, _ := cmd.Flags().GetString(newName)
	oldVal, _ := cmd.Flags().GetString(oldName)
	newVal, oldVal = strings.TrimSpace(newVal), strings.TrimSpace(oldVal)
	if newVal != "" && oldVal != "" && newVal != oldVal {
		return "", clierr.Usagef("--%s 与旧参数 --%s 不能同时指定不同的值", newName, oldName)
	}
	if newVal != "" {
		return newVal, nil
	}
	return oldVal, nil
}

// minutesCurrentUserOpenID 解析 me 对应的当前登录用户 open_id；测试可替换
var minutesCurrentUserOpenID = func(cmd *cobra.Command) (string, error) {
	return resolveCurrentAuthedUserID(cmd, "open_id")
}

// resolveMinutesUserIDs 规范化 open_id 列表：me → 当前登录用户 open_id；去重保序；
// 其余值必须以 ou_ 开头（对齐官方 ValidateUserIDTyped），数量上限同 vc 批量参数。
func resolveMinutesUserIDs(cmd *cobra.Command, flagName string, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if strings.EqualFold(id, "me") {
			openID, err := minutesCurrentUserOpenID(cmd)
			if err != nil || openID == "" {
				return nil, clierr.Usagef("%s 中的 me 需要已登录 User 且能解析当前用户 open_id（请先 `feishu-cli auth login`，或直接传 ou_ 开头的 open_id）: %v", flagName, err)
			}
			id = openID
		}
		if !strings.HasPrefix(id, "ou_") {
			return nil, clierr.Usagef("%s 需要 open_id（以 ou_ 开头）或 me，得到 %q", flagName, id)
		}
		key := strings.ToLower(id)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, id)
	}
	if len(out) > vcBatchLimit {
		return nil, clierr.Usagef("%s 最多 %d 个，当前 %d 个", flagName, vcBatchLimit, len(out))
	}
	return out, nil
}

// stripMinuteHighlight 去除 display_info 的高亮标签，并取首行作为标题
func stripMinuteHighlight(s string) string {
	clean := minuteSearchHighlightPattern.ReplaceAllString(s, "")
	if idx := strings.IndexByte(clean, '\n'); idx >= 0 {
		clean = clean[:idx]
	}
	return strings.TrimSpace(clean)
}

func init() {
	minutesCmd.AddCommand(minutesSearchCmd)
	minutesSearchCmd.Flags().String("query", "", "搜索关键词（1-50 字符）")
	minutesSearchCmd.Flags().String("keyword", "", "--query 的别名")
	minutesSearchCmd.Flags().String("owner-ids", "", "所有者 open_id 列表，逗号分隔（me = 当前登录用户）")
	minutesSearchCmd.Flags().String("participant-ids", "", "参与者 open_id 列表，逗号分隔（me = 当前登录用户）")
	minutesSearchCmd.Flags().String("start", "", "创建时间起点（YYYY-MM-DD 或 RFC3339）")
	minutesSearchCmd.Flags().String("end", "", "创建时间终点（YYYY-MM-DD 或 RFC3339）")
	minutesSearchCmd.Flags().String("owner-id", "", "已废弃，请用 --owner-ids")
	minutesSearchCmd.Flags().String("start-time", "", "已废弃，请用 --start")
	minutesSearchCmd.Flags().String("end-time", "", "已废弃，请用 --end")
	_ = minutesSearchCmd.Flags().MarkDeprecated("owner-id", "请改用 --owner-ids（支持逗号分隔多个与 me）")
	_ = minutesSearchCmd.Flags().MarkDeprecated("start-time", "请改用 --start")
	_ = minutesSearchCmd.Flags().MarkDeprecated("end-time", "请改用 --end")
	minutesSearchCmd.Flags().Int("page-size", 15, "每页数量（1-30）")
	minutesSearchCmd.Flags().String("page-token", "", "分页标记")
	minutesSearchCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	minutesSearchCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	addVCReadAsFlag(minutesSearchCmd)
}
