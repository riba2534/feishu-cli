package client

import (
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// GetUserAccessToken 按优先级获取 user_access_token:
// 1. CLI flag --user-access-token
// 2. config.yaml user_access_token
// 3. 环境变量 FEISHU_USER_ACCESS_TOKEN
func GetUserAccessToken(cmd *cobra.Command) string {
	if token, _ := cmd.Flags().GetString("user-access-token"); token != "" {
		return token
	}
	if token := config.Get().UserAccessToken; token != "" {
		return token
	}
	return os.Getenv("FEISHU_USER_ACCESS_TOKEN")
}

// RequireUserAccessToken 获取 user_access_token，为空时返回错误
func RequireUserAccessToken(cmd *cobra.Command) (string, error) {
	token := GetUserAccessToken(cmd)
	if token == "" {
		return "", fmt.Errorf("缺少 User Access Token，请通过以下方式之一提供:\n" +
			"  1. 命令行参数: --user-access-token <token>\n" +
			"  2. 环境变量: export FEISHU_USER_ACCESS_TOKEN=<token>\n" +
			"  3. 配置文件: user_access_token: <token>")
	}
	return token, nil
}
