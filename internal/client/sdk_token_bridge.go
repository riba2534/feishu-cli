package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
)

const (
	legacyTenantTokenInternalPath = "/open-apis/auth/v3/tenant_access_token/internal"
)

// sdkTestTransport 仅测试注入：替换 SDK HTTP 客户端的底层 Transport。
var sdkTestTransport http.RoundTripper

type legacyInternalTokenBridge struct {
	base http.RoundTripper
}

func wrapSDKHTTPClient() *http.Client {
	hc := config.NewHTTPClientWithTransport(sdkTestTransport, 0)
	hc.Transport = &legacyInternalTokenBridge{base: hc.Transport}
	return hc
}

func (t *legacyInternalTokenBridge) RoundTrip(req *http.Request) (*http.Response, error) {
	if req != nil && isOfficialLegacyInternalTokenRequest(req) {
		return fulfillLegacyInternalTokenViaAccountsV3(req)
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

func isOfficialLegacyInternalTokenRequest(req *http.Request) bool {
	if req == nil || req.URL == nil || req.Method != http.MethodPost {
		return false
	}
	if !config.IsOfficialOpenHost(req.URL.Hostname()) || !strings.EqualFold(req.URL.Scheme, "https") {
		return false
	}
	path := strings.TrimSuffix(req.URL.Path, "/")
	return path == legacyTenantTokenInternalPath
}

func fulfillLegacyInternalTokenViaAccountsV3(req *http.Request) (*http.Response, error) {
	var creds struct {
		AppID     string `json:"app_id"`
		AppSecret string `json:"app_secret"`
	}
	if req.Body != nil {
		body, err := readLimitedAuthBodyFromSDK(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return sdkJSONResponse(http.StatusOK, map[string]any{"code": 1, "msg": "读取应用凭证失败"})
		}
		_ = json.Unmarshal(body, &creds)
	}
	tok, err := auth.FetchTenantAccessTokenResult(req.Context(), creds.AppID, creds.AppSecret, "https://"+req.URL.Hostname())
	if err != nil {
		if isTokenFetchNetworkError(err) {
			// 网络 / 超时类错误原样作为传输层错误返回：保留 net.Error 链，由上层归为网络错误（退出码 4、可重试），
			// 不再合成业务码 99991663 误导用户去检查 app_secret
			return nil, err
		}
		return sdkJSONResponse(http.StatusOK, map[string]any{
			"code": 99991663,
			"msg":  "换取 tenant_access_token 失败",
		})
	}
	return sdkJSONResponse(http.StatusOK, map[string]any{
		"code":                0,
		"expire":              tok.ExpiresIn,
		"tenant_access_token": tok.AccessToken,
	})
}

// isTokenFetchNetworkError 判断换取 tenant_access_token 的失败是否来自网络 / 超时，而不是服务端的业务拒绝。
func isTokenFetchNetworkError(err error) bool {
	if err == nil {
		return false
	}
	var tatErr *auth.TATError
	if errors.As(err, &tatErr) {
		return false
	}
	var netErr net.Error
	return errors.As(err, &netErr) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) ||
		clierr.HasKind(err, clierr.KindNetwork)
}

// sdkLogger 替换 SDK 默认 logger（默认写 stdout，会污染 -o json 管道）：
// 非 debug 模式静默——SDK 的 Warn 只是重复随后返回给调用方的错误；debug 模式写 stderr。
type sdkLogger struct {
	debug bool
}

func newSDKLogger(debug bool) larkcore.Logger { return sdkLogger{debug: debug} }

func (l sdkLogger) print(level string, args []any) {
	if l.debug {
		fmt.Fprintf(sdkLogWriter, "%s [%s] %v\n", time.Now().Format("2006/01/02 15:04:05"), level, args)
	}
}

// sdkLogWriter 仅为测试可替换；生产固定为 stderr。
var sdkLogWriter io.Writer = os.Stderr

func (l sdkLogger) Debug(_ context.Context, args ...any) { l.print("Debug", args) }
func (l sdkLogger) Info(_ context.Context, args ...any)  { l.print("Info", args) }
func (l sdkLogger) Warn(_ context.Context, args ...any)  { l.print("Warn", args) }
func (l sdkLogger) Error(_ context.Context, args ...any) { l.print("Error", args) }

func readLimitedAuthBodyFromSDK(r io.Reader) ([]byte, error) {
	const max = 1 << 20
	body, err := io.ReadAll(io.LimitReader(r, int64(max)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > max {
		return nil, fmt.Errorf("请求体超过 1MiB")
	}
	return body, nil
}

func sdkJSONResponse(status int, payload map[string]any) (*http.Response, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		Status:        http.StatusText(status),
		StatusCode:    status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
		Body:          io.NopCloser(bytes.NewReader(raw)),
		ContentLength: int64(len(raw)),
	}, nil
}
