package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// resolveBaseToken 从 --base-token 读取多维表格 token
func resolveBaseToken(cmd *cobra.Command) (string, error) {
	token, _ := cmd.Flags().GetString("base-token")
	if token == "" {
		return "", fmt.Errorf("--base-token 必填")
	}
	return token, nil
}

// addBaseTokenFlag 给命令添加 --base-token flag 并标记为必填
func addBaseTokenFlag(cmd *cobra.Command) {
	cmd.Flags().String("base-token", "", "多维表格 base_token（必填）")
	mustMarkFlagRequired(cmd, "base-token")
}

// bitableCreateCmd 创建多维表格
var bitableCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建多维表格",
	Long: `创建一个新的多维表格（base）。

必填:
  --name    多维表格名称

可选:
  --folder-token  目标文件夹 token（默认根目录）
  --time-zone     时区（如 Asia/Shanghai）

以 Bot 身份创建时，自动给当前 CLI 登录用户授予 full_access（输出 permission_grant）。

示例:
  feishu-cli bitable create --name "项目管理"
  feishu-cli bitable create --name "销售数据" --folder-token fldxxx --time-zone Asia/Shanghai`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}

		name, _ := cmd.Flags().GetString("name")
		folderToken, _ := cmd.Flags().GetString("folder-token")
		timeZone, _ := cmd.Flags().GetString("time-zone")
		output, _ := cmd.Flags().GetString("output")

		if name == "" {
			return fmt.Errorf("--name 必填")
		}

		body := map[string]any{"name": name}
		if folderToken != "" {
			body["folder_token"] = folderToken
		}
		if timeZone != "" {
			body["time_zone"] = timeZone
		}

		data, err := client.BaseV3Call("POST", client.BaseV3Path("bases"), nil, body, token)
		if err != nil {
			return err
		}

		// --as bot（或 auto 未登录）创建时，自动给当前 CLI 登录用户授予 full_access
		grant := autoGrantCurrentUser(token, extractBaseTokenFromResponse(data), client.ResourceTypeBitable)
		if data != nil {
			withPermissionGrant(data, grant)
		}

		if output == "json" {
			return printJSON(data)
		}

		fmt.Printf("多维表格创建成功!\n")
		if m, ok := data["base"].(map[string]any); ok {
			if t, _ := m["base_token"].(string); t != "" {
				fmt.Printf("  base_token: %s\n", t)
			}
			if n, _ := m["name"].(string); n != "" {
				fmt.Printf("  name:       %s\n", n)
			}
			if u, _ := m["url"].(string); u != "" {
				fmt.Printf("  URL:        %s\n", u)
			}
		}
		printPermissionGrantText(os.Stdout, grant)
		return nil
	},
}

// bitableGetCmd 获取多维表格信息
var bitableGetCmd = &cobra.Command{
	Use:   "get",
	Short: "获取多维表格信息",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		baseToken, err := resolveBaseToken(cmd)
		if err != nil {
			return err
		}
		data, err := client.BaseV3Call("GET", client.BaseV3Path("bases", baseToken), nil, nil, token)
		if err != nil {
			return err
		}
		return printJSON(data)
	},
}

// bitableCopyCmd 复制多维表格
var bitableCopyCmd = &cobra.Command{
	Use:   "copy",
	Short: "复制多维表格",
	Long: `复制一个已有的多维表格。

必填:
  --base-token  源多维表格 token
  --name        新表格名称

可选:
  --folder-token    目标文件夹
  --without-content bool  只复制结构（不含数据）`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		baseToken, err := resolveBaseToken(cmd)
		if err != nil {
			return err
		}
		name, _ := cmd.Flags().GetString("name")
		folderToken, _ := cmd.Flags().GetString("folder-token")
		withoutContent, _ := cmd.Flags().GetBool("without-content")
		output, _ := cmd.Flags().GetString("output")

		if name == "" {
			return fmt.Errorf("--name 必填")
		}

		body := map[string]any{"name": name, "without_content": withoutContent}
		if folderToken != "" {
			body["folder_token"] = folderToken
		}

		data, err := client.BaseV3Call("POST", client.BaseV3Path("bases", baseToken, "copy"), nil, body, token)
		if err != nil {
			return err
		}
		// --as bot（或 auto 未登录）复制时，自动给当前 CLI 登录用户授予副本 full_access
		grant := autoGrantCurrentUser(token, extractBaseTokenFromResponse(data), client.ResourceTypeBitable)
		if data != nil {
			withPermissionGrant(data, grant)
		}
		if output == "json" {
			return printJSON(data)
		}
		fmt.Printf("多维表格复制成功!\n")
		if m, ok := data["base"].(map[string]any); ok {
			if t, _ := m["base_token"].(string); t != "" {
				fmt.Printf("  new base_token: %s\n", t)
			}
		}
		printPermissionGrantText(os.Stdout, grant)
		return nil
	},
}

func init() {
	bitableCmd.AddCommand(bitableCreateCmd)
	bitableCreateCmd.Flags().String("name", "", "多维表格名称（必填）")
	bitableCreateCmd.Flags().String("folder-token", "", "目标文件夹 token")
	bitableCreateCmd.Flags().String("time-zone", "", "时区（如 Asia/Shanghai）")
	bitableCreateCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	bitableCreateCmd.Flags().String("user-access-token", "", "User Access Token")
	mustMarkFlagRequired(bitableCreateCmd, "name")

	bitableCmd.AddCommand(bitableGetCmd)
	addBaseTokenFlag(bitableGetCmd)
	bitableGetCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	bitableGetCmd.Flags().String("user-access-token", "", "User Access Token")

	bitableCmd.AddCommand(bitableCopyCmd)
	addBaseTokenFlag(bitableCopyCmd)
	bitableCopyCmd.Flags().String("name", "", "新表格名称（必填）")
	bitableCopyCmd.Flags().String("folder-token", "", "目标文件夹 token")
	bitableCopyCmd.Flags().Bool("without-content", false, "只复制结构")
	bitableCopyCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	bitableCopyCmd.Flags().String("user-access-token", "", "User Access Token")
	mustMarkFlagRequired(bitableCopyCmd, "name")
}

// extractBaseTokenFromResponse 从 base/v3 创建/复制响应中取出新表的 token。
// 兼容 base 对象直接位于 data 顶层与嵌套在 data.base 两种形状，字段名兼容 base_token / app_token。
func extractBaseTokenFromResponse(data map[string]any) string {
	candidates := []map[string]any{data}
	if nested, ok := data["base"].(map[string]any); ok {
		candidates = append(candidates, nested)
	}
	for _, m := range candidates {
		for _, key := range []string{"base_token", "app_token"} {
			if t, _ := m[key].(string); strings.TrimSpace(t) != "" {
				return strings.TrimSpace(t)
			}
		}
	}
	return ""
}
