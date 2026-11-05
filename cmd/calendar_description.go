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
	// 在遮掉代码的副本上匹配：代码块/行内代码里的图片语法只是文本，不上传；遮罩等长，下标可直接用于原文
	for _, m := range markdownImageRe.FindAllStringSubmatchIndex(maskMarkdownCode(md), -1) {
		off, src := imageDestination(md[m[4]:m[5]])
		if !isLocalImageSrc(src) {
			continue
		}
		path, err := validateDescriptionImagePath(src)
		if err != nil {
			return nil, err
		}
		// 只记录路径本身的位置：替换时保留两侧空白、尖括号与 title
		start := m[4] + off
		rd.Images = append(rd.Images, descriptionImage{Src: src, Path: path, srcStart: start, srcEnd: start + len(src)})
	}
	return rd, nil
}

// imageDestination 从 ![alt](...) 括号内取出图片地址，返回其在 raw 中的偏移与内容。
// 支持 <path> 尖括号写法与 "title" / 'title' / (title) 标题；不带标题的路径按原样（允许含空格）。
func imageDestination(raw string) (int, string) {
	off := len(raw) - len(strings.TrimLeft(raw, " \t\n"))
	rest := raw[off:]
	if strings.HasPrefix(rest, "<") {
		if end := strings.IndexByte(rest, '>'); end > 0 {
			return off + 1, rest[1:end]
		}
	}
	rest = strings.TrimRight(rest, " \t\n")
	for i := 0; i < len(rest); i++ {
		if rest[i] != ' ' && rest[i] != '\t' && rest[i] != '\n' {
			continue
		}
		tail := strings.TrimLeft(rest[i:], " \t\n")
		if tail != "" && strings.ContainsRune(`"'(`, rune(tail[0])) {
			return off, rest[:i]
		}
	}
	return off, rest
}

// maskMarkdownCode 把围栏代码块（``` / ~~~）与行内代码 `...` 的内容替换为等长空格（保留换行），
// 使后续正则不会匹配到代码里的语法，同时字节下标与原文一致。
func maskMarkdownCode(md string) string {
	b := []byte(md)
	blank := func(from, to int) {
		for i := from; i < to; i++ {
			if b[i] != '\n' {
				b[i] = ' '
			}
		}
	}
	// 围栏代码块：开栏行最多 3 个空格缩进，闭栏为同字符且不短于开栏；未闭合则到文末
	lineStart := 0
	fenceStart, fenceChar, fenceLen := -1, byte(0), 0
	for lineStart <= len(md) {
		lineEnd := strings.IndexByte(md[lineStart:], '\n')
		if lineEnd < 0 {
			lineEnd = len(md)
		} else {
			lineEnd += lineStart
		}
		line := md[lineStart:lineEnd]
		trimmed := strings.TrimLeft(line, " ")
		indent := len(line) - len(trimmed)
		if indent <= 3 && len(trimmed) >= 3 && (trimmed[0] == '`' || trimmed[0] == '~') {
			c := trimmed[0]
			n := len(trimmed) - len(strings.TrimLeft(trimmed, string(c)))
			switch {
			case fenceStart < 0 && n >= 3:
				fenceStart, fenceChar, fenceLen = lineStart, c, n
			case fenceStart >= 0 && c == fenceChar && n >= fenceLen && strings.TrimSpace(trimmed[n:]) == "":
				blank(fenceStart, lineEnd)
				fenceStart = -1
			}
		}
		if lineEnd == len(md) {
			break
		}
		lineStart = lineEnd + 1
	}
	if fenceStart >= 0 {
		blank(fenceStart, len(md))
	}
	// 行内代码：n 个反引号开头，到下一个恰好 n 个反引号结束；无闭合则按普通文本
	s := string(b)
	for i := 0; i < len(s); {
		if s[i] != '`' {
			i++
			continue
		}
		n := 1
		for i+n < len(s) && s[i+n] == '`' {
			n++
		}
		closed := false
		for j := i + n; j < len(s); {
			if s[j] != '`' {
				j++
				continue
			}
			m := 1
			for j+m < len(s) && s[j+m] == '`' {
				m++
			}
			if m == n {
				blank(i, j+m)
				i, closed = j+m, true
				break
			}
			j += m
		}
		if !closed {
			i += n
		}
	}
	return string(b)
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
