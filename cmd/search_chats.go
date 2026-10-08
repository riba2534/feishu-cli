package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
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
  --as             身份：bot | user | auto（默认 auto）
  --user-id-type   仅空 query 的 list 回退使用
  --output, -o     输出格式 (json)

示例:
  # 列出所有群聊
  feishu-cli msg search-chats

  # 搜索包含关键词的群聊
  feishu-cli msg search-chats --query "测试群" --as auto

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
		}

		result, err := collectSearchChats(opts, token, pageAll, pageLimit)
		if err != nil {
			return err
		}
		printSearchNotice(cmd.ErrOrStderr(), result.Notice)

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
	searchChatsCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
	searchChatsCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
