package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var replyMsgCmd = &cobra.Command{
	Use:   "reply <message_id>",
	Short: "回复消息（支持话题与媒体）",
	Long: `回复指定的 om_xxx 消息。发送到既有话题时，应回复话题内任一 om_xxx 消息；
目标消息已经属于话题时，飞书会默认把回复放入同一话题。不要传 omt_xxx thread_id。

内容参数:
  --text, -t          简单文本
  --markdown          Markdown（自动包装为 post）
  --content, -c       消息内容 JSON
  --content-file      消息内容 JSON 文件
  --image             本地图片路径或 image_key
  --file, -f          本地文件路径或 file_key
  --audio             本地 Opus/Ogg Opus 路径或 file_key
  --video             本地 MP4 路径或 file_key（需同时指定 --video-cover）
  --video-cover       视频封面本地图片路径或 image_key
  --upload-images     上传 post/interactive 中引用的本地图片
  --attachment        post 附件区文件（file_key 或本地路径，可重复）
  （--text/--markdown/--content 支持 - 读 stdin、@path 读文件、@@ 转义）

回复参数:
  --reply-in-thread   在普通消息群中开启新话题；目标已在话题中时通常无需指定
  --idempotency-key   幂等键（≤50 字符）；相同键一小时内至多成功回复一条
  --dry-run           只预览请求（本地文件不上传），不发送
  --output, -o        输出格式（json）

示例:
  # 回复文本
  feishu-cli msg reply om_xxx --text "收到，谢谢！"

  # 回复本地图片（自动上传后提交 image_key）
  feishu-cli msg reply om_xxx --image /path/to/screenshot.png \
    --idempotency-key "reply-image-20260730"

  # 在普通消息群中开启一个新话题
  feishu-cli msg reply om_xxx --text "这里开个话题" --reply-in-thread

  # 回复卡片并上传其中的本地图片
  feishu-cli msg reply om_xxx --msg-type interactive \
    --content-file card.json --upload-images`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		messageID := strings.TrimSpace(args[0])
		if err := validateReplyMessageID(messageID); err != nil {
			return err
		}
		idempotencyKey, _ := cmd.Flags().GetString("idempotency-key")
		if err := validateIdempotencyKey(idempotencyKey); err != nil {
			return err
		}
		contentInput := readMessageContentInput(cmd)
		if err := contentInput.expandInputSources(cmd.ErrOrStderr()); err != nil {
			return err
		}
		if err := contentInput.validate(); err != nil {
			return clierr.Usage(err)
		}
		replyInThread, _ := cmd.Flags().GetBool("reply-in-thread")
		token := resolveOptionalUserToken(cmd)

		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			contentInput.dryRun = true
			msgType, msgContent, err := contentInput.resolve()
			if err != nil {
				return err
			}
			base := map[string]any{}
			if replyInThread {
				base["reply_in_thread"] = true
			}
			return printMessageDryRun(cmd, "回复消息（预览，不会真正发送）", "/open-apis/im/v1/messages/"+messageID+"/reply",
				nil, messageDryRunBody(base, msgType, msgContent, idempotencyKey), token)
		}

		if err := config.Validate(); err != nil {
			return err
		}

		msgType, msgContent, err := contentInput.resolve()
		if err != nil {
			return err
		}
		sent, err := client.ReplyMessageDetailed(
			messageID,
			msgType,
			msgContent,
			replyInThread,
			token,
			idempotencyKey,
		)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			out := map[string]string{
				"message_id":        sent.MessageID,
				"parent_message_id": messageID,
			}
			if sent.ChatID != "" {
				out["chat_id"] = sent.ChatID
			}
			if sent.CreateTime != "" {
				out["create_time"] = sent.CreateTime
			}
			if sent.ThreadID != "" {
				out["thread_id"] = sent.ThreadID
			}
			return printJSON(out)
		}
		fmt.Printf("消息回复成功！\n")
		fmt.Printf("  原消息 ID: %s\n", messageID)
		fmt.Printf("  新消息 ID: %s\n", sent.MessageID)
		return nil
	},
}

func validateReplyMessageID(messageID string) error {
	if strings.HasPrefix(messageID, "omt_") {
		return fmt.Errorf("无效的 message_id %q：omt_ 是 thread_id；请传入话题内的 om_xxx 消息 ID", messageID)
	}
	if !strings.HasPrefix(messageID, "om_") {
		return fmt.Errorf("无效的 message_id %q：回复消息需要 om_xxx 消息 ID", messageID)
	}
	return nil
}

func init() {
	msgCmd.AddCommand(replyMsgCmd)
	addMessageContentFlags(replyMsgCmd)
	replyMsgCmd.Flags().Bool("reply-in-thread", false, "以话题形式回复（reply_in_thread=true）")
	replyMsgCmd.Flags().String("idempotency-key", "", "幂等键（≤50 字符），相同键一小时内至多成功回复一条")
	replyMsgCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	replyMsgCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	replyMsgCmd.Flags().Bool("dry-run", false, "只预览将要发送的请求（本地文件不上传，用占位 key），不真正发送")
}
