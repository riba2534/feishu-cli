package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var uploadMediaCmd = &cobra.Command{
	Use:   "upload <file>",
	Short: "上传素材文件",
	Long: `上传素材文件（图片、视频等）到飞书云空间。

参数:
  --parent-type   父节点类型（默认: doc_image）
  --parent-node   父节点 token，即文档 ID（必填）
  --name          文件名（默认使用原文件名）
  --output, -o    输出格式（json）

父节点类型:
  doc_image      文档图片
  doc_file       文档文件

示例:
  # 上传图片到文档
  feishu-cli media upload image.png --parent-type doc_image --parent-node DOC_ID

  # 上传文件
  feishu-cli media upload document.pdf --parent-type doc_file --parent-node DOC_ID

  # 指定文件名
  feishu-cli media upload photo.jpg --parent-node DOC_ID --name "封面图.jpg"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		filePath := args[0]
		parentType, _ := cmd.Flags().GetString("parent-type")
		parentNode, _ := cmd.Flags().GetString("parent-node")
		fileName, _ := cmd.Flags().GetString("name")

		if parentType == "" {
			parentType = "doc_image"
		}

		if parentNode == "" {
			return fmt.Errorf("必须指定 --parent-node（文档ID）")
		}

		if fileName == "" {
			fileName = filepath.Base(filePath)
		}

		token, err := client.UploadMedia(filePath, parentType, parentNode, fileName)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			data, _ := json.MarshalIndent(map[string]string{
				"file_token": token,
			}, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("上传成功！\n")
			fmt.Printf("  文件 Token: %s\n", token)
		}

		return nil
	},
}

func init() {
	mediaCmd.AddCommand(uploadMediaCmd)
	uploadMediaCmd.Flags().String("parent-type", "doc_image", "父节点类型（doc_image/doc_file）")
	uploadMediaCmd.Flags().String("parent-node", "", "父节点 token（文档ID）")
	uploadMediaCmd.Flags().String("name", "", "文件名（默认使用原文件名）")
	uploadMediaCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	_ = uploadMediaCmd.MarkFlagRequired("parent-node")
}
