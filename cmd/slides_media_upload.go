package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// maxSlidesMediaUploadSize slides 的 medias/upload_all 单分片上限（20 MB）
// 多分片 upload_prepare 不接受 parent_type=slide_file，所以这里硬限 20 MB
const maxSlidesMediaUploadSize = 20 * 1024 * 1024

var slidesMediaUploadCmd = &cobra.Command{
	Use:   "media-upload",
	Short: "上传本地图片到 Slides 演示文稿，返回 file_token",
	Long: `把本地图片上传到指定的 Slides 演示文稿，返回 file_token。
返回的 file_token 可作为 slide XML 中 <img src="..."> 的值。

参数:
  --file                本地图片路径（必填，≤ 20 MB）
  --presentation-token  目标演示文稿的 xml_presentation_id（必填）
  --output, -o          输出格式，可选 json

注意:
  - slides 后端只接受 parent_type=slide_file + parent_node=xml_presentation_id
  - 多分片上传不支持 slide_file，所以单文件上限 20 MB
  - 权限: docs:document.media:upload

示例:
  feishu-cli slides media-upload --file ./cover.png --presentation-token <xml_presentation_id>`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		filePath, _ := cmd.Flags().GetString("file")
		presentationToken, _ := cmd.Flags().GetString("presentation-token")
		output, _ := cmd.Flags().GetString("output")
		userAccessToken := resolveOptionalUserToken(cmd)

		if filePath == "" {
			return fmt.Errorf("--file 不能为空")
		}
		if presentationToken == "" {
			return fmt.Errorf("--presentation-token 不能为空")
		}

		stat, err := os.Stat(filePath)
		if err != nil {
			return fmt.Errorf("读取文件失败: %w", err)
		}
		if !stat.Mode().IsRegular() {
			return fmt.Errorf("--file 必须是普通文件: %s", filePath)
		}
		if stat.Size() > maxSlidesMediaUploadSize {
			return fmt.Errorf("文件 %s 大小 %d 字节超过 slides 上传限制（20 MB）",
				filepath.Base(filePath), stat.Size())
		}

		fileName := filepath.Base(filePath)
		fileToken, err := client.UploadSlidesMedia(filePath, fileName, presentationToken, userAccessToken)
		if err != nil {
			return fmt.Errorf("上传 slides 图片失败: %w", err)
		}

		result := map[string]any{
			"file_token":      fileToken,
			"file_name":       fileName,
			"size":            stat.Size(),
			"presentation_id": presentationToken,
		}

		if output == "json" {
			return printJSON(result)
		}

		fmt.Printf("图片上传成功：\n")
		fmt.Printf("  file_token:      %s\n", fileToken)
		fmt.Printf("  file_name:       %s\n", fileName)
		fmt.Printf("  size:            %d bytes\n", stat.Size())
		fmt.Printf("  presentation_id: %s\n", presentationToken)
		fmt.Printf("\n提示: 在 slide XML 中可用作 <img src=\"%s\"/>\n", fileToken)
		return nil
	},
}

func init() {
	slidesCmd.AddCommand(slidesMediaUploadCmd)
	slidesMediaUploadCmd.Flags().String("file", "", "本地图片路径（必填，≤ 20 MB）")
	slidesMediaUploadCmd.Flags().String("presentation-token", "", "目标演示文稿的 xml_presentation_id（必填）")
	slidesMediaUploadCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
	slidesMediaUploadCmd.Flags().String("user-access-token", "", "User Access Token")
}
