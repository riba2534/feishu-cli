package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var getQuotaCmd = &cobra.Command{
	Use:   "quota",
	Short: "查询云空间容量",
	Long: `查询当前用户的云空间容量信息（GET /open-apis/drive/v2/quota_details/{user_id}）。

返回信息:
  - 用户配额上限与已用容量（未设置上限时显示"不限"）
  - 各业务用量：ccm（云文档）、im（聊天文件）、vc（视频会议）、mail（邮箱）、all（合计）
  - 租户配额是否超限、部门配额（若有）

身份与权限:
  - 只支持 User Access Token（自动使用当前登录用户的 user_id）
  - drive:quota_detail:read_one

示例:
  # 查询云空间容量
  feishu-cli file quota

  # JSON 格式输出
  feishu-cli file quota --output json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")

		token, err := requireUserToken(cmd, "file quota")
		if err != nil {
			return err
		}
		userID, err := resolveCurrentAuthedUserID(cmd, "user_id")
		if err != nil {
			return fmt.Errorf("获取当前用户 user_id 失败: %w", err)
		}

		quota, err := client.GetDriveQuota(userID, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(quota)
		}
		fmt.Printf("云空间容量信息\n")
		if quota.Unlimited {
			fmt.Printf("  总容量:   不限\n")
		} else {
			fmt.Printf("  总容量:   %s\n", formatBytes(quota.Total))
		}
		fmt.Printf("  已用容量: %s\n", formatBytes(quota.Used))
		if !quota.Unlimited {
			fmt.Printf("  剩余容量: %s\n", formatBytes(quota.Total-quota.Used))
			percentage := float64(quota.Used) / float64(quota.Total) * 100
			fmt.Printf("  使用率:   %.2f%%\n", percentage)
		}
		for _, b := range quota.BizInfos {
			fmt.Printf("  - %-5s %s\n", b.Name, formatBytes(b.Used))
		}
		if quota.IsTenantQuotaExceeded {
			fmt.Printf("  ⚠ 租户容量已超限\n")
		}
		return nil
	},
}

// formatBytes 将字节数格式化为人类可读的形式
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.2f TB", float64(bytes)/TB)
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func init() {
	fileCmd.AddCommand(getQuotaCmd)
	getQuotaCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	getQuotaCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
