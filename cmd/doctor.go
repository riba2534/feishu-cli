package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	buildinfo "runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// checkResult 表示一项诊断结果
type checkResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // pass / fail / warn / skip
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

func checkPass(name, msg string) checkResult {
	return checkResult{Name: name, Status: "pass", Message: msg}
}

func checkFail(name, msg, hint string) checkResult {
	return checkResult{Name: name, Status: "fail", Message: msg, Hint: hint}
}

func checkWarn(name, msg, hint string) checkResult {
	return checkResult{Name: name, Status: "warn", Message: msg, Hint: hint}
}

func checkSkip(name, msg string) checkResult {
	return checkResult{Name: name, Status: "skip", Message: msg}
}

var doctorJSON bool
var doctorOffline bool
var doctorOnly string

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "健康检查（配置 / 认证 / 网络 / 依赖一把验）",
	Long: `feishu-cli doctor 跑一组本地诊断，验证 CLI 是否处于可用状态。

检查项：
  config_file          配置文件存在 + app_id/app_secret 非空
  user_token           token.json 存在 + 未过期
  endpoint_open        open.feishu.cn HTTPS 可达
  endpoint_larksuite   open.larksuite.com HTTPS 可达
  proxy                HTTP(S)_PROXY 与 NO_PROXY 配置合理
  dependencies         Go 版本 / SDK 版本

输出：
  默认：pretty 表格
  --json：机器可读 JSON

退出码：
  0 = 全部通过（或仅 warn）
  1 = 至少一项 fail`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_ = config.Init(cfgFile) // 复用 root.go 的 cfgFile
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		only, parseErr := parseOnly(doctorOnly)
		if parseErr != nil {
			return parseErr
		}
		var results []checkResult

		// 1. config_file
		if shouldRun("config_file", only) {
			results = append(results, checkConfigFile())
		}

		// 2. user_token
		if shouldRun("user_token", only) {
			results = append(results, checkUserToken())
		}

		// 3 & 4. endpoints
		if !doctorOffline {
			if shouldRun("endpoint_open", only) || shouldRun("endpoint_larksuite", only) {
				netChecks := checkEndpoints(ctx, only)
				results = append(results, netChecks...)
			}
		} else {
			if shouldRun("endpoint_open", only) {
				results = append(results, checkSkip("endpoint_open", "已跳过 (--offline)"))
			}
			if shouldRun("endpoint_larksuite", only) {
				results = append(results, checkSkip("endpoint_larksuite", "已跳过 (--offline)"))
			}
		}

		// 5. proxy
		if shouldRun("proxy", only) {
			results = append(results, checkProxy())
		}

		// 6. dependencies
		if shouldRun("dependencies", only) {
			results = append(results, checkDependencies())
		}

		// 输出
		if doctorJSON {
			return outputJSON(results)
		}
		return outputPretty(results)
	},
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorJSON, "json", false, "输出 JSON 格式（AI agent 自检友好）")
	doctorCmd.Flags().BoolVar(&doctorOffline, "offline", false, "跳过网络检查")
	doctorCmd.Flags().StringVar(&doctorOnly, "only", "", "仅运行指定检查（逗号分隔，如 user_token,endpoint_open）")
	rootCmd.AddCommand(doctorCmd)
}

// validOnlyNames doctor 支持的所有 check 名（与 shouldRun 调用处同步）。
// 新增 check 时务必同步更新；缺失时 --only 会拒接收以避免 CI 静默 pass。
var validOnlyNames = map[string]bool{
	"config_file":        true,
	"user_token":         true,
	"endpoint_open":      true,
	"endpoint_larksuite": true,
	"proxy":              true,
	"dependencies":       true,
}

// parseOnly 解析 --only 参数；空字符串表示全部；包含未知 name 返回 error
func parseOnly(s string) (map[string]bool, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	m := make(map[string]bool)
	var bad []string
	for _, raw := range strings.Split(s, ",") {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if !validOnlyNames[name] {
			bad = append(bad, name)
			continue
		}
		m[name] = true
	}
	if len(bad) > 0 {
		valid := make([]string, 0, len(validOnlyNames))
		for k := range validOnlyNames {
			valid = append(valid, k)
		}
		sort.Strings(valid)
		return nil, fmt.Errorf("--only 包含未知 check 名: %s（合法值: %s）",
			strings.Join(bad, ", "), strings.Join(valid, ", "))
	}
	if len(m) == 0 {
		return nil, fmt.Errorf("--only 为空（去除空白后）")
	}
	return m, nil
}

func shouldRun(name string, only map[string]bool) bool {
	if only == nil {
		return true
	}
	return only[name]
}

// ── 检查函数 ──

func checkConfigFile() checkResult {
	cfg := config.Get()
	if cfg == nil || cfg.AppID == "" {
		return checkFail("config_file", "未找到 app_id 配置",
			"运行 feishu-cli config init 或设置 FEISHU_APP_ID 环境变量")
	}
	if cfg.AppSecret == "" {
		return checkFail("config_file", "未找到 app_secret 配置",
			"运行 feishu-cli config init 或设置 FEISHU_APP_SECRET")
	}
	return checkPass("config_file", fmt.Sprintf("app_id=%s baseURL=%s", auth.MaskToken(cfg.AppID), cfg.BaseURL))
}

func checkUserToken() checkResult {
	token, err := auth.LoadToken()
	if err != nil {
		// 区分「文件损坏」和「文件不存在」——文件损坏要明确报错
		return checkFail("user_token", "读取 token.json 失败: "+err.Error(),
			"删除 ~/.feishu-cli/token.json 后重新 feishu-cli auth login")
	}
	if token == nil {
		return checkWarn("user_token", "未登录用户 (token.json 不存在)",
			"运行 feishu-cli auth login（仅 vc/minutes/mail/drive/search 等命令必需）")
	}
	status := token.TokenStatus()
	switch status {
	case "valid":
		// 不暴露 token 本体，只给状态
		return checkPass("user_token", "access_token 有效")
	case "needs_refresh":
		return checkPass("user_token", "access_token 过期但 refresh_token 有效（下次调用自动刷新）")
	case "expired":
		return checkFail("user_token", "access_token 与 refresh_token 均已过期",
			"运行 feishu-cli auth login 重新授权")
	default:
		return checkWarn("user_token", "token 状态未知: "+status, "")
	}
}

func checkEndpoints(ctx context.Context, only map[string]bool) []checkResult {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	type probeTarget struct {
		name string
		url  string
	}
	targets := []probeTarget{
		{"endpoint_open", "https://open.feishu.cn"},
		{"endpoint_larksuite", "https://open.larksuite.com"},
	}
	var wg sync.WaitGroup
	results := make([]checkResult, len(targets))
	for i, t := range targets {
		if !shouldRun(t.name, only) {
			results[i] = checkSkip(t.name, "已跳过")
			continue
		}
		wg.Add(1)
		go func(i int, t probeTarget) {
			defer wg.Done()
			start := time.Now()
			req, err := http.NewRequestWithContext(ctx, http.MethodHead, t.url, nil)
			if err != nil {
				results[i] = checkFail(t.name, err.Error(), "")
				return
			}
			resp, err := httpClient.Do(req)
			rtt := time.Since(start)
			if err != nil {
				results[i] = checkFail(t.name, fmt.Sprintf("%s 不可达: %v", t.url, err),
					"检查网络 / 代理设置")
				return
			}
			resp.Body.Close()
			results[i] = checkPass(t.name, fmt.Sprintf("%s 可达 (RTT %dms, status %d)", t.url, rtt.Milliseconds(), resp.StatusCode))
		}(i, t)
	}
	wg.Wait()
	return results
}

func checkProxy() checkResult {
	httpProxy := strings.TrimSpace(firstEnv("HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"))
	noProxy := firstEnv("NO_PROXY", "no_proxy")
	if httpProxy == "" {
		return checkPass("proxy", "未设置 HTTP(S)_PROXY")
	}
	// v1 PR 二轮 rv 加固：按逗号 split 而非 strings.Contains 子串匹配。
	// Go net/http 的 NO_PROXY 接受多种格式：`feishu.cn` / `.feishu.cn` / `*.feishu.cn` / 带端口；
	// 我们的"包含飞书域"判定也按这套规则——只要某个 entry 与目标 domain 匹配即视为已覆盖。
	feishuDomains := []string{"feishu.cn", "larkoffice.com", "larksuite.com"}
	entries := splitNoProxyEntries(noProxy)
	var missing []string
	for _, d := range feishuDomains {
		if !noProxyCovers(entries, d) {
			missing = append(missing, "."+d)
		}
	}
	msg := fmt.Sprintf("HTTPS_PROXY=%s", redactProxyURL(httpProxy))
	if len(missing) > 0 {
		return checkWarn("proxy", msg,
			fmt.Sprintf("NO_PROXY 缺少飞书域：%v；建议加入 NO_PROXY 避免代理拦截内部域", missing))
	}
	return checkPass("proxy", msg+"，NO_PROXY 已包含飞书域")
}

func firstEnv(names ...string) string {
	for _, n := range names {
		if v := os.Getenv(n); v != "" {
			return v
		}
	}
	return ""
}

func checkDependencies() checkResult {
	goVer := runtime.Version()
	sdkVer := "unknown"
	if info, ok := buildinfo.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			if dep.Path == "github.com/larksuite/oapi-sdk-go/v3" {
				sdkVer = dep.Version
				break
			}
		}
	}
	return checkPass("dependencies", fmt.Sprintf("go=%s larksuite-sdk=%s", goVer, sdkVer))
}

// ── 输出 ──

func outputJSON(results []checkResult) error {
	allOK := true
	for _, r := range results {
		if r.Status == "fail" {
			allOK = false
		}
	}
	out := map[string]interface{}{
		"ok":     allOK,
		"checks": results,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return err
	}
	if !allOK {
		return fmt.Errorf("doctor: 有检查未通过")
	}
	return nil
}

func outputPretty(results []checkResult) error {
	allOK := true
	for _, r := range results {
		var icon string
		switch r.Status {
		case "pass":
			icon = "✓"
		case "fail":
			icon = "✗"
			allOK = false
		case "warn":
			icon = "⚠️"
		case "skip":
			icon = "-"
		default:
			icon = "?"
		}
		fmt.Printf("%-22s %s  %s\n", r.Name, icon, r.Message)
		if r.Hint != "" {
			fmt.Printf("%-22s    💡 %s\n", "", r.Hint)
		}
	}
	if !allOK {
		fmt.Fprintln(os.Stderr, "\ndoctor: 有检查未通过，详见 fail 项的 hint")
		return fmt.Errorf("doctor: 有检查未通过")
	}
	fmt.Println("\n全部通过 ✓")
	return nil
}

// redactProxyURL 去掉 proxy URL 中的 userinfo，避免 doctor 输出泄露凭证。
// 入参不是合法 URL 时原样返回；含 userinfo 但 URL malformed 时仍做 best-effort string redaction。
//
// 实现走纯字符串解析不依赖 net/url：
//   - net/url URL.String() 会把 "***" percent-encode 成 "%2A%2A%2A"，对用户不直观
//   - net/url Parse() 对 malformed URL（如 `https://user:secret@[::1`）返回 err，
//     若 err 时 return raw 会让含凭证的 malformed URL 原样泄露——defense-in-depth gap
//
// authority 内 userinfo / host 边界按 RFC 3986 + Go net/url url.go:500-545 用**最后一个 `@`**
// 分隔（不是第一个）。否则 `https://user:p@ssword@host` 会被切成 userinfo=`user:p`、
// host=`ssword@host`，输出 `https://***@ssword@host` 半泄密码。
//
// 已知限制（doctor 是 user-facing 诊断，不是 paranoid security tool）：
// 密码里如果**裸写** `/`、`?`、`#`（RFC 3986 在 userinfo 位置必须 percent-encode，
// 95%+ 用户的 hex / base64 / 字母数字 token 不会触发），authority 边界会先在该字符
// 处被截断、找不到 `@`，doctor 会原样回显该 URL。real-world 撞到概率 < 1%；用户即便
// 撞到也是自己配错 RFC 不合规，且大多数 HTTP 库会拒绝这种 URL。修复需引入启发式判定
// （isAllDigits port + IPv6 `[` 前缀 + 不全数字 user:pass 形态）和 4+ 测试 case，
// ROI 太低不收。
func redactProxyURL(raw string) string {
	if raw == "" {
		return raw
	}
	// scheme:// 边界
	schemeIdx := strings.Index(raw, "://")
	if schemeIdx < 0 {
		// 无 scheme，不是 URL 形态，原样返回
		return raw
	}
	after := raw[schemeIdx+3:]
	// authority 范围：scheme:// 之后到第一个 / ? # 之前（path/query/fragment 里的 @ 不算 userinfo 分隔符）
	authEnd := strings.IndexAny(after, "/?#")
	var authority, tail string
	if authEnd < 0 {
		authority = after
		tail = ""
	} else {
		authority = after[:authEnd]
		tail = after[authEnd:]
	}
	// authority 内最后一个 `@` 才是 userinfo 与 host 边界（密码里可裸 `@`，RFC 兼容）
	at := strings.LastIndex(authority, "@")
	if at < 0 {
		// 无 userinfo（含纯 host:port、IPv6 `[::1]:port` 等）
		return raw
	}
	// 整段 userinfo（[:at]，含 username/password 不论形态）替换为 ***，保留 `@` 起始的 host 段
	return raw[:schemeIdx+3] + "***" + authority[at:] + tail
}

// splitNoProxyEntries 把 NO_PROXY 按逗号 split + 去空白，每项去前导点（".feishu.cn" → "feishu.cn"）。
func splitNoProxyEntries(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.TrimPrefix(p, ".")
		// 去端口（如 "feishu.cn:443" → "feishu.cn"）
		if i := strings.LastIndex(p, ":"); i > 0 && !strings.Contains(p[i+1:], ".") {
			p = p[:i]
		}
		if p != "" {
			out = append(out, strings.ToLower(p))
		}
	}
	return out
}

// noProxyCovers 判定用户在 NO_PROXY 中是否"提到"了飞书根域（feishu.cn / larkoffice.com /
// larksuite.com）。这是 doctor user-facing 友好提示，**故意采用宽松语义**，与 Go net/http
// httpproxy 的标准 NO_PROXY-host 匹配不同：
//
//   - 标准 Go 语义：entry `feishu.cn` matches host `a.feishu.cn`（entry 是 host suffix）
//   - 本函数语义：entry == d / entry 是 d 的子域（如 `a.feishu.cn` 也算"提到了 feishu.cn"）/
//     entry 为 `*`（Go httpproxy 标准：单独 `*` 表所有请求不走代理）
//
// 选择宽松的动机：doctor 的检查目的是"用户有没有意识到飞书需要 NO_PROXY"——若用户写
// `NO_PROXY=open.feishu.cn`，说明已意识到飞书域有 NO_PROXY 需求，doctor 不再 warn。
// 副作用是不会提醒用户"你只配了 a.feishu.cn 不覆盖整个飞书"——这是 doctor 范围外的细化建议。
//
// 不要按 Go 标准反向阅读 `HasSuffix(e, "."+d)`：这里是"entry 以 .root 结尾即视为提到了 root"，
// 不是"root 以 .entry 结尾"。
func noProxyCovers(entries []string, domain string) bool {
	d := strings.ToLower(domain)
	for _, e := range entries {
		if e == "*" || e == d || strings.HasSuffix(e, "."+d) {
			return true
		}
	}
	return false
}
