package cmd

import (
	"fmt"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var msgResourceDownloadCmd = &cobra.Command{
	Use:   "resource-download <message_id> <file_key>",
	Short: "下载消息中的资源文件（图片/文件）",
	Long: `下载消息中的图片或文件资源。

参数:
  message_id  消息 ID（om_xxx 格式）
  file_key    文件 key（img_xxx 或 file_xxx 格式）

选项:
  --type       资源类型（image 或 file，必填）
  -o, --output 输出文件路径（默认使用 file_key）
  --timeout    下载超时时间（默认 5m，大文件可设置更长如 30m、1h）

示例:
  # 下载图片
  feishu-cli msg resource-download om_xxx img_xxx --type image -o photo.png

  # 下载文件
  feishu-cli msg resource-download om_xxx file_xxx --type file -o document.pdf

  # 大文件下载，设置 30 分钟超时
  feishu-cli msg resource-download om_xxx file_xxx --type file -o large.zip --timeout 30m`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		messageID := args[0]
		fileKey := args[1]
		resourceType, _ := cmd.Flags().GetString("type")
		outputPath, _ := cmd.Flags().GetString("output")
		timeoutStr, _ := cmd.Flags().GetString("timeout")
		userToken := resolveOptionalUserToken(cmd)

		if outputPath == "" {
			outputPath = fileKey
		}

		var timeout time.Duration
		if timeoutStr != "" {
			var err error
			timeout, err = time.ParseDuration(timeoutStr)
			if err != nil {
				return fmt.Errorf("无效的超时时间格式: %s（示例: 10m, 1h）", timeoutStr)
			}
		}

		if err := client.DownloadMessageResource(messageID, fileKey, resourceType, outputPath, userToken, timeout); err != nil {
			return err
		}

		fmt.Printf("资源下载成功！\n")
		fmt.Printf("  消息 ID:   %s\n", messageID)
		fmt.Printf("  文件 Key:  %s\n", fileKey)
		fmt.Printf("  资源类型:  %s\n", resourceType)
		fmt.Printf("  保存路径:  %s\n", outputPath)

		return nil
	},
}

func init() {
	msgCmd.AddCommand(msgResourceDownloadCmd)
	msgResourceDownloadCmd.Flags().String("type", "", "资源类型（image 或 file）")
	msgResourceDownloadCmd.Flags().StringP("output", "o", "", "输出文件路径（默认使用 file_key）")
	msgResourceDownloadCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
	msgResourceDownloadCmd.Flags().String("timeout", "", "下载超时时间（默认 5m，示例: 10m, 30m, 1h）")
	mustMarkFlagRequired(msgResourceDownloadCmd, "type")
}
