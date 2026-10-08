package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// mailManageMaxMessageIDs 飞书 batch_modify / batch_trash 单次请求的邮件数上限；超过时 CLI 自动按此分批。
const mailManageMaxMessageIDs = 20

// parseMailManageMessageIDs 解析 --message-ids（逗号分隔），去空白、去空项、去重保序；
// 不再限制总数（超过 20 封由调用方按 mailManageMaxMessageIDs 自动分批，对齐官方）。
func parseMailManageMessageIDs(raw string) ([]string, error) {
	ids := splitAndTrim(raw)
	if len(ids) == 0 {
		return nil, clierr.Usagef("--message-ids 至少提供一个邮件 ID（逗号分隔）")
	}
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, nil
}

// mailManageSystemLabels 系统标签（大小写不敏感输入 → 服务端要求的大写 ID）。
var mailManageSystemLabels = map[string]string{
	"UNREAD":               "UNREAD",
	"IMPORTANT":            "IMPORTANT",
	"OTHER":                "OTHER",
	"FLAGGED":              "FLAGGED",
	"READ_RECEIPT_REQUEST": "READ_RECEIPT_REQUEST",
}

// mailManageSystemFolders 可作为移动目标的系统文件夹（TRASH 不允许，删除请用 message-trash）。
var mailManageSystemFolders = map[string]string{
	"INBOX":    "INBOX",
	"SENT":     "SENT",
	"SPAM":     "SPAM",
	"ARCHIVE":  "ARCHIVED",
	"ARCHIVED": "ARCHIVED",
}

// normalizeMailManageLabels 系统标签规范化为大写，自定义标签 ID 原样；去重，单次最多 20 个。
func normalizeMailManageLabels(raw, flagName string) ([]string, error) {
	out := []string{} // 与旧版输出保持一致：未指定时 JSON 为 []，而不是 null
	seen := map[string]bool{}
	for _, id := range splitAndTrim(raw) {
		if sys, ok := mailManageSystemLabels[strings.ToUpper(id)]; ok {
			id = sys
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	if len(out) > 20 {
		return nil, clierr.Usagef("%s 单次最多 20 个标签（当前 %d）", flagName, len(out))
	}
	return out, nil
}

// normalizeMailManageFolder 系统文件夹规范化（inbox/archive → INBOX/ARCHIVED），拒绝 TRASH。
func normalizeMailManageFolder(raw string) (string, error) {
	folder := strings.TrimSpace(raw)
	if folder == "" {
		return "", nil
	}
	if strings.EqualFold(folder, "TRASH") {
		return "", clierr.Usagef("--folder-id 不支持 TRASH；删除邮件请用 `feishu-cli mail message-trash`")
	}
	if sys, ok := mailManageSystemFolders[strings.ToUpper(folder)]; ok {
		return sys, nil
	}
	return folder, nil
}

// chunkMailIDs 按 size 切分 ID 列表。
func chunkMailIDs(ids []string, size int) [][]string {
	var out [][]string
	for start := 0; start < len(ids); start += size {
		end := start + size
		if end > len(ids) {
			end = len(ids)
		}
		out = append(out, ids[start:end])
	}
	return out
}

// mailManageFailure 失败批次中的单封邮件。
type mailManageFailure struct {
	MessageID string `json:"message_id"`
	Reason    string `json:"reason"`
}

// runMailManageBatches 按 20 封一批顺序执行 op，汇总成功/失败的 ID；返回最后一次成功响应。
func runMailManageBatches(ids []string, op func(batch []string) (json.RawMessage, error)) (success []string, failed []mailManageFailure, last json.RawMessage) {
	failed = []mailManageFailure{}
	for _, batch := range chunkMailIDs(ids, mailManageMaxMessageIDs) {
		data, err := op(batch)
		if err != nil {
			for _, id := range batch {
				failed = append(failed, mailManageFailure{MessageID: id, Reason: err.Error()})
			}
			continue
		}
		success = append(success, batch...)
		last = data
	}
	if success == nil {
		success = []string{}
	}
	return success, failed, last
}

// mailManageBatchError 存在失败批次时返回错误（摘要已输出），保证脚本能感知部分失败。
func mailManageBatchError(action string, success []string, failed []mailManageFailure) error {
	if len(failed) == 0 {
		return nil
	}
	if len(success) == 0 {
		return fmt.Errorf("%s失败（全部 %d 封）: %s", action, len(failed), failed[0].Reason)
	}
	return fmt.Errorf("%s部分失败：成功 %d 封，失败 %d 封（失败原因: %s）", action, len(success), len(failed), failed[0].Reason)
}

// ==================== mail message-modify ====================

var mailMessageModifyCmd = &cobra.Command{
	Use:   "message-modify",
	Short: "批量修改邮件：添加/移除标签、移动到文件夹",
	Long: `批量给邮件添加/移除标签，或移动到指定文件夹。

对一批 message_id 一次性应用标签与文件夹变更，标签操作可逆（再执行一次反向操作即可还原）。

必填:
  --message-ids   邮件 message_id，逗号分隔（去重；超过 20 封自动按 20 封一批顺序执行）

至少指定一项操作:
  --add-label-ids      要添加的标签 ID，逗号分隔（系统标签 FLAGGED/IMPORTANT/UNREAD/OTHER 大小写不敏感，自动转大写）
  --remove-label-ids   要移除的标签 ID，逗号分隔（同一标签不能同时添加和移除）
  --folder-id          目标文件夹 ID，把邮件移入该文件夹（系统文件夹 inbox/archive 等自动规范化；不支持 TRASH，删除请用 mail message-trash）

部分批次失败时输出成功/失败明细（JSON: success_message_ids / failed_message_ids）并以非 0 退出。

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
		addLabels, err := normalizeMailManageLabels(addLabelsRaw, "--add-label-ids")
		if err != nil {
			return err
		}
		removeLabels, err := normalizeMailManageLabels(removeLabelsRaw, "--remove-label-ids")
		if err != nil {
			return err
		}
		for _, a := range addLabels {
			for _, r := range removeLabels {
				if a == r {
					return clierr.Usagef("标签 %s 不能同时添加和移除", a)
				}
			}
		}
		folderID, err = normalizeMailManageFolder(folderID)
		if err != nil {
			return err
		}
		if len(addLabels) == 0 && len(removeLabels) == 0 && folderID == "" {
			return clierr.Usagef("至少指定一项操作：--add-label-ids / --remove-label-ids / --folder-id")
		}

		token, err := requireUserToken(cmd, "mail message-modify")
		if err != nil {
			return err
		}

		success, failed, data := runMailManageBatches(messageIDs, func(batch []string) (json.RawMessage, error) {
			return client.BatchModifyMailMessages(mailbox, batch, addLabels, removeLabels, folderID, userIDType, token)
		})

		if output == "json" {
			result := map[string]any{
				"message_ids":         messageIDs,
				"add_label_ids":       addLabels,
				"remove_label_ids":    removeLabels,
				"folder_id":           folderID,
				"data":                data,
				"success_message_ids": success,
				"failed_message_ids":  failed,
			}
			if err := printJSON(result); err != nil {
				return err
			}
			return mailManageBatchError("批量修改邮件", success, failed)
		}
		fmt.Printf("已修改 %d 封邮件", len(success))
		if len(failed) > 0 {
			fmt.Printf("，失败 %d 封", len(failed))
		}
		fmt.Println()
		if len(addLabels) > 0 {
			fmt.Printf("  添加标签: %v\n", addLabels)
		}
		if len(removeLabels) > 0 {
			fmt.Printf("  移除标签: %v\n", removeLabels)
		}
		if folderID != "" {
			fmt.Printf("  移动到文件夹: %s\n", folderID)
		}
		return mailManageBatchError("批量修改邮件", success, failed)
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
  --message-ids   邮件 message_id，逗号分隔（去重；超过 20 封自动分批）

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
			if err := confirmDangerousAction(cmd, prompt); err != nil {
				return err
			}
		}

		token, err := requireUserToken(cmd, "mail message-trash")
		if err != nil {
			return err
		}

		success, failed, data := runMailManageBatches(messageIDs, func(batch []string) (json.RawMessage, error) {
			return client.BatchTrashMailMessages(mailbox, batch, token)
		})

		if output == "json" {
			result := map[string]any{
				"message_ids":         messageIDs,
				"trashed":             len(failed) == 0,
				"data":                data,
				"success_message_ids": success,
				"failed_message_ids":  failed,
			}
			if err := printJSON(result); err != nil {
				return err
			}
			return mailManageBatchError("批量软删除邮件", success, failed)
		}
		fmt.Printf("已将 %d 封邮件移入废纸篓", len(success))
		if len(failed) > 0 {
			fmt.Printf("，失败 %d 封", len(failed))
		}
		fmt.Println()
		return mailManageBatchError("批量软删除邮件", success, failed)
	},
}

func init() {
	mailCmd.AddCommand(mailMessageModifyCmd)
	mailMessageModifyCmd.Flags().String("mailbox", "me", "邮箱地址（默认 me）")
	mailMessageModifyCmd.Flags().String("message-ids", "", "邮件 message_id，逗号分隔，超过 20 个自动分批（必填）")
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
	mailMessageTrashCmd.Flags().String("message-ids", "", "邮件 message_id，逗号分隔，超过 20 个自动分批（必填）")
	mailMessageTrashCmd.Flags().Bool("yes", false, "跳过二次确认")
	mailMessageTrashCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mailMessageTrashCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(mailMessageTrashCmd, "message-ids")
}
