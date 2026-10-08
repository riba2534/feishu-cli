package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
)

// 默认 API 调用超时时间
const defaultTimeout = 30 * time.Second

var (
	mu       sync.Mutex
	instance *lark.Client
	// lastCfg 用于检测配置变更。secretFingerprint 是 SHA-256 十六进制，绝不保存明文。
	lastCfg struct {
		appID             string
		baseURL           string
		debug             bool
		secretFingerprint string
	}
)

// secretFingerprint 返回 App Secret 的不可逆指纹，用于检测同长度 secret 轮换。
func secretFingerprint(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// GetClient returns a Feishu API client, recreating if config changed
func GetClient() (*lark.Client, error) {
	cfg := config.Get()
	if cfg.AppID == "" || cfg.AppSecret == "" {
		return nil, clierr.Authf("缺少 app_id 或 app_secret 配置")
	}
	if err := config.CheckBaseURL(cfg.BaseURL); err != nil {
		return nil, err
	}

	mu.Lock()
	defer mu.Unlock()

	fp := secretFingerprint(cfg.AppSecret)

	configChanged := instance == nil ||
		lastCfg.appID != cfg.AppID ||
		lastCfg.secretFingerprint != fp ||
		lastCfg.baseURL != cfg.BaseURL ||
		lastCfg.debug != cfg.Debug

	if configChanged {
		opts := []lark.ClientOptionFunc{
			lark.WithOpenBaseUrl(cfg.BaseURL),
			lark.WithHttpClient(wrapSDKHTTPClient()),
		}
		if cfg.Debug {
			opts = append(opts, lark.WithLogLevel(larkcore.LogLevelDebug))
		}
		instance = lark.NewClient(cfg.AppID, cfg.AppSecret, opts...)

		lastCfg.appID = cfg.AppID
		lastCfg.secretFingerprint = fp
		lastCfg.baseURL = cfg.BaseURL
		lastCfg.debug = cfg.Debug
	}

	return instance, nil
}

// Context returns a context with timeout for API calls.
// 默认超时时间为 30 秒，防止 API 调用无限阻塞。
// 通过 goroutine 等待 ctx.Done 后调用 cancel，释放关联的计时器资源。
func Context() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	go func() {
		<-ctx.Done()
		cancel()
	}()
	return ctx
}

// ContextWithTimeout returns a context with custom timeout.
func ContextWithTimeout(timeout time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	go func() {
		<-ctx.Done()
		cancel()
	}()
	return ctx
}
