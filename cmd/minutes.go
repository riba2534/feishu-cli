package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/safefile"
	"github.com/spf13/cobra"
)

// minutesCmd 妙记父命令
var minutesCmd = &cobra.Command{
	Use:   "minutes",
	Short: "妙记操作命令",
	Long: `妙记相关操作。

子命令:
  get               获取妙记基础信息（可选合并 AI 产物、可轮询等待就绪）
  search            按关键词 / 所有者 / 时间搜索妙记
  apply-permission  申请妙记查看 / 编辑权限
  download          下载妙记媒体文件（批量）

示例:
  feishu-cli minutes get obcnxxxx
  feishu-cli minutes get obcnxxxx --with-artifacts
  feishu-cli minutes search --query "预算复盘"
  feishu-cli minutes apply-permission --minute-token obcnxxxx --perm view
  feishu-cli minutes download --minute-tokens obcnxxxx --output ./media`,
}

var minutesGetCmd = &cobra.Command{
	Use:   "get <minute_token>",
	Short: "获取妙记信息（可选择获取 AI 产物）",
	Long: `通过妙记 Token 获取妙记基础信息，包括标题、链接、创建时间、时长等，
并可按需选择获取 AI 产物（对齐官方 minutes +detail 的选择性获取）。

参数:
  minute_token  妙记 Token

AI 产物（按需选择，可组合）:
  --summary         AI 摘要（summary）
  --todo            待办（minute_todos）
  --chapter         章节（minute_chapters）
  --keyword         关键词（keywords）
  --transcript      逐字稿：写入文件，输出中只给 transcript_file 路径（不再内联，避免输出过大）
  --with-artifacts  兼容旧 flag，等价于同时指定以上 5 项（逐字稿同样写文件）

其他可选:
  --output-dir      逐字稿保存目录（默认 ./minutes/<minute_token>/；指定后为 <dir>/artifact-<标题>-<token>/）
  --overwrite       覆盖已存在的逐字稿文件（默认保留已有文件并提示）
  --wait-ready      轮询直到妙记 / AI 产物就绪（转写完成）再返回，适合刚结束的会议
  --wait-timeout    --wait-ready 最长等待秒数（默认 300）
  --wait-interval   --wait-ready 轮询间隔秒数（默认 10）
  -o, --output json 以 JSON 格式输出

无权限（2091005）时会提示用 minutes apply-permission 申请查看权限（申请前先征得用户同意）。

权限:
  - 默认 User 身份（--as user），可用 --as bot|auto 切换
  - minutes:minutes:readonly
  - 选择任一 AI 产物时额外需要 minutes:minutes.artifacts:read

示例:
  feishu-cli minutes get obcnxxxx
  feishu-cli minutes get obcnxxxx --summary --todo -o json
  feishu-cli minutes get obcnxxxx --transcript --output-dir ./notes
  feishu-cli minutes get obcnxxxx --wait-ready --wait-timeout 600`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		minuteToken := args[0]
		if err := ensureMinuteToken(minuteToken); err != nil {
			return err
		}

		sel := minuteArtifactSelectionFromFlags(cmd)
		output, _ := cmd.Flags().GetString("output")
		outputDir, _ := cmd.Flags().GetString("output-dir")
		overwrite, _ := cmd.Flags().GetBool("overwrite")
		waitReady, _ := cmd.Flags().GetBool("wait-ready")
		waitTimeout, _ := cmd.Flags().GetInt("wait-timeout")
		waitInterval, _ := cmd.Flags().GetInt("wait-interval")
		if outputDir != "" {
			if err := safefile.ValidateOutputPath(outputDir); err != nil {
				return err
			}
		}

		token, err := resolveVCReadIdentity(cmd)
		if err != nil {
			return err
		}

		timeout := time.Duration(waitTimeout) * time.Second
		interval := time.Duration(waitInterval) * time.Second
		minuteData, err := callMinuteUntilReady(waitReady, timeout, interval, func() (json.RawMessage, error) {
			return client.GetMinute(minuteToken, token)
		})
		if err != nil {
			return decorateMinutePermissionError(err, minuteToken)
		}

		var artifacts map[string]any
		if sel.any() {
			artData, artErr := callMinuteUntilReady(waitReady, timeout, interval, func() (json.RawMessage, error) {
				return client.GetMinuteArtifacts(minuteToken, token)
			})
			if artErr != nil {
				artErr = decorateMinutePermissionError(artErr, minuteToken)
				if output == "json" {
					return printJSON(map[string]any{
						"minute":          json.RawMessage(minuteData),
						"artifacts_error": artErr.Error(),
					})
				}
				printMinuteText(minuteData, nil)
				fmt.Printf("\nAI 产物获取失败: %v\n", artErr)
				return nil
			}
			artifacts, err = selectMinuteArtifacts(artData, sel, minuteToken, minuteTitle(minuteData), outputDir, overwrite)
			if err != nil {
				return err
			}
		}

		if output == "json" {
			result := map[string]any{"minute": json.RawMessage(minuteData)}
			if artifacts != nil {
				result["artifacts"] = artifacts
			}
			return printJSON(result)
		}

		printMinuteText(minuteData, artifacts)
		return nil
	},
}

// minuteArtifactSelection minutes get 的 AI 产物选择
type minuteArtifactSelection struct {
	Summary, Todo, Chapter, Keyword, Transcript bool
}

func (s minuteArtifactSelection) any() bool {
	return s.Summary || s.Todo || s.Chapter || s.Keyword || s.Transcript
}

// minuteArtifactSelectionFromFlags 读取选择性 flag；--with-artifacts 兼容旧行为，等价于全选
func minuteArtifactSelectionFromFlags(cmd *cobra.Command) minuteArtifactSelection {
	b := func(name string) bool {
		v, _ := cmd.Flags().GetBool(name)
		return v
	}
	sel := minuteArtifactSelection{
		Summary:    b("summary"),
		Todo:       b("todo"),
		Chapter:    b("chapter"),
		Keyword:    b("keyword"),
		Transcript: b("transcript"),
	}
	if b("with-artifacts") {
		sel = minuteArtifactSelection{Summary: true, Todo: true, Chapter: true, Keyword: true, Transcript: true}
	}
	return sel
}

// selectMinuteArtifacts 只保留选中的 AI 产物（沿用服务端字段名 summary / minute_todos /
// minute_chapters / keywords，兼容旧 --with-artifacts 的 JSON 消费方）；
// 逐字稿写入文件，输出中以 transcript_file 给出路径，不再内联（实测内联逐字稿使输出达 67KB）。
func selectMinuteArtifacts(raw json.RawMessage, sel minuteArtifactSelection, minuteToken, title, outputDir string, overwrite bool) (map[string]any, error) {
	var art map[string]any
	if err := json.Unmarshal(raw, &art); err != nil {
		return nil, fmt.Errorf("解析妙记 AI 产物失败: %w", err)
	}
	pick := func(key string, empty any) any {
		if v, ok := art[key]; ok && v != nil {
			return v
		}
		return empty
	}
	out := map[string]any{}
	if sel.Summary {
		out["summary"] = pick("summary", "")
	}
	if sel.Todo {
		out["minute_todos"] = pick("minute_todos", []any{})
	}
	if sel.Chapter {
		out["minute_chapters"] = pick("minute_chapters", []any{})
	}
	if sel.Keyword {
		out["keywords"] = pick("keywords", []any{})
	}
	if sel.Transcript {
		out["transcript_file"] = ""
		if text, _ := art["transcript"].(string); text != "" {
			path, err := saveMinuteTranscript(minuteToken, title, outputDir, []byte(text), overwrite)
			if err != nil {
				return nil, err
			}
			out["transcript_file"] = path
		}
	}
	return out, nil
}

// saveMinuteTranscript 逐字稿落盘：默认 ./minutes/<token>/transcript.txt；
// 指定 --output-dir 时为 <dir>/artifact-<标题>-<token>/transcript.txt（与 vc notes 布局一致）。
// 文件已存在且未 --overwrite 时保留原文件并在 stderr 提示。
func saveMinuteTranscript(minuteToken, title, outputDir string, content []byte, overwrite bool) (string, error) {
	var dir string
	if outputDir != "" {
		name := safeOutputPath(title, "")
		if name == "" {
			name = "untitled"
		}
		if len(name) > 50 {
			name = name[:50]
		}
		dir = filepath.Join(outputDir, fmt.Sprintf("artifact-%s-%s", name, minuteToken))
	} else {
		dir = filepath.Join("minutes", minuteToken)
	}
	path := filepath.Join(dir, "transcript.txt")
	if err := safefile.ValidateOutputPath(path); err != nil {
		return "", err
	}
	if !overwrite {
		if _, err := os.Stat(path); err == nil {
			fmt.Fprintf(os.Stderr, "逐字稿文件已存在，保留原文件: %s（加 --overwrite 覆盖）\n", path)
			return path, nil
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建逐字稿目录失败: %w", err)
	}
	if err := safefile.AtomicWriteFile(path, content, 0o600); err != nil {
		return "", fmt.Errorf("写入逐字稿失败: %w", err)
	}
	fmt.Fprintf(os.Stderr, "逐字稿已保存: %s（%d 字节）\n", path, len(content))
	return path, nil
}

// minuteTitle 从妙记基础信息中取标题（用于逐字稿目录名）
func minuteTitle(minuteData json.RawMessage) string {
	var parsed struct {
		Minute struct {
			Title string `json:"title"`
		} `json:"minute"`
	}
	_ = json.Unmarshal(minuteData, &parsed)
	return parsed.Minute.Title
}

// minuteProcessingCodeNum 妙记仍在生成（转写未完成）时飞书返回的业务错误码
const minuteProcessingCodeNum = 2091003

// minuteNoPermissionCode 无妙记阅读权限（随 HTTP 403 下发）
const minuteNoPermissionCode = 2091005

// decorateMinutePermissionError 2091005 无权限时提示用 minutes apply-permission 申请（官方 minutes_detail.go）
func decorateMinutePermissionError(err error, minuteToken string) error {
	if err == nil || !client.HasAPICode(err, minuteNoPermissionCode) {
		return err
	}
	return clierr.Auth(fmt.Errorf("%w\n提示：当前身份没有该妙记的阅读权限。征得用户同意后可申请查看权限：feishu-cli minutes apply-permission --minute-token %s --perm view", err, minuteToken))
}

// callMinuteUntilReady 调用妙记接口。开启 --wait-ready 时，遇到"仍在生成"
// （错误码 2091003）会按 interval 轮询，直到就绪或超过 timeout。
// 未开启 wait-ready、超时或遇到其他错误时，直接返回当前结果。
func callMinuteUntilReady(waitReady bool, timeout, interval time.Duration, call func() (json.RawMessage, error)) (json.RawMessage, error) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		data, err := call()
		if err == nil || !waitReady || !isMinuteProcessing(err) {
			return data, err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 || interval > remaining {
			return nil, err
		}
		fmt.Fprintf(os.Stderr, "妙记仍在生成中（转写未完成），%s 后重试...\n", interval)
		time.Sleep(interval)
	}
}

// isMinuteProcessing 判断错误是否为"妙记仍在生成"（可重试）。
// 用词边界安全的 client.HasAPICode 判定，避免 log_id/body 中的无关同数字串误判。
func isMinuteProcessing(err error) bool {
	return client.HasAPICode(err, minuteProcessingCodeNum)
}

// printMinuteText 文本格式化输出妙记信息与已选择的 AI 产物
func printMinuteText(minuteData json.RawMessage, artifacts map[string]any) {
	var parsed struct {
		Minute struct {
			Token      string `json:"token"`
			Title      string `json:"title"`
			URL        string `json:"url"`
			CreateTime string `json:"create_time"`
			OwnerID    string `json:"owner_id"`
			Duration   string `json:"duration"`
		} `json:"minute"`
	}
	if err := json.Unmarshal(minuteData, &parsed); err != nil {
		fmt.Println(string(minuteData))
		return
	}

	m := parsed.Minute
	title := m.Title
	if title == "" {
		title = "(无标题)"
	}

	fmt.Printf("妙记信息:\n\n")
	fmt.Printf("  标题:      %s\n", title)
	if m.Token != "" {
		fmt.Printf("  Token:     %s\n", m.Token)
	}
	if m.URL != "" {
		fmt.Printf("  链接:      %s\n", m.URL)
	}
	if m.CreateTime != "" {
		fmt.Printf("  创建时间:  %s\n", formatVCTime(m.CreateTime))
	}
	if m.OwnerID != "" {
		fmt.Printf("  创建者:    %s\n", m.OwnerID)
	}
	if m.Duration != "" {
		fmt.Printf("  时长:      %s\n", m.Duration)
	}

	if artifacts == nil {
		return
	}
	if v, ok := artifacts["summary"]; ok {
		summary, _ := v.(string)
		if summary == "" {
			fmt.Printf("\nAI 摘要: （无）\n")
		} else {
			if r := []rune(summary); len(r) > 400 {
				summary = string(r[:400]) + "…"
			}
			fmt.Printf("\nAI 摘要:\n  %s\n", summary)
		}
	}
	printArtifactList := func(label, key string) {
		v, ok := artifacts[key]
		if !ok {
			return
		}
		if list, isList := v.([]any); isList && len(list) == 0 {
			fmt.Printf("\n%s: （无）\n", label)
			return
		}
		if b, _ := json.MarshalIndent(v, "  ", "  "); len(b) > 0 {
			fmt.Printf("\n%s:\n  %s\n", label, string(b))
		}
	}
	printArtifactList("Todo 列表", "minute_todos")
	printArtifactList("章节", "minute_chapters")
	if v, ok := artifacts["keywords"]; ok {
		var words []string
		if list, isList := v.([]any); isList {
			for _, w := range list {
				words = append(words, fmt.Sprint(w))
			}
		}
		if len(words) == 0 {
			fmt.Printf("\n关键词: （无）\n")
		} else {
			fmt.Printf("\n关键词: %s\n", strings.Join(words, "、"))
		}
	}
	if v, ok := artifacts["transcript_file"]; ok {
		if path, _ := v.(string); path != "" {
			fmt.Printf("\n逐字稿文件: %s\n", path)
		} else {
			fmt.Printf("\n逐字稿: （无）\n")
		}
	}
}

func init() {
	rootCmd.AddCommand(minutesCmd)
	minutesCmd.AddCommand(minutesGetCmd)
	minutesGetCmd.Flags().Bool("with-artifacts", false, "兼容旧 flag：等价于 --summary --todo --chapter --keyword --transcript（逐字稿写文件）")
	minutesGetCmd.Flags().Bool("summary", false, "获取 AI 摘要")
	minutesGetCmd.Flags().Bool("todo", false, "获取待办（minute_todos）")
	minutesGetCmd.Flags().Bool("chapter", false, "获取章节（minute_chapters）")
	minutesGetCmd.Flags().Bool("keyword", false, "获取关键词（keywords）")
	minutesGetCmd.Flags().Bool("transcript", false, "获取逐字稿并写入文件（输出 transcript_file 路径）")
	minutesGetCmd.Flags().String("output-dir", "", "逐字稿保存目录（默认 ./minutes/<minute_token>/）")
	minutesGetCmd.Flags().Bool("overwrite", false, "覆盖已存在的逐字稿文件")
	minutesGetCmd.Flags().Bool("wait-ready", false, "轮询直到妙记就绪（转写完成）再返回")
	minutesGetCmd.Flags().Int("wait-timeout", 300, "--wait-ready 最长等待秒数")
	minutesGetCmd.Flags().Int("wait-interval", 10, "--wait-ready 轮询间隔秒数")
	minutesGetCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	minutesGetCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	addVCReadAsFlag(minutesGetCmd)
}
