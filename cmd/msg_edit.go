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

var msgEditCmd = &cobra.Command{
	Use:   "edit <message_id>",
	Short: "编辑已发送的文本 / 富文本消息（仅 Bot）",
	Long: `编辑 Bot 已发送的 text 或 post 消息（PUT /open-apis/im/v1/messages/:message_id）。

身份:
  仅 Bot（应用）身份：编辑接口不接受 User Token，只能编辑本应用 Bot 发送的消息。
  卡片消息（interactive）请用卡片更新接口，不在本命令范围内。

内容参数（三选一，可与附件参数组合）:
  --text            纯文本（编辑 text 消息）
  --markdown        Markdown（自动包装为 post，并做样式归一）
  --content         消息内容 JSON（配合 --msg-type text|post）
  以上均支持 - 读 stdin、@path 读文件、@@ 转义字面 @。

附件区（仅 post）:
  --set-attachments    用这些 file_key（或本地文件路径，自动上传）覆盖 post 附件区，可重复
  --clear-attachments  清空 post 附件区

其他:
  --dry-run         只预览请求，不真正编辑
  --output, -o      输出格式（json）：{message_id, chat_id, update_time}

注意:
  - 编辑不能改变消息类型（text 只能改成 text，post 只能改成 post）；
  - 飞书对可编辑次数与时间窗口有限制，超限时返回业务错误码。

示例:
  feishu-cli msg edit om_xxx --text "更正：会议改到 15:00"
  feishu-cli msg edit om_xxx --markdown @notice.md
  feishu-cli msg edit om_xxx --markdown "**最终版**" --set-attachments file_xxx
  feishu-cli msg edit om_xxx --msg-type post --content '{"zh_cn":{"content":[[{"tag":"md","text":"hi"}]]}}' --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		messageID := strings.TrimSpace(args[0])
		if err := validateReplyMessageID(messageID); err != nil {
			return clierr.Usage(err)
		}

		input := messageContentInput{errOut: cmd.ErrOrStderr()}
		input.msgType, _ = cmd.Flags().GetString("msg-type")
		input.msgTypeChanged = cmd.Flags().Changed("msg-type")
		input.text, _ = cmd.Flags().GetString("text")
		input.markdown, _ = cmd.Flags().GetString("markdown")
		input.content, _ = cmd.Flags().GetString("content")
		input.uploadImages, _ = cmd.Flags().GetBool("upload-images")
		setAttachments, _ := cmd.Flags().GetStringSlice("set-attachments")
		clearAttachments, _ := cmd.Flags().GetBool("clear-attachments")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		if err := input.expandInputSources(cmd.ErrOrStderr()); err != nil {
			return err
		}
		if err := validateEditInput(&input, setAttachments, clearAttachments); err != nil {
			return err
		}
		input.attachments = setAttachments
		input.dryRun = dryRun

		if !dryRun {
			if err := config.Validate(); err != nil {
				return err
			}
		}
		msgType, content, err := resolveEditContent(&input, setAttachments, clearAttachments)
		if err != nil {
			return err
		}

		if dryRun {
			return printDryRunPlan(cmd, "编辑消息（预览，不会真正编辑）", map[string]any{"identity": "bot", "message_id": messageID}, []dryRunStep{{
				Method: "PUT",
				URL:    "/open-apis/im/v1/messages/" + messageID,
				Body:   map[string]any{"msg_type": msgType, "content": content},
			}})
		}

		edited, err := client.EditMessage(messageID, msgType, content)
		if err != nil {
			return err
		}
		if output, _ := cmd.Flags().GetString("output"); output == "json" {
			return printJSON(edited)
		}
		fmt.Printf("消息编辑成功！\n")
		fmt.Printf("  消息 ID: %s\n", edited.MessageID)
		return nil
	},
}

// validateEditInput 校验编辑参数：内容参数互斥；msg-type 只能是 text/post；附件参数只用于 post。
func validateEditInput(input *messageContentInput, setAttachments []string, clearAttachments bool) error {
	if input.msgType != "text" && input.msgType != "post" {
		return clierr.Usagef("msg edit 只支持 --msg-type text|post，得到 %q（卡片请使用卡片更新接口）", input.msgType)
	}
	var specified []string
	for _, f := range []struct{ name, value string }{
		{"--text", input.text}, {"--markdown", input.markdown}, {"--content", input.content},
	} {
		if f.value != "" {
			specified = append(specified, f.name)
		}
	}
	if len(specified) > 1 {
		return clierr.Usagef("以下内容参数互斥，只能指定其中一个: %s", strings.Join(specified, ", "))
	}
	if len(specified) == 0 && len(setAttachments) == 0 && !clearAttachments {
		return clierr.Usagef("必须指定 --text、--markdown、--content、--set-attachments 或 --clear-attachments 之一")
	}
	if inferred := input.inferredMessageType(); input.msgTypeChanged && inferred != "" && inferred != input.msgType {
		return clierr.Usagef("--msg-type %q 与内容参数推断出的消息类型 %q 冲突", input.msgType, inferred)
	}
	if input.content != "" && !json.Valid([]byte(input.content)) {
		return clierr.Usagef("--content 必须是有效 JSON")
	}
	if clearAttachments && len(setAttachments) > 0 {
		return clierr.Usagef("--clear-attachments 不能与 --set-attachments 同时使用")
	}
	if len(setAttachments) > 0 || clearAttachments {
		if input.text != "" {
			return clierr.Usagef("附件区只属于 post 消息，不能与 --text 同时使用；正文请改用 --markdown")
		}
		if input.content != "" && input.msgType != "post" {
			return clierr.Usagef("附件区只属于 post 消息：配合 --content 时需指定 --msg-type post")
		}
		if input.content != "" && postContentHasFiles(input.content) {
			return clierr.Usagef("--content 已包含 files 附件区，不能再与 --set-attachments/--clear-attachments 同时使用")
		}
		for _, v := range setAttachments {
			v = strings.TrimSpace(v)
			if v == "" {
				return clierr.Usagef("--set-attachments 的取值不能为空")
			}
			if !isIMFileKey(v) {
				if err := validateFileInput("--set-attachments", v, "附件"); err != nil {
					return clierr.Usage(err)
				}
			}
		}
	}
	return nil
}

// resolveEditContent 生成编辑请求的 msg_type 与 content。
func resolveEditContent(input *messageContentInput, setAttachments []string, clearAttachments bool) (string, string, error) {
	msgType := input.msgType
	var content string
	switch {
	case input.markdown != "":
		msgType = "post"
		md, err := input.prepareMarkdown(input.markdown)
		if err != nil {
			return "", "", err
		}
		content = createMarkdownPostContent(md)
	case input.text != "":
		msgType = "text"
		content = client.CreateTextMessageContent(client.NormalizeAtMentions(input.text))
	case input.content != "":
		content = client.NormalizeAtMentionsInJSON(input.content)
	}

	if clearAttachments || len(setAttachments) > 0 {
		msgType = "post"
		if content == "" {
			content = `{"zh_cn":{"content":[]}}`
		}
		var keys []string
		if len(setAttachments) > 0 {
			var err error
			if keys, err = input.resolveAttachmentKeys(); err != nil {
				return "", "", err
			}
		}
		merged, err := setPostAttachments(content, keys)
		if err != nil {
			return "", "", fmt.Errorf("设置 post 附件区失败: %w", err)
		}
		content = merged
	}
	return msgType, content, nil
}

func init() {
	msgCmd.AddCommand(msgEditCmd)
	msgEditCmd.Flags().String("msg-type", "text", "消息类型（text/post）；--text/--markdown 时自动推断")
	msgEditCmd.Flags().StringP("text", "t", "", "纯文本内容（支持 - / @file）")
	msgEditCmd.Flags().String("markdown", "", "Markdown 内容，自动包装为 post（支持 - / @file）")
	msgEditCmd.Flags().StringP("content", "c", "", "消息内容 JSON（支持 - / @file）")
	msgEditCmd.Flags().Bool("upload-images", false, "自动上传 --markdown 中引用的本地图片")
	msgEditCmd.Flags().StringSlice("set-attachments", nil, "覆盖 post 附件区：file_key 或本地文件路径（可重复/逗号分隔）")
	msgEditCmd.Flags().Bool("clear-attachments", false, "清空 post 附件区")
	msgEditCmd.Flags().Bool("dry-run", false, "只预览请求，不真正编辑")
	msgEditCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
