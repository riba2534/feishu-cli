package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// chatListValidSortTypes 是 im/v1/chats 端点接受的排序方式。
var chatListValidSortTypes = []string{"ByCreateTimeAsc", "ByActiveTimeDesc"}

// chatListPageDelay 是 --page-all 翻页时的页间隔，避免拉大量群时触发频控。
const chatListPageDelay = 200 * time.Millisecond

// chatListMaxPages 是 --page-all 的安全上限，防止服务端异常时无限翻页。
const chatListMaxPages = 1000

var chatListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出当前身份加入的所有群",
	Long: `列出当前身份（User 或 Bot）加入的所有群聊。

默认使用 User Token（列出你本人加入的群），未登录时回退 App Token（列出 Bot 加入的群）。

参数:
  --sort-type    排序方式: ByCreateTimeAsc（创建时间升序）/ ByActiveTimeDesc（活跃时间降序），默认 ByCreateTimeAsc
  --user-id-type 群主 owner_id 的 ID 类型: open_id/union_id/user_id（默认 open_id）
  --page-size    每页数量（1-100）
  --page-token   分页标记（手动翻页时用）
  --page-all     自动翻页拉取全部群（忽略 --page-token）
  -o json        以 JSON 输出

示例:
  feishu-cli chat list                                # 列出前一页
  feishu-cli chat list --page-size 20
  feishu-cli chat list --page-all                     # 拉全量
  feishu-cli chat list --sort-type ByActiveTimeDesc   # 按活跃时间降序
  feishu-cli chat list --page-all -o json | jq '.items[].name'`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		sortType, _ := cmd.Flags().GetString("sort-type")
		if err := validateEnum(sortType, "排序方式（--sort-type）", chatListValidSortTypes); err != nil {
			return err
		}

		userIDType, _ := cmd.Flags().GetString("user-id-type")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		pageAll, _ := cmd.Flags().GetBool("page-all")
		output, _ := cmd.Flags().GetString("output")

		// User 优先、Tenant 兜底：已登录列本人加入的群，未登录列 Bot 加入的群。
		token := resolveOptionalUserTokenWithFallback(cmd)

		var result *client.ListChatsResult
		if pageAll {
			// 自动翻页时若未显式指定每页大小，用最大值 100 减少往返。
			effectiveSize := pageSize
			if !cmd.Flags().Changed("page-size") {
				effectiveSize = 100
			}
			r, err := listAllChats(userIDType, sortType, effectiveSize, token)
			if err != nil {
				return translateChatError(err)
			}
			result = r
		} else {
			r, err := client.ListChats(userIDType, sortType, pageSize, pageToken, token)
			if err != nil {
				return translateChatError(err)
			}
			result = r
		}

		if output == "json" {
			return printJSON(result)
		}

		if len(result.Items) == 0 {
			fmt.Println("当前身份没有加入任何群")
			return nil
		}

		fmt.Printf("群列表（共 %d 个）:\n\n", len(result.Items))
		for i, c := range result.Items {
			fmt.Printf("[%d] %s\n", i+1, c.Name)
			fmt.Printf("    Chat ID: %s\n", c.ChatID)
			if c.Description != "" {
				fmt.Printf("    描述: %s\n", c.Description)
			}
			if c.OwnerID != "" {
				fmt.Printf("    群主: %s\n", c.OwnerID)
			}
			fmt.Printf("    外部群: %v\n", c.External)
			if c.ChatStatus != "" {
				fmt.Printf("    状态: %s\n", c.ChatStatus)
			}
			fmt.Println()
		}

		if result.HasMore {
			fmt.Printf("还有更多群，下一页 token: %s\n", result.PageToken)
			fmt.Println("提示: 加 --page-all 可一次性拉取全部群")
		}

		return nil
	},
}

// listAllChats 自动翻页拉取全部群。带非递增 token 保护与安全页数上限，防止无限循环。
func listAllChats(userIDType, sortType string, pageSize int, token string) (*client.ListChatsResult, error) {
	all := &client.ListChatsResult{}
	pageToken := ""
	for page := 0; page < chatListMaxPages; page++ {
		r, err := client.ListChats(userIDType, sortType, pageSize, pageToken, token)
		if err != nil {
			return nil, err
		}
		all.Items = append(all.Items, r.Items...)

		if !r.HasMore || r.PageToken == "" {
			return all, nil
		}
		if r.PageToken == pageToken {
			// 服务端异常回显相同 token 却仍标记 has_more，停止翻页避免死循环。
			fmt.Fprintln(os.Stderr, "警告: 服务端返回了不推进的分页标记，停止翻页，结果可能不完整")
			return all, nil
		}
		pageToken = r.PageToken
		time.Sleep(chatListPageDelay)
	}
	fmt.Fprintf(os.Stderr, "警告: 已达到翻页上限（%d 页），结果可能不完整\n", chatListMaxPages)
	return all, nil
}

func init() {
	chatCmd.AddCommand(chatListCmd)
	chatListCmd.Flags().String("sort-type", "ByCreateTimeAsc", "排序方式: ByCreateTimeAsc / ByActiveTimeDesc")
	chatListCmd.Flags().String("user-id-type", "open_id", "群主 owner_id 的 ID 类型: open_id/union_id/user_id")
	chatListCmd.Flags().Int("page-size", 0, "每页数量（1-100）")
	chatListCmd.Flags().String("page-token", "", "分页标记")
	chatListCmd.Flags().Bool("page-all", false, "自动翻页拉取全部群（忽略 --page-token）")
	chatListCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	chatListCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
