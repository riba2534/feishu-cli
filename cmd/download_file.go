package cmd

import (
	"fmt"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var downloadFileCmd = &cobra.Command{
	Use:   "download <file_token>",
	Short: "下载云空间文件",
	Long: `从云空间下载文件到本地。

参数:
  file_token    文件的 Token

选项:
  -o, --output   输出文件路径（默认使用当前目录下的文件名）
  --timeout      下载超时时间（默认 5m，大文件可设置更长如 30m、1h）

示例:
  # 下载文件到当前目录
  feishu-cli file download boxcnXXXXXXXXX

  # 下载文件到指定路径
  feishu-cli file download boxcnXXXXXXXXX -o /tmp/myfile.pdf

  # 大文件下载，设置 30 分钟超时
  feishu-cli file download boxcnXXXXXXXXX -o large.zip --timeout 30m`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken := args[0]
		outputPath, _ := cmd.Flags().GetString("output")
		timeoutStr, _ := cmd.Flags().GetString("timeout")
		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		if outputPath == "" {
			outputPath = fileToken
		}

		var timeout time.Duration
		if timeoutStr != "" {
			var err error
			timeout, err = time.ParseDuration(timeoutStr)
			if err != nil {
				return fmt.Errorf("无效的超时时间格式: %s（示例: 10m, 1h）", timeoutStr)
			}
		}

		if err := client.DownloadFileWithToken(fileToken, outputPath, userAccessToken, timeout); err != nil {
			return err
		}

		fmt.Printf("文件下载成功！\n")
		fmt.Printf("  文件 Token: %s\n", fileToken)
		fmt.Printf("  保存路径:   %s\n", outputPath)

		return nil
	},
}

func init() {
	fileCmd.AddCommand(downloadFileCmd)
	downloadFileCmd.Flags().StringP("output", "o", "", "输出文件路径")
	downloadFileCmd.Flags().String("timeout", "", "下载超时时间（默认 5m，示例: 10m, 30m, 1h）")
	downloadFileCmd.Flags().String("user-access-token", "", "User Access Token（可选，使用用户身份访问文件）")
}
