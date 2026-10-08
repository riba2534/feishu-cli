package cmd

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/safefile"
	"github.com/spf13/cobra"
)

// Slides 编辑命令（add-slide / delete-slide / replace-slide / update-slide / screenshot / create --slide）共用的辅助函数。
// 契约细节对齐官方 lark-cli shortcuts/slides（helpers.go、slides_shared.go、slides_lint_*.go、slides_update_slide.go），
// 这些都是官方实测踩过的坑，改动前先读对应注释。

const (
	// slidesLintBlockedCode 服务端 XML lint 拒绝写入的业务码（官方 slides_lint_error.go）。
	slidesLintBlockedCode = 4000153
	// slidesInvalidParamCode slides 编辑接口的兜底参数错误码。
	slidesInvalidParamCode = 3350001
	// slidesLintBodyKey lint 开关必须放在请求体：放 query 会被网关当未声明参数丢弃，
	// 请求照样成功但服务端读到"未要求 lint"（官方 slides_lint_param.go 实测）。
	slidesLintBodyKey = "lint_xml"
	// maxSlidesPerCreate slides create 一次最多带的页数（官方 slides_create.go）。
	maxSlidesPerCreate = 10
	// maxSlidesReplaceParts replace-slide 单次 parts 上限（API 目录"最少 1 条，最多 200 条"）。
	maxSlidesReplaceParts = 200
	// maxSlidesPerScreenshot 截图接口单次最多页数。
	maxSlidesPerScreenshot = 10
)

func slidesPresentationArgOptions(userAccessToken string) resourceArgOptions {
	return resourceArgOptions{
		ArgName:         "<presentation>",
		DefaultType:     client.ResourceTypeSlides,
		Allowed:         []string{client.ResourceTypeSlides},
		ResolveWiki:     true,
		UserAccessToken: userAccessToken,
	}
}

// resolvePresentationArg 把 <xml_presentation_id | /slides/ URL | /wiki/ URL> 解析为 xml_presentation_id。
// wiki 走 node_by_token 解包并校验 obj_type=slides；docx 等其他 URL 在本地拒绝，避免误导性的 404。
func resolvePresentationArg(raw, userAccessToken string) (string, error) {
	opts := slidesPresentationArgOptions(userAccessToken)
	// 先离线解析：输入格式错误按用法错误（exit 2）返回，不发请求
	if _, err := parseResourceArg(raw, opts); err != nil {
		return "", clierr.Usage(err)
	}
	res, err := resolveResourceArg(raw, opts)
	if err != nil {
		return "", err
	}
	noteWikiResolved(res)
	return res.Token, nil
}

// parsePresentationArgOffline 纯离线解析（dry-run 用）：wiki URL 不联网解包，isWiki=true 时 token 是 wiki 节点 token。
func parsePresentationArgOffline(raw string) (token string, isWiki bool, err error) {
	res, err := parseResourceArg(raw, slidesPresentationArgOptions(""))
	if err != nil {
		return "", false, clierr.Usage(err)
	}
	if res.InputType == client.ResourceTypeWiki {
		return res.InputToken, true, nil
	}
	return res.Token, false, nil
}

// slidesDryRunPresentation 返回 dry-run 中展示的演示文稿 ID：wiki 输入不联网解包，用占位符。
func slidesDryRunPresentation(raw string) (string, map[string]any, error) {
	token, isWiki, err := parsePresentationArgOffline(raw)
	if err != nil {
		return "", nil, err
	}
	if isWiki {
		return "<resolved_slides_token>", map[string]any{
			"method": "GET",
			"path":   client.WikiNodeByTokenPath,
			"params": map[string]any{"token": token},
			"desc":   "解析 wiki 节点为底层演示文稿（obj_type 必须为 slides）",
		}, nil
	}
	return token, nil, nil
}

// readSlidesFlagInput 读取支持 @file / - (stdin) 的字符串 flag：
//   - "-"       → 读 stdin
//   - "@path"   → 读本地文件（拒绝敏感目录）
//   - "@@text"  → 字面量 "@text"
//   - 其他      → 原样
//
// 用 @file 传 XML/JSON 可以绕开 shell 多层转义——官方统计这是 3350001 的头号来源。
func readSlidesFlagInput(cmd *cobra.Command, name string) (string, error) {
	raw, _ := cmd.Flags().GetString(name)
	return resolveSlidesInputValue(raw, "--"+name)
}

var slidesStdin io.Reader = os.Stdin

func resolveSlidesInputValue(raw, flag string) (string, error) {
	switch {
	case raw == "-":
		data, err := io.ReadAll(slidesStdin)
		if err != nil {
			return "", fmt.Errorf("%s 读取标准输入失败: %w", flag, err)
		}
		return stripUTF8BOM(string(data)), nil
	case strings.HasPrefix(raw, "@@"):
		return raw[1:], nil
	case strings.HasPrefix(raw, "@"):
		path := strings.TrimSpace(raw[1:])
		if path == "" {
			return "", clierr.Usagef("%s: @ 后面的文件路径不能为空", flag)
		}
		data, err := readLocalInputFile(path)
		if err != nil {
			if clierr.HasKind(err, clierr.KindUsage) {
				return "", err
			}
			return "", clierr.Usagef("%s: 读取文件 %s 失败: %v", flag, path, err)
		}
		return stripUTF8BOM(string(data)), nil
	default:
		return raw, nil
	}
}

func stripUTF8BOM(s string) string {
	return strings.TrimPrefix(s, "\ufeff")
}

// slidesLintFlag 给写页面的命令注册 --no-lint。
func addSlidesNoLintFlag(cmd *cobra.Command) {
	cmd.Flags().Bool("no-lint", false, "跳过服务端 XML lint 直接提交（默认服务端逐页 lint，有 error 级问题拒绝写入）")
}

// withSlidesLint 把 lint 开关写进请求体（总是显式下发 true/false，不依赖服务端默认值）。
func withSlidesLint(cmd *cobra.Command, body map[string]any) map[string]any {
	if body == nil {
		body = map[string]any{}
	}
	noLint, _ := cmd.Flags().GetBool("no-lint")
	body[slidesLintBodyKey] = !noLint
	return body
}

// enrichSlidesWriteError 给 slides 写接口的错误补充可执行的修复建议，不改变错误链（HasAPICode / AsAPIError 仍可用）。
func enrichSlidesWriteError(err error, extraHint string) error {
	if err == nil {
		return nil
	}
	var hints []string
	switch {
	case client.HasAPICode(err, slidesLintBlockedCode):
		head := "XML lint 拦截"
		if apiErr, ok := client.AsAPIError(err); ok {
			var report struct {
				Summary struct {
					ErrorCount int `json:"error_count"`
				} `json:"summary"`
				SchemaIssues string `json:"schema_issues"`
			}
			if json.Unmarshal([]byte(strings.TrimSpace(apiErr.Msg)), &report) == nil {
				if report.Summary.ErrorCount > 0 {
					head = fmt.Sprintf("%s：%d 个 error 级问题", head, report.Summary.ErrorCount)
				}
				if report.SchemaIssues != "" {
					head += "（msg 中另含 schema_issues）"
				}
			}
		}
		hints = append(hints, head+"。该页未写入：按 msg 中的报告修正 error 级问题后重试；确认 lint 误判且页面必须原样提交时加 --no-lint")
	case client.HasAPICode(err, slidesInvalidParamCode):
		hints = append(hints, "3350001 常见原因：(1) block_id / slide_id 不在当前页（先 slides get --slide-id 回读最新 XML）；(2) XML 结构非法或用了不支持的元素（<shape> 缺 <content/>、<image> 应为 <img>）；(3) 元素坐标超出页面 960×540")
	}
	if extraHint != "" {
		hints = append(hints, extraHint)
	}
	if len(hints) == 0 {
		return err
	}
	return fmt.Errorf("%w\n提示：%s", err, strings.Join(hints, "\n提示："))
}

// validateCompleteSlideXML 校验 content 恰好是一个完整的 <slide> 文档：单根、根外无文本、元素闭合、无 <?xml?> 声明。
// 官方实测：带 <?xml ...?> 前导的页面在 .../slide 上被拒（4001000），且此时演示文稿已创建，留下半成品。
func validateCompleteSlideXML(content string) error {
	dec := xml.NewDecoder(strings.NewReader(content))
	depth := 0
	seenRoot := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if depth == 0 {
				if seenRoot {
					return fmt.Errorf("存在多个根元素")
				}
				if t.Name.Local != "slide" {
					return fmt.Errorf("根元素是 <%s>，应为 <slide>", t.Name.Local)
				}
				seenRoot = true
			}
			depth++
		case xml.EndElement:
			depth--
		case xml.ProcInst:
			return fmt.Errorf("不支持 <?%s ...?> 声明，删除后让文档以 <slide> 开头", t.Target)
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(t)) != "" {
				return fmt.Errorf("根元素外存在非空白文本")
			}
		}
	}
	if !seenRoot {
		return fmt.Errorf("缺少根元素 <slide>")
	}
	if depth != 0 {
		return fmt.Errorf("存在未闭合的 XML 元素")
	}
	return nil
}

// checkSlideRoot 校验 content 恰好是一个 <slide> 元素并返回根上的 id 属性（没有则为空）。
func checkSlideRoot(content string) (string, error) {
	dec := xml.NewDecoder(strings.NewReader(content))
	depth := 0
	seenRoot := false
	rootID := ""
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("不是合法的 XML: %v", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if depth == 0 {
				if seenRoot {
					return "", fmt.Errorf("只能有一个 <slide> 根元素，发现第二个根 <%s>", t.Name.Local)
				}
				if t.Name.Local != "slide" {
					return "", fmt.Errorf("根元素必须是 <slide>，得到 <%s>；只改单个元素请用 slides replace-slide", t.Name.Local)
				}
				seenRoot = true
				for _, attr := range t.Attr {
					if attr.Name.Local == "id" && attr.Name.Space == "" {
						rootID = attr.Value
					}
				}
			}
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(t)) != "" {
				return "", fmt.Errorf("<slide> 元素外存在文本: %q", strings.TrimSpace(string(t)))
			}
		}
	}
	if !seenRoot {
		return "", fmt.Errorf("缺少 <slide> 元素")
	}
	return rootID, nil
}

// xmlRootOpenTagRegex 匹配 XML 片段的根开始标签（跳过前导空白、<?xml?> 声明与注释）。
// 分组：1 前缀；2 标签名；3 属性串；4 结束符（"/>" 或 ">"）。
var xmlRootOpenTagRegex = regexp.MustCompile(`(?s)\A(\s*(?:<\?[^?]*(?:\?[^>][^?]*)*\?>\s*)?(?:<!--.*?-->\s*)*)<([A-Za-z_][\w.-]*)((?:\s[^>]*?)?)(/?>)`)

// xmlIDAttrRegex 匹配独立的 id="..." 属性（不会误匹配 data-id / xml:id）。
var xmlIDAttrRegex = regexp.MustCompile(`(?s)(?:^|\s)id\s*=\s*(["'])(.*?)(["'])`)

// ensureXMLRootID 保证片段根元素带 id="want"：没有则注入，值不同则覆盖，相同则原样返回。
// block_replace 要求 replacement 根元素带上被替换块的 id，否则服务端报 3350001（官方实测、文档未写）。
func ensureXMLRootID(fragment, want string) (string, error) {
	m := xmlRootOpenTagRegex.FindStringSubmatchIndex(fragment)
	if m == nil {
		return "", fmt.Errorf("XML 片段中找不到根元素")
	}
	prefix := fragment[m[2]:m[3]]
	tag := fragment[m[4]:m[5]]
	attrs := fragment[m[6]:m[7]]
	closer := fragment[m[8]:m[9]]
	rest := fragment[m[1]:]
	if sub := xmlIDAttrRegex.FindStringSubmatchIndex(attrs); sub != nil {
		if attrs[sub[4]:sub[5]] == want {
			return fragment, nil
		}
		return prefix + "<" + tag + attrs[:sub[4]] + want + attrs[sub[5]:] + closer + rest, nil
	}
	trimmed := strings.TrimRight(attrs, " \t\n\r")
	trailing := attrs[len(trimmed):]
	return prefix + "<" + tag + trimmed + fmt.Sprintf(` id="%s"`, want) + trailing + closer + rest, nil
}

var xmlContentTagRegex = regexp.MustCompile(`<content(?:\s|/|>)`)

// ensureShapeHasContent 给缺 <content/> 的 <shape> 根元素补上空 content（SML 2.0 要求，缺了报 3350001）。
// 只处理根为 <shape> 且主体为空的情况；已有 <p> 等子元素时不擅自改写，交给服务端报错。
func ensureShapeHasContent(fragment string) string {
	m := xmlRootOpenTagRegex.FindStringSubmatchIndex(fragment)
	if m == nil {
		return fragment
	}
	tag := fragment[m[4]:m[5]]
	if tag != "shape" {
		return fragment
	}
	closer := fragment[m[8]:m[9]]
	if closer == "/>" {
		prefix := fragment[m[2]:m[3]]
		attrs := strings.TrimRight(fragment[m[6]:m[7]], " \t\n\r")
		return prefix + "<" + tag + attrs + "><content/></" + tag + ">" + fragment[m[1]:]
	}
	after := fragment[m[1]:]
	if xmlContentTagRegex.MatchString(after) {
		return fragment
	}
	closeTag := "</" + tag + ">"
	idx := strings.Index(after, closeTag)
	if idx < 0 || strings.TrimSpace(after[:idx]) != "" {
		return fragment
	}
	return fragment[:m[1]] + "<content/>" + after
}

var noteIDAttrRegex = regexp.MustCompile(`\s+id\s*=\s*(?:"[^"]*"|'[^']*')`)

// stripSlideNoteID 去掉 <slide> 直接子元素 <note> 上的 id。
// 整页替换时若带着过期的 <note id>（从别的页复制、或页面被重建过），服务端会整页拒绝
// "block is not NoteBlock"；去掉后服务端自动定位本页备注块（官方 slides_update_slide.go）。
func stripSlideNoteID(content string) string {
	dec := xml.NewDecoder(strings.NewReader(content))
	var stack []string
	var prev int64
	var spans [][2]int64
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		cur := dec.InputOffset()
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "note" && len(stack) > 0 && stack[len(stack)-1] == "slide" {
				spans = append(spans, [2]int64{prev, cur})
			}
			stack = append(stack, t.Name.Local)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
		prev = cur
	}
	out := content
	for i := len(spans) - 1; i >= 0; i-- {
		s, e := spans[i][0], spans[i][1]
		out = out[:s] + noteIDAttrRegex.ReplaceAllString(out[s:e], "") + out[e:]
	}
	return out
}

// imgSrcPlaceholderRegex 匹配 <img ... src="@path">（@ 前缀表示本地文件，上传后替换为 file_token）。
var imgSrcPlaceholderRegex = regexp.MustCompile(`(?s)<img\b[^>]*?\bsrc\s*=\s*(["'])@([^"']+)(["'])`)

// extractImagePlaceholderPaths 按首次出现顺序去重返回 <img src="@path"> 中的本地路径。
func extractImagePlaceholderPaths(xmls []string) []string {
	var paths []string
	seen := map[string]bool{}
	for _, x := range xmls {
		for _, m := range imgSrcPlaceholderRegex.FindAllStringSubmatch(x, -1) {
			if m[1] != m[3] {
				continue
			}
			p := strings.TrimSpace(m[2])
			if p == "" || seen[p] {
				continue
			}
			seen[p] = true
			paths = append(paths, p)
		}
	}
	return paths
}

// replaceImagePlaceholders 把 <img src="@path"> 替换为上传得到的 file_token。
func replaceImagePlaceholders(x string, tokens map[string]string) string {
	return imgSrcPlaceholderRegex.ReplaceAllStringFunc(x, func(match string) string {
		sub := imgSrcPlaceholderRegex.FindStringSubmatch(match)
		if len(sub) < 4 || sub[1] != sub[3] {
			return match
		}
		token, ok := tokens[strings.TrimSpace(sub[2])]
		if !ok {
			return match
		}
		return strings.Replace(match, sub[1]+"@"+sub[2]+sub[3], sub[1]+token+sub[3], 1)
	})
}

// validateImagePlaceholderFiles 在任何 API 调用前检查占位图片：存在、普通文件、≤20 MB、不在敏感目录。
// 路径相对当前工作目录解析（不是相对 --slide @file 所在目录）。
func validateImagePlaceholderFiles(flag string, paths []string) error {
	for _, p := range paths {
		desc := fmt.Sprintf(`%s: <img src="@%s">（相对当前目录解析）`, flag, p)
		if err := validateLocalReadPath(p); err != nil {
			return clierr.Usagef("%s: %v", desc, err)
		}
		st, err := os.Stat(p)
		if err != nil {
			if os.IsNotExist(err) {
				return clierr.Usagef("%s: 文件不存在", desc)
			}
			return clierr.Usagef("%s: 无法读取: %v", desc, err)
		}
		if !st.Mode().IsRegular() {
			return clierr.Usagef("%s: 必须是普通文件", desc)
		}
		if st.Size() > maxSlidesMediaUploadSize {
			return clierr.Usagef("%s: 文件 %d 字节超过 slides 图片上传上限 20 MB", desc, st.Size())
		}
	}
	return nil
}

// validateLocalReadPath 复用 readLocalInputFile 的敏感目录检查（不读文件）。
func validateLocalReadPath(path string) error {
	return safefile.ValidateInputPath(path)
}

// uploadSlidesPlaceholders 把占位图片逐个上传到演示文稿，返回 path→file_token 与成功上传数。
func uploadSlidesPlaceholders(presentationID string, paths []string, userAccessToken string) (map[string]string, int, error) {
	tokens := make(map[string]string, len(paths))
	for i, p := range paths {
		token, err := client.UploadSlidesMedia(p, filepath.Base(p), presentationID, userAccessToken)
		if err != nil {
			return tokens, i, fmt.Errorf("上传图片 @%s 失败: %w", p, err)
		}
		tokens[p] = token
	}
	return tokens, len(paths), nil
}

// slidesNumber 把 SlidesCall 返回的数字（json.Number/float64/int）转成 int。
func slidesNumber(v any) (int, bool) {
	switch n := v.(type) {
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			f, ferr := n.Float64()
			if ferr != nil {
				return 0, false
			}
			return int(f), true
		}
		return int(i), true
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	}
	return 0, false
}

// slidesString 安全读取字符串字段。
func slidesString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

// slidesMap 安全读取子对象。
func slidesMap(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}
	sub, _ := m[key].(map[string]any)
	return sub
}

// copySlidesRevision 把响应里的 revision_id 复制到结果（存在才写）。
func copySlidesRevision(dst, data map[string]any) {
	if v, ok := data["revision_id"]; ok {
		if n, ok := slidesNumber(v); ok {
			dst["revision_id"] = n
		}
	}
}

// validateSlidesRevisionID 校验 --revision-id：-1 表示最新；0 服务端报 3350001；< -1 无意义。
func validateSlidesRevisionID(cmd *cobra.Command) (int, error) {
	rev, _ := cmd.Flags().GetInt("revision-id")
	if rev == 0 || rev < -1 {
		return 0, clierr.Usagef("--revision-id 取值 %d 无效：-1 表示最新版本，或传正整数版本号用于乐观锁；0 会被服务端拒绝（3350001）", rev)
	}
	return rev, nil
}
