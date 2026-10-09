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

var vcSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "搜索历史会议记录（多维过滤）",
	Long: `搜索历史会议记录。
使用飞书 POST /open-apis/vc/v1/meetings/search API，支持关键词 + 时间范围 + 参会者 + 主持人 + 会议室多维过滤。

必须指定至少一个过滤条件：--query / --start / --end / --organizer-ids / --participant-ids / --room-ids

可选参数:
  --query            搜索关键词（1-50 字符）
  --start            起始时间（支持 YYYY-MM-DD 或 RFC3339）
  --end              结束时间（支持 YYYY-MM-DD 或 RFC3339；纯日期会对齐到 23:59:59）
  --organizer-ids    主持人 open_id 列表，逗号分隔
  --participant-ids  参会者 open_id 列表，逗号分隔
  --room-ids         会议室 ID 列表，逗号分隔
  --page-size        每页数量（1-30，默认 15）
  --page-token       分页标记
  --output, -o       输出格式（json）

权限:
  默认 User 身份（--as user），可用 --as bot|auto 切换；需要 vc:meeting.search:read 权限

示例:
  # 按关键词搜索
  feishu-cli vc search --query "周会"

  # 按时间范围搜索
  feishu-cli vc search --start 2026-03-20 --end 2026-04-10

  # 按主持人过滤
  feishu-cli vc search --organizer-ids ou_xxx,ou_yyy

  # 组合过滤 + JSON 输出
  feishu-cli vc search --query "需求评审" --start 2026-03-01 -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		query, _ := cmd.Flags().GetString("query")
		startStr, _ := cmd.Flags().GetString("start")
		endStr, _ := cmd.Flags().GetString("end")
		organizerRaw, _ := cmd.Flags().GetString("organizer-ids")
		participantRaw, _ := cmd.Flags().GetString("participant-ids")
		roomRaw, _ := cmd.Flags().GetString("room-ids")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		output, _ := cmd.Flags().GetString("output")

		// 至少一个过滤条件
		if strings.TrimSpace(query) == "" &&
			strings.TrimSpace(startStr) == "" &&
			strings.TrimSpace(endStr) == "" &&
			strings.TrimSpace(organizerRaw) == "" &&
			strings.TrimSpace(participantRaw) == "" &&
			strings.TrimSpace(roomRaw) == "" {
			return clierr.Usagef("请至少指定一个过滤条件（--query / --start / --end / --organizer-ids / --participant-ids / --room-ids）")
		}

		if l := len([]rune(query)); l > 50 {
			return clierr.Usagef("--query 长度不能超过 50 字符（当前 %d）", l)
		}

		// 时间解析
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

		// page-size 范围
		if pageSize < 0 || pageSize > 30 {
			return clierr.Usagef("--page-size 取值范围 1-30（当前 %d）", pageSize)
		}
		if pageSize == 0 {
			pageSize = 15
		}

		organizerIDs, err := parseCSVIDs(organizerRaw, "organizer-ids")
		if err != nil {
			return err
		}
		participantIDs, err := parseCSVIDs(participantRaw, "participant-ids")
		if err != nil {
			return err
		}
		roomIDs, err := parseCSVIDs(roomRaw, "room-ids")
		if err != nil {
			return err
		}

		// 参数校验通过后再解析身份：用法错误（exit 2）不应被"未登录"（exit 3）遮住
		token, err := resolveVCReadIdentity(cmd)
		if err != nil {
			return err
		}

		req := client.SearchMeetingsReq{
			Query:          strings.TrimSpace(query),
			StartRFC3339:   startRFC,
			EndRFC3339:     endRFC,
			OrganizerIDs:   organizerIDs,
			ParticipantIDs: participantIDs,
			RoomIDs:        roomIDs,
			PageSize:       pageSize,
			PageToken:      pageToken,
		}

		data, err := client.SearchMeetings(req, token)
		if err != nil {
			return err
		}

		// JSON 输出：原样透传 data（含服务端 <h></h> 高亮标签）
		if output == "json" {
			return printJSON(json.RawMessage(data))
		}

		// 文本输出
		var parsed struct {
			Items []struct {
				ID          string `json:"id"`
				DisplayInfo string `json:"display_info"`
				MetaData    struct {
					Description string `json:"description"`
				} `json:"meta_data"`
			} `json:"items"`
			Total     int    `json:"total"`
			HasMore   bool   `json:"has_more"`
			PageToken string `json:"page_token"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			// 解析失败就直接打印原 JSON
			fmt.Println(string(data))
			return nil
		}

		if len(parsed.Items) == 0 {
			fmt.Println("未找到匹配的会议记录")
			return nil
		}

		fmt.Printf("会议列表（共 %d 条）:\n\n", len(parsed.Items))
		for i, it := range parsed.Items {
			// 文本输出去掉服务端命中高亮标签 <h>...</h>
			title := strings.TrimSpace(stripSearchHighlight(it.DisplayInfo))
			if title == "" {
				title = "(无标题)"
			}
			fmt.Printf("[%d] %s\n", i+1, title)
			fmt.Printf("    会议 ID:  %s\n", it.ID)
			if meta := strings.TrimSpace(stripSearchHighlight(it.MetaData.Description)); meta != "" {
				fmt.Printf("    时间/备注: %s\n", meta)
			}
			fmt.Println()
		}
		if parsed.HasMore {
			fmt.Printf("还有更多会议，可用 --page-token %s 获取下一页\n", parsed.PageToken)
		}
		return nil
	},
}

func init() {
	vcCmd.AddCommand(vcSearchCmd)
	vcSearchCmd.Flags().String("query", "", "搜索关键词（1-50 字符）")
	vcSearchCmd.Flags().String("start", "", "起始时间（YYYY-MM-DD 或 RFC3339）")
	vcSearchCmd.Flags().String("end", "", "结束时间（YYYY-MM-DD 或 RFC3339）")
	vcSearchCmd.Flags().String("organizer-ids", "", "主持人 open_id 列表，逗号分隔")
	vcSearchCmd.Flags().String("participant-ids", "", "参会者 open_id 列表，逗号分隔")
	vcSearchCmd.Flags().String("room-ids", "", "会议室 ID 列表，逗号分隔")
	vcSearchCmd.Flags().Int("page-size", 15, "每页数量（1-30）")
	vcSearchCmd.Flags().String("page-token", "", "分页标记")
	vcSearchCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	vcSearchCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	addVCReadAsFlag(vcSearchCmd)
}
