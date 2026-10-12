package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/config"
)

const (
	legacyTenantTokenInternalPath = config.TenantTokenInternalPath
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
	if req != nil && isGatewayTenantTokenRequest(req) {
		return fulfillGatewayTenantToken(req)
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

// isGatewayTenantTokenRequest 判断网关模式下发往 base_url 的 tenant token 请求（SDK 自取或 drive 下载）。
func isGatewayTenantTokenRequest(req *http.Request) bool {
	if req == nil || req.URL == nil || req.Method != http.MethodPost || !config.GatewayMode() {
		return false
	}
	return strings.TrimSuffix(req.URL.Path, "/") == legacyTenantTokenInternalPath
}

// fulfillGatewayTenantToken 把网关模式下的 tenant token 请求统一交给 auth.FetchTenantAccessTokenResult，
// 保证所有路径发给网关的请求体一致，SDK 的本地占位 secret 不会被发出。
func fulfillGatewayTenantToken(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(req.Body, 1<<20))
		_ = req.Body.Close()
	}
	cfg := config.Get()
	tok, err := auth.FetchTenantAccessTokenResult(req.Context(), cfg.AppID, cfg.AppSecret, cfg.BaseURL)
	if err != nil {
		return sdkJSONResponse(http.StatusOK, map[string]any{
			"code": 99991663,
			"msg":  "换取 tenant_access_token 失败: " + err.Error(),
		})
	}
	return sdkJSONResponse(http.StatusOK, map[string]any{
		"code":                0,
		"expire":              tok.ExpiresIn,
		"tenant_access_token": tok.AccessToken,
	})
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
