package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/riba2534/feishu-cli/internal/config"
)

// 默认 API 调用超时时间
const defaultTimeout = 30 * time.Second

var (
	mu       sync.Mutex
	instance *lark.Client
	// lastCfg 用于检测配置变更，不存储敏感信息的明文
	lastCfg struct {
		appID   string
		baseURL string
		debug   bool
		// 使用配置的哈希值而非明文存储 secret
		cfgHash string
	}
)

// GetClient returns a Feishu API client, recreating if config changed
func GetClient() (*lark.Client, error) {
	cfg := config.Get()
	if cfg.AppID == "" || cfg.AppSecret == "" {
		return nil, fmt.Errorf("缺少 app_id 或 app_secret 配置")
	}

	mu.Lock()
	defer mu.Unlock()

	// 使用简单的配置指纹来检测变更，避免存储敏感信息
	currentHash := fmt.Sprintf("%s:%d", cfg.AppID, len(cfg.AppSecret))

	// Check if config changed or instance is nil
	configChanged := instance == nil ||
		lastCfg.appID != cfg.AppID ||
		lastCfg.cfgHash != currentHash ||
		lastCfg.baseURL != cfg.BaseURL ||
		lastCfg.debug != cfg.Debug

	if configChanged {
		opts := []lark.ClientOptionFunc{
			lark.WithOpenBaseUrl(cfg.BaseURL),
		}
		if cfg.Debug {
			opts = append(opts, lark.WithLogLevel(larkcore.LogLevelDebug))
		}
		instance = lark.NewClient(cfg.AppID, cfg.AppSecret, opts...)

		// Save current config (不存储 secret 明文)
		lastCfg.appID = cfg.AppID
		lastCfg.cfgHash = currentHash
		lastCfg.baseURL = cfg.BaseURL
		lastCfg.debug = cfg.Debug
	}

	return instance, nil
}

// Context returns a context with timeout for API calls
// 默认超时时间为 30 秒，防止 API 调用无限阻塞
// 注意：cancel 函数被忽略，因为 CLI 进程生命周期短，context 超时后会自动释放
func Context() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	// 在 CLI 场景下，进程结束时资源会自动释放
	// 这里不调用 cancel 是有意的设计，避免在调用链中传递 cancel
	_ = cancel
	return ctx
}

// ContextWithTimeout returns a context with custom timeout
// 注意：cancel 函数被忽略，因为 CLI 进程生命周期短
func ContextWithTimeout(timeout time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	_ = cancel // 有意忽略，见上方注释
	return ctx
}
