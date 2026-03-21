package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "获取配置项的值",
	Long: `获取配置项的当前值（合并环境变量、配置文件和默认值后的最终值）。

支持的配置项:
  app_id               应用 ID
  app_secret           应用密钥（出于安全仅显示前 4 位）
  base_url             API 地址
  owner_email          文档所有者邮箱
  transfer_ownership   创建文档后是否转移所有权
  debug                调试模式

示例:
  feishu-cli config get owner_email
  feishu-cli config get transfer_ownership`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]

		if !viper.IsSet(key) {
			return fmt.Errorf("未知的配置项: %s", key)
		}

		val := viper.Get(key)

		// 敏感字段脱敏
		if key == "app_secret" {
			s := fmt.Sprintf("%v", val)
			if len(s) > 4 {
				s = s[:4] + "****"
			}
			val = s
		}

		fmt.Println(val)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configGetCmd)
}
