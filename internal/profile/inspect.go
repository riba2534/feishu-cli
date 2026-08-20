package profile

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// ConfigFields 是单个 config.yaml 里和身份相关的字段。
// AppSecret 只用于判断是否已配置，调用方不得打印明文。
type ConfigFields struct {
	AppID     string
	AppSecret string
	BaseURL   string
	// UserAccessToken 仅用于判断配置里是否存在静态 user_access_token，
	// 它是 auth.ResolveUserAccessToken 链的最后一级。调用方不得打印明文。
	UserAccessToken string
}

// EnvOverrideFlags 报告哪些凭证被环境变量覆盖。
type EnvOverrideFlags struct {
	AppID     bool `json:"app_id"`
	AppSecret bool `json:"app_secret"`
	Profile   bool `json:"profile"`
}

// EnvOverrides 检测会压过配置文件的环境变量（非空即视为覆盖）。
func EnvOverrides() EnvOverrideFlags {
	return EnvOverrideFlags{
		AppID:     strings.TrimSpace(os.Getenv("FEISHU_APP_ID")) != "",
		AppSecret: strings.TrimSpace(os.Getenv("FEISHU_APP_SECRET")) != "",
		Profile:   strings.TrimSpace(os.Getenv(EnvVar)) != "",
	}
}

// EnvAppID 返回 FEISHU_APP_ID（trim 后），未设置则为空。
func EnvAppID() string {
	return strings.TrimSpace(os.Getenv("FEISHU_APP_ID"))
}

// EnvBaseURL 返回 FEISHU_BASE_URL（trim 后），未设置则为空。
func EnvBaseURL() string {
	return strings.TrimSpace(os.Getenv("FEISHU_BASE_URL"))
}

// ReadConfigFields 读取 dir/config.yaml 的 app_id / app_secret / base_url。
// 文件不存在时返回零值，不报错。使用独立 viper 实例，不污染全局配置。
func ReadConfigFields(dir string) (ConfigFields, error) {
	if dir == "" {
		return ConfigFields{}, nil
	}
	return ReadConfigFieldsFile(filepath.Join(dir, configFileName))
}

// ReadConfigFieldsFile 按显式路径读取配置字段，供 --config 覆盖场景使用。
// 文件不存在时返回零值，不报错。
func ReadConfigFieldsFile(path string) (ConfigFields, error) {
	if path == "" {
		return ConfigFields{}, nil
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return ConfigFields{}, nil
		}
		return ConfigFields{}, err
	}

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		return ConfigFields{}, err
	}
	return ConfigFields{
		AppID:           strings.TrimSpace(v.GetString("app_id")),
		AppSecret:       strings.TrimSpace(v.GetString("app_secret")),
		BaseURL:         strings.TrimRight(strings.TrimSpace(v.GetString("base_url")), "/"),
		UserAccessToken: strings.TrimSpace(v.GetString("user_access_token")),
	}, nil
}

// EffectiveBaseURL 叠加环境变量后的最终 base_url。
func EffectiveBaseURL(fileBaseURL string) string {
	if env := EnvBaseURL(); env != "" {
		return strings.TrimRight(env, "/")
	}
	if fileBaseURL != "" {
		return fileBaseURL
	}
	return "https://open.feishu.cn"
}
