package cmd

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/safefile"
	"github.com/spf13/cobra"
)

// ============================================================================
// 日程描述格式：--description-format text（默认，写 description 纯文本）| markdown（写 description_rich）。
// markdown 模式下 ![alt](本地路径) 会先上传到云空间再改写为图片 URL；本地图片只接受当前目录内的
// 相对路径（对齐官方），路径校验在任何网络请求之前完成。
// ============================================================================

const (
	descriptionFormatText     = "text"
	descriptionFormatMarkdown = "markdown"
)

var markdownImageRe = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]*)\)`)

// descriptionImage Markdown 中引用的一张本地图片
type descriptionImage struct {
	Src      string // 原始写法（如 ./a.png）
	Path     string // 校验后的本地路径
	srcStart int
	srcEnd   int
}

// richDescription 待写入 description_rich 的 Markdown 及其中的本地图片
type richDescription struct {
	Markdown string
	Images   []descriptionImage
}

// validateMeetingOwnerFlag 校验 --meeting-owner-id：须为 ou_ 开头的 open_id，且只在 Bot 身份（应用日历）下生效。
func validateMeetingOwnerFlag(cmd *cobra.Command) (string, error) {
	owner, _ := cmd.Flags().GetString("meeting-owner-id")
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return "", nil
	}
	if !strings.HasPrefix(owner, "ou_") || owner == "ou_" {
		return "", clierr.Usagef("--meeting-owner-id 必须是用户 open_id（ou_ 开头），得到 %q", owner)
	}
	as, _ := cmd.Flags().GetString("as")
	switch strings.ToLower(strings.TrimSpace(as)) {
	case "bot", "tenant", "app":
		return owner, nil
	default:
		return "", clierr.Usagef("--meeting-owner-id 只在 Bot 身份的应用日历上生效，请追加 --as bot")
	}
}

func addDescriptionFormatFlag(c *cobra.Command) {
	c.Flags().String("description-format", descriptionFormatText, "描述格式：text（默认，纯文本 description）| markdown（富文本 description_rich，本地图片自动上传）")
}

// parseRichDescriptionFlags 读取 --description-format：text 返回 nil；markdown 校验后返回待写入的富文本。
func parseRichDescriptionFlags(cmd *cobra.Command, description string) (*richDescription, error) {
	format, _ := cmd.Flags().GetString("description-format")
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", descriptionFormatText:
		return nil, nil
	case descriptionFormatMarkdown:
		if strings.TrimSpace(description) == "" {
			return nil, clierr.Usagef("--description-format markdown 需要同时传 --description")
		}
		return prepareRichDescription(description)
	default:
		return nil, clierr.Usagef("--description-format 仅支持 text | markdown，得到 %q", format)
	}
}

// prepareRichDescription 找出 Markdown 中的本地图片并校验路径（不联网）
func prepareRichDescription(md string) (*richDescription, error) {
	rd := &richDescription{Markdown: md}
	for _, m := range markdownImageRe.FindAllStringSubmatchIndex(md, -1) {
		srcStart, srcEnd := m[4], m[5]
		raw := md[srcStart:srcEnd]
		src := strings.TrimSpace(raw)
		if !isLocalImageSrc(src) {
			continue
		}
		path, err := validateDescriptionImagePath(src)
		if err != nil {
			return nil, err
		}
		// 去掉 src 两侧空白后的精确位置，替换时保留原有空白
		lead := len(raw) - len(strings.TrimLeft(raw, " \t"))
		rd.Images = append(rd.Images, descriptionImage{Src: src, Path: path, srcStart: srcStart + lead, srcEnd: srcStart + lead + len(src)})
	}
	return rd, nil
}

// isLocalImageSrc 判断图片地址是否为本地文件（http/https/data 等远端地址原样保留）
func isLocalImageSrc(src string) bool {
	if src == "" {
		return false
	}
	lower := strings.ToLower(src)
	switch {
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"), strings.HasPrefix(lower, "data:"):
		return false
	case strings.HasPrefix(lower, "file://"):
		return true
	}
	return !strings.Contains(src, "://")
}

// validateDescriptionImagePath 本地图片必须是当前目录内的相对路径、存在且不在敏感目录（用法错误 exit 2）
func validateDescriptionImagePath(src string) (string, error) {
	p := src
	if strings.HasPrefix(strings.ToLower(p), "file://") {
		if u, err := url.Parse(p); err == nil && u.Path != "" {
			p = u.Path
		}
	}
	if decoded, err := url.PathUnescape(p); err == nil {
		p = decoded
	}
	if filepath.IsAbs(p) {
		return "", clierr.Usagef("--description 图片 %q 必须是当前目录内的相对路径（如 ./images/a.png），不接受绝对路径", src)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", clierr.Usagef("--description 图片 %q 无法确定当前目录: %v", src, err)
	}
	within, err := safefile.IsWithinResolved(p, cwd)
	if err != nil || !within {
		return "", clierr.Usagef("--description 图片 %q 不在当前目录内（先 cd 到图片所在目录，或改用已上传的图片 URL）", src)
	}
	if _, err := safefile.StatInputFile(p); err != nil {
		return "", clierr.Usagef("--description 图片 %q 无法读取: %v", src, err)
	}
	return p, nil
}

// rewrite 依次把本地图片地址替换为 urls[i]
func (rd *richDescription) rewrite(urls []string) string {
	if len(rd.Images) == 0 {
		return rd.Markdown
	}
	var b strings.Builder
	last := 0
	for i, img := range rd.Images {
		b.WriteString(rd.Markdown[last:img.srcStart])
		b.WriteString(urls[i])
		last = img.srcEnd
	}
	b.WriteString(rd.Markdown[last:])
	return b.String()
}

// preview dry-run 展示的 description_rich：本地图片换成占位符
func (rd *richDescription) preview() string {
	urls := make([]string, len(rd.Images))
	for i, img := range rd.Images {
		urls[i] = "<上传后的图片 URL: " + img.Src + ">"
	}
	return rd.rewrite(urls)
}

// uploadSteps dry-run 中列出的图片上传步骤
func (rd *richDescription) uploadSteps(calendarID string) []dryRunStep {
	var steps []dryRunStep
	for _, img := range rd.Images {
		steps = append(steps, dryRunStep{
			Method: "POST", URL: "/open-apis/drive/v1/medias/upload_all",
			Desc: "上传描述图片 " + img.Src + "（parent_type=calendar），改写为图片 URL",
			Body: map[string]any{"file_name": filepath.Base(img.Path), "parent_type": "calendar", "parent_node": calendarID},
		})
	}
	return steps
}

// resolve 上传本地图片（同一路径只传一次）并返回最终 description_rich
func (rd *richDescription) resolve(calendarID, token string) (string, error) {
	cache := map[string]string{}
	urls := make([]string, len(rd.Images))
	for i, img := range rd.Images {
		if u, ok := cache[img.Path]; ok {
			urls[i] = u
			continue
		}
		u, err := client.UploadCalendarDescriptionImage(img.Path, filepath.Base(img.Path), calendarID, token)
		if err != nil {
			return "", err
		}
		cache[img.Path] = u
		urls[i] = u
	}
	return rd.rewrite(urls), nil
}
