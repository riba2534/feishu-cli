package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var initConfigCmd = &cobra.Command{
	Use:   "init",
	Short: "初始化配置文件",
	Long: `在 ~/.feishu-cli/ 目录下创建默认配置文件。

配置文件位置:
  ~/.feishu-cli/config.yaml

创建后请编辑配置文件，填入您的飞书应用凭证：
  1. 访问 https://open.feishu.cn/app 创建应用
  2. 获取 App ID 和 App Secret
  3. 编辑配置文件填入凭证

也可以使用环境变量（优先级更高）:
  export FEISHU_APP_ID="cli_xxx"
  export FEISHU_APP_SECRET="xxx"

示例:
  feishu-cli config init`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.CreateDefaultConfig(); err != nil {
			return err
		}
		fmt.Println("请编辑配置文件，填入您的飞书应用凭证。")
		return nil
	},
}

func init() {
	configCmd.AddCommand(initConfigCmd)
}
