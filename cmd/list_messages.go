package cmd

import (
	"fmt"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var listMessagesCmd = &cobra.Command{
	Use:   "list",
	Short: "获取消息列表",
	Long: `获取指定会话中的消息列表。

参数:
  --container-id       会话 ID（必填）
  --container-id-type  会话 ID 类型（默认: chat）
  --start-time         起始时间戳（秒）
  --end-time           结束时间戳（秒）
  --sort-type          排序方式（ByCreateTimeAsc/ByCreateTimeDesc）
  --page-size          每页数量（默认: 20，最大: 50）
  --page-token         分页标记
  --output, -o         输出格式（json）

示例:
  # 获取群聊消息列表
  feishu-cli msg list --container-id oc_xxx

  # 获取指定时间范围内的消息
  feishu-cli msg list --container-id oc_xxx --start-time 1609459200 --end-time 1609545600

  # 分页获取
  feishu-cli msg list --container-id oc_xxx --page-size 10

  # JSON 格式输出
  feishu-cli msg list --container-id oc_xxx --output json

  # interactive 卡片返回 schema 2.0 JSON，并额外提取 card_texts（默认）
  feishu-cli msg list --container-id oc_xxx --card-content-type user`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserTokenWithFallback(cmd)

		containerID, _ := cmd.Flags().GetString("container-id")
		containerIDType, _ := cmd.Flags().GetString("container-id-type")
		startTime, _ := cmd.Flags().GetString("start-time")
		endTime, _ := cmd.Flags().GetString("end-time")
		sortType, _ := cmd.Flags().GetString("sort-type")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")

		cardContentType, err := resolveCardContentType(cmd)
		if err != nil {
			return err
		}

		opts := client.ListMessagesOptions{
			ContainerIDType: containerIDType,
			StartTime:       startTime,
			EndTime:         endTime,
			SortType:        sortType,
			PageSize:        pageSize,
			PageToken:       pageToken,
			CardContentType: cardContentType,
		}

		result, err := client.ListMessages(containerID, opts, token)

		// 降级判断：有 User Token 时，list API 失败或返回空结果都尝试 search+get
		needFallback := false
		if err != nil && token != "" {
			needFallback = true
		} else if err != nil {
			return err
		} else if token != "" && len(result.Items) == 0 && result.HasMore {
			needFallback = true
		}

		if needFallback {
			fmt.Fprintf(cmd.ErrOrStderr(), "[提示] bot 不在此群中，通过搜索方式获取消息...\n")
			fallbackResult, fallbackErr := listMessagesViaSearch(containerID, pageSize, pageToken, token, cardContentType)
			if fallbackErr != nil {
				if err != nil {
					return err
				}
				return fmt.Errorf("搜索降级失败: %w", fallbackErr)
			}
			result = fallbackResult
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			// 合并主消息 + merge_forward 子消息，统一解析 sender_names
			allMsgs := append([]*larkim.Message{}, result.Items...)
			for _, subs := range result.MergeForwardSubMessages {
				allMsgs = append(allMsgs, subs...)
			}
			senderNames := client.ResolveSenderNames(allMsgs, token)
			payload := map[string]any{
				"items":        result.Items,
				"page_token":   result.PageToken,
				"has_more":     result.HasMore,
				"sender_names": senderNames,
			}
			if cardTextMap := client.ExtractCardTextMap(result.Items); len(cardTextMap) > 0 {
				payload["card_texts"] = cardTextMap
			}
			if len(result.MergeForwardSubMessages) > 0 {
				payload["merge_forward_sub_messages"] = result.MergeForwardSubMessages
				var subMsgs []*larkim.Message
				for _, subs := range result.MergeForwardSubMessages {
					subMsgs = append(subMsgs, subs...)
				}
				if cardTextMap := client.ExtractCardTextMap(subMsgs); len(cardTextMap) > 0 {
					payload["merge_forward_card_texts"] = cardTextMap
				}
			}
			if err := printJSON(payload); err != nil {
				return err
			}
		} else {
			fmt.Printf("消息列表（共 %d 条）:\n", len(result.Items))
			for i, msg := range result.Items {
				msgID := ""
				msgType := ""
				createTime := ""
				senderID := ""
				if msg.MessageId != nil {
					msgID = *msg.MessageId
				}
				if msg.MsgType != nil {
					msgType = *msg.MsgType
				}
				if msg.CreateTime != nil {
					createTime = *msg.CreateTime
				}
				if msg.Sender != nil && msg.Sender.Id != nil {
					senderID = *msg.Sender.Id
				}
				fmt.Printf("\n[%d] 消息 ID: %s\n", i+1, msgID)
				fmt.Printf("    类型: %s\n", msgType)
				fmt.Printf("    发送者: %s\n", senderID)
				fmt.Printf("    创建时间: %s\n", createTime)
			}
			if result.HasMore {
				fmt.Printf("\n还有更多消息，使用 --page-token %s 获取下一页\n", result.PageToken)
			}
		}

		return nil
	},
}

func init() {
	msgCmd.AddCommand(listMessagesCmd)
	listMessagesCmd.Flags().String("container-id", "", "会话 ID")
	listMessagesCmd.Flags().String("container-id-type", "chat", "会话 ID 类型（chat）")
	listMessagesCmd.Flags().String("start-time", "", "起始时间戳（秒）")
	listMessagesCmd.Flags().String("end-time", "", "结束时间戳（秒）")
	listMessagesCmd.Flags().String("sort-type", "", "排序方式（ByCreateTimeAsc/ByCreateTimeDesc）")
	listMessagesCmd.Flags().Int("page-size", 20, "每页数量（最大 50）")
	listMessagesCmd.Flags().String("page-token", "", "分页标记")
	listMessagesCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	listMessagesCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	addCardContentTypeFlag(listMessagesCmd)
	mustMarkFlagRequired(listMessagesCmd, "container-id")
}
