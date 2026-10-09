package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

// msgFlagCmd 是 msg flag 子命令组的入口，挂载在 msg 之下。
// 飞书消息书签分两层：
//   - message 层（item_type=default, flag_type=message）：消息本身被收藏，最常见用法
//   - feed   层（item_type=thread|msg_thread, flag_type=feed）：会话/线程在侧边栏置顶
var msgFlagCmd = &cobra.Command{
	Use:   "flag",
	Short: "消息书签（收藏/取消收藏/列表）",
	Long: `消息书签，对应飞书 OpenAPI /im/v1/flags。

子命令:
  create   为消息创建书签
  list     列出当前用户的书签
  cancel   取消（删除）书签

权限要求:
  必须使用 User Access Token；list 需要 im:feed.flag:read，create/cancel 需要 im:feed.flag:write

支持的 item_type × flag_type 组合:
  default     + message  消息层（最常见）
  thread      + feed     话题群（topic-style）feed 层
  msg_thread  + feed     普通群消息线程 feed 层

示例:
  feishu-cli msg flag create om_xxx
  feishu-cli msg flag list --page-size 20
  feishu-cli msg flag cancel om_xxx
  feishu-cli msg flag cancel om_xxx --item-type thread --flag-type feed`,
}

func init() {
	msgCmd.AddCommand(msgFlagCmd)
}

func parseMsgFlagTypes(cmd *cobra.Command) (itemTypeStr, flagTypeStr string, itemType, flagType int, err error) {
	itemTypeStr, _ = cmd.Flags().GetString("item-type")
	flagTypeStr, _ = cmd.Flags().GetString("flag-type")

	if shouldAutoDetectMsgFlagItemType(cmd) {
		flagType, err = client.ParseFlagFlagType(flagTypeStr)
		if err != nil {
			return "", "", 0, 0, err
		}
		return "auto", flagTypeStr, 0, flagType, nil
	}

	itemType, err = client.ParseFlagItemType(itemTypeStr)
	if err != nil {
		return "", "", 0, 0, err
	}
	flagType, err = client.ParseFlagFlagType(flagTypeStr)
	if err != nil {
		return "", "", 0, 0, err
	}
	if err := validateMsgFlagCombination(itemTypeStr, flagTypeStr); err != nil {
		return "", "", 0, 0, err
	}
	return itemTypeStr, flagTypeStr, itemType, flagType, nil
}

func shouldAutoDetectMsgFlagItemType(cmd *cobra.Command) bool {
	flagTypeStr, _ := cmd.Flags().GetString("flag-type")
	return strings.ToLower(strings.TrimSpace(flagTypeStr)) == "feed" && !cmd.Flags().Changed("item-type")
}

func shouldDoubleCancelMsgFlag(cmd *cobra.Command) bool {
	return !cmd.Flags().Changed("item-type") && !cmd.Flags().Changed("flag-type")
}

func validateMsgFlagMessageID(messageID string) error {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return fmt.Errorf("message_id 不能为空")
	}
	if strings.HasPrefix(messageID, "omt_") {
		return fmt.Errorf("无效的 message_id %q：omt_ 是 thread_id，书签操作需要 om_ 消息 ID", messageID)
	}
	if !strings.HasPrefix(messageID, "om_") {
		return fmt.Errorf("无效的 message_id %q：书签操作需要 om_ 消息 ID", messageID)
	}
	return nil
}

func validateMsgFlagCombination(itemType, flagType string) error {
	itemType = strings.ToLower(strings.TrimSpace(itemType))
	flagType = strings.ToLower(strings.TrimSpace(flagType))
	if itemType == "" {
		itemType = "default"
	}
	if flagType == "" {
		flagType = "message"
	}
	switch {
	case itemType == "default" && flagType == "message":
		return nil
	case itemType == "thread" && flagType == "feed":
		return nil
	case itemType == "msg_thread" && flagType == "feed":
		return nil
	default:
		return fmt.Errorf("无效的书签类型组合: item-type=%s, flag-type=%s（支持 default+message、thread+feed、msg_thread+feed）", itemType, flagType)
	}
}

func validateMsgFlagPageSize(pageSize int) error {
	if pageSize < 1 || pageSize > 50 {
		return fmt.Errorf("--page-size 必须在 1-50 之间，当前 %d", pageSize)
	}
	return nil
}
