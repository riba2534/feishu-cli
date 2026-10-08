package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var mergeForwardMsgCmd = &cobra.Command{
	Use:   "merge-forward",
	Short: "合并转发消息",
	Long: `将多条消息合并转发给指定的接收者。

参数:
  --receive-id        接收者 ID（必填）
  --receive-id-type   接收者 ID 类型（默认 email）
  --message-ids       消息 ID 列表（逗号分隔，必填）

身份:
  仅支持 Bot（应用）身份：飞书合并转发接口只接受 tenant_access_token。
  传 --user-access-token / FEISHU_USER_ACCESS_TOKEN 会被忽略并在 stderr 提示。

接收者类型:
  email       邮箱
  open_id     Open ID
  user_id     用户 ID
  union_id    Union ID
  chat_id     群组 ID

示例:
  # 合并转发消息给用户
  feishu-cli msg merge-forward \
    --receive-id user@example.com \
    --receive-id-type email \
    --message-ids om_xxx,om_yyy,om_zzz

  # 合并转发到群聊
  feishu-cli msg merge-forward \
    --receive-id oc_xxx \
    --receive-id-type chat_id \
    --message-ids om_xxx,om_yyy`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		// 合并转发接口只接受 tenant_access_token（Bot 身份）：传入的 User Token 会被
		// 服务端拒绝或被 SDK 静默改用 Bot，这里固定 Bot 并明确提示，避免误以为以本人身份转发。
		if userToken := resolveOptionalUserToken(cmd); userToken != "" {
			fmt.Fprintln(cmd.ErrOrStderr(), "[提示] msg merge-forward 仅支持 Bot（应用）身份，已忽略 User Access Token，以 Bot 身份执行（Bot 需能看到这些消息且在目标会话中）")
		}

		receiveID, _ := cmd.Flags().GetString("receive-id")
		receiveIDType, _ := cmd.Flags().GetString("receive-id-type")
		messageIDsStr, _ := cmd.Flags().GetString("message-ids")

		messageIDs := splitAndTrim(messageIDsStr)
		if len(messageIDs) == 0 {
			return fmt.Errorf("消息 ID 列表不能为空")
		}

		newMessageID, err := client.MergeForwardMessage(receiveID, receiveIDType, messageIDs, "")
		if err != nil {
			return err
		}

		fmt.Printf("消息合并转发成功！\n")
		fmt.Printf("  新消息 ID: %s\n", newMessageID)
		fmt.Printf("  转发数量: %d\n", len(messageIDs))

		return nil
	},
}

func init() {
	msgCmd.AddCommand(mergeForwardMsgCmd)
	mergeForwardMsgCmd.Flags().String("receive-id", "", "接收者 ID")
	mergeForwardMsgCmd.Flags().String("receive-id-type", "email", "接收者 ID 类型（email/open_id/user_id/union_id/chat_id）")
	mergeForwardMsgCmd.Flags().String("message-ids", "", "消息 ID 列表（逗号分隔）")
	mergeForwardMsgCmd.Flags().String("user-access-token", "", "已忽略：合并转发接口仅支持 Bot 身份")
	mustMarkFlagRequired(mergeForwardMsgCmd, "receive-id", "message-ids")
}
