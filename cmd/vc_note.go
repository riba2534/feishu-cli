package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/riba2534/feishu-cli/v2/internal/safefile"
	"github.com/spf13/cobra"
)

// 智能纪要相关业务码（对齐官方 shortcuts/note）
const (
	// noteNoReadPermissionCode 无该纪要阅读权限
	noteNoReadPermissionCode = 121005
	// noteTranscriptNotSupportCode 统一逐字稿接口不支持该纪要（普通纪要），服务端 msg=not support
	noteTranscriptNotSupportCode = 121002
)

// note_display_type 枚举（官方 note.go：1=normal 普通纪要，2=unified 统一纪要）
const (
	noteDisplayTypeNormal  = 1
	noteDisplayTypeUnified = 2
)

// note artifact_type 枚举：1=纪要主文档，2=逐字稿文档
const (
	noteArtifactTypeMainDoc  = 1
	noteArtifactTypeVerbatim = 2
)

// 统一逐字稿翻页参数（对齐官方 note_transcript.go）
const (
	noteTranscriptPageSize = 200
	noteTranscriptMaxPages = 500
	noteTranscriptPageGap  = 100 * time.Millisecond
)

// vcNoteTranscriptPageGap 翻页间隔，测试可置 0
var vcNoteTranscriptPageGap = noteTranscriptPageGap

// vc note —— 智能会议纪要（smart notes）的独立入口。
// 与 vc notes（按会议/妙记批量查产物）互补：note 子命令组按 note_id 直接操作单篇纪要。
var vcNoteCmd = &cobra.Command{
	Use:   "note",
	Short: "智能会议纪要（按 note_id 查详情 / 导出统一逐字稿）",
	Long: `智能会议纪要（smart notes）操作。note_id 可从 vc notes / vc detail 结果获取。

子命令:
  detail       查询纪要详情（展示类型、关联文档 token 等）
  transcript   导出统一逐字稿（仅统一纪要 note_display_type=2 支持，自动翻页拉全量）

纪要类型（note_display_type）:
  1 = normal  普通纪要：逐字稿在 verbatim 文档（artifact_type=2 的 doc_token）里，用 doc export 读取
  2 = unified 统一纪要：用 vc note transcript 导出统一逐字稿

权限要求: vc:note:read`,
}

var vcNoteDetailCmd = &cobra.Command{
	Use:   "detail <note_id>",
	Short: "查询智能纪要详情",
	Long: `查询智能纪要详情（GET /open-apis/vc/v1/notes/{note_id}），输出原始 data。

关键字段:
  note.note_display_type   1=普通纪要（逐字稿在 verbatim 文档）/ 2=统一纪要（可用 vc note transcript）
  note.artifacts[]         artifact_type=1 纪要主文档，artifact_type=2 逐字稿文档（doc_token）
  note.references[]        纪要引用的共享文档

身份:
  --as user（默认）| bot | auto。Bot 身份需应用开通 vc:note:read 且对纪要有权限。

权限要求: vc:note:read

示例:
  feishu-cli vc note detail 7690848884788907213
  feishu-cli vc note detail 7690848884788907213 --as bot`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		token, err := resolveVCReadIdentity(cmd)
		if err != nil {
			return err
		}
		raw, err := client.GetMeetingNote(strings.TrimSpace(args[0]), token)
		if err != nil {
			return decorateNoteError(err)
		}
		var result map[string]any
		if err := json.Unmarshal(raw, &result); err != nil {
			return fmt.Errorf("解析纪要详情失败: %w", err)
		}
		return printJSON(result)
	},
}

var vcNoteTranscriptCmd = &cobra.Command{
	Use:   "transcript <note_id>",
	Short: "导出智能纪要的统一逐字稿（自动翻页拉全量）",
	Long: `导出统一纪要（note_display_type=2）的统一逐字稿（unified transcript）。

流程（对齐官方 note +transcript）:
  1. 先查纪要详情确认类型：普通纪要（note_display_type=1）不支持统一逐字稿，
     直接提示改读 verbatim 逐字稿文档（feishu-cli doc export <verbatim_doc_token>）
  2. 按 format / page_size=200 / locale 拉取，cursor_id 自动翻页（重复游标防护，上限 500 页）
  3. 任一页失败或结果为空即报错，不输出半截逐字稿

参数:
  <note_id>    纪要 ID
  --format     逐字稿格式: markdown（默认）/ plain_text（旧值 text 为 plain_text 的别名）
  --locale     逐字稿语言，如 zh_cn / en_us / ja_jp（默认飞书 zh_cn，Lark 品牌 en_us）
  --output     保存文件路径（缺省打印到 stdout）

权限要求（User Token）: vc:note:read

示例:
  feishu-cli vc note transcript 7690848884788907213
  feishu-cli vc note transcript 7690848884788907213 --format plain_text --output transcript.txt`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		format, _ := cmd.Flags().GetString("format")
		format, err := normalizeNoteTranscriptFormat(format)
		if err != nil {
			return err
		}
		locale, _ := cmd.Flags().GetString("locale")
		locale = resolveNoteTranscriptLocale(locale)
		output, _ := cmd.Flags().GetString("output")
		if output != "" {
			if err := safefile.ValidateOutputPath(output); err != nil {
				return err
			}
		}
		noteID := strings.TrimSpace(args[0])
		if noteID == "" {
			return clierr.Usagef("note_id 不能为空")
		}

		token, err := requireUserToken(cmd, "vc note transcript")
		if err != nil {
			return err
		}

		// 先确认纪要类型：普通纪要直接给出改读逐字稿文档的指引，不再盲调统一逐字稿接口
		detail, err := fetchNoteDetail(noteID, token)
		if err != nil {
			return err
		}
		if detail.DisplayType == noteDisplayTypeNormal {
			return noteNotUnifiedError(noteID, detail)
		}

		content, err := fetchUnifiedNoteTranscript(noteID, format, locale, token)
		if err != nil {
			if client.HasAPICode(err, noteTranscriptNotSupportCode) {
				return noteNotUnifiedError(noteID, detail)
			}
			return decorateNoteError(err)
		}

		if output != "" {
			if err := safefile.AtomicWriteFile(output, content, 0600); err != nil {
				return fmt.Errorf("写入文件失败: %w", err)
			}
			fmt.Printf("逐字稿已保存到 %s（%d 字节）\n", output, len(content))
			return nil
		}
		_, _ = os.Stdout.Write(content)
		return nil
	},
}

// noteDetail 纪要详情中命令层关心的字段
type noteDetail struct {
	NoteID           string
	DisplayType      int // 0 = 未返回 / 未知
	NoteDocToken     string
	VerbatimDocToken string
	SharedDocTokens  []string
	CreateTime       string
}

// displayTypeName 把 note_display_type 转成稳定的字符串（normal / unified / unknown）
func (d *noteDetail) displayTypeName() string {
	switch d.DisplayType {
	case noteDisplayTypeNormal:
		return "normal"
	case noteDisplayTypeUnified:
		return "unified"
	default:
		return "unknown"
	}
}

// fetchNoteDetail 查询并解析纪要详情
func fetchNoteDetail(noteID, token string) (*noteDetail, error) {
	raw, err := client.GetMeetingNote(noteID, token)
	if err != nil {
		return nil, decorateNoteError(err)
	}
	return parseNoteDetail(noteID, raw)
}

// parseNoteDetail 解析 GET /vc/v1/notes/{note_id} 的 data。
// note_display_type 兼容整数 / 字符串；缺失时保持 0（unknown），由调用方决定是否继续。
func parseNoteDetail(noteID string, raw json.RawMessage) (*noteDetail, error) {
	var parsed struct {
		Note *struct {
			CreateTime      string          `json:"create_time"`
			NoteDisplayType json.RawMessage `json:"note_display_type"`
			DisplayType     json.RawMessage `json:"display_type"`
			Artifacts       []struct {
				ArtifactType json.RawMessage `json:"artifact_type"`
				DocToken     string          `json:"doc_token"`
			} `json:"artifacts"`
			References []struct {
				DocToken string `json:"doc_token"`
			} `json:"references"`
		} `json:"note"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("解析纪要详情失败: %w", err)
	}
	if parsed.Note == nil {
		return nil, fmt.Errorf("纪要 %s 详情为空（响应缺少 note 对象）", noteID)
	}
	d := &noteDetail{NoteID: noteID, CreateTime: parsed.Note.CreateTime}
	dt := parsed.Note.NoteDisplayType
	if len(dt) == 0 {
		dt = parsed.Note.DisplayType
	}
	d.DisplayType = looseJSONInt(dt)
	for _, a := range parsed.Note.Artifacts {
		switch looseJSONInt(a.ArtifactType) {
		case noteArtifactTypeMainDoc:
			d.NoteDocToken = a.DocToken
		case noteArtifactTypeVerbatim:
			d.VerbatimDocToken = a.DocToken
		}
	}
	for _, r := range parsed.Note.References {
		if r.DocToken != "" {
			d.SharedDocTokens = append(d.SharedDocTokens, r.DocToken)
		}
	}
	return d, nil
}

// looseJSONInt 解析整数或数字字符串，失败返回 0
func looseJSONInt(raw json.RawMessage) int {
	s := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if s == "" || s == "null" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// noteNotUnifiedError 普通纪要不支持统一逐字稿：提示改读 verbatim 逐字稿文档
func noteNotUnifiedError(noteID string, d *noteDetail) error {
	typeName := "unknown"
	if d != nil {
		typeName = d.displayTypeName()
	}
	if d != nil && d.VerbatimDocToken != "" {
		return clierr.Usagef("纪要 %s 不是统一纪要（note_display_type=%s），不支持统一逐字稿接口。"+
			"逐字稿在 verbatim 文档 %s 中，请改用: feishu-cli doc export %s -o transcript.md",
			noteID, typeName, d.VerbatimDocToken, d.VerbatimDocToken)
	}
	return clierr.Usagef("纪要 %s 不是统一纪要（note_display_type=%s），不支持统一逐字稿接口，且详情中没有 verbatim 逐字稿文档。"+
		"可用 feishu-cli vc note detail %s 查看关联文档 token", noteID, typeName, noteID)
}

// decorateNoteError 无纪要阅读权限时给出可执行提示，其余错误原样返回
func decorateNoteError(err error) error {
	if err == nil {
		return nil
	}
	if client.HasAPICode(err, noteNoReadPermissionCode) {
		return clierr.Auth(fmt.Errorf("%w\n提示：当前身份没有该纪要的阅读权限，请联系纪要所有者授权后重试", err))
	}
	return err
}

// normalizeNoteTranscriptFormat 校验并规范化 --format；旧值 text 映射为 plain_text
func normalizeNoteTranscriptFormat(format string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "markdown", "md":
		return "markdown", nil
	case "plain_text", "plain-text", "plaintext":
		return "plain_text", nil
	case "text":
		fmt.Fprintln(os.Stderr, "提示：--format text 已更名为 plain_text（对齐服务端参数），本次按 plain_text 处理")
		return "plain_text", nil
	default:
		return "", clierr.Usagef("--format 仅支持 markdown / plain_text，得到 %q", format)
	}
}

// resolveNoteTranscriptLocale 显式 --locale 优先；否则 Lark 品牌用 en_us，飞书用 zh_cn（对齐官方）
func resolveNoteTranscriptLocale(explicit string) string {
	if s := strings.TrimSpace(explicit); s != "" {
		return s
	}
	if cfg := config.Get(); cfg != nil && config.ParseBrand(cfg.BaseURL) == config.BrandLark {
		return "en_us"
	}
	return "zh_cn"
}

// fetchUnifiedNoteTranscript 拉取统一逐字稿全部分页并拼接。
// 任一页失败、游标不前进/重复、超过页数上限或最终为空都报错，避免保存半截逐字稿。
func fetchUnifiedNoteTranscript(noteID, format, locale, token string) ([]byte, error) {
	var buf strings.Builder
	cursor := ""
	seen := map[string]bool{}
	for page := 1; ; page++ {
		if page > noteTranscriptMaxPages {
			return nil, fmt.Errorf("纪要 %s 逐字稿超过 %d 页仍未结束，已中止以避免无限翻页", noteID, noteTranscriptMaxPages)
		}
		params := map[string]string{
			"format":    format,
			"page_size": strconv.Itoa(noteTranscriptPageSize),
			"locale":    locale,
		}
		if cursor != "" {
			params["cursor_id"] = cursor
		}
		data, err := client.GetUnifiedNoteTranscript(noteID, params, token)
		if err != nil {
			return nil, err
		}
		chunk, err := parseUnifiedTranscriptPage(data, format)
		if err != nil {
			return nil, fmt.Errorf("纪要 %s 第 %d 页: %w", noteID, page, err)
		}
		buf.WriteString(chunk)

		hasMore, _ := data["has_more"].(bool)
		if !hasMore {
			break
		}
		next, ok := parseLooseCursor(data["next_cursor_id"])
		if !ok {
			next, ok = parseLooseCursor(data["cursor_id"])
		}
		if !ok || next == cursor || seen[next] {
			return nil, fmt.Errorf("纪要 %s 第 %d 页 has_more=true 但翻页游标未前进，已中止以避免输出不完整的逐字稿", noteID, page)
		}
		seen[cursor] = true
		cursor = next
		if vcNoteTranscriptPageGap > 0 {
			time.Sleep(vcNoteTranscriptPageGap)
		}
	}
	if buf.Len() == 0 {
		return nil, fmt.Errorf("纪要 %s 的 %s 逐字稿为空（可能仍在生成中，稍后重试）", noteID, format)
	}
	return []byte(buf.String()), nil
}

// parseUnifiedTranscriptPage 从 unified_note_transcript 响应 data 中取本页逐字稿。
// 官方结构：data.transcript 是对象，按 format 取 data.transcript.<format> 字符串；
// 兼容旧形态 data.transcript 为字符串。
// 识别不出逐字稿字段时**显式报错**（列出实际字段名），绝不把响应 envelope 伪装成逐字稿内容输出——
// 那会产出垃圾文件且静默截断翻页，比失败更难发现。
func parseUnifiedTranscriptPage(data map[string]any, format string) (string, error) {
	if data == nil {
		return "", fmt.Errorf("响应 data 为空")
	}
	raw, ok := data["transcript"]
	if !ok || raw == nil {
		keys := make([]string, 0, len(data))
		for k := range data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return "", fmt.Errorf(
			"无法从响应中识别逐字稿字段（实际字段: %s）。响应结构可能已变更，可用 `feishu-cli api GET /open-apis/vc/v1/notes/<note_id>/unified_note_transcript --params '{\"format\":\"%s\"}'` 查看原始响应并反馈 issue",
			strings.Join(keys, ", "), format)
	}
	switch v := raw.(type) {
	case map[string]any:
		if s, ok := v[format].(string); ok {
			return s, nil
		}
		if val, ok := v[format]; ok && val == nil {
			return "", nil
		}
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) == 0 {
			// 空对象：本页无内容
			return "", nil
		}
		return "", fmt.Errorf("响应 transcript 对象中没有 %s 字段（实际字段: %s）", format, strings.Join(keys, ", "))
	case string:
		return v, nil
	default:
		return "", fmt.Errorf("响应 transcript 字段类型无法识别（%T）", raw)
	}
}

// parseLooseCursor 把游标统一成字符串：兼容字符串、json.Number、float64（仅安全整数）。
// 空串、"0"、非正数视为无游标。
func parseLooseCursor(v any) (string, bool) {
	switch n := v.(type) {
	case string:
		s := strings.TrimSpace(n)
		if s == "" || s == "0" {
			return "", false
		}
		return s, true
	case json.Number:
		s := strings.TrimSpace(n.String())
		if i, err := strconv.ParseInt(s, 10, 64); err != nil || i <= 0 {
			return "", false
		}
		return s, true
	case float64:
		const maxSafeJSONInteger = 1<<53 - 1
		if n <= 0 || n != float64(int64(n)) || n > maxSafeJSONInteger {
			return "", false
		}
		return strconv.FormatInt(int64(n), 10), true
	case int64:
		if n <= 0 {
			return "", false
		}
		return strconv.FormatInt(n, 10), true
	case int:
		if n <= 0 {
			return "", false
		}
		return strconv.Itoa(n), true
	}
	return "", false
}

func init() {
	vcCmd.AddCommand(vcNoteCmd)
	vcNoteCmd.AddCommand(vcNoteDetailCmd, vcNoteTranscriptCmd)
	addVCReadAsFlag(vcNoteDetailCmd)
	vcNoteDetailCmd.Flags().String("user-access-token", "", "User Access Token")
	vcNoteTranscriptCmd.Flags().String("user-access-token", "", "User Access Token")
	vcNoteTranscriptCmd.Flags().String("output", "", "保存文件路径（缺省打印到 stdout）")
	vcNoteTranscriptCmd.Flags().String("format", "markdown", "逐字稿格式: markdown / plain_text（旧值 text 为 plain_text 的别名）")
	vcNoteTranscriptCmd.Flags().String("locale", "", "逐字稿语言，如 zh_cn / en_us / ja_jp（默认飞书 zh_cn，Lark 品牌 en_us）")
}
