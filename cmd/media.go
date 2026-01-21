package cmd

import (
	"github.com/spf13/cobra"
)

var mediaCmd = &cobra.Command{
	Use:   "media",
	Short: "素材操作命令",
	Long: `素材操作命令，包括上传和下载图片、文件等素材。

子命令:
  upload    上传素材文件
  download  下载素材文件

示例:
  # 上传图片
  feishu-cli media upload image.png --parent-type doc_image --parent-node <document_id>

  # 下载素材
  feishu-cli media download <file_token> --output image.png`,
}

func init() {
	rootCmd.AddCommand(mediaCmd)
}
