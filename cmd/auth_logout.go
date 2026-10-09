package cmd

import (
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "退出登录（吊销并清除本地 token）",
	Long: `吊销服务端 token 授权，并清除本地存储的 OAuth token 信息。

流程:
  1. 若本地存在 token，先调用飞书吊销端点使其在服务端失效
  2. 删除文件: ~/.feishu-cli/token.json

吊销失败（如网络异常）不会阻断本地清理，仅打印告警。
可用 --no-revoke 跳过服务端吊销，只删本地文件。

示例:
  feishu-cli auth logout
  feishu-cli auth logout --no-revoke`,
	RunE: func(cmd *cobra.Command, args []string) error {
		noRevoke, _ := cmd.Flags().GetBool("no-revoke")

		// 先尝试在线吊销（best-effort）：失败仅告警，不阻断本地清理
		if !noRevoke {
			revokeStoredTokenBestEffort()
		}

		if err := auth.DeleteToken(); err != nil {
			return err
		}
		if err := clearLoginRequestedScopeCache(); err != nil {
			return err
		}

		path, _ := auth.TokenPath()
		fmt.Printf("已清除本地授权信息 (%s)\n", path)
		return nil
	},
}

// revokeStoredTokenBestEffort 读取本地 token 并调用吊销端点。
// 任何一步失败都只打印告警到 stderr，不返回 error——logout 的本地清理必须继续。
func revokeStoredTokenBestEffort() {
	token, err := auth.LoadToken()
	if err != nil || token == nil {
		// 无本地 token（未登录或读取失败）：无可吊销，静默跳过
		return
	}
	cfg := config.Get()
	if cfg == nil || cfg.AppID == "" || cfg.AppSecret == "" {
		fmt.Fprintln(os.Stderr, "警告: 缺少 app_id/app_secret，跳过服务端吊销，仅清除本地 token")
		return
	}
	if err := auth.RevokeStoredToken(token, cfg.AppID, cfg.AppSecret, cfg.BaseURL); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 服务端吊销 token 失败（%v），继续清除本地 token\n", err)
		return
	}
	fmt.Fprintln(os.Stderr, "已在服务端吊销 token 授权")
}

func init() {
	authCmd.AddCommand(authLogoutCmd)
	authLogoutCmd.Flags().Bool("no-revoke", false, "跳过服务端吊销，只删除本地 token 文件")
}
