package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var slidesCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建新的 Slides 演示文稿",
	Long: `创建新的飞书 Slides 演示文稿。

参数:
  --title, -t       演示文稿标题（默认 "Untitled"）
  --width           幻灯片宽度像素（默认 960）
  --height          幻灯片高度像素（默认 540）
  --output, -o      输出格式，可选 json

权限: slides:presentation:create 或 slides:presentation:write_only

示例:
  # 创建空白演示文稿
  feishu-cli slides create --title "Q2 OKR"

  # JSON 输出
  feishu-cli slides create --title "Demo" --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		title, _ := cmd.Flags().GetString("title")
		width, _ := cmd.Flags().GetInt("width")
		height, _ := cmd.Flags().GetInt("height")
		output, _ := cmd.Flags().GetString("output")
		userAccessToken := resolveOptionalUserToken(cmd)

		result, err := client.CreateSlides(client.CreateSlidesOptions{
			Title:           title,
			Width:           width,
			Height:          height,
			UserAccessToken: userAccessToken,
		})
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(result)
		}

		fmt.Printf("Slides 演示文稿已创建：\n")
		fmt.Printf("  xml_presentation_id: %s\n", result.XmlPresentationID)
		fmt.Printf("  title:               %s\n", result.Title)
		if result.RevisionID > 0 {
			fmt.Printf("  revision_id:         %d\n", result.RevisionID)
		}
		return nil
	},
}

func init() {
	slidesCmd.AddCommand(slidesCreateCmd)
	slidesCreateCmd.Flags().StringP("title", "t", "", "演示文稿标题（默认 \"Untitled\"）")
	slidesCreateCmd.Flags().Int("width", 0, "幻灯片宽度像素（默认 960）")
	slidesCreateCmd.Flags().Int("height", 0, "幻灯片高度像素（默认 540）")
	slidesCreateCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
	slidesCreateCmd.Flags().String("user-access-token", "", "User Access Token")
}
