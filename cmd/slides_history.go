package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// slides history：演示文稿历史版本与回滚（slides_ai，对齐官方 slides +history-list / +history-revert /
// +history-revert-status）。结构与 doc history 一致；回滚接口异步执行，任务创建后只轮询状态、绝不重放。

const (
	// slidesRevertDefaultPoll poll_after_ms 缺失、为 0 或非法时的默认轮询间隔（官方 skill 约定 10s）。
	slidesRevertDefaultPoll = 10 * time.Second
	// slidesRevertMaxStatusRetries 状态查询连续失败的最大重试次数（只重试状态查询，不重新发起回滚）。
	slidesRevertMaxStatusRetries = 3
)

var (
	// slidesRevertWaitLimit --wait 的整体轮询上限（5 分钟）；测试中可调小。
	slidesRevertWaitLimit = 5 * time.Minute
	// slidesRevertSleep 轮询等待；测试中替换为不真正 sleep 的实现。
	slidesRevertSleep = time.Sleep
)

var slidesHistoryCmd = &cobra.Command{
	Use:   "history",
	Short: "演示文稿历史版本：列出 / 回滚 / 查询回滚状态",
	Long: `查看 Slides 演示文稿历史版本、按 history_version_id 回滚，以及查询回滚任务状态（slides_ai 接口）。

子命令:
  list            列出历史版本（每页 1-20 条，has_more 时用 --page-token 翻页）
  revert          回滚到指定 history_version_id（写操作，非交互环境需 --yes；支持 --dry-run、--wait）
  revert-status   查询回滚任务状态（running / done / partial_failed / failed）

安全流程:
  1. 用 list 找到目标版本的 history_version_id。revision_id 不唯一，不要把 revision_id 当作
     history_version_id；同一 revision_id 命中多条时按 edit_time（UTC RFC3339）挑选，无法区分时先向用户确认。
  2. revert 立即返回 task_id，回滚在服务端异步执行。任务创建后不要因为状态查询失败而重新发起 revert。
  3. status 为 running 时按 poll_after_ms 等待后用 revert-status 查询（或直接给 revert 加 --wait）。
  4. done 后用 slides get 读取演示文稿确认内容；partial_failed / failed 时查看 failed_block_tokens。

示例:
  feishu-cli slides history list <xml_presentation_id>
  feishu-cli slides history revert <xml_presentation_id> --history-version-id 11 --wait --yes
  feishu-cli slides history revert-status <xml_presentation_id> --task-id task_xxx`,
}

var slidesHistoryListCmd = &cobra.Command{
	Use:   "list <presentation>",
	Short: "列出演示文稿历史版本",
	Long: `列出 Slides 演示文稿历史版本（GET /open-apis/slides_ai/v1/xml_presentations/{id}/histories）。

<presentation> 可以是 xml_presentation_id、/slides/ URL 或 /wiki/ URL（wiki 自动解析为底层演示文稿）。
每条记录含 revision_id、history_version_id、edit_time（UTC RFC3339）、type、name、description、editor_ids。
输出为接口原始 data（JSON）；has_more=true 时 stderr 提示用 --page-token 继续。

身份：已登录时优先 User Token，未登录回退 App Token（与 slides get 一致）。

示例:
  feishu-cli slides history list <xml_presentation_id>
  feishu-cli slides history list <xml_presentation_id> --page-size 5 --page-token TOKEN
  feishu-cli slides history list https://xxx.feishu.cn/wiki/WIKI_TOKEN --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runSlidesHistoryList,
}

var slidesHistoryRevertCmd = &cobra.Command{
	Use:   "revert <presentation>",
	Short: "回滚演示文稿到指定历史版本",
	Long: `把演示文稿回滚到 slides history list 返回的某个 history_version_id
（POST /open-apis/slides_ai/v1/xml_presentations/{id}/history/revert）。

回滚以历史版本内容替换当前演示文稿（当前内容仍保留在历史中，可再次回滚）。
非交互环境必须加 --yes；--dry-run 只打印将发出的请求（不联网、不需要凭据）。

--wait：任务返回 running 时按响应中的 poll_after_ms（缺省 10 秒）轮询 revert-status，整体最多 5 分钟。
  轮询只调用状态查询接口，绝不重新发起回滚；状态查询连续失败超过 3 次时停止并给出 task_id。
  5 分钟后仍是 running 时以退出码 0 结束并提示稍后用 revert-status 查询（不代表回滚失败）。
done 退出码 0；partial_failed / failed 输出 failed_block_tokens 并以非零退出码结束。

身份：默认 Bot（不传 --as 时显式 --user-access-token / FEISHU_USER_ACCESS_TOKEN 仍生效）；
操作自己的演示文稿用 --as user。

示例:
  feishu-cli slides history revert <xml_presentation_id> --history-version-id 11 --yes
  feishu-cli slides history revert <xml_presentation_id> --history-version-id 11 --wait --as user --yes
  feishu-cli slides history revert <xml_presentation_id> --history-version-id 11 --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runSlidesHistoryRevert,
}

var slidesHistoryRevertStatusCmd = &cobra.Command{
	Use:   "revert-status <presentation>",
	Short: "查询演示文稿回滚任务状态",
	Long: `查询 slides history revert 返回的回滚任务状态
（GET /open-apis/slides_ai/v1/xml_presentations/{id}/history/revert_status）。

status 为 partial_failed / failed 时以非零退出码结束，并输出 failed_block_tokens。
身份默认与 revert 相同（Bot）；revert 用了 --as user 时这里也传 --as user。

示例:
  feishu-cli slides history revert-status <xml_presentation_id> --task-id task_xxx
  feishu-cli slides history revert-status <xml_presentation_id> --task-id task_xxx --as user`,
	Args: cobra.ExactArgs(1),
	RunE: runSlidesHistoryRevertStatus,
}

func init() {
	slidesCmd.AddCommand(slidesHistoryCmd)
	slidesHistoryCmd.AddCommand(slidesHistoryListCmd, slidesHistoryRevertCmd, slidesHistoryRevertStatusCmd)

	slidesHistoryListCmd.Flags().Int("page-size", 20, "每页条数（1-20）")
	slidesHistoryListCmd.Flags().String("page-token", "", "上一页返回的 page_token")
	slidesHistoryListCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求，不联网、不需要凭据")
	slidesHistoryListCmd.Flags().String("user-access-token", "", "User Access Token（可选）")

	slidesHistoryRevertCmd.Flags().String("history-version-id", "", "要回滚到的 history_version_id（slides history list 返回，正整数）")
	slidesHistoryRevertCmd.Flags().Bool("wait", false, "running 时按 poll_after_ms 轮询状态直到结束（最多 5 分钟，只轮询不重放）")
	slidesHistoryRevertCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求，不联网、不需要凭据")
	slidesHistoryRevertCmd.Flags().String("user-access-token", "", "User Access Token（可选）")
	addVisualWriteAsFlag(slidesHistoryRevertCmd)
	mustMarkFlagRequired(slidesHistoryRevertCmd, "history-version-id")

	slidesHistoryRevertStatusCmd.Flags().String("task-id", "", "slides history revert 返回的 task_id")
	slidesHistoryRevertStatusCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求，不联网、不需要凭据")
	slidesHistoryRevertStatusCmd.Flags().String("user-access-token", "", "User Access Token（可选）")
	addVisualWriteAsFlag(slidesHistoryRevertStatusCmd)
	mustMarkFlagRequired(slidesHistoryRevertStatusCmd, "task-id")
}

// printSlidesHistoryDryRun 静态预览：wiki 链接不联网，用占位符表示解析步骤。
func printSlidesHistoryDryRun(cmd *cobra.Command, raw, method, suffix string, params, body map[string]any) error {
	pres, wikiStep, err := slidesDryRunPresentation(raw)
	if err != nil {
		return err
	}
	var steps []map[string]any
	if wikiStep != nil {
		steps = append(steps, wikiStep)
	}
	step := map[string]any{"method": method, "path": client.SlidesPresentationPath(pres, suffix)}
	if params != nil {
		step["params"] = params
	}
	if body != nil {
		step["body"] = body
	}
	steps = append(steps, step)
	return printJSON(withVisualDryRunIdentity(cmd, map[string]any{"dry_run": true, "xml_presentation_id": pres, "steps": steps}))
}

func runSlidesHistoryList(cmd *cobra.Command, args []string) error {
	pageSize, _ := cmd.Flags().GetInt("page-size")
	pageToken := strings.TrimSpace(flagString(cmd, "page-token"))
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	if pageSize < 1 || pageSize > 20 {
		return clierr.Usagef("--page-size 必须在 1-20 之间，当前: %d", pageSize)
	}
	if _, _, err := parsePresentationArgOffline(args[0]); err != nil {
		return err
	}
	params := map[string]any{"page_size": pageSize}
	if pageToken != "" {
		params["page_token"] = pageToken
	}
	if dryRun {
		return printSlidesHistoryDryRun(cmd, args[0], "GET", "/histories", params, nil)
	}

	if err := config.Validate(); err != nil {
		return err
	}
	userAccessToken := resolveOptionalUserTokenWithFallback(cmd)
	presentationID, err := resolvePresentationArg(args[0], userAccessToken)
	if err != nil {
		return err
	}
	data, err := client.SlidesCall("查询演示文稿历史版本", "GET", client.SlidesPresentationPath(presentationID, "/histories"), params, nil, userAccessToken)
	if err != nil {
		return err
	}
	if data == nil {
		data = map[string]any{}
	}
	if _, ok := data["entries"]; !ok {
		data["entries"] = []any{}
	}
	if hasMore, _ := data["has_more"].(bool); hasMore {
		if next, _ := data["page_token"].(string); next != "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "提示: 还有更多历史版本，使用 --page-token %s 继续\n", next)
		}
	}
	return printJSON(data)
}

// validateSlidesHistoryVersionID 与官方一致：必须是正整数字符串（history_version_id，不是 revision_id）。
func validateSlidesHistoryVersionID(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if n, err := strconv.ParseInt(v, 10, 64); err != nil || n <= 0 {
		return "", clierr.Usagef("--history-version-id 必须是 slides history list 返回的正整数，当前: %q（不要传 revision_id）", raw)
	}
	return v, nil
}

func runSlidesHistoryRevert(cmd *cobra.Command, args []string) error {
	versionID, err := validateSlidesHistoryVersionID(flagString(cmd, "history-version-id"))
	if err != nil {
		return err
	}
	wait, _ := cmd.Flags().GetBool("wait")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	if _, _, err := parsePresentationArgOffline(args[0]); err != nil {
		return err
	}
	if err := validateIdentityAs(cmd); err != nil {
		return err
	}
	body := map[string]any{"history_version_id": versionID}
	if dryRun {
		return printSlidesHistoryDryRun(cmd, args[0], "POST", "/history/revert", nil, body)
	}

	if err := config.Validate(); err != nil {
		return err
	}
	userAccessToken, err := resolveVisualWriteToken(cmd)
	if err != nil {
		return err
	}
	if err := confirmDangerousAction(cmd, fmt.Sprintf("将把演示文稿 %s 回滚到历史版本 %s（当前内容仍保留在历史中）", args[0], versionID)); err != nil {
		return err
	}
	presentationID, err := resolvePresentationArg(args[0], userAccessToken)
	if err != nil {
		return err
	}
	data, err := client.SlidesCall("回滚演示文稿历史版本", "POST", client.SlidesPresentationPath(presentationID, "/history/revert"), nil, body, userAccessToken)
	if err != nil {
		return enrichSlidesWriteError(err, "")
	}
	if data == nil {
		data = map[string]any{}
	}
	if wait {
		data = waitSlidesRevert(cmd, presentationID, data, userAccessToken)
	}
	return reportSlidesRevert(cmd, args[0], data)
}

// waitSlidesRevert 在 status=running 时按 poll_after_ms 轮询状态接口，直到终态、超过 5 分钟或状态查询连续失败。
// 只调用 revert_status，绝不重新发起 revert。返回值保留 task_id，并在未得到终态时标注原因。
func waitSlidesRevert(cmd *cobra.Command, presentationID string, data map[string]any, userAccessToken string) map[string]any {
	taskID, _ := data["task_id"].(string)
	if taskID == "" || !strings.EqualFold(slidesRevertStatus(data), "running") {
		return data
	}
	// 已耗时 = 累计等待间隔 + 状态查询本身的耗时（不直接用墙钟，便于测试替换 sleep）
	var elapsed time.Duration
	failures := 0
	current := data
	for {
		interval := slidesRevertPollInterval(current)
		if elapsed+interval > slidesRevertWaitLimit {
			current["wait_timed_out"] = true
			return current
		}
		slidesRevertSleep(interval)
		callStart := time.Now()
		status, err := client.SlidesCall("查询演示文稿回滚状态", "GET", client.SlidesPresentationPath(presentationID, "/history/revert_status"),
			map[string]any{"task_id": taskID}, nil, userAccessToken)
		elapsed += interval + time.Since(callStart)
		if err != nil {
			failures++
			fmt.Fprintf(cmd.ErrOrStderr(), "⚠ 查询回滚状态失败（第 %d 次，任务仍在服务端执行，不会重新发起回滚）: %v\n", failures, err)
			if failures > slidesRevertMaxStatusRetries {
				current["wait_error"] = err.Error()
				return current
			}
			continue
		}
		failures = 0
		if status == nil {
			status = map[string]any{}
		}
		if _, ok := status["task_id"]; !ok {
			status["task_id"] = taskID
		}
		// 状态响应可能不带 poll_after_ms：沿用上一次的间隔
		if _, ok := status["poll_after_ms"]; !ok {
			if v, ok := current["poll_after_ms"]; ok {
				status["poll_after_ms"] = v
			}
		}
		current = status
		if !strings.EqualFold(slidesRevertStatus(current), "running") {
			return current
		}
	}
}

func slidesRevertStatus(data map[string]any) string {
	s, _ := data["status"].(string)
	return strings.TrimSpace(s)
}

// slidesRevertPollInterval 读取 poll_after_ms；缺失、为 0 或非法时默认 10 秒。
func slidesRevertPollInterval(data map[string]any) time.Duration {
	ms, ok := slidesNumber(data["poll_after_ms"])
	if !ok || ms <= 0 {
		return slidesRevertDefaultPoll
	}
	return time.Duration(ms) * time.Millisecond
}

// reportSlidesRevert 输出回滚结果（接口原始 data）；partial_failed / failed 非零退出。
func reportSlidesRevert(cmd *cobra.Command, presentationArg string, data map[string]any) error {
	if err := printJSON(data); err != nil {
		return err
	}
	status := strings.ToLower(slidesRevertStatus(data))
	taskID, _ := data["task_id"].(string)
	switch status {
	case "done":
		return nil
	case "running", "":
		if taskID != "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "提示: 回滚仍在进行（不代表失败），稍后用 `feishu-cli slides history revert-status %s --task-id %s` 查询；不要重新发起 revert\n", presentationArg, taskID)
		}
		if msg, _ := data["wait_error"].(string); msg != "" {
			return fmt.Errorf("回滚任务已创建（task_id=%s），但状态查询连续失败，已停止轮询: %s", taskID, msg)
		}
		return nil
	default:
		failed, _ := json.Marshal(data["failed_block_tokens"])
		return fmt.Errorf("回滚未完全成功（status=%s），failed_block_tokens=%s；请用 slides get 核对当前内容，确认后再决定是否重新回滚", status, failed)
	}
}

func runSlidesHistoryRevertStatus(cmd *cobra.Command, args []string) error {
	taskID := strings.TrimSpace(flagString(cmd, "task-id"))
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	if taskID == "" {
		return clierr.Usagef("--task-id 不能为空")
	}
	if _, _, err := parsePresentationArgOffline(args[0]); err != nil {
		return err
	}
	if err := validateIdentityAs(cmd); err != nil {
		return err
	}
	params := map[string]any{"task_id": taskID}
	if dryRun {
		return printSlidesHistoryDryRun(cmd, args[0], "GET", "/history/revert_status", params, nil)
	}

	if err := config.Validate(); err != nil {
		return err
	}
	userAccessToken, err := resolveVisualWriteToken(cmd)
	if err != nil {
		return err
	}
	presentationID, err := resolvePresentationArg(args[0], userAccessToken)
	if err != nil {
		return err
	}
	data, err := client.SlidesCall("查询演示文稿回滚状态", "GET", client.SlidesPresentationPath(presentationID, "/history/revert_status"), params, nil, userAccessToken)
	if err != nil {
		return err
	}
	if data == nil {
		data = map[string]any{}
	}
	if _, ok := data["task_id"]; !ok {
		data["task_id"] = taskID
	}
	return reportSlidesRevert(cmd, args[0], data)
}
