package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

// doc history：文档历史版本与回滚（docs_ai，对齐官方 docs +history-list / +history-revert / +history-revert-status）。

var docHistoryCmd = &cobra.Command{
	Use:   "history",
	Short: "文档历史版本：列出 / 回滚 / 查询回滚状态",
	Long: `查看 docx 历史版本、按 history_version_id 回滚，以及查询回滚任务状态（docs_ai 接口）。

子命令:
  list            列出历史版本（每页 1-20 条，--page-all 自动翻页）
  revert          回滚到指定 history_version_id（写操作，非交互环境需 --yes；支持 --dry-run）
  revert-status   查询回滚任务状态（running / done / partial_failed / failed）

注意：回滚接口只接受 history_version_id（list 返回的字段），不要传 revision_id。

示例:
  feishu-cli doc history list DOC_ID
  feishu-cli doc history revert DOC_ID --history-version-id 11 --yes
  feishu-cli doc history revert-status DOC_ID --task-id task_xxx`,
}

var docHistoryListCmd = &cobra.Command{
	Use:   "list <document_id|url>",
	Short: "列出文档历史版本",
	Long: `列出 docx 历史版本（GET /open-apis/docs_ai/v1/documents/{id}/histories）。

每条记录含 revision_id、history_version_id、edit_time（RFC3339）、name、description、editor_ids。
has_more=true 时会在 stderr 提示并在 JSON 中返回 page_token；--page-all 自动翻完（最多 50 页）。

示例:
  feishu-cli doc history list DOC_ID
  feishu-cli doc history list DOC_ID --page-size 5 --page-token TOKEN -o json
  feishu-cli doc history list https://xxx.feishu.cn/wiki/WIKI_TOKEN --page-all -o json`,
	Args: cobra.ExactArgs(1),
	RunE: runDocHistoryList,
}

var docHistoryRevertCmd = &cobra.Command{
	Use:   "revert <document_id|url>",
	Short: "回滚文档到指定历史版本",
	Long: `把文档回滚到 doc history list 返回的某个 history_version_id
（POST /open-apis/docs_ai/v1/documents/{id}/history/revert）。

回滚会以历史版本内容替换当前正文（当前内容仍保留在历史中，可再次回滚）。
非交互环境必须加 --yes；--dry-run 只打印将发出的请求。
返回 running 时用 doc history revert-status 查询；只有 done 表示成功，
partial_failed / failed 以非零退出码结束并给出 failed_block_tokens。

示例:
  feishu-cli doc history revert DOC_ID --history-version-id 11 --yes
  feishu-cli doc history revert DOC_ID --history-version-id 11 --wait-timeout-ms 0 --yes   # 只发起，不等待
  feishu-cli doc history revert DOC_ID --history-version-id 11 --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runDocHistoryRevert,
}

var docHistoryRevertStatusCmd = &cobra.Command{
	Use:   "revert-status <document_id|url>",
	Short: "查询回滚任务状态",
	Long: `查询 doc history revert 返回的回滚任务状态
（GET /open-apis/docs_ai/v1/documents/{id}/history/revert_status）。

status 为 partial_failed / failed 时以非零退出码结束，并输出 failed_block_tokens。

示例:
  feishu-cli doc history revert-status DOC_ID --task-id task_xxx`,
	Args: cobra.ExactArgs(1),
	RunE: runDocHistoryRevertStatus,
}

func init() {
	docCmd.AddCommand(docHistoryCmd)
	docHistoryCmd.AddCommand(docHistoryListCmd, docHistoryRevertCmd, docHistoryRevertStatusCmd)

	docHistoryListCmd.Flags().Int("page-size", 20, "每页条数（1-20）")
	docHistoryListCmd.Flags().String("page-token", "", "上一页返回的 page_token")
	docHistoryListCmd.Flags().Bool("page-all", false, "自动翻页直到 has_more=false（最多 50 页）")
	docHistoryListCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	docHistoryListCmd.Flags().String("user-access-token", "", "User Access Token（可选）")

	docHistoryRevertCmd.Flags().String("history-version-id", "", "要回滚到的 history_version_id（doc history list 返回，正整数）")
	docHistoryRevertCmd.Flags().Int("wait-timeout-ms", 30000, "等待回滚完成的毫秒数（0-30000，0 表示只发起不等待）")
	docHistoryRevertCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求，不执行")
	docHistoryRevertCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	docHistoryRevertCmd.Flags().String("user-access-token", "", "User Access Token（可选）")
	mustMarkFlagRequired(docHistoryRevertCmd, "history-version-id")

	docHistoryRevertStatusCmd.Flags().String("task-id", "", "doc history revert 返回的 task_id")
	docHistoryRevertStatusCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	docHistoryRevertStatusCmd.Flags().String("user-access-token", "", "User Access Token（可选）")
	mustMarkFlagRequired(docHistoryRevertStatusCmd, "task-id")
}

func historyPath(documentID, suffix string) string {
	return fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s/%s", url.PathEscape(documentID), suffix)
}

func historyOutput(cmd *cobra.Command) (string, error) {
	output, _ := cmd.Flags().GetString("output")
	output = strings.ToLower(strings.TrimSpace(output))
	if output != "" && output != "json" {
		return "", clierr.Usagef("不支持的 --output %q，仅支持 json", output)
	}
	return output, nil
}

func runDocHistoryList(cmd *cobra.Command, args []string) error {
	if err := config.Validate(); err != nil {
		return err
	}
	pageSize, _ := cmd.Flags().GetInt("page-size")
	pageToken, _ := cmd.Flags().GetString("page-token")
	pageAll, _ := cmd.Flags().GetBool("page-all")
	output, err := historyOutput(cmd)
	if err != nil {
		return err
	}
	if pageSize < 1 || pageSize > 20 {
		return clierr.Usagef("--page-size 必须在 1-20 之间，当前: %d", pageSize)
	}
	userAccessToken := resolveOptionalUserTokenWithFallback(cmd)
	documentID, err := resolveDocxArg(args[0], "<document_id|url>", userAccessToken)
	if err != nil {
		return err
	}

	var entries []any
	hasMore := false
	nextToken := strings.TrimSpace(pageToken)
	seen := map[string]bool{}
	const maxPages = 50
	for page := 0; ; page++ {
		params := map[string]any{"page_size": pageSize}
		if nextToken != "" {
			params["page_token"] = nextToken
		}
		data, err := client.DocsAIRequest("GET", historyPath(documentID, "histories"), params, nil, "查询文档历史版本", userAccessToken)
		if err != nil {
			return err
		}
		if list, ok := data["entries"].([]any); ok {
			entries = append(entries, list...)
		}
		hasMore, _ = data["has_more"].(bool)
		nextToken, _ = data["page_token"].(string)
		if !pageAll || !hasMore || nextToken == "" {
			break
		}
		if seen[nextToken] {
			return fmt.Errorf("历史版本翻页返回了重复的 page_token，已停止以免死循环")
		}
		seen[nextToken] = true
		if page+1 >= maxPages {
			fmt.Fprintf(cmd.ErrOrStderr(), "⚠ 已达到 %d 页上限，仍有更多历史版本；可用 --page-token %s 继续\n", maxPages, nextToken)
			break
		}
	}
	if hasMore && nextToken != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "提示: 还有更多历史版本，使用 --page-token %s 继续，或加 --page-all\n", nextToken)
	}
	if entries == nil {
		entries = []any{}
	}
	if output == "json" {
		out := map[string]any{"document_id": documentID, "entries": entries, "has_more": hasMore}
		if hasMore {
			out["page_token"] = nextToken
		}
		return printJSON(out)
	}
	if len(entries) == 0 {
		fmt.Println("（无历史版本）")
		return nil
	}
	fmt.Printf("%-20s %-10s %-22s %s\n", "history_version_id", "revision", "edit_time", "name")
	for _, raw := range entries {
		e, _ := raw.(map[string]any)
		name, _ := e["name"].(string)
		fmt.Printf("%-20v %-10v %-22v %s\n", e["history_version_id"], jsonNumberText(e["revision_id"]), e["edit_time"], name)
	}
	return nil
}

func jsonNumberText(v any) string {
	switch n := v.(type) {
	case float64:
		return strconv.FormatInt(int64(n), 10)
	case json.Number:
		return n.String()
	case nil:
		return ""
	}
	return fmt.Sprint(v)
}

func runDocHistoryRevert(cmd *cobra.Command, args []string) error {
	if err := config.Validate(); err != nil {
		return err
	}
	versionID, _ := cmd.Flags().GetString("history-version-id")
	versionID = strings.TrimSpace(versionID)
	waitMs, _ := cmd.Flags().GetInt("wait-timeout-ms")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	output, err := historyOutput(cmd)
	if err != nil {
		return err
	}
	if v, perr := strconv.ParseInt(versionID, 10, 64); perr != nil || v <= 0 {
		return clierr.Usagef("--history-version-id 必须是 doc history list 返回的正整数，当前: %q（不要传 revision_id）", versionID)
	}
	if waitMs < 0 || waitMs > 30000 {
		return clierr.Usagef("--wait-timeout-ms 必须在 0-30000 之间，当前: %d", waitMs)
	}
	body := map[string]any{"history_version_id": versionID, "wait_timeout_ms": waitMs}
	userAccessToken := resolveOptionalUserToken(cmd)

	if dryRun {
		// dry-run 不发任何请求：wiki URL 只做离线解析
		ref, err := parseResourceArg(args[0], resourceArgOptions{ArgName: "<document_id|url>", DefaultType: "docx", Allowed: []string{"docx", "wiki"}})
		if err != nil {
			return err
		}
		docID := ref.Token
		if ref.Type == "wiki" {
			docID = "<wiki 解析后的 docx token>"
		}
		return printJSON(map[string]any{
			"dry_run": true,
			"method":  "POST",
			"path":    historyPath(docID, "history/revert"),
			"body":    body,
		})
	}
	documentID, err := resolveDocxArg(args[0], "<document_id|url>", userAccessToken)
	if err != nil {
		return err
	}
	if err := confirmDangerousAction(cmd, fmt.Sprintf("将把文档 %s 回滚到历史版本 %s（当前内容仍保留在历史中）", documentID, versionID)); err != nil {
		return err
	}
	data, err := client.DocsAIRequest("POST", historyPath(documentID, "history/revert"), nil, body, "回滚文档历史版本", userAccessToken)
	if err != nil {
		return err
	}
	return reportHistoryRevert(cmd, output, documentID, data)
}

func runDocHistoryRevertStatus(cmd *cobra.Command, args []string) error {
	if err := config.Validate(); err != nil {
		return err
	}
	taskID, _ := cmd.Flags().GetString("task-id")
	taskID = strings.TrimSpace(taskID)
	output, err := historyOutput(cmd)
	if err != nil {
		return err
	}
	if taskID == "" {
		return clierr.Usagef("--task-id 不能为空")
	}
	userAccessToken := resolveOptionalUserTokenWithFallback(cmd)
	documentID, err := resolveDocxArg(args[0], "<document_id|url>", userAccessToken)
	if err != nil {
		return err
	}
	data, err := client.DocsAIRequest("GET", historyPath(documentID, "history/revert_status"), map[string]any{"task_id": taskID}, nil, "查询回滚状态", userAccessToken)
	if err != nil {
		return err
	}
	return reportHistoryRevert(cmd, output, documentID, data)
}

// reportHistoryRevert 输出回滚结果；partial_failed / failed 非零退出。
func reportHistoryRevert(cmd *cobra.Command, output, documentID string, data map[string]any) error {
	status, _ := data["status"].(string)
	if output == "json" {
		if err := printJSON(data); err != nil {
			return err
		}
	} else {
		fmt.Printf("回滚状态: %s\n", status)
		if v := data["history_version_id"]; v != nil {
			fmt.Printf("  history_version_id: %v\n", v)
		}
		if v, _ := data["task_id"].(string); v != "" {
			fmt.Printf("  task_id: %s\n", v)
		}
	}
	switch strings.ToLower(status) {
	case "done":
		return nil
	case "running", "":
		if taskID, _ := data["task_id"].(string); taskID != "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "提示: 回滚仍在进行，稍后用 `feishu-cli doc history revert-status %s --task-id %s` 查询\n", documentID, taskID)
		}
		return nil
	default:
		failed, _ := json.Marshal(data["failed_block_tokens"])
		return fmt.Errorf("回滚未完全成功（status=%s），failed_block_tokens=%s；请用 doc read --with-ids 核对当前内容", status, failed)
	}
}
