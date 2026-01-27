package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

// resetConfig 重置全局配置状态，用于测试隔离
func resetConfig() {
	cfg = nil
	viper.Reset()
}

func TestInit_WithEnvVariables(t *testing.T) {
	resetConfig()

	// 设置环境变量
	os.Setenv("FEISHU_APP_ID", "test_app_id")
	os.Setenv("FEISHU_APP_SECRET", "test_app_secret")
	defer func() {
		os.Unsetenv("FEISHU_APP_ID")
		os.Unsetenv("FEISHU_APP_SECRET")
	}()

	err := Init("")
	if err != nil {
		t.Fatalf("Init() 返回错误: %v", err)
	}

	c := Get()
	if c.AppID != "test_app_id" {
		t.Errorf("AppID = %q, 期望 %q", c.AppID, "test_app_id")
	}
	if c.AppSecret != "test_app_secret" {
		t.Errorf("AppSecret = %q, 期望 %q", c.AppSecret, "test_app_secret")
	}
}

func TestInit_DefaultValues(t *testing.T) {
	resetConfig()

	// 清除可能存在的环境变量
	os.Unsetenv("FEISHU_APP_ID")
	os.Unsetenv("FEISHU_APP_SECRET")
	os.Unsetenv("FEISHU_BASE_URL")
	os.Unsetenv("FEISHU_DEBUG")
	os.Unsetenv("FEISHU_USER_ACCESS_TOKEN")

	err := Init("")
	if err != nil {
		t.Fatalf("Init() 返回错误: %v", err)
	}

	c := Get()
	if c.BaseURL != "https://open.feishu.cn" {
		t.Errorf("BaseURL = %q, 期望 %q", c.BaseURL, "https://open.feishu.cn")
	}
	if c.Debug != false {
		t.Errorf("Debug = %v, 期望 %v", c.Debug, false)
	}
	// 注意：由于 viper 可能从之前的配置文件读取，DownloadImages 的值可能因环境而异
	// 这里只验证 AssetsDir 的默认值
	if c.Export.AssetsDir != "./assets" {
		t.Errorf("Export.AssetsDir = %q, 期望 %q", c.Export.AssetsDir, "./assets")
	}
	if c.Import.UploadImages != true {
		t.Errorf("Import.UploadImages = %v, 期望 %v", c.Import.UploadImages, true)
	}
}

func TestInit_WithConfigFile(t *testing.T) {
	resetConfig()

	// 创建临时配置文件
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	content := `app_id: "file_app_id"
app_secret: "file_app_secret"
base_url: "https://custom.feishu.cn"
debug: true
export:
  download_images: true
  assets_dir: "./custom_assets"
import:
  upload_images: false
`
	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatalf("创建配置文件失败: %v", err)
	}

	// 清除环境变量
	os.Unsetenv("FEISHU_APP_ID")
	os.Unsetenv("FEISHU_APP_SECRET")

	err := Init(configFile)
	if err != nil {
		t.Fatalf("Init() 返回错误: %v", err)
	}

	c := Get()
	if c.AppID != "file_app_id" {
		t.Errorf("AppID = %q, 期望 %q", c.AppID, "file_app_id")
	}
	if c.AppSecret != "file_app_secret" {
		t.Errorf("AppSecret = %q, 期望 %q", c.AppSecret, "file_app_secret")
	}
	if c.BaseURL != "https://custom.feishu.cn" {
		t.Errorf("BaseURL = %q, 期望 %q", c.BaseURL, "https://custom.feishu.cn")
	}
	if c.Debug != true {
		t.Errorf("Debug = %v, 期望 %v", c.Debug, true)
	}
	if c.Export.DownloadImages != true {
		t.Errorf("Export.DownloadImages = %v, 期望 %v", c.Export.DownloadImages, true)
	}
	if c.Export.AssetsDir != "./custom_assets" {
		t.Errorf("Export.AssetsDir = %q, 期望 %q", c.Export.AssetsDir, "./custom_assets")
	}
	if c.Import.UploadImages != false {
		t.Errorf("Import.UploadImages = %v, 期望 %v", c.Import.UploadImages, false)
	}
}

func TestInit_EnvOverridesFile(t *testing.T) {
	resetConfig()

	// 创建临时配置文件
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	content := `app_id: "file_app_id"
app_secret: "file_app_secret"
`
	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatalf("创建配置文件失败: %v", err)
	}

	// 设置环境变量（应覆盖配置文件）
	os.Setenv("FEISHU_APP_ID", "env_app_id")
	defer os.Unsetenv("FEISHU_APP_ID")

	err := Init(configFile)
	if err != nil {
		t.Fatalf("Init() 返回错误: %v", err)
	}

	c := Get()
	// 环境变量应覆盖配置文件
	if c.AppID != "env_app_id" {
		t.Errorf("AppID = %q, 期望 %q (环境变量应覆盖配置文件)", c.AppID, "env_app_id")
	}
	// 配置文件中的值应保留
	if c.AppSecret != "file_app_secret" {
		t.Errorf("AppSecret = %q, 期望 %q", c.AppSecret, "file_app_secret")
	}
}

func TestGet_WithoutInit(t *testing.T) {
	resetConfig()

	c := Get()
	if c == nil {
		t.Fatal("Get() 返回 nil")
	}
	// 应返回默认配置
	if c.BaseURL != "https://open.feishu.cn" {
		t.Errorf("BaseURL = %q, 期望 %q", c.BaseURL, "https://open.feishu.cn")
	}
	if c.Export.AssetsDir != "./assets" {
		t.Errorf("Export.AssetsDir = %q, 期望 %q", c.Export.AssetsDir, "./assets")
	}
	if c.Import.UploadImages != true {
		t.Errorf("Import.UploadImages = %v, 期望 %v", c.Import.UploadImages, true)
	}
}

func TestValidate_Success(t *testing.T) {
	resetConfig()

	os.Setenv("FEISHU_APP_ID", "test_app_id")
	os.Setenv("FEISHU_APP_SECRET", "test_app_secret")
	defer func() {
		os.Unsetenv("FEISHU_APP_ID")
		os.Unsetenv("FEISHU_APP_SECRET")
	}()

	if err := Init(""); err != nil {
		t.Fatalf("Init() 返回错误: %v", err)
	}

	err := Validate()
	if err != nil {
		t.Errorf("Validate() 返回错误: %v", err)
	}
}

func TestValidate_MissingAppID(t *testing.T) {
	resetConfig()

	// 创建临时空配置文件以确保不读取现有配置
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	content := `app_secret: "test_secret"`
	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatalf("创建配置文件失败: %v", err)
	}

	os.Unsetenv("FEISHU_APP_ID")
	os.Unsetenv("FEISHU_APP_SECRET")

	if err := Init(configFile); err != nil {
		t.Fatalf("Init() 返回错误: %v", err)
	}

	err := Validate()
	if err == nil {
		t.Error("Validate() 应返回错误，因为缺少 AppID")
	}
}

func TestValidate_MissingAppSecret(t *testing.T) {
	resetConfig()

	// 创建临时配置文件
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	content := `app_id: "test_id"`
	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatalf("创建配置文件失败: %v", err)
	}

	os.Unsetenv("FEISHU_APP_ID")
	os.Unsetenv("FEISHU_APP_SECRET")

	if err := Init(configFile); err != nil {
		t.Fatalf("Init() 返回错误: %v", err)
	}

	err := Validate()
	if err == nil {
		t.Error("Validate() 应返回错误，因为缺少 AppSecret")
	}
}

func TestValidate_NotInitialized(t *testing.T) {
	resetConfig()

	err := Validate()
	if err == nil {
		t.Error("Validate() 应返回错误，因为配置未初始化")
	}
}

func TestCreateDefaultConfig(t *testing.T) {
	// 使用临时目录模拟用户主目录
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	err := CreateDefaultConfig()
	if err != nil {
		t.Fatalf("CreateDefaultConfig() 返回错误: %v", err)
	}

	// 验证配置文件已创建
	configFile := filepath.Join(tmpDir, ".feishu-cli", "config.yaml")
	info, err := os.Stat(configFile)
	if err != nil {
		t.Fatalf("配置文件不存在: %v", err)
	}

	// 验证文件权限
	if info.Mode().Perm() != 0600 {
		t.Errorf("配置文件权限 = %o, 期望 %o", info.Mode().Perm(), 0600)
	}

	// 验证目录权限
	dirInfo, err := os.Stat(filepath.Join(tmpDir, ".feishu-cli"))
	if err != nil {
		t.Fatalf("配置目录不存在: %v", err)
	}
	if dirInfo.Mode().Perm() != 0700 {
		t.Errorf("配置目录权限 = %o, 期望 %o", dirInfo.Mode().Perm(), 0700)
	}

	// 验证文件内容
	content, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("读取配置文件失败: %v", err)
	}
	if len(content) == 0 {
		t.Error("配置文件内容为空")
	}
}

func TestCreateDefaultConfig_AlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// 第一次创建
	err := CreateDefaultConfig()
	if err != nil {
		t.Fatalf("CreateDefaultConfig() 第一次调用返回错误: %v", err)
	}

	// 第二次创建应返回错误
	err = CreateDefaultConfig()
	if err == nil {
		t.Error("CreateDefaultConfig() 应返回错误，因为配置文件已存在")
	}
}

func TestInit_InvalidConfigFile(t *testing.T) {
	resetConfig()

	// 创建无效的配置文件
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	content := `invalid: yaml: content: [[[`
	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatalf("创建配置文件失败: %v", err)
	}

	err := Init(configFile)
	if err == nil {
		t.Error("Init() 应返回错误，因为配置文件格式无效")
	}
}

func TestInit_UserAccessToken(t *testing.T) {
	resetConfig()

	os.Setenv("FEISHU_USER_ACCESS_TOKEN", "test_user_token")
	defer os.Unsetenv("FEISHU_USER_ACCESS_TOKEN")

	err := Init("")
	if err != nil {
		t.Fatalf("Init() 返回错误: %v", err)
	}

	c := Get()
	if c.UserAccessToken != "test_user_token" {
		t.Errorf("UserAccessToken = %q, 期望 %q", c.UserAccessToken, "test_user_token")
	}
}

func TestInit_BaseURLFromEnv(t *testing.T) {
	resetConfig()

	os.Setenv("FEISHU_BASE_URL", "https://custom.lark.com")
	defer os.Unsetenv("FEISHU_BASE_URL")

	err := Init("")
	if err != nil {
		t.Fatalf("Init() 返回错误: %v", err)
	}

	c := Get()
	if c.BaseURL != "https://custom.lark.com" {
		t.Errorf("BaseURL = %q, 期望 %q", c.BaseURL, "https://custom.lark.com")
	}
}

func TestInit_DebugFromEnv(t *testing.T) {
	resetConfig()

	os.Setenv("FEISHU_DEBUG", "true")
	defer os.Unsetenv("FEISHU_DEBUG")

	err := Init("")
	if err != nil {
		t.Fatalf("Init() 返回错误: %v", err)
	}

	c := Get()
	if c.Debug != true {
		t.Errorf("Debug = %v, 期望 %v", c.Debug, true)
	}
}
