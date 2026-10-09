// 部分实现改编自 larksuite/cli（MIT License, Copyright (c) 2026 Lark Technologies Pte. Ltd.）

package cmd

// docs_ai 写入（doc create / doc content-update）前对本地文件引用的预处理：
//
//   - <html5-block path="@./widget.html"/>：读取本地单文件 HTML，标签改写为 data-ref="html5_N"，
//     HTML 写入请求体 reference_map["html5-block"]["html5_N"].data；
//   - <whiteboard type="svg|mermaid|plantuml" path="@./x.svg"/>（或标签体写 @./x.mmd）：读取本地源文件，
//     展开为内联内容（mermaid / plantuml 做 XML 文本转义，svg 原样）；
//   - --reference-map：结构化 reference_map（内联 JSON、@file 或 - 读 stdin），原样随请求发送；
//     其中 html5-block 条目的 path（@./x.html）会被读取为 data；
//   - 本地图片/附件（<img path>/<source path>/![](@./a.png)）见 doc_content_local_resources.go。
//
// Markdown 格式下只处理围栏代码块以外的标签；路径以 @ 开头，相对当前目录解析；XML 内容来自文件
// （--content-file / --markdown-file）时，当前目录下不存在的相对路径回退到该文件所在目录。
// 本地文件一律经 safefile 校验（拒绝敏感目录；不存在、是目录返回用法错误 exit 2）。
//
// 部分实现改编自 larksuite/cli（MIT License, Copyright (c) 2026 Lark Technologies Pte. Ltd.），
// 对应 shortcuts/doc/html5_block_resources.go 与 doc_resource_path.go。

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/safefile"
)

const html5BlockTag = "html5-block"

// docsAIWriteOptions 描述一次写入内容的来源与校验强度。
type docsAIWriteOptions struct {
	Format     string // markdown | xml
	SourceFile string // 内容来自的本地文件（--content-file / --markdown-file），内联内容为空
	BaseDir    string // Markdown 非 @ 本地图片路径（![](./a.png)）的解析基准目录，默认取 SourceFile 所在目录
	// Strict 按官方 docs +create 严格校验本地图片/附件标签：path 必须以 @ 开头、不能与 src/token 等并用、
	// 图片必须可识别、<source name> 不能含路径分隔符，并把 alt 改名为 caption。
	// doc create 开启；doc content-update 为兼容既有用法保持宽松。
	Strict bool
}

func (o docsAIWriteOptions) markdown() bool { return o.Format == "markdown" }

func (o docsAIWriteOptions) markdownBaseDir() string {
	if o.BaseDir != "" || o.SourceFile == "" {
		return o.BaseDir
	}
	return filepath.Dir(o.SourceFile)
}

// resourceFallbackDir 返回 @ 相对路径在当前目录不存在时的回退目录（仅 XML 内容来自文件时，对齐官方）。
func (o docsAIWriteOptions) resourceFallbackDir() string {
	if o.SourceFile == "" || o.markdown() {
		return ""
	}
	return filepath.Dir(o.SourceFile)
}

// docsAIWriteInput 是预处理后的写入内容。
type docsAIWriteInput struct {
	Content      string
	ReferenceMap map[string]any
	Resources    []*localDocResource
}

// html5RefEntry 是 reference_map["html5-block"] 的条目（data 与 path 二选一）。
type html5RefEntry struct {
	Data   string `json:"data,omitempty"`
	Path   string `json:"path,omitempty"`
	UserID string `json:"user_id,omitempty"`
}

// prepareDocsAIWriteInput 依次处理本地图片/附件、画板本地源文件、html5-block 本地 HTML 与 reference_map（离线）。
func prepareDocsAIWriteInput(content string, refMap map[string]any, opts docsAIWriteOptions) (docsAIWriteInput, error) {
	refMap = cloneReferenceMap(refMap)
	group, err := html5GroupFromReferenceMap(refMap)
	if err != nil {
		return docsAIWriteInput{}, err
	}
	content, resources, err := prepareLocalDocResourcesWith(content, opts)
	if err != nil {
		return docsAIWriteInput{}, err
	}
	if content, err = prepareWhiteboardFileRefs(content, opts); err != nil {
		return docsAIWriteInput{}, err
	}
	if content, group, err = prepareHTML5BlockRefs(content, group, opts); err != nil {
		return docsAIWriteInput{}, err
	}
	if err := resolveHTML5RefPaths(group, opts); err != nil {
		return docsAIWriteInput{}, err
	}
	if len(group) > 0 {
		if refMap == nil {
			refMap = map[string]any{}
		}
		refMap[html5BlockTag] = group
	}
	return docsAIWriteInput{Content: content, ReferenceMap: refMap, Resources: resources}, nil
}

// docWriteStdin 是 --reference-map - 读取的标准输入，测试中可替换。
var docWriteStdin io.Reader = os.Stdin

// readReferenceMapFlag 解析 --reference-map：内联 JSON、@file（相对当前目录）或 - 读 stdin。
// allowEmpty=true 时空值 / null 视为未提供（对齐官方 docs +create）；否则必须是非空 JSON 对象（对齐 docs +update）。
func readReferenceMapFlag(raw string, allowEmpty bool) (map[string]any, error) {
	value := raw
	switch {
	case raw == "-":
		data, err := io.ReadAll(docWriteStdin)
		if err != nil {
			return nil, fmt.Errorf("--reference-map 读取标准输入失败: %w", err)
		}
		value = string(data)
	case strings.HasPrefix(strings.TrimSpace(raw), "@"):
		path := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "@"))
		if path == "" {
			return nil, clierr.Usagef("--reference-map: @ 后面的文件路径不能为空")
		}
		data, err := readLocalInputFile(path)
		if err != nil {
			return nil, err
		}
		value = string(data)
	}
	value = strings.TrimSpace(strings.TrimPrefix(value, "\ufeff"))
	if value == "" || value == "null" {
		if allowEmpty {
			return nil, nil
		}
		return nil, clierr.Usagef("--reference-map 必须是非空 JSON 对象")
	}
	var refMap map[string]any
	if err := json.Unmarshal([]byte(value), &refMap); err != nil {
		return nil, clierr.Usagef("--reference-map 不是合法的 reference_map JSON 对象: %v", err)
	}
	return refMap, nil
}

func cloneReferenceMap(refMap map[string]any) map[string]any {
	if len(refMap) == 0 {
		return nil
	}
	out := make(map[string]any, len(refMap))
	for k, v := range refMap {
		out[k] = v
	}
	return out
}

// html5GroupFromReferenceMap 取出 reference_map["html5-block"] 并按条目结构解析。
func html5GroupFromReferenceMap(refMap map[string]any) (map[string]html5RefEntry, error) {
	raw, ok := refMap[html5BlockTag]
	if !ok || raw == nil {
		return nil, nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, clierr.Usagef("reference_map.%s 不是合法的 JSON: %v", html5BlockTag, err)
	}
	var group map[string]html5RefEntry
	if err := json.Unmarshal(b, &group); err != nil {
		return nil, clierr.Usagef("reference_map.%s 必须是 {\"<ref>\": {\"data\": \"<html>\"}} 形式的对象: %v", html5BlockTag, err)
	}
	if len(group) == 0 {
		return nil, nil
	}
	return group, nil
}

// prepareHTML5BlockRefs 把 <html5-block path="@x.html"/> 改写为 data-ref，并校验已有 data-ref 都能在 reference_map 中找到。
func prepareHTML5BlockRefs(content string, group map[string]html5RefEntry, opts docsAIWriteOptions) (string, map[string]html5RefEntry, error) {
	if !strings.Contains(content, "<"+html5BlockTag) {
		return content, group, nil
	}
	out := make(map[string]html5RefEntry, len(group))
	for k, v := range group {
		out[k] = v
	}
	next := 1
	nextRef := func() string {
		for {
			ref := fmt.Sprintf("html5_%d", next)
			next++
			if _, exists := out[ref]; !exists {
				return ref
			}
		}
	}

	rewrite := func(seg string) (string, error) {
		var b strings.Builder
		for i := 0; i < len(seg); {
			at := indexTagStart(seg, i, html5BlockTag)
			if at < 0 {
				b.WriteString(seg[i:])
				break
			}
			b.WriteString(seg[i:at])
			end := findTagEnd(seg, at)
			if end < 0 {
				return "", clierr.Usagef("html5-block 标签不完整（缺少 >）")
			}
			tag := parseStartTag(seg[at:end])
			if !tag.SelfClosing {
				// 标签体必须为空：HTML 只能来自 path 或 reference_map
				closeAt := indexCloseTag(seg, end, html5BlockTag)
				if closeAt >= 0 && strings.TrimSpace(seg[end:closeAt]) != "" {
					return "", clierr.Usagef("<html5-block> 与 </html5-block> 之间不能写内容；HTML 请放进本地 .html 文件用 path=\"@./widget.html\" 引用，或通过 --reference-map 提供")
				}
			}
			if tag.has("data") {
				return "", clierr.Usagef("html5-block 的 data 属性保留给内部使用；请用 path=\"@./widget.html\"，或 data-ref 配合 --reference-map")
			}
			pathValue, hasPath := tag.get("path")
			dataRef, hasRef := tag.get("data-ref")
			switch {
			case hasPath && hasRef:
				return "", clierr.Usagef("html5-block 不能同时包含 path 与 data-ref")
			case hasRef:
				ref := strings.TrimSpace(dataRef)
				if ref == "" {
					return "", clierr.Usagef("html5-block 的 data-ref 不能为空")
				}
				if _, ok := out[ref]; !ok {
					return "", clierr.Usagef("html5-block data-ref=%q 需要在 --reference-map 中提供 reference_map.%s.%s", ref, html5BlockTag, ref)
				}
				b.WriteString(tag.render(false))
			case hasPath:
				data, err := readHTML5BlockPath(pathValue, "html5-block path", opts)
				if err != nil {
					return "", err
				}
				ref := nextRef()
				out[ref] = html5RefEntry{Data: data}
				tag.remove("path", "data-ref", "data")
				tag.set("data-ref", ref)
				b.WriteString(tag.render(false))
			default:
				return "", clierr.Usagef("html5-block 需要 path=\"@./widget.html\"，或 data-ref 配合 --reference-map")
			}
			i = end
		}
		return b.String(), nil
	}

	var (
		result string
		err    error
	)
	if opts.markdown() {
		result, err = mapOutsideFences(content, rewrite)
	} else {
		result, err = rewrite(content)
	}
	if err != nil {
		return "", nil, err
	}
	if len(out) == 0 {
		return result, nil, nil
	}
	return result, out, nil
}

// resolveHTML5RefPaths 把 reference_map 中 html5-block 条目的 path（@./x.html）读取为 data。
func resolveHTML5RefPaths(group map[string]html5RefEntry, opts docsAIWriteOptions) error {
	for ref, entry := range group {
		if strings.TrimSpace(entry.Path) == "" {
			continue
		}
		if entry.Data != "" {
			return clierr.Usagef("reference_map.%s.%s 只能使用 data 或 path 其中之一", html5BlockTag, ref)
		}
		data, err := readHTML5BlockPath(entry.Path, fmt.Sprintf("reference_map.%s.%s.path", html5BlockTag, ref), opts)
		if err != nil {
			return err
		}
		entry.Data, entry.Path = data, ""
		group[ref] = entry
	}
	return nil
}

func readHTML5BlockPath(pathValue, label string, opts docsAIWriteOptions) (string, error) {
	raw := strings.TrimSpace(pathValue)
	if !strings.HasPrefix(raw, "@") {
		return "", clierr.Usagef("%s %q 必须以 @ 开头，例如 @./widget.html", label, pathValue)
	}
	p := strings.TrimSpace(strings.TrimPrefix(raw, "@"))
	if p == "" {
		return "", clierr.Usagef("%s 的 @ 后面不能为空", label)
	}
	if !strings.EqualFold(filepath.Ext(p), ".html") {
		return "", clierr.Usagef("%s %q 必须指向 .html 文件", label, pathValue)
	}
	data, err := readDocResourceFile(p, opts)
	if err != nil {
		return "", wrapDocResourceReadError(label, p, err)
	}
	return string(data), nil
}

// prepareWhiteboardFileRefs 把 <whiteboard type="..." path="@x"/>（或标签体 @x）展开为内联内容。
// 同一次写入中的全部读取错误汇总后一起返回，便于一次修正。
func prepareWhiteboardFileRefs(content string, opts docsAIWriteOptions) (string, error) {
	if !strings.Contains(content, "<whiteboard") {
		return content, nil
	}
	var errs []string
	rewrite := func(seg string) (string, error) {
		var b strings.Builder
		for i := 0; i < len(seg); {
			at := indexTagStart(seg, i, "whiteboard")
			if at < 0 {
				b.WriteString(seg[i:])
				break
			}
			b.WriteString(seg[i:at])
			startEnd := findTagEnd(seg, at)
			if startEnd < 0 {
				b.WriteString(seg[at:])
				break
			}
			tag := parseStartTag(seg[at:startEnd])
			elemEnd, body := startEnd, ""
			if !tag.SelfClosing {
				closeAt := indexCloseTag(seg, startEnd, "whiteboard")
				if closeAt < 0 {
					// 没有闭合标签：不是本地文件引用的完整写法，原样交给服务端
					b.WriteString(seg[at:startEnd])
					i = startEnd
					continue
				}
				body = seg[startEnd:closeAt]
				elemEnd = closeAt + len("</whiteboard>")
			}
			rendered, err := rewriteWhiteboardElement(tag, body, opts)
			if err != nil {
				errs = append(errs, err.Error())
				b.WriteString(seg[at:elemEnd])
			} else if rendered == "" {
				b.WriteString(seg[at:elemEnd])
			} else {
				b.WriteString(rendered)
			}
			i = elemEnd
		}
		return b.String(), nil
	}
	var out string
	if opts.markdown() {
		out, _ = mapOutsideFences(content, rewrite)
	} else {
		out, _ = rewrite(content)
	}
	if len(errs) > 0 {
		return "", clierr.Usagef("画板本地文件引用处理失败: %s", strings.Join(errs, "; "))
	}
	return out, nil
}

// rewriteWhiteboardElement 返回展开后的画板元素；不是本地文件引用时返回空串（原样保留）。
func rewriteWhiteboardElement(tag startTag, body string, opts docsAIWriteOptions) (string, error) {
	pathValue, hasPath := tag.get("path")
	bodyPath, hasBodyPath := whiteboardBodyPathRef(body)
	if !hasPath && !hasBodyPath {
		return "", nil
	}
	if hasPath && strings.TrimSpace(body) != "" {
		return "", fmt.Errorf("whiteboard 不能同时写 path 与内联内容")
	}
	typRaw, _ := tag.get("type")
	typ := strings.ToLower(strings.TrimSpace(typRaw))
	exts, ok := whiteboardFileExts[typ]
	if !ok {
		return "", fmt.Errorf("whiteboard 本地文件引用只支持 type=\"svg\" / \"mermaid\" / \"plantuml\"，当前 type=%q", typRaw)
	}
	if hasBodyPath {
		pathValue = bodyPath
	}
	raw := strings.TrimSpace(pathValue)
	if !strings.HasPrefix(raw, "@") {
		return "", fmt.Errorf("whiteboard %s path %q 必须以 @ 开头，例如 @./diagram%s", typ, pathValue, exts[0])
	}
	p := strings.TrimSpace(strings.TrimPrefix(raw, "@"))
	if p == "" {
		return "", fmt.Errorf("whiteboard %s path 的 @ 后面不能为空", typ)
	}
	ext := strings.ToLower(filepath.Ext(p))
	allowed := false
	for _, e := range exts {
		if ext == e {
			allowed = true
		}
	}
	if !allowed {
		return "", fmt.Errorf("whiteboard %s path %q 必须指向 %s 文件", typ, pathValue, strings.Join(exts, " / "))
	}
	data, err := readDocResourceFile(p, opts)
	if err != nil {
		return "", wrapDocResourceReadError("whiteboard "+typ+" path", p, err)
	}
	text := string(data)
	if typ != "svg" {
		text = escapeXMLText(text)
	}
	tag.set("type", typ)
	tag.remove("path")
	tag.SelfClosing = false // 闭合标签由下面统一补上
	return tag.render(false) + text + "</whiteboard>", nil
}

var whiteboardFileExts = map[string][]string{
	"svg":      {".svg"},
	"mermaid":  {".mermaid", ".mmd"},
	"plantuml": {".plantuml", ".puml", ".pu", ".uml"},
}

// whiteboardBodyPathRef 识别标签体只有一行 @path 的写法（@@ 开头视为字面内容）。
func whiteboardBodyPathRef(body string) (string, bool) {
	t := strings.TrimSpace(body)
	if !strings.HasPrefix(t, "@") || strings.HasPrefix(t, "@@") || strings.ContainsAny(t, "\r\n") {
		return "", false
	}
	return t, true
}

// resolveDocResourcePath 解析内容中 @ 引用的本地路径：相对当前目录；不存在时可回退到 fallbackDir（对齐官方：
// 只有"不存在"才回退，敏感目录等校验失败不会改选其他文件）。返回最终路径与文件信息（已经过 safefile 校验）。
func resolveDocResourcePath(p, fallbackDir string) (string, os.FileInfo, error) {
	if fallbackDir != "" && !filepath.IsAbs(p) {
		if err := safefile.ValidateInputPath(p); err == nil {
			if _, statErr := os.Stat(p); errors.Is(statErr, fs.ErrNotExist) {
				candidate := filepath.Join(fallbackDir, p)
				if candidate != filepath.Clean(p) {
					if _, err := os.Stat(candidate); err == nil {
						p = candidate
					}
				}
			}
		}
	}
	info, err := safefile.StatInputFile(p)
	if err != nil {
		return p, nil, err
	}
	return p, info, nil
}

func readDocResourceFile(p string, opts docsAIWriteOptions) ([]byte, error) {
	resolved, _, err := resolveDocResourcePath(p, opts.resourceFallbackDir())
	if err != nil {
		return nil, err
	}
	return safefile.ReadInputFile(resolved)
}

func wrapDocResourceReadError(label, p string, err error) error {
	return clierr.Usagef("%s %q 无法读取: %v", label, p, err)
}

// ============================================================
// 轻量 XML/HTML 起始标签解析（容忍裸 &、单引号与布尔属性，按原顺序重新渲染）
// ============================================================

type startTag struct {
	Name        string
	Attrs       []tagAttr
	SelfClosing bool
}

type tagAttr struct {
	Name  string
	Value string // 已反转义
}

// indexTagStart 返回 from 之后第一个 <name（其后为空白、/ 或 >）的位置，找不到返回 -1。
func indexTagStart(s string, from int, name string) int {
	prefix := "<" + name
	for i := from; i < len(s); {
		k := strings.Index(s[i:], prefix)
		if k < 0 {
			return -1
		}
		at := i + k
		next := at + len(prefix)
		if next >= len(s) || isTagBoundary(s[next]) {
			return at
		}
		i = next
	}
	return -1
}

// indexCloseTag 返回 from 之后第一个 </name> 的位置（大小写不敏感），找不到返回 -1。
func indexCloseTag(s string, from int, name string) int {
	k := strings.Index(strings.ToLower(s[from:]), "</"+name+">")
	if k < 0 {
		return -1
	}
	return from + k
}

func isTagBoundary(c byte) bool {
	return c == '>' || c == '/' || c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

// findTagEnd 从 < 开始找起始标签结束的 >（引号内的 > 不算），返回其后位置；找不到返回 -1。
func findTagEnd(s string, start int) int {
	var quote byte
	for i := start + 1; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
		case '>':
			return i + 1
		}
	}
	return -1
}

// parseStartTag 解析 <name a="1" b='2' c/> 形式的起始标签。
func parseStartTag(raw string) startTag {
	t := startTag{SelfClosing: strings.HasSuffix(strings.TrimSpace(raw), "/>")}
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "<")
	s = strings.TrimSuffix(s, ">")
	s = strings.TrimSuffix(s, "/")
	i := 0
	for i < len(s) && !isTagSpace(s[i]) && s[i] != '/' {
		i++
	}
	t.Name = s[:i]
	for i < len(s) {
		for i < len(s) && (isTagSpace(s[i]) || s[i] == '/') {
			i++
		}
		start := i
		for i < len(s) && !isTagSpace(s[i]) && s[i] != '=' && s[i] != '/' {
			i++
		}
		if start == i {
			i++
			continue
		}
		name := s[start:i]
		for i < len(s) && isTagSpace(s[i]) {
			i++
		}
		if i >= len(s) || s[i] != '=' {
			t.Attrs = append(t.Attrs, tagAttr{Name: name})
			continue
		}
		i++
		for i < len(s) && isTagSpace(s[i]) {
			i++
		}
		var value string
		if i < len(s) && (s[i] == '"' || s[i] == '\'') {
			q := s[i]
			end := strings.IndexByte(s[i+1:], q)
			if end < 0 {
				value = s[i+1:]
				i = len(s)
			} else {
				value = s[i+1 : i+1+end]
				i = i + 1 + end + 1
			}
		} else {
			vs := i
			for i < len(s) && !isTagSpace(s[i]) {
				i++
			}
			value = s[vs:i]
		}
		t.Attrs = append(t.Attrs, tagAttr{Name: name, Value: html.UnescapeString(value)})
	}
	return t
}

func isTagSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }

func (t startTag) get(name string) (string, bool) {
	for _, a := range t.Attrs {
		if strings.EqualFold(a.Name, name) {
			return a.Value, true
		}
	}
	return "", false
}

func (t startTag) has(name string) bool {
	_, ok := t.get(name)
	return ok
}

func (t *startTag) set(name, value string) {
	for i := range t.Attrs {
		if strings.EqualFold(t.Attrs[i].Name, name) {
			t.Attrs[i].Value = value
			return
		}
	}
	t.Attrs = append(t.Attrs, tagAttr{Name: name, Value: value})
}

func (t *startTag) remove(names ...string) {
	kept := t.Attrs[:0]
	for _, a := range t.Attrs {
		drop := false
		for _, n := range names {
			if strings.EqualFold(a.Name, n) {
				drop = true
				break
			}
		}
		if !drop {
			kept = append(kept, a)
		}
	}
	t.Attrs = kept
}

// rename 把属性 oldName 改名为 newName（newName 已存在时不改）。
func (t *startTag) rename(oldName, newName string) {
	if t.has(newName) {
		return
	}
	for i := range t.Attrs {
		if strings.EqualFold(t.Attrs[i].Name, oldName) {
			t.Attrs[i].Name = newName
			return
		}
	}
}

// render 重新渲染起始标签。selfClosing=false 且原标签自闭合时补上闭合标签（<x/> → <x></x>）。
func (t startTag) render(selfClosing bool) string {
	var b strings.Builder
	b.WriteByte('<')
	b.WriteString(t.Name)
	for _, a := range t.Attrs {
		b.WriteByte(' ')
		b.WriteString(a.Name)
		b.WriteString(`="`)
		b.WriteString(escapeXMLAttr(a.Value))
		b.WriteByte('"')
	}
	if selfClosing {
		b.WriteString("/>")
		return b.String()
	}
	b.WriteByte('>')
	if t.SelfClosing {
		b.WriteString("</" + t.Name + ">")
	}
	return b.String()
}

func escapeXMLAttr(v string) string {
	return strings.NewReplacer(`&`, "&amp;", `<`, "&lt;", `>`, "&gt;", `"`, "&quot;", `'`, "&apos;").Replace(v)
}

func escapeXMLText(v string) string {
	return strings.NewReplacer(`&`, "&amp;", `<`, "&lt;").Replace(v)
}

// mapOutsideFences 对 Markdown 围栏代码块以外的连续片段应用 fn（片段保留换行），围栏内原样保留。
func mapOutsideFences(content string, fn func(seg string) (string, error)) (string, error) {
	var out, seg strings.Builder
	var fenceChar byte
	fenceLen := 0
	flush := func() error {
		if seg.Len() == 0 {
			return nil
		}
		s, err := fn(seg.String())
		seg.Reset()
		if err != nil {
			return err
		}
		out.WriteString(s)
		return nil
	}
	for _, line := range strings.SplitAfter(content, "\n") {
		if ch, n, ok := dialectFence(strings.TrimRight(line, "\r\n")); ok {
			if fenceChar == 0 {
				if err := flush(); err != nil {
					return "", err
				}
				fenceChar, fenceLen = ch, n
			} else if ch == fenceChar && n >= fenceLen {
				fenceChar, fenceLen = 0, 0
			}
			out.WriteString(line)
			continue
		}
		if fenceChar != 0 {
			out.WriteString(line)
			continue
		}
		seg.WriteString(line)
	}
	if err := flush(); err != nil {
		return "", err
	}
	return out.String(), nil
}
