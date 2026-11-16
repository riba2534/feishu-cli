package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

var chatCreateSyncCmd = &cobra.Command{
	Use:   "create-sync",
	Short: "从双人私聊原生建群并同步所选消息（实验性）",
	Long: `以网页登录用户身份，从双人私聊创建私有群并原生同步消息。

这不是转发或合并转发。新群包含私聊双方，群主是网页登录用户。
使用未公开的网页端 PutChatRequest（fromChatId + initMessageIds），不支持 OAuth Token。
需要已有网页 session JSON（默认 ~/.larkx/credentials.json），本命令不会登录或更新凭据。
仅接受网页端数字 ID，不接受 oc_/om_/ou_。目前仅支持普通私有群。

--message-ids 与 --last-messages 必须且只能指定一个，最多同步 50 条。
--last-messages 从最近 100 个位置中选取非系统、非空消息；数量不足时不建群。
--dry-run 只预览参数，不读取登录态、不联网。
--preview 只读核对实际双方和消息，不建群，输出 JSON。
实际执行前展示同步计划并确认；非交互环境需 --yes。
--idempotency-key 必填；同键重复执行只核验已有群，不重复建群。
同键的参数、身份或消息变化时拒绝执行；最近消息发生变化时也不自动换键重建。
网络结果未知或进程异常退出时拒绝自动重建，需先检查群列表与回执。
机器人请建群后使用 chat member add --member-id-type app_id 另行加入。

示例:
  feishu-cli chat create-sync --name "消息同步测试" --from-chat-id 123456789 \
    --last-messages 3 --idempotency-key sync-example --dry-run
  feishu-cli chat create-sync --name "消息同步测试" --from-chat-id 123456789 \
    --message-ids 111111111,222222222 --idempotency-key sync-example --preview`,
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		fromChatID, _ := cmd.Flags().GetString("from-chat-id")
		messageIDs, _ := cmd.Flags().GetString("message-ids")
		lastMessages, _ := cmd.Flags().GetInt("last-messages")
		sessionFile, _ := cmd.Flags().GetString("web-session-file")
		key, _ := cmd.Flags().GetString("idempotency-key")
		options := client.ChatSyncOptions{Name: name, FromChatID: fromChatID, MessageIDs: splitAndTrim(messageIDs),
			LastMessages: lastMessages, SessionFile: sessionFile, IdempotencyKey: key}
		if err := client.ValidateChatSyncOptions(options); err != nil {
			return err
		}
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		if dryRun {
			return printJSON(map[string]any{"dry_run": true, "identity": "web_user", "operation": "native_create_with_history", "options": options})
		}
		plan, err := client.PrepareChatSync(options)
		if err != nil {
			return err
		}
		preview, _ := cmd.Flags().GetBool("preview")
		if preview {
			return printJSON(plan)
		}
		encoded, _ := json.MarshalIndent(plan, "", "  ")
		fmt.Fprintln(cmd.ErrOrStderr(), string(encoded))
		if err := confirmDangerousAction(cmd, "确认从私聊原生建群并向群成员同步以上消息？"); err != nil {
			return err
		}
		result, err := client.CreateChatWithHistory(options, plan)
		if result != nil {
			if outputErr := printJSON(result); outputErr != nil {
				return outputErr
			}
		}
		return err
	},
}

func init() {
	home, _ := os.UserHomeDir()
	chatCmd.AddCommand(chatCreateSyncCmd)
	chatCreateSyncCmd.Flags().String("name", "", "群名称，最多 60 字")
	chatCreateSyncCmd.Flags().String("from-chat-id", "", "来源私聊的网页端数字会话 ID")
	chatCreateSyncCmd.Flags().String("message-ids", "", "网页端数字消息 ID（逗号分隔，最多 50）")
	chatCreateSyncCmd.Flags().Int("last-messages", 0, "同步最近 N 条非系统消息（1 到 50）")
	chatCreateSyncCmd.Flags().String("web-session-file", filepath.Join(home, ".larkx", "credentials.json"), "已有网页 session JSON 文件（不接受 OAuth Token）")
	chatCreateSyncCmd.Flags().String("idempotency-key", "", "固定请求键，同一操作重试必须复用（最多 50 字）")
	chatCreateSyncCmd.Flags().Bool("dry-run", false, "离线预览参数，不读取凭据、不联网")
	chatCreateSyncCmd.Flags().Bool("preview", false, "只读核对实际成员与同步消息，不建群")
	mustMarkFlagRequired(chatCreateSyncCmd, "name")
	mustMarkFlagRequired(chatCreateSyncCmd, "from-chat-id")
	mustMarkFlagRequired(chatCreateSyncCmd, "idempotency-key")
}
