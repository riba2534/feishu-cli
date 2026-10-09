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
	Long: `从云空间下载文件到本地（User/Bot 身份均为流式下载，不受 100MB 限制）。
如遇到飞书大文件限制，会自动使用 HTTP Range 分片下载并合并；分片失败有界重试并断点续传，
空闲 60 秒无数据视为超时；写盘为临时文件 + rename，失败不会破坏已存在的同名文件。

参数:
  file_token    文件的 Token

选项:
  -o, --output   输出文件路径（默认使用当前目录下的文件名）
  --timeout      整个下载的总时长上限（默认不限，只受空闲超时约束；如 30m、1h）

示例:
  # 下载文件到当前目录
  feishu-cli file download boxcnXXXXXXXXX

  # 下载文件到指定路径
  feishu-cli file download boxcnXXXXXXXXX -o /tmp/myfile.pdf

  # 大文件下载，限制总时长 30 分钟
  feishu-cli file download boxcnXXXXXXXXX -o large.zip --timeout 30m`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken := args[0]
		outputPath, _ := cmd.Flags().GetString("output")
		timeoutStr, _ := cmd.Flags().GetString("timeout")

		if outputPath == "" {
			outputPath = fileToken
		}
		// 输出路径在任何网络请求（含 token 刷新）之前校验，敏感目录直接拒绝
		if err := validateOutputPath(outputPath, ""); err != nil {
			return err
		}
		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

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
	downloadFileCmd.Flags().String("timeout", "", "整个下载的总时长上限（默认不限，示例: 10m, 30m, 1h）")
	downloadFileCmd.Flags().String("user-access-token", "", "User Access Token（可选，使用用户身份访问文件）")
}
