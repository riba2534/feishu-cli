package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/converter"
)

// createdBlockNode 记录一个已在服务端创建的块：转换节点、块 ID、父块 ID 与服务端返回的块
// （含 children、board.token 等建块响应字段，供附件 View→File、画板 token 等后续步骤使用）。
type createdBlockNode struct {
	node     *converter.BlockNode
	blockID  string
	parentID string
	created  *larkdocx.Block
}

// blockContentRejectedCodes 是建块接口「这批内容本身不被接受」的业务码：整批原子失败，
// 逐块重建可以把问题块隔离出来，其余内容照常写入（实测：带 token 的 Image/File/Board/Sheet/Bitable 报 1770001，
// QuoteContainer 嵌套报 1770030，显式创建 grid_column 报 1770028，字段越界报 99992402）。
// 1770035（单次请求画板块超限）正常不会出现——分批时已按 maxBoardsPerCreate 切开，列在这里作为兜底：万一触发就逐块重建。
var blockContentRejectedCodes = []int{1770001, 1770028, 1770030, 1770035, 99992402}

// maxBoardsPerCreate 单次建块请求中画板块（Board）的上限。实测：同一请求 6 个画板即报
// 1770035 resource count exceed limit，5 个成功且与文档已有画板数量无关；图片、附件不受此限
// （单批 50 张图片、20 个附件、5 画板 + 45 图片均成功）。
const maxBoardsPerCreate = 5

// blockBatchRanges 把 blocks 切成连续批次 [start, end)：每批不超过 maxBlocks 个块、不超过
// maxBoardsPerCreate 个画板块，保持原有顺序。
func blockBatchRanges(blocks []*larkdocx.Block, maxBlocks int) [][2]int {
	var ranges [][2]int
	start, boards := 0, 0
	for i, b := range blocks {
		isBoard := b != nil && b.BlockType != nil && *b.BlockType == int(converter.BlockTypeBoard)
		if i-start >= maxBlocks || (isBoard && boards >= maxBoardsPerCreate) {
			ranges = append(ranges, [2]int{start, i})
			start, boards = i, 0
		}
		if isBoard {
			boards++
		}
	}
	if start < len(blocks) {
		ranges = append(ranges, [2]int{start, len(blocks)})
	}
	return ranges
}

func isBlockContentRejected(err error) bool {
	for _, code := range blockContentRejectedCodes {
		if client.HasAPICode(err, code) {
			return true
		}
	}
	return false
}

// blockLabel 返回块的可读类型名，用于失败明细。
func blockLabel(b *larkdocx.Block) string {
	if b == nil || b.BlockType == nil {
		return "未知块"
	}
	return converter.BlockTypeName(converter.BlockType(*b.BlockType))
}

// createChildrenIsolated 在 parentID 下按顺序追加创建 blocks（每批 ≤50 块且 ≤5 个画板，复用幂等 client_token 重试）。
// 某批因内容被拒（见 blockContentRejectedCodes）整批失败时，改为逐块创建以隔离问题块：
// 被拒的块记入 blockErrs、其余块照常写入。返回值 created 与 blocks 一一对应（失败位置为 nil）。
// 非内容类错误（权限、网络重试耗尽等）作为 fatalErr 返回并停止后续创建——此时继续写只会重复失败。
func createChildrenIsolated(documentID, parentID string, blocks []*larkdocx.Block, userAccessToken string) (created []*larkdocx.Block, blockErrs []error, fatalErr error) {
	created = make([]*larkdocx.Block, len(blocks))
	retryCfg := client.RetryConfig{MaxRetries: 5, RetryOnRateLimit: true}
	const batchSize = 50
	for _, r := range blockBatchRanges(blocks, batchSize) {
		i, end := r[0], r[1]
		res := client.CreateBlockWithRetry(documentID, parentID, blocks[i:end], -1, retryCfg, userAccessToken)
		if res.Err == nil {
			for k, b := range res.Value {
				if i+k < end {
					created[i+k] = b
				}
			}
			continue
		}
		if !isBlockContentRejected(res.Err) || end-i == 1 {
			if isBlockContentRejected(res.Err) {
				blockErrs = append(blockErrs, fmt.Errorf("%s 创建失败（已跳过该块）: %w", blockLabel(blocks[i]), res.Err))
				continue
			}
			return created, blockErrs, fmt.Errorf("创建块失败 (parent=%s): %w", parentID, res.Err)
		}
		// 整批被拒：逐块重建，隔离问题块（失败的单块本身是原子的，不会产生重复内容）
		for k := i; k < end; k++ {
			one := client.CreateBlockWithRetry(documentID, parentID, blocks[k:k+1], -1, retryCfg, userAccessToken)
			if one.Err != nil {
				if !isBlockContentRejected(one.Err) {
					return created, blockErrs, fmt.Errorf("创建块失败 (parent=%s): %w", parentID, one.Err)
				}
				blockErrs = append(blockErrs, fmt.Errorf("%s 创建失败（已跳过该块）: %w", blockLabel(blocks[k]), one.Err))
				continue
			}
			if len(one.Value) > 0 {
				created[k] = one.Value[0]
			}
		}
	}
	return created, blockErrs, nil
}

// createChildrenOf 为已创建的块写入其子块。分栏（Grid）的列由服务端按 column_size 自动生成，
// 子块要写进服务端列里，不能再显式创建 GridColumn（1770028）。
func createChildrenOf(documentID string, parent createdBlockNode, userAccessToken string) (int, []createdBlockNode, error) {
	children := parent.node.Children
	if len(children) == 0 {
		return 0, nil, nil
	}
	if isGridColumnNodes(children) {
		var columnIDs []string
		if parent.created != nil {
			columnIDs = parent.created.Children
		}
		return fillGridColumns(documentID, parent.blockID, columnIDs, children, userAccessToken)
	}
	return createNestedChildren(documentID, parent.blockID, children, userAccessToken)
}

// isGridColumnNodes 判断子节点是否为分栏列（转换器为 <grid> 生成的 GridColumn 节点）。
func isGridColumnNodes(nodes []*converter.BlockNode) bool {
	if len(nodes) == 0 {
		return false
	}
	for _, n := range nodes {
		if n == nil || n.Block == nil || n.Block.BlockType == nil || *n.Block.BlockType != int(converter.BlockTypeGridColumn) {
			return false
		}
	}
	return true
}

// fillGridColumns 把转换器生成的每列内容写入服务端自动生成的分栏列，并删除列内自动生成的空文本块。
// columnIDs 来自建 Grid 的响应（children），缺失时回读分栏子块。
func fillGridColumns(documentID, gridID string, columnIDs []string, columns []*converter.BlockNode, userAccessToken string) (int, []createdBlockNode, error) {
	ids := columnIDs
	for attempt := 0; attempt < 3 && len(ids) < len(columns); attempt++ {
		if attempt > 0 {
			time.Sleep(300 * time.Millisecond)
		}
		kids, err := client.GetAllBlockChildren(documentID, gridID, userAccessToken)
		if err != nil {
			continue
		}
		ids = ids[:0:0]
		for _, k := range kids {
			if k.BlockId != nil {
				ids = append(ids, *k.BlockId)
			}
		}
	}
	if len(ids) == 0 {
		return 0, nil, fmt.Errorf("分栏 %s 未返回分栏列，%d 列内容未写入", gridID, len(columns))
	}

	var errs []error
	total := 0
	var createdNodes []createdBlockNode
	for i, col := range columns {
		if i >= len(ids) {
			errs = append(errs, fmt.Errorf("分栏 %s 只有 %d 列，第 %d 列内容未写入", gridID, len(ids), i+1))
			continue
		}
		colID := ids[i]
		createdNodes = append(createdNodes, createdBlockNode{node: col, blockID: colID, parentID: gridID})
		content := gridColumnContent(col.Children)
		if len(content) == 0 {
			continue // 空列保留服务端自带的空文本块
		}
		n, nested, err := createNestedChildren(documentID, colID, content, userAccessToken)
		total += n
		createdNodes = append(createdNodes, nested...)
		if err != nil {
			errs = append(errs, err)
		}
		if n > 0 {
			deleteContainerAutoEmptyBlock(documentID, colID, int(converter.BlockTypeGridColumn), userAccessToken)
		}
	}
	return total, createdNodes, errors.Join(errs...)
}

// gridColumnContent 返回列内需要写入的子块；转换器为空列补的单个空文本块视为无内容。
func gridColumnContent(children []*converter.BlockNode) []*converter.BlockNode {
	if len(children) == 1 && children[0] != nil && children[0].Block != nil && isEmptyTextBlock(children[0].Block) && len(children[0].Children) == 0 {
		return nil
	}
	return children
}

// ===== 资源补齐任务（图片 / 附件与视频 / 画板复制）=====

// mediaTaskSet 汇总阶段一收集的、需要在阶段二补齐内容的块。
type mediaTaskSet struct {
	images []imageTask
	files  []videoTask // 附件与视频（File 块）
	boards []boardCopyTask
}

func (m mediaTaskSet) videoCount() int {
	n := 0
	for _, f := range m.files {
		if f.video {
			n++
		}
	}
	return n
}

// boardCopyTask 表示把源画板（<whiteboard token>）的节点复制到新建空画板的任务。
type boardCopyTask struct {
	index        int
	blockID      string // 新建 Board 块 ID
	parentID     string
	whiteboardID string // 新建画板 token（服务端生成）
	sourceToken  string // 源画板 token
}

// collectMediaTasks 按树序遍历已创建的节点，依据转换器登记的 MediaRef 生成补齐任务。
func collectMediaTasks(set *mediaTaskSet, refs map[*larkdocx.Block]*converter.MediaRef, created []createdBlockNode, basePath string) {
	for _, cn := range created {
		if cn.node == nil || cn.node.Block == nil || cn.blockID == "" {
			continue
		}
		ref := refs[cn.node.Block]
		if ref == nil {
			continue
		}
		switch ref.Kind {
		case converter.MediaKindImage:
			set.images = append(set.images, imageTask{
				index:        len(set.images) + 1,
				imageBlockID: cn.blockID,
				parentID:     cn.parentID,
				source:       ref.UploadSource(),
				basePath:     basePath,
				width:        ref.Width,
				height:       ref.Height,
				align:        ref.Align,
				reuseToken:   ref.Token,
			})
		case converter.MediaKindFile:
			// 建 File 块时服务端返回外层 View 块（block_type=33），素材要上传到其 children[0]（File 块）
			fileBlockID := cn.blockID
			if cn.created != nil && cn.created.BlockType != nil && *cn.created.BlockType == int(converter.BlockTypeView) && len(cn.created.Children) > 0 {
				fileBlockID = cn.created.Children[0]
			}
			set.files = append(set.files, videoTask{
				index:       len(set.files) + 1,
				fileBlockID: fileBlockID,
				viewBlockID: cn.blockID,
				parentID:    cn.parentID,
				source:      ref.UploadSource(),
				basePath:    basePath,
				name:        ref.Name,
				video:       ref.Video,
				reuseToken:  ref.Token,
			})
		case converter.MediaKindWhiteboard:
			wb := ""
			if cn.created != nil && cn.created.Board != nil {
				wb = client.StringVal(cn.created.Board.Token)
			}
			set.boards = append(set.boards, boardCopyTask{
				index:        len(set.boards) + 1,
				blockID:      cn.blockID,
				parentID:     cn.parentID,
				whiteboardID: wb,
				sourceToken:  ref.Token,
			})
		}
	}
}

// downloadFeishuMedia 把飞书素材 token 下载到临时文件（素材复用：下载 → 重新上传到新块）。
// 先走临时下载链接，失败再走 SDK 直下（与 doc export --download-images 一致）。素材 token 绑定源文档，
// 当前身份没有源文档读权限时下载失败，调用方据此降级为占位文本并计入 failures。
func downloadFeishuMedia(token, name, defaultExt, userAccessToken string) (string, string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "feishu-media-*")
	if err != nil {
		return "", "", nil, fmt.Errorf("创建临时目录失败: %w", err)
	}
	cleanup := func() { os.RemoveAll(tmpDir) }
	tmpPath := filepath.Join(tmpDir, "media")
	opts := client.DownloadMediaOptions{UserAccessToken: userAccessToken}
	var dlErr error
	if tmpURL, urlErr := client.GetMediaTempURL(token, opts); urlErr == nil {
		dlErr = client.DownloadFromURL(tmpURL, tmpPath)
	} else {
		dlErr = urlErr
	}
	if dlErr != nil {
		if sdkErr := client.DownloadMedia(token, tmpPath, opts); sdkErr != nil {
			cleanup()
			return "", "", nil, fmt.Errorf("下载素材 %s 失败: %s（素材不存在或当前身份无源文档读权限；可改用 doc export --download-images 落地资源后再导入，或用 --user-access-token 以有权限的身份导入）",
				token, conciseDownloadError(sdkErr))
		}
	}
	ext := sniffMediaExtension(tmpPath)
	if ext == "" {
		ext = filepath.Ext(name)
	}
	if ext == "" {
		ext = defaultExt
	}
	fileName := strings.TrimSpace(name)
	if fileName == "" || fileName == "." || fileName == "/" {
		fileName = token + ext
	} else if filepath.Ext(fileName) == "" {
		fileName += ext
	}
	return tmpPath, fileName, cleanup, nil
}

// conciseDownloadError 压缩 SDK 下载失败的错误信息：SDK 对非 JSON 响应会把整份响应头拼进错误串，
// 这里只保留 HTTP 状态码或首段描述，避免 failures / 占位文本被几千字符的响应头淹没。
func conciseDownloadError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if i := strings.Index(msg, "StatusCode: "); i >= 0 {
		rest := msg[i+len("StatusCode: "):]
		if j := strings.IndexAny(rest, ", "); j > 0 {
			rest = rest[:j]
		}
		return "HTTP " + rest
	}
	if i := strings.Index(msg, ", response:"); i > 0 {
		msg = msg[:i]
	}
	return truncateRunes(msg, 200)
}

// replaceBlockWithText 删除 parentID 下的 blockID，并在原位置插入一段占位文本（用于无法补齐内容的空块：
// 图片/附件/视频素材复用失败、画板复制失败），避免文档里留下用户看不出原因的空块。
func replaceBlockWithText(documentID, parentID, blockID, text, userAccessToken string) error {
	if parentID == "" {
		parentID = documentID
	}
	children, err := client.GetAllBlockChildren(documentID, parentID, userAccessToken)
	if err != nil {
		return fmt.Errorf("获取子块失败: %w", err)
	}
	idx := -1
	for i, child := range children {
		if child.BlockId != nil && *child.BlockId == blockID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("块 %s 不在父块 %s 下", blockID, parentID)
	}
	if _, err := client.DeleteBlocks(documentID, parentID, idx, idx+1, userAccessToken); err != nil {
		return fmt.Errorf("删除空块失败: %w", err)
	}
	textType := int(converter.BlockTypeText)
	textBlock := &larkdocx.Block{
		BlockType: &textType,
		Text: &larkdocx.Text{Elements: []*larkdocx.TextElement{
			{TextRun: &larkdocx.TextRun{Content: &text}},
		}},
	}
	if _, _, err := client.CreateBlock(documentID, parentID, []*larkdocx.Block{textBlock}, idx, userAccessToken); err != nil {
		return fmt.Errorf("插入占位文本失败: %w", err)
	}
	return nil
}

// boardCloneBatchSize / boardCloneInterval 画板节点复制的分批参数（与 board clone 默认一致的保守节流）。
const (
	boardCloneBatchSize = 10
	boardCloneInterval  = 500 * time.Millisecond
)

// boardCopyResult 描述画板复制最终采用的方式。
type boardCopyResult string

const (
	boardCopiedBySyntax boardCopyResult = "syntax" // 源画板由 Mermaid/PlantUML 生成：用节点里保存的源码重新导入（可编辑）
	boardCopiedByClone  boardCopyResult = "clone"  // 逐节点复制（先形状后连线，连线端点重映射）
	boardCopiedAsImage  boardCopyResult = "image"  // 前两种都失败：把源画板导出为图片插入（不可编辑，设计内降级）
)

// processBoardCopyTask 把源画板（<whiteboard token>）复制到新建的空画板：
//  1. 源画板由 Mermaid/PlantUML 生成时（section 节点带 syntax.code），用源码重新导入，结果可编辑；
//  2. 否则逐节点复制；
//  3. 以上失败时把空画板块替换为源画板的图片；仍失败则替换为占位文本并返回错误（计入 failures）。
func processBoardCopyTask(documentID string, task boardCopyTask, verbose bool, userAccessToken string) (boardCopyResult, error) {
	how, err := copyWhiteboard(task.sourceToken, task.whiteboardID, userAccessToken)
	if err == nil {
		if verbose {
			syncPrintf("  ✓ 画板 %d 复制成功 (源 %s, 方式 %s)\n", task.index, task.sourceToken, how)
		}
		return how, nil
	}
	syncPrintf("  ⚠ 画板 %d 无法复制为可编辑画板 (源 %s): %v；改为插入源画板图片\n", task.index, task.sourceToken, err)
	imgErr := replaceBoardWithImage(documentID, task, userAccessToken)
	if imgErr == nil {
		return boardCopiedAsImage, nil
	}
	syncPrintf("  ✗ 画板 %d 降级为图片也失败: %v\n", task.index, imgErr)
	placeholder := fmt.Sprintf("[画板未能复制: 源画板 %s]", task.sourceToken)
	if phErr := replaceBlockWithText(documentID, task.parentID, task.blockID, placeholder, userAccessToken); phErr != nil {
		syncPrintf("  ⚠ 画板 %d 占位失败: %v\n", task.index, phErr)
	}
	return "", fmt.Errorf("复制画板失败: %v；降级为图片失败: %w", err, imgErr)
}

// replaceBoardWithImage 把空画板块替换为源画板导出的图片（删除画板块 → 原位置建空 Image 块 → 上传 → replace_image）。
func replaceBoardWithImage(documentID string, task boardCopyTask, userAccessToken string) error {
	tmpDir, err := os.MkdirTemp("", "feishu-board-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	imgPath, err := client.GetBoardImage(task.sourceToken, filepath.Join(tmpDir, "board"), userAccessToken)
	if err != nil {
		return fmt.Errorf("导出源画板图片失败: %w", err)
	}

	parentID := task.parentID
	if parentID == "" {
		parentID = documentID
	}
	children, err := client.GetAllBlockChildren(documentID, parentID, userAccessToken)
	if err != nil {
		return fmt.Errorf("获取子块失败: %w", err)
	}
	idx := -1
	for i, child := range children {
		if child.BlockId != nil && *child.BlockId == task.blockID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("画板块 %s 不在父块 %s 下", task.blockID, parentID)
	}
	if _, err := client.DeleteBlocks(documentID, parentID, idx, idx+1, userAccessToken); err != nil {
		return fmt.Errorf("删除空画板失败: %w", err)
	}
	imageType := int(converter.BlockTypeImage)
	created, _, err := client.CreateBlock(documentID, parentID, []*larkdocx.Block{{BlockType: &imageType, Image: &larkdocx.Image{}}}, idx, userAccessToken)
	if err != nil || len(created) == 0 || created[0].BlockId == nil {
		if err == nil {
			err = fmt.Errorf("未返回图片块 ID")
		}
		return fmt.Errorf("创建图片块失败: %w", err)
	}
	imageBlockID := *created[0].BlockId
	extra := fmt.Sprintf(`{"drive_route_token":"%s"}`, documentID)
	retryCfg := client.RetryConfig{MaxRetries: 3, RetryOnRateLimit: true}
	up := client.DoWithRetry(func() (string, http.Header, error) {
		return client.UploadMediaWithExtra(imgPath, "docx_image", imageBlockID, filepath.Base(imgPath), extra, userAccessToken)
	}, retryCfg)
	if up.Err == nil {
		pxW, pxH := decodeImagePixelSize(imgPath)
		rep := client.DoVoidWithRetry(func() (http.Header, error) {
			return client.ReplaceImage(documentID, imageBlockID, up.Value, client.ReplaceImageOptions{Width: pxW, Height: pxH}, userAccessToken)
		}, retryCfg)
		if rep.Err == nil {
			return nil
		}
		up.Err = rep.Err
	}
	// 上传/绑定失败：把刚建的空图片块再替换为占位文本，交由调用方记录失败
	placeholder := fmt.Sprintf("[画板未能复制: 源画板 %s]", task.sourceToken)
	if phErr := replaceBlockWithText(documentID, parentID, imageBlockID, placeholder, userAccessToken); phErr != nil {
		syncPrintf("  ⚠ 画板 %d 占位失败: %v\n", task.index, phErr)
	}
	return fmt.Errorf("上传画板图片失败: %w", up.Err)
}

// boardSyntaxSource 是画板节点里保存的图表源码（Mermaid/PlantUML 生成的画板在 section 节点的 syntax 字段）。
type boardSyntaxSource struct {
	code   string
	syntax string // mermaid / plantuml
}

// diagramSourcesOfBoard 当画板的全部节点都属于「带 syntax 源码的节点」或其后代时，返回这些源码；否则返回 nil。
func diagramSourcesOfBoard(nodes []map[string]any) []boardSyntaxSource {
	byID := map[string]map[string]any{}
	for _, n := range nodes {
		if id, _ := n["id"].(string); id != "" {
			byID[id] = n
		}
	}
	var sources []boardSyntaxSource
	isSyntax := func(n map[string]any) bool {
		syn, _ := n["syntax"].(map[string]any)
		code, _ := syn["code"].(string)
		return strings.TrimSpace(code) != ""
	}
	for _, n := range nodes {
		if !isSyntax(n) {
			continue
		}
		syn := n["syntax"].(map[string]any)
		code, _ := syn["code"].(string)
		kind := "mermaid"
		if t, ok := syn["syntax_type"].(float64); ok && int(t) == 1 {
			kind = "plantuml"
		}
		sources = append(sources, boardSyntaxSource{code: code, syntax: kind})
	}
	if len(sources) == 0 {
		return nil
	}
	// 每个节点都必须能沿 parent_id 追溯到某个 syntax 节点，否则说明画板上还有手工添加的内容，不能只按源码重建
	for _, n := range nodes {
		cur, ok := n, true
		for depth := 0; ok && depth < 64; depth++ {
			if isSyntax(cur) {
				break
			}
			pid, _ := cur["parent_id"].(string)
			cur, ok = byID[pid]
		}
		if !ok || !isSyntax(cur) {
			return nil
		}
	}
	return sources
}

// copyWhiteboard 读取源画板节点并写入目标画板：优先按图表源码重新导入，否则逐节点复制。
func copyWhiteboard(srcID, dstID, userAccessToken string) (boardCopyResult, error) {
	if dstID == "" {
		return "", fmt.Errorf("新建画板未返回画板 token")
	}
	raw, err := client.GetBoardNodes(srcID, userAccessToken)
	if err != nil {
		return "", fmt.Errorf("读取源画板节点失败: %w", err)
	}
	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Nodes []map[string]any `json:"nodes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return "", fmt.Errorf("解析源画板节点失败: %w", err)
	}
	if apiResp.Code != 0 {
		return "", fmt.Errorf("读取源画板节点失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	nodes := apiResp.Data.Nodes
	if len(nodes) == 0 {
		return boardCopiedByClone, nil // 源画板本身为空
	}

	if sources := diagramSourcesOfBoard(nodes); len(sources) > 0 {
		var syntaxErr error
		for i, src := range sources {
			res := client.DoWithRetry(func() (*client.ImportDiagramResult, http.Header, error) {
				return client.ImportDiagram(dstID, src.code, client.ImportDiagramOptions{
					SourceType:      "content",
					Syntax:          src.syntax,
					UserAccessToken: userAccessToken,
					Overwrite:       i == 0, // 首个图覆盖写入，重试不会叠出重复图
				})
			}, client.RetryConfig{MaxRetries: 3, RetryOnRateLimit: true, IsPermanent: client.IsPermanentError})
			if res.Err != nil {
				syntaxErr = res.Err
				break
			}
		}
		if syntaxErr == nil {
			return boardCopiedBySyntax, nil
		}
		// 源码重新导入失败（如服务端不再支持该语法）：退回逐节点复制
	}

	return boardCopiedByClone, cloneBoardNodes(nodes, dstID, userAccessToken)
}

// cloneBoardNodes 逐节点复制（逻辑同 board clone：移除只读字段、先形状后连线、连线端点按新 ID 重映射）。
func cloneBoardNodes(nodes []map[string]any, dstID, userAccessToken string) error {
	var shapes, conns []map[string]any
	for _, n := range nodes {
		if t, _ := n["type"].(string); t == "connector" {
			conns = append(conns, n)
		} else {
			shapes = append(shapes, n)
		}
	}
	idMap := map[string]string{}
	post := func(batchNodes []map[string]any, oldIDs []string) error {
		for i := 0; i < len(batchNodes); i += boardCloneBatchSize {
			end := min(i+boardCloneBatchSize, len(batchNodes))
			body, _ := json.Marshal(batchNodes[i:end])
			res := client.DoWithRetry(func() ([]string, http.Header, error) {
				ids, err := client.CreateBoardNodes(dstID, string(body), client.CreateBoardNotesOptions{UserAccessToken: userAccessToken})
				return ids, nil, err
			}, client.RetryConfig{MaxRetries: 3, RetryOnRateLimit: true})
			if res.Err != nil {
				return fmt.Errorf("写入画板节点 %d-%d 失败: %w", i, end, res.Err)
			}
			for k, id := range res.Value {
				if oldIDs != nil && i+k < len(oldIDs) {
					idMap[oldIDs[i+k]] = id
				}
			}
			if end < len(batchNodes) {
				time.Sleep(boardCloneInterval)
			}
		}
		return nil
	}
	sanitized := make([]map[string]any, 0, len(shapes))
	oldIDs := make([]string, 0, len(shapes))
	for _, n := range shapes {
		id, _ := n["id"].(string)
		oldIDs = append(oldIDs, id)
		sanitized = append(sanitized, sanitizeNode(n))
	}
	if err := post(sanitized, oldIDs); err != nil {
		return err
	}
	var rewired []map[string]any
	for _, n := range conns {
		s := sanitizeNode(n)
		if rewireConnector(s, idMap) {
			rewired = append(rewired, s)
		}
	}
	return post(rewired, nil)
}
