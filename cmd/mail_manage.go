package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// mailManageMaxMessageIDs 单次批量操作邮件的数量上限（飞书 batch 端点约束）。
const mailManageMaxMessageIDs = 20

// parseMailManageMessageIDs 解析 --message-ids（逗号分隔），去空白、去空项，
// 并校验非空与数量上限（≤ mailManageMaxMessageIDs），供 message-modify / message-trash 复用。
func parseMailManageMessageIDs(raw string) ([]string, error) {
	ids := splitAndTrim(raw)
	if len(ids) == 0 {
		return nil, fmt.Errorf("--message-ids 至少提供一个邮件 ID（逗号分隔）")
	}
	if len(ids) > mailManageMaxMessageIDs {
		return nil, fmt.Errorf("--message-ids 单次最多 %d 个，当前 %d 个（请分批执行）", mailManageMaxMessageIDs, len(ids))
	}
	return ids, nil
}

// ==================== mail message-modify ====================

var mailMessageModifyCmd = &cobra.Command{
	Use:   "message-modify",
	Short: "批量修改邮件：添加/移除标签、移动到文件夹",
	Long: `批量给邮件添加/移除标签，或移动到指定文件夹。

对一批 message_id 一次性应用标签与文件夹变更，标签操作可逆（再执行一次反向操作即可还原）。

必填:
  --message-ids   邮件 message_id，逗号分隔，单次最多 20 个

至少指定一项操作:
  --add-label-ids      要添加的标签 ID，逗号分隔（系统标签如 FLAGGED/IMPORTANT/UNREAD 需大写）
  --remove-label-ids   要移除的标签 ID，逗号分隔
  --folder-id          目标文件夹 ID，把邮件移入该文件夹（系统文件夹如 INBOX/ARCHIVED；不支持 TRASH，删除请用 mail message-trash）

可选:
  --mailbox            邮箱地址（默认 me，即当前登录用户）
  --user-id-type       用户 ID 类型（open_id/user_id/union_id，一般无需指定）
  -o json              JSON 格式输出

权限:
  - User Access Token
  - mail:user_mailbox.message:modify

示例:
  feishu-cli mail message-modify --message-ids m1,m2 --add-label-ids FLAGGED
  feishu-cli mail message-modify --message-ids m1 --add-label-ids IMPORTANT --remove-label-ids UNREAD
  feishu-cli mail message-modify --message-ids m1,m2,m3 --folder-id ARCHIVED`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		mailbox, _ := cmd.Flags().GetString("mailbox")
		messageIDsRaw, _ := cmd.Flags().GetString("message-ids")
		addLabelsRaw, _ := cmd.Flags().GetString("add-label-ids")
		removeLabelsRaw, _ := cmd.Flags().GetString("remove-label-ids")
		folderID, _ := cmd.Flags().GetString("folder-id")
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		output, _ := cmd.Flags().GetString("output")

		messageIDs, err := parseMailManageMessageIDs(messageIDsRaw)
		if err != nil {
			return err
		}
		addLabels := splitAndTrim(addLabelsRaw)
		removeLabels := splitAndTrim(removeLabelsRaw)
		if len(addLabels) == 0 && len(removeLabels) == 0 && folderID == "" {
			return fmt.Errorf("至少指定一项操作：--add-label-ids / --remove-label-ids / --folder-id")
		}

		token, err := requireUserToken(cmd, "mail message-modify")
		if err != nil {
			return err
		}

		data, err := client.BatchModifyMailMessages(mailbox, messageIDs, addLabels, removeLabels, folderID, userIDType, token)
		if err != nil {
			return fmt.Errorf("批量修改邮件失败: %w", err)
		}

		if output == "json" {
			result := map[string]any{
				"message_ids":      messageIDs,
				"add_label_ids":    addLabels,
				"remove_label_ids": removeLabels,
				"folder_id":        folderID,
				"data":             json.RawMessage(data),
			}
			return printJSON(result)
		}
		fmt.Printf("已修改 %d 封邮件\n", len(messageIDs))
		if len(addLabels) > 0 {
			fmt.Printf("  添加标签: %v\n", addLabels)
		}
		if len(removeLabels) > 0 {
			fmt.Printf("  移除标签: %v\n", removeLabels)
		}
		if folderID != "" {
			fmt.Printf("  移动到文件夹: %s\n", folderID)
		}
		return nil
	},
}

// ==================== mail draft-send ====================

var mailDraftSendCmd = &cobra.Command{
	Use:   "draft-send",
	Short: "发送已存在的草稿（需 --confirm-send 确认）",
	Long: `发送一封已存在的草稿。

发送邮件不可撤销，因此默认不真正发送：不带 --confirm-send 时仅提示确认方式；
只有显式加 --confirm-send 才会调用发送接口。草稿可先用 mail draft-create 创建。

必填:
  --draft-id       草稿 ID

可选:
  --mailbox        邮箱地址（默认 me，即当前登录用户）
  --confirm-send   确认发送（不加则不会真正发送）
  -o json          JSON 格式输出

权限:
  - User Access Token
  - mail:user_mailbox.message:send

示例:
  feishu-cli mail draft-send --draft-id xxx                 # 仅提示，不发送
  feishu-cli mail draft-send --draft-id xxx --confirm-send  # 确认发送`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		mailbox, _ := cmd.Flags().GetString("mailbox")
		draftID, _ := cmd.Flags().GetString("draft-id")
		confirmSend, _ := cmd.Flags().GetBool("confirm-send")
		output, _ := cmd.Flags().GetString("output")

		if draftID == "" {
			return fmt.Errorf("--draft-id 必填")
		}

		// 发送不可撤销：未确认时短路，仅提示确认方式，不调用任何接口。
		if !confirmSend {
			result := map[string]any{
				"draft_id":  draftID,
				"confirmed": false,
				"tip":       "发送草稿不可撤销，请加 --confirm-send 确认发送。",
			}
			if output == "json" {
				return printJSON(result)
			}
			fmt.Printf("未发送：发送草稿是不可撤销操作。\n")
			fmt.Printf("确认发送请加 --confirm-send：\n")
			fmt.Printf("  feishu-cli mail draft-send --draft-id %s --confirm-send\n", draftID)
			return nil
		}

		token, err := requireUserToken(cmd, "mail draft-send")
		if err != nil {
			return err
		}

		data, err := client.SendMailDraft(mailbox, draftID, token)
		if err != nil {
			return fmt.Errorf("发送草稿失败: %w", err)
		}

		var parsed struct {
			MessageID string `json:"message_id"`
			ThreadID  string `json:"thread_id"`
		}
		_ = json.Unmarshal(data, &parsed)

		result := map[string]any{
			"draft_id":   draftID,
			"message_id": parsed.MessageID,
			"thread_id":  parsed.ThreadID,
			"confirmed":  true,
		}
		if output == "json" {
			return printJSON(result)
		}
		fmt.Printf("草稿发送成功!\n")
		fmt.Printf("  草稿 ID: %s\n", draftID)
		if parsed.MessageID != "" {
			fmt.Printf("  邮件 ID: %s\n", parsed.MessageID)
		}
		if parsed.ThreadID != "" {
			fmt.Printf("  线程 ID: %s\n", parsed.ThreadID)
		}
		return nil
	},
}

// ==================== mail message-trash ====================

var mailMessageTrashCmd = &cobra.Command{
	Use:   "message-trash",
	Short: "批量软删除邮件（移入废纸篓）",
	Long: `批量把邮件移入废纸篓（软删除，可在飞书邮箱废纸篓内恢复）。

必填:
  --message-ids   邮件 message_id，逗号分隔，单次最多 20 个

可选:
  --mailbox       邮箱地址（默认 me，即当前登录用户）
  --yes           跳过二次确认（不加则会交互式确认）
  -o json         JSON 格式输出

权限:
  - User Access Token
  - mail:user_mailbox.message:modify

示例:
  feishu-cli mail message-trash --message-ids m1,m2         # 交互式确认后删除
  feishu-cli mail message-trash --message-ids m1,m2 --yes   # 跳过确认直接删除`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		mailbox, _ := cmd.Flags().GetString("mailbox")
		messageIDsRaw, _ := cmd.Flags().GetString("message-ids")
		skipConfirm, _ := cmd.Flags().GetBool("yes")
		output, _ := cmd.Flags().GetString("output")

		messageIDs, err := parseMailManageMessageIDs(messageIDsRaw)
		if err != nil {
			return err
		}

		if !skipConfirm {
			prompt := fmt.Sprintf("将把 %d 封邮件移入废纸篓，确认?", len(messageIDs))
			if !confirmAction(prompt) {
				fmt.Println("已取消")
				return nil
			}
		}

		token, err := requireUserToken(cmd, "mail message-trash")
		if err != nil {
			return err
		}

		data, err := client.BatchTrashMailMessages(mailbox, messageIDs, token)
		if err != nil {
			return fmt.Errorf("批量软删除邮件失败: %w", err)
		}

		if output == "json" {
			result := map[string]any{
				"message_ids": messageIDs,
				"trashed":     true,
				"data":        json.RawMessage(data),
			}
			return printJSON(result)
		}
		fmt.Printf("已将 %d 封邮件移入废纸篓\n", len(messageIDs))
		return nil
	},
}

func init() {
	mailCmd.AddCommand(mailMessageModifyCmd)
	mailMessageModifyCmd.Flags().String("mailbox", "me", "邮箱地址（默认 me）")
	mailMessageModifyCmd.Flags().String("message-ids", "", "邮件 message_id，逗号分隔，最多 20 个（必填）")
	mailMessageModifyCmd.Flags().String("add-label-ids", "", "要添加的标签 ID，逗号分隔")
	mailMessageModifyCmd.Flags().String("remove-label-ids", "", "要移除的标签 ID，逗号分隔")
	mailMessageModifyCmd.Flags().String("folder-id", "", "目标文件夹 ID，把邮件移入该文件夹")
	mailMessageModifyCmd.Flags().String("user-id-type", "", "用户 ID 类型（open_id/user_id/union_id，一般无需指定）")
	mailMessageModifyCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mailMessageModifyCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(mailMessageModifyCmd, "message-ids")

	mailCmd.AddCommand(mailDraftSendCmd)
	mailDraftSendCmd.Flags().String("mailbox", "me", "邮箱地址（默认 me）")
	mailDraftSendCmd.Flags().String("draft-id", "", "草稿 ID（必填）")
	mailDraftSendCmd.Flags().Bool("confirm-send", false, "确认发送（不加则不会真正发送）")
	mailDraftSendCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mailDraftSendCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(mailDraftSendCmd, "draft-id")

	mailCmd.AddCommand(mailMessageTrashCmd)
	mailMessageTrashCmd.Flags().String("mailbox", "me", "邮箱地址（默认 me）")
	mailMessageTrashCmd.Flags().String("message-ids", "", "邮件 message_id，逗号分隔，最多 20 个（必填）")
	mailMessageTrashCmd.Flags().Bool("yes", false, "跳过二次确认")
	mailMessageTrashCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mailMessageTrashCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(mailMessageTrashCmd, "message-ids")
}
