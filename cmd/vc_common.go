package cmd

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

// vcBatchLimit VC / Minutes 命令批量入参上限
const vcBatchLimit = 50

// vcBatchDelay 批量处理单条之间的延迟（缓解频率限制）
const vcBatchDelay = 100 * time.Millisecond

// parseVCTime 将用户输入的时间字符串解析为 RFC3339 格式
// 支持：RFC3339、2006-01-02T15:04:05、2006-01-02 15:04:05、2006-01-02
// isEnd=true 且输入是纯日期时，对齐到当天 23:59:59；否则 00:00:00
// 解析失败返回错误
func parseVCTime(input string, isEnd bool) (string, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", nil
	}

	// 优先尝试包含时区的格式
	tzLayouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04Z07:00",
	}
	for _, layout := range tzLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format(time.RFC3339), nil
		}
	}

	// 无时区格式，走本地时区
	loc := time.Local
	localLayouts := []string{
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04",
	}
	for _, layout := range localLayouts {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t.Format(time.RFC3339), nil
		}
	}

	// 仅日期
	if t, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
		if isEnd {
			t = t.Add(24*time.Hour - time.Second)
		}
		return t.Format(time.RFC3339), nil
	}

	return "", fmt.Errorf("无法解析时间 %q，支持格式: RFC3339 / 2006-01-02 / 2006-01-02 15:04:05", input)
}

var minuteTokenPattern = regexp.MustCompile(`^[A-Za-z0-9]+$`)

// ensureMinuteToken 校验 minute_token 基础格式
func ensureMinuteToken(token string) error {
	if len(token) < 5 {
		return clierr.Usagef("minute_token 长度过短: %q", token)
	}
	if !minuteTokenPattern.MatchString(token) {
		return clierr.Usagef("minute_token 含非法字符: %q", token)
	}
	return nil
}

// normalizeMinuteTokenInput 接受 minute_token 或妙记链接
// （https://*.feishu.cn|larksuite.com|larkoffice.com/minutes/<token>），返回校验后的 minute_token。
// 非法输入均为用法错误（exit 2）。
func normalizeMinuteTokenInput(raw string) (string, error) {
	input := strings.TrimSpace(raw)
	token := input
	if client.LooksLikeURL(input) {
		t, err := client.ParseMinuteURL(input)
		if err != nil {
			return "", clierr.Usagef("无法从妙记链接提取 minute_token: %v", err)
		}
		token = t
	}
	if err := ensureMinuteToken(token); err != nil {
		return "", err
	}
	return token, nil
}

// parseMinuteTokenList 解析逗号分隔的 minute_token / 妙记链接列表：逐个规范化为 minute_token，
// 去重保序并校验上限（链接与裸 token 指向同一妙记时只保留一条）。
func parseMinuteTokenList(raw, label string) ([]string, error) {
	items, err := parseCSVIDs(raw, label)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		token, err := normalizeMinuteTokenInput(item)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}
	return out, nil
}

// parseCSVIDs 解析逗号分隔的 ID 列表，去重保序并校验上限
// label 用于错误提示（如 "meeting-ids"）
func parseCSVIDs(raw, label string) ([]string, error) {
	items := splitAndTrim(raw)
	if len(items) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, v := range items {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	if len(out) > vcBatchLimit {
		return nil, clierr.Usagef("--%s 最多 %d 条，当前 %d 条", label, vcBatchLimit, len(out))
	}
	return out, nil
}

// vcBatchItem 批量处理结果项
type vcBatchItem struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

// vcBatchSummary 批量处理统计
type vcBatchSummary struct {
	Total     int `json:"total"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}

// summarizeBatch 计算批量处理统计
func summarizeBatch(items []vcBatchItem) vcBatchSummary {
	s := vcBatchSummary{Total: len(items)}
	for _, it := range items {
		if it.OK {
			s.Succeeded++
		} else {
			s.Failed++
		}
	}
	return s
}

// exactlyOneNonEmpty 确保 values 里恰好一个非空
// names 与 values 一一对应，用于错误提示
func exactlyOneNonEmpty(names []string, values []string) error {
	picked := 0
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			picked++
		}
	}
	if picked == 0 {
		return clierr.Usagef("请恰好指定 %s 之一", strings.Join(nameList(names), " / "))
	}
	if picked > 1 {
		return clierr.Usagef("%s 不能同时使用，请只选其一", strings.Join(nameList(names), " / "))
	}
	return nil
}

// nameList 把 flag 名字加上 --- 前缀
func nameList(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = "--" + n
	}
	return out
}

// dedupStrings 去重保序（空字符串被忽略）
// 用于 VC 命令从 API 响应里拿到的 ID 列表去重
func dedupStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// parseVCTimestamp 自动识别 Unix 时间戳单位（秒/毫秒/微秒/纳秒）
// 飞书不同接口会混用 10/13 位时间戳，这里统一做兼容。
func parseVCTimestamp(ts string) (time.Time, bool) {
	var raw int64
	if _, err := fmt.Sscanf(strings.TrimSpace(ts), "%d", &raw); err != nil || raw <= 0 {
		return time.Time{}, false
	}

	switch {
	case raw >= 1e18:
		return time.Unix(0, raw), true
	case raw >= 1e15:
		return time.UnixMicro(raw), true
	case raw >= 1e12:
		return time.UnixMilli(raw), true
	default:
		return time.Unix(raw, 0), true
	}
}

// formatVCTime 尝试将 Unix 时间戳字符串转为可读时间（本地时区）
// 用于 VC/Minutes 命令的文本输出
func formatVCTime(ts string) string {
	if parsed, ok := parseVCTimestamp(ts); ok {
		return parsed.In(time.Local).Format("2006-01-02 15:04:05")
	}
	return ts
}

// vcReadAsFlagHelp 会议 / 纪要 / 妙记读命令的 --as 帮助。默认 user 保持历史行为（这些命令原先只收 User Token）。
const vcReadAsFlagHelp = "身份: user(默认，User Token) | bot(App Token，需应用开通对应权限) | auto(User 优先；未配置回退 Bot；已配置但解析/刷新失败 fail-closed)"

// addVCReadAsFlag 给会议 / 纪要 / 妙记读命令注册 --as，默认 user（不改变既有默认身份）。
func addVCReadAsFlag(cmd *cobra.Command) {
	cmd.Flags().String("as", "user", vcReadAsFlagHelp)
}

// resolveVCReadIdentity 按 --as 解析会议 / 纪要 / 妙记读命令的身份；空串表示 Bot（App Token）。
// 复用 resolveIdentityToken：auto = User 优先、未配置回退 Bot、已配置但不可用 fail-closed。
func resolveVCReadIdentity(cmd *cobra.Command) (string, error) {
	return resolveIdentityToken(cmd)
}
