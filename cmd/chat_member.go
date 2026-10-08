package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// chatMemberPageDelay 是 --page-all 翻页时的页间隔，避免拉大群成员时触发频控。
const chatMemberPageDelay = 200 * time.Millisecond

// chatMemberMaxPages 是 --page-all 的安全上限，防止服务端异常时无限翻页。
const chatMemberMaxPages = 1000

var chatMemberCmd = &cobra.Command{
	Use:   "member",
	Short: "群成员管理",
	Long: `群成员管理命令，用于查询、添加、移除群成员。

子命令:
  list     获取群成员列表
  add      添加群成员
  remove   移除群成员

身份选择 (--as):
  auto  默认。优先 User Token（自动加载 ~/.feishu-cli/token.json），无则回退 Bot Token
  user  强制 User Token（未登录直接报错）
  bot   强制 Bot Token（App Token），常用于外部群（需 App 开启"对外共享能力"）

示例:
  feishu-cli chat member list oc_xxx
  feishu-cli chat member list oc_xxx --as bot     # 外部群推荐
  feishu-cli chat member add oc_xxx --id-list ou_xxx,ou_yyy
  feishu-cli chat member remove oc_xxx --id-list ou_xxx

外部群提示:
  外部群（external=true）所有"群信息/成员/配置"类 API 默认 232033 拒绝。
  需要 App 开启「对外共享能力」+ Bot 在群里。详见 skills/feishu-cli-messaging/references/workflows/chat/references/external-chat.md`,
}

// resolveChatToken 按 --as 解析应该传给 client 的 token 字符串。
// 返回空字符串表示走 App/Tenant Token（Bot 身份）。
func resolveChatToken(cmd *cobra.Command, asFlag string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(asFlag)) {
	case "bot", "tenant", "app":
		return "", nil
	case "user":
		token, err := resolveRequiredUserToken(cmd)
		if err != nil {
			return "", fmt.Errorf("--as user 需要 User Access Token（请先 `feishu-cli auth login`）: %w", err)
		}
		return token, nil
	case "", "auto":
		return resolveOptionalUserTokenWithFallback(cmd), nil
	default:
		return "", fmt.Errorf("--as 仅支持 bot|user|auto，得到 %q", asFlag)
	}
}

var chatMemberListCmd = &cobra.Command{
	Use:   "list <chat_id>",
	Short: "获取群成员列表（含群内机器人）",
	Long: `获取指定群聊的成员列表（含群昵称），用户与机器人分开返回。

底层走 GET /open-apis/im/v1/chats/{chat_id}/members/list（旧端点拿不到群内机器人）。

参数:
  chat_id             群 ID（必填）
  --member-id-type    成员 ID 类型（open_id/user_id/union_id，默认 open_id）
  --member-types      只返回某类成员：user / bot / user,bot（默认全部）
  --page-size         每页数量（1-100）
  --page-token        分页标记
  --page-all          自动翻页拉取全部成员（忽略 --page-token）
  --as                身份选择（auto/user/bot，默认 auto）

输出（JSON）:
  users[]         用户成员；items[] 与 users[] 相同（兼容旧版字段，仍只含用户）
  bots[]          群内机器人（member_id / name / app_id）
  truncations[]   非空表示服务端因群安全设置截断了某类成员名单（名单不完整）
  user_total / bot_total、has_more / page_token

示例:
  feishu-cli chat member list oc_xxx                # 默认 auto
  feishu-cli chat member list oc_xxx --as bot       # 外部群推荐：用 App Token
  feishu-cli chat member list oc_xxx --member-types bot
  feishu-cli chat member list oc_xxx --member-id-type user_id --page-size 50
  feishu-cli chat member list oc_xxx --page-all     # 拉全量成员

外部群拉成员推荐用 --as bot（需 App 开了"对外共享能力" + Bot 已加群）。
服务端截断名单时会在 stderr 打印告警，并在 truncations[] 中说明。`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		memberTypes, err := normalizeChatMemberTypes(flagString(cmd, "member-types"))
		if err != nil {
			return err
		}
		if err := config.Validate(); err != nil {
			return err
		}

		asFlag, _ := cmd.Flags().GetString("as")
		token, err := resolveChatToken(cmd, asFlag)
		if err != nil {
			return err
		}

		chatID := args[0]
		memberIDType, _ := cmd.Flags().GetString("member-id-type")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		pageAll, _ := cmd.Flags().GetBool("page-all")

		opts := client.ChatMembersListOptions{
			MemberIDType: memberIDType,
			MemberTypes:  memberTypes,
			PageSize:     pageSize,
			PageToken:    pageToken,
		}

		var result *client.ListChatMembersResult
		if pageAll {
			// 自动翻页时若未显式指定每页大小，用最大值 100 减少往返。
			if !cmd.Flags().Changed("page-size") {
				opts.PageSize = 100
			}
			opts.PageToken = ""
			result, err = listAllChatMembers(chatID, opts, token)
		} else {
			result, err = client.ListChatMembersV2(chatID, opts, token)
		}
		if err != nil {
			return translateChatError(err)
		}
		warnChatMemberTruncations(cmd.ErrOrStderr(), result)
		if !pageAll && result.HasMore {
			fmt.Fprintf(cmd.ErrOrStderr(), "[提示] 还有更多成员：带 --page-token %s 继续，或用 --page-all 拉取全部\n", result.PageToken)
		}
		return printJSON(result)
	},
}

// normalizeChatMemberTypes 校验 --member-types（user/bot，逗号分隔，去重保序）。
func normalizeChatMemberTypes(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	seen := map[string]bool{}
	var out []string
	for _, p := range splitAndTrim(raw) {
		p = strings.ToLower(p)
		if p != "user" && p != "bot" {
			return "", clierr.Usagef("--member-types 仅支持 user、bot（逗号分隔），得到 %q", p)
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return strings.Join(out, ","), nil
}

// warnChatMemberTruncations 服务端因群安全设置截断某类成员时，在 stderr 明确告警，
// 避免 Agent 把不完整的名单当成全量。
func warnChatMemberTruncations(w io.Writer, res *client.ListChatMembersResult) {
	if res == nil {
		return
	}
	for _, t := range res.Truncations {
		memberType, _ := t["member_type"].(string)
		if memberType == "" {
			memberType = "member"
		}
		fmt.Fprintf(w, "警告: 服务端因群安全设置截断了 %s 成员名单（上限 %v），数据不完整（见 truncations）\n", memberType, t["limit"])
	}
}

// listAllChatMembers 自动翻页拉取全部群成员（用户 + 机器人）。
// 带非推进 token 保护与安全页数上限，防止服务端异常时无限循环；
// truncations / *_total / has_more 取最后一页。
func listAllChatMembers(chatID string, opts client.ChatMembersListOptions, token string) (*client.ListChatMembersResult, error) {
	all := &client.ListChatMembersResult{
		Items:       []*client.ChatMemberInfo{},
		Users:       []*client.ChatMemberInfo{},
		Bots:        []*client.ChatBotMemberInfo{},
		Truncations: []map[string]any{},
	}
	for page := 0; page < chatMemberMaxPages; page++ {
		p, err := client.ListChatMembersV2(chatID, opts, token)
		if err != nil {
			return nil, err
		}
		all.AppendPage(p)

		if !p.HasMore || p.PageToken == "" {
			break
		}
		if p.PageToken == opts.PageToken {
			// 服务端异常回显相同 token 却仍标记 has_more，停止翻页避免死循环。
			fmt.Fprintln(os.Stderr, "警告: 服务端返回了不推进的分页标记，停止翻页，成员名单可能不完整")
			break
		}
		opts.PageToken = p.PageToken
		time.Sleep(chatMemberPageDelay)
	}

	// 翻页结束后服务端声称的用户总数仍大于已取回条数：名单不完整（兜底，truncations 为主信号）。
	if all.UserTotal != nil && *all.UserTotal > len(all.Items) && len(all.Truncations) == 0 && !all.HasMore {
		fmt.Fprintf(os.Stderr,
			"警告: 该群用户成员总数为 %d，但仅能取回 %d 条，成员名单可能不完整\n",
			*all.UserTotal, len(all.Items))
	}
	return all, nil
}

var chatMemberAddCmd = &cobra.Command{
	Use:   "add <chat_id>",
	Short: "添加群成员",
	Long: `向指定群聊添加成员。

参数:
  chat_id             群 ID（必填）
  --id-list           成员 ID 列表（逗号分隔，必填）
  --member-id-type    成员 ID 类型（open_id/user_id/union_id/app_id，默认 open_id）
  --as                身份选择（auto/user/bot，默认 auto）

示例:
  feishu-cli chat member add oc_xxx --id-list ou_xxx,ou_yyy
  feishu-cli chat member add oc_xxx --id-list user_xxx --member-id-type user_id`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		asFlag, _ := cmd.Flags().GetString("as")
		token, err := resolveChatToken(cmd, asFlag)
		if err != nil {
			return err
		}

		chatID := args[0]
		memberIDType, _ := cmd.Flags().GetString("member-id-type")
		idListStr, _ := cmd.Flags().GetString("id-list")

		idList := splitAndTrim(idListStr)
		if len(idList) == 0 {
			return fmt.Errorf("成员 ID 列表不能为空")
		}

		if err := client.AddChatMembers(chatID, memberIDType, idList, token); err != nil {
			return translateChatError(err)
		}

		fmt.Printf("群成员添加成功！\n")
		fmt.Printf("  群 ID: %s\n", chatID)
		fmt.Printf("  添加数量: %d\n", len(idList))

		return nil
	},
}

var chatMemberRemoveCmd = &cobra.Command{
	Use:   "remove <chat_id>",
	Short: "移除群成员",
	Long: `从指定群聊移除成员。

参数:
  chat_id             群 ID（必填）
  --id-list           成员 ID 列表（逗号分隔，必填）
  --member-id-type    成员 ID 类型（open_id/user_id/union_id/app_id，默认 open_id）
  --as                身份选择（auto/user/bot，默认 auto）

示例:
  feishu-cli chat member remove oc_xxx --id-list ou_xxx,ou_yyy
  feishu-cli chat member remove oc_xxx --id-list user_xxx --member-id-type user_id`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		asFlag, _ := cmd.Flags().GetString("as")
		token, err := resolveChatToken(cmd, asFlag)
		if err != nil {
			return err
		}

		chatID := args[0]
		memberIDType, _ := cmd.Flags().GetString("member-id-type")
		idListStr, _ := cmd.Flags().GetString("id-list")

		idList := splitAndTrim(idListStr)
		if len(idList) == 0 {
			return fmt.Errorf("成员 ID 列表不能为空")
		}

		if err := client.RemoveChatMembers(chatID, memberIDType, idList, token); err != nil {
			return translateChatError(err)
		}

		fmt.Printf("群成员移除成功！\n")
		fmt.Printf("  群 ID: %s\n", chatID)
		fmt.Printf("  移除数量: %d\n", len(idList))

		return nil
	},
}

func init() {
	chatCmd.AddCommand(chatMemberCmd)

	// list 子命令
	chatMemberCmd.AddCommand(chatMemberListCmd)
	chatMemberListCmd.Flags().String("member-id-type", "open_id", "成员 ID 类型（open_id/user_id/union_id）")
	chatMemberListCmd.Flags().String("member-types", "", "只返回某类成员：user / bot / user,bot（默认全部）")
	chatMemberListCmd.Flags().Int("page-size", 0, "每页数量（1-100）")
	chatMemberListCmd.Flags().String("page-token", "", "分页标记")
	chatMemberListCmd.Flags().Bool("page-all", false, "自动翻页拉取全部成员（忽略 --page-token）")
	chatMemberListCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	chatMemberListCmd.Flags().String("as", "auto", "身份选择: bot | user | auto（默认 auto = User 优先，回退 Bot）")

	// add 子命令
	chatMemberCmd.AddCommand(chatMemberAddCmd)
	chatMemberAddCmd.Flags().String("member-id-type", "open_id", "成员 ID 类型（open_id/user_id/union_id/app_id）")
	chatMemberAddCmd.Flags().String("id-list", "", "成员 ID 列表（逗号分隔）")
	chatMemberAddCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	chatMemberAddCmd.Flags().String("as", "auto", "身份选择: bot | user | auto")
	mustMarkFlagRequired(chatMemberAddCmd, "id-list")

	// remove 子命令
	chatMemberCmd.AddCommand(chatMemberRemoveCmd)
	chatMemberRemoveCmd.Flags().String("member-id-type", "open_id", "成员 ID 类型（open_id/user_id/union_id/app_id）")
	chatMemberRemoveCmd.Flags().String("id-list", "", "成员 ID 列表（逗号分隔）")
	chatMemberRemoveCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	chatMemberRemoveCmd.Flags().String("as", "auto", "身份选择: bot | user | auto")
	mustMarkFlagRequired(chatMemberRemoveCmd, "id-list")
}
