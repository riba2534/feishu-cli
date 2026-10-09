// 部分实现改编自 larksuite/cli（MIT License, Copyright (c) 2026 Lark Technologies Pte. Ltd.）
// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/safefile"
)

// doc script parse 的资源预检（对齐官方 docs +script 复用的写入前资源准备逻辑，但只检查、不上传）：
//
//  1. <img path="@..."/> 与 <source path="@..."/>：路径以 @ 开头，非保留占位标记，文件存在、为非空普通文件且可读；
//     图片还须能解出 BMP/GIF/JPEG/PNG/TIFF/WebP 尺寸。<img href="URL"/> 先做 URL 语法校验，
//     不能与 src/token/img_key/url 同用。img 与 source 共用序号（occurrence）。
//  2. <whiteboard type="svg|mermaid|plantuml" path="@..."/>（或标签体为 @路径）：扩展名匹配且文件可读。
//  3. <html5-block path="@x.html"/>：.html 文件可读；data 属性保留给 SDK，data-ref 需要 reference_map（parse 无此参数）。
//
// 以上任一项失败时整体返回一条 resource_preflight_failed（与官方一致，只报第一个问题）；
// 全部通过后再对 <img href> 逐个做远程可用性探测，同一原因的失败合并为一条并列出 image_indices。
//
// 相对路径先按当前目录解析；仅文件不存在且内容来自 @文件 时，回退到该 XML 文件所在目录（同名以 cwd 为准）。

type docScriptRemoteImage struct {
	occurrence int
	url        string
}

// docScriptResourceError 是资源预检失败（msg 为原因，hint 为可选的修复建议）。
type docScriptResourceError struct {
	msg  string
	hint string
}

func (e *docScriptResourceError) Error() string { return e.msg }

func docScriptResourceErrorf(format string, a ...any) error {
	return &docScriptResourceError{msg: fmt.Sprintf(format, a...)}
}

var (
	docScriptWhiteboardPattern = regexp.MustCompile(`(?is)<whiteboard\b[^>]*(?:/>|>.*?</whiteboard>)`)
	docScriptHTML5BlockPattern = regexp.MustCompile(`(?is)<html5-block\b[^>]*?(?:/>|>(.*?)</html5-block>)`)
)

// docScriptProbeRemoteImage 供测试替换远程探测。
var docScriptProbeRemoteImage = func(rawURL string) error {
	return client.ProbeRemoteImage(client.Context(), rawURL)
}

func docScriptResourceDiagnostics(content, contentPath string) []docScriptDiagnostic {
	remotes, err := preflightDocScriptLocalResources(content, contentPath)
	if err != nil {
		diagnostic := docScriptDiagnostic{Severity: docScriptDiagnosticError, Code: docScriptCodeResourceCheck, Msg: err.Error()}
		var resErr *docScriptResourceError
		if errors.As(err, &resErr) {
			diagnostic.Suggested = resErr.hint
		}
		return []docScriptDiagnostic{diagnostic}
	}
	diagnostics := make([]docScriptDiagnostic, 0)
	groups := make(map[string]int)
	for _, remote := range remotes {
		probeErr := docScriptProbeRemoteImage(remote.url)
		if probeErr == nil {
			continue
		}
		diagnostic := docScriptRemoteImageDiagnostic(probeErr, remote.occurrence)
		key := diagnostic.Code + "\x00" + diagnostic.Msg + "\x00" + diagnostic.Suggested
		if index, ok := groups[key]; ok {
			diagnostics[index].ImageIndices = append(diagnostics[index].ImageIndices, remote.occurrence)
			continue
		}
		groups[key] = len(diagnostics)
		diagnostics = append(diagnostics, diagnostic)
	}
	return diagnostics
}

func docScriptRemoteImageDiagnostic(err error, occurrence int) docScriptDiagnostic {
	diagnostic := docScriptDiagnostic{
		Severity:     docScriptDiagnosticError,
		Code:         docScriptCodeImagePreflight,
		ImageIndices: []int{occurrence},
		Msg:          err.Error(),
		Suggested:    "把这些图片下载到草稿工作区，再把 href 改为 <img path=\"@相对路径\"/>。",
	}
	var probeErr *client.RemoteImageProbeError
	if !errors.As(err, &probeErr) {
		return diagnostic
	}
	diagnostic.Msg = probeErr.Reason
	switch probeErr.Kind {
	case client.RemoteImageSourceDisallowed:
		diagnostic.Code = docScriptCodeImageSource
	case client.RemoteImageTooLarge:
		diagnostic.Code = docScriptCodeImageTooLarge
		diagnostic.Suggested = "把这些图片压缩到 20MiB 以下并保存到草稿工作区，改用 <img path=\"@相对路径\"/>。"
	case client.RemoteImageFormat:
		diagnostic.Code = docScriptCodeImageFormat
		diagnostic.Suggested = "把这些图片转换为 BMP、GIF、JPEG、PNG、TIFF 或 WebP 并保存到草稿工作区，改用 <img path=\"@相对路径\"/>。"
	case client.RemoteImageUnavailable:
		diagnostic.Code = docScriptCodeImageMissing
		diagnostic.Suggested = "确认这些图片 URL 可公开访问，或把图片下载到草稿工作区并改用 <img path=\"@相对路径\"/>。"
	}
	return diagnostic
}

// docScriptHasRemoteImages 判断内容中是否有需要远程探测的 <img href>（dry-run 用于声明 network）。
func docScriptHasRemoteImages(content string) bool {
	remotes, err := preflightDocScriptImageTags(content, "", false)
	return err == nil && len(remotes) > 0
}

func preflightDocScriptLocalResources(content, contentPath string) ([]docScriptRemoteImage, error) {
	remotes, err := preflightDocScriptImageTags(content, contentPath, true)
	if err != nil {
		return nil, err
	}
	if err := preflightDocScriptWhiteboards(content, contentPath); err != nil {
		return nil, err
	}
	if err := preflightDocScriptHTML5Blocks(content, contentPath); err != nil {
		return nil, err
	}
	return remotes, nil
}

// preflightDocScriptImageTags 扫描 <img>/<source> 起始标签（跳过注释与 CDATA）；checkFiles=false 时只收集远程图片。
func preflightDocScriptImageTags(content, contentPath string, checkFiles bool) ([]docScriptRemoteImage, error) {
	if !strings.Contains(content, "<img") && !strings.Contains(content, "<source") {
		return nil, nil
	}
	var remotes []docScriptRemoteImage
	occurrence := 0
	for i := 0; i < len(content); {
		switch {
		case strings.HasPrefix(content[i:], "<!--"):
			end := strings.Index(content[i+4:], "-->")
			if end < 0 {
				return remotes, nil
			}
			i += 4 + end + 3
			continue
		case strings.HasPrefix(content[i:], "<![CDATA["):
			end := strings.Index(content[i+9:], "]]>")
			if end < 0 {
				return remotes, nil
			}
			i += 9 + end + 3
			continue
		}
		name := docScriptResourceTagNameAt(content, i)
		if name == "" {
			i++
			continue
		}
		end := docScriptStartTagEnd(content, i)
		if end < 0 {
			return nil, docScriptResourceErrorf("<%s> 资源标签未闭合", name)
		}
		raw := content[i:end]
		i = end
		attrs, err := parseDocScriptResourceTag(raw, name)
		if err != nil {
			return nil, docScriptResourceErrorf("<%s> 资源标签非法: %v", name, err)
		}
		pathValue, hasPath := attrs["path"]
		if !hasPath {
			href, hasHref := attrs["href"]
			if name != "img" || !hasHref {
				continue
			}
			occurrence++
			for _, conflict := range []string{"src", "token", "img_key", "img-key", "url"} {
				if _, ok := attrs[conflict]; ok {
					return nil, docScriptResourceErrorf("<img> 的 href 不能与 %s 同时使用", conflict)
				}
			}
			u, err := url.Parse(strings.TrimSpace(href))
			if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || strings.TrimSpace(u.Hostname()) == "" || u.User != nil {
				return nil, docScriptResourceErrorf("远程图片 #%d 的 href 必须是不带用户名密码的绝对 HTTP(S) URL", occurrence)
			}
			remotes = append(remotes, docScriptRemoteImage{occurrence: occurrence, url: u.String()})
			continue
		}
		occurrence++
		if !checkFiles {
			continue
		}
		for _, conflict := range []string{"src", "href", "token", "img_key", "img-key", "url"} {
			if _, ok := attrs[conflict]; ok {
				return nil, docScriptResourceErrorf("<%s> 的本地 path 不能与 %s 同时使用", name, conflict)
			}
		}
		kind := "图片"
		if name == "source" {
			kind = "附件"
		}
		if err := checkDocScriptLocalFile(kind, occurrence, pathValue, contentPath, name == "img"); err != nil {
			return nil, err
		}
		if fileName, ok := attrs["name"]; ok && name == "source" {
			fileName = strings.TrimSpace(fileName)
			if fileName == "" || fileName == "." || fileName == ".." || strings.ContainsAny(fileName, `/\`) {
				return nil, docScriptResourceErrorf("<source> 的 name 必须是不含路径分隔符的非空文件名")
			}
		}
	}
	return remotes, nil
}

func docScriptResourceTagNameAt(content string, index int) string {
	for _, name := range []string{"img", "source"} {
		prefix := "<" + name
		if !strings.HasPrefix(content[index:], prefix) {
			continue
		}
		next := index + len(prefix)
		if next >= len(content) || strings.IndexByte(">/ \t\r\n", content[next]) >= 0 {
			return name
		}
	}
	return ""
}

func docScriptStartTagEnd(content string, start int) int {
	var quote byte
	for i := start + 1; i < len(content); i++ {
		if quote != 0 {
			if content[i] == quote {
				quote = 0
			}
			continue
		}
		switch content[i] {
		case '\'', '"':
			quote = content[i]
		case '>':
			return i + 1
		}
	}
	return -1
}

// parseDocScriptResourceTag 用 encoding/xml 解析单个起始标签的属性（属性名小写；重复属性报错）。
// href 中未转义的 & 先转义，兼容直接粘贴的 URL 查询串。
func parseDocScriptResourceTag(raw, expected string) (map[string]string, error) {
	raw = escapeDocScriptBareAmpersands(raw)
	decoder := xml.NewDecoder(strings.NewReader(raw))
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local != expected {
			return nil, fmt.Errorf("应为 <%s>，实际为 <%s>", expected, start.Name.Local)
		}
		attrs := make(map[string]string, len(start.Attr))
		for _, attr := range start.Attr {
			key := strings.ToLower(strings.TrimSpace(attr.Name.Local))
			if _, exists := attrs[key]; exists {
				return nil, fmt.Errorf("属性 %q 重复", key)
			}
			attrs[key] = attr.Value
		}
		return attrs, nil
	}
}

var docScriptEntityPattern = regexp.MustCompile(`^&(?:amp|lt|gt|quot|apos|#[0-9]+|#[xX][0-9a-fA-F]+);`)

func escapeDocScriptBareAmpersands(raw string) string {
	if !strings.Contains(raw, "&") {
		return raw
	}
	var out strings.Builder
	for i := 0; i < len(raw); i++ {
		if raw[i] == '&' && !docScriptEntityPattern.MatchString(raw[i:]) {
			out.WriteString("&amp;")
			continue
		}
		out.WriteByte(raw[i])
	}
	return out.String()
}

// statDocScriptResource 先按 cwd 解析；仅相对路径不存在且内容来自 @文件 时回退到源 XML 所在目录。
func statDocScriptResource(path, contentPath string) (string, os.FileInfo, error) {
	if err := safefile.ValidateInputPath(path); err != nil {
		return path, nil, err
	}
	info, err := os.Stat(path)
	if err == nil || !errors.Is(err, fs.ErrNotExist) || filepath.IsAbs(path) || contentPath == "" {
		return path, info, err
	}
	candidate := filepath.Join(filepath.Dir(contentPath), path)
	if candidate == filepath.Clean(path) {
		return path, info, err
	}
	if verr := safefile.ValidateInputPath(candidate); verr != nil {
		return candidate, nil, verr
	}
	info, err = os.Stat(candidate)
	return candidate, info, err
}

func checkDocScriptLocalFile(kind string, occurrence int, pathValue, contentPath string, isImage bool) error {
	pathValue = strings.TrimSpace(pathValue)
	if !strings.HasPrefix(pathValue, "@") {
		return docScriptResourceErrorf("本地%s #%d: path 必须以 @ 开头（如 @./img.png）", kind, occurrence)
	}
	if strings.HasPrefix(pathValue, "@lcli_img_") || strings.HasPrefix(pathValue, "@lcli_file_") {
		return docScriptResourceErrorf("本地%s #%d: path 使用了 CLI 保留的占位标记", kind, occurrence)
	}
	filePath := strings.TrimSpace(strings.TrimPrefix(pathValue, "@"))
	if filePath == "" {
		return docScriptResourceErrorf("本地%s #%d: @ 后面的路径不能为空", kind, occurrence)
	}
	resolved, info, err := statDocScriptResource(filePath, contentPath)
	if err != nil {
		return docScriptResourceErrorf("本地%s #%d: 文件不存在或路径不安全（%s）: %v", kind, occurrence, filePath, err)
	}
	if !info.Mode().IsRegular() {
		return docScriptResourceErrorf("本地%s #%d: path 必须指向普通文件（%s）", kind, occurrence, filePath)
	}
	if info.Size() <= 0 {
		return docScriptResourceErrorf("本地%s #%d: 文件不能为空（%s）", kind, occurrence, filePath)
	}
	file, err := os.Open(resolved)
	if err != nil {
		return docScriptResourceErrorf("本地%s #%d: 文件不可读（%s）: %v", kind, occurrence, filePath, err)
	}
	defer file.Close()
	if isImage {
		cfg, _, err := image.DecodeConfig(file)
		if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
			return docScriptResourceErrorf("本地%s #%d: 不是支持的 BMP、GIF、JPEG、PNG、TIFF 或 WebP 图片（%s）", kind, occurrence, filePath)
		}
	}
	return nil
}

func preflightDocScriptWhiteboards(content, contentPath string) error {
	if !strings.Contains(content, "<whiteboard") {
		return nil
	}
	for _, raw := range docScriptWhiteboardPattern.FindAllString(content, -1) {
		startEnd := docScriptStartTagEnd(raw, 0)
		if startEnd < 0 {
			continue
		}
		startRaw := raw[:startEnd]
		body := ""
		if len(raw) > startEnd {
			// 正则保证非自闭合时以 </whiteboard>（不区分大小写）结尾
			body = raw[startEnd : len(raw)-len("</whiteboard>")]
		}
		attrs, err := parseDocScriptResourceTag(startRaw, "whiteboard")
		if err != nil {
			return docScriptResourceErrorf("<whiteboard> 标签非法: %v", err)
		}
		pathValue, hasPath := attrs["path"]
		bodyPath, hasBodyPath := docScriptWhiteboardBodyPath(body)
		if !hasPath && !hasBodyPath {
			continue
		}
		if hasPath && strings.TrimSpace(body) != "" {
			return docScriptResourceErrorf("<whiteboard> 不能同时使用 path 和标签内内容")
		}
		typRaw := strings.ToLower(strings.TrimSpace(attrs["type"]))
		exts, ok := map[string][]string{
			"svg":      {".svg"},
			"mermaid":  {".mermaid", ".mmd"},
			"plantuml": {".plantuml", ".puml", ".pu", ".uml"},
		}[typRaw]
		if !ok {
			return docScriptResourceErrorf(`<whiteboard> 从文件导入时 type 必须是 "svg"、"mermaid" 或 "plantuml"（当前 %q）`, attrs["type"])
		}
		if hasBodyPath {
			pathValue = bodyPath
		}
		pathRaw := strings.TrimSpace(pathValue)
		if !strings.HasPrefix(pathRaw, "@") {
			return docScriptResourceErrorf("<whiteboard> %s 的 path %q 必须以 @ 开头（如 @diagram%s）", typRaw, pathValue, exts[0])
		}
		filePath := strings.TrimSpace(strings.TrimPrefix(pathRaw, "@"))
		if filePath == "" {
			return docScriptResourceErrorf("<whiteboard> %s 的 path 在 @ 后不能为空", typRaw)
		}
		ext := strings.ToLower(filepath.Ext(filePath))
		matched := false
		for _, allowed := range exts {
			if ext == allowed {
				matched = true
			}
		}
		if !matched {
			return docScriptResourceErrorf("<whiteboard> %s 的 path %q 必须指向 %s 文件", typRaw, pathValue, strings.Join(exts, "、"))
		}
		if err := checkDocScriptReadable(filePath, contentPath); err != nil {
			return docScriptResourceErrorf("<whiteboard> %s 的文件 %q 无法读取: %v", typRaw, filePath, err)
		}
	}
	return nil
}

func docScriptWhiteboardBodyPath(body string) (string, bool) {
	trimmed := strings.TrimSpace(body)
	if !strings.HasPrefix(trimmed, "@") || strings.HasPrefix(trimmed, "@@") || strings.ContainsAny(trimmed, "\r\n") {
		return "", false
	}
	return trimmed, true
}

func preflightDocScriptHTML5Blocks(content, contentPath string) error {
	if !strings.Contains(content, "<html5-block") {
		return nil
	}
	for _, match := range docScriptHTML5BlockPattern.FindAllStringSubmatch(content, -1) {
		if strings.TrimSpace(match[1]) != "" {
			return &docScriptResourceError{
				msg:  "html5-block 的 HTML 必须通过 path=\"@x.html\" 加载，不能写在 <html5-block> 标签体内",
				hint: "把完整单文件 HTML 保存为 .html 文件，改写为 <html5-block path=\"@./widget.html\"/>。",
			}
		}
		startEnd := docScriptStartTagEnd(match[0], 0)
		if startEnd < 0 {
			continue
		}
		attrs, err := parseDocScriptResourceTag(match[0][:startEnd], "html5-block")
		if err != nil {
			return docScriptResourceErrorf("<html5-block> 标签非法: %v", err)
		}
		if _, ok := attrs["data"]; ok {
			return docScriptResourceErrorf(`html5-block 的 data 属性保留给 CLI 内部使用，请改用 path="@x.html"`)
		}
		pathValue, hasPath := attrs["path"]
		_, hasDataRef := attrs["data-ref"]
		switch {
		case hasPath && hasDataRef:
			return docScriptResourceErrorf("html5-block 不能同时使用 path 和 data-ref")
		case hasDataRef:
			return docScriptResourceErrorf(`html5-block 的 data-ref 需要配合 reference_map，parse 无法校验；草稿中请改用 path="@x.html"`)
		case !hasPath:
			return docScriptResourceErrorf(`html5-block 需要 path="@x.html"`)
		}
		pathRaw := strings.TrimSpace(pathValue)
		if !strings.HasPrefix(pathRaw, "@") {
			return docScriptResourceErrorf("html5-block 的 path %q 必须以 @ 开头（如 @widget.html）", pathValue)
		}
		filePath := strings.TrimSpace(strings.TrimPrefix(pathRaw, "@"))
		if filePath == "" {
			return docScriptResourceErrorf("html5-block 的 path 在 @ 后不能为空")
		}
		if strings.ToLower(filepath.Ext(filePath)) != ".html" {
			return docScriptResourceErrorf("html5-block 的 path %q 必须指向 .html 文件", pathValue)
		}
		if err := checkDocScriptReadable(filePath, contentPath); err != nil {
			return docScriptResourceErrorf("html5-block 的文件 %q 无法读取: %v", filePath, err)
		}
	}
	return nil
}

func checkDocScriptReadable(filePath, contentPath string) error {
	resolved, info, err := statDocScriptResource(filePath, contentPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s 是目录", resolved)
	}
	file, err := os.Open(resolved)
	if err != nil {
		return err
	}
	return file.Close()
}
