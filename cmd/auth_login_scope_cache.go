package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/riba2534/feishu-cli/internal/auth"
)

var loginScopeCacheSafeChars = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

type loginScopeCacheRecord struct {
	RequestedScope string `json:"requested_scope"`
}

func loginScopeCacheDir() (string, error) {
	tokenPath, err := auth.TokenPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(tokenPath), "cache", "auth_login_scopes"), nil
}

func loginScopeCachePath(deviceCode string) (string, error) {
	dir, err := loginScopeCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, sanitizeLoginScopeCacheKey(deviceCode)+".json"), nil
}

func sanitizeLoginScopeCacheKey(deviceCode string) string {
	deviceCode = loginScopeCacheSafeChars.ReplaceAllString(deviceCode, "_")
	if deviceCode == "" {
		return "default"
	}
	return deviceCode
}

func saveLoginRequestedScope(deviceCode, requestedScope string) error {
	if deviceCode == "" {
		return fmt.Errorf("device_code 不能为空")
	}
	path, err := loginScopeCachePath(deviceCode)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("创建授权缓存目录失败: %w", err)
	}
	data, err := json.MarshalIndent(loginScopeCacheRecord{RequestedScope: requestedScope}, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化授权缓存失败: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("写入授权缓存失败: %w", err)
	}
	return nil
}

func loadLoginRequestedScope(deviceCode string) (string, error) {
	path, err := loginScopeCachePath(deviceCode)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("读取授权缓存失败: %w", err)
	}
	var record loginScopeCacheRecord
	if err := json.Unmarshal(data, &record); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("解析授权缓存失败: %w", err)
	}
	return record.RequestedScope, nil
}

func removeLoginRequestedScope(deviceCode string) error {
	path, err := loginScopeCachePath(deviceCode)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("删除授权缓存失败: %w", err)
	}
	return nil
}

func clearLoginRequestedScopeCache() error {
	dir, err := loginScopeCacheDir()
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("清理授权缓存目录失败: %w", err)
	}
	return nil
}
