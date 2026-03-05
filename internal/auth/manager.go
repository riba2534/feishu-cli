package auth

import (
	"fmt"
	"sync"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// TokenMode 令牌模式
type TokenMode string

const (
	TokenModeAuto   TokenMode = "auto"   // 自动选择：优先 user_access_token，回退 tenant_access_token
	TokenModeUser   TokenMode = "user"   // 强制使用 user_access_token
	TokenModeTenant TokenMode = "tenant" // 强制使用 tenant_access_token
)

// ParseTokenMode 解析令牌模式字符串
func ParseTokenMode(s string) (TokenMode, error) {
	switch s {
	case "", "auto":
		return TokenModeAuto, nil
	case "user":
		return TokenModeUser, nil
	case "tenant":
		return TokenModeTenant, nil
	default:
		return "", fmt.Errorf("无效的 token-mode: %q（可选值: auto, user, tenant）", s)
	}
}

// Manager 管理用户令牌的获取和刷新
type Manager struct {
	mu        sync.Mutex
	appID     string
	appSecret string
	baseURL   string
}

var (
	globalManager *Manager
	managerMu     sync.Mutex
)

// InitManager 初始化全局令牌管理器
func InitManager(appID, appSecret, baseURL string) {
	managerMu.Lock()
	defer managerMu.Unlock()
	globalManager = &Manager{
		appID:     appID,
		appSecret: appSecret,
		baseURL:   baseURL,
	}
}

// GetUserAccessToken 获取有效的 user_access_token，过期时自动刷新
// 返回空字符串表示未登录或令牌不可用
func GetUserAccessToken() (string, error) {
	token, err := LoadToken()
	if err != nil {
		return "", err
	}
	if token == nil {
		return "", nil
	}

	// token 有效，直接返回
	if token.IsValid() {
		return token.AccessToken, nil
	}

	// token 过期但可刷新
	if token.IsRefreshable() {
		managerMu.Lock()
		mgr := globalManager
		managerMu.Unlock()

		if mgr == nil {
			return "", fmt.Errorf("令牌管理器未初始化")
		}

		return mgr.refreshToken(token)
	}

	// refresh_token 也过期了
	return "", fmt.Errorf("用户令牌已过期，请执行 feishu-cli login 重新登录")
}

// refreshToken 刷新令牌并保存
func (m *Manager) refreshToken(token *UserToken) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 双重检查：可能在等锁期间已被其他 goroutine 刷新
	freshToken, err := LoadToken()
	if err == nil && freshToken != nil && freshToken.IsValid() {
		return freshToken.AccessToken, nil
	}

	newToken, err := RefreshAccessToken(OAuthConfig{
		AppID:     m.appID,
		AppSecret: m.appSecret,
		BaseURL:   m.baseURL,
	}, token.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("自动刷新令牌失败: %w\n请执行 feishu-cli login 重新登录", err)
	}

	if err := SaveToken(newToken); err != nil {
		return "", fmt.Errorf("保存刷新后的令牌失败: %w", err)
	}

	return newToken.AccessToken, nil
}

// UserTokenRequestOption 根据令牌模式返回请求选项
// 当使用 user 模式时，返回 WithUserAccessToken 选项
// 当使用 tenant 模式时，返回 nil（使用默认的 tenant_access_token）
// 当使用 auto 模式时，有 user_access_token 则使用，否则返回 nil
func UserTokenRequestOption(mode TokenMode) (larkcore.RequestOptionFunc, error) {
	switch mode {
	case TokenModeTenant:
		return nil, nil

	case TokenModeUser:
		token, err := GetUserAccessToken()
		if err != nil {
			return nil, err
		}
		if token == "" {
			return nil, fmt.Errorf("未登录，请先执行 feishu-cli login 进行用户授权")
		}
		return larkcore.WithUserAccessToken(token), nil

	case TokenModeAuto:
		token, err := GetUserAccessToken()
		if err != nil {
			// auto 模式下获取失败不报错，回退到 tenant
			return nil, nil
		}
		if token == "" {
			return nil, nil
		}
		return larkcore.WithUserAccessToken(token), nil

	default:
		return nil, fmt.Errorf("未知的 token-mode: %q", mode)
	}
}
