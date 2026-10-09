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

	"github.com/itchyny/gojq"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/output"
	"github.com/riba2534/feishu-cli/internal/runctx"
	"github.com/riba2534/feishu-cli/internal/safefile"
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
	apiPageAll        bool
	apiPageLimit      int
	apiPageDelayMs    int
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

  # 可识别 has_more + page_token/next_page_token 的列表接口自动翻页
  feishu-cli api GET /open-apis/im/v1/chats --page-all --page-limit 10 --as user

  # 直接传完整 URL 也行（仅官方 OpenAPI host）
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
	apiCmd.Flags().BoolVar(&apiPageAll, "page-all", false, "自动翻页（仅识别 data.has_more + page_token/next_page_token）")
	apiCmd.Flags().IntVar(&apiPageLimit, "page-limit", 10, "配合 --page-all 的最大页数（0=不限；空/重复 cursor 仍会停止）")
	apiCmd.Flags().IntVar(&apiPageDelayMs, "page-delay", 200, "翻页间隔毫秒")
	apiCmd.Flags().String("user-access-token", "", "显式 User Token（用于 auto/user；--as bot 固定应用身份）")

	rootCmd.AddCommand(apiCmd)
}

func runAPI(cmd *cobra.Command, args []string) error {
	method := strings.ToUpper(strings.TrimSpace(args[0]))
	if !isValidHTTPMethod(method) {
		return clierr.Usagef("不支持的 HTTP method %q，可选: GET, POST, PUT, DELETE, PATCH", method)
	}

	apiPath, embeddedQuery, err := normalizeAPIPath(args[1])
	if err != nil {
		return clierr.Usage(err)
	}

	// 校验 --as 取值合法性（前置验证）
	asLower := strings.ToLower(strings.TrimSpace(apiAs))
	switch asLower {
	case "", "auto", "bot", "tenant", "app", "user":
		// 合法
	default:
		return clierr.Usagef("--as 仅支持 bot|user|auto，得到 %q", apiAs)
	}

	if apiPageLimit < 0 {
		return clierr.Usagef("--page-limit 必须 >= 0，得到 %d", apiPageLimit)
	}
	if apiPageDelayMs < 0 {
		return clierr.Usagef("--page-delay 必须 >= 0，得到 %d", apiPageDelayMs)
	}
	if apiTimeoutSec <= 0 {
		return clierr.Usagef("--timeout 必须 > 0，得到 %d", apiTimeoutSec)
	}

	// 校验 --format / --jq 参数合法性（在网络请求与 token 刷新前验证）
	if apiFormat != "" || apiJQ != "" {
		if _, err := output.NewOptions(apiFormat, apiJQ); err != nil {
			return clierr.Usage(err)
		}
		if apiJQ != "" {
			if _, err := gojq.Parse(apiJQ); err != nil {
				return clierr.Usagef("jq 表达式解析失败: %w", err)
			}
		}
	}

	// 校验 --output 路径合法性（前置验证）
	if apiOutput != "" {
		if err := validateOutputPath(apiOutput, ""); err != nil {
			return err
		}
	}

	// 解析 query 参数：优先合并 path 中内嵌的 query，再用 --params 追加/覆盖
	queryParams, err := parseQueryParams(apiParams)
	if err != nil {
		return clierr.Usagef("解析 --params 失败: %w", err)
	}
	for k, vals := range embeddedQuery {
		if _, override := queryParams[k]; override {
			continue // --params 显式优先
		}
		for _, v := range vals {
			queryParams.Add(k, v)
		}
	}

	// 解析 body（在网络调用前验证合法 JSON）
	bodyBytes, err := loadAPIBody(apiData, apiDataFile)
	if err != nil {
		return err // 互斥 / 敏感路径为用法错误（已打标签），读文件失败为一般错误
	}
	var body any
	if len(bodyBytes) > 0 {
		// 校验是合法 JSON（防止用户传 raw text 调一些 JSON-only API）
		var probe any
		if err := json.Unmarshal(bodyBytes, &probe); err != nil {
			return clierr.Usagef("--data/--data-file 不是合法 JSON: %w", err)
		}
		// 校验通过后用 UseNumber 重新解码：数字保持原始字面量，
		// 否则 19 位 ID 等大整数经 float64 往返会被舍入（如 ...789 → ...800）。
		parsed, err := decodeJSONUseNumber(bodyBytes)
		if err != nil {
			return clierr.Usagef("--data/--data-file 不是合法 JSON: %w", err)
		}
		body = parsed
	}

	if apiPageAll && apiOutput != "" && apiFormat == "" && apiJQ == "" {
		return clierr.Usagef("--output 与 --page-all 不能同时用于二进制下载；去掉其中一个，或给 --page-all 加上 --format/--jq")
	}

	// dry-run：静态检查 token 策略，打印请求后直接返回（不触发 token refresh，不写 token 文件，不发网络请求）
	if apiDryRun {
		tokenTypes, hasUserToken, err := resolveAPITokenDryRun(cmd, apiAs)
		if err != nil {
			return err
		}
		return printAPIDryRun(method, apiPath, queryParams, body, tokenTypes, hasUserToken)
	}

	// 解析 token 策略（auto 模式下若检测到 User 身份但刷新/解析失败，将 fail-closed 报错，绝不静默切 Bot）
	tokenTypes, userToken, err := resolveAPIToken(cmd, apiAs)
	if err != nil {
		return err
	}

	if apiPageAll {
		return runAPIPaginated(method, apiPath, queryParams, body, tokenTypes, userToken)
	}

	resp, err := invokeAPI(method, apiPath, queryParams, body, tokenTypes, userToken)
	if err != nil {
		return err
	}
	return emitAPIResponse(resp)
}

// isValidHTTPMethod 仅放行飞书 OpenAPI 实际用到的方法
func isValidHTTPMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
		return true
	}
	return false
}

func invokeAPI(method, apiPath string, queryParams larkcore.QueryParams, body any, tokenTypes []larkcore.AccessTokenType, userToken string) (*larkcore.ApiResp, error) {
	req := &larkcore.ApiReq{
		HttpMethod:                method,
		ApiPath:                   apiPath,
		QueryParams:               queryParams,
		Body:                      body,
		SupportedAccessTokenTypes: tokenTypes,
	}
	cli, err := client.GetClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(runctx.Root(), time.Duration(apiTimeoutSec)*time.Second)
	defer cancel()
	var opts []larkcore.RequestOptionFunc
	if userToken != "" {
		opts = append(opts, larkcore.WithUserAccessToken(userToken))
	}
	resp, err := cli.Do(ctx, req, opts...)
	if err != nil {
		return nil, fmt.Errorf("API 调用失败: %w", err)
	}
	return resp, nil
}

func emitAPIResponse(resp *larkcore.ApiResp) error {
	return emitAPIBody(resp.StatusCode, resp.Header, resp.RawBody)
}

func emitAPIBody(status int, header http.Header, rawBody []byte) error {
	if apiIncludeHeaders {
		fmt.Fprintf(os.Stderr, "HTTP/1.1 %d\n", status)
		printRespHeaders(os.Stderr, header)
		fmt.Fprintln(os.Stderr)
	}
	// 先按飞书业务信封解析（业务错误常随 HTTP 400 下发），再看 HTTP 状态码
	if respErr := client.ParseAPIResponse("", status, header, rawBody); respErr != nil {
		return emitAPIError(rawBody, respErr)
	}
	if apiFormat != "" || apiJQ != "" {
		o, oerr := output.NewOptions(apiFormat, apiJQ)
		if oerr != nil {
			return oerr
		}
		o.OutputFile = apiOutput
		parsed, err := decodeJSONUseNumber(rawBody)
		if err != nil {
			return fmt.Errorf("响应不是合法 JSON，无法用 --format/--jq 渲染（去掉这两个 flag 可用 --raw 原样输出）: %w", err)
		}
		return output.Render(o, parsed)
	}
	return writeAPIOutput(rawBody, apiRaw)
}

// writeAPIOutput 把响应体写到 stdout 或 --output 文件（原子写入：失败不留半截文件、不破坏原文件）。
func writeAPIOutput(rawBody []byte, raw bool) error {
	if apiOutput == "" {
		return writeAPIResponse(os.Stdout, rawBody, raw)
	}
	var buf bytes.Buffer
	if err := writeAPIResponse(&buf, rawBody, raw); err != nil {
		return err
	}
	if err := safefile.AtomicWriteFile(apiOutput, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("写入输出文件失败: %w", err)
	}
	return nil
}

// emitAPIError 处理业务错误 / HTTP 错误响应：stdout 不输出错误体（--jq/--format 也不处理），
// 避免管道下游把错误 JSON 当成功结果消费；错误信息、诊断与修复建议由根命令写 stderr。
// --raw 保留原样输出响应体的调试能力（stdout 或 --output），退出码仍非 0。
func emitAPIError(rawBody []byte, respErr error) error {
	if apiRaw {
		if err := writeAPIOutput(rawBody, true); err != nil {
			return err
		}
	}
	apiErr, ok := client.AsAPIError(respErr)
	if !ok {
		return respErr // 非飞书信封的 HTTP 错误："HTTP <status>, body: <预览>"
	}
	return &apiBizError{apiErr: apiErr, hint: apiBizErrorHint(apiErr.Code, apiErr.Msg)}
}

// apiBizError 是 api 命令的业务错误：文本保持 "飞书业务错误: code=N, msg=M"，
// Unwrap 到 *client.APIError 供根命令附加 log_id / 缺失 scope 等诊断，Hint 提供 api 专属建议。
type apiBizError struct {
	apiErr *client.APIError
	hint   string
}

func (e *apiBizError) Error() string {
	return fmt.Sprintf("飞书业务错误: code=%d, msg=%s", e.apiErr.Code, e.apiErr.Msg)
}

func (e *apiBizError) Unwrap() error { return e.apiErr }

func (e *apiBizError) Hint() string { return e.hint }

func decodeJSONUseNumber(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var parsed any
	if err := dec.Decode(&parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

// officialOpenAPIHosts 是 `api` 命令允许出现在**用户输入 URL** 中的官方 host。
//
// 比 internal/config.officialOpenHosts 多一个 open.larkoffice.com：后者是租户可见域，
// 用户常直接从浏览器地址栏粘贴。这里不冲突——normalizeAPIPath 只从 URL 里取 path 与 query，
// host 随即被丢弃，真实请求走配置的 BaseURL，因此 transport 层的 CheckRequestURL 永远
// 看不到 larkoffice。改动此处不影响传输层的 host 策略，两者刻意保持不同职责。
func officialOpenAPIHosts() map[string]bool {
	return map[string]bool{
		"open.feishu.cn":      true,
		"open.larksuite.com":  true,
		"open.larkoffice.com": true,
	}
}

func isOfficialOpenAPIHost(host string) bool {
	return officialOpenAPIHosts()[strings.ToLower(host)]
}

// normalizeAPIPath 把用户输入的 path 规范化为 SDK 需要的 /open-apis/... 格式。
// fragment 先于 query 剥离，完整 URL 只接受官方 OpenAPI host，短 path 仍兼容。
func normalizeAPIPath(raw string) (string, larkcore.QueryParams, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil, fmt.Errorf("path 不能为空")
	}

	embedded := larkcore.QueryParams{}

	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil {
			return "", nil, fmt.Errorf("解析 URL 失败: %w", err)
		}
		if u.Scheme != "https" {
			return "", nil, fmt.Errorf("完整 URL 仅支持 https，得到 %q", u.Scheme)
		}
		if !isOfficialOpenAPIHost(u.Hostname()) {
			return "", nil, fmt.Errorf("完整 URL 仅支持官方 OpenAPI host（open.feishu.cn / open.larksuite.com / open.larkoffice.com），得到 %q", u.Hostname())
		}
		for k, vs := range u.Query() {
			for _, v := range vs {
				embedded.Add(k, v)
			}
		}
		s = u.EscapedPath()
		if s == "" {
			s = u.Path
		}
	} else {
		// RFC 3986：先剥 fragment，fragment 绝不能进入 query。
		if idx := strings.Index(s, "#"); idx >= 0 {
			s = s[:idx]
		}
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
	}

	if s == "" {
		s = "/"
	}
	if !strings.HasPrefix(s, "/") {
		s = "/" + s
	}
	if !strings.HasPrefix(s, "/open-apis/") {
		s = "/open-apis" + s
	}
	return s, embedded, nil
}

// ensureNoTrailingJSON 确认 decoder 已读完全部输入。
// json.Decoder 只消费第一个 JSON 值，用于校验用户手写的 JSON 参数，
// 避免 `{"a":1} {"b":2}` 这类输入静默只生效前一半。
func ensureNoTrailingJSON(dec *json.Decoder, flagName string) error {
	var extra json.RawMessage
	if err := dec.Decode(&extra); err == nil {
		return fmt.Errorf("%s 只能是单个 JSON 对象，检测到多余内容: %s", flagName, strings.TrimSpace(string(extra)))
	} else if err != io.EOF {
		return fmt.Errorf("%s 解析失败（尾部有非法内容）: %w", flagName, err)
	}
	return nil
}

// parseQueryParams 把 JSON 对象转成 QueryParams（map[string][]string）
// 支持值类型：string / number / bool / array (会展开为多值)
func parseQueryParams(raw string) (larkcore.QueryParams, error) {
	q := larkcore.QueryParams{}
	if strings.TrimSpace(raw) == "" {
		return q, nil
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil, fmt.Errorf("--params 必须是 JSON 对象: %w", err)
	}
	// 流式 Decoder 只读第一个 JSON 值，尾部残留会被静默丢弃：
	// `--params '{"a":1} {"b":2}'` 会只带上 a，用户却以为两个参数都生效了。
	// 显式拒绝，避免拼错的 JSON 变成"少传了参数"的静默错误。
	if err := ensureNoTrailingJSON(dec, "--params"); err != nil {
		return nil, err
	}
	for k, v := range obj {
		switch tv := v.(type) {
		case string:
			q.Set(k, tv)
		case bool:
			q.Set(k, fmt.Sprintf("%v", tv))
		case json.Number:
			q.Set(k, tv.String())
		case float64: // 兼容非 UseNumber 路径
			if tv == float64(int64(tv)) {
				q.Set(k, fmt.Sprintf("%d", int64(tv)))
			} else {
				q.Set(k, fmt.Sprintf("%v", tv))
			}
		case nil:
			// 跳过 null
		case []any:
			for _, item := range tv {
				q.Add(k, stringifyQueryValue(item))
			}
		default:
			b, _ := json.Marshal(tv)
			q.Set(k, string(b))
		}
	}
	return q, nil
}

func stringifyQueryValue(v any) string {
	switch tv := v.(type) {
	case json.Number:
		return tv.String()
	case string:
		return tv
	default:
		return fmt.Sprintf("%v", tv)
	}
}

// loadAPIBody 解析 --data / --data-file（互斥）
// --data-file 用 "-" 表示 stdin
func loadAPIBody(inline, file string) ([]byte, error) {
	if inline != "" && file != "" {
		return nil, clierr.Usagef("--data 和 --data-file 不能同时使用")
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
	return readLocalInputFile(file)
}

// resolveAPIToken 根据 --as 选择 token 策略
// 返回：SDK 支持的 token 类型列表 + 显式 User Token（如有）
func resolveAPIToken(cmd *cobra.Command, as string) ([]larkcore.AccessTokenType, string, error) {
	as = strings.ToLower(strings.TrimSpace(as))
	switch as {
	case "", "auto":
		userToken, err := resolveAutoUserToken(cmd)
		if err != nil {
			return nil, "", err
		}
		if userToken != "" {
			return []larkcore.AccessTokenType{
				larkcore.AccessTokenTypeTenant,
				larkcore.AccessTokenTypeUser,
			}, userToken, nil
		}
		// 自然未配置 User Token，回退到 Tenant Token
		return []larkcore.AccessTokenType{
			larkcore.AccessTokenTypeTenant,
		}, "", nil

	case "bot", "tenant", "app":
		return []larkcore.AccessTokenType{larkcore.AccessTokenTypeTenant}, "", nil

	case "user":
		userToken, err := resolveRequiredUserToken(cmd)
		if err != nil {
			return nil, "", fmt.Errorf("--as user 需要 User Access Token（请先 `feishu-cli auth login`）: %w", err)
		}
		return []larkcore.AccessTokenType{larkcore.AccessTokenTypeUser}, userToken, nil

	default:
		return nil, "", clierr.Usagef("--as 仅支持 bot|user|auto，得到 %q", as)
	}
}

// resolveAPITokenDryRun 在 dry-run 模式下静态解析 token 策略，不发起任何网络请求，不写 token 文件
func resolveAPITokenDryRun(cmd *cobra.Command, as string) ([]larkcore.AccessTokenType, bool, error) {
	as = strings.ToLower(strings.TrimSpace(as))
	flagToken, _ := cmd.Flags().GetString("user-access-token")
	cfg := config.Get()
	hasUserToken := auth.HasUserTokenConfigured(flagToken, cfg.UserAccessToken)

	switch as {
	case "", "auto":
		if hasUserToken {
			return []larkcore.AccessTokenType{
				larkcore.AccessTokenTypeTenant,
				larkcore.AccessTokenTypeUser,
			}, true, nil
		}
		return []larkcore.AccessTokenType{
			larkcore.AccessTokenTypeTenant,
		}, false, nil

	case "bot", "tenant", "app":
		return []larkcore.AccessTokenType{larkcore.AccessTokenTypeTenant}, false, nil

	case "user":
		if !hasUserToken {
			// 与真实调用（resolveRequiredUserToken 失败）同属鉴权类错误，退出码 3
			return nil, false, clierr.Auth(fmt.Errorf("--as user 需要 User Access Token（请先 `feishu-cli auth login`）: %w", auth.ErrNoUserTokenConfigured))
		}
		return []larkcore.AccessTokenType{larkcore.AccessTokenTypeUser}, true, nil

	default:
		return nil, false, clierr.Usagef("--as 仅支持 bot|user|auto，得到 %q", as)
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
		"page_all":          apiPageAll,
		"page_limit":        apiPageLimit,
	}
	return printJSON(out)
}

func cloneQueryParams(src larkcore.QueryParams) larkcore.QueryParams {
	out := larkcore.QueryParams{}
	for k, vs := range src {
		cp := make([]string, len(vs))
		copy(cp, vs)
		out[k] = cp
	}
	return out
}

func runAPIPaginated(method, apiPath string, queryParams larkcore.QueryParams, body any, tokenTypes []larkcore.AccessTokenType, userToken string) error {
	limit := apiPageLimit
	initial := queryParams.Get("page_token")
	seen := map[string]struct{}{}
	if initial != "" {
		seen[initial] = struct{}{}
	}
	var pages []map[string]any
	var lastResp *larkcore.ApiResp
	token := initial
	truncated := false
	listField := ""

	for page := 1; ; page++ {
		if limit > 0 && page > limit {
			truncated = len(pages) > 0
			break
		}
		q := cloneQueryParams(queryParams)
		if token != "" {
			q.Set("page_token", token)
		}
		resp, err := invokeAPI(method, apiPath, q, body, tokenTypes, userToken)
		if err != nil {
			return err
		}
		lastResp = resp
		if page == 1 && apiIncludeHeaders {
			fmt.Fprintf(os.Stderr, "HTTP/1.1 %d\n", resp.StatusCode)
			printRespHeaders(os.Stderr, resp.Header)
			fmt.Fprintln(os.Stderr)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return emitAPIBody(resp.StatusCode, resp.Header, resp.RawBody)
		}

		parsed, err := decodeJSONUseNumber(resp.RawBody)
		if err != nil {
			if page == 1 {
				return emitAPIBody(resp.StatusCode, resp.Header, resp.RawBody)
			}
			return fmt.Errorf("第 %d 页响应不是合法 JSON: %w", page, err)
		}
		obj, ok := parsed.(map[string]any)
		if !ok {
			return emitAPIBody(resp.StatusCode, resp.Header, resp.RawBody)
		}
		if _, _, hasBizErr := parseFeishuBizError(resp.RawBody); hasBizErr {
			return emitAPIBody(resp.StatusCode, resp.Header, resp.RawBody)
		}

		data, _ := obj["data"].(map[string]any)
		hasMore, hasMoreOK := false, false
		if data != nil {
			hasMore, hasMoreOK = parseHasMoreFlag(data["has_more"])
		}
		if page == 1 {
			if hasMore {
				field, ferr := resolveAPIArrayField(data)
				if ferr != nil {
					return fmt.Errorf("分页第 %d 页无法继续翻页: %w", page, ferr)
				}
				listField = field
			}
		} else {
			if err := requireExactArrayField(data, listField, page); err != nil {
				return err
			}
		}
		pages = append(pages, obj)

		if !hasMoreOK || !hasMore {
			truncated = false
			break
		}
		next, kind := pageCursorFromData(data)
		if kind == "nonstring" {
			return fmt.Errorf("分页第 %d 页游标不是字符串，拒绝猜测非标准游标", page)
		}
		if next == "" {
			return fmt.Errorf("分页第 %d 页 has_more=true 但 page_token/next_page_token 为空，已停止以免静默重复", page)
		}
		if _, dup := seen[next]; dup {
			return fmt.Errorf("分页第 %d 页重复游标 %q，已停止以免静默重复", page, next)
		}
		seen[next] = struct{}{}
		token = next
		if limit > 0 && page == limit {
			truncated = true
			break
		}
		if apiPageDelayMs > 0 {
			time.Sleep(time.Duration(apiPageDelayMs) * time.Millisecond)
		}
	}

	merged, err := mergeAPIPages(pages, truncated, listField)
	if err != nil {
		return err
	}
	raw, err := marshalPreserveNumbers(merged)
	if err != nil {
		return err
	}
	savedInclude := apiIncludeHeaders
	apiIncludeHeaders = false
	defer func() { apiIncludeHeaders = savedInclude }()
	status := 200
	var header http.Header
	if lastResp != nil {
		status = lastResp.StatusCode
		header = lastResp.Header
	}
	return emitAPIBody(status, header, raw)
}

// pageCursorFromData 从 data 中取续翻游标。
//
// 两个 key 都要看：部分端点会同时输出 page_token:"" 与有效的 next_page_token。
// 若在第一个「存在但为空」的 key 上就返回，翻页会被判成"游标为空"而中止，
// 丢掉后续所有页（调用方只看到第 1 页且以为已翻完）。
// 类型不对（非字符串）仍立即返回 nonstring，由调用方 fail-closed。
func pageCursorFromData(data map[string]any) (token string, kind string) {
	if data == nil {
		return "", "missing"
	}
	sawKey := false
	for _, key := range []string{"page_token", "next_page_token"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		s, isStr := v.(string)
		if !isStr {
			return "", "nonstring"
		}
		sawKey = true
		if strings.TrimSpace(s) == "" {
			continue // 空游标：继续看另一个 key
		}
		return s, key
	}
	if sawKey {
		return "", "empty" // key 存在但都是空值
	}
	return "", "missing"
}

// parseHasMoreFlag 解析 has_more 标志，容忍飞书各端点不一致的类型表达。
//
// 响应经 decodeJSONUseNumber 解码，标量类型为 bool / json.Number / string。
// 少数端点把 has_more 表达成 "true" 或 1，若只做 `.(bool)` 断言会得到
// (false, false)：翻页在第 1 页静默停止，且 truncated 被清空，
// 调用方无法区分「只有一页」与「被截断」——正是本文件其它错误分支
// （"已停止以免静默重复"）要防的静默截断。
//
// 返回 (值, 是否可判定)。字段缺失或类型无法解释时返回 (false, false)。
func parseHasMoreFlag(v any) (bool, bool) {
	switch tv := v.(type) {
	case bool:
		return tv, true
	case json.Number:
		n, err := tv.Int64()
		if err != nil {
			return false, false
		}
		return n != 0, true
	case float64: // 兼容非 UseNumber 路径
		return tv != 0, true
	case string:
		switch strings.ToLower(strings.TrimSpace(tv)) {
		case "true", "1":
			return true, true
		case "false", "0", "":
			return false, true
		}
		return false, false
	default:
		return false, false
	}
}

func resolveAPIArrayField(data map[string]any) (string, error) {
	if data == nil {
		return "", fmt.Errorf("响应 data 不是对象，无法识别列表字段")
	}
	known := []string{
		"items", "files", "events", "rooms", "records", "nodes",
		"members", "departments", "calendar_list", "acl_list", "freebusy_list",
		"users",
	}
	var knownHits []string
	for _, name := range known {
		if _, ok := data[name].([]any); ok {
			knownHits = append(knownHits, name)
		}
	}
	if len(knownHits) == 1 {
		return knownHits[0], nil
	}
	if len(knownHits) > 1 {
		return "", fmt.Errorf("响应含多个可识别列表字段 %s，拒绝猜测", strings.Join(knownHits, "/"))
	}
	var unknown []string
	for k, v := range data {
		if _, ok := v.([]any); ok {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 1 {
		return unknown[0], nil
	}
	if len(unknown) == 0 {
		return "", fmt.Errorf("响应没有可识别的列表数组，拒绝继续翻页")
	}
	sort.Strings(unknown)
	return "", fmt.Errorf("响应含多个未知列表字段 %s，拒绝按字母猜测", strings.Join(unknown, "/"))
}

func requireExactArrayField(data map[string]any, field string, page int) error {
	if field == "" {
		return fmt.Errorf("分页第 %d 页缺少已锁定的列表字段", page)
	}
	if data == nil {
		return fmt.Errorf("分页第 %d 页 data 不是对象，缺少列表字段 %q", page, field)
	}
	if _, ok := data[field].([]any); !ok {
		return fmt.Errorf("分页第 %d 页列表字段 %q 缺失或不是数组", page, field)
	}
	return nil
}

func mergeAPIPages(pages []map[string]any, truncated bool, listField string) (map[string]any, error) {
	if len(pages) == 0 {
		return map[string]any{}, nil
	}
	if len(pages) == 1 && !truncated {
		return pages[0], nil
	}
	first := pages[0]
	data, ok := first["data"].(map[string]any)
	if !ok {
		if len(pages) == 1 {
			return first, nil
		}
		return nil, fmt.Errorf("分页聚合失败：首页 data 不是对象")
	}
	field := listField
	if field == "" {
		resolved, err := resolveAPIArrayField(data)
		if err != nil {
			return nil, err
		}
		field = resolved
	}
	var merged []any
	for i, p := range pages {
		d, _ := p["data"].(map[string]any)
		if err := requireExactArrayField(d, field, i+1); err != nil {
			return nil, err
		}
		items, _ := d[field].([]any)
		merged = append(merged, items...)
	}
	outData := make(map[string]any, len(data)+4)
	for k, v := range data {
		outData[k] = v
	}
	outData[field] = merged
	lastHasMore := false
	var lastData map[string]any
	if last, ok := pages[len(pages)-1]["data"].(map[string]any); ok {
		lastData = last
		lastHasMore, _ = last["has_more"].(bool)
	}
	outData["has_more"] = lastHasMore
	outData["page_count"] = len(pages)
	exhausted := !truncated && !lastHasMore
	if exhausted {
		delete(outData, "page_token")
		delete(outData, "next_page_token")
		delete(outData, "truncated")
	} else {
		outData["truncated"] = true
		delete(outData, "page_token")
		delete(outData, "next_page_token")
		if lastData != nil {
			if v, ok := lastData["page_token"]; ok {
				outData["page_token"] = v
			}
			if v, ok := lastData["next_page_token"]; ok {
				outData["next_page_token"] = v
			}
		}
	}
	result := make(map[string]any, len(first)+1)
	for k, v := range first {
		result[k] = v
	}
	result["data"] = outData
	return result, nil
}

func marshalPreserveNumbers(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
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

// parseFeishuBizError 解析飞书响应体中的业务错误码
func parseFeishuBizError(body []byte) (int, string, bool) {
	if len(body) == 0 {
		return 0, "", false
	}
	var env struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return 0, "", false
	}
	if env.Code == 0 {
		return 0, "", false
	}
	return env.Code, env.Msg, true
}

// apiBizErrorHint 返回 api 命令遇到业务错误时的修复建议：
// 鉴权 / 权限 / 限流等跨领域错误码由 client.APIErrorHint 统一给出（根命令打印），
// 这里只补充 api 透传场景常见的领域错误码。
func apiBizErrorHint(code int, msg string) string {
	switch code {
	case 1254005, 1254404:
		return "提示：资源不存在或无访问权限，请检查 token / ID。"
	case 230001:
		return "提示：请求参数无效（230001），请对照接口文档检查参数名、取值与格式（可用 `feishu-cli schema` 查看参数定义）。"
	case 230002:
		return "提示：Bot 或用户不在该群内（230002），请先把 Bot 拉进群，或确认 chat_id 与调用身份。"
	case 230020:
		return "提示：触发该接口的频率限制（230020），请降低调用频率后重试。"
	case 232033:
		return `提示：外部群权限不足。当前 App 未开启「对外共享能力」或 Bot 未加入此群。
  - 切换到对外共享 App 调用：
      FEISHU_APP_ID=cli_xxx FEISHU_APP_SECRET=xxx feishu-cli api ...
  - 详见 skills/feishu-cli-messaging/references/workflows/chat/references/external-chat.md`
	case 232011:
		return "提示：操作者不在群里。让群管理员邀请进群后重试，或用 `feishu-cli chat member add <chat_id> --id-list <id>`。"
	case 232006:
		return "提示：chat_id 无效。可用 `feishu-cli msg search-chats --query \"<群名关键词>\"` 重新查找。"
	case 232025:
		return "提示：App 未启用机器人能力。请到飞书开放平台 → 应用 → 应用能力 → 添加「机器人」能力并发布。"
	}
	return ""
}
