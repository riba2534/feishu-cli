package cmd

import (
	"fmt"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var getMessageHistoryCmd = &cobra.Command{
	Use:   "history",
	Short: "获取会话历史消息（群聊 / 私聊）",
	Long: `获取飞书会话中的历史消息。三种入口任选其一：

  --container-id + --container-id-type   传统方式（群聊 oc_xxx / 话题 omt_xxx）
  --user-id                              对方 open_id，自动反查 P2P chat_id
  --user-email                           对方邮箱，自动搜用户 + 反查 P2P chat_id

通用参数:
  --start-time          起始时间（秒级时间戳）
  --end-time            结束时间（秒级时间戳）
  --sort-type           排序方式 (ByCreateTimeAsc/ByCreateTimeDesc)，默认 ByCreateTimeDesc
  --page-size           分页大小 (1-50)，默认 50
  --page-token          分页标记
  --output, -o          输出格式 (json)
  --card-content-type   interactive 卡片返回格式：user / raw / rendered（默认 user）

示例:
  # 群聊
  feishu-cli msg history --container-id oc_xxx --container-id-type chat

  # 读和某人的私聊（邮箱入口，推荐）
  feishu-cli msg history --user-email user@example.com --page-size 20 -o json

  # 读和某人的私聊（open_id 入口）
  feishu-cli msg history --user-id ou_xxx --page-size 20 -o json

  # 指定时间范围 + 升序
  feishu-cli msg history --user-email user@example.com \
    --start-time 1704067200 --sort-type ByCreateTimeAsc

  # 保留 OAPI 原始渲染版/降级版
  feishu-cli msg history --container-id oc_xxx --card-content-type rendered -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		containerIDType, _ := cmd.Flags().GetString("container-id-type")
		containerID, _ := cmd.Flags().GetString("container-id")
		userID, _ := cmd.Flags().GetString("user-id")
		userEmail, _ := cmd.Flags().GetString("user-email")
		startTime, _ := cmd.Flags().GetString("start-time")
		endTime, _ := cmd.Flags().GetString("end-time")
		sortType, _ := cmd.Flags().GetString("sort-type")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		pageToken, _ := cmd.Flags().GetString("page-token")
		output, _ := cmd.Flags().GetString("output")
		cardContentType, err := resolveCardContentType(cmd)
		if err != nil {
			return err
		}

		// 三种入口互斥，必须恰好一个
		entryCount := 0
		if containerID != "" {
			entryCount++
		}
		if userID != "" {
			entryCount++
		}
		if userEmail != "" {
			entryCount++
		}
		if entryCount == 0 {
			return fmt.Errorf("必须指定 --container-id / --user-id / --user-email 之一")
		}
		if entryCount > 1 {
			return fmt.Errorf("--container-id / --user-id / --user-email 互斥，只能指定一个")
		}

		var token string
		if userID != "" || userEmail != "" {
			var tokenErr error
			token, tokenErr = resolveRequiredUserToken(cmd)
			if tokenErr != nil {
				return tokenErr
			}
		} else {
			// 群消息历史：调用主体（User 或 Bot）必须在群里。
			// 优先用 token.json 里的 User Token（用户登录后自然在群里），找不到才回落 App Token（要求 Bot 在群里）。
			token = resolveOptionalUserTokenWithFallback(cmd)
		}

		// --user-email：搜索用户 → open_id
		if userEmail != "" {
			res, searchErr := client.SearchUsers(userEmail, 0, "", token)
			if searchErr != nil {
				return fmt.Errorf("按邮箱搜索用户失败: %w", searchErr)
			}
			if res == nil || len(res.Users) == 0 {
				return fmt.Errorf("未找到邮箱为 %s 的用户", userEmail)
			}
			userID = res.Users[0].OpenID
			if userID == "" {
				return fmt.Errorf("搜索结果未返回 open_id（邮箱 %s）", userEmail)
			}
		}

		// --user-id：反查 P2P chat_id
		if userID != "" {
			chatID, resolveErr := client.ResolveP2PChatID(userID, token)
			if resolveErr != nil {
				return resolveErr
			}
			containerID = chatID
			containerIDType = "chat"
		}

		// 走到这里必有 container-id + container-id-type
		if containerIDType == "" {
			return fmt.Errorf("必须指定 --container-id-type")
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

		// 解析发送者名字：从 mentions + contact basic_batch 两路补齐
		// 合并主消息 + merge_forward 子消息，让 sub 中的发送者也能被解析
		allMsgs := append([]*larkim.Message{}, result.Items...)
		for _, subs := range result.MergeForwardSubMessages {
			allMsgs = append(allMsgs, subs...)
		}
		senderNames := client.ResolveSenderNames(allMsgs, token)

		if output == "json" {
			enriched := map[string]any{
				"items":        result.Items,
				"has_more":     result.HasMore,
				"page_token":   result.PageToken,
				"sender_names": senderNames,
			}
			if cardTextMap := client.ExtractCardTextMap(result.Items); len(cardTextMap) > 0 {
				enriched["card_texts"] = cardTextMap
			}
			if len(result.MergeForwardSubMessages) > 0 {
				enriched["merge_forward_sub_messages"] = result.MergeForwardSubMessages
				var subMsgs []*larkim.Message
				for _, subs := range result.MergeForwardSubMessages {
					subMsgs = append(subMsgs, subs...)
				}
				if cardTextMap := client.ExtractCardTextMap(subMsgs); len(cardTextMap) > 0 {
					enriched["merge_forward_card_texts"] = cardTextMap
				}
			}
			if err := printJSON(enriched); err != nil {
				return err
			}
		} else {
			fmt.Printf("找到 %d 条消息:\n\n", len(result.Items))
			for i, msg := range result.Items {
				msgID := ""
				if msg.MessageId != nil {
					msgID = *msg.MessageId
				}
				msgType := ""
				if msg.MsgType != nil {
					msgType = *msg.MsgType
				}
				createTime := ""
				if msg.CreateTime != nil {
					createTime = *msg.CreateTime
				}
				sender := ""
				if msg.Sender != nil && msg.Sender.Id != nil {
					sender = *msg.Sender.Id
				}
				senderDisplay := sender
				if name, ok := senderNames[sender]; ok {
					senderDisplay = fmt.Sprintf("%s (%s)", name, sender)
				}

				fmt.Printf("[%d] 消息 ID: %s\n", i+1, msgID)
				fmt.Printf("    类型: %s\n", msgType)
				fmt.Printf("    发送者: %s\n", senderDisplay)
				fmt.Printf("    时间: %s\n", createTime)
				fmt.Println()
			}
			if result.HasMore {
				fmt.Printf("还有更多消息，使用 --page-token %s 获取下一页\n", result.PageToken)
			}
		}

		return nil
	},
}

func init() {
	msgCmd.AddCommand(getMessageHistoryCmd)
	getMessageHistoryCmd.Flags().String("container-id-type", "chat", "容器类型 (chat/thread)")
	getMessageHistoryCmd.Flags().String("container-id", "", "容器 ID（oc_xxx / omt_xxx），与 --user-id/--user-email 互斥")
	getMessageHistoryCmd.Flags().String("user-id", "", "对方 open_id（ou_xxx），自动反查 P2P chat_id")
	getMessageHistoryCmd.Flags().String("user-email", "", "对方邮箱，自动搜用户 + 反查 P2P chat_id")
	getMessageHistoryCmd.Flags().String("start-time", "", "起始时间（秒级时间戳）")
	getMessageHistoryCmd.Flags().String("end-time", "", "结束时间（秒级时间戳）")
	getMessageHistoryCmd.Flags().String("sort-type", "ByCreateTimeDesc", "排序方式 (ByCreateTimeAsc/ByCreateTimeDesc)")
	getMessageHistoryCmd.Flags().Int("page-size", 50, "分页大小 (1-50)")
	getMessageHistoryCmd.Flags().String("page-token", "", "分页标记")
	getMessageHistoryCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
	getMessageHistoryCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	addCardContentTypeFlag(getMessageHistoryCmd)
}
