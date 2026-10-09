package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var slidesCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建新的 Slides 演示文稿（可一次带最多 10 页）",
	Long: `创建新的飞书 Slides 演示文稿，可选在创建时直接添加页面。

参数:
  --title, -t       演示文稿标题（默认 "Untitled"）
  --width           幻灯片宽度像素（默认 960）
  --height          幻灯片高度像素（默认 540）
  --slides          页面 JSON 数组（每个元素是一个 <slide> XML 字符串，最多 10 页）；支持 @file、- 读 stdin
  --slide           单页 <slide> XML 或 @page.xml，可重复传入（每次一页，最多 10 页），免去手写 JSON 转义；
                    与 --slides 互斥
  --no-lint         跳过服务端 XML lint（默认逐页 lint，error 级问题拒绝写入）
  --dry-run         只打印将要发出的请求（不创建）
  --output, -o      输出格式，可选 json

页面与图片:
  先创建空演示文稿，再逐页添加；某一页被拒绝时停止，已创建的演示文稿与之前的页面保留，
  错误信息会说明进度。超过 10 页时先创建，再用 slides add-slide 逐页追加。
  XML 中 <img src="@./pic.png"> 占位符在创建后、加页前自动上传（≤20 MB/张）并替换为 file_token；
  路径相对当前工作目录解析。
  slides_added 以服务端返回的不同 slide_id 计数：两页 XML 完全相同时服务端可能只新增一页，
  此时 stderr 告警，JSON 另给出 pages_requested 与 duplicate_slides。

权限: slides:presentation:create 或 slides:presentation:write_only（含图片时另需 docs:document.media:upload）

Bot 身份创建（未传 User Token）时，自动给当前 CLI 登录用户授予 full_access，
JSON 输出 url 与 permission_grant；以 User 身份创建时不触发。

示例:
  # 创建空白演示文稿
  feishu-cli slides create --title "Q2 OKR"

  # 创建并带两页（每页一个 XML 文件）
  feishu-cli slides create --title "Q2 OKR" --slide @cover.xml --slide @agenda.xml -o json

  # 页面 JSON 数组从文件读取
  feishu-cli slides create --title "Demo" --slides @slides.json -o json

  # 预览请求
  feishu-cli slides create --title "Demo" --slide @cover.xml --dry-run

  # JSON 输出
  feishu-cli slides create --title "Demo" --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		title, _ := cmd.Flags().GetString("title")
		width, _ := cmd.Flags().GetInt("width")
		height, _ := cmd.Flags().GetInt("height")
		output, _ := cmd.Flags().GetString("output")

		if output != "" && output != "json" {
			return clierr.Usagef("无效的 --output: %s，有效值: json", output)
		}
		if cmd.Flags().Changed("width") && width <= 0 {
			return clierr.Usagef("--width 必须大于 0")
		}
		if cmd.Flags().Changed("height") && height <= 0 {
			return clierr.Usagef("--height 必须大于 0")
		}
		// 先解析并校验全部页面与占位图片：坏路径/坏 XML 不能在演示文稿已创建之后才暴露，留下半成品
		pages, pagesFlag, err := slidesCreatePages(cmd)
		if err != nil {
			return err
		}
		placeholders := extractImagePlaceholderPaths(pages)
		if err := validateImagePlaceholderFiles(pagesFlag, placeholders); err != nil {
			return err
		}

		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			steps := []map[string]any{{
				"method": "POST",
				"path":   "/open-apis/slides_ai/v1/xml_presentations",
				"desc":   fmt.Sprintf("创建演示文稿 %q（Bot 身份创建后自动给当前登录用户授予 full_access）", effectiveSlidesTitle(title)),
			}}
			for _, p := range placeholders {
				steps = append(steps, map[string]any{
					"method": "POST",
					"path":   "/open-apis/drive/v1/medias/upload_all",
					"desc":   fmt.Sprintf("上传 @%s 到新演示文稿", p),
				})
			}
			for i, page := range pages {
				steps = append(steps, map[string]any{
					"method": "POST",
					"path":   "/open-apis/slides_ai/v1/xml_presentations/<xml_presentation_id>/slide",
					"params": map[string]any{"revision_id": -1},
					"body":   withSlidesLint(cmd, map[string]any{"slide": map[string]any{"content": page}}),
					"desc":   fmt.Sprintf("添加第 %d/%d 页", i+1, len(pages)),
				})
			}
			return printJSON(map[string]any{"dry_run": true, "slides": len(pages), "images_to_upload": len(placeholders), "steps": steps})
		}

		if err := config.Validate(); err != nil {
			return err
		}

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

		// Bot 身份创建时自动给当前 CLI 登录用户授予 full_access（User 身份创建不触发）
		grant := autoGrantCurrentUser(userAccessToken, result.XmlPresentationID, client.ResourceTypeSlides)
		url := client.BuildResourceURL(client.ResourceTypeSlides, result.XmlPresentationID)

		out := map[string]any{"xml_presentation_id": result.XmlPresentationID}
		if result.RevisionID != 0 {
			out["revision_id"] = result.RevisionID
		}
		if result.Title != "" {
			out["title"] = result.Title
		}
		if url != "" {
			out["url"] = url
		}

		if len(pages) > 0 {
			if err := addSlidesCreatePages(cmd, result.XmlPresentationID, url, pages, placeholders, userAccessToken, out); err != nil {
				return err
			}
		}

		if output == "json" {
			return printJSON(withPermissionGrant(out, grant))
		}

		fmt.Printf("Slides 演示文稿已创建：\n")
		fmt.Printf("  xml_presentation_id: %s\n", result.XmlPresentationID)
		fmt.Printf("  title:               %s\n", result.Title)
		if rev, ok := out["revision_id"].(int); ok && rev > 0 {
			fmt.Printf("  revision_id:         %d\n", rev)
		}
		if n, ok := out["slides_added"].(int); ok {
			fmt.Printf("  slides_added:        %d\n", n)
		}
		if url != "" {
			fmt.Printf("  url:                 %s\n", url)
		}
		if _, ok := out["slide_issues"]; ok {
			fmt.Fprintln(os.Stderr, "⚠ 部分页面写入时服务端返回了 schema 告警，用 -o json 查看 slide_issues")
		}
		printPermissionGrantText(os.Stdout, grant)
		return nil
	},
}

// slidesCreatePages 解析 --slides（JSON 数组）或 --slide（可重复）为页面 XML 列表，并做结构校验。
// 返回值第二项是出错时应点名的 flag。空列表表示创建空白演示文稿。
func slidesCreatePages(cmd *cobra.Command) ([]string, string, error) {
	slidesGiven := cmd.Flags().Changed("slides")
	slideArgs, _ := cmd.Flags().GetStringArray("slide")
	if slidesGiven && len(slideArgs) > 0 {
		return nil, "--slide", clierr.Usagef("--slide 与 --slides 不能同时使用：整组数组用 --slides，逐页用 --slide")
	}
	var pages []string
	flag := "--slides"
	switch {
	case slidesGiven:
		raw, err := readSlidesFlagInput(cmd, "slides")
		if err != nil {
			return nil, flag, err
		}
		// 空值 / null 不能当作"没给页面"：那是命令替换失败的典型形态，会把空白 deck 报告为成功
		if err := json.Unmarshal([]byte(raw), &pages); err != nil || pages == nil {
			return nil, flag, clierr.Usagef("--slides 必须是 XML 字符串组成的 JSON 数组\n提示：想逐页传 XML 文件可改用重复的 --slide @page.xml")
		}
	case len(slideArgs) > 0:
		flag = "--slide"
		for i, raw := range slideArgs {
			raw = strings.TrimSpace(raw)
			if raw == "-" {
				return nil, flag, clierr.Usagef("--slide 不支持 -（stdin 只有一个）；逐页用 --slide @page.xml，或把整组数组通过 --slides - 传入")
			}
			page, err := resolveSlidesInputValue(raw, fmt.Sprintf("--slide（第 %d 页）", i+1))
			if err != nil {
				return nil, flag, err
			}
			pages = append(pages, page)
		}
	default:
		return nil, flag, nil
	}
	if len(pages) > maxSlidesPerCreate {
		return nil, flag, clierr.Usagef("%s 一次最多 %d 页（实际 %d 页）；先创建，再用 slides add-slide 逐页追加其余页面", flag, maxSlidesPerCreate, len(pages))
	}
	for i := range pages {
		pages[i] = strings.TrimSpace(pages[i])
		if pages[i] == "" {
			return nil, flag, clierr.Usagef("%s: 第 %d 页为空", flag, i+1)
		}
		if err := validateCompleteSlideXML(pages[i]); err != nil {
			return nil, flag, clierr.Usagef("%s: 第 %d 页不是单个完整的 <slide> 文档: %v", flag, i+1, err)
		}
	}
	return pages, flag, nil
}

// addSlidesCreatePages 在新建的演示文稿上传占位图并逐页添加，结果写入 out。
// 任一步失败即停止，错误中说明已创建的演示文稿与已添加的页数。
func addSlidesCreatePages(cmd *cobra.Command, presentationID, url string, pages, placeholders []string, userAccessToken string, out map[string]any) error {
	progress := func(added int) string {
		s := fmt.Sprintf("演示文稿 %s 已创建", presentationID)
		if url != "" {
			s += "（" + url + "）"
		}
		return s + fmt.Sprintf("，失败前已添加 %d/%d 页；用 slides add-slide 补齐剩余页面，不要重新 create", added, len(pages))
	}
	if len(placeholders) > 0 {
		tokens, uploaded, err := uploadSlidesPlaceholders(presentationID, placeholders, userAccessToken)
		if err != nil {
			return fmt.Errorf("%w\n提示：%s；失败前已上传 %d/%d 张图片", err, progress(0), uploaded, len(placeholders))
		}
		for i := range pages {
			pages[i] = replaceImagePlaceholders(pages[i], tokens)
		}
		out["images_uploaded"] = uploaded
	}
	slideIDs := []string{}
	seenSlide := map[string]int{} // slide_id → 首次出现的页序号（1 起）
	var duplicates []map[string]any
	var slideIssues []map[string]any
	for i, page := range pages {
		body := withSlidesLint(cmd, map[string]any{"slide": map[string]any{"content": page}})
		data, err := client.SlidesCall(fmt.Sprintf("添加第 %d/%d 页", i+1, len(pages)), "POST",
			client.SlidesPresentationPath(presentationID, "/slide"), map[string]any{"revision_id": -1}, body, userAccessToken)
		if err != nil {
			return enrichSlidesWriteError(err, progress(i))
		}
		sid := slidesString(data, "slide_id")
		if sid != "" {
			// 以服务端返回为准：同一 slide_id 再次出现说明服务端没有新增页面（如两页 XML 完全相同）
			if first, dup := seenSlide[sid]; dup {
				duplicates = append(duplicates, map[string]any{"slide_index": i + 1, "slide_id": sid, "same_as_index": first})
			} else {
				seenSlide[sid] = i + 1
				slideIDs = append(slideIDs, sid)
			}
		}
		if issues, ok := data["issues"]; ok {
			slideIssues = append(slideIssues, map[string]any{"slide_index": i + 1, "slide_id": sid, "issues": issues})
		}
		copySlidesRevision(out, data)
	}
	out["slide_ids"] = slideIDs
	out["slides_added"] = len(slideIDs)
	if len(duplicates) > 0 {
		out["pages_requested"] = len(pages)
		out["duplicate_slides"] = duplicates
		for _, d := range duplicates {
			fmt.Fprintf(os.Stderr, "⚠ 第 %d 页返回的 slide_id %s 与第 %d 页相同，服务端没有新增页面（常见原因：两页 XML 完全相同）\n",
				d["slide_index"], d["slide_id"], d["same_as_index"])
		}
		fmt.Fprintf(os.Stderr, "⚠ 请求 %d 页，实际新增 %d 页；需要多页时请让每页内容有所区别，或用 slides add-slide 补页\n", len(pages), len(slideIDs))
	}
	if len(slideIssues) > 0 {
		out["slide_issues"] = slideIssues
	}
	return nil
}

func effectiveSlidesTitle(title string) string {
	if strings.TrimSpace(title) == "" {
		return "Untitled"
	}
	return title
}

func init() {
	slidesCmd.AddCommand(slidesCreateCmd)
	slidesCreateCmd.Flags().StringP("title", "t", "", "演示文稿标题（默认 \"Untitled\"）")
	slidesCreateCmd.Flags().Int("width", 0, "幻灯片宽度像素（默认 960）")
	slidesCreateCmd.Flags().Int("height", 0, "幻灯片高度像素（默认 540）")
	slidesCreateCmd.Flags().String("slides", "", "页面 JSON 数组（每项一个 <slide> XML，最多 10 页；支持 @file、- 读 stdin）")
	slidesCreateCmd.Flags().StringArray("slide", nil, "单页 <slide> XML 或 @page.xml，可重复（最多 10 页，与 --slides 互斥）")
	addSlidesNoLintFlag(slidesCreateCmd)
	slidesCreateCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求，不创建")
	slidesCreateCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
	slidesCreateCmd.Flags().String("user-access-token", "", "User Access Token")
}
