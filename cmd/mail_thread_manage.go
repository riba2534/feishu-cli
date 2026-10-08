package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// 线程整理（对齐官方 mail +thread-modify / +thread-trash）：按 20 个线程一批顺序调用 threads/batch_modify、batch_trash。

type mailThreadFailure struct {
	ThreadID string `json:"thread_id"`
	Reason   string `json:"reason"`
}

func toMailThreadFailures(in []mailManageFailure) []mailThreadFailure {
	out := make([]mailThreadFailure, 0, len(in))
	for _, f := range in {
		out = append(out, mailThreadFailure{ThreadID: f.MessageID, Reason: f.Reason})
	}
	return out
}

func parseMailThreadIDs(raw string) ([]string, error) {
	ids, err := parseMailManageMessageIDs(raw)
	if err != nil {
		return nil, clierr.Usagef("--thread-ids 至少提供一个线程 ID（逗号分隔）")
	}
	return ids, nil
}

var mailThreadModifyCmd = &cobra.Command{
	Use:   "thread-modify",
	Short: "批量修改邮件线程：添加/移除标签、移动到文件夹",
	Long: `批量给整个邮件线程（会话）添加/移除标签，或把线程移动到指定文件夹。

线程 ID 可从 mail triage -o json 的 thread_id、mail message 的 thread_id 获取。
超过 20 个线程自动按 20 个一批顺序执行；任一批失败时输出明细并以非 0 退出。

必填:
  --thread-ids         线程 ID，逗号分隔（去重）

至少指定一项操作:
  --add-label-ids      要添加的标签 ID（系统标签 FLAGGED/IMPORTANT/UNREAD/OTHER 大小写不敏感）
  --remove-label-ids   要移除的标签 ID（不能与 --add-label-ids 重叠）
  --folder-id          目标文件夹 ID（inbox/archive 等系统文件夹自动规范化；不支持 TRASH，删除请用 mail thread-trash）

可选:
  --mailbox            邮箱地址（默认 me）
  --dry-run            只打印将要发送的批次请求，不实际调用
  -o json              JSON 格式输出

权限:
  - User Access Token
  - mail:user_mailbox.message:modify

示例:
  feishu-cli mail thread-modify --thread-ids t1,t2 --add-label-ids flagged
  feishu-cli mail thread-modify --thread-ids t1 --remove-label-ids UNREAD --folder-id archive`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		mailbox, _ := cmd.Flags().GetString("mailbox")
		raw, _ := cmd.Flags().GetString("thread-ids")
		addRaw, _ := cmd.Flags().GetString("add-label-ids")
		removeRaw, _ := cmd.Flags().GetString("remove-label-ids")
		folderRaw, _ := cmd.Flags().GetString("folder-id")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		output, _ := cmd.Flags().GetString("output")

		threadIDs, err := parseMailThreadIDs(raw)
		if err != nil {
			return err
		}
		addLabels, err := normalizeMailManageLabels(addRaw, "--add-label-ids")
		if err != nil {
			return err
		}
		removeLabels, err := normalizeMailManageLabels(removeRaw, "--remove-label-ids")
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
		folderID, err := normalizeMailManageFolder(folderRaw)
		if err != nil {
			return err
		}
		if len(addLabels) == 0 && len(removeLabels) == 0 && folderID == "" {
			return clierr.Usagef("至少指定一项操作：--add-label-ids / --remove-label-ids / --folder-id")
		}
		bodyFor := func(batch []string) map[string]any {
			body := map[string]any{"thread_ids": batch}
			if len(addLabels) > 0 {
				body["add_label_ids"] = addLabels
			}
			if len(removeLabels) > 0 {
				body["remove_label_ids"] = removeLabels
			}
			if folderID != "" {
				body["add_folder"] = folderID
			}
			return body
		}

		if dryRun {
			var reqs []map[string]any
			for _, batch := range chunkMailIDs(threadIDs, mailManageMaxMessageIDs) {
				reqs = append(reqs, map[string]any{"method": "POST", "path": client.MailThreadsBatchPath(mailbox, "batch_modify"), "body": bodyFor(batch)})
			}
			return printJSON(map[string]any{"dry_run": true, "batch_size": mailManageMaxMessageIDs, "requests": reqs})
		}

		token, err := requireUserToken(cmd, "mail thread-modify")
		if err != nil {
			return err
		}
		success, failed, _ := runMailManageBatches(threadIDs, func(batch []string) (json.RawMessage, error) {
			return client.BatchModifyMailThreads(mailbox, bodyFor(batch), token)
		})
		if output == "json" {
			if err := printJSON(map[string]any{
				"thread_ids":         threadIDs,
				"add_label_ids":      addLabels,
				"remove_label_ids":   removeLabels,
				"folder_id":          folderID,
				"success_thread_ids": success,
				"failed_thread_ids":  toMailThreadFailures(failed),
			}); err != nil {
				return err
			}
		} else {
			fmt.Printf("已修改 %d 个线程", len(success))
			if len(failed) > 0 {
				fmt.Printf("，失败 %d 个", len(failed))
			}
			fmt.Println()
		}
		return mailManageBatchError("批量修改线程", success, failed)
	},
}

var mailThreadTrashCmd = &cobra.Command{
	Use:   "thread-trash",
	Short: "批量软删除邮件线程（整个会话移入废纸篓）",
	Long: `批量把整个邮件线程移入废纸篓（软删除，可在飞书邮箱废纸篓内恢复）。

超过 20 个线程自动按 20 个一批顺序执行；任一批失败时输出明细并以非 0 退出。

必填:
  --thread-ids    线程 ID，逗号分隔（去重）

可选:
  --mailbox       邮箱地址（默认 me）
  --dry-run       只打印将要发送的批次请求，不实际调用（优先于确认）
  --yes           （全局）跳过二次确认，非交互环境必须显式传入
  -o json         JSON 格式输出

权限:
  - User Access Token
  - mail:user_mailbox.message:modify

示例:
  feishu-cli mail thread-trash --thread-ids t1,t2 --dry-run
  feishu-cli mail thread-trash --thread-ids t1,t2 --yes`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		mailbox, _ := cmd.Flags().GetString("mailbox")
		raw, _ := cmd.Flags().GetString("thread-ids")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		output, _ := cmd.Flags().GetString("output")

		threadIDs, err := parseMailThreadIDs(raw)
		if err != nil {
			return err
		}
		if dryRun {
			var reqs []map[string]any
			for _, batch := range chunkMailIDs(threadIDs, mailManageMaxMessageIDs) {
				reqs = append(reqs, map[string]any{"method": "POST", "path": client.MailThreadsBatchPath(mailbox, "batch_trash"), "body": map[string]any{"thread_ids": batch}})
			}
			return printJSON(map[string]any{"dry_run": true, "batch_size": mailManageMaxMessageIDs, "requests": reqs})
		}
		if err := confirmDangerousAction(cmd, fmt.Sprintf("将把 %d 个邮件线程（含其中全部邮件）移入废纸篓，确认?", len(threadIDs))); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "mail thread-trash")
		if err != nil {
			return err
		}
		success, failed, _ := runMailManageBatches(threadIDs, func(batch []string) (json.RawMessage, error) {
			return client.BatchTrashMailThreads(mailbox, batch, token)
		})
		if output == "json" {
			if err := printJSON(map[string]any{
				"thread_ids":         threadIDs,
				"success_thread_ids": success,
				"failed_thread_ids":  toMailThreadFailures(failed),
			}); err != nil {
				return err
			}
		} else {
			fmt.Printf("已将 %d 个线程移入废纸篓", len(success))
			if len(failed) > 0 {
				fmt.Printf("，失败 %d 个", len(failed))
			}
			fmt.Println()
		}
		return mailManageBatchError("批量软删除线程", success, failed)
	},
}

func init() {
	mailCmd.AddCommand(mailThreadModifyCmd)
	mailThreadModifyCmd.Flags().String("mailbox", "me", "邮箱地址（默认 me）")
	mailThreadModifyCmd.Flags().String("thread-ids", "", "线程 ID，逗号分隔（必填）")
	mailThreadModifyCmd.Flags().String("add-label-ids", "", "要添加的标签 ID，逗号分隔")
	mailThreadModifyCmd.Flags().String("remove-label-ids", "", "要移除的标签 ID，逗号分隔")
	mailThreadModifyCmd.Flags().String("folder-id", "", "目标文件夹 ID")
	mailThreadModifyCmd.Flags().Bool("dry-run", false, "只打印请求，不实际调用")
	mailThreadModifyCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mailThreadModifyCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(mailThreadModifyCmd, "thread-ids")

	mailCmd.AddCommand(mailThreadTrashCmd)
	mailThreadTrashCmd.Flags().String("mailbox", "me", "邮箱地址（默认 me）")
	mailThreadTrashCmd.Flags().String("thread-ids", "", "线程 ID，逗号分隔（必填）")
	mailThreadTrashCmd.Flags().Bool("dry-run", false, "只打印请求，不实际调用")
	mailThreadTrashCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mailThreadTrashCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(mailThreadTrashCmd, "thread-ids")
}
