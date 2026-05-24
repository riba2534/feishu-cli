package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/profile"
	"github.com/spf13/cobra"
)

var configCreateAppCmd = &cobra.Command{
	Use:   "create-app",
	Short: "创建飞书应用（自动注册）",
	Long: `通过 Device Flow 自动注册飞书个人代理应用，无需手动到飞书开放平台操作。

流程:
  1. CLI 发起应用注册请求，获取授权链接
  2. 用户在浏览器中打开链接并扫码确认
  3. CLI 自动获取 App ID 和 App Secret 并保存到配置文件

创建成功后可直接使用:
  feishu-cli auth login

示例:
  # 创建新应用
  feishu-cli config create-app

  # 创建后自动写入配置文件
  feishu-cli config create-app --save

  # 指定 Lark 国际版
  feishu-cli config create-app --brand lark`,
	RunE: func(cmd *cobra.Command, args []string) error {
		brand, _ := cmd.Flags().GetString("brand")
		save, _ := cmd.Flags().GetBool("save")
		output, _ := cmd.Flags().GetString("output")

		baseURL := "https://open.feishu.cn"
		if brand == "lark" {
			baseURL = "https://open.larksuite.com"
		}

		// 步骤 1：发起应用注册
		fmt.Fprintln(os.Stderr, "正在发起应用注册...")
		regResp, err := auth.RequestAppRegistration(baseURL)
		if err != nil {
			return fmt.Errorf("应用注册失败: %w", err)
		}

		// 步骤 2：显示授权链接
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "请在浏览器中打开以下链接，扫码确认创建应用:")
		fmt.Fprintf(os.Stderr, "\n  %s\n\n", regResp.VerificationURIComplete)
		fmt.Fprintf(os.Stderr, "用户码: %s\n", regResp.UserCode)
		fmt.Fprintf(os.Stderr, "有效期: %d 秒\n\n", regResp.ExpiresIn)

		// 步骤 3：轮询等待用户确认
		fmt.Fprintln(os.Stderr, "等待扫码确认...")
		ctx := context.Background()
		result, err := auth.PollAppRegistration(ctx, baseURL, regResp.DeviceCode, regResp.Interval, regResp.ExpiresIn,
			func(elapsed, total int) {
				fmt.Fprintf(os.Stderr, "\r  等待中... %d/%d 秒", elapsed, total)
			})
		fmt.Fprintln(os.Stderr) // 换行
		if err != nil {
			return fmt.Errorf("应用注册失败: %w", err)
		}

		if result.ClientID == "" || result.ClientSecret == "" {
			return fmt.Errorf("应用注册成功但未获取到凭证")
		}

		// 步骤 4：输出结果
		if output == "json" {
			return printJSON(map[string]string{
				"app_id":     result.ClientID,
				"app_secret": result.ClientSecret,
				"brand":      brand,
			})
		}

		fmt.Fprintln(os.Stderr)
		fmt.Println("应用创建成功！")
		fmt.Printf("  App ID:     %s\n", result.ClientID)
		fmt.Printf("  App Secret: %s\n", auth.MaskToken(result.ClientSecret))

		// 步骤 5：保存到配置文件
		if save {
			if err := saveAppConfig(result.ClientID, result.ClientSecret, baseURL); err != nil {
				fmt.Fprintf(os.Stderr, "\n配置保存失败: %v\n", err)
				fmt.Fprintln(os.Stderr, "请手动配置:")
				fmt.Fprintf(os.Stderr, "  export FEISHU_APP_ID=%s\n", result.ClientID)
				fmt.Fprintf(os.Stderr, "  export FEISHU_APP_SECRET=%s\n", result.ClientSecret)
			} else {
				fmt.Println("\n已保存到配置文件")
			}
		} else {
			fmt.Println("\n使用以下命令配置环境变量:")
			fmt.Printf("  export FEISHU_APP_ID=%s\n", result.ClientID)
			fmt.Printf("  export FEISHU_APP_SECRET=%s\n", result.ClientSecret)
			fmt.Println("\n或加 --save 自动写入配置文件:")
			fmt.Println("  feishu-cli config create-app --save")
		}

		return nil
	},
}

// saveAppConfig 将应用凭证保存到配置文件
//
// 路径解析走 profile 系统：
//   - 启用多 profile 时写入当前激活 profile 的 ~/.feishu-cli/profiles/<active>/config.yaml
//   - 未启用 profile 时回落到旧布局 ~/.feishu-cli/config.yaml
func saveAppConfig(appID, appSecret, baseURL string) error {
	configDir, err := profile.ActiveDir()
	if err != nil {
		return fmt.Errorf("获取配置目录失败: %w", err)
	}
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return err
	}

	configFile := filepath.Join(configDir, "config.yaml")

	// 读取现有配置（如果存在）
	var existingContent string
	if data, err := os.ReadFile(configFile); err == nil {
		existingContent = string(data)
	}

	// 如果文件存在，更新 app_id 和 app_secret
	if existingContent != "" {
		lines := strings.Split(existingContent, "\n")
		var newLines []string
		appIDSet, appSecretSet := false, false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "app_id:") {
				newLines = append(newLines, fmt.Sprintf("app_id: \"%s\"", appID))
				appIDSet = true
			} else if strings.HasPrefix(trimmed, "app_secret:") {
				newLines = append(newLines, fmt.Sprintf("app_secret: \"%s\"", appSecret))
				appSecretSet = true
			} else {
				newLines = append(newLines, line)
			}
		}
		if !appIDSet {
			newLines = append([]string{fmt.Sprintf("app_id: \"%s\"", appID)}, newLines...)
		}
		if !appSecretSet {
			newLines = append([]string{fmt.Sprintf("app_secret: \"%s\"", appSecret)}, newLines...)
		}
		return os.WriteFile(configFile, []byte(strings.Join(newLines, "\n")), 0600)
	}

	// 新建配置文件
	content := fmt.Sprintf(`# 飞书 CLI 配置文件（由 feishu-cli config create-app 自动生成）
app_id: "%s"
app_secret: "%s"
base_url: "%s"
owner_email: ""
transfer_ownership: false
debug: false
`, appID, appSecret, baseURL)

	return os.WriteFile(configFile, []byte(content), 0600)
}

func init() {
	configCmd.AddCommand(configCreateAppCmd)
	configCreateAppCmd.Flags().String("brand", "feishu", "平台（feishu/lark）")
	configCreateAppCmd.Flags().Bool("save", false, "自动保存到配置文件")
	configCreateAppCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
