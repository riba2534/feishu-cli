package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var msgUrgentCmd = &cobra.Command{
	Use:   "urgent <message_id>",
	Short: "发送加急消息",
	Long: `对指定消息发送加急提醒（应用内/电话/短信）。

参数:
  message_id      消息 ID（必填，不能是批量消息 ID）
  --urgent-type   加急类型（app/phone/sms，默认 app）
  --user-id-type  用户 ID 类型（open_id/user_id/union_id，默认 open_id）
  --user-ids      目标用户 ID 列表（逗号分隔，必填）
  --output, -o    输出格式（json）

注意:
  - 仅支持对机器人自己发送的消息加急
  - 不支持批量消息 ID（bm_xxx）

示例:
  # 发送应用内加急（默认）
  feishu-cli msg urgent om_xxx --user-id-type open_id --user-ids ou_xxx,ou_yyy

  # 发送电话加急
  feishu-cli msg urgent om_xxx --urgent-type phone --user-id-type user_id --user-ids u_123,u_456

  # 发送短信加急
  feishu-cli msg urgent om_xxx --urgent-type sms --user-id-type union_id --user-ids on_xxx,on_yyy`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		messageID := args[0]
		if strings.HasPrefix(messageID, "bm_") {
			return fmt.Errorf("不支持批量消息 ID（bm_xxx），请传入单条消息 ID（om_xxx）")
		}

		urgentType, _ := cmd.Flags().GetString("urgent-type")
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		userIDsStr, _ := cmd.Flags().GetString("user-ids")
		output, _ := cmd.Flags().GetString("output")

		if err := validateUrgentType(urgentType); err != nil {
			return err
		}
		if err := validateUrgentUserIDType(userIDType); err != nil {
			return err
		}

		userIDs := splitAndTrim(userIDsStr)
		if len(userIDs) == 0 {
			return fmt.Errorf("目标用户 ID 列表不能为空，请通过 --user-ids 传入至少一个用户 ID")
		}

		result, err := client.UrgentMessage(messageID, urgentType, userIDType, userIDs)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(map[string]any{
				"message_id":           messageID,
				"urgent_type":          urgentType,
				"user_id_type":         userIDType,
				"user_ids":             userIDs,
				"invalid_user_id_list": result.InvalidUserIDList,
			})
		}

		if len(result.InvalidUserIDList) > 0 {
			fmt.Printf("消息加急发送完成（非全部送达）\n")
			fmt.Printf("  无效用户 ID: %s\n", strings.Join(result.InvalidUserIDList, ","))
		} else {
			fmt.Printf("消息加急发送成功！\n")
		}
		fmt.Printf("  消息 ID: %s\n", messageID)
		fmt.Printf("  加急类型: %s\n", urgentType)

		return nil
	},
}

func validateUrgentType(urgentType string) error {
	return validateEnum(urgentType, "加急类型", []string{"app", "phone", "sms"})
}

func validateUrgentUserIDType(userIDType string) error {
	return validateEnum(userIDType, "用户 ID 类型", []string{"open_id", "user_id", "union_id"})
}

func init() {
	msgCmd.AddCommand(msgUrgentCmd)
	msgUrgentCmd.Flags().String("urgent-type", "app", "加急类型（app/phone/sms）")
	msgUrgentCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型（open_id/user_id/union_id）")
	msgUrgentCmd.Flags().String("user-ids", "", "目标用户 ID 列表（逗号分隔）")
	msgUrgentCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mustMarkFlagRequired(msgUrgentCmd, "user-ids")
}
