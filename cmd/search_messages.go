package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/output"
	"github.com/spf13/cobra"
)

var searchMessagesCmd = &cobra.Command{
	Use:   "messages <query>",
	Short: "搜索消息（默认 enrich 内容/发送者/群名/时间）",
	Long: `搜索飞书消息。默认对结果做 enrich：在消息 ID 基础上补全内容、发送者、群名、时间
（对齐 lark-cli +messages-search），不再只返回消息 ID。

注意：此功能需要 User Access Token（用户授权令牌），推荐通过 auth login 获取。

参数:
  query           搜索关键词（必需）

选项:
  --chat-ids      指定搜索的会话 ID 列表（逗号分隔）
  --from-ids      指定消息发送者用户 ID 列表（逗号分隔）
  --message-type  消息类型过滤（file/image/media）
  --chat-type     会话类型（group_chat/p2p_chat）
  --from-type     发送者类型（bot/user）
  --start-time    消息发送起始时间（Unix 时间戳，秒）
  --end-time      消息发送结束时间（Unix 时间戳，秒）
  --page-size     每页数量（默认 20）
  --page-token    分页 token
  --page-all      自动翻页拉取全部结果（配合 --page-limit 限制页数）
  --ids-only      仅返回消息 ID（跳过 enrich，省去额外 API 调用，等价旧行为）
  --format        结构化输出: json | pretty | table | ndjson | csv
  --jq            用 jq 表达式过滤结构化输出
  --user-id-type  用户 ID 类型（open_id/union_id/user_id，默认 open_id）

示例:
  # 搜索包含"会议"的消息（默认 enrich，人类可读）
  feishu-cli search messages "会议"

  # 表格输出 + jq 只看发送者和文本
  feishu-cli search messages "会议" --format table
  feishu-cli search messages "会议" --jq '.[] | {sender_name, text}'

  # 指定会话 + 自动翻页 + CSV
  feishu-cli search messages "项目" --chat-ids oc_xxx --page-all --page-limit 5 --format csv

  # 仅要消息 ID（快，旧行为）
  feishu-cli search messages "会议" --ids-only`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		query := args[0]

		userAccessToken, err := resolveRequiredUserToken(cmd)
		if err != nil {
			return err
		}

		chatIDsStr, _ := cmd.Flags().GetString("chat-ids")
		fromIDsStr, _ := cmd.Flags().GetString("from-ids")
		atChatterIDsStr, _ := cmd.Flags().GetString("at-chatter-ids")
		messageType, _ := cmd.Flags().GetString("message-type")
		chatType, _ := cmd.Flags().GetString("chat-type")
		fromType, _ := cmd.Flags().GetString("from-type")
		startTime, _ := cmd.Flags().GetString("start-time")
		endTime, _ := cmd.Flags().GetString("end-time")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		idsOnly, _ := cmd.Flags().GetBool("ids-only")
		pageAll, _ := cmd.Flags().GetBool("page-all")
		pageLimit, _ := cmd.Flags().GetInt("page-limit")
		legacyOutput, _ := cmd.Flags().GetString("output")
		jq, _ := cmd.Flags().GetString("jq")

		opts := client.SearchMessagesOptions{
			Query:        query,
			ChatIDs:      splitAndTrim(chatIDsStr),
			FromIDs:      splitAndTrim(fromIDsStr),
			AtChatterIDs: splitAndTrim(atChatterIDsStr),
			MessageType:  messageType,
			ChatType:     chatType,
			FromType:     fromType,
			StartTime:    startTime,
			EndTime:      endTime,
			PageSize:     pageSize,
			PageToken:    pageToken,
			UserIDType:   userIDType,
		}

		// 是否走结构化输出：显式 --format / --jq，或旧的 -o json
		useStructured := cmd.Flags().Changed("format") || jq != "" || legacyOutput == "json"
		formatVal := output.FormatJSON
		if cmd.Flags().Changed("format") {
			formatVal, _ = cmd.Flags().GetString("format")
		}

		// --ids-only：跳过 enrich，仅翻页收集 ID
		if idsOnly {
			ids, lastRes, err := collectMessageIDs(opts, userAccessToken, pageAll, pageLimit)
			if err != nil {
				return err
			}
			if useStructured {
				o, oerr := output.NewOptions(formatVal, jq)
				if oerr != nil {
					return oerr
				}
				return output.Render(o, map[string]any{"message_ids": ids, "has_more": lastRes.HasMore, "page_token": lastRes.PageToken})
			}
			if len(ids) == 0 {
				fmt.Println("未找到匹配的消息")
				return nil
			}
			fmt.Printf("搜索结果（共 %d 条）:\n\n", len(ids))
			for i, id := range ids {
				fmt.Printf("[%d] %s\n", i+1, id)
			}
			printMoreHint(pageAll, lastRes)
			return nil
		}

		// 默认：enrich（card-content-type 仅 enrich 路径消费，故校验下沉到此，
		// 不让 --ids-only 因非法 --card-content-type 被提前 abort）
		cardContentType, err := resolveCardContentType(cmd)
		if err != nil {
			return err
		}
		enriched, lastRes, err := collectEnrichedMessages(opts, userAccessToken, cardContentType, pageAll, pageLimit)
		if err != nil {
			return err
		}

		if useStructured {
			o, oerr := output.NewOptions(formatVal, jq)
			if oerr != nil {
				return oerr
			}
			return output.Render(o, enriched)
		}

		// 人类可读视图
		if len(enriched) == 0 {
			fmt.Println("未找到匹配的消息")
			return nil
		}
		fmt.Printf("搜索结果（共 %d 条）:\n\n", len(enriched))
		for i, m := range enriched {
			chat := firstNonEmpty(m.ChatName, m.ChatID)
			sender := firstNonEmpty(m.SenderName, m.SenderID)
			fmt.Printf("[%d] %s | %s | %s: %s\n", i+1, m.Time, chat, sender, truncateRunes(m.Text, 120))
			fmt.Printf("    %s\n", m.MessageID)
		}
		printMoreHint(pageAll, lastRes)
		return nil
	},
}

// collectMessageIDs 收集消息 ID，支持 --page-all 翻页（受 --page-limit 限制）。
func collectMessageIDs(opts client.SearchMessagesOptions, token string, pageAll bool, pageLimit int) ([]string, *client.SearchMessagesResult, error) {
	var ids []string
	var last *client.SearchMessagesResult
	pages := 0
	for {
		res, err := client.SearchMessages(opts, token)
		if err != nil {
			return nil, nil, err
		}
		last = res
		ids = append(ids, res.MessageIDs...)
		pages++
		if !pageAll || !res.HasMore || (pageLimit > 0 && pages >= pageLimit) {
			break
		}
		opts.PageToken = res.PageToken
	}
	return ids, last, nil
}

// collectEnrichedMessages 收集 enrich 后的消息，支持 --page-all 翻页（受 --page-limit 限制）。
func collectEnrichedMessages(opts client.SearchMessagesOptions, token, cardContentType string, pageAll bool, pageLimit int) ([]client.EnrichedMessage, *client.SearchMessagesResult, error) {
	var all []client.EnrichedMessage
	var last *client.SearchMessagesResult
	pages := 0
	for {
		enriched, res, err := client.SearchMessagesEnriched(opts, token, cardContentType)
		if err != nil {
			return nil, nil, err
		}
		last = res
		all = append(all, enriched...)
		pages++
		if !pageAll || res == nil || !res.HasMore || (pageLimit > 0 && pages >= pageLimit) {
			break
		}
		opts.PageToken = res.PageToken
	}
	return all, last, nil
}

func printMoreHint(pageAll bool, res *client.SearchMessagesResult) {
	if !pageAll && res != nil && res.HasMore {
		fmt.Printf("\n还有更多结果，使用 --page-token %s 获取下一页（或 --page-all 自动翻页）\n", res.PageToken)
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// truncateRunes 按 rune 截断，超长加省略号。
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func init() {
	searchCmd.AddCommand(searchMessagesCmd)

	searchMessagesCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	searchMessagesCmd.Flags().String("chat-ids", "", "会话 ID 列表（逗号分隔）")
	searchMessagesCmd.Flags().String("from-ids", "", "消息发送者用户 ID 列表（逗号分隔）")
	searchMessagesCmd.Flags().String("at-chatter-ids", "", "@的用户 ID 列表（逗号分隔）")
	searchMessagesCmd.Flags().String("message-type", "", "消息类型（file/image/media）")
	searchMessagesCmd.Flags().String("chat-type", "", "会话类型（group_chat/p2p_chat）")
	searchMessagesCmd.Flags().String("from-type", "", "发送者类型（bot/user）")
	searchMessagesCmd.Flags().String("start-time", "", "消息发送起始时间（Unix 时间戳）")
	searchMessagesCmd.Flags().String("end-time", "", "消息发送结束时间（Unix 时间戳）")
	searchMessagesCmd.Flags().Int("page-size", 20, "每页数量")
	searchMessagesCmd.Flags().String("page-token", "", "分页 token")
	searchMessagesCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型（open_id/union_id/user_id）")
	searchMessagesCmd.Flags().Bool("ids-only", false, "仅返回消息 ID（跳过 enrich）")
	searchMessagesCmd.Flags().StringP("output", "o", "", "输出格式（json，等价 --format json；保留向后兼容）")
	addCardContentTypeFlag(searchMessagesCmd)
	output.AddFormatFlags(searchMessagesCmd)
	output.AddPaginationFlags(searchMessagesCmd)
}
