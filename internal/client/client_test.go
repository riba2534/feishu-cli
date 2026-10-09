package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/viper"
)

// resetClient 重置客户端状态，用于测试隔离
func resetClient() {
	mu.Lock()
	defer mu.Unlock()
	instance = nil
	lastCfg.appID = ""
	lastCfg.secretFingerprint = ""
	lastCfg.baseURL = ""
	lastCfg.debug = false
	sdkTestTransport = nil
}

// resetConfig 重置配置状态
func resetConfig() {
	viper.Reset()
}

func TestGetClient_MissingAppID(t *testing.T) {
	resetClient()
	resetConfig()

	// 设置空的 app_id
	os.Unsetenv("FEISHU_APP_ID")
	os.Unsetenv("FEISHU_APP_SECRET")

	// 初始化空配置
	tmpDir := t.TempDir()
	configFile := tmpDir + "/config.yaml"
	os.WriteFile(configFile, []byte("app_secret: test"), 0600)
	config.Init(configFile)

	_, err := GetClient()
	if err == nil {
		t.Error("GetClient() 应返回错误，因为缺少 app_id")
	}
}

func TestGetClient_MissingAppSecret(t *testing.T) {
	resetClient()
	resetConfig()

	os.Unsetenv("FEISHU_APP_ID")
	os.Unsetenv("FEISHU_APP_SECRET")

	tmpDir := t.TempDir()
	configFile := tmpDir + "/config.yaml"
	os.WriteFile(configFile, []byte("app_id: test"), 0600)
	config.Init(configFile)

	_, err := GetClient()
	if err == nil {
		t.Error("GetClient() 应返回错误，因为缺少 app_secret")
	}
}

func TestGetClient_Success(t *testing.T) {
	resetClient()
	resetConfig()

	os.Unsetenv("FEISHU_APP_ID")
	os.Unsetenv("FEISHU_APP_SECRET")

	tmpDir := t.TempDir()
	configFile := tmpDir + "/config.yaml"
	content := `app_id: "test_app_id"
app_secret: "test_app_secret"
base_url: "https://open.feishu.cn"
`
	os.WriteFile(configFile, []byte(content), 0600)
	config.Init(configFile)

	client, err := GetClient()
	if err != nil {
		t.Fatalf("GetClient() 返回错误: %v", err)
	}

	if client == nil {
		t.Error("GetClient() 返回 nil")
	}
}

func TestGetClient_Singleton(t *testing.T) {
	resetClient()
	resetConfig()

	os.Unsetenv("FEISHU_APP_ID")
	os.Unsetenv("FEISHU_APP_SECRET")

	tmpDir := t.TempDir()
	configFile := tmpDir + "/config.yaml"
	content := `app_id: "test_app_id"
app_secret: "test_app_secret"
`
	os.WriteFile(configFile, []byte(content), 0600)
	config.Init(configFile)

	client1, err1 := GetClient()
	if err1 != nil {
		t.Fatalf("GetClient() 第一次调用返回错误: %v", err1)
	}

	client2, err2 := GetClient()
	if err2 != nil {
		t.Fatalf("GetClient() 第二次调用返回错误: %v", err2)
	}

	// 配置未变更时应返回同一实例
	if client1 != client2 {
		t.Error("GetClient() 应返回同一实例（单例模式）")
	}
}

func TestGetClient_ConfigChange(t *testing.T) {
	resetClient()
	resetConfig()

	os.Unsetenv("FEISHU_APP_ID")
	os.Unsetenv("FEISHU_APP_SECRET")

	tmpDir := t.TempDir()
	configFile := tmpDir + "/config.yaml"

	// 第一个配置
	content1 := `app_id: "test_app_id_1"
app_secret: "test_app_secret_1"
`
	os.WriteFile(configFile, []byte(content1), 0600)
	config.Init(configFile)

	client1, _ := GetClient()

	// 更改配置
	resetConfig()
	content2 := `app_id: "test_app_id_2"
app_secret: "test_app_secret_2"
`
	os.WriteFile(configFile, []byte(content2), 0600)
	config.Init(configFile)

	client2, _ := GetClient()

	// 配置变更后应返回新实例
	if client1 == client2 {
		t.Error("配置变更后 GetClient() 应返回新实例")
	}
}

func TestContext(t *testing.T) {
	ctx := Context()

	if ctx == nil {
		t.Fatal("Context() 返回 nil")
	}

	// 验证 context 有 deadline
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Error("Context() 应返回带有 deadline 的 context")
	}

	// 验证 deadline 大约是 30 秒后
	expected := time.Now().Add(30 * time.Second)
	diff := deadline.Sub(expected)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("Deadline 与预期相差过大: %v", diff)
	}
}

func TestContextWithTimeout(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
	}{
		{"1秒", 1 * time.Second},
		{"5秒", 5 * time.Second},
		{"1分钟", 1 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := ContextWithTimeout(tt.timeout)

			if ctx == nil {
				t.Fatal("ContextWithTimeout() 返回 nil")
			}

			deadline, ok := ctx.Deadline()
			if !ok {
				t.Error("ContextWithTimeout() 应返回带有 deadline 的 context")
			}

			expected := time.Now().Add(tt.timeout)
			diff := deadline.Sub(expected)
			if diff < -100*time.Millisecond || diff > 100*time.Millisecond {
				t.Errorf("Deadline 与预期相差过大: %v", diff)
			}
		})
	}
}

func TestContext_Cancellation(t *testing.T) {
	ctx := Context()

	// 验证 context 尚未取消
	select {
	case <-ctx.Done():
		t.Error("Context 不应立即取消")
	default:
		// 预期行为
	}
}

func TestContextWithTimeout_Zero(t *testing.T) {
	// 零超时应该立即过期
	ctx := ContextWithTimeout(0)

	// 等待一小段时间让 context 过期
	time.Sleep(10 * time.Millisecond)

	select {
	case <-ctx.Done():
		// 预期行为
	default:
		t.Error("零超时的 Context 应该已经过期")
	}
}

func TestGetClient_WithDebugMode(t *testing.T) {
	resetClient()
	resetConfig()

	os.Unsetenv("FEISHU_APP_ID")
	os.Unsetenv("FEISHU_APP_SECRET")

	tmpDir := t.TempDir()
	configFile := tmpDir + "/config.yaml"
	content := `app_id: "test_app_id"
app_secret: "test_app_secret"
debug: true
`
	os.WriteFile(configFile, []byte(content), 0600)
	config.Init(configFile)

	client, err := GetClient()
	if err != nil {
		t.Fatalf("GetClient() 返回错误: %v", err)
	}

	if client == nil {
		t.Error("GetClient() 返回 nil")
	}
}

func TestGetClient_CustomBaseURLRejectedByDefault(t *testing.T) {
	resetClient()
	resetConfig()
	t.Setenv("FEISHU_ALLOW_CUSTOM_BASE_URL", "")
	t.Setenv("FEISHU_APP_ID", "")
	t.Setenv("FEISHU_APP_SECRET", "")

	tmpDir := t.TempDir()
	configFile := tmpDir + "/config.yaml"
	content := `app_id: "test_app_id"
app_secret: "test_app_secret"
base_url: "https://custom.feishu.cn"
`
	os.WriteFile(configFile, []byte(content), 0600)
	config.Init(configFile)

	_, err := GetClient()
	if err == nil {
		t.Fatal("未 opt-in 的自定义远端 host 应被拒绝")
	}
}

func TestGetClient_CustomBaseURLOptIn(t *testing.T) {
	resetClient()
	resetConfig()
	t.Setenv("FEISHU_ALLOW_CUSTOM_BASE_URL", "1")
	t.Setenv("FEISHU_APP_ID", "")
	t.Setenv("FEISHU_APP_SECRET", "")

	tmpDir := t.TempDir()
	configFile := tmpDir + "/config.yaml"
	content := `app_id: "test_app_id"
app_secret: "test_app_secret"
base_url: "https://custom.feishu.cn"
`
	os.WriteFile(configFile, []byte(content), 0600)
	config.Init(configFile)

	client, err := GetClient()
	if err != nil {
		t.Fatalf("opt-in 后自定义 HTTPS 应允许: %v", err)
	}
	if client == nil {
		t.Error("GetClient() 返回 nil")
	}
}

func TestGetClient_SameLengthSecretRebuildsWithoutStoringPlaintext(t *testing.T) {
	resetClient()
	resetConfig()
	t.Setenv("FEISHU_APP_ID", "")
	t.Setenv("FEISHU_APP_SECRET", "")

	tmpDir := t.TempDir()
	configFile := tmpDir + "/config.yaml"
	secretA := "length_matched_a"
	secretB := "length_matched_b"
	if len(secretA) != len(secretB) {
		t.Fatal("测试前置：两个 secret 必须同长度")
	}

	content1 := "app_id: \"test_app_id\"\napp_secret: \"" + secretA + "\"\nbase_url: \"https://open.feishu.cn\"\n"
	os.WriteFile(configFile, []byte(content1), 0600)
	config.Init(configFile)

	client1, err := GetClient()
	if err != nil {
		t.Fatalf("第一次 GetClient: %v", err)
	}
	fp1 := lastCfg.secretFingerprint
	if fp1 == "" || fp1 == secretA || len(fp1) != 64 {
		t.Fatalf("fingerprint 应是 SHA-256 hex，不能等于明文")
	}

	resetConfig()
	content2 := "app_id: \"test_app_id\"\napp_secret: \"" + secretB + "\"\nbase_url: \"https://open.feishu.cn\"\n"
	os.WriteFile(configFile, []byte(content2), 0600)
	config.Init(configFile)

	client2, err := GetClient()
	if err != nil {
		t.Fatalf("同长度 secret 变更后 GetClient: %v", err)
	}
	if client1 == client2 {
		t.Fatal("同长度 App Secret 变更后必须重建 client")
	}
	if lastCfg.secretFingerprint == fp1 {
		t.Fatal("secret 指纹应随内容变化")
	}
	if lastCfg.secretFingerprint == secretB {
		t.Fatal("不得把 secret 明文写入指纹缓存")
	}
}

func TestContext_Type(t *testing.T) {
	ctx := Context()

	// 验证返回的是 context.Context 类型
	var _ context.Context = ctx
}

func TestDefaultTimeout(t *testing.T) {
	if defaultTimeout != 30*time.Second {
		t.Errorf("defaultTimeout = %v, 期望 30s", defaultTimeout)
	}
}

// 测试并发获取客户端
func TestGetClient_Concurrent(t *testing.T) {
	resetClient()
	resetConfig()

	os.Unsetenv("FEISHU_APP_ID")
	os.Unsetenv("FEISHU_APP_SECRET")

	tmpDir := t.TempDir()
	configFile := tmpDir + "/config.yaml"
	content := `app_id: "test_app_id"
app_secret: "test_app_secret"
`
	os.WriteFile(configFile, []byte(content), 0600)
	config.Init(configFile)

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			client, err := GetClient()
			if err != nil {
				t.Errorf("并发调用 GetClient() 返回错误: %v", err)
			}
			if client == nil {
				t.Error("并发调用 GetClient() 返回 nil")
			}
			done <- true
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 10; i++ {
		<-done
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestGetClient_HonorsLongerCallerContext(t *testing.T) {
	resetClient()
	resetConfig()
	delay := 80 * time.Millisecond
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"tenant_access_token":"t-slow","expire":7200}`))
	}))
	t.Cleanup(srv.Close)
	setupTestConfig(t, srv.URL)

	cli, err := GetClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = cli.GetTenantAccessTokenBySelfBuiltApp(ctx, &larkcore.SelfBuiltTenantAccessTokenReq{
		AppID: "test_app", AppSecret: "test_secret",
	})
	if err != nil {
		t.Fatalf("调用方 context 长于任何注入墙钟超时应成功: %v", err)
	}
	if time.Since(start) < delay {
		t.Fatal("请求未等到服务端延迟，测试无效")
	}
}

func TestGetClient_OfficialBotNeverSendsSecretToLegacyOpenHost(t *testing.T) {
	resetClient()
	resetConfig()
	t.Setenv("FEISHU_APP_ID", "")
	t.Setenv("FEISHU_APP_SECRET", "")

	var v3Hits int
	var v3Body string
	accounts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v3Hits++
		b, _ := io.ReadAll(r.Body)
		v3Body = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"access_token":"t-from-v3","expires_in":7200}`))
	}))
	t.Cleanup(accounts.Close)
	orig := auth.TATEndpointFunc
	auth.TATEndpointFunc = func(string) string { return accounts.URL }
	t.Cleanup(func() { auth.TATEndpointFunc = orig })

	var leakedLegacy int
	sdkTestTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		if strings.Contains(req.URL.Path, "tenant_access_token/internal") || strings.Contains(string(body), "app_secret") {
			leakedLegacy++
			return nil, fmt.Errorf("legacy internal token 外送: %s", req.URL)
		}
		return nil, fmt.Errorf("unexpected %s", req.URL)
	})
	t.Cleanup(func() { sdkTestTransport = nil })

	tmpDir := t.TempDir()
	configFile := tmpDir + "/config.yaml"
	if err := os.WriteFile(configFile, []byte("app_id: \"cli_prod\"\napp_secret: \"secret_prod\"\nbase_url: \"https://open.feishu.cn\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := config.Init(configFile); err != nil {
		t.Fatal(err)
	}

	cli, err := GetClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := cli.GetTenantAccessTokenBySelfBuiltApp(ctx, &larkcore.SelfBuiltTenantAccessTokenReq{
		AppID: "cli_prod", AppSecret: "secret_prod",
	})
	if err != nil {
		t.Fatalf("官方模式下应由 v3 桥接换票: %v", err)
	}
	if resp == nil || resp.TenantAccessToken != "t-from-v3" {
		t.Fatalf("未翻译为 SDK 兼容响应: %+v", resp)
	}
	if leakedLegacy != 0 {
		t.Fatal("App Secret 被发到旧 Open host")
	}
	if v3Hits != 1 {
		t.Fatalf("v3 hits=%d", v3Hits)
	}
	if !strings.Contains(v3Body, "grant_type=client_credentials") {
		t.Fatalf("v3 body=%s", v3Body)
	}
}

func TestGetClient_DoesNotInterceptAppAccessTokenInternal(t *testing.T) {
	resetClient()
	resetConfig()
	t.Setenv("FEISHU_APP_ID", "")
	t.Setenv("FEISHU_APP_SECRET", "")

	var v3Hits int
	accounts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v3Hits++
		_, _ = w.Write([]byte(`{"code":0,"access_token":"t-tenant","expires_in":7200}`))
	}))
	t.Cleanup(accounts.Close)
	orig := auth.TATEndpointFunc
	auth.TATEndpointFunc = func(string) string { return accounts.URL }
	t.Cleanup(func() { auth.TATEndpointFunc = orig })

	var appPathHits int
	sdkTestTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "app_access_token/internal") {
			appPathHits++
			payload := `{"code":0,"app_access_token":"a-real-app","expire":100}`
			return &http.Response{
				StatusCode:    http.StatusOK,
				Proto:         "HTTP/1.1",
				ProtoMajor:    1,
				ProtoMinor:    1,
				Header:        http.Header{"Content-Type": []string{"application/json"}},
				Body:          io.NopCloser(strings.NewReader(payload)),
				ContentLength: int64(len(payload)),
				Request:       req,
			}, nil
		}
		return nil, fmt.Errorf("unexpected %s", req.URL)
	})
	t.Cleanup(func() { sdkTestTransport = nil })

	tmpDir := t.TempDir()
	configFile := tmpDir + "/config.yaml"
	if err := os.WriteFile(configFile, []byte("app_id: \"cli_app_tok\"\napp_secret: \"secret_app\"\nbase_url: \"https://open.feishu.cn\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := config.Init(configFile); err != nil {
		t.Fatal(err)
	}
	cli, err := GetClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := cli.GetAppAccessTokenBySelfBuiltApp(ctx, &larkcore.SelfBuiltAppAccessTokenReq{
		AppID: "cli_app_tok", AppSecret: "secret_app",
	})
	if err != nil {
		t.Fatalf("app_access_token/internal 不应被 v3 桥接拦截: %v", err)
	}
	if resp == nil || resp.AppAccessToken != "a-real-app" {
		t.Fatalf("不得把 tenant token 标成 app_access_token: %+v", resp)
	}
	if v3Hits != 0 {
		t.Fatalf("app token 路径不得调用 Accounts v3, hits=%d", v3Hits)
	}
	if appPathHits != 1 {
		t.Fatalf("应将请求交给底层传输, hits=%d", appPathHits)
	}
}

func TestGetClient_OfficialBotBusinessCallUsesV3TenantToken(t *testing.T) {
	resetClient()
	resetConfig()
	t.Setenv("FEISHU_APP_ID", "")
	t.Setenv("FEISHU_APP_SECRET", "")

	var v3Hits int
	accounts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v3Hits++
		_, _ = w.Write([]byte(`{"code":0,"access_token":"t-biz","expires_in":3600}`))
	}))
	t.Cleanup(accounts.Close)
	orig := auth.TATEndpointFunc
	auth.TATEndpointFunc = func(string) string { return accounts.URL }
	t.Cleanup(func() { auth.TATEndpointFunc = orig })

	var (
		legacyHits int
		bizAuth    string
		bizHits    int
	)
	sdkTestTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		if strings.Contains(req.URL.Path, "tenant_access_token/internal") || strings.Contains(string(body), "app_secret") && strings.Contains(req.URL.Path, "/open-apis/auth/") {
			legacyHits++
			return nil, fmt.Errorf("legacy credential request: %s", req.URL)
		}
		if strings.Contains(req.URL.Path, "/open-apis/im/v1/chats") {
			bizHits++
			bizAuth = req.Header.Get("Authorization")
			payload := `{"code":0,"msg":"ok","data":{"items":[{"chat_id":"oc_1","name":"n"}],"has_more":false}}`
			return &http.Response{
				StatusCode:    http.StatusOK,
				Proto:         "HTTP/1.1",
				ProtoMajor:    1,
				ProtoMinor:    1,
				Header:        http.Header{"Content-Type": []string{"application/json"}},
				Body:          io.NopCloser(strings.NewReader(payload)),
				ContentLength: int64(len(payload)),
				Request:       req,
			}, nil
		}
		return nil, fmt.Errorf("unexpected %s", req.URL)
	})
	t.Cleanup(func() { sdkTestTransport = nil })

	tmpDir := t.TempDir()
	configFile := tmpDir + "/config.yaml"
	if err := os.WriteFile(configFile, []byte("app_id: \"cli_biz\"\napp_secret: \"secret_biz\"\nbase_url: \"https://open.feishu.cn\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := config.Init(configFile); err != nil {
		t.Fatal(err)
	}

	got, err := ListChats("", "", 10, "", "")
	if err != nil {
		t.Fatalf("业务 API: %v", err)
	}
	if got == nil || len(got.Items) != 1 || got.Items[0].ChatID != "oc_1" {
		t.Fatalf("业务响应 %+v", got)
	}
	if v3Hits != 1 {
		t.Fatalf("Accounts v3 应恰好一次, hits=%d", v3Hits)
	}
	if legacyHits != 0 {
		t.Fatal("不得向旧 Open host 发送凭证换票")
	}
	if bizHits != 1 || bizAuth != "Bearer t-biz" {
		t.Fatalf("业务请求应携带 v3 tenant token, auth=%q hits=%d", bizAuth, bizHits)
	}
}
