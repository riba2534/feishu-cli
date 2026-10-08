package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var chatCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建群聊（支持话题群、邀请机器人）",
	Long: `创建一个新的群聊（POST /open-apis/im/v1/chats）。

身份:
  固定 Bot（应用）身份：飞书建群接口只接受 tenant_access_token（对齐官方 #2728）。

参数:
  --name           群名称（必填，最多 60 字；公开群至少 2 字）
  --description    群描述（最多 100 字）
  --owner-id       群主 open_id（不传则 Bot 为群主）
  --user-ids       邀请的成员 open_id 列表（逗号分隔，最多 50）
  --bots           邀请的机器人 app_id 列表（cli_xxx，逗号分隔，最多 5）
  --chat-type      群类型（private/public，默认 private）
  --chat-mode      群模式（group 普通群 / topic 话题群，默认 group）
  --output, -o     输出格式（json）：{chat_id, name, chat_type, chat_mode, owner_id, external, share_link}

创建成功后会尝试获取群分享链接（share_link，失败不影响建群）。

示例:
  # 创建私有群
  feishu-cli chat create --name "测试群"

  # 创建话题群并邀请成员与机器人
  feishu-cli chat create --name "项目话题群" --chat-mode topic --user-ids ou_xxx --bots cli_xxx

  # 创建公开群并邀请成员
  feishu-cli chat create --name "公开群" --chat-type public --user-ids ou_xxx,ou_yyy

  # 指定群主创建群，JSON 输出
  feishu-cli chat create --name "项目群" --owner-id ou_xxx --description "项目讨论群" -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		description, _ := cmd.Flags().GetString("description")
		ownerID, _ := cmd.Flags().GetString("owner-id")
		userIDsStr, _ := cmd.Flags().GetString("user-ids")
		botsStr, _ := cmd.Flags().GetString("bots")
		chatType, _ := cmd.Flags().GetString("chat-type")
		chatMode, _ := cmd.Flags().GetString("chat-mode")
		output, _ := cmd.Flags().GetString("output")

		opts := client.CreateChatOptions{
			Name:        name,
			Description: description,
			OwnerID:     ownerID,
			UserIDs:     splitAndTrim(userIDsStr),
			BotIDs:      splitAndTrim(botsStr),
			ChatType:    chatType,
			ChatMode:    chatMode,
		}
		if err := validateCreateChatOptions(opts); err != nil {
			return err
		}
		if err := config.Validate(); err != nil {
			return err
		}

		created, err := client.CreateChatWithOptions(opts)
		if err != nil {
			return err
		}
		// 分享链接 best-effort：失败只提示，不影响建群结果。
		shareLink, linkErr := client.GetChatLink(created.ChatID, "")
		if linkErr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "[提示] 群已创建，但获取分享链接失败（可稍后用 chat link 重试）: %v\n", linkErr)
		}

		if output == "json" {
			return printJSON(map[string]any{
				"chat_id":    created.ChatID,
				"name":       created.Name,
				"chat_type":  created.ChatType,
				"chat_mode":  created.ChatMode,
				"owner_id":   created.OwnerID,
				"external":   created.External,
				"share_link": shareLink,
			})
		}
		fmt.Printf("群聊创建成功！\n")
		fmt.Printf("  群 ID: %s\n", created.ChatID)
		if created.ChatMode != "" {
			fmt.Printf("  群模式: %s\n", created.ChatMode)
		}
		if shareLink != "" {
			fmt.Printf("  分享链接: %s\n", shareLink)
		}
		return nil
	},
}

// validateCreateChatOptions 前置校验（对齐官方 +chat-create）。
func validateCreateChatOptions(opts client.CreateChatOptions) error {
	nameLen := len([]rune(opts.Name))
	if strings.TrimSpace(opts.Name) == "" {
		return clierr.Usagef("--name 不能为空")
	}
	if nameLen > 60 {
		return clierr.Usagef("--name 最多 60 个字符，得到 %d 个", nameLen)
	}
	if opts.ChatType == "public" && nameLen < 2 {
		return clierr.Usagef("公开群的 --name 至少 2 个字符")
	}
	if n := len([]rune(opts.Description)); n > 100 {
		return clierr.Usagef("--description 最多 100 个字符，得到 %d 个", n)
	}
	switch opts.ChatType {
	case "", "private", "public":
	default:
		return clierr.Usagef("--chat-type 仅支持 private、public，得到 %q", opts.ChatType)
	}
	switch opts.ChatMode {
	case "", "group", "topic":
	default:
		return clierr.Usagef("--chat-mode 仅支持 group、topic，得到 %q", opts.ChatMode)
	}
	if len(opts.UserIDs) > 50 {
		return clierr.Usagef("--user-ids 最多 50 个，得到 %d 个", len(opts.UserIDs))
	}
	if len(opts.BotIDs) > 5 {
		return clierr.Usagef("--bots 最多 5 个，得到 %d 个", len(opts.BotIDs))
	}
	for _, id := range opts.BotIDs {
		if !strings.HasPrefix(id, "cli_") {
			return clierr.Usagef("--bots 需要机器人 app_id（cli_xxx），得到 %q", id)
		}
	}
	return nil
}

func init() {
	chatCmd.AddCommand(chatCreateCmd)
	chatCreateCmd.Flags().String("name", "", "群名称")
	chatCreateCmd.Flags().String("description", "", "群描述")
	chatCreateCmd.Flags().String("owner-id", "", "群主 open_id")
	chatCreateCmd.Flags().String("user-ids", "", "邀请的成员 open_id 列表（逗号分隔，最多 50）")
	chatCreateCmd.Flags().String("bots", "", "邀请的机器人 app_id 列表（cli_xxx，逗号分隔，最多 5）")
	chatCreateCmd.Flags().String("chat-type", "private", "群类型（private/public）")
	chatCreateCmd.Flags().String("chat-mode", "group", "群模式（group 普通群 / topic 话题群）")
	chatCreateCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mustMarkFlagRequired(chatCreateCmd, "name")
}
