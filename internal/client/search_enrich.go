package client

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// EnrichedMessage 是消息搜索结果 enrich 后的扁平结构（gap②护城河）：
// 在原始消息 ID 基础上补全内容/发送者/群名/时间，对齐 lark-cli +messages-search 输出。
type EnrichedMessage struct {
	MessageID  string `json:"message_id"`
	MsgType    string `json:"msg_type"`
	ChatID     string `json:"chat_id"`
	ChatName   string `json:"chat_name,omitempty"`
	SenderID   string `json:"sender_id,omitempty"`
	SenderName string `json:"sender_name,omitempty"`
	CreateTime string `json:"create_time,omitempty"` // 毫秒时间戳原值
	Time       string `json:"time,omitempty"`        // 本地可读时间
	Text       string `json:"text"`                  // 提取的可读文本
}

// ExtractMessageText 从消息体提取可读文本，按 msg_type 解析。
// text→纯文本；post→标题+各段文本；interactive→卡片文本；二进制类→占位符。
func ExtractMessageText(msg *larkim.Message) string {
	if msg == nil || msg.Body == nil {
		return ""
	}
	content := StringVal(msg.Body.Content)
	switch StringVal(msg.MsgType) {
	case "text":
		var t struct {
			Text string `json:"text"`
		}
		if json.Unmarshal([]byte(content), &t) == nil {
			return t.Text
		}
		return content
	case "post":
		if s := extractPostText(content); s != "" {
			return s
		}
		return content
	case "interactive":
		if texts := ExtractCardTexts(msg); len(texts) > 0 {
			return strings.Join(texts, " ")
		}
		return "[interactive]"
	case "image":
		return "[image]"
	case "file":
		return "[file]"
	case "audio":
		return "[audio]"
	case "media":
		return "[media]"
	case "sticker":
		return "[sticker]"
	case "share_chat":
		return "[share_chat]"
	case "share_user":
		return "[share_user]"
	default:
		return content
	}
}

// extractPostText 解析富文本 post 内容，兼容直接结构与按语言包裹（{"zh_cn":{...}}）两种形态。
func extractPostText(content string) string {
	type postSeg struct {
		Tag  string `json:"tag"`
		Text string `json:"text"`
	}
	type postDoc struct {
		Title   string      `json:"title"`
		Content [][]postSeg `json:"content"`
	}
	var p postDoc
	if err := json.Unmarshal([]byte(content), &p); err == nil && (p.Title != "" || len(p.Content) > 0) {
		var parts []string
		if p.Title != "" {
			parts = append(parts, p.Title)
		}
		for _, line := range p.Content {
			for _, seg := range line {
				if seg.Text != "" {
					parts = append(parts, seg.Text)
				}
			}
		}
		return strings.Join(parts, " ")
	}
	// 语言包裹形态：取第一个语言块递归解析
	var wrap map[string]json.RawMessage
	if json.Unmarshal([]byte(content), &wrap) == nil {
		for _, v := range wrap {
			if s := extractPostText(string(v)); s != "" {
				return s
			}
		}
	}
	return ""
}

// formatMsgTime 把毫秒时间戳字符串转为本地可读时间；解析失败返回原值。
func formatMsgTime(ms string) string {
	if ms == "" {
		return ""
	}
	n, err := strconv.ParseInt(ms, 10, 64)
	if err != nil {
		return ms
	}
	return time.UnixMilli(n).Local().Format("2006-01-02 15:04:05")
}

// EnrichMessages 纯转换：把消息 + 发送者名映射 + 群名映射 组装成 EnrichedMessage 列表。
func EnrichMessages(msgs []*larkim.Message, senderNames, chatNames map[string]string) []EnrichedMessage {
	out := make([]EnrichedMessage, 0, len(msgs))
	for _, m := range msgs {
		if m == nil {
			continue
		}
		senderID := ""
		if m.Sender != nil {
			senderID = StringVal(m.Sender.Id)
		}
		chatID := StringVal(m.ChatId)
		out = append(out, EnrichedMessage{
			MessageID:  StringVal(m.MessageId),
			MsgType:    StringVal(m.MsgType),
			ChatID:     chatID,
			ChatName:   chatNames[chatID],
			SenderID:   senderID,
			SenderName: senderNames[senderID],
			CreateTime: StringVal(m.CreateTime),
			Time:       formatMsgTime(StringVal(m.CreateTime)),
			Text:       ExtractMessageText(m),
		})
	}
	return out
}

// ResolveChatNames 批量解析群名（去重 + 逐个 GetChat），失败的群跳过不报错。
func ResolveChatNames(chatIDs []string, userAccessToken string) map[string]string {
	names := make(map[string]string)
	seen := map[string]bool{}
	for _, id := range chatIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		data, err := GetChat(id, userAccessToken)
		if err != nil || data == nil {
			continue
		}
		if name := StringVal(data.Name); name != "" {
			names[id] = name
		}
	}
	return names
}

// SearchMessagesEnriched 执行单页消息搜索并 enrich：
// search → 消息 ID → BatchGetMessages 取详情 → 解析发送者名/群名 → 组装。
// 返回 enriched 列表 + 原始搜索结果（含分页 token，供调用方翻页）。
func SearchMessagesEnriched(opts SearchMessagesOptions, userAccessToken, cardContentType string) ([]EnrichedMessage, *SearchMessagesResult, error) {
	searchRes, err := SearchMessages(opts, userAccessToken)
	if err != nil {
		return nil, nil, err
	}
	if len(searchRes.MessageIDs) == 0 {
		return nil, searchRes, nil
	}

	// 用 best-effort 批量取详情：搜索跨大量会话，个别消息可能因撤回 / 退群 / 无可见性
	// 而 GetMessage 失败，best-effort 跳过失败的（Messages[i] 留 nil），不让一条坏消息
	// 拖垮整页 / 整次 --page-all 富化（与本模块 ResolveSenderNames/ResolveChatNames 的容错风格一致）。
	batch, err := BatchGetMessagesBestEffort(searchRes.MessageIDs, userAccessToken, cardContentType)
	if err != nil {
		return nil, searchRes, err
	}

	senderNames := ResolveSenderNames(batch.Messages, userAccessToken)

	chatIDs := make([]string, 0, len(batch.Messages))
	for _, m := range batch.Messages {
		if m != nil {
			chatIDs = append(chatIDs, StringVal(m.ChatId))
		}
	}
	chatNames := ResolveChatNames(chatIDs, userAccessToken)

	// batch.Messages 按输入 messageIDs 顺序填充，best-effort 下失败项为 nil 空洞；
	// EnrichMessages 会跳过 nil，故输出与搜索返回顺序一致（仅缺失不可读的几条），无需重排。
	return EnrichMessages(batch.Messages, senderNames, chatNames), searchRes, nil
}
