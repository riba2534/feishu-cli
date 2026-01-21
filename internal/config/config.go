package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds the application configuration
type Config struct {
	AppID           string       `mapstructure:"app_id"`
	AppSecret       string       `mapstructure:"app_secret"`
	UserAccessToken string       `mapstructure:"user_access_token"`
	BaseURL         string       `mapstructure:"base_url"`
	Debug           bool         `mapstructure:"debug"`
	Export          ExportConfig `mapstructure:"export"`
	Import          ImportConfig `mapstructure:"import"`
}

// ExportConfig holds export-related configuration
type ExportConfig struct {
	DownloadImages bool   `mapstructure:"download_images"`
	AssetsDir      string `mapstructure:"assets_dir"`
}

// ImportConfig holds import-related configuration
type ImportConfig struct {
	UploadImages bool `mapstructure:"upload_images"`
}

var cfg *Config

// Init initializes the configuration from file and environment
// 配置优先级: 环境变量 > 配置文件 > 默认值
func Init(cfgFile string) error {
	// 1. 设置配置文件路径
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("获取用户目录失败: %w", err)
		}

		configDir := filepath.Join(home, ".feishu-cli")
		viper.AddConfigPath(configDir)
		viper.AddConfigPath(".")
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	// 2. 设置默认值
	viper.SetDefault("base_url", "https://open.feishu.cn")
	viper.SetDefault("debug", false)
	viper.SetDefault("export.download_images", false)
	viper.SetDefault("export.assets_dir", "./assets")
	viper.SetDefault("import.upload_images", true)

	// 3. 环境变量支持（优先级最高）
	viper.SetEnvPrefix("FEISHU")
	viper.AutomaticEnv()

	// 绑定环境变量
	_ = viper.BindEnv("app_id", "FEISHU_APP_ID")
	_ = viper.BindEnv("app_secret", "FEISHU_APP_SECRET")
	_ = viper.BindEnv("user_access_token", "FEISHU_USER_ACCESS_TOKEN")
	_ = viper.BindEnv("base_url", "FEISHU_BASE_URL")
	_ = viper.BindEnv("debug", "FEISHU_DEBUG")

	// 4. 读取配置文件
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("读取配置文件失败: %w", err)
		}
	}

	cfg = &Config{}
	if err := viper.Unmarshal(cfg); err != nil {
		return fmt.Errorf("解析配置失败: %w", err)
	}

	return nil
}

// Get returns the current configuration
func Get() *Config {
	if cfg == nil {
		return &Config{
			BaseURL: "https://open.feishu.cn",
			Export: ExportConfig{
				AssetsDir: "./assets",
			},
			Import: ImportConfig{
				UploadImages: true,
			},
		}
	}
	return cfg
}

// Validate validates the configuration
func Validate() error {
	if cfg == nil {
		return fmt.Errorf("配置未初始化")
	}
	if cfg.AppID == "" {
		return fmt.Errorf("缺少 app_id，请通过以下方式之一设置:\n  1. 环境变量: export FEISHU_APP_ID=xxx\n  2. 配置文件: ~/.feishu-cli/config.yaml")
	}
	if cfg.AppSecret == "" {
		return fmt.Errorf("缺少 app_secret，请通过以下方式之一设置:\n  1. 环境变量: export FEISHU_APP_SECRET=xxx\n  2. 配置文件: ~/.feishu-cli/config.yaml")
	}
	return nil
}

// CreateDefaultConfig creates a default configuration file
func CreateDefaultConfig() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("获取用户目录失败: %w", err)
	}

	configDir := filepath.Join(home, ".feishu-cli")
	// 使用 0700 权限，仅所有者可访问，保护敏感配置
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	configFile := filepath.Join(configDir, "config.yaml")
	if _, err := os.Stat(configFile); err == nil {
		return fmt.Errorf("配置文件已存在: %s", configFile)
	}

	content := `# 飞书 CLI 配置文件
# 从飞书开放平台获取应用凭证: https://open.feishu.cn/app
#
# 配置优先级: 环境变量 > 配置文件 > 默认值
#
# 环境变量方式:
#   export FEISHU_APP_ID=your_app_id
#   export FEISHU_APP_SECRET=your_app_secret

app_id: ""
app_secret: ""
base_url: "https://open.feishu.cn"
debug: false

# 导出配置
export:
  download_images: true    # 导出时下载图片到本地
  assets_dir: "./assets"   # 图片保存目录

# 导入配置
import:
  upload_images: true      # 导入时上传本地图片
`

	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	fmt.Printf("已创建配置文件: %s\n", configFile)
	return nil
}
