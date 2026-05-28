package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	apiParams         string
	apiData           string
	apiDataFile       string
	apiAs             string
	apiOutput         string
	apiDryRun         bool
	apiRaw            bool
	apiIncludeHeaders bool
	apiTimeoutSec     int
	apiFormat         string // 输出格式: json|pretty|table|ndjson|csv（空=保持默认 pretty/raw）
	apiJQ             string // jq 表达式（内置 gojq）
)

var apiCmd = &cobra.Command{
	Use:   "api <METHOD> <path>",
	Short: "通用飞书 OpenAPI 透传调用（自动鉴权 + 分页/限流/错误码处理）",
	Long: `以最小侵入的方式直接调用任意飞书 OpenAPI 端点。

自动处理：
  • Token：复用 ~/.feishu-cli/token.json 中的 User Token（自动刷新），或回退到 App Token（Bot）
  • URL 规范化：自动剥离 https://open.feishu.cn 前缀；自动补 /open-apis/ 前缀
  • Body：--data 接受 JSON 字符串，--data-file 从文件/stdin 读取
  • Query：--params 接受 JSON 对象（值会转为字符串）
  • 输出：默认 pretty JSON；--raw 原样输出；--include-headers 附响应头

身份选择 (--as)：
  bot   = 强制 App/Tenant Token（Bot 身份）
  user  = 强制 User Token（找不到时报错）
  auto  = 优先 User Token，找不到回退到 App Token（默认）

示例：
  # 获取当前登录用户信息（User Token）
  feishu-cli api GET /open-apis/authen/v1/user_info --as user

  # 获取消息历史（query 参数）
  feishu-cli api GET /open-apis/im/v1/messages \
    --params '{"container_id_type":"chat","container_id":"oc_xxx","page_size":50}'

  # 发送消息（body）
  feishu-cli api POST /open-apis/im/v1/messages \
    --params '{"receive_id_type":"email"}' \
    --data '{"receive_id":"u@example.com","msg_type":"text","content":"{\"text\":\"hi\"}"}'

  # 预览将要发出的请求（不实际调用）
  feishu-cli api POST /open-apis/im/v1/messages --data '...' --dry-run

  # 从 stdin 读 body
  cat body.json | feishu-cli api POST /open-apis/xxx --data-file -

  # 直接传完整 URL 也行
  feishu-cli api GET https://open.feishu.cn/open-apis/contact/v3/users/me --as user`,
	Args: cobra.ExactArgs(2),
	RunE: runAPI,
}

func init() {
	apiCmd.Flags().StringVarP(&apiParams, "params", "p", "", "Query 参数（JSON 对象，如 '{\"page_size\":100}'）")
	apiCmd.Flags().StringVarP(&apiData, "data", "d", "", "请求体（JSON 字符串）")
	apiCmd.Flags().StringVar(&apiDataFile, "data-file", "", "从文件读请求体（用 - 表示 stdin）")
	apiCmd.Flags().StringVar(&apiAs, "as", "auto", "Token 类型: bot | user | auto（默认 auto = User 优先，回退 Bot）")
	apiCmd.Flags().StringVarP(&apiOutput, "output", "o", "", "响应写入文件（默认 stdout）")
	apiCmd.Flags().BoolVar(&apiDryRun, "dry-run", false, "仅打印将要发出的请求，不实际调用")
	apiCmd.Flags().BoolVar(&apiRaw, "raw", false, "原样输出响应 body（不做 pretty JSON）")
	apiCmd.Flags().BoolVar(&apiIncludeHeaders, "include-headers", false, "在 stderr 打印响应状态码和响应头")
	apiCmd.Flags().IntVar(&apiTimeoutSec, "timeout", 30, "请求超时（秒）")
	apiCmd.Flags().StringVar(&apiFormat, "format", "", "输出格式: json|pretty|table|ndjson|csv（指定后走内置渲染，覆盖默认 pretty）")
	apiCmd.Flags().StringVar(&apiJQ, "jq", "", "用 jq 表达式过滤响应（内置 gojq，无需外部 jq）")
	apiCmd.Flags().String("user-access-token", "", "显式传入 User Access Token（覆盖 --as）")

	rootCmd.AddCommand(apiCmd)
}

func runAPI(cmd *cobra.Command, args []string) error {
	method := strings.ToUpper(strings.TrimSpace(args[0]))
	if !isValidHTTPMethod(method) {
		return fmt.Errorf("不支持的 HTTP method %q，可选: GET, POST, PUT, DELETE, PATCH", method)
	}

	apiPath, embeddedQuery, err := normalizeAPIPath(args[1])
	if err != nil {
		return err
	}

	// 解析 query 参数：优先合并 path 中内嵌的 query，再用 --params 追加/覆盖
	queryParams, err := parseQueryParams(apiParams)
	if err != nil {
		return fmt.Errorf("解析 --params 失败: %w", err)
	}
	for k, vals := range embeddedQuery {
		if _, override := queryParams[k]; override {
			continue // --params 显式优先
		}
		for _, v := range vals {
			queryParams.Add(k, v)
		}
	}

	// 解析 body
	bodyBytes, err := loadAPIBody(apiData, apiDataFile)
	if err != nil {
		return err
	}
	var body any
	if len(bodyBytes) > 0 {
		// 校验是合法 JSON（防止用户传 raw text 调一些 JSON-only API）
		var probe any
		if err := json.Unmarshal(bodyBytes, &probe); err != nil {
			return fmt.Errorf("--data/--data-file 不是合法 JSON: %w", err)
		}
		body = probe
	}

	// 解析 token 策略
	tokenTypes, userToken, err := resolveAPIToken(cmd, apiAs)
	if err != nil {
		return err
	}

	// dry-run：打印请求后返回
	if apiDryRun {
		return printAPIDryRun(method, apiPath, queryParams, body, tokenTypes, userToken != "")
	}

	// 构造请求
	req := &larkcore.ApiReq{
		HttpMethod:                method,
		ApiPath:                   apiPath,
		QueryParams:               queryParams,
		Body:                      body,
		SupportedAccessTokenTypes: tokenTypes,
	}

	cli, err := client.GetClient()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(apiTimeoutSec)*time.Second)
	defer cancel()

	var opts []larkcore.RequestOptionFunc
	if userToken != "" {
		opts = append(opts, larkcore.WithUserAccessToken(userToken))
	}

	resp, err := cli.Do(ctx, req, opts...)
	if err != nil {
		return fmt.Errorf("API 调用失败: %w", err)
	}

	// 输出响应头到 stderr（可选）
	if apiIncludeHeaders {
		fmt.Fprintf(os.Stderr, "HTTP/1.1 %d\n", resp.StatusCode)
		printRespHeaders(os.Stderr, resp.Header)
		fmt.Fprintln(os.Stderr)
	}

	// 输出 body：显式 --format / --jq 时走 internal/output（jq 过滤 + table/csv/ndjson + 大整数保精度）；
	// 否则保持默认 pretty/raw 行为（含 -o 二进制写文件）。
	if apiFormat != "" || apiJQ != "" {
		o, oerr := output.NewOptions(apiFormat, apiJQ)
		if oerr != nil {
			return oerr
		}
		o.OutputFile = apiOutput
		var parsed any
		dec := json.NewDecoder(bytes.NewReader(resp.RawBody))
		dec.UseNumber()
		if err := dec.Decode(&parsed); err != nil {
			return fmt.Errorf("响应不是合法 JSON，无法用 --format/--jq 渲染（去掉这两个 flag 可用 --raw 原样输出）: %w", err)
		}
		if err := output.Render(o, parsed); err != nil {
			return err
		}
	} else {
		outWriter := io.Writer(os.Stdout)
		if apiOutput != "" {
			f, err := os.Create(apiOutput)
			if err != nil {
				return fmt.Errorf("打开输出文件失败: %w", err)
			}
			defer f.Close()
			outWriter = f
		}
		if err := writeAPIResponse(outWriter, resp.RawBody, apiRaw); err != nil {
			return err
		}
	}

	// 业务错误码提示（飞书：code != 0 表示业务错误）
	if hint := detectFeishuBizError(resp.StatusCode, resp.RawBody); hint != "" {
		fmt.Fprintln(os.Stderr, hint)
	}

	// 非 2xx 返回非零退出码（响应已打印）
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return nil
}

// isValidHTTPMethod 仅放行飞书 OpenAPI 实际用到的方法
func isValidHTTPMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
		return true
	}
	return false
}

// normalizeAPIPath 把用户输入的 path 规范化为 SDK 需要的 /open-apis/... 格式
//   - 完整 URL（https://open.feishu.cn/...）→ 剥掉 scheme+host
//   - 缺 / 开头 → 补
//   - 缺 /open-apis/ → 补
//   - 如果 path 内含 ?xxx=yyy → 拆解为 query 参数返回（让 caller 合并到 --params）
func normalizeAPIPath(raw string) (string, larkcore.QueryParams, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil, fmt.Errorf("path 不能为空")
	}

	// 剥掉常见的 base URL 前缀
	for _, prefix := range []string{
		"https://open.feishu.cn",
		"http://open.feishu.cn",
		"https://open.larksuite.com",
		"http://open.larksuite.com",
		"https://open.larkoffice.com",
		"http://open.larkoffice.com",
	} {
		if rest, ok := strings.CutPrefix(s, prefix); ok {
			s = rest
			break
		}
	}

	// 拆解 path 内嵌的 query string（用户可能从浏览器粘贴完整 URL）
	embedded := larkcore.QueryParams{}
	if idx := strings.Index(s, "?"); idx >= 0 {
		qstr := s[idx+1:]
		s = s[:idx]
		vs, err := url.ParseQuery(qstr)
		if err != nil {
			return "", nil, fmt.Errorf("解析 path 中的 query string 失败: %w", err)
		}
		for k, vals := range vs {
			for _, v := range vals {
				embedded.Add(k, v)
			}
		}
	}

	// 剥 fragment（# 后内容飞书 API 用不到）
	if idx := strings.Index(s, "#"); idx >= 0 {
		s = s[:idx]
	}

	if !strings.HasPrefix(s, "/") {
		s = "/" + s
	}

	if !strings.HasPrefix(s, "/open-apis/") {
		// 容忍用户写 /im/v1/messages 这种短路径
		s = "/open-apis" + s
	}

	return s, embedded, nil
}

// parseQueryParams 把 JSON 对象转成 QueryParams（map[string][]string）
// 支持值类型：string / number / bool / array (会展开为多值)
func parseQueryParams(raw string) (larkcore.QueryParams, error) {
	q := larkcore.QueryParams{}
	if strings.TrimSpace(raw) == "" {
		return q, nil
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil, fmt.Errorf("--params 必须是 JSON 对象: %w", err)
	}
	for k, v := range obj {
		switch tv := v.(type) {
		case string:
			q.Set(k, tv)
		case bool:
			q.Set(k, fmt.Sprintf("%v", tv))
		case float64: // JSON 数字默认 float64
			// 整数尽量不带小数点
			if tv == float64(int64(tv)) {
				q.Set(k, fmt.Sprintf("%d", int64(tv)))
			} else {
				q.Set(k, fmt.Sprintf("%v", tv))
			}
		case nil:
			// 跳过 null
		case []any:
			for _, item := range tv {
				q.Add(k, fmt.Sprintf("%v", item))
			}
		default:
			// 对象/嵌套 → 序列化为 JSON 字符串
			b, _ := json.Marshal(tv)
			q.Set(k, string(b))
		}
	}
	return q, nil
}

// loadAPIBody 解析 --data / --data-file（互斥）
// --data-file 用 "-" 表示 stdin
func loadAPIBody(inline, file string) ([]byte, error) {
	if inline != "" && file != "" {
		return nil, fmt.Errorf("--data 和 --data-file 不能同时使用")
	}
	if inline != "" {
		return []byte(inline), nil
	}
	if file == "" {
		return nil, nil
	}
	if file == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(file)
}

// resolveAPIToken 根据 --as 选择 token 策略
// 返回：SDK 支持的 token 类型列表 + 显式 User Token（如有）
func resolveAPIToken(cmd *cobra.Command, as string) ([]larkcore.AccessTokenType, string, error) {
	as = strings.ToLower(strings.TrimSpace(as))
	switch as {
	case "", "auto":
		userToken := resolveOptionalUserTokenWithFallback(cmd)
		// 同时支持两种，SDK 会根据是否传 WithUserAccessToken 决定
		return []larkcore.AccessTokenType{
			larkcore.AccessTokenTypeTenant,
			larkcore.AccessTokenTypeUser,
		}, userToken, nil

	case "bot", "tenant", "app":
		return []larkcore.AccessTokenType{larkcore.AccessTokenTypeTenant}, "", nil

	case "user":
		userToken, err := resolveRequiredUserToken(cmd)
		if err != nil {
			return nil, "", fmt.Errorf("--as user 需要 User Access Token（请先 `feishu-cli auth login`）: %w", err)
		}
		return []larkcore.AccessTokenType{larkcore.AccessTokenTypeUser}, userToken, nil

	default:
		return nil, "", fmt.Errorf("--as 仅支持 bot|user|auto，得到 %q", as)
	}
}

// printAPIDryRun 把请求详情打印到 stdout
func printAPIDryRun(method, path string, q larkcore.QueryParams, body any, tokenTypes []larkcore.AccessTokenType, hasUserToken bool) error {
	out := map[string]any{
		"method":            method,
		"path":              path,
		"query":             toPlainQuery(q),
		"body":              body,
		"supported_tokens":  tokenTypesToString(tokenTypes),
		"will_use_user_tok": hasUserToken,
		"dry_run":           true,
	}
	return printJSON(out)
}

// toPlainQuery 把 QueryParams (map[string][]string) 转成更可读的形式（单值直接是 string）
func toPlainQuery(q larkcore.QueryParams) map[string]any {
	out := make(map[string]any, len(q))
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		vs := q[k]
		if len(vs) == 1 {
			out[k] = vs[0]
		} else {
			out[k] = vs
		}
	}
	return out
}

func tokenTypesToString(ts []larkcore.AccessTokenType) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, string(t))
	}
	return out
}

func printRespHeaders(w io.Writer, h http.Header) {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "%s: %s\n", k, strings.Join(h[k], ", "))
	}
}

// writeAPIResponse 把响应 body 输出到 w
// 默认尝试 pretty-print JSON，失败或 --raw 时原样写
func writeAPIResponse(w io.Writer, body []byte, raw bool) error {
	if raw {
		_, err := w.Write(body)
		return err
	}
	if len(body) == 0 {
		return nil
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, body, "", "  "); err == nil {
		_, werr := w.Write(pretty.Bytes())
		if werr != nil {
			return werr
		}
		// 补一个换行让输出更友好
		if !bytes.HasSuffix(pretty.Bytes(), []byte("\n")) {
			fmt.Fprintln(w)
		}
		return nil
	}
	// 不是 JSON，原样输出
	_, err := w.Write(body)
	return err
}

// detectFeishuBizError 检查飞书业务错误码并给出友好提示
// 飞书约定：HTTP 200 但 body.code != 0 表示业务错误
func detectFeishuBizError(_ int, body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var env struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return ""
	}
	if env.Code == 0 {
		return ""
	}

	// 已知常见错误码 → 解决建议
	var hint string
	switch env.Code {
	case 99991661, 99991663, 99991668, 99991672, 99991679, 99991677:
		hint = "提示：Token 失效或权限不足。请运行 `feishu-cli auth status` 检查，或 `feishu-cli auth login --recommend` 重新授权。"
	case 1254005, 1254404:
		hint = "提示：资源不存在或无访问权限，请检查 token / ID。"
	case 99991400:
		hint = "提示：请求被限流，请降低并发或稍后重试。"
	case 230001, 230002, 230020:
		hint = "提示：scope 不足。可运行 `feishu-cli auth check --scope \"<所需 scope>\"` 预检并补充权限。"
	case 232033:
		hint = `提示：外部群权限不足。当前 App 未开启「对外共享能力」或 Bot 未加入此群。
  - 切换到对外共享 App 调用：
      FEISHU_APP_ID=cli_xxx FEISHU_APP_SECRET=xxx feishu-cli api ...
  - 详见 skills/feishu-cli-chat/references/external-chat.md`
	case 232011:
		hint = "提示：操作者不在群里。让群管理员邀请进群后重试，或用 `feishu-cli chat member add <chat_id> --id-list <id>`。"
	case 232006:
		hint = "提示：chat_id 无效。可用 `feishu-cli msg search-chats --query \"<群名关键词>\"` 重新查找。"
	case 232025:
		hint = "提示：App 未启用机器人能力。请到飞书开放平台 → 应用 → 应用能力 → 添加「机器人」能力并发布。"
	}

	header := fmt.Sprintf("⚠️  飞书业务错误：code=%d, msg=%s", env.Code, env.Msg)
	if hint != "" {
		return header + "\n" + hint
	}
	return header
}
