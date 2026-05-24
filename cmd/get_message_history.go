package cmd

import (
	"fmt"
	"strings"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// messageWithSenderName 嵌入 SDK 的 *larkim.Message，并多加一个 sender_name 字段
// （JSON 序列化时合并到顶层，方便 Agent 直接读 item.sender_name 而不用查 sender_names 字典）。
type messageWithSenderName struct {
	*larkim.Message
	SenderName string `json:"sender_name,omitempty"`
}

// wrapMessagesWithSenderName 给每条消息注入 sender_name 字段（基于 senderNames 字典）。
// 用于 JSON 输出场景；文本输出场景直接读 senderNames 字典即可。
func wrapMessagesWithSenderName(msgs []*larkim.Message, names map[string]string) []messageWithSenderName {
	out := make([]messageWithSenderName, len(msgs))
	for i, m := range msgs {
		out[i].Message = m
		if m == nil || m.Sender == nil || m.Sender.Id == nil {
			continue
		}
		out[i].SenderName = names[*m.Sender.Id]
	}
	return out
}

// wrapMessageGroupsWithSenderName 给一个 chatID/threadID → []messages 的 map 中所有消息注入 sender_name。
func wrapMessageGroupsWithSenderName(groups map[string][]*larkim.Message, names map[string]string) map[string][]messageWithSenderName {
	if len(groups) == 0 {
		return nil
	}
	out := make(map[string][]messageWithSenderName, len(groups))
	for k, msgs := range groups {
		out[k] = wrapMessagesWithSenderName(msgs, names)
	}
	return out
}

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
		expandThreads, _ := cmd.Flags().GetBool("expand-threads")
		threadsPerPage, _ := cmd.Flags().GetInt("threads-per-page")
		threadsTotalLimit, _ := cmd.Flags().GetInt("threads-total-limit")
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
			// 默认 auto：优先 User Token（用户在群里），找不到回落 Bot Token（要求 Bot 在群里）。
			// 显式 --as bot：强制 Bot Token，**外部群拉群昵称推荐**（详见下方）。
			// 显式 --as user：强制 User Token。
			asFlag, _ := cmd.Flags().GetString("as")
			var tokenErr error
			token, tokenErr = resolveChatToken(cmd, asFlag)
			if tokenErr != nil {
				return tokenErr
			}
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

		// 自动展开线程回复（与官方 lark-cli 行为对齐）：对每条带 thread_id 的根消息
		// 拉取其线程内回复（ASC 顺序），结果挂在 result.ThreadReplies。
		// 共享 nameCache，从线程回复的 mentions 里抽到的名字会回填到根消息 sender。
		if expandThreads {
			client.ExpandThreadReplies(result, token, threadsPerPage, threadsTotalLimit)
		}

		// 解析发送者名字：mentions（免费）+ contact basic_batch 兜底（受外部租户限制，约 40% 覆盖率）
		// 合并主消息 + merge_forward 子消息 + thread_replies，让所有来源里的名字都能被解析
		allMsgs := append([]*larkim.Message{}, result.Items...)
		for _, subs := range result.MergeForwardSubMessages {
			allMsgs = append(allMsgs, subs...)
		}
		for _, replies := range result.ThreadReplies {
			allMsgs = append(allMsgs, replies...)
		}
		senderNames := client.ResolveSenderNames(allMsgs, token)

		// 群聊场景额外拉 chat member list 作为"群通讯录视图"补充信息。
		// **重要**：外部群下 member_id 跟 message sender_id 是不同 namespace，**不能**用 member 反查 sender 名字。
		// 所以单独输出到 chat_members 字段，供用户/Agent 知道"群里都有哪些人"，而不是混入 sender_names 误导。
		var chatMembers []*client.ChatMemberInfo
		if containerIDType == "chat" && strings.HasPrefix(containerID, "oc_") {
			chatMembers, _ = client.LoadAllChatMembers(containerID, token) // 静默降级
		}

		if output == "json" {
			enriched := map[string]any{
				"items":        wrapMessagesWithSenderName(result.Items, senderNames),
				"has_more":     result.HasMore,
				"page_token":   result.PageToken,
				"sender_names": senderNames,
			}
			if len(chatMembers) > 0 {
				enriched["chat_members"] = chatMembers
				enriched["chat_members_note"] = "chat_members 是该群完整成员名单（含群昵称），但因为飞书外部群的 ID 隔离机制，无法直接通过 member_id lookup 到 sender_id。两者请独立使用。"
			}
			if cardTextMap := client.ExtractCardTextMap(result.Items); len(cardTextMap) > 0 {
				enriched["card_texts"] = cardTextMap
			}
			if len(result.MergeForwardSubMessages) > 0 {
				enriched["merge_forward_sub_messages"] = wrapMessageGroupsWithSenderName(result.MergeForwardSubMessages, senderNames)
				var subMsgs []*larkim.Message
				for _, subs := range result.MergeForwardSubMessages {
					subMsgs = append(subMsgs, subs...)
				}
				if cardTextMap := client.ExtractCardTextMap(subMsgs); len(cardTextMap) > 0 {
					enriched["merge_forward_card_texts"] = cardTextMap
				}
			}
			if len(result.ThreadReplies) > 0 {
				enriched["thread_replies"] = wrapMessageGroupsWithSenderName(result.ThreadReplies, senderNames)
				if len(result.ThreadHasMore) > 0 {
					enriched["thread_has_more"] = result.ThreadHasMore
				}
				var allReplies []*larkim.Message
				for _, replies := range result.ThreadReplies {
					allReplies = append(allReplies, replies...)
				}
				if cardTextMap := client.ExtractCardTextMap(allReplies); len(cardTextMap) > 0 {
					enriched["thread_replies_card_texts"] = cardTextMap
				}
			}
			if err := printJSON(enriched); err != nil {
				return err
			}
		} else {
			fmt.Printf("找到 %d 条消息:\n\n", len(result.Items))
			totalReplies := 0
			for _, replies := range result.ThreadReplies {
				totalReplies += len(replies)
			}
			if totalReplies > 0 {
				fmt.Printf("（已自动展开 %d 个话题，共 %d 条回复；--expand-threads=false 关闭）\n\n",
					len(result.ThreadReplies), totalReplies)
			}
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
				if msg.ThreadId != nil && *msg.ThreadId != "" {
					tid := *msg.ThreadId
					if replies, ok := result.ThreadReplies[tid]; ok {
						suffix := ""
						if result.ThreadHasMore[tid] {
							suffix = "+"
						}
						fmt.Printf("    话题: %s（含 %d%s 条回复）\n", tid, len(replies), suffix)
					} else {
						fmt.Printf("    话题: %s\n", tid)
					}
				}
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
	getMessageHistoryCmd.Flags().String("as", "auto", "身份选择: bot | user | auto（默认 auto = User 优先回退 Bot）。外部群拉群昵称推荐 --as bot + 对外共享 App")
	// 与官方 lark-cli `+chat-messages-list` 行为对齐：默认对每条带 thread_id 的根消息
	// 自动展开线程内回复。--expand-threads=false 可关闭，回退到只拉根消息。
	getMessageHistoryCmd.Flags().Bool("expand-threads", true, "自动展开每个话题的线程回复（默认开启，与 lark-cli 对齐）")
	getMessageHistoryCmd.Flags().Int("threads-per-page", 50, "每个话题最多拉多少条回复（OpenAPI 上限 50）")
	getMessageHistoryCmd.Flags().Int("threads-total-limit", 500, "所有话题累计拉到的回复总数上限（防止极端话题群打爆 QPS）")
	addCardContentTypeFlag(getMessageHistoryCmd)
}
