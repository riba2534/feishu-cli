package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/token"
	"github.com/spf13/cobra"
)

var (
	loginPort  int
	loginHost  string
	loginScope string
)

// authLoginCmd 登录命令
var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "登录飞书账号，获取 User Access Token",
	Long: `启动 OAuth 2.0 授权流程，获取 User Access Token。

流程说明:
1. 启动本地 HTTP 服务器接收授权码
2. 打开浏览器访问飞书授权页面
3. 用户授权后，飞书重定向到本地服务器
4. 自动换取并保存 User Access Token

Token 将保存到 ~/.lark_user_token 文件，后续命令可自动使用。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. 检查 App ID 和 App Secret 配置
		if err := config.Validate(); err != nil {
			return err
		}

		cfg := config.Get()

		// 2. 确定回调地址
		redirectURI := fmt.Sprintf("http://%s:%d/callback", loginHost, loginPort)

		// 3. 创建 OAuth 客户端
		oauthClient, err := client.NewOAuthClient(redirectURI)
		if err != nil {
			return err
		}

		// 4. 创建 channel 用于接收授权码
		codeChan := make(chan string, 1)
		errChan := make(chan error, 1)

		// 5. 启动 HTTP 服务器
		mux := http.NewServeMux()
		server := &http.Server{
			Addr:    fmt.Sprintf("%s:%d", loginHost, loginPort),
			Handler: mux,
		}

		// 处理回调
		mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
			code := r.URL.Query().Get("code")
			state := r.URL.Query().Get("state")
			errorMsg := r.URL.Query().Get("error")

			if errorMsg != "" {
				errChan <- fmt.Errorf("授权失败: %s", errorMsg)
				http.Error(w, "授权失败，请返回终端查看错误信息", http.StatusBadRequest)
				return
			}

			if code == "" {
				errChan <- fmt.Errorf("未收到授权码")
				http.Error(w, "未收到授权码", http.StatusBadRequest)
				return
			}

			// 简单的 state 验证（可选）
			_ = state

			codeChan <- code

			// 返回成功页面
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>授权成功</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #f5f5f5; }
        .container { text-align: center; background: white; padding: 40px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        .success-icon { font-size: 64px; color: #52c41a; margin-bottom: 20px; }
        h1 { color: #333; margin-bottom: 10px; }
        p { color: #666; }
    </style>
</head>
<body>
    <div class="container">
        <div class="success-icon">✓</div>
        <h1>授权成功</h1>
        <p>请返回终端查看 token 信息</p>
        <p>您可以关闭此页面</p>
    </div>
</body>
</html>`)
		})

		// 6. 启动服务器
		listener, err := net.Listen("tcp", server.Addr)
		if err != nil {
			return fmt.Errorf("启动本地服务器失败: %w", err)
		}
		actualPort := listener.Addr().(*net.TCPAddr).Port
		actualAddr := fmt.Sprintf("http://%s:%d/callback", loginHost, actualPort)

		// 更新 OAuth 客户端的 redirect URI
		oauthClient.RedirectURI = actualAddr

		go func() {
			if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
				errChan <- fmt.Errorf("服务器错误: %w", err)
			}
		}()

		// 7. 生成授权 URL 并提示用户
		authorizeURL := oauthClient.GetAuthorizeURL("random-state", loginScope)

		fmt.Println("=")
		fmt.Println("飞书用户授权")
		fmt.Println("=")
		fmt.Println()
		fmt.Printf("应用 ID: %s\n", cfg.AppID)
		fmt.Printf("回调地址: %s\n", actualAddr)
		fmt.Println()
		fmt.Println("请在浏览器中访问以下链接进行授权:")
		fmt.Println()
		fmt.Println(authorizeURL)
		fmt.Println()
		fmt.Println("等待授权...")
		fmt.Println()

		// 8. 等待授权码或错误
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// 处理信号
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		var authCode string
		select {
		case authCode = <-codeChan:
			// 收到授权码
		case err := <-errChan:
			return err
		case <-sigChan:
			fmt.Println("\n已取消授权")
			server.Shutdown(ctx)
			return nil
		case <-time.After(5 * time.Minute):
			server.Shutdown(ctx)
			return fmt.Errorf("授权超时，请重新运行命令")
		}

		// 9. 关闭服务器
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		server.Shutdown(shutdownCtx)

		fmt.Println("已收到授权码，正在换取 token...")

		// 10. 用授权码换取 token
		userToken, err := oauthClient.ExchangeCodeForToken(authCode)
		if err != nil {
			return fmt.Errorf("换取 token 失败: %w", err)
		}

		// 11. 保存 token
		if err := token.SaveToken(userToken); err != nil {
			return err
		}

		// 12. 显示成功信息
		fmt.Println()
		fmt.Println("=")
		fmt.Println("授权成功！")
		fmt.Println("=")
		fmt.Println()
		fmt.Printf("Access Token: %s...%s\n",
			userToken.AccessToken[:min(10, len(userToken.AccessToken))],
			userToken.AccessToken[max(0, len(userToken.AccessToken)-10):])
		fmt.Printf("Refresh Token: %s...%s\n",
			userToken.RefreshToken[:min(10, len(userToken.RefreshToken))],
			userToken.RefreshToken[max(0, len(userToken.RefreshToken)-10):])
		fmt.Printf("过期时间: %s\n", userToken.FormatExpiry())
		fmt.Println()
		fmt.Printf("Token 已保存到: ~/.lark_user_token\n")
		fmt.Println()
		fmt.Println("现在您可以使用需要 User Access Token 的命令，如:")
		fmt.Println("  feishu-cli search messages \"关键词\"")
		fmt.Println()

		return nil
	},
}

func init() {
	authCmd.AddCommand(authLoginCmd)
	authLoginCmd.Flags().IntVarP(&loginPort, "port", "p", 8080, "本地服务器端口（0 表示随机端口）")
	authLoginCmd.Flags().StringVarP(&loginHost, "host", "H", "127.0.0.1", "本地服务器主机地址")
	authLoginCmd.Flags().StringVarP(&loginScope, "scope", "s", "", "OAuth scope（多个 scope 用空格分隔，如：\"contact:user.read chat:message\"）")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
