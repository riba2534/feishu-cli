package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/riba2534/feishu-cli/internal/profile"
)

// CurrentUserCache stores the current logged-in user's profile metadata.
// It intentionally avoids storing raw tokens and only keeps a token fingerprint
// to determine whether the cache still matches the current OAuth session.
type CurrentUserCache struct {
	OpenID           string    `json:"open_id,omitempty"`
	UserID           string    `json:"user_id,omitempty"`
	UnionID          string    `json:"union_id,omitempty"`
	Name             string    `json:"name,omitempty"`
	CachedAt         time.Time `json:"cached_at"`
	TokenFingerprint string    `json:"token_fingerprint,omitempty"`
}

// userCachePathFunc can be overridden in tests.
var userCachePathFunc = originalUserCachePath

func originalUserCachePath() (string, error) {
	path, err := profile.UserCacheFilePath()
	if err != nil {
		return "", fmt.Errorf("解析当前 profile 失败: %w", err)
	}
	return path, nil
}

// UserCachePath returns the current user cache path (~/.feishu-cli/user_profile.json).
func UserCachePath() (string, error) {
	return userCachePathFunc()
}

// LoadCurrentUserCache loads the current user cache from disk.
// Returns nil, nil when the cache file does not exist.
func LoadCurrentUserCache() (*CurrentUserCache, error) {
	path, err := UserCachePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取当前用户缓存失败: %w", err)
	}

	var cache CurrentUserCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("解析当前用户缓存失败: %w", err)
	}

	return &cache, nil
}

// SaveCurrentUserCache persists the current user cache to disk (0600).
func SaveCurrentUserCache(cache *CurrentUserCache) error {
	if cache == nil {
		return fmt.Errorf("当前用户缓存为空")
	}

	path, err := UserCachePath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	if cache.CachedAt.IsZero() {
		cache.CachedAt = time.Now()
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化当前用户缓存失败: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("写入当前用户缓存失败: %w", err)
	}

	return nil
}

// DeleteCurrentUserCache removes the persisted current user cache.
func DeleteCurrentUserCache() error {
	path, err := UserCachePath()
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("删除当前用户缓存失败: %w", err)
	}

	return nil
}

// UserTokenFingerprint returns a stable fingerprint for the user access token
// without storing the raw token in the cache file.
func UserTokenFingerprint(token string) string {
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:16])
}

// MatchesToken reports whether this cache entry belongs to the given token.
func (c *CurrentUserCache) MatchesToken(token string) bool {
	if c == nil || c.TokenFingerprint == "" {
		return false
	}
	return c.TokenFingerprint == UserTokenFingerprint(token)
}
