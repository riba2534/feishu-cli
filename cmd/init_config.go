package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/profile"
	"github.com/spf13/cobra"
)

var initConfigCmd = &cobra.Command{
	Use:   "init",
	Short: "初始化配置文件",
	Long: `在 ~/.feishu-cli/ 目录下创建默认配置文件。

配置文件位置:
  ~/.feishu-cli/config.yaml（启用 profile 时为当前 profile 目录下的 config.yaml）

创建后请编辑配置文件，填入您的飞书应用凭证：
  1. 访问 https://open.feishu.cn/app 创建应用
  2. 获取 App ID 和 App Secret
  3. 编辑配置文件填入凭证

也可以在初始化时直接写入凭证（App Secret 从 stdin 读取，不进入 shell 历史与 ps 输出）：
  --app-id            预填 app_id
  --app-secret-stdin  从 stdin 读取 app_secret（终端下不回显）
  --base-url          预填 base_url（默认 https://open.feishu.cn）
  --probe             写入前换取一次 tenant_access_token 校验凭证；服务端明确拒绝
                      （secret 错误、应用不存在）时退出码 3 且不写配置，网络/临时错误只告警

也可以使用环境变量（优先级更高）:
  export FEISHU_APP_ID="cli_xxx"
  export FEISHU_APP_SECRET="xxx"

示例:
  feishu-cli config init
  printf '%s' "$SECRET" | feishu-cli config init --app-id cli_xxx --app-secret-stdin --probe`,
	RunE: func(cmd *cobra.Command, args []string) error {
		appID, _ := cmd.Flags().GetString("app-id")
		fromStdin, _ := cmd.Flags().GetBool("app-secret-stdin")
		baseURL, _ := cmd.Flags().GetString("base-url")
		probeFlag, _ := cmd.Flags().GetBool("probe")

		// 先确认配置文件不存在，再读 stdin / 探测凭证
		dir, err := profile.ActiveDir()
		if err != nil {
			return fmt.Errorf("获取配置目录失败: %w", err)
		}
		configFile := filepath.Join(dir, "config.yaml")
		if _, err := os.Stat(configFile); err == nil {
			return fmt.Errorf("配置文件已存在: %s（如需修改请直接编辑该文件）", configFile)
		}

		secret, err := resolveSecretFlags("", fromStdin, cmd.InOrStdin())
		if err != nil {
			return err
		}

		if probeFlag {
			if appID == "" || secret == "" {
				return clierr.Usagef("--probe 需要同时提供 --app-id 与 --app-secret-stdin")
			}
			warning, err := probeAppCredentials(appID, secret, baseURL)
			if err != nil {
				return err
			}
			if warning != "" {
				fmt.Fprintf(os.Stderr, "⚠ %s\n", warning)
			} else {
				fmt.Fprintln(os.Stderr, "凭证校验通过")
			}
		}

		if err := config.CreateDefaultConfigWith(config.DefaultConfigOptions{
			AppID:     appID,
			AppSecret: secret,
			BaseURL:   baseURL,
		}); err != nil {
			return err
		}
		if appID == "" || secret == "" {
			fmt.Println("请编辑配置文件，填入您的飞书应用凭证。")
		}
		return nil
	},
}

func init() {
	configCmd.AddCommand(initConfigCmd)
	initConfigCmd.Flags().String("app-id", "", "预填 app_id")
	initConfigCmd.Flags().Bool("app-secret-stdin", false, "从 stdin 读取 app_secret（终端下不回显）")
	initConfigCmd.Flags().String("base-url", "", "预填 base_url（默认 https://open.feishu.cn）")
	initConfigCmd.Flags().Bool("probe", false, "写入前换取一次 tenant_access_token 校验凭证（明确被拒绝时不写配置）")
}
