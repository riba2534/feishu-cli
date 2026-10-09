package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

func resetAPIFlags() {
	apiParams = ""
	apiData = ""
	apiDataFile = ""
	apiAs = "auto"
	apiOutput = ""
	apiDryRun = false
	apiRaw = false
	apiIncludeHeaders = false
	apiTimeoutSec = 30
	apiFormat = ""
	apiJQ = ""
	apiPageAll = false
	apiPageLimit = 10
	apiPageDelayMs = 200
}

func newTestAPICmd() *cobra.Command {
	c := &cobra.Command{
		Use:  "api <METHOD> <path>",
		Args: cobra.ExactArgs(2),
		RunE: runAPI,
	}
	c.Flags().StringVarP(&apiParams, "params", "p", "", "")
	c.Flags().StringVarP(&apiData, "data", "d", "", "")
	c.Flags().StringVar(&apiDataFile, "data-file", "", "")
	c.Flags().StringVar(&apiAs, "as", "auto", "")
	c.Flags().StringVarP(&apiOutput, "output", "o", "", "")
	c.Flags().BoolVar(&apiDryRun, "dry-run", false, "")
	c.Flags().BoolVar(&apiRaw, "raw", false, "")
	c.Flags().BoolVar(&apiIncludeHeaders, "include-headers", false, "")
	c.Flags().IntVar(&apiTimeoutSec, "timeout", 30, "")
	c.Flags().StringVar(&apiFormat, "format", "", "")
	c.Flags().StringVar(&apiJQ, "jq", "", "")
	c.Flags().BoolVar(&apiPageAll, "page-all", false, "")
	c.Flags().IntVar(&apiPageLimit, "page-limit", 10, "")
	c.Flags().IntVar(&apiPageDelayMs, "page-delay", 200, "")
	c.Flags().String("user-access-token", "", "")
	return c
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func TestNormalizeAPIPath(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantPath  string
		wantQuery map[string]string
		wantErr   bool
	}{
		{
			name:     "短路径自动补 /open-apis/",
			input:    "/im/v1/messages",
			wantPath: "/open-apis/im/v1/messages",
		},
		{
			name:     "已有 /open-apis/ 前缀保持原样",
			input:    "/open-apis/im/v1/messages",
			wantPath: "/open-apis/im/v1/messages",
		},
		{
			name:     "完整 URL 自动剥前缀",
			input:    "https://open.feishu.cn/open-apis/authen/v1/user_info",
			wantPath: "/open-apis/authen/v1/user_info",
		},
		{
			name:     "larksuite.com 前缀也剥",
			input:    "https://open.larksuite.com/open-apis/contact/v3/users",
			wantPath: "/open-apis/contact/v3/users",
		},
		{
			name:     "larkoffice.com 前缀也剥",
			input:    "https://open.larkoffice.com/open-apis/im/v1/chats",
			wantPath: "/open-apis/im/v1/chats",
		},
		{
			name:     "缺斜杠自动补",
			input:    "im/v1/messages",
			wantPath: "/open-apis/im/v1/messages",
		},
		{
			name:      "path 内嵌 query 自动拆解",
			input:     "/open-apis/foo?a=1&b=2",
			wantPath:  "/open-apis/foo",
			wantQuery: map[string]string{"a": "1", "b": "2"},
		},
		{
			name:      "完整 URL + query 同时处理",
			input:     "https://open.feishu.cn/open-apis/foo?x=y",
			wantPath:  "/open-apis/foo",
			wantQuery: map[string]string{"x": "y"},
		},
		{
			name:     "fragment 被忽略",
			input:    "/open-apis/foo#section",
			wantPath: "/open-apis/foo",
		},
		{
			name:      "fragment 不含 query：先剥 # 再解析 ?",
			input:     "/open-apis/foo?a=1#frag?b=2",
			wantPath:  "/open-apis/foo",
			wantQuery: map[string]string{"a": "1"},
		},
		{
			name:     "纯 fragment 中的问号不得进入 query",
			input:    "/open-apis/foo#section?x=1",
			wantPath: "/open-apis/foo",
		},
		{
			name:      "完整官方 URL 的 fragment 不进入 query",
			input:     "https://open.feishu.cn/open-apis/foo?x=y#hash",
			wantPath:  "/open-apis/foo",
			wantQuery: map[string]string{"x": "y"},
		},
		{
			name:    "非官方完整 URL 拒绝",
			input:   "https://example.com/open-apis/foo",
			wantErr: true,
		},
		{
			name:    "租户文档 host 拒绝",
			input:   "https://tenant.feishu.cn/open-apis/foo",
			wantErr: true,
		},
		{
			name:    "完整 URL 必须 https",
			input:   "http://open.feishu.cn/open-apis/foo",
			wantErr: true,
		},
		{
			name:    "空字符串报错",
			input:   "",
			wantErr: true,
		},
		{
			name:    "纯空白报错",
			input:   "   ",
			wantErr: true,
		},
		{
			name:     "path 中含真实 ID（如 message_id）保持原样",
			input:    "/open-apis/im/v1/messages/om_xxxxxx",
			wantPath: "/open-apis/im/v1/messages/om_xxxxxx",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path, q, err := normalizeAPIPath(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("期望 err，实际 nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("非预期 err: %v", err)
			}
			if path != tc.wantPath {
				t.Errorf("path = %q，期望 %q", path, tc.wantPath)
			}
			if tc.wantQuery == nil && q.Get("b") != "" {
				t.Errorf("fragment 中的参数不应进入 query，得到 b=%q", q.Get("b"))
			}
			if tc.wantQuery == nil && q.Get("x") != "" {
				t.Errorf("fragment 中的参数不应进入 query，得到 x=%q", q.Get("x"))
			}
			for k, v := range tc.wantQuery {
				if got := q.Get(k); got != v {
					t.Errorf("query[%s] = %q，期望 %q", k, got, v)
				}
			}
			if tc.wantQuery != nil {
				if got := q.Get("b"); got != "" && tc.wantQuery["b"] == "" {
					t.Errorf("fragment 泄漏到 query: b=%q", got)
				}
			}
		})
	}
}

func TestParseQueryParams(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    map[string]string
		wantErr bool
	}{
		{"空字符串", "", map[string]string{}, false},
		{"纯空白", "   ", map[string]string{}, false},
		{"简单字符串", `{"a":"1","b":"x"}`, map[string]string{"a": "1", "b": "x"}, false},
		{"整数自动转字符串", `{"page_size":100}`, map[string]string{"page_size": "100"}, false},
		{"大整数不四舍五入", `{"message_id":9007199254740993}`, map[string]string{"message_id": "9007199254740993"}, false},
		{"小数保留", `{"x":1.5}`, map[string]string{"x": "1.5"}, false},
		{"布尔值", `{"flag":true}`, map[string]string{"flag": "true"}, false},
		{"null 被跳过", `{"a":"1","b":null}`, map[string]string{"a": "1"}, false},
		{"嵌套对象序列化为 JSON", `{"obj":{"k":"v"}}`, map[string]string{"obj": `{"k":"v"}`}, false},
		{"非 JSON 报错", `not-json`, nil, true},
		{"数组而非对象报错", `[1,2]`, nil, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q, err := parseQueryParams(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("期望 err，实际 nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("非预期 err: %v", err)
			}
			for k, want := range tc.want {
				if got := q.Get(k); got != want {
					t.Errorf("query[%s] = %q，期望 %q", k, got, want)
				}
			}
			if tc.name == "null 被跳过" {
				if _, exists := q["b"]; exists {
					t.Errorf("key b 不应存在（null 应被跳过）")
				}
			}
		})
	}
}

func TestParseQueryParamsArray(t *testing.T) {
	q, err := parseQueryParams(`{"fields":["name","email","mobile"]}`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	got := q["fields"]
	want := []string{"name", "email", "mobile"}
	if len(got) != len(want) {
		t.Fatalf("len = %d，期望 %d", len(got), len(want))
	}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("fields[%d] = %q，期望 %q", i, got[i], v)
		}
	}
}

func TestIsValidHTTPMethod(t *testing.T) {
	valid := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}
	for _, m := range valid {
		if !isValidHTTPMethod(m) {
			t.Errorf("isValidHTTPMethod(%q) = false，期望 true", m)
		}
	}
	invalid := []string{"BOGUS", "get", "", "HEAD", "OPTIONS"}
	for _, m := range invalid {
		if isValidHTTPMethod(m) {
			t.Errorf("isValidHTTPMethod(%q) = true，期望 false", m)
		}
	}
}

// TestAPIBizErrorDiagnostics 验证 api 业务错误由根命令渲染修复建议与诊断（stderr），
// 错误主文本保持 "飞书业务错误: code=N, msg=M"。
func TestAPIBizErrorDiagnostics(t *testing.T) {
	resetAPIFlags()
	defer resetAPIFlags()
	t.Setenv("FEISHU_APP_ID", "cli_test_app")
	t.Setenv("FEISHU_APP_SECRET", "test_secret")
	t.Setenv("FEISHU_BASE_URL", "")
	initTestConfig(t)
	tests := []struct {
		name      string
		body      string
		wantLines []string
		notLines  []string
	}{
		{
			name:      "99991679 用户未授权 → auth login --scope 最窄 scope",
			body:      `{"code":99991679,"msg":"Unauthorized.","error":{"permission_violations":[{"subject":"okr:okr.period:readonly"},{"subject":"okr:okr"}]}}`,
			wantLines: []string{`auth login --scope "okr:okr.period:readonly"`, "所需 scope（满足其一即可）: okr:okr.period:readonly, okr:okr"},
		},
		{
			name:      "99991672 应用未开通 → 指向开放平台，不提示重新登录修复",
			body:      `{"code":99991672,"msg":"Access denied.","error":{"log_id":"LOGID123","permission_violations":[{"subject":"okr:okr:readonly"}]}}`,
			wantLines: []string{"开放平台", "https://open.feishu.cn/app/cli_test_app/auth?q=okr%3Aokr%3Areadonly", "重新 auth login 无法解决", "log_id: LOGID123"},
			notLines:  []string{"auth login --recommend"},
		},
		{
			name:      "99991668 not support → --as bot",
			body:      `{"code":99991668,"msg":"user access token not support"}`,
			wantLines: []string{"--as bot"},
		},
		{
			name:      "99991668 token 无效 → 重新登录",
			body:      `{"code":99991668,"msg":"Invalid access token for authorization."}`,
			wantLines: []string{"auth login"},
			notLines:  []string{"--as bot"},
		},
		{
			name:      "230001 是参数无效，不是 scope 不足",
			body:      `{"code":230001,"msg":"param is invalid"}`,
			wantLines: []string{"参数无效"},
			notLines:  []string{"scope"},
		},
		{
			name:      "限流 99991400",
			body:      `{"code":99991400,"msg":"frequency limit"}`,
			wantLines: []string{"限流"},
		},
		{
			name:      "外部群 232033 指向现有排错文档",
			body:      `{"code":232033,"msg":"forbidden"}`,
			wantLines: []string{"skills/feishu-cli-messaging/references/workflows/chat/references/external-chat.md"},
		},
		{
			name:      "字段校验与 log_id 不再丢失",
			body:      `{"code":99992402,"msg":"field validation failed","error":{"log_id":"LOG42","field_violations":[{"field":"document_id","description":"the min len is 27"}]}}`,
			wantLines: []string{"字段校验: document_id: the min len is 27", "log_id: LOG42"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := emitAPIError([]byte(tc.body), client.ParseAPIResponse("", http.StatusBadRequest, nil, []byte(tc.body)))
			if err == nil {
				t.Fatal("业务错误应返回非 nil")
			}
			if !strings.HasPrefix(err.Error(), "飞书业务错误: code=") {
				t.Fatalf("错误主文本应保持原格式，得到 %q", err.Error())
			}
			rendered := strings.Join(renderErrorDiagnostics(err), "\n")
			for _, want := range tc.wantLines {
				if !strings.Contains(rendered, want) {
					t.Errorf("诊断应包含 %q，实际:\n%s", want, rendered)
				}
			}
			for _, not := range tc.notLines {
				if strings.Contains(rendered, not) {
					t.Errorf("诊断不应包含 %q，实际:\n%s", not, rendered)
				}
			}
		})
	}
}

func TestParseFeishuBizError(t *testing.T) {
	tests := []struct {
		name       string
		body       []byte
		wantCode   int
		wantMsg    string
		wantHasErr bool
	}{
		{
			name:       "成功 code 0",
			body:       []byte(`{"code":0,"msg":"success"}`),
			wantHasErr: false,
		},
		{
			name:       "非零 code",
			body:       []byte(`{"code":99991679,"msg":"Unauthorized."}`),
			wantCode:   99991679,
			wantMsg:    "Unauthorized.",
			wantHasErr: true,
		},
		{
			name:       "非 JSON",
			body:       []byte(`not json`),
			wantHasErr: false,
		},
		{
			name:       "空 body",
			body:       []byte{},
			wantHasErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, msg, hasErr := parseFeishuBizError(tc.body)
			if hasErr != tc.wantHasErr {
				t.Fatalf("hasErr = %v, want %v", hasErr, tc.wantHasErr)
			}
			if hasErr {
				if code != tc.wantCode || msg != tc.wantMsg {
					t.Errorf("got code=%d msg=%q, want code=%d msg=%q", code, msg, tc.wantCode, tc.wantMsg)
				}
			}
		})
	}
}

func isolateAPITestEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FEISHU_PROFILE", "")
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
	t.Setenv("FEISHU_BASE_URL", "")
	t.Setenv("FEISHU_APP_ID", "")
	t.Setenv("FEISHU_APP_SECRET", "")
	resetAPIFlags()
}

// TestRunAPI_PreflightValidation 验证非法参数组合在发起任何网络请求前直接报错
func TestRunAPI_PreflightValidation(t *testing.T) {
	isolateAPITestEnv(t)
	defer resetAPIFlags()

	var serverHits int32
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&serverHits, 1)
		http.Error(w, "should not be called", http.StatusInternalServerError)
	})
	defer cleanup()

	tests := []struct {
		name       string
		args       []string
		setup      func(cmd *cobra.Command)
		wantErrSub string
	}{
		{
			name:       "非法 HTTP Method",
			args:       []string{"INVALID_METHOD", "/open-apis/im/v1/messages"},
			setup:      func(cmd *cobra.Command) {},
			wantErrSub: "不支持的 HTTP method",
		},
		{
			name:       "非法 --as",
			args:       []string{"GET", "/open-apis/im/v1/messages"},
			setup:      func(cmd *cobra.Command) { apiAs = "invalid_identity" },
			wantErrSub: "--as 仅支持 bot|user|auto",
		},
		{
			name:       "非法 --format",
			args:       []string{"GET", "/open-apis/im/v1/messages"},
			setup:      func(cmd *cobra.Command) { apiFormat = "xml" },
			wantErrSub: "不支持的 --format",
		},
		{
			name:       "非法 --jq 表达式",
			args:       []string{"GET", "/open-apis/im/v1/messages"},
			setup:      func(cmd *cobra.Command) { apiJQ = ".[invalid" },
			wantErrSub: "jq 表达式解析失败",
		},
		{
			name:       "非法 --params JSON",
			args:       []string{"GET", "/open-apis/im/v1/messages"},
			setup:      func(cmd *cobra.Command) { apiParams = "{not-json}" },
			wantErrSub: "解析 --params 失败",
		},
		{
			name:       "非法 --timeout",
			args:       []string{"GET", "/open-apis/im/v1/messages"},
			setup:      func(cmd *cobra.Command) { apiTimeoutSec = 0 },
			wantErrSub: "--timeout 必须 > 0",
		},
		{
			name:       "非法 --page-limit",
			args:       []string{"GET", "/open-apis/im/v1/messages"},
			setup:      func(cmd *cobra.Command) { apiPageLimit = -1 },
			wantErrSub: "--page-limit 必须 >= 0",
		},
		{
			name:       "非法 --page-delay",
			args:       []string{"GET", "/open-apis/im/v1/messages"},
			setup:      func(cmd *cobra.Command) { apiPageDelayMs = -1 },
			wantErrSub: "--page-delay 必须 >= 0",
		},
		{
			name:       "非法 --data JSON",
			args:       []string{"POST", "/open-apis/im/v1/messages"},
			setup:      func(cmd *cobra.Command) { apiData = "not-json" },
			wantErrSub: "--data/--data-file 不是合法 JSON",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetAPIFlags()
			atomic.StoreInt32(&serverHits, 0)
			cmd := newTestAPICmd()
			tc.setup(cmd)

			err := cmd.RunE(cmd, tc.args)
			if err == nil {
				t.Fatalf("期望前置验证报错，实际返回 nil")
			}
			if !strings.Contains(err.Error(), tc.wantErrSub) {
				t.Errorf("错误信息 = %q，期望包含 %q", err.Error(), tc.wantErrSub)
			}
			if got := exitCodeFor(err); got != 2 {
				t.Errorf("参数校验失败应为用法错误（退出码 2），得到 %d", got)
			}
			if hits := atomic.LoadInt32(&serverHits); hits != 0 {
				t.Errorf("前置验证失败时不应发出任何网络请求，实际收到 %d 次请求", hits)
			}
		})
	}
}

// TestRunAPI_DryRun_NoNetworkAndNoTokenRefresh 验证 --dry-run 不解析/刷新真实 token、不写 token 文件、不发网络请求
func TestRunAPI_DryRun_NoNetworkAndNoTokenRefresh(t *testing.T) {
	isolateAPITestEnv(t)
	defer resetAPIFlags()

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")

	// 写入一个已过期的 token.json（带有有效 refresh_token）
	tokenDir := filepath.Join(tmpHome, ".feishu-cli")
	_ = os.MkdirAll(tokenDir, 0700)
	tokenFile := filepath.Join(tokenDir, "token.json")
	initialStore := auth.TokenStore{
		AccessToken:      "expired-access-token",
		RefreshToken:     "valid-refresh-token",
		TokenType:        "Bearer",
		ExpiresAt:        time.Now().Add(-1 * time.Hour), // 已过期
		RefreshExpiresAt: time.Now().Add(24 * time.Hour),
		Scope:            "im:message",
	}
	data, _ := json.MarshalIndent(initialStore, "", "  ")
	if err := os.WriteFile(tokenFile, data, 0600); err != nil {
		t.Fatalf("写入 token.json 失败: %v", err)
	}

	hashBefore, err := fileSHA256(tokenFile)
	if err != nil {
		t.Fatalf("计算 token.json hash 失败: %v", err)
	}

	var serverHits int32
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&serverHits, 1)
		http.Error(w, "dry-run 不应调用任何服务端接口", http.StatusInternalServerError)
	})
	defer cleanup()

	for _, asMode := range []string{"auto", "user", "bot"} {
		t.Run("as="+asMode, func(t *testing.T) {
			cmd := newTestAPICmd()
			apiDryRun = true
			apiAs = asMode

			err := cmd.RunE(cmd, []string{"POST", "/open-apis/im/v1/messages"})
			if err != nil {
				t.Fatalf("dry-run 模式执行失败: %v", err)
			}

			if hits := atomic.LoadInt32(&serverHits); hits != 0 {
				t.Errorf("--dry-run 模式下不应触发任何网络请求，得到 %d 次请求", hits)
			}

			hashAfter, err := fileSHA256(tokenFile)
			if err != nil {
				t.Fatalf("计算 token.json hash 失败: %v", err)
			}
			if hashBefore != hashAfter {
				t.Errorf("token.json 文件 hash 发生了改变！before=%s, after=%s", hashBefore, hashAfter)
			}
		})
	}
}

// TestRunAPI_BizErrorReturnsNonZero 验证飞书业务错误码（HTTP 2xx + code != 0）返回非零 error
func TestRunAPI_BizErrorReturnsNonZero(t *testing.T) {
	isolateAPITestEnv(t)
	defer resetAPIFlags()

	tests := []struct {
		name       string
		respStatus int
		respBody   string
		wantErr    bool
		wantErrSub string
	}{
		{
			name:       "HTTP 200 + 业务错误 99991679 返回非零",
			respStatus: http.StatusOK,
			respBody:   `{"code":99991679,"msg":"Unauthorized."}`,
			wantErr:    true,
			wantErrSub: "code=99991679",
		},
		{
			name:       "HTTP 200 + 业务错误 232033 返回非零",
			respStatus: http.StatusOK,
			respBody:   `{"code":232033,"msg":"forbidden"}`,
			wantErr:    true,
			wantErrSub: "code=232033",
		},
		{
			name:       "HTTP 200 + 业务成功 code 0 返回 nil",
			respStatus: http.StatusOK,
			respBody:   `{"code":0,"msg":"success","data":{"id":"123"}}`,
			wantErr:    false,
		},
		{
			name:       "HTTP 500 + code 非零返回非零",
			respStatus: http.StatusInternalServerError,
			respBody:   `{"code":99999999,"msg":"server error"}`,
			wantErr:    true,
			wantErrSub: "code=99999999",
		},
		{
			name:       "HTTP 404 + JSON 错误码返回非零",
			respStatus: http.StatusNotFound,
			respBody:   `{"code":1254404,"msg":"resource not found"}`,
			wantErr:    true,
			wantErrSub: "code=1254404",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-fake","expire":7200}`)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.respStatus)
				_, _ = fmt.Fprint(w, tc.respBody)
			})
			defer cleanup()

			cmd := newTestAPICmd()
			apiAs = "bot"
			err := cmd.RunE(cmd, []string{"GET", "/open-apis/im/v1/messages"})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("期望返回错误，实际返回 nil")
				}
				if !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Errorf("错误信息 = %q，期望包含 %q", err.Error(), tc.wantErrSub)
				}
			} else {
				if err != nil {
					t.Fatalf("非预期错误: %v", err)
				}
			}
		})
	}
}

// TestRunAPI_AutoFailClosedOnUserRefreshError 验证在 auto 身份下，已存在 User 身份但刷新失败时 fail closed，绝不发送 Bot 请求
func TestRunAPI_AutoFailClosedOnUserRefreshError(t *testing.T) {
	isolateAPITestEnv(t)
	defer resetAPIFlags()

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")

	// 写入一个已过期的 token.json（带有 refresh_token）
	tokenDir := filepath.Join(tmpHome, ".feishu-cli")
	_ = os.MkdirAll(tokenDir, 0700)
	tokenFile := filepath.Join(tokenDir, "token.json")
	initialStore := auth.TokenStore{
		AccessToken:      "expired-access-token",
		RefreshToken:     "broken-refresh-token",
		TokenType:        "Bearer",
		ExpiresAt:        time.Now().Add(-1 * time.Hour), // 已过期
		RefreshExpiresAt: time.Now().Add(24 * time.Hour),
		Scope:            "im:message",
	}
	data, _ := json.MarshalIndent(initialStore, "", "  ")
	if err := os.WriteFile(tokenFile, data, 0600); err != nil {
		t.Fatalf("写入 token.json 失败: %v", err)
	}

	var bizRequests int32
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		// token 刷新端点返回失败
		if r.URL.Path == "/open-apis/authen/v2/oauth/token" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"error":"invalid_grant","error_description":"refresh token is invalid"}`)
			return
		}

		// 业务端点
		if strings.HasPrefix(r.URL.Path, "/open-apis/im/v1/messages") {
			atomic.AddInt32(&bizRequests, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"success"}`)
			return
		}

		// tenant token 端点（如果被调用说明尝试了切 Bot）
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			t.Errorf("User refresh 失败时不应请求 tenant access token 尝试切 Bot")
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-fake","expire":7200}`)
			return
		}
	})
	defer cleanup()

	cmd := newTestAPICmd()
	apiAs = "auto"
	err := cmd.RunE(cmd, []string{"GET", "/open-apis/im/v1/messages"})
	if err == nil {
		t.Fatalf("User token 刷新失败时，--as auto 应当 fail closed 返回错误，实际返回 nil")
	}

	if hits := atomic.LoadInt32(&bizRequests); hits != 0 {
		t.Fatalf("User token 刷新失败时绝不能以 Bot 身份向业务端点发请求，实际收到 %d 次请求", hits)
	}
}

// TestRunAPI_AutoFallbacksToBotWhenNoUserToken 验证当完全未配置 User 身份时，auto 模式正常以 Bot 身份调用
func TestRunAPI_AutoFallbacksToBotWhenNoUserToken(t *testing.T) {
	isolateAPITestEnv(t)
	defer resetAPIFlags()

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")

	var bizCalled bool
	var capturedAuth string

	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-bot-token","expire":7200}`)
			return
		}
		if r.URL.Path == "/open-apis/im/v1/messages" {
			bizCalled = true
			capturedAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"items":[]}}`)
			return
		}
		http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
	})
	defer cleanup()

	cmd := newTestAPICmd()
	apiAs = "auto"
	err := cmd.RunE(cmd, []string{"GET", "/open-apis/im/v1/messages"})
	if err != nil {
		t.Fatalf("未配置 User token 时，--as auto 应该正常回退到 Bot 身份，实际报错: %v", err)
	}
	if !bizCalled {
		t.Errorf("期望调用业务端点")
	}
	if !strings.HasPrefix(capturedAuth, "Bearer t-") {
		t.Errorf("Authorization = %q, want Bearer t-...", capturedAuth)
	}
}

// TestParseQueryParams_RejectsTrailingJSON 验证 --params 拒绝尾部残留内容。
// 回归防护：json.Decoder 只消费第一个 JSON 值，`{"a":1} {"b":2}` 曾被静默
// 只取前半，用户以为两个参数都生效了。
func TestParseQueryParams_RejectsTrailingJSON(t *testing.T) {
	bad := []string{
		`{"page_size":2} {"bogus":1}`,
		`{"a":1}{"b":2}`,
		`{"a":1} garbage`,
		`{"a":1} 42`,
	}
	for _, raw := range bad {
		if _, err := parseQueryParams(raw); err == nil {
			t.Errorf("%q: 尾部有多余内容应报错", raw)
		}
	}

	good := []string{
		``,
		`   `,
		`{"page_size":2}`,
		`  {"page_size":2}  `,
		`{"a":1,"b":[2,3]}`,
	}
	for _, raw := range good {
		if _, err := parseQueryParams(raw); err != nil {
			t.Errorf("%q: 合法输入不应报错，得到 %v", raw, err)
		}
	}

	// 确认合法输入的解析结果未被破坏
	q, err := parseQueryParams(`{"page_size":2,"flag":true}`)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if got := q.Get("page_size"); got != "2" {
		t.Errorf("page_size = %q, want 2", got)
	}
	if got := q.Get("flag"); got != "true" {
		t.Errorf("flag = %q, want true", got)
	}
}

// TestRunAPI_DataKeepsLargeIntegerPrecision 验证 --data 中的大整数按原始字面量发出，
// 不经 float64 往返舍入（19 位 ID 是飞书常见字段）。
func TestRunAPI_DataKeepsLargeIntegerPrecision(t *testing.T) {
	isolateAPITestEnv(t)
	defer resetAPIFlags()

	var gotBody string
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-fake","expire":7200}`)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{}}`)
	})
	defer cleanup()

	cmd := newTestAPICmd()
	apiAs = "bot"
	apiData = `{"id":1234567890123456789,"ratio":0.1,"n":1000000}`
	if err := cmd.RunE(cmd, []string{"POST", "/open-apis/im/v1/messages"}); err != nil {
		t.Fatalf("非预期错误: %v", err)
	}
	for _, want := range []string{"1234567890123456789", "0.1", "1000000"} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("请求体 %q 缺少原始数字字面量 %q", gotBody, want)
		}
	}
	if strings.Contains(gotBody, "e+") {
		t.Errorf("请求体不应出现科学计数法: %q", gotBody)
	}
}
