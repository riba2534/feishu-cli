package cmd

import (
	"bytes"
	"fmt"
	"image"
	"path/filepath"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/safefile"
	"github.com/spf13/cobra"
)

var docMediaInsertCmd = &cobra.Command{
	Use:   "media-insert <document_id|url>",
	Short: "向文档中插入图片或文件",
	Long: `向文档末尾插入本地图片或文件。

图片流程：创建空块 → 上传文件 → 绑定到块（三步法）
文件流程：上传文件 → 创建带 token 的块（两步法）

参数:
  document_id  文档 ID 或 URL（必填；支持 /docx/ 与 /wiki/ URL，wiki 自动解析为底层 docx）
  --file       本地文件路径（与 --from-clipboard 二选一）
  --from-clipboard  从系统剪贴板读取图片代替 --file（全程内存、不落临时文件，文件名 clipboard.png；
               macOS/Windows 内置，Linux 需要 xclip / wl-paste / xsel 之一）
  --type       插入类型（image/file，默认 image）
  --file-view  附件块展示方式（card/preview/inline，仅 --type file；不传时由服务端按默认卡片展示；
               preview 把音视频渲染为内嵌播放器；inline 与官方取值一致，但服务端当前只接受 card/preview，
               实测 inline 返回 99992402 且不会创建任何块）
  --align      图片对齐方式（left/center/right，默认 center，仅图片）
  --caption    图片描述（仅图片）
  --width      图片显示宽度（像素，仅图片；只给一边时按原图比例计算另一边）
  --height     图片显示高度（像素，仅图片）
  --output     输出格式（json/text，默认 text）

超过 20MB 的文件自动走分片上传（upload_prepare / upload_part / upload_finish）。

示例:
  # 插入图片（居中对齐）
  feishu-cli doc media-insert DOC_ID --file photo.png --type image --align center

  # 插入图片并添加描述
  feishu-cli doc media-insert DOC_ID --file logo.png --type image --caption "公司 Logo"

  # 指定显示宽度（高度按原图比例）
  feishu-cli doc media-insert DOC_ID --file chart.png --width 600

  # 插入文件（>20MB 自动分片上传）
  feishu-cli doc media-insert DOC_ID --file report.pdf --type file

  # 插入视频附件并以预览（内嵌播放器）方式展示
  feishu-cli doc media-insert DOC_ID --file demo.mp4 --type file --file-view preview

  # 从剪贴板插入截图
  feishu-cli doc media-insert DOC_ID --from-clipboard --caption "架构图"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		filePath, _ := cmd.Flags().GetString("file")
		fromClipboard, _ := cmd.Flags().GetBool("from-clipboard")
		fileView, _ := cmd.Flags().GetString("file-view")
		insertType, _ := cmd.Flags().GetString("type")
		alignStr, _ := cmd.Flags().GetString("align")
		caption, _ := cmd.Flags().GetString("caption")
		output, _ := cmd.Flags().GetString("output")
		userAccessToken := resolveOptionalUserToken(cmd)

		if filePath == "" && !fromClipboard {
			return clierr.Usagef("必须指定 --file 或 --from-clipboard 之一")
		}
		if filePath != "" && fromClipboard {
			return clierr.Usagef("--file 与 --from-clipboard 只能指定一个")
		}
		if insertType != "image" && insertType != "file" {
			return clierr.Usagef("不支持的 --type %q，可选 image / file", insertType)
		}
		fileViewType := 0
		if cmd.Flags().Changed("file-view") {
			v, ok := mediaFileViewTypes[fileView]
			if !ok {
				return clierr.Usagef("不支持的 --file-view %q，可选 card / preview / inline", fileView)
			}
			if insertType != "file" {
				return clierr.Usagef("--file-view 只用于 --type file")
			}
			fileViewType = v
		}
		widthSet, heightSet := cmd.Flags().Changed("width"), cmd.Flags().Changed("height")
		userWidth, _ := cmd.Flags().GetInt("width")
		userHeight, _ := cmd.Flags().GetInt("height")
		if (widthSet || heightSet) && insertType != "image" {
			return clierr.Usagef("--width / --height 只用于 --type image")
		}
		const maxImageDimension = 10000
		if widthSet && (userWidth <= 0 || userWidth > maxImageDimension) {
			return clierr.Usagef("--width 必须是 1-%d 的整数像素，当前: %d", maxImageDimension, userWidth)
		}
		if heightSet && (userHeight <= 0 || userHeight > maxImageDimension) {
			return clierr.Usagef("--height 必须是 1-%d 的整数像素，当前: %d", maxImageDimension, userHeight)
		}
		// 剪贴板在联网前读取：读不到图片时直接以退出码 2 结束，不发任何请求
		var clipboardData []byte
		fileName := filepath.Base(filePath)
		displayFile := filePath
		if fromClipboard {
			data, err := readClipboardImage()
			if err != nil {
				return err
			}
			clipboardData, fileName, displayFile = data, "clipboard.png", "clipboard.png"
		} else if st, statErr := safefile.StatInputFile(filePath); statErr != nil {
			// 敏感目录、不存在、是目录、无权限读取均为用法错误（先于任何网络请求）
			return fmt.Errorf("--file 无效: %w", statErr)
		} else if !st.Mode().IsRegular() || st.Size() == 0 {
			return clierr.Usagef("--file %s 不是非空的普通文件", filePath)
		}
		// 显示尺寸在上传前算好：只给一边时按原图比例补另一边，无法解析原图尺寸则要求两边都给
		dispW, dispH := 0, 0
		if insertType == "image" {
			var pxW, pxH int
			if fromClipboard {
				pxW, pxH = decodeImagePixelSizeBytes(clipboardData)
			} else {
				pxW, pxH = decodeImagePixelSize(filePath)
			}
			var dimErr error
			dispW, dispH, dimErr = resolveImageDisplaySize(userWidth, userHeight, widthSet, heightSet, pxW, pxH)
			if dimErr != nil {
				return dimErr
			}
		}

		// wiki 节点必须先换出底层 docx 的 obj_token：块接口与素材上传的 drive_route_token
		// 都只认文档 token，直接使用 wiki token 会导致创建块失败或素材挂错路由。
		documentID, err := resolveDocxArg(args[0], "<document_id|url>", userAccessToken)
		if err != nil {
			return err
		}

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
		rootBlock, err := client.GetBlock(documentID, documentID, userAccessToken)
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
			fileBlock := &larkdocx.File{Token: &emptyToken}
			// view_type 只能在创建块时设置（replace_file 不接受）；不传 --file-view 时保持服务端默认
			if fileViewType != 0 {
				fileBlock.ViewType = &fileViewType
			}
			newBlock := &larkdocx.Block{
				BlockType: &blockType,
				File:      fileBlock,
			}
			createdBlocks, _, createErr := client.CreateBlock(documentID, documentID, []*larkdocx.Block{newBlock}, insertIndex, userAccessToken)
			if createErr != nil {
				if fileViewType == mediaFileViewTypes["inline"] && client.HasAPICode(createErr, 99992402) {
					return fmt.Errorf("步骤 2 失败 - 创建空文件块: %w\n提示：服务端当前只接受 view_type 1/2（实测 inline=3 返回 99992402，未创建任何块），"+
						"请改用 --file-view card 或 preview", createErr)
				}
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
			fileToken, err = uploadInsertMedia(filePath, clipboardData, parentType, fileBlockID, fileName, documentID, userAccessToken)
			if err != nil {
				rollbackErr := rollbackInsertedBlock(documentID, insertIndex, userAccessToken)
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
			}, userAccessToken)
			if err != nil {
				rollbackErr := rollbackInsertedBlock(documentID, insertIndex, userAccessToken)
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
			createdBlocks, _, createErr := client.CreateBlock(documentID, documentID, []*larkdocx.Block{newBlock}, insertIndex, userAccessToken)
			if createErr != nil {
				return fmt.Errorf("步骤 2 失败 - 创建空块: %w", createErr)
			}
			if len(createdBlocks) == 0 {
				return fmt.Errorf("步骤 2 失败 - 未返回块信息")
			}
			newBlockID = client.StringVal(createdBlocks[0].BlockId)

			// 步骤 3：上传文件到 Drive
			fileToken, err = uploadInsertMedia(filePath, clipboardData, parentType, newBlockID, fileName, documentID, userAccessToken)
			if err != nil {
				rollbackErr := rollbackInsertedBlock(documentID, insertIndex, userAccessToken)
				if rollbackErr != nil {
					return fmt.Errorf("步骤 3 失败 - 上传文件: %w（回滚失败: %v）", err, rollbackErr)
				}
				return fmt.Errorf("步骤 3 失败 - 上传文件: %w（已回滚空块）", err)
			}

			// 步骤 4：绑定文件 token 到图片块（显式宽高与降级策略见 client.ReplaceImage 注释）
			_, err = client.ReplaceImage(documentID, newBlockID, fileToken, client.ReplaceImageOptions{
				Width:   dispW,
				Height:  dispH,
				Align:   align,
				Caption: caption,
			}, userAccessToken)
			if err != nil {
				rollbackErr := rollbackInsertedBlock(documentID, insertIndex, userAccessToken)
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
			"file":        displayFile,
		}

		if output == "json" {
			return printJSON(result)
		} else {
			fmt.Printf("插入成功！\n")
			fmt.Printf("  文档 ID:   %s\n", documentID)
			fmt.Printf("  块 ID:     %s\n", newBlockID)
			fmt.Printf("  文件 Token: %s\n", fileToken)
			fmt.Printf("  类型:      %s\n", insertType)
			fmt.Printf("  文件:      %s\n", displayFile)
		}

		return nil
	},
}

// resolveImageDisplaySize 计算图片显示尺寸：两边都给直接用；只给一边按原图比例计算另一边；
// 都不给时用原图像素尺寸（解析失败返回 0，交由服务端推断）。
func resolveImageDisplaySize(userW, userH int, widthSet, heightSet bool, nativeW, nativeH int) (int, int, error) {
	switch {
	case widthSet && heightSet:
		return userW, userH, nil
	case widthSet || heightSet:
		if nativeW <= 0 || nativeH <= 0 {
			return 0, 0, clierr.Usagef("无法解析原图尺寸，不能按比例计算另一边；请同时提供 --width 与 --height")
		}
		if widthSet {
			h := int(float64(userW)*float64(nativeH)/float64(nativeW) + 0.5)
			if h < 1 {
				h = 1
			}
			return userW, h, nil
		}
		w := int(float64(userH)*float64(nativeW)/float64(nativeH) + 0.5)
		if w < 1 {
			w = 1
		}
		return w, userH, nil
	default:
		return nativeW, nativeH, nil
	}
}

// mediaFileViewTypes 是 --file-view 到 File 块 view_type 的映射（1 卡片、2 预览、3 行内），与官方一致。
var mediaFileViewTypes = map[string]int{
	"card":    1,
	"preview": 2,
	"inline":  3,
}

// uploadInsertMedia 上传 media-insert 的素材：剪贴板走内存上传，本地文件走路径上传（均支持 >20MB 分片）。
func uploadInsertMedia(filePath string, clipboardData []byte, parentType, parentNode, fileName, documentID, userAccessToken string) (string, error) {
	if clipboardData != nil {
		return client.UploadDocMediaBytes(clipboardData, parentType, parentNode, fileName, documentID, userAccessToken)
	}
	return client.UploadDocMedia(filePath, parentType, parentNode, fileName, documentID, userAccessToken)
}

// decodeImagePixelSizeBytes 是 decodeImagePixelSize 的内存版本（剪贴板图片），规则相同：
// 解析失败或 JPEG 带 90°/270° 旋转 EXIF 时返回 0, 0。
func decodeImagePixelSizeBytes(data []byte) (int, int) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0
	}
	if format == "jpeg" {
		if o := jpegEXIFOrientation(bytes.NewReader(data)); o >= 5 && o <= 8 {
			return 0, 0
		}
	}
	return cfg.Width, cfg.Height
}

// rollbackInsertedBlock 回滚创建的空块
func rollbackInsertedBlock(documentID string, blockIndex int, userAccessToken string) error {
	_, err := client.DeleteBlocks(documentID, documentID, blockIndex, blockIndex+1, userAccessToken)
	return err
}

func init() {
	docCmd.AddCommand(docMediaInsertCmd)
	docMediaInsertCmd.Flags().String("file", "", "本地文件路径（与 --from-clipboard 二选一）")
	docMediaInsertCmd.Flags().Bool("from-clipboard", false, "从系统剪贴板读取图片代替 --file（Linux 需要 xclip / wl-paste / xsel）")
	docMediaInsertCmd.Flags().String("type", "image", "插入类型（image/file）")
	docMediaInsertCmd.Flags().String("file-view", "", "附件展示方式（card/preview/inline，仅 --type file；默认由服务端按卡片展示）")
	docMediaInsertCmd.Flags().String("align", "center", "图片对齐方式（left/center/right，仅图片）")
	docMediaInsertCmd.Flags().String("caption", "", "图片描述（仅图片）")
	docMediaInsertCmd.Flags().Int("width", 0, "图片显示宽度（像素，仅图片；只给一边时按原图比例计算另一边）")
	docMediaInsertCmd.Flags().Int("height", 0, "图片显示高度（像素，仅图片）")
	docMediaInsertCmd.Flags().StringP("output", "o", "", "输出格式（json/text）")
	docMediaInsertCmd.Flags().String("user-access-token", "", "User Access Token（可选）")
}
