package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var slidesAddSlideCmd = &cobra.Command{
	Use:   "add-slide <presentation>",
	Short: "向已有演示文稿追加或插入一页",
	Long: `向已有演示文稿追加（默认在末尾）或插入（--before-slide-id）一页。

参数:
  <presentation>       xml_presentation_id、/slides/ URL 或 /wiki/ URL
  --slide              一个完整的 <slide> XML 文档（必填）；支持 @file 读文件、- 读 stdin，绕开 shell 转义
  --before-slide-id    插到该页之前（默认追加到最后）
  --revision-id        -1（默认）表示基于最新版本；传具体版本号用于乐观锁
  --no-lint            跳过服务端 XML lint（默认逐页 lint，error 级问题拒绝写入，错误码 4000153）
  --dry-run            只打印将要发出的请求

图片:
  XML 中 <img src="@./pic.png"> 占位符会先上传到该演示文稿（≤20 MB/张），再替换为 file_token。
  路径相对当前工作目录解析（不是相对 --slide @file 所在目录）；同一张图在逐页循环里会重复上传，
  多页复用的图片先用 slides media-upload 上传一次再写 file_token。

输出 JSON：xml_presentation_id、slide_id、revision_id、images_uploaded；服务端对已写入页面的
schema 告警透出在 issues 字段。

权限: slides:presentation:update 或 slides:presentation:write_only（含图片时另需 docs:document.media:upload）

示例:
  feishu-cli slides add-slide <xml_presentation_id> --slide @page.xml
  feishu-cli slides add-slide <xml_presentation_id> --slide @page.xml --before-slide-id <slide_id>
  cat page.xml | feishu-cli slides add-slide <xml_presentation_id> --slide -
  feishu-cli slides add-slide <xml_presentation_id> --slide @page.xml --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		slideXML, err := readSlidesFlagInput(cmd, "slide")
		if err != nil {
			return err
		}
		slideXML = strings.TrimSpace(slideXML)
		if slideXML == "" {
			return clierr.Usagef("--slide 不能为空（传 XML、@file 或 - 读 stdin）")
		}
		if err := validateCompleteSlideXML(slideXML); err != nil {
			return clierr.Usagef("--slide 不是单个完整的 <slide> 文档: %v", err)
		}
		revisionID, err := validateSlidesRevisionID(cmd)
		if err != nil {
			return err
		}
		beforeSlideID := strings.TrimSpace(flagString(cmd, "before-slide-id"))
		placeholders := extractImagePlaceholderPaths([]string{slideXML})
		if err := validateImagePlaceholderFiles("--slide", placeholders); err != nil {
			return err
		}

		query := map[string]any{"revision_id": revisionID}
		buildBody := func(content string) map[string]any {
			body := map[string]any{"slide": map[string]any{"content": content}}
			// before_slide_id 为空时不能下发：空字符串会被当成不存在的页面
			if beforeSlideID != "" {
				body["before_slide_id"] = beforeSlideID
			}
			return withSlidesLint(cmd, body)
		}

		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			pres, wikiStep, err := slidesDryRunPresentation(args[0])
			if err != nil {
				return err
			}
			var steps []map[string]any
			if wikiStep != nil {
				steps = append(steps, wikiStep)
			}
			for _, p := range placeholders {
				steps = append(steps, map[string]any{
					"method": "POST",
					"path":   "/open-apis/drive/v1/medias/upload_all",
					"desc":   fmt.Sprintf("上传 @%s（parent_type=slide_file，parent_node=%s）", p, pres),
				})
			}
			steps = append(steps, map[string]any{
				"method": "POST",
				"path":   client.SlidesPresentationPath(pres, "/slide"),
				"params": query,
				"body":   buildBody(slideXML),
			})
			return printJSON(map[string]any{"dry_run": true, "steps": steps, "images_to_upload": len(placeholders)})
		}

		if err := config.Validate(); err != nil {
			return err
		}
		userAccessToken := resolveOptionalUserToken(cmd)
		presentationID, err := resolvePresentationArg(args[0], userAccessToken)
		if err != nil {
			return err
		}

		result := map[string]any{"xml_presentation_id": presentationID}
		if len(placeholders) > 0 {
			tokens, uploaded, err := uploadSlidesPlaceholders(presentationID, placeholders, userAccessToken)
			if err != nil {
				return fmt.Errorf("%w\n未添加页面；失败前已上传 %d/%d 张图片", err, uploaded, len(placeholders))
			}
			slideXML = replaceImagePlaceholders(slideXML, tokens)
			result["images_uploaded"] = uploaded
		}

		data, err := client.SlidesCall("添加 slides 页面", "POST", client.SlidesPresentationPath(presentationID, "/slide"), query, buildBody(slideXML), userAccessToken)
		if err != nil {
			extra := ""
			if len(placeholders) > 0 {
				extra = fmt.Sprintf("页面失败前已上传 %d 张图片；直接重跑会再上传一份", len(placeholders))
			}
			return enrichSlidesWriteError(err, extra)
		}
		slideID := slidesString(data, "slide_id")
		if slideID == "" {
			return fmt.Errorf("添加 slides 页面失败: 响应缺少 slide_id")
		}
		result["slide_id"] = slideID
		if beforeSlideID != "" {
			result["before_slide_id"] = beforeSlideID
		}
		copySlidesRevision(result, data)
		// issues 是服务端对已写入页面的 schema 告警（内容被接受但有改动），原样透出供调用方判断
		if issues, ok := data["issues"]; ok {
			result["issues"] = issues
		}
		return printJSON(result)
	},
}

var slidesDeleteSlideCmd = &cobra.Command{
	Use:   "delete-slide <presentation>",
	Short: "按 slide_id 删除演示文稿中的一页",
	Long: `按 slide_id 删除演示文稿中的一页（一次一页，结果明确，不存在"删了几页"的部分失败）。

参数:
  <presentation>   xml_presentation_id、/slides/ URL 或 /wiki/ URL
  --slide-id       要删除的页面 slide_id（必填，先用 slides get 回读确认）
  --revision-id    -1（默认）表示基于最新版本；传具体版本号用于乐观锁
  --dry-run        只打印将要发出的请求
  --yes            跳过确认（非交互环境必须显式传入，否则以退出码 10 拒绝执行）

删除后无法在原地撤销，误删请在飞书客户端的版本记录中恢复。

权限: slides:presentation:update 或 slides:presentation:write_only

示例:
  feishu-cli slides delete-slide <xml_presentation_id> --slide-id <slide_id> --dry-run
  feishu-cli slides delete-slide <xml_presentation_id> --slide-id <slide_id> --yes`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		slideID := strings.TrimSpace(flagString(cmd, "slide-id"))
		if slideID == "" {
			return clierr.Usagef("--slide-id 不能为空")
		}
		revisionID, err := validateSlidesRevisionID(cmd)
		if err != nil {
			return err
		}
		query := map[string]any{"slide_id": slideID, "revision_id": revisionID}

		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			pres, wikiStep, err := slidesDryRunPresentation(args[0])
			if err != nil {
				return err
			}
			var steps []map[string]any
			if wikiStep != nil {
				steps = append(steps, wikiStep)
			}
			steps = append(steps, map[string]any{
				"method": "DELETE",
				"path":   client.SlidesPresentationPath(pres, "/slide"),
				"params": query,
			})
			return printJSON(map[string]any{"dry_run": true, "slide_id": slideID, "steps": steps})
		}

		if err := confirmDangerousAction(cmd, fmt.Sprintf("确定要删除演示文稿 %s 中的页面 %s 吗？", args[0], slideID)); err != nil {
			return err
		}
		if err := config.Validate(); err != nil {
			return err
		}
		userAccessToken := resolveOptionalUserToken(cmd)
		presentationID, err := resolvePresentationArg(args[0], userAccessToken)
		if err != nil {
			return err
		}
		data, err := client.SlidesCall("删除 slides 页面", "DELETE", client.SlidesPresentationPath(presentationID, "/slide"), query, nil, userAccessToken)
		if err != nil {
			return enrichSlidesWriteError(err, "")
		}
		result := map[string]any{"xml_presentation_id": presentationID, "slide_id": slideID, "deleted": true}
		copySlidesRevision(result, data)
		return printJSON(result)
	},
}

func init() {
	slidesCmd.AddCommand(slidesAddSlideCmd)
	slidesAddSlideCmd.Flags().String("slide", "", "一个完整的 <slide> XML 文档（支持 @file、- 读 stdin）")
	slidesAddSlideCmd.Flags().String("before-slide-id", "", "插到该 slide_id 之前（默认追加到最后）")
	slidesAddSlideCmd.Flags().Int("revision-id", -1, "基于的演示文稿版本（-1 表示最新；正整数用于乐观锁）")
	addSlidesNoLintFlag(slidesAddSlideCmd)
	slidesAddSlideCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求，不调用 API")
	slidesAddSlideCmd.Flags().String("user-access-token", "", "User Access Token")

	slidesCmd.AddCommand(slidesDeleteSlideCmd)
	slidesDeleteSlideCmd.Flags().String("slide-id", "", "要删除的页面 slide_id（必填）")
	slidesDeleteSlideCmd.Flags().Int("revision-id", -1, "基于的演示文稿版本（-1 表示最新；正整数用于乐观锁）")
	slidesDeleteSlideCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求，不调用 API")
	slidesDeleteSlideCmd.Flags().String("user-access-token", "", "User Access Token")
}
