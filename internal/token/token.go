package token

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// UserToken 存储用户访问令牌信息
type UserToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	ExpiresIn    int64  `json:"expires_in"`
}

// TokenResponse 飞书 OAuth API 响应
type TokenResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	} `json:"data"`
}

// RefreshResponse 飞书刷新 Token API 响应
type RefreshResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	} `json:"data"`
}

// tokenFilePath 返回 token 文件路径
func tokenFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户目录失败: %w", err)
	}
	return filepath.Join(home, ".lark_user_token"), nil
}

// SaveToken 保存 token 到文件
func SaveToken(token *UserToken) error {
	path, err := tokenFilePath()
	if err != nil {
		return err
	}

	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
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

// LoadToken 从文件加载 token
func LoadToken() (*UserToken, error) {
	path, err := tokenFilePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("token 文件不存在，请先运行登录命令")
		}
		return nil, fmt.Errorf("读取 token 文件失败: %w", err)
	}

	var token UserToken
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("解析 token 文件失败: %w", err)
	}

	return &token, nil
}

// IsExpired 检查 token 是否过期（提前 5 分钟视为过期）
func (t *UserToken) IsExpired() bool {
	if t.ExpiresAt == 0 {
		return true
	}
	// 提前 5 分钟视为过期，避免边界问题
	return time.Now().Unix() >= (t.ExpiresAt - 300)
}

// GetRemainingTime 获取 token 剩余有效时间
func (t *UserToken) GetRemainingTime() time.Duration {
	if t.IsExpired() {
		return 0
	}
	return time.Until(time.Unix(t.ExpiresAt, 0))
}

// FormatExpiry 格式化过期时间显示
func (t *UserToken) FormatExpiry() string {
	if t.ExpiresAt == 0 {
		return "未知"
	}
	expiryTime := time.Unix(t.ExpiresAt, 0)
	remaining := t.GetRemainingTime()

	if t.IsExpired() {
		return fmt.Sprintf("已过期 (%s)", expiryTime.Format("2006-01-02 15:04:05"))
	}

	// 格式化剩余时间
	hours := int(remaining.Hours())
	minutes := int(remaining.Minutes()) % 60
	seconds := int(remaining.Seconds()) % 60

	var timeStr string
	if hours > 0 {
		timeStr = fmt.Sprintf("%d小时%d分钟", hours, minutes)
	} else if minutes > 0 {
		timeStr = fmt.Sprintf("%d分钟%d秒", minutes, seconds)
	} else {
		timeStr = fmt.Sprintf("%d秒", seconds)
	}

	return fmt.Sprintf("%s (过期时间: %s)", timeStr, expiryTime.Format("2006-01-02 15:04:05"))
}

// DeleteToken 删除 token 文件
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

// TokenExists 检查 token 文件是否存在
func TokenExists() bool {
	path, err := tokenFilePath()
	if err != nil {
		return false
	}

	_, err = os.Stat(path)
	return err == nil
}
