package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/converter"
	"github.com/riba2534/feishu-cli/internal/safefile"
)

// doc create / doc content-update 插入本地图片/附件（对齐官方 local_doc_resources 的占位标记协议，测试文档实测）：
//
//  1. 发送前把本地资源改写为占位标签：<img path="@lcli_img_<32hex>"/>、<source path="@lcli_file_<32hex>"/>；
//  2. docs_ai 在 document.new_blocks 中回传占位块：block_token 等于占位标记，block_type 为 image / file；
//  3. 以占位块 ID 为 parent_node 上传素材（docx_image / docx_file，drive_route_token=文档），
//     再用 batch_update 的 replace_image / replace_file 绑定（client_token 在重试间复用）；
//  4. 任一资源上传/绑定失败时删除对应占位块（附件删除其外层视图块），命令以非零退出并输出逐项明细；
//  5. 输出前把 new_blocks 中的占位标记换成上传后的素材 token（失败项去掉），document.revision_id 更新为绑定/清理后的版本。
//
// 支持的写法（围栏代码、行内代码与 HTML 注释中的内容不处理）：
//   - ![说明](@./img.png)、![说明](<@./带 空格.png>)：官方写法，路径相对当前目录；
//   - ![说明](./img.png)、![说明](/abs/img.png)：本地导入写法，相对内容文件所在目录（内联内容相对当前目录）；
//   - <img path="@./img.png" width="600"/>、<source path="@./report.pdf" name="报告.pdf"/>：XML 写法。
//     XML 内容来自文件且 @ 相对路径在当前目录不存在时，回退到内容文件所在目录（对齐官方）。
//
// 图片尺寸归一化（对齐官方 normalizeLocalDocImagePresentation）：<img> 的 width/height 一律改为图片真实像素，
// 模型给出的显示尺寸（scale / width / height / 百分比）换算为 scale；宽度 ≥1020px 时缩放到略小于页面宽度。
//
// 部分实现改编自 larksuite/cli（MIT License, Copyright (c) 2026 Lark Technologies Pte. Ltd.），
// 对应 shortcuts/doc/local_doc_resources.go。

const (
	localResourceBindBatch   = 20
	localResourceUploadTries = 3

	localDocImageMaxDisplayWidthPx = 1020
	localDocImageScalePrecision    = 1000000
)

// localResourceModes 是允许携带本地资源的模式（会新建块的写入）。
var localResourceModes = map[string]bool{
	"append": true, "overwrite": true, "insert_before": true, "insert_after": true, "replace_range": true,
}

// localDocImageAlign 是 replace_image 的 align 枚举。
var localDocImageAlign = map[string]int{"left": 1, "center": 2, "right": 3}

type localDocResource struct {
	Kind       string  `json:"kind"` // image | file
	Path       string  `json:"path"`
	Marker     string  `json:"-"`
	Occurrence int     `json:"-"`
	FileName   string  `json:"file_name"`
	Width      int     `json:"-"`
	Height     int     `json:"-"`
	Align      string  `json:"-"`
	Scale      float64 `json:"-"`
	HasScale   bool    `json:"-"`
	Size       int64   `json:"-"`
	BlockID    string  `json:"block_id,omitempty"`
	FileToken  string  `json:"file_token,omitempty"`
	Status     string  `json:"status"` // bound / failed
	Cleanup    string  `json:"cleanup,omitempty"`
	Error      string  `json:"error,omitempty"`
}

var (
	mdImageRe       = regexp.MustCompile(`!\[([^\]]*)\]\(\s*(<[^>]+>|[^)\s]+)(\s+"[^"]*")?\s*\)`)
	reservedMarkRe  = regexp.MustCompile(`@lcli_(img|file)_`)
	remoteURLPrefix = []string{"http://", "https://", "data:", "#", "feishu://", "mailto:"}
)

// prepareLocalDocResources 改写内容中的本地资源为占位标签，返回改写后的内容与资源清单（离线）。
// baseDir 为 Markdown 非 @ 本地图片路径的解析基准。
func prepareLocalDocResources(content, format, baseDir string) (string, []*localDocResource, error) {
	return prepareLocalDocResourcesWith(content, docsAIWriteOptions{Format: format, BaseDir: baseDir})
}

// prepareLocalDocResourcesWith 按写入选项改写本地图片/附件为占位标签（离线校验文件存在与路径安全）。
func prepareLocalDocResourcesWith(content string, opts docsAIWriteOptions) (string, []*localDocResource, error) {
	if reservedMarkRe.MatchString(content) {
		return "", nil, clierr.Usagef("内容中包含保留的占位标记 @lcli_img_/@lcli_file_，请勿手写")
	}
	if !strings.Contains(content, "<img") && !strings.Contains(content, "<source") &&
		!(opts.markdown() && strings.Contains(content, "![")) {
		return content, nil, nil
	}
	var resources []*localDocResource
	var firstErr error

	rewriteSeg := func(seg string) string {
		if firstErr != nil {
			return seg
		}
		seg, err := rewriteLocalResourceTags(seg, opts, &resources)
		if err != nil {
			firstErr = err
			return seg
		}
		if !opts.markdown() {
			return seg
		}
		// Markdown 图片
		return mdImageRe.ReplaceAllStringFunc(seg, func(raw string) string {
			if firstErr != nil {
				return raw
			}
			m := mdImageRe.FindStringSubmatch(raw)
			dest := strings.TrimSpace(m[2])
			dest = strings.TrimSuffix(strings.TrimPrefix(dest, "<"), ">")
			for _, p := range remoteURLPrefix {
				if strings.HasPrefix(strings.ToLower(dest), p) {
					return raw
				}
			}
			dir, fallback := opts.markdownBaseDir(), ""
			if strings.HasPrefix(dest, "@") {
				dest, dir, fallback = strings.TrimPrefix(dest, "@"), "", opts.resourceFallbackDir()
			}
			res, err := newLocalDocResource("image", dest, dir, fallback, opts.Strict, len(resources)+1)
			if err != nil {
				firstErr = err
				return raw
			}
			resources = append(resources, res)
			out := `<img path="` + res.Marker + `"`
			if alt := strings.TrimSpace(m[1]); alt != "" {
				out += ` caption="` + escapeDialectAttr(alt) + `"`
			}
			return out + "/>"
		})
	}

	out, _ := mapOutsideFences(content, func(seg string) (string, error) {
		protected, spans := protectInertSpans(seg)
		lines := strings.SplitAfter(protected, "\n")
		for i := range lines {
			lines[i] = mapOutsideInlineCode(lines[i], rewriteSeg)
		}
		rewritten := strings.Join(lines, "")
		for _, sp := range spans {
			rewritten = strings.Replace(rewritten, sp.token, sp.original, 1)
		}
		return rewritten, nil
	})
	if firstErr != nil {
		return "", nil, firstErr
	}
	return out, resources, nil
}

// inertSpan 是改写本地资源时需要原样保留的片段（HTML 注释、CDATA）。
type inertSpan struct {
	token    string
	original string
}

// protectInertSpans 用占位符替换 <!-- --> 与 <![CDATA[ ]]> 片段（保留换行数），改写后再还原。
func protectInertSpans(s string) (string, []inertSpan) {
	if !strings.Contains(s, "<!") {
		return s, nil
	}
	var b strings.Builder
	var spans []inertSpan
	for i := 0; i < len(s); {
		end := -1
		switch {
		case strings.HasPrefix(s[i:], "<!--"):
			if k := strings.Index(s[i+4:], "-->"); k >= 0 {
				end = i + 4 + k + 3
			} else {
				end = len(s)
			}
		case strings.HasPrefix(s[i:], "<![CDATA["):
			if k := strings.Index(s[i+9:], "]]>"); k >= 0 {
				end = i + 9 + k + 3
			} else {
				end = len(s)
			}
		}
		if end < 0 {
			b.WriteByte(s[i])
			i++
			continue
		}
		var token string
		for n := len(spans); ; n++ {
			token = fmt.Sprintf("\ue000fcli_inert_%d\ue001", n)
			if !strings.Contains(s, token) {
				break
			}
		}
		token += strings.Repeat("\n", strings.Count(s[i:end], "\n"))
		spans = append(spans, inertSpan{token: token, original: s[i:end]})
		b.WriteString(token)
		i = end
	}
	return b.String(), spans
}

// rewriteLocalResourceTags 改写片段中的 <img path="@..."> 与 <source path="@...">。
func rewriteLocalResourceTags(seg string, opts docsAIWriteOptions, resources *[]*localDocResource) (string, error) {
	if !strings.Contains(seg, "<img") && !strings.Contains(seg, "<source") {
		return seg, nil
	}
	var b strings.Builder
	for i := 0; i < len(seg); {
		at, name := -1, ""
		for _, n := range []string{"img", "source"} {
			if k := indexTagStart(seg, i, n); k >= 0 && (at < 0 || k < at) {
				at, name = k, n
			}
		}
		if at < 0 {
			b.WriteString(seg[i:])
			break
		}
		b.WriteString(seg[i:at])
		end := findTagEnd(seg, at)
		if end < 0 {
			if opts.Strict {
				return "", clierr.Usagef("<%s> 本地资源标签不完整（缺少 >）", name)
			}
			b.WriteString(seg[at:])
			break
		}
		raw := seg[at:end]
		rendered, res, err := rewriteLocalResourceTag(raw, name, opts, len(*resources)+1)
		if err != nil {
			return "", err
		}
		if res != nil {
			*resources = append(*resources, res)
		}
		b.WriteString(rendered)
		i = end
	}
	return b.String(), nil
}

// rewriteLocalResourceTag 处理单个 <img>/<source> 标签；不含本地 path 时原样返回。
func rewriteLocalResourceTag(raw, name string, opts docsAIWriteOptions, occurrence int) (string, *localDocResource, error) {
	kind, label := "image", "图片"
	if name == "source" {
		kind, label = "file", "附件"
	}
	tag := parseStartTag(raw)
	pathValue, hasPath := tag.get("path")
	if !hasPath {
		return raw, nil, nil
	}
	pathValue = strings.TrimSpace(pathValue)
	if !strings.HasPrefix(pathValue, "@") {
		if opts.Strict {
			return "", nil, clierr.Usagef("本地%s #%d: <%s> 的 path 必须以 @ 开头（如 path=\"@./a.png\"），当前 %q", label, occurrence, name, pathValue)
		}
		return raw, nil, nil
	}
	if opts.Strict {
		for _, conflict := range []string{"src", "href", "token", "img_key", "img-key", "url"} {
			if tag.has(conflict) {
				return "", nil, clierr.Usagef("本地%s #%d: <%s> 的本地 path 不能与 %s 同时使用", label, occurrence, name, conflict)
			}
		}
	}
	res, err := newLocalDocResource(kind, strings.TrimPrefix(pathValue, "@"), "", opts.resourceFallbackDir(), opts.Strict, occurrence)
	if err != nil {
		return "", nil, err
	}
	tag.set("path", res.Marker)
	if kind == "image" {
		if opts.Strict {
			tag.rename("alt", "caption")
		}
		if res.Width > 0 && res.Height > 0 {
			normalizeDocImagePresentation(&tag, res.Width, res.Height)
		} else {
			// 尺寸未知（JPEG EXIF 旋转、无法识别的图片）：沿用标签上的显式宽高，交给服务端推断其余
			res.Width, res.Height = 0, 0
		}
		res.captureImagePresentation(tag)
	} else if n, ok := tag.get("name"); ok {
		n = strings.TrimSpace(n)
		if n != "" && n != "." && n != ".." && !strings.ContainsAny(n, `/\`) {
			res.FileName = n
			tag.set("name", n)
		} else if opts.Strict {
			return "", nil, clierr.Usagef("本地附件 #%d: <source> 的 name 必须是不含路径分隔符的文件名", occurrence)
		}
	}
	return tag.render(tag.SelfClosing), res, nil
}

// newLocalDocResource 校验本地文件并生成占位标记。dir 非空时相对路径基于 dir 解析；
// 否则相对当前目录，不存在时回退到 fallbackDir（可为空）。strict 时图片必须可识别。
func newLocalDocResource(kind, pathValue, dir, fallbackDir string, strict bool, occurrence int) (*localDocResource, error) {
	pathValue = strings.TrimSpace(pathValue)
	label := "图片"
	if kind == "file" {
		label = "附件"
	}
	if pathValue == "" {
		return nil, clierr.Usagef("本地%s路径为空", label)
	}
	p := pathValue
	if !filepath.IsAbs(p) && dir != "" {
		p = filepath.Join(dir, p)
	}
	if err := safefile.ValidateInputPath(p); err != nil {
		return nil, clierr.Usagef("本地%s路径不安全 %s: %v", label, pathValue, err)
	}
	if dir == "" && fallbackDir != "" {
		p, _, _ = resolveDocResourcePath(p, fallbackDir)
	}
	st, err := os.Stat(p)
	if err != nil {
		return nil, clierr.Usagef("本地%s不存在或不可读: %s（%v）", label, pathValue, err)
	}
	if !st.Mode().IsRegular() || st.Size() == 0 {
		return nil, clierr.Usagef("本地%s不是非空的普通文件: %s", label, pathValue)
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("生成占位标记失败: %w", err)
	}
	prefix := "@lcli_img_"
	if kind == "file" {
		prefix = "@lcli_file_"
	}
	res := &localDocResource{Kind: kind, Path: p, Marker: prefix + hex.EncodeToString(raw), Occurrence: occurrence,
		FileName: filepath.Base(p), Size: st.Size()}
	if kind == "image" {
		w, h, decodable := probeLocalImageSize(p)
		if !decodable && strict {
			return nil, clierr.Usagef("本地图片 #%d 不是可识别的 BMP / GIF / JPEG / PNG / TIFF / WebP 图片: %s", occurrence, pathValue)
		}
		res.Width, res.Height = w, h
	}
	return res, nil
}

// probeLocalImageSize 读取图片像素尺寸（只读文件头）。decodable=false 表示不是可识别的图片；
// JPEG 带 EXIF 旋转（5-8）时宽高转置，返回 decodable=true 但宽高为 0，交给服务端推断。
func probeLocalImageSize(path string) (w, h int, decodable bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	cfg, format, err := image.DecodeConfig(f)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0, false
	}
	if format == "jpeg" {
		if _, err := f.Seek(0, io.SeekStart); err == nil {
			if o := jpegEXIFOrientation(f); o >= 5 && o <= 8 {
				return 0, 0, true
			}
		}
	}
	return cfg.Width, cfg.Height, true
}

// normalizeDocImagePresentation 把 <img> 的 width/height 改为真实像素，模型给出的显示尺寸换算为 scale
// （优先级 scale > width > height；均未给出且宽度 ≥1020px 时缩放到略小于页面宽度）。
func normalizeDocImagePresentation(tag *startTag, nativeW, nativeH int) {
	modelScale, hasScale := positiveImageFloatAttr(*tag, "scale", 0)
	modelW, hasW := positiveImageFloatAttr(*tag, "width", nativeW)
	modelH, hasH := positiveImageFloatAttr(*tag, "height", nativeH)
	tag.set("width", strconv.Itoa(nativeW))
	tag.set("height", strconv.Itoa(nativeH))

	var scale float64
	switch {
	case hasScale:
		scale = modelScale
	case hasW:
		scale = modelW / float64(nativeW)
	case hasH:
		scale = modelH / float64(nativeH)
	case nativeW >= localDocImageMaxDisplayWidthPx:
		scale = 1
	default:
		tag.remove("scale")
		return
	}
	if floored := math.Floor(scale*localDocImageScalePrecision) / localDocImageScalePrecision; floored > 0 {
		scale = floored
	}
	scale = capImageScaleBelowPageWidth(nativeW, scale)
	tag.set("scale", strconv.FormatFloat(scale, 'f', 6, 64))
}

// positiveImageFloatAttr 解析正数属性；nativeSize>0 时支持百分比（相对真实像素）。
func positiveImageFloatAttr(tag startTag, name string, nativeSize int) (float64, bool) {
	v, ok := tag.get(name)
	if !ok {
		return 0, false
	}
	v = strings.TrimSpace(v)
	percent := false
	if nativeSize > 0 && strings.HasSuffix(v, "%") {
		v, percent = strings.TrimSpace(strings.TrimSuffix(v, "%")), true
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f <= 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	if percent {
		return float64(nativeSize) * f / 100, true
	}
	return f, true
}

func capImageScaleBelowPageWidth(nativeW int, scale float64) float64 {
	maxScale := float64(localDocImageMaxDisplayWidthPx) / float64(nativeW)
	if scale < maxScale {
		return scale
	}
	capped := math.Floor(maxScale*localDocImageScalePrecision) / localDocImageScalePrecision
	if capped >= maxScale {
		capped -= 1.0 / localDocImageScalePrecision
	}
	if capped <= 0 {
		return math.Nextafter(maxScale, 0)
	}
	return capped
}

// captureImagePresentation 从归一化后的标签读取绑定时使用的 width / height / align / scale。
func (r *localDocResource) captureImagePresentation(tag startTag) {
	if v, ok := tag.get("width"); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			r.Width = n
		}
	}
	if v, ok := tag.get("height"); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			r.Height = n
		}
	}
	if v, ok := tag.get("align"); ok {
		if a := strings.ToLower(strings.TrimSpace(v)); localDocImageAlign[a] > 0 {
			r.Align = a
		}
	}
	if v, ok := tag.get("scale"); ok {
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && f > 0 {
			r.Scale, r.HasScale = f, true
		}
	}
}

// replaceImageRequest 构造 batch_update 的 replace_image（dry-run 与真实绑定共用）。
func (r *localDocResource) replaceImageRequest(token string) map[string]any {
	ri := map[string]any{"token": token}
	if r.Width > 0 {
		ri["width"] = r.Width
	}
	if r.Height > 0 {
		ri["height"] = r.Height
	}
	if a, ok := localDocImageAlign[r.Align]; ok {
		ri["align"] = a
	}
	if r.HasScale {
		ri["scale"] = r.Scale
	}
	return ri
}

// finalizeLocalDocResources 上传并绑定占位块；失败项清理占位块。返回是否全部成功。
// 同时改写 data：new_blocks 的占位标记换成素材 token（失败项去掉），document.revision_id 更新为最新版本。
func finalizeLocalDocResources(p *contentUpdateParams, data map[string]any, resources []*localDocResource) bool {
	if len(resources) == 0 {
		return true
	}
	byMarker := map[string][]map[string]any{}
	doc, _ := data["document"].(map[string]any)
	if blocks, ok := doc["new_blocks"].([]any); ok {
		for _, raw := range blocks {
			b, _ := raw.(map[string]any)
			if tok, _ := b["block_token"].(string); tok != "" {
				byMarker[tok] = append(byMarker[tok], b)
			}
		}
	}

	var ready []*localDocResource
	blockOf := map[*localDocResource]map[string]any{}
	uploads := 0
	for _, r := range resources {
		matches := byMarker[r.Marker]
		if len(matches) != 1 {
			r.Status, r.Error = "failed", fmt.Sprintf("服务端返回 %d 个占位块（期望 1 个），无法关联", len(matches))
			for _, b := range matches {
				if id, _ := b["block_id"].(string); id != "" && r.BlockID == "" {
					r.BlockID = id
				}
			}
			continue
		}
		blockOf[r] = matches[0]
		bt, _ := matches[0]["block_type"].(string)
		r.BlockID, _ = matches[0]["block_id"].(string)
		if r.BlockID == "" || bt != r.Kind {
			r.Status, r.Error = "failed", fmt.Sprintf("占位块类型异常（block_type=%s）", bt)
			continue
		}
		// 上传：限流/5xx 重试；串行上传之间留间隔，避免触发素材上传限流
		if uploads > 0 && waitBetweenLocalUploads > 0 {
			time.Sleep(waitBetweenLocalUploads)
		}
		uploads++
		parentType := "docx_image"
		if r.Kind == "file" {
			parentType = "docx_file"
		}
		res := client.DoWithRetry(func() (string, http.Header, error) {
			tok, err := client.UploadDocMedia(r.Path, parentType, r.BlockID, r.FileName, p.documentID, p.userToken)
			return tok, nil, err
		}, client.RetryConfig{MaxRetries: localResourceUploadTries - 1, MaxTotalAttempts: localResourceUploadTries + 2, RetryOnRateLimit: true})
		if res.Err != nil {
			r.Status, r.Error = "failed", "上传失败: "+res.Err.Error()
			continue
		}
		r.FileToken = res.Value
		ready = append(ready, r)
	}

	// 绑定：batch_update，每批 ≤20，client_token 在重试间复用
	latestRevision := 0
	for start := 0; start < len(ready); start += localResourceBindBatch {
		end := min(start+localResourceBindBatch, len(ready))
		chunk := ready[start:end]
		var reqs []map[string]any
		for _, r := range chunk {
			if r.Kind == "file" {
				reqs = append(reqs, map[string]any{"block_id": r.BlockID, "replace_file": map[string]any{"token": r.FileToken}})
				continue
			}
			reqs = append(reqs, map[string]any{"block_id": r.BlockID, "replace_image": r.replaceImageRequest(r.FileToken)})
		}
		payload, _ := jsonMarshalNoEscape(reqs)
		clientToken := client.NewClientToken()
		res := client.DoWithRetry(func() (*client.BatchUpdateBlocksResult, http.Header, error) {
			return client.BatchUpdateBlocks(p.documentID, payload, client.BatchUpdateBlocksOptions{
				ClientToken: clientToken, UserAccessToken: p.userToken,
			})
		}, client.RetryConfig{MaxRetries: 2, MaxTotalAttempts: 5, RetryOnRateLimit: true})
		if res.Err == nil && res.Value != nil && res.Value.DocumentRevision > 0 {
			latestRevision = res.Value.DocumentRevision
		}
		for _, r := range chunk {
			if res.Err != nil {
				r.Status, r.Error = "failed", "绑定失败: "+res.Err.Error()
			} else {
				r.Status = "bound"
			}
		}
	}

	// 清理失败项的占位块（附件删除外层视图块，避免留下空卡片）
	ok := true
	var cleanupIDs []string
	var cleanupOwners []*localDocResource
	for _, r := range resources {
		if r.Status == "bound" {
			continue
		}
		ok = false
		if r.BlockID == "" {
			r.Cleanup = "skipped"
			continue
		}
		target := r.BlockID
		if r.Kind == "file" {
			if blk, err := client.GetBlock(p.documentID, r.BlockID, p.userToken); err == nil {
				if parent := client.StringVal(blk.ParentId); parent != "" && parent != p.documentID {
					if pb, err := client.GetBlock(p.documentID, parent, p.userToken); err == nil && isSoleChildView(pb, r.BlockID) {
						target = parent
					}
				}
			}
		}
		cleanupIDs = append(cleanupIDs, target)
		cleanupOwners = append(cleanupOwners, r)
	}
	if len(cleanupIDs) > 0 {
		body := map[string]any{"format": "markdown", "command": "block_delete", "block_id": strings.Join(cleanupIDs, ",")}
		cleanupData, err := p.sendUpdate(body, -1)
		for _, r := range cleanupOwners {
			if err != nil {
				r.Cleanup = "failed: " + err.Error()
			} else {
				r.Cleanup = "deleted"
			}
		}
		if err == nil {
			if rev := extractRevisionID(cleanupData); rev > 0 {
				latestRevision = rev
			}
		}
	}

	// 输出前把占位标记换成素材 token（失败项去掉），版本号更新为绑定/清理之后的版本
	for _, r := range resources {
		if b := blockOf[r]; b != nil {
			if r.Status == "bound" {
				b["block_token"] = r.FileToken
			} else {
				delete(b, "block_token")
			}
		}
	}
	scrubLocalResourceMarkers(doc)
	if doc != nil && latestRevision > 0 {
		doc["revision_id"] = latestRevision
	}
	if failures := localResourceFailures(resources); len(failures) > 0 {
		data["local_resource_failures"] = failures
	}
	return ok
}

// scrubLocalResourceMarkers 去掉 new_blocks 中残留的占位标记（服务端回传了未能关联的占位块时）。
func scrubLocalResourceMarkers(doc map[string]any) {
	blocks, _ := doc["new_blocks"].([]any)
	for _, raw := range blocks {
		b, _ := raw.(map[string]any)
		if tok, _ := b["block_token"].(string); reservedMarkRe.MatchString(tok) {
			delete(b, "block_token")
		}
	}
}

// localResourceFailures 生成官方形状的失败明细（occurrence / kind / status / cleanup_status / error）。
func localResourceFailures(resources []*localDocResource) []map[string]any {
	var out []map[string]any
	for _, r := range resources {
		if r.Status == "bound" {
			continue
		}
		cleanup := r.Cleanup
		switch {
		case cleanup == "deleted":
			cleanup = "succeeded"
		case strings.HasPrefix(cleanup, "failed"):
			cleanup = "failed"
		}
		item := map[string]any{"occurrence": r.Occurrence, "kind": r.Kind, "status": r.Status, "cleanup_status": cleanup}
		if r.Error != "" {
			item["error"] = r.Error
		}
		out = append(out, item)
	}
	return out
}

// isSoleChildView 判断块是否为只包含指定子块的视图块（附件外层容器）。
func isSoleChildView(b *larkdocx.Block, childID string) bool {
	if b == nil || b.BlockType == nil || converter.BlockType(*b.BlockType) != converter.BlockTypeView {
		return false
	}
	return len(b.Children) == 1 && b.Children[0] == childID
}

// printLocalResourceLines 文本模式下输出本地资源结果（失败项到 stderr）。
func (p *contentUpdateParams) printLocalResourceLines() {
	if len(p.resources) == 0 || p.output == "json" {
		return
	}
	bound := 0
	for _, r := range p.resources {
		if r.Status == "bound" {
			bound++
			continue
		}
		label := "图片"
		if r.Kind == "file" {
			label = "附件"
		}
		fmt.Fprintf(p.errOut(), "✗ 本地%s %s: %s（占位块清理: %s）\n", label, r.Path, r.Error, r.Cleanup)
	}
	fmt.Fprintf(p.out(), "本地资源: %d/%d 上传并绑定成功\n", bound, len(p.resources))
}

// reportLocalResources 有失败时返回非零错误。
func (p *contentUpdateParams) reportLocalResources(_ map[string]any, resources []*localDocResource, ok bool) error {
	if ok || len(resources) == 0 {
		return nil
	}
	where := "上方输出"
	if p.output == "json" {
		where = " local_resources 字段"
	}
	return fmt.Errorf("部分本地图片/附件未能写入文档（明细见%s），失败项的占位块已尝试清理", where)
}

// waitBetweenLocalUploads 串行上传之间的间隔（drive 素材上传约 5 QPS），测试中可置 0。
var waitBetweenLocalUploads = 220 * time.Millisecond

func jsonMarshalNoEscape(v any) (string, error) {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSpace(b.String()), nil
}
