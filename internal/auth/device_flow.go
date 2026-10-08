package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/riba2534/feishu-cli/internal/textutil"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/runctx"
)

const (
	deviceAuthPath      = "/oauth/v1/device_authorization"
	feishuDeviceAuthURL = config.OfficialFeishuAccounts + deviceAuthPath
	larkDeviceAuthURL   = config.OfficialLarkAccounts + deviceAuthPath

	maxPollInterval = 60  // slow_down 最大间隔（秒）
	maxPollAttempts = 200 // 安全上限，远超设备码有效期
)

// DeviceAuthResponse 设备授权响应（RFC 8628 步骤一）
type DeviceAuthResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"` // 设备码有效期（秒）
	Interval                int    `json:"interval"`   // 推荐轮询间隔（秒）
}

// resolveDeviceAuthURL 根据 baseURL 推导设备授权端点
//
// baseURL 为 open API 基础地址（如 https://open.feishu.cn），
// 设备授权端点在 accounts.feishu.cn，按 open.X → accounts.X 规则推导。
func resolveDeviceAuthURL(baseURL string) string {
	return strings.TrimRight(config.ResolveAccountsBase(baseURL), "/") + deviceAuthPath
}

// deviceAuthURLFunc 解析设备授权端点，测试可替换为 httptest URL。
var deviceAuthURLFunc = resolveDeviceAuthURL

// RequestDeviceAuthorization 向飞书设备授权端点发起请求（RFC 8628 步骤一）
//
// 使用 HTTP Basic 认证（appID:appSecret）发送 form 表单请求。
// 自动将 offline_access 追加到 scope 以确保返回 refresh_token。
func RequestDeviceAuthorization(appID, appSecret, baseURL, scope string) (*DeviceAuthResponse, error) {
	if appID == "" || appSecret == "" {
		return nil, fmt.Errorf("缺少 app_id 或 app_secret，请先配置:\n" +
			"  环境变量: export FEISHU_APP_ID=xxx && export FEISHU_APP_SECRET=xxx\n" +
			"  配置文件: feishu-cli config init")
	}

	// 线上格式统一为空格分隔（服务端把 "a,b" 当成一个非法 scope）；按 token 判断是否已含 offline_access
	scope = JoinScopes(UniqueScopeList(scope), []string{"offline_access"})

	deviceAuthURL := deviceAuthURLFunc(baseURL)
	basicAuth := base64.StdEncoding.EncodeToString([]byte(appID + ":" + appSecret))

	formBody := url.Values{}
	formBody.Set("client_id", appID)
	formBody.Set("scope", scope)

	req, err := http.NewRequestWithContext(runctx.Root(), "POST", deviceAuthURL, strings.NewReader(formBody.Encode()))
	if err != nil {
		return nil, fmt.Errorf("构造设备授权请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+basicAuth)

	httpClient := config.NewHTTPClient(15 * time.Second)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("设备授权请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("读取设备授权响应失败: %w", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("设备授权失败: HTTP %d – %s", resp.StatusCode, deviceFlowTruncate(string(respBody), 200))
	}

	if errVal, ok := raw["error"].(string); ok && errVal != "" {
		desc, _ := raw["error_description"].(string)
		if desc == "" {
			desc = errVal
		}
		return nil, fmt.Errorf("设备授权失败: %s", desc)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("设备授权失败: HTTP %d – %s", resp.StatusCode, deviceFlowTruncate(string(respBody), 200))
	}

	deviceCode, _ := raw["device_code"].(string)
	userCode, _ := raw["user_code"].(string)
	verificationURI, _ := raw["verification_uri"].(string)
	verificationURIComplete, _ := raw["verification_uri_complete"].(string)
	if verificationURIComplete == "" {
		verificationURIComplete = verificationURI
	}
	expiresIn := int(deviceFlowToFloat64(raw["expires_in"], 240))
	interval := int(deviceFlowToFloat64(raw["interval"], 5))

	if deviceCode == "" || userCode == "" || verificationURI == "" {
		return nil, fmt.Errorf("设备授权响应缺少必要字段，响应: %s", deviceFlowTruncate(string(respBody), 300))
	}

	return &DeviceAuthResponse{
		DeviceCode:              deviceCode,
		UserCode:                userCode,
		VerificationURI:         verificationURI,
		VerificationURIComplete: verificationURIComplete,
		ExpiresIn:               expiresIn,
		Interval:                interval,
	}, nil
}

// PollDeviceToken 轮询 token 端点直至授权完成（RFC 8628 步骤二）
//
// onTick 在每轮等待前被调用（已等待秒数、总有效期秒数），可为 nil。
// 正确处理 authorization_pending / slow_down / access_denied / expired_token。
func PollDeviceToken(appID, appSecret, baseURL, deviceCode string, interval, expiresIn int, onTick func(elapsed, total int)) (*TokenStore, error) {
	baseURL = config.ResolveOpenBase(baseURL)
	if err := config.CheckBaseURL(baseURL); err != nil {
		return nil, err
	}
	tokenURL := baseURL + "/open-apis/authen/v2/oauth/token"

	startTime := time.Now()
	deadline := startTime.Add(time.Duration(expiresIn) * time.Second)
	currentInterval := interval
	if currentInterval <= 0 {
		currentInterval = 5
	}

	httpClient := config.NewHTTPClient(15 * time.Second)
	attempts := 0

	for time.Now().Before(deadline) && attempts < maxPollAttempts {
		attempts++

		// 将等待时间拆成 1 秒粒度，每秒触发一次 onTick，实现逐秒进度更新。
		// HTTP 请求频率保持 currentInterval 不变。
		for i := 0; i < currentInterval; i++ {
			if !time.Now().Before(deadline) {
				break
			}
			if onTick != nil {
				elapsed := int(time.Since(startTime).Seconds())
				onTick(elapsed, expiresIn)
			}
			time.Sleep(time.Second)
		}

		if !time.Now().Before(deadline) {
			break
		}

		formBody := url.Values{}
		formBody.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
		formBody.Set("device_code", deviceCode)
		formBody.Set("client_id", appID)
		formBody.Set("client_secret", appSecret)

		req, err := http.NewRequestWithContext(runctx.Root(), "POST", tokenURL, strings.NewReader(formBody.Encode()))
		if err != nil {
			currentInterval = min(currentInterval+1, maxPollInterval)
			continue
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := httpClient.Do(req)
		if err != nil {
			currentInterval = min(currentInterval+1, maxPollInterval)
			continue
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if err != nil {
			currentInterval = min(currentInterval+1, maxPollInterval)
			continue
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(body, &raw); err != nil {
			currentInterval = min(currentInterval+1, maxPollInterval)
			continue
		}

		errVal, _ := raw["error"].(string)

		if errVal == "" {
			if accessToken, ok := raw["access_token"].(string); ok && accessToken != "" {
				now := time.Now()
				tokenExpiresIn := int(deviceFlowToFloat64(raw["expires_in"], 7200))
				refreshExpiresIn := int(deviceFlowToFloat64(raw["refresh_token_expires_in"], 0))
				refreshToken, _ := raw["refresh_token"].(string)
				scope, _ := raw["scope"].(string)
				tokenType, _ := raw["token_type"].(string)

				store := &TokenStore{
					AccessToken:  accessToken,
					RefreshToken: refreshToken,
					TokenType:    tokenType,
					ExpiresAt:    now.Add(time.Duration(tokenExpiresIn) * time.Second),
					Scope:        scope,
					AppID:        appID,
				}
				if refreshExpiresIn > 0 {
					store.RefreshExpiresAt = now.Add(time.Duration(refreshExpiresIn) * time.Second)
				}
				// 服务端提示（如部分 scope 被静默裁剪），仅透传展示，不落盘
				store.StatusMessage, _ = raw["status_message"].(string)
				return store, nil
			}
		}

		switch errVal {
		case "authorization_pending":
			continue
		case "slow_down":
			currentInterval = min(currentInterval+5, maxPollInterval)
			continue
		case "access_denied":
			return nil, fmt.Errorf("用户拒绝了授权")
		case "expired_token", "invalid_grant":
			return nil, fmt.Errorf("授权码已过期，请重新执行 feishu-cli auth login")
		default:
			desc, _ := raw["error_description"].(string)
			if desc == "" {
				desc = errVal
			}
			if desc == "" {
				desc = "未知错误"
			}
			return nil, fmt.Errorf("轮询 token 失败: %s", desc)
		}
	}

	if attempts >= maxPollAttempts {
		return nil, fmt.Errorf("超过最大轮询次数（%d 次），请重新执行 feishu-cli auth login", maxPollAttempts)
	}
	return nil, fmt.Errorf("授权超时（设备码已过期），请重新执行 feishu-cli auth login")
}

func deviceFlowToFloat64(v interface{}, defaultVal float64) float64 {
	if v == nil {
		return defaultVal
	}
	if n, ok := v.(float64); ok {
		return n
	}
	return defaultVal
}

func deviceFlowTruncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return textutil.TruncateUTF8(s, maxLen) + "..."
}
