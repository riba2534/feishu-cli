package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// UserToken 存储 OAuth 用户令牌信息
type UserToken struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresAt        int64  `json:"expires_at"`
	RefreshExpiresAt int64  `json:"refresh_expires_at"`
	TokenType        string `json:"token_type"`
	Scope            string `json:"scope"`
}

// IsValid 检查 access_token 是否有效（未过期且剩余 > 5 分钟）
func (t *UserToken) IsValid() bool {
	return t.AccessToken != "" && time.Now().Unix() < t.ExpiresAt-300
}

// IsRefreshable 检查 refresh_token 是否可用（未过期）
func (t *UserToken) IsRefreshable() bool {
	return t.RefreshToken != "" && time.Now().Unix() < t.RefreshExpiresAt
}

// tokenFilePath 返回 token 文件路径
func tokenFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户目录失败: %w", err)
	}
	return filepath.Join(home, ".feishu-cli", "user_token.json"), nil
}

// LoadToken 从文件加载用户令牌
func LoadToken() (*UserToken, error) {
	path, err := tokenFilePath()
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

	var token UserToken
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("解析 token 文件失败: %w", err)
	}

	return &token, nil
}

// SaveToken 保存用户令牌到文件
func SaveToken(token *UserToken) error {
	path, err := tokenFilePath()
	if err != nil {
		return err
	}

	// 确保目录存在，使用 0700 权限保护敏感信息
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 token 失败: %w", err)
	}

	// 使用 0600 权限，仅所有者可读写
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("写入 token 文件失败: %w", err)
	}

	return nil
}

// DeleteToken 删除本地缓存的用户令牌
func DeleteToken() error {
	path, err := tokenFilePath()
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("删除 token 文件失败: %w", err)
	}

	return nil
}
