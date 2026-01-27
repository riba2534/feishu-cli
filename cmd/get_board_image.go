package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var getBoardImageCmd = &cobra.Command{
	Use:   "image <whiteboard_id> <output_path>",
	Short: "下载画板图片",
	Long: `下载飞书画板（白板）的图片。

参数:
  <whiteboard_id>  画板唯一标识符（必填）
  <output_path>    输出文件路径或目录（必填）
  --output, -o     输出格式 (json)

说明:
  如果 output_path 是目录，则使用画板 ID 作为文件名。

示例:
  # 下载画板图片到指定文件
  feishu-cli board image <whiteboard_id> board.png

  # 下载到目录（使用画板 ID 作为文件名）
  feishu-cli board image <whiteboard_id> ./images/

  # JSON 格式输出
  feishu-cli board image <whiteboard_id> board.png -o json`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		whiteboardID := args[0]
		outputPath := args[1]
		output, _ := cmd.Flags().GetString("output")

		err := client.GetBoardImage(whiteboardID, outputPath)
		if err != nil {
			return err
		}

		if output == "json" {
			result := map[string]string{
				"whiteboard_id": whiteboardID,
				"output_path":   outputPath,
				"status":        "success",
			}
			data, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("画板图片下载成功！\n")
			fmt.Printf("  画板 ID: %s\n", whiteboardID)
			fmt.Printf("  保存路径: %s\n", outputPath)
		}

		return nil
	},
}

func init() {
	boardCmd.AddCommand(getBoardImageCmd)
	getBoardImageCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
}
