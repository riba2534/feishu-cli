package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var slidesGetCmd = &cobra.Command{
	Use:   "get <xml_presentation_id|url>",
	Short: "读取 Slides 演示文稿内容（XML 格式）",
	Long: `读取指定飞书 Slides 演示文稿的全文信息，以 XML 格式返回。

参数:
  <xml_presentation_id>   演示文稿唯一标识，或 /slides/ URL、/wiki/ URL（wiki 自动解析为底层演示文稿，
                          底层不是 slides 时报错）
  --revision-id           演示文稿版本号（默认 -1 表示最新版本）
  --output, -o            输出格式，可选 json

权限: slides:presentation:read

示例:
  # 读取演示文稿 XML
  feishu-cli slides get <xml_presentation_id>
  feishu-cli slides get https://xxx.feishu.cn/slides/<xml_presentation_id>
  feishu-cli slides get https://xxx.feishu.cn/wiki/<node_token>

  # JSON 输出（含 content/revision_id/presentation_id）
  feishu-cli slides get <xml_presentation_id> --output json

  # 读取指定版本
  feishu-cli slides get <xml_presentation_id> --revision-id 1`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		revisionID, _ := cmd.Flags().GetInt("revision-id")
		output, _ := cmd.Flags().GetString("output")
		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		// 只按 URL 路径前缀识别 /slides/ 与 /wiki/；把 docx 等其他 URL 原样当 ID 会得到误导性的 404
		res, err := resolveResourceArg(args[0], resourceArgOptions{
			ArgName:         "<xml_presentation_id|url>",
			DefaultType:     client.ResourceTypeSlides,
			Allowed:         []string{client.ResourceTypeSlides},
			ResolveWiki:     true,
			UserAccessToken: userAccessToken,
		})
		if err != nil {
			return err
		}
		noteWikiResolved(res)
		presentationID := res.Token

		result, err := client.GetSlides(presentationID, revisionID, userAccessToken)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(result)
		}

		fmt.Println(result.Content)
		return nil
	},
}

func init() {
	slidesCmd.AddCommand(slidesGetCmd)
	slidesGetCmd.Flags().Int("revision-id", -1, "演示文稿版本号（默认 -1 表示最新版本）")
	slidesGetCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
	slidesGetCmd.Flags().String("user-access-token", "", "User Access Token")
}
