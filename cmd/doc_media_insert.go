package cmd

import (
	"fmt"
	"path/filepath"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var docMediaInsertCmd = &cobra.Command{
	Use:   "media-insert <document_id>",
	Short: "向文档中插入图片或文件",
	Long: `向文档末尾插入本地图片或文件。

图片流程：创建空块 → 上传文件 → 绑定到块（三步法）
文件流程：上传文件 → 创建带 token 的块（两步法）

参数:
  document_id  文档 ID（必填）
  --file       本地文件路径（必填）
  --type       插入类型（image/file，默认 image）
  --align      图片对齐方式（left/center/right，默认 center，仅图片）
  --caption    图片描述（仅图片）
  --output     输出格式（json/text，默认 text）

示例:
  # 插入图片（居中对齐）
  feishu-cli doc media-insert DOC_ID --file photo.png --type image --align center

  # 插入图片并添加描述
  feishu-cli doc media-insert DOC_ID --file logo.png --type image --caption "公司 Logo"

  # 插入文件
  feishu-cli doc media-insert DOC_ID --file report.pdf --type file`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		documentID := args[0]
		filePath, _ := cmd.Flags().GetString("file")
		insertType, _ := cmd.Flags().GetString("type")
		alignStr, _ := cmd.Flags().GetString("align")
		caption, _ := cmd.Flags().GetString("caption")
		output, _ := cmd.Flags().GetString("output")

		// 确定块类型和上传 parent_type
		var blockType int
		var parentType string
		switch insertType {
		case "file":
			blockType = client.BlockTypeFile
			parentType = "docx_file"
		default:
			blockType = client.BlockTypeImage
			parentType = "docx_image"
		}

		// 解析对齐方式（1=left, 2=center, 3=right）
		align := 2 // 默认居中
		switch alignStr {
		case "left":
			align = 1
		case "right":
			align = 3
		}

		// ===== 步骤 1：获取文档根块信息 =====
		rootBlock, err := client.GetBlock(documentID, documentID)
		if err != nil {
			return fmt.Errorf("步骤 1 失败 - 获取文档根块: %w", err)
		}

		insertIndex := 0
		if rootBlock.Children != nil {
			insertIndex = len(rootBlock.Children)
		}

		var newBlockID string
		var fileToken string

		if blockType == client.BlockTypeFile {
			// ===== 文件类型：三步法（创建空块 → 上传 → 绑定）=====
			// 飞书文档 FAQ：File Block 必须先创建空块，再上传文件到空块，最后用 replace_file 绑定

			// 步骤 2：创建空文件块（token 为空字符串）
			emptyToken := ""
			newBlock := &larkdocx.Block{
				BlockType: &blockType,
				File:      &larkdocx.File{Token: &emptyToken},
			}
			createdBlocks, _, createErr := client.CreateBlock(documentID, documentID, []*larkdocx.Block{newBlock}, insertIndex)
			if createErr != nil {
				return fmt.Errorf("步骤 2 失败 - 创建空文件块: %w", createErr)
			}
			if len(createdBlocks) == 0 {
				return fmt.Errorf("步骤 2 失败 - 未返回块信息")
			}

			// 创建 File Block 后，API 返回的是 View Block（block_type=33），
			// File Block ID 在 View Block 的 children 中
			viewBlock := createdBlocks[0]
			viewBlockID := client.StringVal(viewBlock.BlockId)
			fileBlockID := viewBlockID // 默认使用 view block ID
			if viewBlock.Children != nil && len(viewBlock.Children) > 0 {
				fileBlockID = viewBlock.Children[0]
			}
			newBlockID = fileBlockID

			// 步骤 3：上传文件到 Drive，使用 File Block ID 作为 parent_node
			extra := fmt.Sprintf(`{"drive_route_token":"%s"}`, documentID)
			fileName := filepath.Base(filePath)
			fileToken, _, err = client.UploadMediaWithExtra(filePath, parentType, fileBlockID, fileName, extra)
			if err != nil {
				rollbackErr := rollbackInsertedBlock(documentID, insertIndex)
				if rollbackErr != nil {
					return fmt.Errorf("步骤 3 失败 - 上传文件: %w（回滚失败: %v）", err, rollbackErr)
				}
				return fmt.Errorf("步骤 3 失败 - 上传文件: %w（已回滚空块）", err)
			}

			// 步骤 4：绑定文件 token 到文件块（使用 replace_file，非 replace_image）
			_, err = client.UpdateBlock(documentID, fileBlockID, map[string]any{
				"replace_file": map[string]any{
					"token": fileToken,
				},
			})
			if err != nil {
				rollbackErr := rollbackInsertedBlock(documentID, insertIndex)
				if rollbackErr != nil {
					return fmt.Errorf("步骤 4 失败 - 绑定文件: %w（回滚失败: %v）", err, rollbackErr)
				}
				return fmt.Errorf("步骤 4 失败 - 绑定文件: %w（已回滚空块）", err)
			}
		} else {
			// ===== 图片类型：创建空块 → 上传 → 绑定（三步法）=====

			// 步骤 2：创建空图片块
			newBlock := &larkdocx.Block{
				BlockType: &blockType,
				Image:     &larkdocx.Image{},
			}
			createdBlocks, _, createErr := client.CreateBlock(documentID, documentID, []*larkdocx.Block{newBlock}, insertIndex)
			if createErr != nil {
				return fmt.Errorf("步骤 2 失败 - 创建空块: %w", createErr)
			}
			if len(createdBlocks) == 0 {
				return fmt.Errorf("步骤 2 失败 - 未返回块信息")
			}
			newBlockID = client.StringVal(createdBlocks[0].BlockId)

			// 步骤 3：上传文件到 Drive
			extra := fmt.Sprintf(`{"drive_route_token":"%s"}`, documentID)
			fileName := filepath.Base(filePath)
			fileToken, _, err = client.UploadMediaWithExtra(filePath, parentType, newBlockID, fileName, extra)
			if err != nil {
				rollbackErr := rollbackInsertedBlock(documentID, insertIndex)
				if rollbackErr != nil {
					return fmt.Errorf("步骤 3 失败 - 上传文件: %w（回滚失败: %v）", err, rollbackErr)
				}
				return fmt.Errorf("步骤 3 失败 - 上传文件: %w（已回滚空块）", err)
			}

			// 步骤 4：绑定文件 token 到图片块
			replaceReq := map[string]any{
				"token": fileToken,
				"align": align,
			}
			if caption != "" {
				replaceReq["caption"] = map[string]string{"content": caption}
			}
			_, err = client.UpdateBlock(documentID, newBlockID, map[string]any{
				"replace_image": replaceReq,
			})
			if err != nil {
				rollbackErr := rollbackInsertedBlock(documentID, insertIndex)
				if rollbackErr != nil {
					return fmt.Errorf("步骤 4 失败 - 绑定图片: %w（回滚失败: %v）", err, rollbackErr)
				}
				return fmt.Errorf("步骤 4 失败 - 绑定图片: %w（已回滚空块）", err)
			}
		}

		// 输出结果
		result := map[string]string{
			"document_id": documentID,
			"block_id":    newBlockID,
			"file_token":  fileToken,
			"type":        insertType,
			"file":        filePath,
		}

		if output == "json" {
			return printJSON(result)
		} else {
			fmt.Printf("插入成功！\n")
			fmt.Printf("  文档 ID:   %s\n", documentID)
			fmt.Printf("  块 ID:     %s\n", newBlockID)
			fmt.Printf("  文件 Token: %s\n", fileToken)
			fmt.Printf("  类型:      %s\n", insertType)
			fmt.Printf("  文件:      %s\n", filePath)
		}

		return nil
	},
}

// rollbackInsertedBlock 回滚创建的空块
func rollbackInsertedBlock(documentID string, blockIndex int) error {
	_, err := client.DeleteBlocks(documentID, documentID, blockIndex, blockIndex+1)
	return err
}

func init() {
	docCmd.AddCommand(docMediaInsertCmd)
	docMediaInsertCmd.Flags().String("file", "", "本地文件路径（必填）")
	docMediaInsertCmd.Flags().String("type", "image", "插入类型（image/file）")
	docMediaInsertCmd.Flags().String("align", "center", "图片对齐方式（left/center/right，仅图片）")
	docMediaInsertCmd.Flags().String("caption", "", "图片描述（仅图片）")
	docMediaInsertCmd.Flags().StringP("output", "o", "", "输出格式（json/text）")
	docMediaInsertCmd.Flags().String("user-access-token", "", "User Access Token（可选）")
	mustMarkFlagRequired(docMediaInsertCmd, "file")
}
