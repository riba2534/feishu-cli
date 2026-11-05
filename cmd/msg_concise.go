package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
)

// --concise：把消息列表渲染为紧凑 Markdown，便于 Agent 阅读总结；默认 JSON / 文本输出不受影响。
// 每条消息保留 message_id、发送者、时间、话题与回复关系，翻页信息放在末尾。

const msgConciseFlagHelp = "紧凑 Markdown 视图（渲染正文、保留 message_id/话题/回复关系/page_token），与 -o json 互斥"

// validateConciseOutput --concise 与 -o json 互斥（exit 2）。
func validateConciseOutput(concise bool, output string) error {
	if concise && output != "" {
		return clierr.Usagef("--concise 与 -o %s 互斥，只能指定其中一个", output)
	}
	return nil
}

type conciseRenderer struct {
	w           io.Writer
	senderNames map[string]string
	loc         *time.Location
}

func newConciseRenderer(w io.Writer, senderNames map[string]string) *conciseRenderer {
	return &conciseRenderer{w: w, senderNames: senderNames, loc: time.Local}
}

// header 输出一行消息头：- 时间 发送者 · message_id [· 回复 parent] 。
func (c *conciseRenderer) message(m *larkim.Message, indent string) {
	if m == nil {
		return
	}
	var meta []string
	if t := c.formatTime(client.StringVal(m.CreateTime)); t != "" {
		meta = append(meta, t)
	}
	meta = append(meta, c.sender(m))
	head := strings.Join(meta, " ")
	ids := []string{client.StringVal(m.MessageId)}
	if parent := client.StringVal(m.ParentId); parent != "" {
		ids = append(ids, "回复 "+parent)
	}
	if m.Deleted != nil && *m.Deleted {
		ids = append(ids, "已撤回")
	}
	if m.Updated != nil && *m.Updated {
		ids = append(ids, "已编辑")
	}
	fmt.Fprintf(c.w, "%s- %s · %s\n", indent, head, strings.Join(ids, " · "))
	body := conciseMessageBody(m)
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimRight(line, " \t\r")
		if line == "" {
			continue
		}
		fmt.Fprintf(c.w, "%s  %s\n", indent, line)
	}
}

func (c *conciseRenderer) sender(m *larkim.Message) string {
	if m.Sender == nil {
		return "未知发送者"
	}
	id := client.StringVal(m.Sender.Id)
	name := c.senderNames[id]
	switch {
	case name != "" && id != "":
		return fmt.Sprintf("%s (%s)", name, id)
	case id != "":
		return id
	case client.StringVal(m.Sender.SenderType) != "":
		return client.StringVal(m.Sender.SenderType)
	}
	return "未知发送者"
}

func (c *conciseRenderer) formatTime(ms string) string {
	n, err := strconv.ParseInt(strings.TrimSpace(ms), 10, 64)
	if err != nil || n <= 0 {
		return ""
	}
	return time.UnixMilli(n).In(c.loc).Format("2006-01-02 15:04:05")
}

// thread 渲染某个根消息下已展开的话题回复。
func (c *conciseRenderer) thread(threadID string, replies []*larkim.Message, hasMore bool) {
	suffix := ""
	if hasMore {
		suffix = "，还有更多（用 msg thread-messages " + threadID + " 翻页）"
	}
	fmt.Fprintf(c.w, "  > 话题 %s：%d 条回复%s\n", threadID, len(replies), suffix)
	for _, r := range replies {
		c.message(r, "    ")
	}
}

func (c *conciseRenderer) footer(hasMore bool, pageToken string) {
	if hasMore {
		fmt.Fprintf(c.w, "\n还有更多：--page-token %s\n", pageToken)
	}
}

// conciseMessageBody 把消息 content 渲染为可读文本（未知类型给出 [类型] 占位）。
func conciseMessageBody(m *larkim.Message) string {
	msgType := client.StringVal(m.MsgType)
	content := ""
	if m.Body != nil {
		content = client.StringVal(m.Body.Content)
	}
	var v map[string]any
	_ = json.Unmarshal([]byte(content), &v)
	str := func(k string) string {
		s, _ := v[k].(string)
		return s
	}
	var out string
	switch msgType {
	case "text":
		out = str("text")
	case "post":
		out = renderPostText(v)
	case "image":
		out = "[图片 " + str("image_key") + "]"
	case "file":
		out = "[文件 " + firstNonEmptyString(str("file_name"), str("file_key")) + "]"
	case "audio":
		out = "[语音]"
	case "media":
		out = "[视频 " + firstNonEmptyString(str("file_name"), str("file_key")) + "]"
	case "sticker":
		out = "[表情]"
	case "interactive":
		texts := client.ExtractCardTexts(m)
		if len(texts) == 0 {
			out = "[卡片]"
		} else {
			out = "[卡片] " + strings.Join(texts, " / ")
		}
	case "merge_forward":
		out = "[合并转发]"
	case "share_chat":
		out = "[群名片 " + str("chat_id") + "]"
	case "share_user":
		out = "[个人名片 " + str("user_id") + "]"
	case "system":
		out = "[系统消息]"
	default:
		if content == "" {
			out = "[" + msgType + "]"
		} else {
			out = "[" + msgType + "] " + content
		}
	}
	return replaceMentionKeys(out, m.Mentions)
}

// renderPostText 渲染富文本：兼容 {title, content} 与按语言包裹的 {zh_cn:{title, content}}。
func renderPostText(v map[string]any) string {
	if v == nil {
		return "[富文本]"
	}
	if _, ok := v["content"]; !ok {
		for _, lang := range []string{"zh_cn", "en_us", "ja_jp"} {
			if inner, ok := v[lang].(map[string]any); ok {
				v = inner
				break
			}
		}
	}
	var b strings.Builder
	if title, _ := v["title"].(string); title != "" {
		b.WriteString(title)
		b.WriteString("\n")
	}
	paras, _ := v["content"].([]any)
	for _, p := range paras {
		elems, _ := p.([]any)
		var line strings.Builder
		for _, e := range elems {
			el, _ := e.(map[string]any)
			tag, _ := el["tag"].(string)
			text, _ := el["text"].(string)
			switch tag {
			case "text", "md":
				line.WriteString(text)
			case "a":
				href, _ := el["href"].(string)
				fmt.Fprintf(&line, "[%s](%s)", text, href)
			case "at":
				name, _ := el["user_name"].(string)
				if name == "" {
					name, _ = el["user_id"].(string)
				}
				line.WriteString("@" + name)
			case "img":
				line.WriteString("[图片]")
			case "media":
				line.WriteString("[视频]")
			case "emotion":
				key, _ := el["emoji_type"].(string)
				line.WriteString("[" + key + "]")
			case "code_block":
				line.WriteString("```\n" + text + "\n```")
			case "hr":
				line.WriteString("---")
			default:
				line.WriteString(text)
			}
		}
		b.WriteString(line.String())
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// replaceMentionKeys 把 @_user_1 这类占位替换为 @名字。
func replaceMentionKeys(s string, mentions []*larkim.Mention) string {
	for _, mn := range mentions {
		if mn == nil {
			continue
		}
		key := client.StringVal(mn.Key)
		name := client.StringVal(mn.Name)
		if key != "" && name != "" {
			s = strings.ReplaceAll(s, key, "@"+name)
		}
	}
	return s
}

// renderHistoryConcise msg history --concise 输出。
func renderHistoryConcise(w io.Writer, title string, result *client.ListMessagesResult, senderNames map[string]string) {
	c := newConciseRenderer(w, senderNames)
	fmt.Fprintf(w, "# %s（%d 条）\n\n", title, len(result.Items))
	for _, m := range result.Items {
		c.message(m, "")
		tid := ""
		if m != nil {
			tid = client.StringVal(m.ThreadId)
		}
		if m != nil {
			if subs := result.MergeForwardSubMessages[client.StringVal(m.MessageId)]; len(subs) > 0 {
				fmt.Fprintf(w, "  > 合并转发内容：%d 条\n", len(subs))
				for _, sub := range subs {
					c.message(sub, "    ")
				}
			}
		}
		if replies, ok := result.ThreadReplies[tid]; ok && tid != "" {
			c.thread(tid, replies, result.ThreadHasMore[tid])
		} else if tid != "" {
			fmt.Fprintf(w, "  > 话题 %s\n", tid)
		}
	}
	c.footer(result.HasMore, result.PageToken)
}
