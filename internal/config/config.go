package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/riba2534/feishu-cli/internal/profile"
	"github.com/spf13/viper"
)

// Config holds the application configuration
type Config struct {
	AppID             string       `mapstructure:"app_id"`
	AppSecret         string       `mapstructure:"app_secret"`
	UserAccessToken   string       `mapstructure:"user_access_token"`
	BaseURL           string       `mapstructure:"base_url"`
	OwnerEmail        string       `mapstructure:"owner_email"`
	TransferOwnership bool         `mapstructure:"transfer_ownership"`
	Debug             bool         `mapstructure:"debug"`
	Export            ExportConfig `mapstructure:"export"`
	Import            ImportConfig `mapstructure:"import"`
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

// 命令行 --bot-app-id / --bot-app-secret，优先于环境变量和配置文件，不写盘。
var botFlagAppID, botFlagAppSecret string

// SetBotFlagCredentials 记录本进程 --bot-app-id / --bot-app-secret（trim 后）。
// 空字符串表示该 flag 未传，不覆盖。
func SetBotFlagCredentials(appID, appSecret string) {
	botFlagAppID = strings.TrimSpace(appID)
	botFlagAppSecret = strings.TrimSpace(appSecret)
}

// BotFlagAppID 返回 --bot-app-id，未传则为空。
func BotFlagAppID() string { return botFlagAppID }

// BotFlagAppSecretSet 报告是否传了 --bot-app-secret（不返回明文）。
func BotFlagAppSecretSet() bool { return botFlagAppSecret != "" }

func applyBotFlagCredentials() {
	if botFlagAppID != "" {
		if cfg != nil {
			cfg.AppID = botFlagAppID
		}
		viper.Set("app_id", botFlagAppID)
	}
	if botFlagAppSecret != "" {
		if cfg != nil {
			cfg.AppSecret = botFlagAppSecret
		}
		viper.Set("app_secret", botFlagAppSecret)
	}
}

// ApplyBotFlagCredentials 在 Init 被跳过时仍把 flag 写进进程配置，供 doctor/list 看到覆盖。
func ApplyBotFlagCredentials() {
	if botFlagAppID == "" && botFlagAppSecret == "" {
		return
	}
	if cfg == nil {
		cfg = &Config{
			BaseURL: "https://open.feishu.cn",
			Export:  ExportConfig{AssetsDir: "./assets"},
			Import:  ImportConfig{UploadImages: true},
		}
	}
	applyBotFlagCredentials()
}

// Init initializes the configuration from file and environment
// 应用凭证优先级: --bot-app-id/--bot-app-secret > 环境变量 > 选中目录的配置文件 > 默认值
//
// 配置文件路径解析顺序：
//  1. --config <path> 显式指定
//  2. ${FEISHU_PROFILE}/${active-profile} 指向的 profile 目录下的 config.yaml
//  3. 旧布局 ~/.feishu-cli/config.yaml（向后兼容，profile 系统未启用时）
func Init(cfgFile string) error {
	// 1. 设置配置文件路径
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		// 优先走 profile 系统，profile 未启用时回退到旧布局
		dir, err := profile.ActiveDir()
		if err != nil {
			return fmt.Errorf("解析当前 profile 失败: %w", err)
		}
		viper.AddConfigPath(dir)
		viper.AddConfigPath(".")
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	// 2. 设置默认值
	viper.SetDefault("base_url", "https://open.feishu.cn")
	viper.SetDefault("owner_email", "")
	viper.SetDefault("transfer_ownership", false)
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
	_ = viper.BindEnv("owner_email", "FEISHU_OWNER_EMAIL")
	_ = viper.BindEnv("transfer_ownership", "FEISHU_TRANSFER_OWNERSHIP")
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

	// 统一去除 BaseURL 尾部斜杠，避免拼接 API 路径时产生双斜杠
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")

	// 5. 命令行凭证覆盖环境变量和文件，不写盘
	applyBotFlagCredentials()

	return nil
}

// Get returns the current configuration
func Get() *Config {
	if cfg == nil {
		return &Config{
			BaseURL:           "https://open.feishu.cn",
			OwnerEmail:        "",
			TransferOwnership: false,
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
	cfgPath := activeConfigPathForError()
	if cfg.AppID == "" {
		return fmt.Errorf("缺少 app_id，请通过以下方式之一设置:\n  1. 命令行: --bot-app-id cli_xxx --bot-app-secret xxx\n  2. 环境变量: export FEISHU_APP_ID=xxx\n  3. 配置文件: %s", cfgPath)
	}
	if cfg.AppSecret == "" {
		return fmt.Errorf("缺少 app_secret，请通过以下方式之一设置:\n  1. 命令行: --bot-app-id cli_xxx --bot-app-secret xxx\n  2. 环境变量: export FEISHU_APP_SECRET=xxx\n  3. 配置文件: %s", cfgPath)
	}
	return nil
}

// activeConfigPathForError 返回当前 profile 的 config.yaml 路径（用于错误提示）。
// 解析失败时回退到旧布局占位符，避免 Validate 报错叠加二次错误。
func activeConfigPathForError() string {
	if path, err := profile.ConfigFilePath(); err == nil && path != "" {
		return path
	}
	if root, err := profile.RootDir(); err == nil && root != "" {
		return filepath.Join(root, "config.yaml")
	}
	return "~/.feishu-cli/config.yaml"
}

// CreateDefaultConfig creates a default configuration file in the active profile
// directory (legacy ~/.feishu-cli/ when profile system not initialized).
func CreateDefaultConfig() error {
	configDir, err := profile.ActiveDir()
	if err != nil {
		return fmt.Errorf("获取配置目录失败: %w", err)
	}
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
# 应用凭证优先级: --bot-app-id/--bot-app-secret > 环境变量 > 配置文件 > 默认值
#
# 环境变量方式:
#   export FEISHU_APP_ID=your_app_id
#   export FEISHU_APP_SECRET=your_app_secret

app_id: ""
app_secret: ""
base_url: "https://open.feishu.cn"
owner_email: ""              # 文档创建后自动授权的邮箱（环境变量: FEISHU_OWNER_EMAIL）
transfer_ownership: false    # 创建文档后是否转移所有权给 owner_email（默认仅添加 full_access）
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
