package cmd

import (
	"fmt"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var docMediaDownloadCmd = &cobra.Command{
	Use:   "media-download <token>",
	Short: "下载文档中的素材（图片/文件/画板缩略图）",
	Long: `下载文档中嵌入的图��、文件或画板缩略图。

参数:
  token       素材文件 token 或画板 ID
  --type      素材类型（media/whiteboard，默认 media）
  --output    输出文件路径（默认使用 token 作为文件名）
  --timeout   下载超时时间（默认 5m，大文件可设置更长如 30m、1h）

示例:
  # 下载图片素材
  feishu-cli doc media-download boxcnXXX -o image.png

  # 下载画板缩略图
  feishu-cli doc media-download XXX --type whiteboard -o board.png

  # 大文件下载，设置 30 分钟超时
  feishu-cli doc media-download boxcnXXX -o large.bin --timeout 30m`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := args[0]
		mediaType, _ := cmd.Flags().GetString("type")
		output, _ := cmd.Flags().GetString("output")
		timeoutStr, _ := cmd.Flags().GetString("timeout")
		userAccessToken := resolveOptionalUserToken(cmd)

		if output == "" {
			output = safeOutputPath(token, "")
		}

		if err := validateOutputPath(output, ""); err != nil {
			return fmt.Errorf("输出路径不安全: %w", err)
		}

		var timeout time.Duration
		if timeoutStr != "" {
			var err error
			timeout, err = time.ParseDuration(timeoutStr)
			if err != nil {
				return fmt.Errorf("无效的超时时间格式: %s（示例: 10m, 1h）", timeoutStr)
			}
		}

		switch mediaType {
		case "whiteboard":
			// 下载画板缩略图
			if err := client.GetBoardImage(token, output, userAccessToken); err != nil {
				return fmt.Errorf("下载画板缩略图失��: %w", err)
			}

		default:
			// 下载媒体文件（图片/附件）
			opts := client.DownloadMediaOptions{
				UserAccessToken: userAccessToken,
				Timeout:         timeout,
			}

			// 优先尝试临时 URL 下载
			url, err := client.GetMediaTempURL(token, opts)
			if err == nil {
				if dlErr := client.DownloadFromURL(url, output, timeout); dlErr == nil {
					fmt.Printf("已下载到 %s\n", output)
					return nil
				}
			}

			// 降级为直接下载
			if err := client.DownloadMedia(token, output, opts); err != nil {
				return fmt.Errorf("下载素材失败: %w", err)
			}
		}

		fmt.Printf("已下载到 %s\n", output)
		return nil
	},
}

func init() {
	docCmd.AddCommand(docMediaDownloadCmd)
	docMediaDownloadCmd.Flags().String("type", "media", "素材类型（media/whiteboard）")
	docMediaDownloadCmd.Flags().StringP("output", "o", "", "输出文件路径")
	docMediaDownloadCmd.Flags().String("user-access-token", "", "User Access Token（可选）")
	docMediaDownloadCmd.Flags().String("timeout", "", "下载超时时间（默认 5m，示例: 10m, 30m, 1h）")
}
