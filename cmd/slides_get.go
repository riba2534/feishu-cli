package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/riba2534/feishu-cli/v2/internal/safefile"
	"github.com/spf13/cobra"
)

var slidesGetCmd = &cobra.Command{
	Use:     "get <xml_presentation_id|url>",
	Aliases: []string{"xml-get"},
	Short:   "读取 Slides 演示文稿全文或单页 XML",
	Long: `读取飞书 Slides 演示文稿的 XML（全文或单页）。别名: xml-get。

参数:
  <xml_presentation_id>   演示文稿唯一标识，或 /slides/ URL、/wiki/ URL（wiki 自动解析为底层演示文稿，
                          底层不是 slides 时报错）
  --slide-id              只读取该页（slide_id）
  --slide-number          只读取第 N 页（从 1 开始），与 --slide-id 互斥
  --remove-attr-id        去掉返回 XML 中的 id 属性（仅全文读取可用；适合只读浏览，不适合后续按 id 编辑）
  --output-file           把 XML 写入本地文件（原子写入，已存在则覆盖），stdout 只输出元信息
  --revision-id           -1（默认）表示最新版本。注意：读取接口目前忽略正整数版本号、始终返回最新版本，
                          0 会被服务端拒绝（3350001）；需要历史版本请在飞书客户端的版本记录中查看
  --output, -o            输出格式，可选 json

权限: slides:presentation:read

示例:
  # 读取演示文稿全文 XML
  feishu-cli slides get <xml_presentation_id>
  feishu-cli slides get https://xxx.feishu.cn/slides/<xml_presentation_id>
  feishu-cli slides get https://xxx.feishu.cn/wiki/<node_token>

  # 只读第 2 页 / 指定 slide_id 的页（编辑前先读，拿到元素 id）
  feishu-cli slides get <xml_presentation_id> --slide-number 2
  feishu-cli slides get <xml_presentation_id> --slide-id <slide_id> -o json

  # 存到本地文件
  feishu-cli slides get <xml_presentation_id> --output-file deck.xml

  # JSON 输出（含 content/revision_id/xml_presentation_id）
  feishu-cli slides get <xml_presentation_id> --output json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		output, _ := cmd.Flags().GetString("output")
		if output != "" && output != "json" {
			return clierr.Usagef("无效的 --output: %s，有效值: json（写文件请用 --output-file <path>）", output)
		}
		revisionID, err := validateSlidesRevisionID(cmd)
		if err != nil {
			return err
		}
		slideID := strings.TrimSpace(flagString(cmd, "slide-id"))
		slideNumber, _ := cmd.Flags().GetInt("slide-number")
		removeAttrID, _ := cmd.Flags().GetBool("remove-attr-id")
		outputFile := strings.TrimSpace(flagString(cmd, "output-file"))
		if cmd.Flags().Changed("slide-id") && slideID == "" {
			return clierr.Usagef("--slide-id 不能为空")
		}
		if cmd.Flags().Changed("slide-number") && slideNumber < 1 {
			return clierr.Usagef("--slide-number 必须是正整数（从 1 开始）")
		}
		if slideID != "" && slideNumber > 0 {
			return clierr.Usagef("--slide-id 与 --slide-number 只能二选一")
		}
		singleSlide := slideID != "" || slideNumber > 0
		if singleSlide && removeAttrID {
			return clierr.Usagef("--remove-attr-id 只支持读取全文 XML")
		}
		if outputFile != "" {
			if err := safefile.ValidateOutputPath(outputFile); err != nil {
				return err
			}
		}

		if err := config.Validate(); err != nil {
			return err
		}
		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		// 只按 URL 路径前缀识别 /slides/ 与 /wiki/；把 docx 等其他 URL 原样当 ID 会得到误导性的 404
		presentationID, err := resolvePresentationArg(args[0], userAccessToken)
		if err != nil {
			return err
		}

		var content string
		out := map[string]any{"xml_presentation_id": presentationID}
		if singleSlide {
			query := map[string]any{"revision_id": revisionID}
			if slideID != "" {
				query["slide_id"] = slideID
			}
			if slideNumber > 0 {
				query["slide_number"] = slideNumber
			}
			data, err := client.SlidesCall("读取 slides 页面", "GET", client.SlidesPresentationPath(presentationID, "/slide"), query, nil, userAccessToken)
			if err != nil {
				return err
			}
			slide := slidesMap(data, "slide")
			content = slidesString(slide, "content")
			if strings.TrimSpace(content) == "" {
				return fmt.Errorf("读取 slides 页面失败: 返回的 slide.content 为空")
			}
			out["scope"] = "slide"
			if id := slidesString(slide, "slide_id"); id != "" {
				out["slide_id"] = id
			} else if slideID != "" {
				out["slide_id"] = slideID
			}
			if slideNumber > 0 {
				out["slide_number"] = slideNumber
			}
			copySlidesRevision(out, data)
		} else {
			result, err := client.GetSlidesWithOptions(presentationID, revisionID, removeAttrID, userAccessToken)
			if err != nil {
				return err
			}
			content = result.Content
			out["xml_presentation_id"] = result.XmlPresentationID
			out["scope"] = "presentation"
			out["revision_id"] = result.RevisionID
			if removeAttrID {
				out["remove_attr_id"] = true
			}
		}
		warnSlidesRevisionIgnored(revisionID, out["revision_id"])

		if outputFile != "" {
			if err := safefile.AtomicWriteFile(outputFile, []byte(content), 0o644); err != nil {
				return fmt.Errorf("写入 %s 失败: %w", outputFile, err)
			}
			out["path"] = outputFile
			out["size"] = len(content)
			out["content_saved"] = true
			if output == "json" {
				return printJSON(out)
			}
			fmt.Printf("已保存 %s XML 到 %s（%d 字节）\n", out["scope"], outputFile, len(content))
			return nil
		}

		if output == "json" {
			out["content"] = content
			return printJSON(out)
		}
		fmt.Println(content)
		return nil
	},
}

// warnSlidesRevisionIgnored 用户传了正整数版本号但服务端返回的版本不同：读取接口不支持历史版本，提示一下避免误以为读到了旧版。
func warnSlidesRevisionIgnored(requested int, got any) {
	n, ok := got.(int)
	if requested > 0 && ok && n != requested {
		fmt.Fprintf(os.Stderr, "提示：读取接口忽略 --revision-id %d，返回的是最新版本 %d\n", requested, n)
	}
}

func init() {
	slidesCmd.AddCommand(slidesGetCmd)
	slidesGetCmd.Flags().Int("revision-id", -1, "演示文稿版本号（-1 表示最新；读取接口目前忽略正整数，0 无效）")
	slidesGetCmd.Flags().String("slide-id", "", "只读取该 slide_id 的页面")
	slidesGetCmd.Flags().Int("slide-number", 0, "只读取第 N 页（从 1 开始）")
	slidesGetCmd.Flags().Bool("remove-attr-id", false, "去掉返回 XML 中的 id 属性（仅全文读取）")
	slidesGetCmd.Flags().String("output-file", "", "把 XML 写入本地文件")
	slidesGetCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
	slidesGetCmd.Flags().String("user-access-token", "", "User Access Token")
}
