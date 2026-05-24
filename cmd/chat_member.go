package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

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
  需要 App 开启「对外共享能力」+ Bot 在群里。详见 skills/feishu-cli-chat/references/external-chat.md`,
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
	Short: "获取群成员列表",
	Long: `获取指定群聊的成员列表（含群昵称）。

参数:
  chat_id             群 ID（必填）
  --member-id-type    成员 ID 类型（open_id/user_id/union_id，默认 open_id）
  --page-size         每页数量
  --page-token        分页标记
  --as                身份选择（auto/user/bot，默认 auto）

示例:
  feishu-cli chat member list oc_xxx                # 默认 auto
  feishu-cli chat member list oc_xxx --as bot       # 外部群推荐：用 App Token
  feishu-cli chat member list oc_xxx --member-id-type user_id --page-size 50

外部群拉成员推荐用 --as bot（需 App 开了"对外共享能力" + Bot 已加群）。`,
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
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")

		result, err := client.ListChatMembers(chatID, memberIDType, pageSize, pageToken, token)
		if err != nil {
			return translateChatError(err)
		}

		return printJSON(result)
	},
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
	chatMemberListCmd.Flags().Int("page-size", 0, "每页数量")
	chatMemberListCmd.Flags().String("page-token", "", "分页标记")
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
