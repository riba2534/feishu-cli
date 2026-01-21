package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var downloadMediaCmd = &cobra.Command{
	Use:   "download <file_token>",
	Short: "下载素材文件",
	Long: `从飞书云空间下载素材文件。

参数:
  --output, -o    输出文件路径（默认使用 file_token 作为文件名）

示例:
  # 下载到当前目录
  feishu-cli media download ABC123token456

  # 指定输出路径
  feishu-cli media download ABC123token456 --output ./images/photo.png`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken := args[0]
		output, _ := cmd.Flags().GetString("output")

		if output == "" {
			output = safeOutputPath(fileToken, "")
		}

		// 路径安全检查：防止路径遍历攻击
		if err := validateOutputPath(output, ""); err != nil {
			return fmt.Errorf("输出路径不安全: %w", err)
		}

		// Try to get temp URL first
		url, err := client.GetMediaTempURL(fileToken)
		if err == nil {
			if err := client.DownloadFromURL(url, output); err == nil {
				fmt.Printf("已下载到 %s\n", output)
				return nil
			}
		}

		// Fallback to direct download
		if err := client.DownloadMedia(fileToken, output); err != nil {
			return err
		}

		fmt.Printf("已下载到 %s\n", output)
		return nil
	},
}

func init() {
	mediaCmd.AddCommand(downloadMediaCmd)
	downloadMediaCmd.Flags().StringP("output", "o", "", "输出文件路径")
}
