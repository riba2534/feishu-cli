package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/riba2534/feishu-cli/internal/profile"
)

// TokenStore 存储 OAuth token 信息
type TokenStore struct {
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	TokenType        string    `json:"token_type"`
	ExpiresAt        time.Time `json:"expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
	Scope            string    `json:"scope"`
}

// tokenPathFunc 可在测试中替换的路径函数
var tokenPathFunc = originalTokenPath

// originalTokenPath 返回当前激活 profile 的 token.json 路径。
// 未启用 profile 系统时回退到旧布局 ~/.feishu-cli/token.json，保持向后兼容。
func originalTokenPath() (string, error) {
	dir, err := profile.ActiveDir()
	if err != nil {
		return "", fmt.Errorf("解析当前 profile 失败: %w", err)
	}
	return filepath.Join(dir, "token.json"), nil
}

// TokenPath 返回 token 文件路径（当前激活 profile 下的 token.json）。
func TokenPath() (string, error) {
	return tokenPathFunc()
}

// LoadToken 从文件加载 token，文件不存在返回 nil, nil
func LoadToken() (*TokenStore, error) {
	path, err := TokenPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取 token 文件失败: %w", err)
	}

	var t TokenStore
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("解析 token 文件失败: %w", err)
	}

	return &t, nil
}

// SaveToken 保存 token 到文件（0600 权限）
func SaveToken(t *TokenStore) error {
	path, err := TokenPath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 token 失败: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("写入 token 文件失败: %w", err)
	}

	clearCurrentUserCacheBestEffort()
	return nil
}

// DeleteToken 删除 token 文件
func DeleteToken() error {
	path, err := TokenPath()
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("删除 token 文件失败: %w", err)
		}
	}

	clearCurrentUserCacheBestEffort()
	return nil
}

// clearCurrentUserCacheBestEffort clears derived user cache without failing the
// primary token persistence flow.
func clearCurrentUserCacheBestEffort() {
	_ = DeleteCurrentUserCache()
}

// tokenRefreshAhead 是 token 过期前主动刷新的时间窗口（5 分钟）。
// 窗口过小会让运行时间较长的命令（如大文档导入、长时间搜索）在执行途中遇到 token 过期。
const tokenRefreshAhead = 5 * time.Minute

// IsAccessTokenValid 检查 access_token 是否有效（预留 5 分钟缓冲）。
func (t *TokenStore) IsAccessTokenValid() bool {
	return t.AccessToken != "" && time.Now().Add(tokenRefreshAhead).Before(t.ExpiresAt)
}

// IsRefreshTokenValid 检查 refresh_token 是否有效。
// 当 RefreshExpiresAt 为零值时（服务端未返回过期时间），假定有效，让服务端决定。
// 有值时预留 5 分钟缓冲，与 access_token 策略一致。
func (t *TokenStore) IsRefreshTokenValid() bool {
	if t.RefreshToken == "" {
		return false
	}
	if t.RefreshExpiresAt.IsZero() {
		return true
	}
	return time.Now().Add(tokenRefreshAhead).Before(t.RefreshExpiresAt)
}

// TokenStatus returns valid / needs_refresh / expired, aligned with auth status semantics.
func (t *TokenStore) TokenStatus() string {
	switch {
	case t == nil:
		return "expired"
	case t.IsAccessTokenValid():
		return "valid"
	case t.IsRefreshTokenValid():
		return "needs_refresh"
	default:
		return "expired"
	}
}

// MaskToken 对 token 脱敏显示（前 6 + 后 6）
func MaskToken(token string) string {
	if len(token) <= 12 {
		return "***"
	}
	return token[:6] + "..." + token[len(token)-6:]
}
