package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/riba2534/feishu-cli/v2/internal/output"
	"github.com/spf13/cobra"
)

var searchMessagesCmd = &cobra.Command{
	Use:   "messages [query]",
	Short: "搜索消息（默认返回消息 ID，--enrich 补全内容/发送者/群名/时间）",
	Long: `搜索飞书消息（POST /open-apis/im/v1/messages/search）。默认返回消息 ID
（人类可读列表 + -o json 返回 {MessageIDs,HasMore,PageToken}）。

query 可省略，仅用 filter 搜索。加 --enrich 才补全内容/发送者/群名/时间。

身份：--as bot|user|auto（默认 auto = User 优先，未配置回落 Bot；已配置 User
但刷新失败 fail-closed，不会静默切 Bot）。

选项:
  --chat-ids              指定搜索的会话 ID 列表（逗号分隔）
  --from-ids              指定消息发送者用户 ID 列表（逗号分隔）
  --message-type          附件类型（file/image/media/video/link；media→video）
  --chat-type             会话类型（group_chat/p2p_chat 或 group/p2p）
  --from-type             发送者类型（bot/user）
  --exclude-from-type     排除发送者类型（bot/user）
  --is-at-me              仅搜索 @我 的消息
  --start-time            起始时间（RFC3339 / YYYY-MM-DD / Unix 秒）
  --end-time              结束时间（RFC3339 / YYYY-MM-DD / Unix 秒）
  --page-size             每页数量（1-50，默认 20；越界报错）
  --page-token            分页 token
  --page-all              自动翻页（最多 40 页）
  --page-limit            自动翻页页数（1-40；0 在 --page-all 时等于 40，不是无限）
  --enrich                补全内容/发送者/群名/时间（额外 API 调用，opt-in）
  --as                    身份：bot | user | auto
  --format                结构化输出: json | pretty | table | ndjson | csv
  --jq                    用 jq 表达式过滤结构化输出

示例:
  # 搜索包含"会议"的消息（默认返回消息 ID，人类可读）
  feishu-cli search messages "会议"

  # JSON 输出消息 ID（旧 schema：{MessageIDs,HasMore,PageToken}）
  feishu-cli search messages "会议" -o json

  # 富化：补全内容/发送者/群名/时间
  feishu-cli search messages "会议" --enrich

  # 富化 + 表格输出 + jq 只看发送者和文本
  feishu-cli search messages "会议" --enrich --format table
  feishu-cli search messages "会议" --enrich --jq '.[] | {sender_name, text}'

  # 富化 + 指定会话 + 自动翻页 + CSV
  feishu-cli search messages "项目" --enrich --chat-ids oc_xxx --page-all --page-limit 5 --format csv`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		query := ""
		if len(args) == 1 {
			query = args[0]
		}

		chatIDsStr, _ := cmd.Flags().GetString("chat-ids")
		fromIDsStr, _ := cmd.Flags().GetString("from-ids")
		atChatterIDsStr, _ := cmd.Flags().GetString("at-chatter-ids")
		messageType, _ := cmd.Flags().GetString("message-type")
		chatType, _ := cmd.Flags().GetString("chat-type")
		fromType, _ := cmd.Flags().GetString("from-type")
		excludeFromType, _ := cmd.Flags().GetString("exclude-from-type")
		isAtMe, _ := cmd.Flags().GetBool("is-at-me")
		startTime, _ := cmd.Flags().GetString("start-time")
		endTime, _ := cmd.Flags().GetString("end-time")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		enrich, _ := cmd.Flags().GetBool("enrich")
		pageAll, _ := cmd.Flags().GetBool("page-all")
		pageLimit, _ := cmd.Flags().GetInt("page-limit")
		legacyOutput, _ := cmd.Flags().GetString("output")
		jq, _ := cmd.Flags().GetString("jq")

		opts := client.SearchMessagesOptions{
			Query:           query,
			ChatIDs:         splitAndTrim(chatIDsStr),
			FromIDs:         splitAndTrim(fromIDsStr),
			AtChatterIDs:    splitAndTrim(atChatterIDsStr),
			MessageType:     messageType,
			ChatType:        chatType,
			FromType:        fromType,
			ExcludeFromType: excludeFromType,
			IsAtMe:          isAtMe,
			StartTime:       startTime,
			EndTime:         endTime,
			PageSize:        pageSize,
			PageToken:       pageToken,
		}
		if err := client.ValidateSearchMessagesOptions(opts); err != nil {
			return err
		}
		pageLimit, err := client.ResolvePageLimit(pageLimit, client.SearchPageLimitMax, pageAll)
		if err != nil {
			return err
		}

		// 结构化输出校验必须在身份解析/刷新和发网之前。
		useStructured := cmd.Flags().Changed("format") || jq != "" || legacyOutput == "json"
		formatVal := output.FormatJSON
		if cmd.Flags().Changed("format") {
			formatVal, _ = cmd.Flags().GetString("format")
		}
		var structuredOpts *output.Options
		if useStructured {
			o, oerr := output.NewOptions(formatVal, jq)
			if oerr != nil {
				return oerr
			}
			structuredOpts = o
		}

		userAccessToken, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}

		// 默认（向后兼容）：仅翻页收集消息 ID，不做 enrich。
		// -o json / --format json 返回旧 schema {MessageIDs,HasMore,PageToken}。
		if !enrich {
			ids, lastRes, err := collectMessageIDs(opts, userAccessToken, pageAll, pageLimit)
			if err != nil {
				return err
			}
			printSearchNotice(cmd.ErrOrStderr(), lastRes.Notice)
			if useStructured {
				// 渲染 SearchMessagesResult → 旧 schema {MessageIDs,PageToken,HasMore}（+ notice，非空时）。
				// MessageIDs 用翻页累计的全量 ids；HasMore/PageToken 取最后一页。
				return output.Render(structuredOpts, &client.SearchMessagesResult{
					MessageIDs: ids,
					PageToken:  lastRes.PageToken,
					HasMore:    lastRes.HasMore,
					Notice:     lastRes.Notice,
				})
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

		// --enrich（opt-in）：card-content-type 仅 enrich 路径消费，故校验下沉到此，
		// 不让默认（非 enrich）模式因非法 --card-content-type 被提前 abort。
		cardContentType, err := resolveCardContentType(cmd)
		if err != nil {
			return err
		}
		enriched, lastRes, err := collectEnrichedMessages(opts, userAccessToken, cardContentType, pageAll, pageLimit)
		if err != nil {
			return err
		}
		if lastRes != nil {
			printSearchNotice(cmd.ErrOrStderr(), lastRes.Notice)
		}

		if useStructured {
			return output.Render(structuredOpts, enriched)
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
	notice := ""
	pages := 0
	defer func() {
		// 服务端 notice 往往只出现在首页，翻页后保留下来，避免被后续页覆盖丢失
		if last != nil && last.Notice == "" {
			last.Notice = notice
		}
	}()
	for {
		res, err := client.SearchMessages(opts, token)
		if err != nil {
			return nil, nil, err
		}
		last = res
		if notice == "" {
			notice = res.Notice
		}
		ids = append(ids, res.MessageIDs...)
		pages++
		more, next, err := client.PaginationCursor(res.HasMore, res.PageToken, "", opts.PageToken)
		if err != nil {
			return nil, nil, err
		}
		if !pageAll || !more || (pageLimit > 0 && pages >= pageLimit) {
			break
		}
		opts.PageToken = next
	}
	return ids, last, nil
}

// collectEnrichedMessages 收集 enrich 后的消息，支持 --page-all 翻页（受 --page-limit 限制）。
func collectEnrichedMessages(opts client.SearchMessagesOptions, token, cardContentType string, pageAll bool, pageLimit int) ([]client.EnrichedMessage, *client.SearchMessagesResult, error) {
	var all []client.EnrichedMessage
	var last *client.SearchMessagesResult
	notice := ""
	pages := 0
	defer func() {
		if last != nil && last.Notice == "" {
			last.Notice = notice
		}
	}()
	for {
		enriched, res, err := client.SearchMessagesEnriched(opts, token, cardContentType)
		if err != nil {
			return nil, nil, err
		}
		last = res
		if res != nil && notice == "" {
			notice = res.Notice
		}
		all = append(all, enriched...)
		pages++
		hasMore := res != nil && res.HasMore
		pageTok := ""
		if res != nil {
			pageTok = res.PageToken
		}
		more, next, err := client.PaginationCursor(hasMore, pageTok, "", opts.PageToken)
		if err != nil {
			return nil, nil, err
		}
		if !pageAll || !more || (pageLimit > 0 && pages >= pageLimit) {
			break
		}
		opts.PageToken = next
	}
	return all, last, nil
}

// printSearchNotice 把服务端 notice（如"查询词超过 50 字已截断"）透出到 stderr，
// 避免用户误以为结果是按完整查询词匹配的。
func printSearchNotice(w io.Writer, notice string) {
	if strings.TrimSpace(notice) == "" {
		return
	}
	fmt.Fprintf(w, "[提示] 服务端提示: %s\n", notice)
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
	searchMessagesCmd.Flags().String("as", "auto", "身份选择: bot | user | auto（默认 auto）")
	searchMessagesCmd.Flags().String("chat-ids", "", "会话 ID 列表（逗号分隔）")
	searchMessagesCmd.Flags().String("from-ids", "", "消息发送者用户 ID 列表（逗号分隔）")
	searchMessagesCmd.Flags().String("at-chatter-ids", "", "@的用户 ID 列表（逗号分隔）")
	searchMessagesCmd.Flags().String("message-type", "", "附件类型（file/image/media/video/link；media 映射为 video）")
	searchMessagesCmd.Flags().String("chat-type", "", "会话类型（group_chat/p2p_chat 或 group/p2p）")
	searchMessagesCmd.Flags().String("from-type", "", "发送者类型（bot/user）")
	searchMessagesCmd.Flags().String("exclude-from-type", "", "排除发送者类型（bot/user）")
	searchMessagesCmd.Flags().Bool("is-at-me", false, "仅搜索 @我 的消息")
	searchMessagesCmd.Flags().String("start-time", "", "消息发送起始时间（RFC3339 / YYYY-MM-DD / Unix 秒）")
	searchMessagesCmd.Flags().String("end-time", "", "消息发送结束时间（RFC3339 / YYYY-MM-DD / Unix 秒）")
	searchMessagesCmd.Flags().Int("page-size", 20, "每页数量（1-50）")
	searchMessagesCmd.Flags().String("page-token", "", "分页 token")
	// page-all / page-limit 由 AddPaginationFlags 注册；--page-all 最多 40 页。
	searchMessagesCmd.Flags().String("user-id-type", "open_id", "已废弃：current /im/v1/messages/search 忽略此参数")
	_ = searchMessagesCmd.Flags().MarkHidden("user-id-type")
	searchMessagesCmd.Flags().Bool("enrich", false, "补全内容/发送者/群名/时间（额外 API 调用，opt-in）")
	searchMessagesCmd.Flags().StringP("output", "o", "", "输出格式（json，等价 --format json；保留向后兼容）")
	addCardContentTypeFlag(searchMessagesCmd)
	output.AddFormatFlags(searchMessagesCmd)
	output.AddPaginationFlags(searchMessagesCmd)
	if f := searchMessagesCmd.Flags().Lookup("page-all"); f != nil {
		f.Usage = "自动翻页拉取结果（最多 40 页）"
	}
	if f := searchMessagesCmd.Flags().Lookup("page-limit"); f != nil {
		f.Usage = "自动翻页页数（1-40；0 在 --page-all 时等于 40，不是无限）"
	}
}
