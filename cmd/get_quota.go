package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var getQuotaCmd = &cobra.Command{
	Use:   "quota",
	Short: "查询云空间容量",
	Long: `查询当前用户的云空间容量信息。

返回信息:
  - 总容量
  - 已用容量
  - 剩余容量
  - 使用百分比

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

		quota, err := client.GetDriveQuota()
		if err != nil {
			return err
		}

		if output == "json" {
			data, _ := json.MarshalIndent(quota, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("云空间容量信息\n")
			fmt.Printf("  总容量:   %s\n", formatBytes(quota.Total))
			fmt.Printf("  已用容量: %s\n", formatBytes(quota.Used))
			fmt.Printf("  剩余容量: %s\n", formatBytes(quota.Total-quota.Used))
			if quota.Total > 0 {
				percentage := float64(quota.Used) / float64(quota.Total) * 100
				fmt.Printf("  使用率:   %.2f%%\n", percentage)
			}
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
	getQuotaCmd.Flags().StringP("output", "o", "", "输出格式（json）")
}
