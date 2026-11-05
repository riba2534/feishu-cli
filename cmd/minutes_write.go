package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// 妙记写命令（对齐官方 minutes +upload/+update/+summary/+todo/+speaker-replace/+word-replace）。
// 官方 6 个命令的 Risk 均为 "write"（非 high-risk-write），因此不加确认门禁；全部仅 User 身份。

const minutesASRQuotaHint = "提示：ASR/AI 配额不足，无法完成本次操作；请在妙记详情页查看配额情况后再试"

// minutesEditPermissionHint 返回无编辑权限（2091005）时的申请命令提示。
func minutesEditPermissionHint(minuteToken string) string {
	return fmt.Sprintf("提示：当前用户没有该妙记的编辑权限；征得同意后可申请：feishu-cli minutes apply-permission --minute-token %s --perm edit", minuteToken)
}

// wrapMinutesWriteError 为妙记写接口的典型业务错误追加中文提示（保留错误链）。
func wrapMinutesWriteError(err error, minuteToken string) error {
	switch {
	case err == nil:
		return nil
	case client.HasAPICode(err, client.MinutesCodeASRQuotaNotEnough):
		return fmt.Errorf("%w\n%s", err, minutesASRQuotaHint)
	case minuteToken != "" && client.HasAPICode(err, client.MinutesCodeNoEditPermission):
		return fmt.Errorf("%w\n%s", err, minutesEditPermissionHint(minuteToken))
	case client.HasAPICode(err, client.MinutesCodeOthersEditing):
		return fmt.Errorf("%w\n提示：文字稿正被他人编辑，请等对方结束后重试", err)
	case client.HasAPICode(err, client.MinutesCodeWordsNotFound):
		return fmt.Errorf("%w\n提示：所有 source_word 都未在文字稿中找到，未做任何替换；请核对拼写、大小写与空格后只重试这些词", err)
	case client.HasAPICode(err, client.MinutesCodeSpeakerNotFound):
		return fmt.Errorf("%w\n提示：--from-speaker-id 与文字稿中的说话人不匹配；请先读取逐字稿确认 speaker_id", err)
	}
	return err
}

// minuteTokenArg 读取位置参数 <minute_token|妙记链接>。
func minuteTokenArg(args []string) (string, error) {
	if len(args) != 1 {
		return "", clierr.Usagef("需要 1 个参数 <minute_token|妙记链接>")
	}
	return normalizeMinuteTokenInput(args[0])
}

// runMinutesWrite 统一执行顺序：参数已校验 → --dry-run → config.Validate → 必需 User 身份 → 确认门禁 → 调用。
// confirm 非空时为覆盖 / 删除类写入，执行前走 confirmDangerousAction（非交互未 --yes 时 exit 10）。
func runMinutesWrite(cmd *cobra.Command, cmdName, desc, method, path string, body map[string]any, confirm string, call func(token string) (json.RawMessage, error), onSuccess func(data json.RawMessage) error) error {
	if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
		return printDryRunPlan(cmd, desc, map[string]any{"as": "user"}, []dryRunStep{{Method: method, URL: path, Body: body}})
	}
	if err := config.Validate(); err != nil {
		return err
	}
	token, err := requireUserToken(cmd, cmdName)
	if err != nil {
		return err
	}
	if confirm != "" {
		if err := confirmDangerousAction(cmd, confirm); err != nil {
			return err
		}
	}
	data, err := call(token)
	if err != nil {
		return err
	}
	return onSuccess(data)
}

func addMinutesWriteFlags(c *cobra.Command) {
	c.Flags().Bool("dry-run", false, "只打印将要发出的请求（不读取凭据、不联网）")
	c.Flags().StringP("output", "o", "", "输出格式（json）")
	c.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
}

// printMinutesWriteResult 按 -o 输出结果：json 输出对象，否则逐行打印。
func printMinutesWriteResult(cmd *cobra.Command, title string, out map[string]any, keys ...string) error {
	if output, _ := cmd.Flags().GetString("output"); output == "json" {
		return printJSON(out)
	}
	fmt.Println(title)
	for _, k := range keys {
		if v, ok := out[k]; ok {
			fmt.Printf("  %s: %v\n", k, v)
		}
	}
	return nil
}

// ---------- minutes upload ----------

var minutesUploadCmd = &cobra.Command{
	Use:   "upload",
	Short: "由云盘音视频文件生成妙记",
	Long: `用已上传到云盘的音视频文件（file_token）生成妙记。

使用飞书 POST /open-apis/minutes/v1/minutes/upload API。只接受 --file-token：
本地文件请先用 feishu-cli drive upload 上传到云盘。

支持的格式: 音频 wav/mp3/m4a/aac/ogg/wma/amr；视频 avi/wmv/mov/mp4/m4v/mpeg/ogg/flv。
原始文件不超过 6GB、时长不超过 6 小时。ASR/AI 配额不足（2091008）时给出单独提示。

身份: 仅 User（需先 feishu-cli auth login；scope minutes:minutes.upload:write）

输出: minute_url 与从中解析出的 minute_token

示例:
  feishu-cli minutes upload --file-token boxcnxxxx
  feishu-cli minutes upload --file-token boxcnxxxx --dry-run`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		fileToken, _ := cmd.Flags().GetString("file-token")
		fileToken = strings.TrimSpace(fileToken)
		if fileToken == "" {
			return clierr.Usagef("--file-token 必填")
		}
		if err := validateResourceIdentifier(fileToken, "--file-token"); err != nil {
			return clierr.Usage(err)
		}
		body := map[string]any{"file_token": fileToken}
		return runMinutesWrite(cmd, "minutes upload", "Generate a minute from a Drive media file", "POST", client.MinutesUploadPath, body, "",
			func(token string) (json.RawMessage, error) {
				data, err := client.UploadMinute(fileToken, token)
				return data, wrapMinutesWriteError(err, "")
			},
			func(data json.RawMessage) error {
				var parsed struct {
					MinuteURL string `json:"minute_url"`
				}
				_ = json.Unmarshal(data, &parsed)
				out := map[string]any{"minute_url": parsed.MinuteURL}
				if t := extractUploadedMinuteToken(parsed.MinuteURL); t != "" {
					out["minute_token"] = t
				}
				return printMinutesWriteResult(cmd, "已提交妙记生成:", out, "minute_url", "minute_token")
			})
	},
}

// extractUploadedMinuteToken 从 minute_url（.../minutes/<token>）中解析 minute_token；解析不到返回空。
func extractUploadedMinuteToken(minuteURL string) string {
	u, err := url.Parse(strings.TrimSpace(minuteURL))
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.TrimRight(u.Path, "/"), "/")
	for i, part := range parts {
		if part == "minutes" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// ---------- minutes update ----------

var minutesUpdateCmd = &cobra.Command{
	Use:   "update <minute_token|妙记链接>",
	Short: "修改妙记标题",
	Long: `修改妙记标题。

使用飞书 PATCH /open-apis/minutes/v1/minutes/{minute_token} API。
身份: 仅 User（scope minutes:minutes:update）；无编辑权限（2091005）时提示用 minutes apply-permission 申请。

示例:
  feishu-cli minutes update obcnxxxx --topic "周会纪要"
  feishu-cli minutes update https://example.feishu.cn/minutes/obcnxxxx --topic "周会纪要" --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		minuteToken, err := minuteTokenArg(args)
		if err != nil {
			return err
		}
		topic, _ := cmd.Flags().GetString("topic")
		if strings.TrimSpace(topic) == "" {
			return clierr.Usagef("--topic 必填")
		}
		return runMinutesWrite(cmd, "minutes update", "Update minute title", "PATCH", client.MinutePath(minuteToken, ""), map[string]any{"topic": topic}, "",
			func(token string) (json.RawMessage, error) {
				data, err := client.UpdateMinuteTopic(minuteToken, topic, token)
				return data, wrapMinutesWriteError(err, minuteToken)
			},
			func(json.RawMessage) error {
				return printMinutesWriteResult(cmd, "妙记标题已更新:", map[string]any{"minute_token": minuteToken, "topic": topic}, "minute_token", "topic")
			})
	},
}

// ---------- minutes summary ----------

var minutesSummaryCmd = &cobra.Command{
	Use:   "summary <minute_token|妙记链接>",
	Short: "替换妙记 AI 摘要",
	Long: `用给定文本替换妙记的 AI 摘要（整体覆盖）。

使用飞书 PUT /open-apis/minutes/v1/minutes/{minute_token}/summary API。
--summary 支持直接文本、@file（读取本地文件）或 -（读取标准输入）；@@ 开头表示字面量 @。
摘要接受任意文本，飞书妙记对纯文本、换行、标题（# / ## / ###）、粗体（**x**）和列表（- / * / 1.）渲染最好。
替换前可先用 feishu-cli minutes get <token> --summary 读取当前摘要。

身份: 仅 User（scope minutes:minutes:update）

示例:
  feishu-cli minutes summary obcnxxxx --summary @summary.md
  cat summary.md | feishu-cli minutes summary obcnxxxx --summary -
  feishu-cli minutes summary obcnxxxx --summary "## 结论\n- 本周上线" --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		minuteToken, err := minuteTokenArg(args)
		if err != nil {
			return err
		}
		raw, _ := cmd.Flags().GetString("summary")
		summary, err := resolveSlidesInputValue(raw, "--summary")
		if err != nil {
			return err
		}
		summary = strings.TrimSpace(summary)
		if summary == "" {
			return clierr.Usagef("--summary 必填且不能为空")
		}
		return runMinutesWrite(cmd, "minutes summary", "Replace minute AI summary", "PUT", client.MinutePath(minuteToken, "/summary"), map[string]any{"summary": summary},
			fmt.Sprintf("确认用新内容整体覆盖妙记 %s 的 AI 摘要？原摘要不可恢复", minuteToken),
			func(token string) (json.RawMessage, error) {
				data, err := client.ReplaceMinuteSummary(minuteToken, summary, token)
				return data, wrapMinutesWriteError(err, minuteToken)
			},
			func(json.RawMessage) error {
				return printMinutesWriteResult(cmd, "妙记摘要已替换:", map[string]any{"minute_token": minuteToken, "updated": true}, "minute_token")
			})
	},
}

// ---------- minutes todo ----------

// minuteTodoDeleteConfirm 待办写入里含 delete 操作时返回确认提示；只有 add / update 时不需要确认。
func minuteTodoDeleteConfirm(minuteToken string, items []map[string]any) string {
	n := 0
	for _, it := range items {
		if op, _ := it["operation"].(string); op == "delete" {
			n++
		}
	}
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("确认删除妙记 %s 的 %d 条待办？此操作不可撤销", minuteToken, n)
}

type minuteTodoSpec struct {
	Operation string `json:"operation"`
	Content   string `json:"content"`
	IsDone    *bool  `json:"is_done"`
	TodoID    string `json:"todo_id"`
}

// buildMinuteTodoItem 校验单条待办并构造 todo_items 元素（对齐官方 buildMinuteTodoItem）。
func buildMinuteTodoItem(spec minuteTodoSpec) (map[string]any, error) {
	op := strings.TrimSpace(spec.Operation)
	content := strings.TrimSpace(spec.Content)
	todoID := strings.TrimSpace(spec.TodoID)
	item := map[string]any{"operation": op}
	switch op {
	case "add":
		if todoID != "" {
			return nil, fmt.Errorf("operation \"add\" 不接受 todo_id（会新建待办）")
		}
		if content == "" || spec.IsDone == nil {
			return nil, fmt.Errorf("operation \"add\" 需要 content 与 is_done")
		}
		item["content"], item["is_done"] = content, *spec.IsDone
	case "update":
		if todoID == "" || content == "" || spec.IsDone == nil {
			return nil, fmt.Errorf("operation \"update\" 需要 todo_id、content 与 is_done")
		}
		item["todo_id"], item["content"], item["is_done"] = todoID, content, *spec.IsDone
	case "delete":
		if todoID == "" {
			return nil, fmt.Errorf("operation \"delete\" 需要 todo_id")
		}
		if content != "" || spec.IsDone != nil {
			return nil, fmt.Errorf("operation \"delete\" 只接受 todo_id（不要带 content / is_done）")
		}
		item["todo_id"] = todoID
	case "":
		return nil, fmt.Errorf("operation 必填（add / update / delete）")
	default:
		return nil, fmt.Errorf("operation %q 无效，可选值: add, update, delete", op)
	}
	return item, nil
}

// resolveMinuteTodoItems 从单条 flag 或 --todos 批量 JSON 构造 todo_items。
func resolveMinuteTodoItems(cmd *cobra.Command) ([]map[string]any, error) {
	rawTodos, _ := cmd.Flags().GetString("todos")
	hasSingle := cmd.Flags().Changed("operation") || cmd.Flags().Changed("todo") || cmd.Flags().Changed("is-done") || cmd.Flags().Changed("todo-id")
	if strings.TrimSpace(rawTodos) != "" {
		if hasSingle {
			return nil, clierr.Usagef("--todos（批量）与单条参数（--operation/--todo/--is-done/--todo-id）只能二选一")
		}
		text, err := resolveSlidesInputValue(rawTodos, "--todos")
		if err != nil {
			return nil, err
		}
		var specs []minuteTodoSpec
		if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &specs); err != nil {
			return nil, clierr.Usagef("--todos 不是合法的 JSON 数组: %v", err)
		}
		if len(specs) == 0 {
			return nil, clierr.Usagef("--todos 至少包含一条待办")
		}
		items := make([]map[string]any, 0, len(specs))
		for i, spec := range specs {
			item, err := buildMinuteTodoItem(spec)
			if err != nil {
				return nil, clierr.Usagef("--todos 第 %d 条: %v", i+1, err)
			}
			items = append(items, item)
		}
		return items, nil
	}
	op, _ := cmd.Flags().GetString("operation")
	if strings.TrimSpace(op) == "" {
		return nil, clierr.Usagef("单条模式需要 --operation（add / update / delete），批量请用 --todos")
	}
	rawTodo, _ := cmd.Flags().GetString("todo")
	todo, err := resolveSlidesInputValue(rawTodo, "--todo")
	if err != nil {
		return nil, err
	}
	todoID, _ := cmd.Flags().GetString("todo-id")
	spec := minuteTodoSpec{Operation: op, Content: todo, TodoID: todoID}
	if cmd.Flags().Changed("is-done") {
		done, _ := cmd.Flags().GetBool("is-done")
		spec.IsDone = &done
	}
	item, err := buildMinuteTodoItem(spec)
	if err != nil {
		return nil, clierr.Usagef("%v（单条参数: --todo / --is-done / --todo-id）", err)
	}
	return []map[string]any{item}, nil
}

var minutesTodoCmd = &cobra.Command{
	Use:   "todo <minute_token|妙记链接>",
	Short: "新增 / 修改 / 删除妙记待办",
	Long: `新增、修改或删除妙记的待办（单条或批量）。

使用飞书 POST /open-apis/minutes/v1/minutes/{minute_token}/todo API（body.todo_items 保持数组顺序）。

单条:
  --operation add    --todo "内容" --is-done=false
  --operation update --todo-id <id> --todo "内容" --is-done
  --operation delete --todo-id <id>
批量（可混合 add/update/delete；支持 @file 与 -）:
  --todos '[{"operation":"add","content":"...","is_done":false},{"operation":"delete","todo_id":"..."}]'

content 只支持纯文本。写之前可用 feishu-cli minutes get <token> --todo 读取现有待办与 todo_id。
身份: 仅 User（scope minutes:minutes:update）

示例:
  feishu-cli minutes todo obcnxxxx --operation add --todo "补充测试报告" --is-done=false
  feishu-cli minutes todo obcnxxxx --todos @todos.json --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		minuteToken, err := minuteTokenArg(args)
		if err != nil {
			return err
		}
		items, err := resolveMinuteTodoItems(cmd)
		if err != nil {
			return err
		}
		return runMinutesWrite(cmd, "minutes todo", "Write minute todo items", "POST", client.MinutePath(minuteToken, "/todo"), map[string]any{"todo_items": items},
			minuteTodoDeleteConfirm(minuteToken, items),
			func(token string) (json.RawMessage, error) {
				data, err := client.WriteMinuteTodos(minuteToken, items, token)
				return data, wrapMinutesWriteError(err, minuteToken)
			},
			func(json.RawMessage) error {
				out := map[string]any{"minute_token": minuteToken, "count": len(items), "updated": true}
				if len(items) == 1 {
					out["operation"] = items[0]["operation"]
				}
				return printMinutesWriteResult(cmd, "妙记待办已更新:", out, "minute_token", "count", "operation")
			})
	},
}

// ---------- minutes speaker-replace ----------

var minutesSpeakerReplaceCmd = &cobra.Command{
	Use:   "speaker-replace <minute_token|妙记链接>",
	Short: "替换妙记文字稿的说话人",
	Long: `把妙记文字稿中的某个说话人重新绑定为指定用户。

使用飞书 PUT /open-apis/minutes/v1/minutes/{minute_token}/transcript/speaker?user_id_type=open_id API。
--from-speaker-id 是文字稿说话人列表里的不透明 speaker_id（不要传显示名）；--to-user-id 必须是 ou_ 开头的 open_id。

身份: 仅 User（scope minutes:minutes:readonly + minutes:minutes:update）

示例:
  feishu-cli minutes speaker-replace obcnxxxx --from-speaker-id spk_xxx --to-user-id ou_xxx
  feishu-cli minutes speaker-replace obcnxxxx --from-speaker-id spk_xxx --to-user-id ou_xxx --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		minuteToken, err := minuteTokenArg(args)
		if err != nil {
			return err
		}
		from, _ := cmd.Flags().GetString("from-speaker-id")
		to, _ := cmd.Flags().GetString("to-user-id")
		from, to = strings.TrimSpace(from), strings.TrimSpace(to)
		if from == "" {
			return clierr.Usagef("--from-speaker-id 必填")
		}
		if to == "" {
			return clierr.Usagef("--to-user-id 必填")
		}
		if !strings.HasPrefix(to, "ou_") {
			return clierr.Usagef("--to-user-id 需要 open_id（以 ou_ 开头），得到 %q", to)
		}
		body := map[string]any{"from_speaker_id": from, "to_user_id": to}
		path := client.MinutePath(minuteToken, "/transcript/speaker") + "?user_id_type=open_id"
		return runMinutesWrite(cmd, "minutes speaker-replace", "Replace transcript speaker", "PUT", path, body,
			fmt.Sprintf("确认批量替换妙记 %s 文字稿中的说话人？", minuteToken),
			func(token string) (json.RawMessage, error) {
				data, err := client.ReplaceMinuteSpeaker(minuteToken, body, token)
				return data, wrapMinutesWriteError(err, minuteToken)
			},
			func(json.RawMessage) error {
				return printMinutesWriteResult(cmd, "妙记说话人已替换:", map[string]any{"minute_token": minuteToken, "from_speaker_id": from, "to_user_id": to}, "minute_token", "from_speaker_id", "to_user_id")
			})
	},
}

// ---------- minutes word-replace ----------

// parseMinuteReplaceWords 解析 --replace-words：JSON 数组，source_word 去首尾空白后非空且不重复（对齐官方 parseReplaceWords）。
func parseMinuteReplaceWords(raw string) ([]map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, clierr.Usagef("--replace-words 必填")
	}
	var items []struct {
		SourceWord string `json:"source_word"`
		TargetWord string `json:"target_word"`
	}
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, clierr.Usagef("--replace-words 必须是 {source_word,target_word} 对象组成的 JSON 数组: %v", err)
	}
	if len(items) == 0 {
		return nil, clierr.Usagef("--replace-words 至少包含一条替换")
	}
	out := make([]map[string]string, 0, len(items))
	seen := make(map[string]bool, len(items))
	for i, item := range items {
		source := strings.TrimSpace(item.SourceWord)
		if source == "" {
			return nil, clierr.Usagef("--replace-words 第 %d 条的 source_word 不能为空", i+1)
		}
		if seen[source] {
			return nil, clierr.Usagef("--replace-words 中 source_word %q 重复", source)
		}
		seen[source] = true
		out = append(out, map[string]string{"source_word": source, "target_word": item.TargetWord})
	}
	return out, nil
}

// splitMinuteWordReplaceResults 按 replace_word_counts 把替换词分为成功（次数>0）与未命中两组；
// 服务端未返回 replace_word_counts 时 ok=false。
func splitMinuteWordReplaceResults(words []map[string]string, data json.RawMessage) (succeeded, failed []string, ok bool) {
	var parsed struct {
		Counts []struct {
			SourceWord   string      `json:"source_word"`
			ReplaceCount json.Number `json:"replace_count"`
		} `json:"replace_word_counts"`
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	if err := dec.Decode(&parsed); err != nil || parsed.Counts == nil {
		return nil, nil, false
	}
	counts := make(map[string]int64, len(parsed.Counts))
	for _, c := range parsed.Counts {
		n, _ := c.ReplaceCount.Int64()
		counts[strings.TrimSpace(c.SourceWord)] = n
	}
	succeeded, failed = []string{}, []string{}
	for _, w := range words {
		if counts[w["source_word"]] > 0 {
			succeeded = append(succeeded, w["source_word"])
		} else {
			failed = append(failed, w["source_word"])
		}
	}
	return succeeded, failed, true
}

var minutesWordReplaceCmd = &cobra.Command{
	Use:   "word-replace <minute_token|妙记链接>",
	Short: "批量替换妙记文字稿中的词",
	Long: `批量替换妙记文字稿中的错词。

使用飞书 PUT /open-apis/minutes/v1/minutes/{minute_token}/transcript/word API。
--replace-words 为 JSON 数组（支持 @file 与 -），如 [{"source_word":"飞树","target_word":"飞书"}]；
source_word 不能为空且不能重复（本地校验，exit 2）。
输出按 replace_word_counts 列出成功与未命中的词；全部未命中时报错。不要重复处理已成功的词。

身份: 仅 User（scope minutes:minutes:update）

示例:
  feishu-cli minutes word-replace obcnxxxx --replace-words '[{"source_word":"飞树","target_word":"飞书"}]'
  feishu-cli minutes word-replace obcnxxxx --replace-words @words.json --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		minuteToken, err := minuteTokenArg(args)
		if err != nil {
			return err
		}
		raw, _ := cmd.Flags().GetString("replace-words")
		text, err := resolveSlidesInputValue(raw, "--replace-words")
		if err != nil {
			return err
		}
		words, err := parseMinuteReplaceWords(text)
		if err != nil {
			return err
		}
		body := map[string]any{"minute_token": minuteToken, "replace_words": words}
		return runMinutesWrite(cmd, "minutes word-replace", "Replace words in minute transcript", "PUT", client.MinutePath(minuteToken, "/transcript/word"), body,
			fmt.Sprintf("确认批量替换妙记 %s 文字稿中的 %d 组词语？", minuteToken, len(words)),
			func(token string) (json.RawMessage, error) {
				data, err := client.ReplaceMinuteWords(minuteToken, words, token)
				return data, wrapMinutesWriteError(err, minuteToken)
			},
			func(data json.RawMessage) error {
				succeeded, failed, ok := splitMinuteWordReplaceResults(words, data)
				if !ok {
					return fmt.Errorf("替换请求已成功，但服务端未返回 replace_word_counts，无法逐词确认结果；请先用 feishu-cli minutes get %s --transcript 核对后再决定是否重试（不要重复处理已成功的词）", minuteToken)
				}
				if len(succeeded) == 0 {
					return fmt.Errorf("所有 source_word 都未在妙记 %s 的文字稿中找到，未做任何替换: %s；请核对拼写、大小写与空格后只重试这些词", minuteToken, strings.Join(failed, ", "))
				}
				return printMinutesWriteResult(cmd, "妙记文字稿替换完成（不要重复处理已成功的词）:", map[string]any{
					"minute_token": minuteToken,
					"succeeded":    succeeded,
					"failed":       failed,
				}, "minute_token", "succeeded", "failed")
			})
	},
}

func init() {
	minutesCmd.AddCommand(minutesUploadCmd)
	minutesUploadCmd.Flags().String("file-token", "", "已上传到云盘的音视频 file_token（必填）")
	addMinutesWriteFlags(minutesUploadCmd)

	minutesCmd.AddCommand(minutesUpdateCmd)
	minutesUpdateCmd.Flags().String("topic", "", "新的妙记标题（必填）")
	addMinutesWriteFlags(minutesUpdateCmd)

	minutesCmd.AddCommand(minutesSummaryCmd)
	minutesSummaryCmd.Flags().String("summary", "", "替换后的摘要：文本、@file 或 -（标准输入）（必填）")
	addMinutesWriteFlags(minutesSummaryCmd)

	minutesCmd.AddCommand(minutesTodoCmd)
	minutesTodoCmd.Flags().String("operation", "", "单条操作：add / update / delete")
	minutesTodoCmd.Flags().String("todo", "", "待办纯文本内容（单条 add/update 必填；支持 @file 与 -）")
	minutesTodoCmd.Flags().Bool("is-done", false, "是否完成（单条 add/update 必填，需显式传入）")
	minutesTodoCmd.Flags().String("todo-id", "", "已有待办 ID（单条 update/delete 必填）")
	minutesTodoCmd.Flags().String("todos", "", "批量 todo_items JSON 数组（支持 @file 与 -），与单条参数互斥")
	addMinutesWriteFlags(minutesTodoCmd)

	minutesCmd.AddCommand(minutesSpeakerReplaceCmd)
	minutesSpeakerReplaceCmd.Flags().String("from-speaker-id", "", "要替换的说话人 speaker_id（必填）")
	minutesSpeakerReplaceCmd.Flags().String("to-user-id", "", "新说话人 open_id（ou_ 开头，必填）")
	addMinutesWriteFlags(minutesSpeakerReplaceCmd)

	minutesCmd.AddCommand(minutesWordReplaceCmd)
	minutesWordReplaceCmd.Flags().String("replace-words", "", `替换列表 JSON 数组（支持 @file 与 -），如 [{"source_word":"a","target_word":"b"}]（必填）`)
	addMinutesWriteFlags(minutesWordReplaceCmd)
}
