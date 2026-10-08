package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/runctx"
)

const (
	tatHTTPTimeout      = 10 * time.Second
	minTATExpireSeconds = 1
	maxTATExpireSeconds = 24 * 60 * 60
)

// TenantAccessToken 是 Accounts OAuth v3 client_credentials 的已校验结果。
type TenantAccessToken struct {
	AccessToken string
	ExpiresIn   int
}

type tatResponse struct {
	Code             int    `json:"code"`
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int    `json:"expires_in"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	Msg              string `json:"msg"`
}

// TATEndpointFunc 解析 Accounts OAuth v3 token 端点，测试可替换为 httptest URL。
var TATEndpointFunc = DefaultTATEndpoint

// DefaultTATEndpoint 返回官方 Accounts `{origin}/oauth/v3/token`。
// FEISHU_TAT_ENDPOINT 仅在 loopback 时生效，供本地 mock；远端覆盖会被忽略以免外送凭证。
func DefaultTATEndpoint(baseURL string) string {
	if ep := strings.TrimSpace(os.Getenv("FEISHU_TAT_ENDPOINT")); ep != "" {
		if u, err := url.Parse(ep); err == nil &&
			(strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https")) &&
			config.IsLoopbackHost(u.Hostname()) {
			return ep
		}
	}
	return strings.TrimRight(config.ResolveAccountsBase(baseURL), "/") + config.OAuthTokenV3Path
}

// FetchTenantAccessToken 用 client_credentials 向官方 Accounts OAuth v3 换取 tenant access token。
func FetchTenantAccessToken(appID, appSecret, baseURL string) (string, error) {
	tok, err := FetchTenantAccessTokenResult(runctx.Root(), appID, appSecret, baseURL)
	if err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}

// FetchTenantAccessTokenContext 与 FetchTenantAccessToken 相同，但使用调用方 context。
func FetchTenantAccessTokenContext(ctx context.Context, appID, appSecret, baseURL string) (string, error) {
	tok, err := FetchTenantAccessTokenResult(ctx, appID, appSecret, baseURL)
	if err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}

// FetchTenantAccessTokenResult 返回已校验的 token 与 expires_in（秒）。
func FetchTenantAccessTokenResult(ctx context.Context, appID, appSecret, baseURL string) (*TenantAccessToken, error) {
	if ctx == nil {
		ctx = runctx.Root()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, tatHTTPTimeout)
		defer cancel()
	}
	return fetchTenantAccessToken(ctx, config.NewHTTPClient(tatHTTPTimeout), appID, appSecret, baseURL)
}

func validateTATExpiresIn(expiresIn int) error {
	if expiresIn < minTATExpireSeconds {
		return fmt.Errorf("tenant token 缺少有效的 expires_in")
	}
	if expiresIn > maxTATExpireSeconds {
		return fmt.Errorf("tenant token expires_in=%d 超出合理范围（1s–24h）", expiresIn)
	}
	return nil
}

func fetchTenantAccessToken(ctx context.Context, httpClient *http.Client, appID, appSecret, baseURL string) (*TenantAccessToken, error) {
	if appID == "" || appSecret == "" {
		return nil, fmt.Errorf("缺少 app_id 或 app_secret 配置")
	}
	if httpClient == nil {
		httpClient = config.NewHTTPClient(tatHTTPTimeout)
	}
	endpoint := TATEndpointFunc(baseURL)
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("tenant token 端点无效: %w", err)
	}
	if err := config.CheckRequestURL(u); err != nil {
		return nil, err
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", appID)
	form.Set("client_secret", appSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("构造 tenant token 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 tenant token 失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := readLimitedAuthBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 tenant token 响应失败: %w", err)
	}

	var result tatResponse
	if err := json.Unmarshal(body, &result); err != nil {
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("获取 tenant_access_token 失败: HTTP %d %s", resp.StatusCode, authBodyPreview(body))
		}
		return nil, fmt.Errorf("解析 tenant token 响应失败（HTTP %d）: %w", resp.StatusCode, err)
	}

	if resp.StatusCode < 400 && result.Code == 0 && result.AccessToken != "" && result.Error == "" {
		if err := validateTATExpiresIn(result.ExpiresIn); err != nil {
			return nil, err
		}
		return &TenantAccessToken{AccessToken: result.AccessToken, ExpiresIn: result.ExpiresIn}, nil
	}

	if result.Error == "server_error" || result.Error == "temporarily_unavailable" || result.Error == "slow_down" || resp.StatusCode >= 500 {
		desc := result.ErrorDescription
		if desc == "" {
			desc = result.Msg
		}
		return nil, fmt.Errorf("tenant token 端点暂时失败（HTTP %d, code=%d, error=%s）: %s",
			resp.StatusCode, result.Code, result.Error, redactAuthPreview(desc))
	}

	return nil, classifyTATFailure(resp.StatusCode, body)
}

// TATError 是 tenant_access_token 换取被服务端拒绝的结果；Error() 文本与历史版本一致。
type TATError struct {
	msg        string
	HTTPStatus int
	Code       int
	OAuthError string
}

func (e *TATError) Error() string { return e.msg }

// credentialRejectCodes 是 Accounts / Open API 对 app_id、app_secret 的确定性拒绝码：
// 20002 client secret 无效、20048 应用不存在、99991543 app_id/app_secret 错误、10014 旧端点 secret 无效。
var credentialRejectCodes = map[int]bool{20002: true, 20048: true, 99991543: true, 10014: true}

// CredentialRejected 报告服务端是否确定性拒绝了 app_id / app_secret（而不是网络或临时错误）。
func (e *TATError) CredentialRejected() bool {
	if e == nil {
		return false
	}
	return e.OAuthError == "invalid_client" || e.OAuthError == "unauthorized_client" || credentialRejectCodes[e.Code]
}

// IsCredentialRejected 报告 err 链上是否为 App 凭证被确定性拒绝。
func IsCredentialRejected(err error) bool {
	var te *TATError
	return errors.As(err, &te) && te.CredentialRejected()
}

func classifyTATFailure(status int, body []byte) error {
	err := classifyTATFailureMsg(status, body)
	te := &TATError{msg: err.Error(), HTTPStatus: status}
	var result tatResponse
	if json.Unmarshal(body, &result) == nil {
		te.Code = result.Code
		te.OAuthError = result.Error
	}
	if te.CredentialRejected() {
		// App 凭证配置错误归鉴权类（退出码 3），文本不变
		return clierr.Auth(te)
	}
	return te
}

func classifyTATFailureMsg(status int, body []byte) error {
	var result tatResponse
	if json.Unmarshal(body, &result) == nil {
		desc := result.ErrorDescription
		if desc == "" {
			desc = result.Msg
		}
		if desc == "" {
			desc = result.Error
		}
		if result.Error != "" || result.Code != 0 {
			if desc == "" {
				desc = "未知错误"
			}
			desc = redactAuthPreview(desc)
			if result.Error != "" {
				return fmt.Errorf("获取 tenant_access_token 失败: HTTP %d error=%s code=%d %s",
					status, result.Error, result.Code, desc)
			}
			return fmt.Errorf("获取 tenant_access_token 失败: HTTP %d code=%d %s", status, result.Code, desc)
		}
		if result.AccessToken == "" {
			return fmt.Errorf("tenant token 响应缺少 access_token（HTTP %d）", status)
		}
	}
	return fmt.Errorf("获取 tenant_access_token 失败: HTTP %d %s", status, authBodyPreview(body))
}
