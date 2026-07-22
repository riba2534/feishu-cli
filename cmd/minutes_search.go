package cmd

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// minuteSearchHighlightPattern 匹配搜索结果 display_info 里的高亮标签（<h></h> / <b></b> 等）
var minuteSearchHighlightPattern = regexp.MustCompile(`</?[a-zA-Z]+>`)

var minutesSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "按关键词 / 所有者 / 时间搜索妙记",
	Long: `搜索妙记列表，支持关键词、所有者、创建时间范围多条件过滤。

使用飞书 POST /open-apis/minutes/v1/minutes/search API。至少指定一个过滤条件：
--query / --owner-id / --start-time / --end-time。

可选参数:
  --query        搜索关键词（1-50 字符）
  --owner-id     所有者 open_id（按妙记所有者过滤）
  --start-time   创建时间起点（YYYY-MM-DD 或 RFC3339）
  --end-time     创建时间终点（YYYY-MM-DD 或 RFC3339；纯日期对齐到 23:59:59）
  --page-size    每页数量（1-30，默认 15）
  --page-token   分页标记
  --output, -o   输出格式（json）

权限:
  需要 User Access Token + minutes:minutes.search:read 权限

示例:
  # 按关键词搜索
  feishu-cli minutes search --query "预算复盘"

  # 按时间范围搜索
  feishu-cli minutes search --start-time 2026-03-10 --end-time 2026-03-17

  # 按所有者过滤
  feishu-cli minutes search --owner-id ou_xxx

  # 组合过滤 + JSON 输出
  feishu-cli minutes search --query "周会" --start-time 2026-03-01 -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token, err := requireUserToken(cmd, "minutes search")
		if err != nil {
			return err
		}

		query, _ := cmd.Flags().GetString("query")
		ownerID, _ := cmd.Flags().GetString("owner-id")
		startStr, _ := cmd.Flags().GetString("start-time")
		endStr, _ := cmd.Flags().GetString("end-time")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		output, _ := cmd.Flags().GetString("output")

		query = strings.TrimSpace(query)
		ownerID = strings.TrimSpace(ownerID)

		// 至少一个过滤条件
		if query == "" && ownerID == "" &&
			strings.TrimSpace(startStr) == "" && strings.TrimSpace(endStr) == "" {
			return fmt.Errorf("请至少指定一个过滤条件（--query / --owner-id / --start-time / --end-time）")
		}

		if l := len([]rune(query)); l > 50 {
			return fmt.Errorf("--query 长度不能超过 50 字符（当前 %d）", l)
		}

		startRFC, err := parseVCTime(startStr, false)
		if err != nil {
			return fmt.Errorf("解析 --start-time 失败: %w", err)
		}
		endRFC, err := parseVCTime(endStr, true)
		if err != nil {
			return fmt.Errorf("解析 --end-time 失败: %w", err)
		}
		if startRFC != "" && endRFC != "" && startRFC > endRFC {
			return fmt.Errorf("--start-time 不能晚于 --end-time")
		}

		if pageSize < 0 || pageSize > 30 {
			return fmt.Errorf("--page-size 取值范围 1-30（当前 %d）", pageSize)
		}
		if pageSize == 0 {
			pageSize = 15
		}

		var ownerIDs []string
		if ownerID != "" {
			ownerIDs = []string{ownerID}
		}

		req := client.SearchMinutesReq{
			Query:        query,
			OwnerIDs:     ownerIDs,
			StartRFC3339: startRFC,
			EndRFC3339:   endRFC,
			PageSize:     pageSize,
			PageToken:    pageToken,
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
	minutesSearchCmd.Flags().String("owner-id", "", "所有者 open_id")
	minutesSearchCmd.Flags().String("start-time", "", "创建时间起点（YYYY-MM-DD 或 RFC3339）")
	minutesSearchCmd.Flags().String("end-time", "", "创建时间终点（YYYY-MM-DD 或 RFC3339）")
	minutesSearchCmd.Flags().Int("page-size", 15, "每页数量（1-30）")
	minutesSearchCmd.Flags().String("page-token", "", "分页标记")
	minutesSearchCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	minutesSearchCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
}
