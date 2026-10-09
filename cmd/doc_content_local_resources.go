package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/converter"
	"github.com/riba2534/feishu-cli/v2/internal/safefile"
)

// content-update 插入本地图片/附件（对齐官方 local_doc_resources 的占位标记协议，测试文档实测）：
//
//  1. 发送前把本地资源改写为占位标签：<img path="@lcli_img_<32hex>"/>、<source path="@lcli_file_<32hex>"/>；
//  2. docs_ai 在 document.new_blocks 中回传占位块：block_token 等于占位标记，block_type 为 image / file；
//  3. 以占位块 ID 为 parent_node 上传素材（docx_image / docx_file，drive_route_token=文档），
//     再用 batch_update 的 replace_image / replace_file 绑定（client_token 在重试间复用）；
//  4. 任一资源上传/绑定失败时删除对应占位块（附件删除其外层视图块），命令以非零退出并输出逐项明细。
//
// 支持的写法（围栏代码与行内代码中的内容不处理）：
//   - ![说明](@./img.png)、![说明](<@./带 空格.png>)：官方写法，路径相对当前目录；
//   - ![说明](./img.png)、![说明](/abs/img.png)：本地导入写法，相对 --markdown-file 所在目录（内联内容相对当前目录）；
//   - <img path="@./img.png" width="600"/>、<source path="@./report.pdf" name="报告.pdf"/>：XML 写法。

const (
	localResourceBindBatch   = 20
	localResourceUploadTries = 3
)

// localResourceModes 是允许携带本地资源的模式（会新建块的写入）。
var localResourceModes = map[string]bool{
	"append": true, "overwrite": true, "insert_before": true, "insert_after": true, "replace_range": true,
}

type localDocResource struct {
	Kind      string `json:"kind"` // image | file
	Path      string `json:"path"`
	Marker    string `json:"-"`
	FileName  string `json:"file_name"`
	Width     int    `json:"-"`
	Height    int    `json:"-"`
	BlockID   string `json:"block_id,omitempty"`
	FileToken string `json:"file_token,omitempty"`
	Status    string `json:"status"` // bound / failed
	Cleanup   string `json:"cleanup,omitempty"`
	Error     string `json:"error,omitempty"`
}

var (
	mdImageRe       = regexp.MustCompile(`!\[([^\]]*)\]\(\s*(<[^>]+>|[^)\s]+)(\s+"[^"]*")?\s*\)`)
	xmlLocalTagRe   = regexp.MustCompile(`<(img|source)\b([^>]*?)\bpath\s*=\s*"(@[^"]+)"([^>]*?)(/?)>`)
	reservedMarkRe  = regexp.MustCompile(`@lcli_(img|file)_`)
	xmlAttrValueRe  = regexp.MustCompile(`\b(width|height|name)\s*=\s*"([^"]*)"`)
	remoteURLPrefix = []string{"http://", "https://", "data:", "#", "feishu://", "mailto:"}
)

// prepareLocalDocResources 改写内容中的本地资源为占位标签，返回改写后的内容与资源清单（离线）。
func prepareLocalDocResources(content, format, baseDir string) (string, []*localDocResource, error) {
	if reservedMarkRe.MatchString(content) {
		return "", nil, clierr.Usagef("内容中包含保留的占位标记 @lcli_img_/@lcli_file_，请勿手写")
	}
	var resources []*localDocResource
	var firstErr error
	markdownMode := format == "markdown"

	rewriteSeg := func(seg string) string {
		if firstErr != nil {
			return seg
		}
		// XML 写法 <img path="@..."/> / <source path="@..."/>
		seg = xmlLocalTagRe.ReplaceAllStringFunc(seg, func(raw string) string {
			if firstErr != nil {
				return raw
			}
			m := xmlLocalTagRe.FindStringSubmatch(raw)
			kind := "image"
			if m[1] == "source" {
				kind = "file"
			}
			res, err := newLocalDocResource(kind, strings.TrimPrefix(m[3], "@"), "")
			if err != nil {
				firstErr = err
				return raw
			}
			attrs := m[2] + m[4]
			for _, am := range xmlAttrValueRe.FindAllStringSubmatch(attrs, -1) {
				switch am[1] {
				case "width":
					fmt.Sscanf(am[2], "%d", &res.Width)
				case "height":
					fmt.Sscanf(am[2], "%d", &res.Height)
				case "name":
					if n := strings.TrimSpace(am[2]); n != "" && !strings.ContainsAny(n, `/\`) {
						res.FileName = n
					}
				}
			}
			resources = append(resources, res)
			return "<" + m[1] + m[2] + `path="` + res.Marker + `"` + m[4] + m[5] + ">"
		})
		if !markdownMode {
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
			dir := baseDir
			if strings.HasPrefix(dest, "@") {
				dest, dir = strings.TrimPrefix(dest, "@"), ""
			}
			res, err := newLocalDocResource("image", dest, dir)
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

	lines := strings.Split(content, "\n")
	var fenceChar byte
	fenceLen := 0
	for i, line := range lines {
		if ch, n, ok := dialectFence(line); ok {
			if fenceChar == 0 {
				fenceChar, fenceLen = ch, n
			} else if ch == fenceChar && n >= fenceLen {
				fenceChar, fenceLen = 0, 0
			}
			continue
		}
		if fenceChar != 0 {
			continue
		}
		lines[i] = mapOutsideInlineCode(line, rewriteSeg)
	}
	if firstErr != nil {
		return "", nil, firstErr
	}
	return strings.Join(lines, "\n"), resources, nil
}

// newLocalDocResource 校验本地文件并生成占位标记。dir 非空时相对路径基于 dir 解析。
func newLocalDocResource(kind, pathValue, dir string) (*localDocResource, error) {
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
	res := &localDocResource{Kind: kind, Path: p, Marker: prefix + hex.EncodeToString(raw), FileName: filepath.Base(p)}
	if kind == "image" {
		res.Width, res.Height = decodeImagePixelSize(p)
	}
	return res, nil
}

// finalizeLocalDocResources 上传并绑定占位块；失败项清理占位块。返回逐项结果与是否全部成功。
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
	for start := 0; start < len(ready); start += localResourceBindBatch {
		end := min(start+localResourceBindBatch, len(ready))
		chunk := ready[start:end]
		var reqs []map[string]any
		for _, r := range chunk {
			if r.Kind == "file" {
				reqs = append(reqs, map[string]any{"block_id": r.BlockID, "replace_file": map[string]any{"token": r.FileToken}})
				continue
			}
			ri := map[string]any{"token": r.FileToken}
			if r.Width > 0 && r.Height > 0 {
				ri["width"], ri["height"] = r.Width, r.Height
			}
			reqs = append(reqs, map[string]any{"block_id": r.BlockID, "replace_image": ri})
		}
		payload, _ := jsonMarshalNoEscape(reqs)
		clientToken := client.NewClientToken()
		res := client.DoVoidWithRetry(func() (http.Header, error) {
			_, h, err := client.BatchUpdateBlocks(p.documentID, payload, client.BatchUpdateBlocksOptions{
				ClientToken: clientToken, UserAccessToken: p.userToken,
			})
			return h, err
		}, client.RetryConfig{MaxRetries: 2, MaxTotalAttempts: 5, RetryOnRateLimit: true})
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
		_, err := p.sendUpdate(body, -1)
		for _, r := range cleanupOwners {
			if err != nil {
				r.Cleanup = "failed: " + err.Error()
			} else {
				r.Cleanup = "deleted"
			}
		}
	}
	return ok
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
