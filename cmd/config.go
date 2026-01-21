package cmd

import (
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "配置管理命令",
	Long: `配置管理命令，用于初始化和管理 CLI 配置。

子命令:
  init    初始化配置文件

配置文件位置:
  ~/.feishu-cli/config.yaml

配置优先级:
  环境变量 > 配置文件 > 默认值

环境变量:
  FEISHU_APP_ID      应用 ID
  FEISHU_APP_SECRET  应用密钥
  FEISHU_BASE_URL    API 地址（可选）
  FEISHU_DEBUG       调试模式（可选）

示例:
  # 初始化配置文件
  feishu-cli config init

  # 使用环境变量
  export FEISHU_APP_ID="cli_xxx"
  export FEISHU_APP_SECRET="xxx"`,
}

func init() {
	rootCmd.AddCommand(configCmd)
}
