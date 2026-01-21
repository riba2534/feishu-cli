package cmd

import (
	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "搜索相关命令",
	Long: `搜索相关命令，支持搜索消息和应用。

子命令:
  messages  搜索消息
  apps      搜索应用

注意:
  搜索功能需要 User Access Token（用户授权令牌）。
  请通过 --user-access-token 参数或 FEISHU_USER_ACCESS_TOKEN 环境变量提供。

获取 User Access Token:
  1. 在飞书开放平台创建应用并配置重定向 URL
  2. 引导用户访问授权页面进行登录授权
  3. 使用授权码换取 User Access Token
  详情参考: https://open.feishu.cn/document/ukTMukTMukTM/ukDNz4SO0MjL5QzM/auth-v3/auth/authorize-user-access-token

示例:
  # 搜索消息
  feishu-cli search messages "关键词" --user-access-token <token>

  # 搜索消息（使用环境变量）
  export FEISHU_USER_ACCESS_TOKEN="u-xxx"
  feishu-cli search messages "关键词"

  # 搜索应用
  feishu-cli search apps "关键词" --user-access-token <token>`,
}

func init() {
	rootCmd.AddCommand(searchCmd)
}
