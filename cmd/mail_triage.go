package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/output"
	"github.com/spf13/cobra"
)

// triage 翻页上限（对齐官方 mail +triage：--max 1-400，内部自动翻页）。
const (
	mailTriageDefaultMax = 20
	mailTriageMaxLimit   = 400
	mailListPageMax      = 20 // messages 列表端点单页上限
	mailSearchPageMax    = 15 // search 端点单页上限
)

var mailTriageCmd = &cobra.Command{
	Use:   "triage",
	Short: "列出/搜索/过滤邮件摘要（时间/发件人/主题/message_id）",
	Long: `列出邮箱中的邮件摘要，支持多种过滤，内部自动翻页直到 --max 封。

可选过滤:
  --folder       文件夹：系统文件夹（INBOX/SENT/DRAFT/TRASH/SPAM/ARCHIVED，或 inbox/收件箱 等别名）、
                 自定义文件夹 ID 或名称（名称自动解析为 ID）
  --label        标签：系统标签（IMPORTANT/FLAGGED/OTHER 或 important/flagged 等别名）、自定义标签 ID 或名称
  --query        关键词全文搜索（走 search 端点）
  --unread-only  只显示未读
  --max          最多返回多少封（1-400，默认 20；--page-size 为其别名）
  --page-token   从上次输出的 page_token 继续

输出:
  文本模式输出表格（时间 / 发件人 / 主题 / message_id），stderr 提示数量与下一页命令；
  -o json 输出 {items（API 原始条目，兼容旧版）, messages（摘要）, count, has_more, page_token, mailbox_id}。
  列表路径的摘要通过 batch_get format=metadata 补全；搜索路径直接取搜索结果元信息。

Folders / labels 查询:
  feishu-cli mail triage --list-folders  # 列出可用文件夹
  feishu-cli mail triage --list-labels   # 列出可用标签

示例:
  feishu-cli mail triage --folder inbox --unread-only --max 50
  feishu-cli mail triage --query "会议" -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		folder, _ := cmd.Flags().GetString("folder")
		label, _ := cmd.Flags().GetString("label")
		query, _ := cmd.Flags().GetString("query")
		unreadOnly, _ := cmd.Flags().GetBool("unread-only")
		pageToken, _ := cmd.Flags().GetString("page-token")
		listFolders, _ := cmd.Flags().GetBool("list-folders")
		listLabels, _ := cmd.Flags().GetBool("list-labels")
		output, _ := cmd.Flags().GetString("output")

		maxCount, err := resolveMailTriageMax(cmd)
		if err != nil {
			return err
		}

		token, mailbox, err := resolveMailReadIdentity(cmd)
		if err != nil {
			return err
		}

		// 辅助功能：列出 folders / labels
		if listFolders || listLabels {
			var data json.RawMessage
			if listFolders {
				data, err = client.ListMailFolders(mailbox, token)
			} else {
				data, err = client.ListMailLabels(mailbox, token)
			}
			if err != nil {
				return err
			}
			if output == "json" {
				return printJSON(json.RawMessage(data))
			}
			return printMailNamedItemsTable(data)
		}

		var result *mailTriageResult
		if query != "" {
			result, err = runMailTriageSearch(mailbox, query, folder, label, unreadOnly, pageToken, maxCount, token)
		} else {
			result, err = runMailTriageList(mailbox, folder, label, unreadOnly, pageToken, maxCount, token)
		}
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(map[string]any{
				"items":      result.Items,
				"messages":   result.Messages,
				"count":      len(result.Messages),
				"has_more":   result.HasMore,
				"page_token": result.PageToken,
				"mailbox_id": mailbox,
			})
		}
		return printMailTriageTable(result)
	},
}

// mailTriageResult triage 结果：Items 为 API 原始条目（列表路径是 message_id 字符串，搜索路径是搜索结果对象），
// Messages 为统一摘要。
type mailTriageResult struct {
	Items     []any
	Messages  []map[string]any
	HasMore   bool
	PageToken string
}

// resolveMailTriageMax 解析 --max（--page-size 为旧名别名），范围 1-400，0 取默认 20。
func resolveMailTriageMax(cmd *cobra.Command) (int, error) {
	maxCount, _ := cmd.Flags().GetInt("max")
	if cmd.Flags().Changed("page-size") && !cmd.Flags().Changed("max") {
		maxCount, _ = cmd.Flags().GetInt("page-size")
	}
	if maxCount < 0 {
		return 0, clierr.Usagef("--max 不能为负数")
	}
	if maxCount == 0 {
		maxCount = mailTriageDefaultMax
	}
	if maxCount > mailTriageMaxLimit {
		return 0, clierr.Usagef("--max 最大 %d（当前 %d）；更多结果请用输出的 page_token 继续翻页", mailTriageMaxLimit, maxCount)
	}
	return maxCount, nil
}

// decodeMailJSON 用 UseNumber 解析，保留大整数精度。
func decodeMailJSON(data json.RawMessage) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("解析邮件列表响应失败: %w", err)
	}
	return m, nil
}

// runMailTriageList 列表路径：folder/label 先解析为 ID，按页拉取 message_id，再 batch_get(metadata) 补摘要。
func runMailTriageList(mailbox, folder, label string, unreadOnly bool, pageToken string, maxCount int, token string) (*mailTriageResult, error) {
	folderID, err := client.ResolveMailFolderID(mailbox, folder, token)
	if err != nil {
		return nil, clierr.Usage(err)
	}
	labelID, err := client.ResolveMailLabelID(mailbox, label, token)
	if err != nil {
		return nil, clierr.Usage(err)
	}
	res := &mailTriageResult{}
	var ids []string
	seen := map[string]bool{}
	for len(ids) < maxCount {
		size := maxCount - len(ids)
		if size > mailListPageMax {
			size = mailListPageMax
		}
		data, err := client.ListMailMessages(client.ListMailMessagesParams{
			MailboxID:  mailbox,
			FolderID:   folderID,
			LabelID:    labelID,
			UnreadOnly: unreadOnly,
			PageSize:   size,
			PageToken:  pageToken,
		}, token)
		if err != nil {
			return nil, err
		}
		page, err := decodeMailJSON(data)
		if err != nil {
			return nil, err
		}
		items, _ := page["items"].([]any)
		for _, it := range items {
			id := mailTriageItemID(it)
			if id == "" {
				continue
			}
			res.Items = append(res.Items, it)
			ids = append(ids, id)
		}
		hasMore, _ := page["has_more"].(bool)
		next := mailAnyString(page["page_token"])
		res.HasMore, res.PageToken = hasMore, next
		if !hasMore || next == "" {
			res.PageToken = ""
			break
		}
		if seen[next] || next == pageToken {
			fmt.Fprintf(os.Stderr, "警告: 服务端返回了重复的 page_token，已停止翻页\n")
			break
		}
		seen[next] = true
		pageToken = next
	}
	if len(ids) > maxCount {
		ids, res.Items = ids[:maxCount], res.Items[:maxCount]
	}
	metas, err := fetchMailTriageMetas(mailbox, ids, token)
	if err != nil {
		return nil, err
	}
	res.Messages = metas
	return res, nil
}

// runMailTriageSearch 搜索路径：自动翻页（单页 ≤15），摘要取自搜索结果 meta_data。
func runMailTriageSearch(mailbox, query, folder, label string, unreadOnly bool, pageToken string, maxCount int, token string) (*mailTriageResult, error) {
	res := &mailTriageResult{}
	seen := map[string]bool{}
	for len(res.Messages) < maxCount {
		size := maxCount - len(res.Messages)
		if size > mailSearchPageMax {
			size = mailSearchPageMax
		}
		filter := map[string]any{"page_size": size}
		if folder != "" {
			filter["folder"] = []string{folder}
		}
		if label != "" {
			filter["label"] = []string{label}
		}
		if unreadOnly {
			filter["is_unread"] = true
		}
		if pageToken != "" {
			filter["page_token"] = pageToken
		}
		data, err := client.SearchMailMessages(mailbox, query, filter, token)
		if err != nil {
			return nil, err
		}
		page, err := decodeMailJSON(data)
		if err != nil {
			return nil, err
		}
		items, _ := page["items"].([]any)
		for _, it := range items {
			if summary := mailTriageSearchSummary(it); summary != nil {
				res.Items = append(res.Items, it)
				res.Messages = append(res.Messages, summary)
			}
		}
		if notice := mailAnyString(page["notice"]); notice != "" {
			fmt.Fprintf(os.Stderr, "提示: %s\n", sanitizeMailSingleLine(notice))
		}
		hasMore, _ := page["has_more"].(bool)
		next := mailAnyString(page["page_token"])
		res.HasMore, res.PageToken = hasMore, next
		if !hasMore || next == "" {
			res.PageToken = ""
			break
		}
		if seen[next] || next == pageToken {
			fmt.Fprintf(os.Stderr, "警告: 服务端返回了重复的 page_token，已停止翻页\n")
			break
		}
		seen[next] = true
		pageToken = next
	}
	if len(res.Messages) > maxCount {
		res.Messages, res.Items = res.Messages[:maxCount], res.Items[:maxCount]
	}
	return res, nil
}

// mailFormatISOTime 把搜索结果的 RFC3339 时间转为与列表路径一致的本地时间格式；无法解析时原样返回。
func mailFormatISOTime(s string) string {
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(s)); err == nil {
		return t.Local().Format("2006-01-02 15:04:05")
	}
	return s
}

func mailTriageItemID(it any) string {
	switch v := it.(type) {
	case string:
		return v
	case map[string]any:
		if id := mailAnyString(v["message_id"]); id != "" {
			return id
		}
		return mailAnyString(v["id"])
	}
	return ""
}

// fetchMailTriageMetas 用 batch_get(format=metadata) 为 message_id 列表补摘要；保持输入顺序，缺失项标注 error。
func fetchMailTriageMetas(mailbox string, ids []string, token string) ([]map[string]any, error) {
	if len(ids) == 0 {
		return []map[string]any{}, nil
	}
	data, err := client.BatchGetMailMessages(mailbox, ids, "metadata", token)
	if err != nil {
		return nil, fmt.Errorf("获取邮件摘要失败: %w", err)
	}
	payload, err := decodeMailJSON(data)
	if err != nil {
		return nil, err
	}
	byID := map[string]map[string]any{}
	if list, ok := payload["messages"].([]any); ok {
		for _, it := range list {
			if m, ok := it.(map[string]any); ok {
				byID[mailAnyString(m["message_id"])] = m
			}
		}
	}
	out := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		m, ok := byID[id]
		if !ok {
			out = append(out, map[string]any{"message_id": id, "error": "batch_get 未返回该邮件的元信息"})
			continue
		}
		var labels []string
		if ls, ok := m["label_ids"].([]any); ok {
			for _, l := range ls {
				labels = append(labels, mailAnyString(l))
			}
		}
		out = append(out, map[string]any{
			"message_id": id,
			"thread_id":  mailAnyString(m["thread_id"]),
			"subject":    sanitizeMailSingleLine(mailAnyString(m["subject"])),
			"from":       mailAddressDisplay(m["head_from"]),
			"date":       mailFormatMillis(m["internal_date"]),
			"folder":     mailAnyString(m["folder_id"]),
			"labels":     strings.Join(labels, ","),
		})
	}
	return out, nil
}

// mailTriageSearchSummary 从搜索结果条目提取摘要（meta_data.message_biz_id/title/from/create_time）。
func mailTriageSearchSummary(it any) map[string]any {
	item, ok := it.(map[string]any)
	if !ok {
		return nil
	}
	meta, _ := item["meta_data"].(map[string]any)
	id := ""
	if meta != nil {
		id = mailAnyString(meta["message_biz_id"])
	}
	if id == "" {
		id = mailAnyString(item["id"])
	}
	if id == "" {
		return nil
	}
	summary := map[string]any{"message_id": id}
	if meta != nil {
		summary["thread_id"] = mailAnyString(meta["thread_id"])
		summary["subject"] = sanitizeMailSingleLine(mailAnyString(meta["title"]))
		summary["from"] = mailAddressDisplay(meta["from"])
		summary["date"] = mailFormatISOTime(mailAnyString(meta["create_time"]))
	}
	return summary
}

func printMailTriageTable(res *mailTriageResult) error {
	if len(res.Messages) == 0 {
		fmt.Fprintln(os.Stderr, "没有找到邮件。")
		return nil
	}
	rows := make([]map[string]any, 0, len(res.Messages))
	for _, m := range res.Messages {
		rows = append(rows, map[string]any{
			"date":       m["date"],
			"from":       m["from"],
			"subject":    m["subject"],
			"message_id": m["message_id"],
		})
	}
	text, err := output.RenderString(&output.Options{Format: output.FormatTable}, rows)
	if err != nil {
		return err
	}
	fmt.Print(text)
	fmt.Fprintf(os.Stderr, "\n共 %d 封\n", len(res.Messages))
	if res.HasMore && res.PageToken != "" {
		fmt.Fprintf(os.Stderr, "还有更多，可加 --page-token %s 继续（保持其它过滤参数不变）\n", res.PageToken)
	}
	return nil
}

// printMailNamedItemsTable 以表格输出文件夹/标签列表（id / name / 未读数）。
func printMailNamedItemsTable(data json.RawMessage) error {
	payload, err := decodeMailJSON(data)
	if err != nil {
		return err
	}
	var items []any
	for _, key := range []string{"items", "folders", "labels"} {
		if list, ok := payload[key].([]any); ok {
			items = append(items, list...)
		}
	}
	rows := make([]map[string]any, 0, len(items))
	for _, it := range items {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		row := map[string]any{"id": m["id"], "name": sanitizeMailSingleLine(mailAnyString(m["name"]))}
		if v, ok := m["unread_message_count"]; ok {
			row["unread"] = v
		}
		rows = append(rows, row)
	}
	text, err := output.RenderString(&output.Options{Format: output.FormatTable}, rows)
	if err != nil {
		return err
	}
	fmt.Print(text)
	return nil
}

func init() {
	mailCmd.AddCommand(mailTriageCmd)
	mailTriageCmd.Flags().String("mailbox", "me", "邮箱地址（默认 me）")
	mailTriageCmd.Flags().String("folder", "", "文件夹：系统文件夹/别名（inbox、收件箱…）、自定义文件夹 ID 或名称")
	mailTriageCmd.Flags().String("label", "", "标签：系统标签/别名（important、flagged…）、自定义标签 ID 或名称")
	mailTriageCmd.Flags().String("query", "", "关键词搜索")
	mailTriageCmd.Flags().Bool("unread-only", false, "只显示未读")
	mailTriageCmd.Flags().Int("max", mailTriageDefaultMax, "最多返回多少封（1-400，自动翻页）")
	mailTriageCmd.Flags().Int("page-size", 0, "同 --max（旧名，保留兼容）")
	mailTriageCmd.Flags().String("page-token", "", "分页标记（上次输出的 page_token）")
	mailTriageCmd.Flags().Bool("list-folders", false, "列出可用文件夹")
	mailTriageCmd.Flags().Bool("list-labels", false, "列出可用标签")
	mailTriageCmd.Flags().String("as", "auto", "身份选择: bot | user | auto（默认 auto）")
	mailTriageCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mailTriageCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
}
