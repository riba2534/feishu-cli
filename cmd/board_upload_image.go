package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var boardUploadImageCmd = &cobra.Command{
	Use:   "upload-image <whiteboard_id> <local_image_path>",
	Short: "上传本地图片成画板 image 节点（一键，免手工构造 JSON）",
	Long: `通过 drive 媒体上传 API 将本地图片以 parent_type=whiteboard 上传，
拿到 image token 后立即构造 image 节点落到画板。

参数:
  <whiteboard_id>       画板 ID
  <local_image_path>    本地图片路径（jpeg/png/gif/webp/bmp/tiff）

特性:
  - 默认宽高读自图片实际像素，可用 --width/--height 覆盖
  - 每张图片独立 token，API 不支持复用
  - --dry-run 只 decode 图片信息，不上传

示例:
  feishu-cli board upload-image <id> photo.png --x 100 --y 100
  feishu-cli board upload-image <id> photo.png --width 600 --height 400 -o json`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		whiteboardID := args[0]
		imgPath := args[1]
		xFlag, _ := cmd.Flags().GetFloat64("x")
		yFlag, _ := cmd.Flags().GetFloat64("y")
		widthFlag, _ := cmd.Flags().GetFloat64("width")
		heightFlag, _ := cmd.Flags().GetFloat64("height")
		zIndex, _ := cmd.Flags().GetInt("z-index")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		output, _ := cmd.Flags().GetString("output")
		userAccessToken := resolveOptionalUserToken(cmd)

		// 读图片实际像素（只读文件头，见 decodeImagePixelSize；解析失败返回 0）
		pxW, pxH := decodeImagePixelSize(imgPath)
		pxWidth, pxHeight := float64(pxW), float64(pxH)

		width := widthFlag
		height := heightFlag
		if width == 0 {
			width = pxWidth
		}
		if height == 0 {
			height = pxHeight
		}
		if width == 0 || height == 0 {
			return fmt.Errorf("无法识别图片尺寸（不支持的格式或带旋转 EXIF 的 JPEG），请显式指定 --width/--height")
		}

		if dryRun {
			r := map[string]any{
				"whiteboard_id": whiteboardID,
				"image_path":    imgPath,
				"x":             xFlag, "y": yFlag,
				"width": width, "height": height,
				"z_index":   zIndex,
				"px_width":  pxWidth,
				"px_height": pxHeight,
				"dry_run":   true,
			}
			if output == "json" {
				return printJSON(r)
			}
			fmt.Printf("[dry-run] 将上传 %s（实际 %.0fx%.0f）→ 画板 %s 位置 (%.0f,%.0f) 尺寸 %.0fx%.0f\n",
				imgPath, pxWidth, pxHeight, whiteboardID, xFlag, yFlag, width, height)
			return nil
		}

		if err := config.Validate(); err != nil {
			return err
		}

		fileName := filepath.Base(imgPath)
		token, _, err := client.UploadMedia(imgPath, "whiteboard", whiteboardID, fileName, userAccessToken)
		if err != nil {
			return fmt.Errorf("上传图片失败: %w", err)
		}

		node := map[string]any{
			"type":    "image",
			"x":       xFlag,
			"y":       yFlag,
			"width":   width,
			"height":  height,
			"angle":   0,
			"z_index": zIndex,
			"image":   map[string]any{"token": token},
		}
		nodesJSON, _ := json.Marshal([]map[string]any{node})
		nodeIDs, err := client.CreateBoardNodes(whiteboardID, string(nodesJSON), client.CreateBoardNotesOptions{
			UserAccessToken: userAccessToken,
		})
		if err != nil {
			return fmt.Errorf("创建图片节点失败: %w", err)
		}

		r := map[string]any{
			"whiteboard_id": whiteboardID,
			"image_token":   token,
			"node_id":       firstID(nodeIDs),
			"x":             xFlag, "y": yFlag,
			"width": width, "height": height,
		}
		if output == "json" {
			return printJSON(r)
		}
		fmt.Printf("画板图片节点已创建：\n  画板 ID: %s\n  Image Token: %s\n  Node ID: %s\n  位置: (%.0f, %.0f) 尺寸 %.0fx%.0f\n",
			whiteboardID, token, firstID(nodeIDs), xFlag, yFlag, width, height)
		return nil
	},
}

func init() {
	boardCmd.AddCommand(boardUploadImageCmd)
	boardUploadImageCmd.Flags().Float64("x", 0, "落点 x")
	boardUploadImageCmd.Flags().Float64("y", 0, "落点 y")
	boardUploadImageCmd.Flags().Float64("width", 0, "节点宽度（默认按图片实际像素）")
	boardUploadImageCmd.Flags().Float64("height", 0, "节点高度（默认按图片实际像素）")
	boardUploadImageCmd.Flags().Int("z-index", 10, "节点层级")
	boardUploadImageCmd.Flags().Bool("dry-run", false, "预览不调用 API")
	boardUploadImageCmd.Flags().String("user-access-token", "", "User Access Token")
	boardUploadImageCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
}
