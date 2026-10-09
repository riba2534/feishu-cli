package cmd

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/riba2534/feishu-cli/v2/internal/safefile"
	"github.com/spf13/cobra"
)

const defaultSlidesScreenshotDir = "slides_screenshots"

var slidesScreenshotCmd = &cobra.Command{
	Use:   "screenshot [presentation]",
	Short: "把幻灯片页面（或一段 slide XML）渲染成本地图片",
	Long: `调用服务端渲染幻灯片截图并保存为本地图片文件；stdout 只输出文件元信息，不打印 Base64。

两种模式:
  1. 已有页面：<presentation> + --slide-id / --slide-number（可重复或逗号分隔，每次最多 10 页，二者互斥）
  2. 直接渲染 XML：--content <slide XML | @file | ->（不需要 <presentation>，适合写回前预览效果）

参数:
  <presentation>   xml_presentation_id、/slides/ URL 或 /wiki/ URL（--content 模式不传）
  --slide-id       页面 slide_id，可重复或逗号分隔
  --slide-number   页码（从 1 开始），可重复或逗号分隔
  --content        直接渲染的 <slide> XML（支持 @file、- 读 stdin）
  --output         单张截图的输出路径（扩展名可省略，按实际格式补 .png/.jpg）；只能对应一页
  --output-dir     输出目录（默认 slides_screenshots），文件名为 <presentation>_p<页码>_<slide_id>.<ext>，同名自动加序号
  --output-name    --content 模式下的文件名（不含扩展名）
  --dry-run        只打印将要发出的请求

权限: slides:presentation:screenshot（读取 wiki URL 另需 wiki:node:read）

示例:
  feishu-cli slides screenshot <xml_presentation_id> --slide-number 1 --output cover
  feishu-cli slides screenshot <xml_presentation_id> --slide-number 1,2,3 --output-dir shots
  feishu-cli slides screenshot <xml_presentation_id> --slide-id <slide_id>
  feishu-cli slides screenshot --content @page.xml --output preview`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		renderMode := cmd.Flags().Changed("content")
		slideIDs := normalizeSlideIDList(mustStringSlice(cmd, "slide-id"))
		slideNumbers, _ := cmd.Flags().GetIntSlice("slide-number")
		outputPath := strings.TrimSpace(flagString(cmd, "output"))
		outputDir := flagString(cmd, "output-dir")
		outputName := strings.TrimSpace(flagString(cmd, "output-name"))

		var content string
		if renderMode {
			c, err := readSlidesFlagInput(cmd, "content")
			if err != nil {
				return err
			}
			content = c
			if strings.TrimSpace(content) == "" {
				return clierr.Usagef("--content 不能为空")
			}
			if len(args) > 0 || len(slideIDs) > 0 || len(slideNumbers) > 0 {
				return clierr.Usagef("--content 模式不能同时传 <presentation>、--slide-id 或 --slide-number")
			}
		} else {
			if len(args) == 0 {
				return clierr.Usagef("缺少 <presentation>；或用 --content 直接渲染 XML")
			}
			if cmd.Flags().Changed("output-name") {
				return clierr.Usagef("--output-name 只用于 --content 模式；已有页面请用 --output 或 --output-dir")
			}
			if len(slideIDs) == 0 && len(slideNumbers) == 0 {
				return clierr.Usagef("需要 --slide-id 或 --slide-number 指定页面（每次最多 10 页）")
			}
			if len(slideIDs) > 0 && len(slideNumbers) > 0 {
				return clierr.Usagef("--slide-id 与 --slide-number 不能同时使用")
			}
			var err error
			if slideNumbers, err = normalizeSlideNumberList(slideNumbers); err != nil {
				return err
			}
			if n := len(slideIDs) + len(slideNumbers); n > maxSlidesPerScreenshot {
				return clierr.Usagef("一次最多截图 %d 页（实际 %d 页），请分批执行", maxSlidesPerScreenshot, n)
			}
		}
		count := 1
		if !renderMode {
			count = len(slideIDs) + len(slideNumbers)
		}
		if outputPath != "" {
			if cmd.Flags().Changed("output-dir") || cmd.Flags().Changed("output-name") {
				return clierr.Usagef("--output 不能与 --output-dir / --output-name 同时使用")
			}
			if count != 1 {
				return clierr.Usagef("--output 只能对应一页；多页请用 --output-dir")
			}
			ext := strings.ToLower(filepath.Ext(outputPath))
			if ext != "" && ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
				return clierr.Usagef("--output 扩展名只能是 .png / .jpg / .jpeg，或省略（按实际格式补全）")
			}
			if err := safefile.ValidateOutputPath(outputPath); err != nil {
				return err
			}
		} else if err := safefile.ValidateOutputPath(filepath.Join(outputDir, "probe.png")); err != nil {
			return err
		}

		var path string
		body := map[string]any{}
		presentationID := ""
		if renderMode {
			path = "/open-apis/slides_ai/v1/slide_image/render"
			body["content"] = content
		} else {
			if len(slideIDs) > 0 {
				body["slide_ids"] = slideIDs
			}
			if len(slideNumbers) > 0 {
				body["slide_numbers"] = slideNumbers
			}
		}

		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			var steps []map[string]any
			dryBody := body
			if renderMode {
				dryBody = map[string]any{"content": fmt.Sprintf("<xml 已省略，长度 %d>", len(content))}
			} else {
				pres, wikiStep, err := slidesDryRunPresentation(args[0])
				if err != nil {
					return err
				}
				if wikiStep != nil {
					steps = append(steps, wikiStep)
				}
				path = client.SlidesPresentationPath(pres, "/slide_images")
			}
			steps = append(steps, map[string]any{"method": "POST", "path": path, "body": dryBody})
			out := map[string]any{"dry_run": true, "steps": steps, "base64_output": "执行时解码写入本地文件，不打印到 stdout"}
			if outputPath != "" {
				out["output"] = outputPath
			} else {
				out["output_dir"] = outputDir
			}
			return printJSON(out)
		}

		if err := config.Validate(); err != nil {
			return err
		}
		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)
		if !renderMode {
			id, err := resolvePresentationArg(args[0], userAccessToken)
			if err != nil {
				return err
			}
			presentationID = id
			path = client.SlidesPresentationPath(presentationID, "/slide_images")
		}

		action := "获取 slides 截图"
		if renderMode {
			action = "渲染 slide XML 截图"
		}
		data, err := client.SlidesCall(action, "POST", path, nil, body, userAccessToken)
		if err != nil {
			if len(slideNumbers) > 0 {
				return fmt.Errorf("%w\n提示：确认页码在该演示文稿中存在，或改用 --slide-id", err)
			}
			return err
		}

		var items []map[string]any
		if renderMode {
			if item := slidesMap(data, "slide_image"); item != nil {
				items = append(items, item)
			}
		} else if list, ok := data["slide_images"].([]any); ok {
			for _, it := range list {
				if m, ok := it.(map[string]any); ok {
					items = append(items, m)
				}
			}
		}
		if len(items) == 0 {
			return fmt.Errorf("%s失败: 响应中没有图片数据", action)
		}
		if outputPath != "" && len(items) != 1 {
			return fmt.Errorf("%s失败: --output 只对应一页，但服务端返回了 %d 张图片", action, len(items))
		}

		saved := make([]map[string]any, 0, len(items))
		for i, item := range items {
			entry, err := saveSlideScreenshot(item, i, presentationID, outputPath, outputDir, outputName)
			if err != nil {
				return err
			}
			saved = append(saved, entry)
		}
		result := map[string]any{"screenshots": saved}
		if presentationID != "" {
			result["xml_presentation_id"] = presentationID
		}
		if outputPath != "" {
			result["output"] = saved[0]["path"]
		} else {
			result["output_dir"] = outputDir
		}
		return printJSON(result)
	},
}

var unsafeScreenshotNameChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// saveSlideScreenshot 解码一张截图并写入本地文件（同名不覆盖，自动加 _2/_3 序号）。
func saveSlideScreenshot(item map[string]any, index int, presentationID, outputPath, outputDir, outputName string) (map[string]any, error) {
	slideID := strings.TrimSpace(slidesString(item, "slide_id"))
	slideNumber, _ := slidesNumber(item["slide_number"])
	format, _ := slidesNumber(item["format"])
	var ext, label string
	switch format {
	case 1:
		ext, label = ".png", "png"
	case 2:
		ext, label = ".jpg", "jpeg"
	default:
		return nil, fmt.Errorf("截图格式 %d 不受支持（slide_id=%s）", format, slideID)
	}
	encoded := strings.TrimSpace(slidesString(item, "data"))
	if encoded == "" {
		return nil, fmt.Errorf("截图数据为空（slide_id=%s）", slideID)
	}
	img, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("解码截图失败（slide_id=%s）: %w", slideID, err)
	}

	var target string
	if outputPath != "" {
		cur := filepath.Ext(outputPath)
		switch {
		case cur == "":
			target = outputPath + ext
		case strings.EqualFold(cur, ext) || (ext == ".jpg" && strings.EqualFold(cur, ".jpeg")):
			target = outputPath
		default:
			// 扩展名与服务端实际格式不符时按实际格式改名，避免写出内容与扩展名不一致的文件
			target = strings.TrimSuffix(outputPath, cur) + ext
		}
	} else {
		base := outputName
		if base == "" {
			switch {
			case presentationID != "" && slideNumber > 0 && slideID != "":
				base = fmt.Sprintf("%s_p%03d_%s", presentationID, slideNumber, slideID)
			case presentationID != "" && slideNumber > 0:
				base = fmt.Sprintf("%s_p%03d", presentationID, slideNumber)
			case presentationID != "" && slideID != "":
				base = presentationID + "_" + slideID
			case slideID != "":
				base = slideID
			case presentationID == "":
				base = "rendered-slide"
			default:
				base = fmt.Sprintf("slide-%d", index+1)
			}
		}
		base = strings.Trim(unsafeScreenshotNameChars.ReplaceAllString(base, "_"), "._-")
		if base == "" {
			base = "slide"
		}
		target = filepath.Join(outputDir, base+ext)
	}
	finalPath, err := writeUniqueFile(target, img)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"slide_id":     slideID,
		"slide_number": slideNumber,
		"format":       label,
		"path":         finalPath,
		"size":         len(img),
	}, nil
}

// writeUniqueFile 写入 path；已存在则依次尝试 _2、_3…（最多 1000 次），返回实际写入路径。
func writeUniqueFile(path string, data []byte) (string, error) {
	if err := safefile.ValidateOutputPath(path); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 0; i < 1000; i++ {
		candidate := path
		if i > 0 {
			candidate = fmt.Sprintf("%s_%d%s", base, i+1, ext)
		}
		if _, err := os.Stat(candidate); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("检查文件 %s 失败: %w", candidate, err)
		}
		if err := safefile.AtomicWriteFile(candidate, data, 0o644); err != nil {
			return "", fmt.Errorf("写入 %s 失败: %w", candidate, err)
		}
		return candidate, nil
	}
	return "", fmt.Errorf("写入 %s 失败: 同名文件过多", path)
}

func mustStringSlice(cmd *cobra.Command, name string) []string {
	v, _ := cmd.Flags().GetStringSlice(name)
	return v
}

func normalizeSlideIDList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func normalizeSlideNumberList(values []int) ([]int, error) {
	out := make([]int, 0, len(values))
	seen := map[int]bool{}
	for _, n := range values {
		if n < 1 {
			return nil, clierr.Usagef("--slide-number 必须是正整数（从 1 开始）")
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out, nil
}

func init() {
	slidesCmd.AddCommand(slidesScreenshotCmd)
	slidesScreenshotCmd.Flags().StringSlice("slide-id", nil, "页面 slide_id（可重复或逗号分隔，每次最多 10 页）")
	slidesScreenshotCmd.Flags().IntSlice("slide-number", nil, "页码，从 1 开始（可重复或逗号分隔，每次最多 10 页）")
	slidesScreenshotCmd.Flags().String("content", "", "直接渲染的 <slide> XML（支持 @file、- 读 stdin）")
	slidesScreenshotCmd.Flags().String("output", "", "单张截图的输出路径（扩展名可省略）")
	slidesScreenshotCmd.Flags().String("output-dir", defaultSlidesScreenshotDir, "截图输出目录")
	slidesScreenshotCmd.Flags().String("output-name", "", "--content 模式下的文件名（不含扩展名）")
	slidesScreenshotCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求，不调用 API")
	slidesScreenshotCmd.Flags().String("user-access-token", "", "User Access Token")
}
