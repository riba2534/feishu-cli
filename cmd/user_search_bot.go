package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var userSearchBotCmd = &cobra.Command{
	Use:   "search-bot",
	Short: "按关键词搜索机器人（应用），需 User Token",
	Long: `按关键词搜索机器人（POST /open-apis/bot/v4/bot/search，对齐官方 contact +search-bot）。

可在整个租户范围找，也可用 --chat-ids 只在指定群里找；结果里的 open_id 可直接用于
@机器人 或 chat member add（--member-id-type 需按 app_id 时请另查应用 ID）。

参数:
  --query        搜索关键词（必填，≤50 字）
  --chat-ids     只在这些群里找（逗号分隔，≤100）
  --has-chatted  只看和你聊过天的机器人
  --page-size    每页数量（1-30，默认 20）
  --page-token   分页标记
  --output, -o   输出格式（json）：{bots:[{open_id,name,description,chat_id,enable_join_group,is_agent,tenant_id,match_segments}], has_more, page_token, notice}

身份: 只支持 User Token（scope: search:bot）。

示例:
  feishu-cli user search-bot --query "告警"
  feishu-cli user search-bot --query "助手" --chat-ids oc_xxx -o json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.TrimSpace(flagString(cmd, "query"))
		if query == "" {
			return clierr.Usagef("--query 不能为空（--chat-ids / --has-chatted 只能缩小关键词搜索范围，不能单独枚举机器人）")
		}
		if n := len([]rune(query)); n > client.MaxContactSearchQuery {
			return clierr.Usagef("--query 最多 %d 个字符，得到 %d 个", client.MaxContactSearchQuery, n)
		}
		chatIDs := splitAndTrim(flagString(cmd, "chat-ids"))
		if len(chatIDs) > 100 {
			return clierr.Usagef("--chat-ids 最多 100 个，得到 %d 个", len(chatIDs))
		}
		pageSize := flagInt(cmd, "page-size")
		if pageSize < 1 || pageSize > client.MaxContactSearchPageSize {
			return clierr.Usagef("--page-size 范围 1-%d，得到 %d", client.MaxContactSearchPageSize, pageSize)
		}
		hasChatted, _ := cmd.Flags().GetBool("has-chatted")

		if err := config.Validate(); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "user search-bot")
		if err != nil {
			return err
		}
		res, err := client.SearchBots(client.SearchBotsOptions{
			Query:      query,
			ChatIDs:    chatIDs,
			HasChatted: hasChatted,
			PageSize:   pageSize,
			PageToken:  flagString(cmd, "page-token"),
		}, token)
		if err != nil {
			return err
		}
		printSearchNotice(cmd.ErrOrStderr(), res.Notice)
		if output, _ := cmd.Flags().GetString("output"); output == "json" {
			return printJSON(res)
		}
		if len(res.Bots) == 0 {
			fmt.Println("未找到匹配的机器人")
			return nil
		}
		fmt.Printf("机器人（共 %d 个）:\n\n", len(res.Bots))
		for i, b := range res.Bots {
			fmt.Printf("[%d] %s\n", i+1, b.Name)
			fmt.Printf("    open_id: %s\n", b.OpenID)
			if b.Description != "" {
				fmt.Printf("    简介: %s\n", truncateRunes(b.Description, 80))
			}
			if b.ChatID != "" {
				fmt.Printf("    单聊 chat_id: %s\n", b.ChatID)
			}
			fmt.Println()
		}
		if res.HasMore {
			fmt.Fprintf(cmd.ErrOrStderr(), "[提示] 还有更多结果，带 --page-token %s 翻页\n", res.PageToken)
		}
		return nil
	},
}

func init() {
	userCmd.AddCommand(userSearchBotCmd)
	userSearchBotCmd.Flags().String("query", "", "搜索关键词（≤50 字）")
	userSearchBotCmd.Flags().String("chat-ids", "", "只在这些群里找（逗号分隔，≤100）")
	userSearchBotCmd.Flags().Bool("has-chatted", false, "只看和你聊过天的机器人")
	userSearchBotCmd.Flags().Int("page-size", 20, "每页数量（1-30）")
	userSearchBotCmd.Flags().String("page-token", "", "分页标记")
	userSearchBotCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	userSearchBotCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
