package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
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
  --types        会话类型：group（默认）/ p2p / p2p,group；p2p（单聊）仅用户身份可列
  --exclude-muted 过滤掉你设置了免打扰的会话（仅用户身份生效）
  -o json        以 JSON 输出

示例:
  feishu-cli chat list                                # 列出前一页
  feishu-cli chat list --page-size 20
  feishu-cli chat list --page-all                     # 拉全量
  feishu-cli chat list --sort-type ByActiveTimeDesc   # 按活跃时间降序
  feishu-cli chat list --types p2p,group --sort-type ByActiveTimeDesc   # 含单聊
  feishu-cli chat list --page-all --exclude-muted     # 只看没设免打扰的群
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
		excludeMuted, _ := cmd.Flags().GetBool("exclude-muted")
		types, err := normalizeChatListTypes(flagString(cmd, "types"))
		if err != nil {
			return err
		}

		// User 优先、Tenant 兜底：已登录列本人加入的群，未登录列 Bot 加入的群。
		token := resolveOptionalUserTokenWithFallback(cmd)

		// Bot 身份出于隐私不能列单聊（对齐官方 bot_strip_p2p）：只要 p2p 时直接报错，混合时去掉 p2p 并提示。
		effectiveTypes, err := resolveChatListTypesForIdentity(cmd.ErrOrStderr(), types, token)
		if err != nil {
			return err
		}

		opts := client.ListChatsOptions{
			UserIDType: userIDType,
			SortType:   sortType,
			Types:      strings.Join(effectiveTypes, ","),
			PageSize:   pageSize,
			PageToken:  pageToken,
		}
		var result *client.ListChatsResult
		if pageAll {
			// 自动翻页时若未显式指定每页大小，用最大值 100 减少往返。
			if !cmd.Flags().Changed("page-size") {
				opts.PageSize = 100
			}
			r, err := listAllChats(opts, token)
			if err != nil {
				return translateChatError(err)
			}
			result = r
		} else {
			r, err := client.ListChatsWithOptions(opts, token)
			if err != nil {
				return translateChatError(err)
			}
			result = r
		}

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
			if c.ChatMode != "" {
				fmt.Printf("    类型: %s\n", c.ChatMode)
			}
			if c.P2PTargetID != "" {
				fmt.Printf("    单聊对象: %s（%s）\n", c.P2PTargetID, c.P2PTargetType)
			}
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
func listAllChats(opts client.ListChatsOptions, token string) (*client.ListChatsResult, error) {
	all := &client.ListChatsResult{}
	pageToken := ""
	for page := 0; page < chatListMaxPages; page++ {
		opts.PageToken = pageToken
		r, err := client.ListChatsWithOptions(opts, token)
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
	chatListCmd.Flags().String("types", "", "会话类型：group / p2p / p2p,group（默认仅群；p2p 仅用户身份）")
	chatListCmd.Flags().Bool("exclude-muted", false, "过滤当前用户设置了免打扰的会话（仅用户身份生效）")
}

// normalizeChatListTypes 校验 --types（group / p2p，逗号分隔，去重保序）。
func normalizeChatListTypes(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	seen := map[string]bool{}
	var out []string
	for _, p := range splitAndTrim(raw) {
		p = strings.ToLower(p)
		if p != "group" && p != "p2p" {
			return nil, clierr.Usagef("--types 仅支持 group、p2p，得到 %q", p)
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out, nil
}

// resolveChatListTypesForIdentity Bot 身份不能列单聊：只要 p2p → 报错；p2p,group → 去掉 p2p 并提示。
func resolveChatListTypesForIdentity(errOut io.Writer, types []string, userToken string) ([]string, error) {
	if userToken != "" || len(types) == 0 {
		return types, nil
	}
	var kept []string
	for _, t := range types {
		if t != "p2p" {
			kept = append(kept, t)
		}
	}
	if len(kept) == len(types) {
		return types, nil
	}
	if len(kept) == 0 {
		return nil, clierr.Usagef("--types p2p（单聊）只支持用户身份：为保护隐私，Bot 不能列出单聊。请先 auth login，或在 --types 中包含 group")
	}
	fmt.Fprintln(errOut, "[提示] 为保护隐私，Bot 身份不能列出单聊，已从 --types 中去掉 p2p（仅列群）；登录后可列出单聊")
	return kept, nil
}
