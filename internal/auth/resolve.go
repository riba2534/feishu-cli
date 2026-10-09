package auth

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/riba2534/feishu-cli/v2/internal/clierr"
)

// logf 输出日志到 stderr，避免污染 stdout 的 JSON 输出
func logf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
}

// ErrNoUserTokenConfigured 表示未找到任何 User Token 来源（无 flag/env/token.json/config）。
var ErrNoUserTokenConfigured = errors.New("缺少 User Access Token，请通过以下方式之一提供:\n" +
	"  1. OAuth 登录: feishu-cli auth login\n" +
	"  2. 命令行参数: --user-access-token <token>\n" +
	"  3. 环境变量: export FEISHU_USER_ACCESS_TOKEN=<token>\n" +
	"  4. 配置文件: user_access_token: <token>")

// IsNoUserTokenConfigured 判断是否为未配置 User Token 的自然缺失状态。
func IsNoUserTokenConfigured(err error) bool {
	return errors.Is(err, ErrNoUserTokenConfigured)
}

// HasUserTokenConfigured 静态探测当前环境是否存在 User Token 配置（无网络请求，不执行刷新）。
func HasUserTokenConfigured(flagValue, configValue string) bool {
	if flagValue != "" {
		return true
	}
	if os.Getenv("FEISHU_USER_ACCESS_TOKEN") != "" {
		return true
	}
	token, err := LoadToken()
	if err == nil && token != nil && (token.AccessToken != "" || token.RefreshToken != "") {
		return true
	}
	if configValue != "" {
		return true
	}
	return false
}

// ResolveUserAccessToken 按优先级链获取 user_access_token，支持自动刷新
//
// 优先级:
//  1. flagValue（--user-access-token 参数）
//     - 若 flagValue 等于 token.json 中已过期的 access_token 且 refresh_token 仍有效，
//     自动刷新并返回新 access_token（写回 token.json）。常见场景：脚本从 token.json
//     读取 access_token 后传入 --user-access-token，本质是延伸本机身份。
//  2. FEISHU_USER_ACCESS_TOKEN 环境变量（同样支持本机身份延伸时的自动刷新）
//  3. token.json（access_token 有效直接返回；过期则用 refresh_token 刷新）
//  4. configValue（config.yaml 静态配置）
//  5. 全部为空 → 返回 ErrNoUserTokenConfigured
func ResolveUserAccessToken(flagValue, configValue, appID, appSecret, baseURL string) (string, error) {
	token, err := resolveUserAccessToken(flagValue, configValue, appID, appSecret, baseURL)
	if err != nil {
		// 统一标记为鉴权类错误（退出码 3）；文本不变，errors.Is(ErrNoUserTokenConfigured) 仍成立。
		// 刷新因断网失败时错误链里带网络错误，cmd 层会优先按网络错误（退出码 4）处理。
		return "", clierr.Auth(err)
	}
	return token, nil
}

func resolveUserAccessToken(flagValue, configValue, appID, appSecret, baseURL string) (string, error) {
	// 1. 命令行参数
	if flagValue != "" {
		refreshed, err := refreshIfStaleLocalToken(flagValue, appID, appSecret, baseURL)
		if err != nil {
			return "", err
		}
		if refreshed != "" {
			return refreshed, nil
		}
		return flagValue, nil
	}

	// 2. 环境变量
	if envToken := os.Getenv("FEISHU_USER_ACCESS_TOKEN"); envToken != "" {
		refreshed, err := refreshIfStaleLocalToken(envToken, appID, appSecret, baseURL)
		if err != nil {
			return "", err
		}
		if refreshed != "" {
			return refreshed, nil
		}
		return envToken, nil
	}

	// 3. token.json：任何使用都必须已绑定当前 App，未绑定不得因 access 仍有效而静默沿用。
	var tokenFileExpired bool
	token, err := LoadToken()
	if err != nil {
		return "", fmt.Errorf("读取本地 token 文件失败: %w", err)
	}
	if token != nil {
		if err := token.RequireBoundApp(appID); err != nil {
			return "", err
		}
		if token.IsAccessTokenValid() {
			return token.AccessToken, nil
		}

		// access_token 过期，尝试刷新（跨进程锁：reload → check → refresh → commit）
		if token.IsRefreshTokenValid() {
			logf("[自动刷新] Access Token 已过期，正在刷新...")
			newToken, refreshErr := refreshLocalTokenLocked(appID, appSecret, baseURL, false, token.RefreshToken)
			if refreshErr != nil {
				logf("[自动刷新] 刷新失败: %v", refreshErr)
				return "", fmt.Errorf("自动刷新 Access Token 失败: %w", refreshErr)
			}
			logf("[自动刷新] 刷新成功，新 Token 有效期至 %s", newToken.ExpiresAt.Format("2006-01-02 15:04:05"))
			return newToken.AccessToken, nil
		}
		tokenFileExpired = true // token.json 存在但所有 token 都过期了
	}

	// 4. 配置文件
	if configValue != "" {
		return configValue, nil
	}

	// 5. 区分"从未登录"和"登录过期"
	if tokenFileExpired {
		if token.RefreshFailure != nil {
			// 终态标记：上一次刷新已被服务端判定 refresh_token 失效，不再发起注定失败的刷新
			return "", token.RefreshFailure.Err()
		}
		return "", fmt.Errorf("User Access Token 已过期（access_token 和 refresh_token 均已失效）。\n" +
			"请重新登录: feishu-cli auth login")
	}
	return "", ErrNoUserTokenConfigured
}

// refreshIfStaleLocalToken 当显式传入的 token 等于 token.json 的 access_token 时，
// 按本地绑定/刷新规则处理。仅当显式值与本地 access_token 不同（外部无关 token）时，
// 才绕过本地绑定，让调用方原样使用显式值。
//
// 返回值:
//   - (newToken, nil): 已成功刷新，调用方使用新 token
//   - ("", nil): 与本地无关或本地仍有效，调用方使用原始显式 token
//   - ("", err): 显式值匹配 token.json，但绑定校验或刷新失败，必须 fail closed
func refreshIfStaleLocalToken(explicitToken, appID, appSecret, baseURL string) (string, error) {
	local, err := LoadToken()
	if err != nil {
		// 无法解析本地文件就不能判定“匹配”，外部显式 token 保持优先。
		return "", nil
	}
	if local == nil || local.AccessToken != explicitToken {
		return "", nil
	}
	if err := local.RequireBoundApp(appID); err != nil {
		return "", err
	}
	if local.IsAccessTokenValid() {
		return "", nil
	}
	if local.RefreshFailure != nil {
		return "", local.RefreshFailure.Err()
	}
	if !local.IsRefreshTokenValid() {
		return "", fmt.Errorf("显式传入的 access_token 匹配本地 token.json，但 refresh_token 已失效。请重新 `feishu-cli auth login`")
	}
	logf("[自动刷新] 显式传入的 access_token 已过期且匹配本地 token.json，正在刷新...")
	newToken, refreshErr := refreshLocalTokenLocked(appID, appSecret, baseURL, false, local.RefreshToken)
	if refreshErr != nil {
		logf("[自动刷新] 刷新失败: %v", refreshErr)
		return "", fmt.Errorf("自动刷新 Access Token 失败: %w", refreshErr)
	}
	logf("[自动刷新] 刷新成功，新 Token 有效期至 %s", newToken.ExpiresAt.Format("2006-01-02 15:04:05"))
	return newToken.AccessToken, nil
}

// EnsureFreshLocalToken 按业务命令同样的规则取得 token.json 中可用的 User Token。
//
// snapshot 是调用方先前从 token.json 读到的快照：
//   - 先校验已绑定当前 App（RequireBoundApp）：未绑定的旧 token 不得被静默绑定或刷新；
//   - access_token 仍有效 → 原样返回（refreshed=false）；
//   - 否则走跨进程加锁的 reload → 代际校验 → refresh → commit 路径，
//     其他进程已轮换时直接采用新一代，不会二次消耗同一 refresh_token。
//
// 供 `auth status --verify` 等诊断命令复用，禁止绕过锁直接 RefreshAccessToken + SaveToken。
func EnsureFreshLocalToken(appID, appSecret, baseURL string, snapshot *TokenStore) (*TokenStore, bool, error) {
	fresh, refreshed, err := ensureFreshLocalToken(appID, appSecret, baseURL, snapshot)
	if err != nil {
		return nil, false, clierr.Auth(err)
	}
	return fresh, refreshed, nil
}

func ensureFreshLocalToken(appID, appSecret, baseURL string, snapshot *TokenStore) (*TokenStore, bool, error) {
	if snapshot == nil {
		return nil, false, fmt.Errorf("未登录（token.json 不存在），请先 `feishu-cli auth login`")
	}
	if err := snapshot.RequireBoundApp(appID); err != nil {
		return nil, false, err
	}
	if snapshot.IsAccessTokenValid() {
		return snapshot, false, nil
	}
	if snapshot.RefreshFailure != nil {
		return nil, false, snapshot.RefreshFailure.Err()
	}
	if !snapshot.IsRefreshTokenValid() {
		return nil, false, fmt.Errorf("access_token 和 refresh_token 都已失效，请重新 `feishu-cli auth login`")
	}
	fresh, err := refreshLocalTokenLocked(appID, appSecret, baseURL, false, snapshot.RefreshToken)
	if err != nil {
		return nil, false, err
	}
	return fresh, fresh.AccessToken != snapshot.AccessToken, nil
}

// ForceRefreshLocalToken 强制刷新 token.json 中的 access_token，
// 即使当前 access_token 仍然有效。由 `auth refresh` 子命令调用。
//
// 失败原因可能是: token.json 不存在、refresh_token 已过期、网络/服务端错误。
func ForceRefreshLocalToken(appID, appSecret, baseURL string) (*TokenStore, error) {
	store, err := forceRefreshLocalToken(appID, appSecret, baseURL)
	if err != nil {
		return nil, clierr.Auth(err)
	}
	return store, nil
}

func forceRefreshLocalToken(appID, appSecret, baseURL string) (*TokenStore, error) {
	local, err := LoadToken()
	if err != nil {
		return nil, fmt.Errorf("读取 token.json 失败: %w", err)
	}
	if local == nil {
		return nil, fmt.Errorf("未登录（token.json 不存在），请先 `feishu-cli auth login`")
	}
	if local.RefreshToken == "" {
		return nil, fmt.Errorf("token.json 中缺少 refresh_token，请重新 `feishu-cli auth login`")
	}
	if local.RefreshFailure != nil {
		return nil, local.RefreshFailure.Err()
	}
	if !local.IsRefreshTokenValid() {
		return nil, fmt.Errorf("refresh_token 已过期（%s），请重新 `feishu-cli auth login`",
			local.RefreshExpiresAt.Format("2006-01-02 15:04:05"))
	}
	if err := local.RequireBoundApp(appID); err != nil {
		return nil, err
	}
	return refreshLocalTokenLocked(appID, appSecret, baseURL, true, local.RefreshToken)
}

// refreshLocalTokenLocked 在跨进程锁下 reload→check→refresh→commit。
// expectedRefresh 是加锁前看到的 refresh_token 代际；reload 后若已轮换则直接采用，避免二次消耗。
// 写失败时保留旧 token 文件。force 为 true 时即使 access 仍有效也刷新。
func refreshLocalTokenLocked(appID, appSecret, baseURL string, force bool, expectedRefresh string) (*TokenStore, error) {
	path, err := TokenPath()
	if err != nil {
		return nil, err
	}
	var result *TokenStore
	err = withTokenFileLock(path, func() error {
		current, err := LoadTokenFrom(path)
		if err != nil {
			return err
		}
		if current == nil {
			return fmt.Errorf("未登录（token.json 不存在），请先 `feishu-cli auth login`")
		}
		if err := current.RequireBoundApp(appID); err != nil {
			return err
		}
		if expectedRefresh != "" && current.RefreshToken != expectedRefresh {
			result = current
			return nil
		}
		if !force && current.IsAccessTokenValid() {
			result = current
			return nil
		}
		if current.RefreshFailure != nil {
			return current.RefreshFailure.Err()
		}
		if !current.IsRefreshTokenValid() {
			return fmt.Errorf("refresh_token 已过期，请重新 `feishu-cli auth login`")
		}
		snapshotRefresh := current.RefreshToken
		fresh, refreshErr := RefreshAccessToken(current, appID, appSecret, baseURL)
		if refreshErr != nil {
			// 可能已被另一进程消耗同一 refresh_token；reload 后若已是新一代则直接采用。
			reloaded, loadErr := LoadTokenFrom(path)
			if loadErr == nil && reloaded != nil && reloaded.RefreshToken != snapshotRefresh && reloaded.IsAccessTokenValid() {
				if bindErr := reloaded.CheckAppMismatch(appID); bindErr == nil {
					result = reloaded
					return nil
				}
			}
			// 终态失效：在 token.json 记录标记（仍是同一代 refresh_token 时），后续命令直接报错，
			// 不再每条命令重复发起注定失败的刷新；重新 auth login 写入新 token 后标记自然消失。
			// 保持 fail-closed：不删除 token、不静默切换 Bot。
			var re *RefreshError
			if errors.As(refreshErr, &re) && re.Terminal && loadErr == nil && reloaded != nil && reloaded.RefreshToken == snapshotRefresh {
				reloaded.RefreshFailure = &RefreshFailure{
					Code:        re.Code,
					Error:       re.OAuthError,
					Description: re.Description,
					At:          time.Now(),
				}
				if werr := writeTokenFileUnlocked(path, reloaded, false); werr != nil {
					logf("[自动刷新] 记录 refresh_token 失效标记失败: %v", werr)
				}
			}
			return refreshErr
		}
		if err := writeTokenUnlocked(path, fresh); err != nil {
			return fmt.Errorf("刷新成功但写入 token.json 失败（原文件未改动）: %w", err)
		}
		result = fresh
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
