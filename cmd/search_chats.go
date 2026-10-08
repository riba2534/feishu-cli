package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var searchChatsCmd = &cobra.Command{
	Use:   "search-chats",
	Short: "搜索群聊",
	Long: `搜索飞书群聊列表（POST /open-apis/im/v2/chats/search）。
无 --query 时回退列出已加入的群（GET /im/v1/chats）。

参数:
  --query          关键词搜索（含连字符的词会自动加引号）
  --page-token     分页标记（兼容服务端 next_page_token）
  --page-size      分页大小 (1-100)，默认 50；越界报错
  --page-all       自动翻页（最多 40 页）；has_more 但游标为空/重复时失败以免死循环
  --page-limit     自动翻页页数（1-40；0 在 --page-all 时等于 40，不是无限）
  --member-ids     只返回包含这些成员的群（open_id，逗号分隔，最多 50；可不带 --query）
  --chat-modes     群模式过滤：group / topic（需配合 --query 或 --member-ids）
  --sort           排序（降序）：create_time / update_time / member_count
  --exclude-muted  过滤掉你设置了免打扰的群（仅用户身份；Bot 身份提示后返回全部）
  --as             身份：bot | user | auto（默认 auto）
  --user-id-type   仅空 query 的 list 回退使用
  --output, -o     输出格式 (json)

示例:
  # 列出所有群聊
  feishu-cli msg search-chats

  # 搜索包含关键词的群聊
  feishu-cli msg search-chats --query "测试群" --as auto

  # 我和某人共同所在的话题群，按成员数排序
  feishu-cli msg search-chats --member-ids ou_xxx --chat-modes topic --sort member_count

  # 分页获取
  feishu-cli msg search-chats --page-size 20 --page-token xxx

  # JSON 格式输出
  feishu-cli msg search-chats -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		userIDType, _ := cmd.Flags().GetString("user-id-type")
		query, _ := cmd.Flags().GetString("query")
		pageToken, _ := cmd.Flags().GetString("page-token")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageAll, _ := cmd.Flags().GetBool("page-all")
		pageLimit, _ := cmd.Flags().GetInt("page-limit")
		output, _ := cmd.Flags().GetString("output")

		memberIDs := splitAndTrim(flagString(cmd, "member-ids"))
		chatModes := splitAndTrim(flagString(cmd, "chat-modes"))
		sortField := flagString(cmd, "sort")
		excludeMuted, _ := cmd.Flags().GetBool("exclude-muted")
		if err := validateSearchChatsFilters(query, memberIDs, chatModes, sortField); err != nil {
			return err
		}

		if _, err := client.ResolvePageSize(pageSize, 50, 1, 100); err != nil {
			return err
		}
		pageLimit, err := client.ResolvePageLimit(pageLimit, client.SearchPageLimitMax, pageAll)
		if err != nil {
			return err
		}

		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}

		opts := client.SearchChatsOptions{
			UserIDType: userIDType,
			Query:      query,
			PageToken:  pageToken,
			PageSize:   pageSize,
			MemberIDs:  memberIDs,
			ChatModes:  chatModes,
			Sort:       sortField,
		}

		result, err := collectSearchChats(opts, token, pageAll, pageLimit)
		if err != nil {
			return err
		}
		printSearchNotice(cmd.ErrOrStderr(), result.Notice)

		if excludeMuted {
			ids := make([]string, 0, len(result.Items))
			for _, c := range result.Items {
				ids = append(ids, c.ChatID)
			}
			if muted := fetchMutedChatSet(cmd.ErrOrStderr(), ids, token); muted != nil {
				kept := result.Items[:0]
				for _, c := range result.Items {
					if !muted[c.ChatID] {
						kept = append(kept, c)
					}
				}
				filtered := len(result.Items) - len(kept)
				result.Items = kept
				printMuteFilterResult(cmd.ErrOrStderr(), filtered, len(kept), result.HasMore)
			}
		}

		if output == "json" {
			if err := printJSON(result); err != nil {
				return err
			}
		} else {
			fmt.Printf("找到 %d 个群聊:\n\n", len(result.Items))
			for i, chat := range result.Items {
				fmt.Printf("[%d] %s\n", i+1, chat.Name)
				fmt.Printf("    群聊 ID: %s\n", chat.ChatID)
				if chat.Description != "" {
					fmt.Printf("    描述: %s\n", chat.Description)
				}
				if chat.OwnerID != "" {
					fmt.Printf("    群主: %s\n", chat.OwnerID)
				}
				if chat.ChatMode != "" {
					fmt.Printf("    群模式: %s\n", chat.ChatMode)
				}
				fmt.Println()
			}
			if result.HasMore {
				fmt.Printf("还有更多结果，使用 --page-token %s 获取下一页\n", result.PageToken)
			}
		}

		return nil
	},
}

func collectSearchChats(opts client.SearchChatsOptions, token string, pageAll bool, pageLimit int) (*client.SearchChatsResult, error) {
	var all []*client.ChatInfo
	var last *client.SearchChatsResult
	notice := ""
	pages := 0
	for {
		res, err := client.SearchChats(opts, token)
		if err != nil {
			return nil, err
		}
		last = res
		if notice == "" {
			notice = res.Notice
		}
		all = append(all, res.Items...)
		pages++
		more, next, err := client.PaginationCursor(res.HasMore, res.PageToken, "", opts.PageToken)
		if err != nil {
			return nil, err
		}
		if !pageAll || !more || (pageLimit > 0 && pages >= pageLimit) {
			break
		}
		opts.PageToken = next
	}
	if last == nil {
		return &client.SearchChatsResult{}, nil
	}
	last.Items = all
	if last.Notice == "" {
		last.Notice = notice
	}
	return last, nil
}

func init() {
	msgCmd.AddCommand(searchChatsCmd)
	searchChatsCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型 (open_id/union_id/user_id)，仅空 query 的 list 回退使用")
	searchChatsCmd.Flags().String("query", "", "关键词搜索")
	searchChatsCmd.Flags().String("page-token", "", "分页标记")
	searchChatsCmd.Flags().Int("page-size", 50, "分页大小 (1-100)")
	searchChatsCmd.Flags().Bool("page-all", false, "自动翻页拉取结果（最多 40 页）")
	searchChatsCmd.Flags().Int("page-limit", 0, "自动翻页页数（1-40；0 在 --page-all 时等于 40）")
	searchChatsCmd.Flags().String("as", "auto", "身份选择: bot | user | auto（默认 auto）")
	searchChatsCmd.Flags().String("member-ids", "", "只返回包含这些成员的群（open_id，逗号分隔，最多 50）")
	searchChatsCmd.Flags().String("chat-modes", "", "群模式过滤：group / topic（逗号分隔）")
	searchChatsCmd.Flags().String("sort", "", "排序字段（降序）：create_time / update_time / member_count")
	searchChatsCmd.Flags().Bool("exclude-muted", false, "过滤当前用户设置了免打扰的群（仅用户身份生效）")
	searchChatsCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
	searchChatsCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}

// validateSearchChatsFilters 校验群搜索过滤参数（对齐官方 +chat-search 校验）。
func validateSearchChatsFilters(query string, memberIDs, chatModes []string, sortField string) error {
	if len(memberIDs) > 50 {
		return clierr.Usagef("--member-ids 最多 50 个，得到 %d 个", len(memberIDs))
	}
	for _, id := range memberIDs {
		if !strings.HasPrefix(id, "ou_") {
			return clierr.Usagef("--member-ids 需要 open_id（ou_xxx），得到 %q", id)
		}
	}
	for _, m := range chatModes {
		if m != "group" && m != "topic" {
			return clierr.Usagef("--chat-modes 仅支持 group、topic，得到 %q", m)
		}
	}
	switch sortField {
	case "", "create_time", "update_time", "member_count":
	default:
		return clierr.Usagef("--sort 仅支持 create_time、update_time、member_count，得到 %q", sortField)
	}
	if query == "" && len(memberIDs) == 0 && (len(chatModes) > 0 || sortField != "") {
		return clierr.Usagef("--chat-modes / --sort 需配合 --query 或 --member-ids 使用（不带条件时走已加入群列表，不支持这些过滤）")
	}
	return nil
}
