package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var msgReactionCmd = &cobra.Command{
	Use:   "reaction",
	Short: "消息表情回复管理",
	Long: `消息表情回复管理命令，用于添加、删除、查询消息的表情回复。

子命令:
  add      添加表情回复
  remove   删除表情回复
  list     查询表情回复列表

常用 emoji 类型:
  THUMBSUP    点赞
  SMILE       微笑
  LAUGH       大笑
  HEART       爱心
  OK          OK
  CLAP        鼓掌
  FIRE        火
  PARTY       派对

示例:
  feishu-cli msg reaction add om_xxx --emoji-type THUMBSUP
  feishu-cli msg reaction remove om_xxx --reaction-id reaction_xxx
  feishu-cli msg reaction list om_xxx`,
}

var msgReactionAddCmd = &cobra.Command{
	Use:   "add <message_id>",
	Short: "添加表情回复",
	Long: `给指定消息添加表情回复。

参数:
  message_id     消息 ID（必填）
  --emoji-type   emoji 类型（必填，如 THUMBSUP/SMILE/LAUGH 等）

示例:
  feishu-cli msg reaction add om_xxx --emoji-type THUMBSUP`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		messageID := args[0]
		emojiType, _ := cmd.Flags().GetString("emoji-type")

		reactionID, err := client.CreateReaction(messageID, emojiType, token)
		if err != nil {
			return err
		}

		fmt.Printf("表情回复添加成功！\n")
		fmt.Printf("  Reaction ID: %s\n", reactionID)

		return nil
	},
}

var msgReactionRemoveCmd = &cobra.Command{
	Use:   "remove <message_id>",
	Short: "删除表情回复",
	Long: `删除指定消息的表情回复。

参数:
  message_id      消息 ID（必填）
  --reaction-id   Reaction ID（必填）

示例:
  feishu-cli msg reaction remove om_xxx --reaction-id reaction_xxx`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		messageID := args[0]
		reactionID, _ := cmd.Flags().GetString("reaction-id")

		if err := client.DeleteReaction(messageID, reactionID, token); err != nil {
			return err
		}

		fmt.Printf("表情回复已删除！\n")

		return nil
	},
}

var msgReactionListCmd = &cobra.Command{
	Use:   "list <message_id>",
	Short: "查询表情回复列表",
	Long: `查询指定消息的表情回复列表。

参数:
  message_id     消息 ID（必填）
  --emoji-type   筛选 emoji 类型（可选）
  --page-size    每页数量
  --page-token   分页标记

示例:
  feishu-cli msg reaction list om_xxx
  feishu-cli msg reaction list om_xxx --emoji-type THUMBSUP`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		messageID := args[0]
		emojiType, _ := cmd.Flags().GetString("emoji-type")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")

		result, err := client.ListReactions(messageID, emojiType, pageSize, pageToken, token)
		if err != nil {
			return err
		}

		return printJSON(result)
	},
}

func init() {
	msgCmd.AddCommand(msgReactionCmd)

	msgReactionCmd.AddCommand(msgReactionAddCmd)
	msgReactionAddCmd.Flags().String("emoji-type", "", "emoji 类型（如 THUMBSUP/SMILE/LAUGH 等）")
	msgReactionAddCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	mustMarkFlagRequired(msgReactionAddCmd, "emoji-type")

	msgReactionCmd.AddCommand(msgReactionRemoveCmd)
	msgReactionRemoveCmd.Flags().String("reaction-id", "", "Reaction ID")
	msgReactionRemoveCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	mustMarkFlagRequired(msgReactionRemoveCmd, "reaction-id")

	msgReactionCmd.AddCommand(msgReactionListCmd)
	msgReactionListCmd.Flags().String("emoji-type", "", "筛选 emoji 类型")
	msgReactionListCmd.Flags().Int("page-size", 0, "每页数量")
	msgReactionListCmd.Flags().String("page-token", "", "分页标记")
	msgReactionListCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
