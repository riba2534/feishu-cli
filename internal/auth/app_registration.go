package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/riba2534/feishu-cli/v2/internal/textutil"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/riba2534/feishu-cli/v2/internal/runctx"
)

const (
	appRegPath = "/oauth/v1/app/registration"

	// 注册协议默认值（对齐官方 lark-cli internal/auth/app_registration.go）
	registrationBootstrapBrand = config.BrandFeishu // 注册始终在飞书端发起
	defaultRegPollInterval     = 5                  // interval 缺失时的轮询间隔（秒）
	defaultRegExpireIn         = 600                // expire_in 缺失时的有效期（秒）
	maxRegPollInterval         = 60
	maxRegPollAttempts         = 200
	regRequestTimeout          = 15 * time.Second
)

// 注册流程的终态错误，供调用方分类。
var (
	ErrRegistrationDenied   = errors.New("用户拒绝了应用注册")
	ErrRegistrationExpired  = errors.New("注册码已过期，请重新执行 feishu-cli config create-app")
	ErrRegistrationTimedOut = errors.New("应用注册超时，请重新执行 feishu-cli config create-app")
)

// appRegistrationEndpointFunc 返回指定品牌的注册端点，测试可替换为 httptest URL。
var appRegistrationEndpointFunc = func(brand config.Brand) string {
	return config.OfficialAccountsBase(brand) + appRegPath
}

// regPollTick 是轮询等待的时间粒度（每个 tick 触发一次 onTick），测试可调小。
var regPollTick = time.Second

// AppRegistrationResponse 应用注册设备流响应
type AppRegistrationResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// AppRegistrationResult 应用注册成功结果
type AppRegistrationResult struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	OpenID       string `json:"open_id,omitempty"`
	TenantBrand  string `json:"tenant_brand,omitempty"` // "feishu" or "lark"
}

// RequestAppRegistration 发起应用自注册 Device Flow（action=begin）。
//
// 协议要点（对齐官方）：
//   - 注册始终在飞书 accounts 端发起；brand 只决定展示给用户的确认页域名
//   - 有效期字段是 expire_in（兼容旧拼写 expires_in），缺失时默认 600s；interval 缺失默认 5s
func RequestAppRegistration(brand config.Brand) (*AppRegistrationResponse, error) {
	ctx, cancel := context.WithTimeout(runctx.Root(), regRequestTimeout)
	defer cancel()

	form := url.Values{}
	form.Set("action", "begin")
	form.Set("archetype", "PersonalAgent")
	form.Set("auth_method", "client_secret")
	form.Set("request_user_info", "open_id tenant_brand")

	endpoint := appRegistrationEndpointFunc(registrationBootstrapBrand)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := config.NewHTTPClient(regRequestTimeout).Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("HTTP %d，响应非 JSON: %s", resp.StatusCode, truncateStr(redactAuthPreview(string(body)), 200))
	}

	if _, hasErr := data["error"]; hasErr || resp.StatusCode >= 400 {
		desc := getStrField(data, "error_description")
		if desc == "" {
			desc = getStrField(data, "error")
		}
		if desc == "" {
			desc = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("应用注册失败: %s", desc)
	}

	deviceCode := getStrField(data, "device_code")
	if deviceCode == "" {
		return nil, fmt.Errorf("应用注册失败: 响应缺少 device_code")
	}

	// 协议字段是 expire_in；兼容旧拼写 expires_in，二者都缺失时按协议默认值
	expiresIn := getIntField(data, "expire_in", 0)
	if expiresIn <= 0 {
		expiresIn = getIntField(data, "expires_in", 0)
	}
	if expiresIn <= 0 {
		expiresIn = defaultRegExpireIn
	}
	interval := getIntField(data, "interval", 0)
	if interval <= 0 {
		interval = defaultRegPollInterval
	}

	userCode := getStrField(data, "user_code")
	verificationURIComplete := fmt.Sprintf("%s/page/cli?user_code=%s", config.OfficialOpenBase(brand), url.QueryEscape(userCode))

	return &AppRegistrationResponse{
		DeviceCode:              deviceCode,
		UserCode:                userCode,
		VerificationURI:         getStrField(data, "verification_uri"),
		VerificationURIComplete: verificationURIComplete,
		ExpiresIn:               expiresIn,
		Interval:                interval,
	}, nil
}

// PollAppRegistration 轮询应用注册结果，返回凭证与签发凭证的品牌。
//
// 对齐官方 RegisterAppWithDiscovery：
//   - 从飞书端开始轮询；响应 user_info.tenant_brand 与当前轮询域不同（Lark 租户）时，
//     立即切换到该品牌的 accounts 域继续轮询（只切换一次），可与 authorization_pending 同时到达
//   - 无错误但凭证不完整（缺 client_secret）时继续轮询
//   - 最终凭证所在品牌即为生效品牌；最终响应声明的 tenant_brand 与之矛盾视为协议错误
func PollAppRegistration(ctx context.Context, deviceCode string, interval, expiresIn int, onTick func(elapsed, total int)) (*AppRegistrationResult, config.Brand, error) {
	if ctx == nil {
		ctx = runctx.Root()
	}
	if interval <= 0 {
		interval = defaultRegPollInterval
	}
	if expiresIn <= 0 {
		expiresIn = defaultRegExpireIn
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(expiresIn)*regPollTick)
	defer cancel()

	httpClient := config.NewHTTPClient(regRequestTimeout)
	start := time.Now()
	currentBrand := registrationBootstrapBrand
	switched := false
	waitBeforePoll := false

	for attempts := 0; attempts < maxRegPollAttempts; attempts++ {
		if waitBeforePoll {
			if err := waitRegPoll(ctx, interval, expiresIn, start, onTick); err != nil {
				return nil, currentBrand, err
			}
		}
		waitBeforePoll = true
		if err := regContextError(ctx); err != nil {
			return nil, currentBrand, err
		}

		data, err := pollAppRegistrationOnce(ctx, httpClient, currentBrand, deviceCode)
		if err != nil {
			if cerr := regContextError(ctx); cerr != nil {
				return nil, currentBrand, cerr
			}
			interval = min(interval+1, maxRegPollInterval)
			continue
		}

		// Lark 租户：切换轮询域（只切换一次，立即重新轮询）
		if !switched {
			if userInfo, ok := data["user_info"].(map[string]interface{}); ok {
				if tb := getStrField(userInfo, "tenant_brand"); tb != "" {
					if actual := parseRegBrand(tb); actual != currentBrand {
						currentBrand = actual
						switched = true
						waitBeforePoll = false
						continue
					}
				}
			}
		}

		errStr := getStrField(data, "error")
		if errStr == "" {
			result := &AppRegistrationResult{
				ClientID:     getStrField(data, "client_id"),
				ClientSecret: getStrField(data, "client_secret"),
			}
			if userInfo, ok := data["user_info"].(map[string]interface{}); ok {
				result.OpenID = getStrField(userInfo, "open_id")
				result.TenantBrand = getStrField(userInfo, "tenant_brand")
			}
			if result.ClientID != "" && result.ClientSecret != "" {
				if result.TenantBrand != "" && parseRegBrand(result.TenantBrand) != currentBrand {
					return nil, currentBrand, fmt.Errorf("应用注册返回的凭证与租户品牌 %q 矛盾，请重试", result.TenantBrand)
				}
				return result, currentBrand, nil
			}
			// 凭证不完整且无错误：继续轮询
			continue
		}

		switch errStr {
		case "authorization_pending":
			continue
		case "slow_down":
			interval = min(interval+5, maxRegPollInterval)
			continue
		case "access_denied":
			return nil, currentBrand, ErrRegistrationDenied
		case "expired_token", "invalid_grant":
			return nil, currentBrand, ErrRegistrationExpired
		}

		desc := getStrField(data, "error_description")
		if desc == "" {
			desc = errStr
		}
		return nil, currentBrand, fmt.Errorf("应用注册失败: %s", desc)
	}
	return nil, currentBrand, ErrRegistrationTimedOut
}

func pollAppRegistrationOnce(ctx context.Context, httpClient *http.Client, brand config.Brand, deviceCode string) (map[string]interface{}, error) {
	form := url.Values{}
	form.Set("action", "poll")
	form.Set("device_code", deviceCode)

	req, err := http.NewRequestWithContext(ctx, "POST", appRegistrationEndpointFunc(brand), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	return data, nil
}

// waitRegPoll 按 interval 个 tick 等待，每个 tick 回调一次进度；ctx 结束时返回终态错误。
func waitRegPoll(ctx context.Context, interval, total int, start time.Time, onTick func(elapsed, total int)) error {
	for i := 0; i < interval; i++ {
		if onTick != nil {
			onTick(int(time.Since(start)/regPollTick), total)
		}
		select {
		case <-time.After(regPollTick):
		case <-ctx.Done():
			return regContextError(ctx)
		}
	}
	return nil
}

func regContextError(ctx context.Context) error {
	switch {
	case ctx.Err() == nil:
		return nil
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return ErrRegistrationTimedOut
	default:
		return fmt.Errorf("应用注册已取消: %w", ctx.Err())
	}
}

// parseRegBrand 解析注册响应中的 tenant_brand；无法识别时按飞书处理。
func parseRegBrand(s string) config.Brand {
	if strings.EqualFold(strings.TrimSpace(s), string(config.BrandLark)) {
		return config.BrandLark
	}
	return config.BrandFeishu
}

func getStrField(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getIntField(m map[string]interface{}, key string, defaultVal int) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return defaultVal
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return textutil.TruncateUTF8(s, maxLen) + "..."
}
