package cmd

import (
	"encoding/base64"
	"fmt"
	"net/mail"
	"regexp"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/riba2534/feishu-cli/v2/internal/mailmime"
	"github.com/spf13/cobra"
)

var mailDraftEditCmd = &cobra.Command{
	Use:   "draft-edit",
	Short: "编辑已有邮件草稿（只改指定部分，保留其余头与附件）",
	Long: `编辑已有的邮件草稿，不发送。

采用"读取 → 局部修改 → 写回"：先读取草稿原始 EML，只修改显式传入的字段，
未修改的邮件头（In-Reply-To / References / X-LMS-Reply-To-Message-Id 等）、
附件与内联图片原样保留。回复/转发草稿中由飞书生成的引用块默认保留。

至少指定一项修改:
  --to / --cc / --bcc   替换整个收件人/抄送/密送列表（传空字符串表示清空该列表）
  --subject             替换主题
  --body                替换正文（默认保留回复/转发引用块；--drop-quote 一并删除）
  --from / --from-name  替换发件人地址 / 显示名

可选:
  --html / --plain-text 强制新正文类型（默认按 --body 内容自动检测）
  --drop-quote          替换正文时连同引用块一起删除
  --mailbox             邮箱 ID（默认 me）
  -o json               JSON 格式输出

注意：本命令没有乐观锁，同一草稿被并发编辑时以最后一次写入为准。

示例:
  feishu-cli mail draft-edit --draft-id xxx --subject "修改后的主题"
  feishu-cli mail draft-edit --draft-id xxx --body "新的回复内容"          # 保留引用块与附件
  feishu-cli mail draft-edit --draft-id xxx --to user@example.com --cc ""   # 改收件人并清空抄送`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		mailbox, _ := cmd.Flags().GetString("mailbox")
		draftID, _ := cmd.Flags().GetString("draft-id")
		forceHTML, _ := cmd.Flags().GetBool("html")
		plainText, _ := cmd.Flags().GetBool("plain-text")
		dropQuote, _ := cmd.Flags().GetBool("drop-quote")
		output, _ := cmd.Flags().GetString("output")

		if strings.TrimSpace(draftID) == "" {
			return clierr.Usagef("--draft-id 必填")
		}
		if forceHTML && plainText {
			return clierr.Usagef("--html 与 --plain-text 互斥，只能指定其中一个")
		}
		edit, err := collectMailDraftEdit(cmd)
		if err != nil {
			return err
		}
		if edit.empty() {
			return clierr.Usagef("至少指定一项修改：--to / --cc / --bcc / --subject / --body / --from / --from-name")
		}
		edit.forceHTML, edit.plainText, edit.dropQuote = forceHTML, plainText, dropQuote

		token, err := requireUserToken(cmd, "mail draft-edit")
		if err != nil {
			return err
		}

		rawB64, err := client.GetMailDraftRaw(mailbox, draftID, token)
		if err != nil {
			return fmt.Errorf("读取草稿原文失败: %w", err)
		}
		emlBytes, err := decodeMailRawEML(rawB64)
		if err != nil {
			return err
		}
		updated, changed, err := applyMailDraftEdit(emlBytes, edit)
		if err != nil {
			return err
		}

		if err := client.UpdateMailDraft(mailbox, draftID, base64.RawURLEncoding.EncodeToString(updated), token); err != nil {
			return err
		}

		result := map[string]any{"draft_id": draftID, "updated": true, "changed": changed}
		if output == "json" {
			return printJSON(result)
		}
		fmt.Printf("草稿更新成功: %s（修改: %s）\n", draftID, strings.Join(changed, ", "))
		return nil
	},
}

// mailDraftEdit draft-edit 的修改意图；指针为 nil 表示该字段未修改。
type mailDraftEdit struct {
	to, cc, bcc *[]string
	subject     *string
	body        *string
	from        *string
	fromName    *string
	forceHTML   bool
	plainText   bool
	dropQuote   bool
}

func (e mailDraftEdit) empty() bool {
	return e.to == nil && e.cc == nil && e.bcc == nil && e.subject == nil && e.body == nil && e.from == nil && e.fromName == nil
}

// collectMailDraftEdit 只收集显式传入（Changed）的 flag；--cc "" 表示清空抄送。
func collectMailDraftEdit(cmd *cobra.Command) (mailDraftEdit, error) {
	var e mailDraftEdit
	for _, item := range []struct {
		flag string
		dst  **[]string
	}{{"to", &e.to}, {"cc", &e.cc}, {"bcc", &e.bcc}} {
		if !cmd.Flags().Changed(item.flag) {
			continue
		}
		raw, _ := cmd.Flags().GetString(item.flag)
		list, err := parseEmailList(raw)
		if err != nil {
			return e, clierr.Usage(err)
		}
		*item.dst = &list
	}
	if e.to != nil && len(*e.to) == 0 {
		return e, clierr.Usagef("--to 不能为空（草稿至少需要一个收件人）")
	}
	for _, item := range []struct {
		flag string
		dst  **string
	}{{"subject", &e.subject}, {"body", &e.body}, {"from", &e.from}, {"from-name", &e.fromName}} {
		if !cmd.Flags().Changed(item.flag) {
			continue
		}
		v, _ := cmd.Flags().GetString(item.flag)
		*item.dst = &v
	}
	if e.from != nil {
		if _, err := mail.ParseAddress(*e.from); err != nil {
			return e, clierr.Usagef("--from 邮箱格式不正确: %v", err)
		}
	}
	return e, nil
}

// decodeMailRawEML 解码草稿 raw（base64url，带/不带 padding 均可）。
func decodeMailRawEML(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	for _, enc := range []*base64.Encoding{base64.RawURLEncoding, base64.URLEncoding, base64.StdEncoding, base64.RawStdEncoding} {
		if b, err := enc.DecodeString(raw); err == nil {
			return b, nil
		}
	}
	return nil, fmt.Errorf("草稿 raw 不是合法的 base64url 编码")
}

// applyMailDraftEdit 在原 EML 上做最小修改，返回新 EML 与被修改的字段名列表。
func applyMailDraftEdit(eml []byte, e mailDraftEdit) ([]byte, []string, error) {
	root, err := mailmime.Parse(eml)
	if err != nil {
		return nil, nil, fmt.Errorf("解析草稿 EML 失败（为避免丢失附件/引用，未做任何修改）: %w", err)
	}
	var changed []string

	setAddrs := func(header string, list *[]string) error {
		if list == nil {
			return nil
		}
		changed = append(changed, strings.ToLower(header))
		if len(*list) == 0 {
			root.Del(header)
			return nil
		}
		return root.Set(header, formatMailAddressHeader(*list))
	}
	if err := setAddrs("To", e.to); err != nil {
		return nil, nil, err
	}
	if err := setAddrs("Cc", e.cc); err != nil {
		return nil, nil, err
	}
	if err := setAddrs("Bcc", e.bcc); err != nil {
		return nil, nil, err
	}
	if e.subject != nil {
		if err := root.Set("Subject", mimeEncodeHeader(*e.subject)); err != nil {
			return nil, nil, err
		}
		changed = append(changed, "subject")
	}
	if e.from != nil || e.fromName != nil {
		name, addr := "", ""
		if cur, perr := mail.ParseAddress(root.Get("From")); perr == nil {
			name, addr = cur.Name, cur.Address
		}
		if e.from != nil {
			if a, perr := mail.ParseAddress(*e.from); perr == nil {
				addr = a.Address
				if a.Name != "" {
					name = a.Name
				}
			}
		}
		if e.fromName != nil {
			name = *e.fromName
		}
		if addr == "" {
			return nil, nil, clierr.Usagef("草稿当前没有发件人地址，请同时传 --from")
		}
		if err := root.Set("From", formatMailAddress(name, addr)); err != nil {
			return nil, nil, err
		}
		changed = append(changed, "from")
	}
	if e.body != nil {
		isReply := root.Has("In-Reply-To") || root.Has("References") || root.Has("X-LMS-Reply-To-Message-Id")
		if err := replaceMailDraftBody(root, *e.body, e, isReply); err != nil {
			return nil, nil, err
		}
		changed = append(changed, "body")
	}
	return root.Bytes(), changed, nil
}

// replaceMailDraftBody 替换正文 part，保留引用块（除非 --drop-quote）与其它 part。
//   - 草稿含 text/html：新正文（纯文本会转义为 HTML）+ 原引用块写入 HTML part；若同时有 text/plain（摘要），
//     用新 HTML 的纯文本形式同步更新，避免两份正文不一致；
//   - 草稿只有 text/plain：新正文为纯文本时直接替换；为 HTML 时把该 part 改为 text/html，原纯文本引用块转义后附在末尾。
func replaceMailDraftBody(root *mailmime.Part, body string, e mailDraftEdit, isReply bool) error {
	plainPart, htmlPart := root.BodyParts()
	newIsHTML := e.forceHTML || (!e.plainText && detectHTMLBody(body))
	keepQuote := !e.dropQuote

	// 纯文本草稿：服务端 format=raw 导出时会合成一个 "<p>" + 纯文本 + "</p>" 的 HTML 备选（未转义）。
	// 这类草稿按纯文本处理：引用块从 text/plain 中拆分；新正文为纯文本时移除合成的 HTML 备选，
	// 由服务端按纯文本重新生成（避免把未转义内容当 HTML 写回）。
	if htmlPart != nil && plainPart != nil {
		oldHTML, herr := htmlPart.Decoded()
		oldPlain, perr := plainPart.Decoded()
		if herr == nil && perr == nil && isServerSynthesizedHTML(string(oldHTML), string(oldPlain)) {
			quote := ""
			if keepQuote {
				_, quote = splitMailPlainQuote(string(oldPlain), isReply)
			}
			if newIsHTML {
				html := body
				if strings.TrimSpace(quote) != "" {
					html += fmt.Sprintf(mailQuotePlainBodyHTML, mailHTMLEscape(strings.TrimLeft(quote, "\n")))
				}
				if err := htmlPart.SetContent("text/html", []byte(html)); err != nil {
					return err
				}
				return plainPart.SetContent("text/plain", []byte(mailHTMLToText(html)))
			}
			root.RemoveChild(htmlPart)
			return plainPart.SetContent("text/plain", []byte(body+quote))
		}
	}

	if htmlPart != nil {
		oldHTML, err := htmlPart.Decoded()
		if err != nil {
			return fmt.Errorf("解码草稿 HTML 正文失败: %w", err)
		}
		userHTML := body
		if !newIsHTML {
			userHTML = mailPlainTextToHTML(body)
		}
		quote := ""
		if keepQuote {
			_, quote = splitMailHTMLQuote(string(oldHTML))
		}
		newHTML := userHTML + quote
		if err := htmlPart.SetContent("text/html", []byte(newHTML)); err != nil {
			return err
		}
		if plainPart != nil {
			newPlain := body
			if newIsHTML {
				newPlain = mailHTMLToText(body)
			}
			if quote != "" {
				newPlain += "\n\n" + mailHTMLToText(quote)
			}
			if err := plainPart.SetContent("text/plain", []byte(newPlain)); err != nil {
				return err
			}
		}
		return nil
	}
	if plainPart == nil {
		return fmt.Errorf("草稿中未找到正文 part（text/plain 或 text/html），未做修改")
	}
	oldPlain, err := plainPart.Decoded()
	if err != nil {
		return fmt.Errorf("解码草稿纯文本正文失败: %w", err)
	}
	quote := ""
	if keepQuote {
		_, quote = splitMailPlainQuote(string(oldPlain), isReply)
	}
	if newIsHTML {
		html := body
		if strings.TrimSpace(quote) != "" {
			html += fmt.Sprintf(mailQuotePlainBodyHTML, mailHTMLEscape(strings.TrimLeft(quote, "\r\n")))
		}
		return plainPart.SetContent("text/html", []byte(html))
	}
	return plainPart.SetContent("text/plain", []byte(body+quote))
}

// isServerSynthesizedHTML 判断 HTML 备选是否为服务端由纯文本合成（"<p>" + 纯文本 + "</p>"）。
func isServerSynthesizedHTML(htmlContent, plainContent string) bool {
	h := strings.TrimSpace(strings.ReplaceAll(htmlContent, "\r\n", "\n"))
	p := strings.TrimSpace(strings.ReplaceAll(plainContent, "\r\n", "\n"))
	return h == "<p>"+p+"</p>"
}

// mailQuoteWrapperRe 定位飞书引用块（与官方 draft/projection.go 一致）。
var mailQuoteWrapperRe = regexp.MustCompile(`<div\s[^>]*class="[^"]*` + mailQuoteWrapperClass + `[^"]*"`)

// splitMailHTMLQuote 把 HTML 正文拆为用户正文与末尾的引用块；无引用块时 quote 为空。
func splitMailHTMLQuote(html string) (body, quote string) {
	loc := mailQuoteWrapperRe.FindStringIndex(html)
	if loc == nil {
		return html, ""
	}
	return html[:loc[0]], html[loc[0]:]
}

// mailForwardSeparators 纯文本转发分隔行（新旧格式）。
var mailForwardSeparators = []string{
	"--------- 转发消息 ---------",
	"---------- Forwarded message ---------",
	"---------- 转发邮件 ----------",
}

// splitMailPlainQuote 把纯文本正文拆为用户正文与末尾的引用块（quote 含前导空行）。
//   - 任意草稿：从转发分隔行开始视为引用块；
//   - 回复草稿（isReply）：末尾连续的 ">" 引用行（允许夹空行，至少 2 行）视为引用块，
//     其前一行若是 "xxx 写道:" / "wrote:" 一并纳入。
func splitMailPlainQuote(text string, isReply bool) (body, quote string) {
	norm := strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(norm, "\n")
	cut := -1
	for i, line := range lines {
		t := strings.TrimSpace(line)
		for _, sep := range mailForwardSeparators {
			if t == sep {
				cut = i
				break
			}
		}
		if cut >= 0 {
			break
		}
	}
	if cut < 0 && isReply {
		end := len(lines)
		for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
			end--
		}
		start := end
		quoted := 0
		for start > 0 {
			t := strings.TrimSpace(lines[start-1])
			if strings.HasPrefix(t, ">") {
				quoted++
				start--
				continue
			}
			break
		}
		if quoted >= 2 {
			cut = start
			if cut > 0 {
				prev := strings.TrimSpace(lines[cut-1])
				if strings.HasSuffix(prev, "写道:") || strings.HasSuffix(prev, "写道：") || strings.HasSuffix(strings.ToLower(prev), "wrote:") {
					cut--
				}
			}
		}
	}
	if cut < 0 {
		return text, ""
	}
	// 把引用块前的空行也划入 quote，保证替换后正文与引用之间的空行不丢
	for cut > 0 && strings.TrimSpace(lines[cut-1]) == "" {
		cut--
	}
	return strings.Join(lines[:cut], "\n"), "\n" + strings.Join(lines[cut:], "\n")
}

func init() {
	mailCmd.AddCommand(mailDraftEditCmd)
	mailDraftEditCmd.Flags().String("mailbox", "me", "邮箱 ID（默认 me）")
	mailDraftEditCmd.Flags().String("draft-id", "", "草稿 ID（必填）")
	mailDraftEditCmd.Flags().String("to", "", "替换收件人列表（逗号分隔）")
	mailDraftEditCmd.Flags().String("cc", "", "替换抄送列表（传空字符串表示清空）")
	mailDraftEditCmd.Flags().String("bcc", "", "替换密送列表（传空字符串表示清空）")
	mailDraftEditCmd.Flags().String("subject", "", "替换主题")
	mailDraftEditCmd.Flags().String("body", "", "替换正文（默认保留回复/转发引用块）")
	mailDraftEditCmd.Flags().String("from", "", "替换发件人地址")
	mailDraftEditCmd.Flags().String("from-name", "", "替换发件人显示名")
	mailDraftEditCmd.Flags().Bool("html", false, "强制视为 HTML body")
	mailDraftEditCmd.Flags().Bool("plain-text", false, "强制视为纯文本")
	mailDraftEditCmd.Flags().Bool("drop-quote", false, "替换正文时连同回复/转发引用块一起删除")
	mailDraftEditCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mailDraftEditCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(mailDraftEditCmd, "draft-id")
}
