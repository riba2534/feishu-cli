package client

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/viper"
)

// resetClient 重置客户端状态，用于测试隔离
func resetClient() {
	mu.Lock()
	defer mu.Unlock()
	instance = nil
	lastCfg.appID = ""
	lastCfg.cfgHash = ""
	lastCfg.baseURL = ""
	lastCfg.debug = false
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

func TestGetClient_CustomBaseURL(t *testing.T) {
	resetClient()
	resetConfig()

	os.Unsetenv("FEISHU_APP_ID")
	os.Unsetenv("FEISHU_APP_SECRET")

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
		t.Fatalf("GetClient() 返回错误: %v", err)
	}

	if client == nil {
		t.Error("GetClient() 返回 nil")
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
