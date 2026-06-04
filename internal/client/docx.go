package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
)

// 块类型常量（避免魔术数字）
const (
	blockTypeText   = 2
	blockTypeBullet = 12
	BlockTypeFile   = 23
	BlockTypeImage  = 27
	blockTypeBoard  = 43
)

// CreateDocument creates a new document
func CreateDocument(title string, folderToken string, userAccessToken ...string) (*larkdocx.Document, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larkdocx.NewCreateDocumentReqBuilder().
		Body(larkdocx.NewCreateDocumentReqBodyBuilder().
			Title(title).
			FolderToken(folderToken).
			Build()).
		Build()

	resp, err := client.Docx.Document.Create(Context(), req, UserTokenOption(firstString(userAccessToken))...)
	if err != nil {
		return nil, fmt.Errorf("创建文档失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("创建文档失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data.Document, nil
}

// GetDocument 获取文档信息（仅使用 App/Tenant Token）。
// 保留作为 GetDocumentWithToken 的向后兼容 wrapper。
func GetDocument(documentID string) (*larkdocx.Document, error) {
	return GetDocumentWithToken(documentID, "")
}

// GetDocumentWithToken 获取文档信息，可选使用 User Access Token。
// 当 userAccessToken 非空时以用户身份访问（可读取用户有权限但 App 无权限的文档）；
// 为空时回退到 App/Tenant Token。
func GetDocumentWithToken(documentID string, userAccessToken string) (*larkdocx.Document, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larkdocx.NewGetDocumentReqBuilder().
		DocumentId(documentID).
		Build()

	resp, err := client.Docx.Document.Get(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("获取文档失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取文档失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data.Document, nil
}

// GetRawContent retrieves raw JSON content of a document
func GetRawContent(documentID string, userAccessToken ...string) (string, error) {
	client, err := GetClient()
	if err != nil {
		return "", err
	}

	req := larkdocx.NewRawContentDocumentReqBuilder().
		DocumentId(documentID).
		Build()

	resp, err := client.Docx.Document.RawContent(Context(), req, UserTokenOption(firstString(userAccessToken))...)
	if err != nil {
		return "", fmt.Errorf("获取原始内容失败: %w", err)
	}

	if !resp.Success() {
		return "", fmt.Errorf("获取原始内容失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data.Content == nil {
		return "", nil
	}

	return *resp.Data.Content, nil
}

// ListBlocks retrieves all blocks in a document
func ListBlocks(documentID string, pageToken string, pageSize int) ([]*larkdocx.Block, string, error) {
	return ListBlocksWithToken(documentID, pageToken, pageSize, "")
}

// ListBlocksWithToken retrieves all blocks in a document, optionally using a User Access Token
func ListBlocksWithToken(documentID string, pageToken string, pageSize int, userAccessToken string) ([]*larkdocx.Block, string, error) {
	client, err := GetClient()
	if err != nil {
		return nil, "", err
	}

	reqBuilder := larkdocx.NewListDocumentBlockReqBuilder().
		DocumentId(documentID).
		PageSize(pageSize)

	if pageToken != "" {
		reqBuilder.PageToken(pageToken)
	}

	resp, err := client.Docx.DocumentBlock.List(Context(), reqBuilder.Build(), UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, "", fmt.Errorf("获取块列表失败: %w", err)
	}

	if !resp.Success() {
		return nil, "", fmt.Errorf("获取块列表失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	nextPageToken := StringVal(resp.Data.PageToken)

	return resp.Data.Items, nextPageToken, nil
}

// GetAllBlocks retrieves all blocks in a document with pagination
func GetAllBlocks(documentID string) ([]*larkdocx.Block, error) {
	return GetAllBlocksWithToken(documentID, "")
}

// GetAllBlocksWithToken retrieves all blocks in a document with pagination, optionally using a User Access Token
func GetAllBlocksWithToken(documentID string, userAccessToken string) ([]*larkdocx.Block, error) {
	var allBlocks []*larkdocx.Block
	pageToken := ""
	pageSize := 500
	pageCount := 0
	const maxPages = 1000 // 防止无限分页

	for {
		if pageCount >= maxPages {
			return nil, fmt.Errorf("超过最大分页限制 %d，文档可能有异常", maxPages)
		}
		blocks, nextToken, err := ListBlocksWithToken(documentID, pageToken, pageSize, userAccessToken)
		if err != nil {
			return nil, err
		}

		allBlocks = append(allBlocks, blocks...)

		if nextToken == "" {
			break
		}
		pageToken = nextToken
		pageCount++
	}

	return allBlocks, nil
}

// GetBlock retrieves a specific block
func GetBlock(documentID string, blockID string, userAccessToken ...string) (*larkdocx.Block, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larkdocx.NewGetDocumentBlockReqBuilder().
		DocumentId(documentID).
		BlockId(blockID).
		Build()

	resp, err := client.Docx.DocumentBlock.Get(Context(), req, UserTokenOption(firstString(userAccessToken))...)
	if err != nil {
		return nil, fmt.Errorf("获取块失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取块失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data.Block, nil
}

// CreateBlock creates a new block under a parent block.
// 受单文档 3 QPS 写限制：调用 SDK 之前先过 docWriteLimiter（issue #159）。
func CreateBlock(documentID string, blockID string, children []*larkdocx.Block, index int, userAccessToken ...string) ([]*larkdocx.Block, http.Header, error) {
	client, err := GetClient()
	if err != nil {
		return nil, nil, err
	}

	req := larkdocx.NewCreateDocumentBlockChildrenReqBuilder().
		DocumentId(documentID).
		BlockId(blockID).
		DocumentRevisionId(-1).
		Body(larkdocx.NewCreateDocumentBlockChildrenReqBodyBuilder().
			Children(children).
			Index(index).
			Build()).
		Build()

	if err := AcquireDocWriteSlot(ContextWithTimeout(DocWriteSlotAcquireTimeout), documentID); err != nil {
		return nil, nil, fmt.Errorf("等待文档写入配额失败: %w", err)
	}
	resp, err := client.Docx.DocumentBlockChildren.Create(Context(), req, UserTokenOption(firstString(userAccessToken))...)
	if err != nil {
		return nil, nil, fmt.Errorf("创建块失败: %w", err)
	}

	headers := resp.ApiResp.Header
	if !resp.Success() {
		return nil, headers, fmt.Errorf("创建块失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data.Children, headers, nil
}

// UpdateBlock updates an existing block.
// 受单文档 3 QPS 写限制：调用 SDK 之前先过 docWriteLimiter（issue #159）。
// InsertTableRow/InsertTableColumn/Delete/MergeTableCells 等都委托到本函数，因此自动受限。
func UpdateBlock(documentID string, blockID string, updateContent any, userAccessToken ...string) (http.Header, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	// The updateContent should be marshaled to the appropriate update request body
	contentBytes, err := json.Marshal(updateContent)
	if err != nil {
		return nil, fmt.Errorf("序列化更新内容失败: %w", err)
	}

	var updateBody larkdocx.UpdateBlockRequest
	if err := json.Unmarshal(contentBytes, &updateBody); err != nil {
		return nil, fmt.Errorf("反序列化更新内容失败: %w", err)
	}

	req := larkdocx.NewPatchDocumentBlockReqBuilder().
		DocumentId(documentID).
		BlockId(blockID).
		DocumentRevisionId(-1).
		UpdateBlockRequest(&updateBody).
		Build()

	if err := AcquireDocWriteSlot(ContextWithTimeout(DocWriteSlotAcquireTimeout), documentID); err != nil {
		return nil, fmt.Errorf("等待文档写入配额失败: %w", err)
	}
	resp, err := client.Docx.DocumentBlock.Patch(Context(), req, UserTokenOption(firstString(userAccessToken))...)
	if err != nil {
		return nil, fmt.Errorf("更新块失败: %w", err)
	}

	headers := resp.ApiResp.Header
	if !resp.Success() {
		return headers, fmt.Errorf("更新块失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return headers, nil
}

// ReplaceImage replaces the image token of an Image block.
// 用于图片三步法上传的第三步：将上传后的 fileToken 设置到 Image Block。
func ReplaceImage(documentID, imageBlockID, fileToken string, userAccessToken ...string) (http.Header, error) {
	return UpdateBlock(documentID, imageBlockID, map[string]any{
		"replace_image": map[string]any{
			"token": fileToken,
		},
	}, userAccessToken...)
}

// DeleteBlocks deletes child blocks from a parent block by index range.
// 受单文档 3 QPS 写限制：调用 SDK 之前先过 docWriteLimiter（issue #159）。
// startIndex is the starting index (0-based), endIndex is exclusive
func DeleteBlocks(documentID string, blockID string, startIndex int, endIndex int, userAccessToken ...string) (http.Header, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larkdocx.NewBatchDeleteDocumentBlockChildrenReqBuilder().
		DocumentId(documentID).
		BlockId(blockID).
		DocumentRevisionId(-1).
		Body(larkdocx.NewBatchDeleteDocumentBlockChildrenReqBodyBuilder().
			StartIndex(startIndex).
			EndIndex(endIndex).
			Build()).
		Build()

	if err := AcquireDocWriteSlot(ContextWithTimeout(DocWriteSlotAcquireTimeout), documentID); err != nil {
		return nil, fmt.Errorf("等待文档写入配额失败: %w", err)
	}
	resp, err := client.Docx.DocumentBlockChildren.BatchDelete(Context(), req, UserTokenOption(firstString(userAccessToken))...)
	if err != nil {
		return nil, fmt.Errorf("删除块失败: %w", err)
	}

	headers := resp.ApiResp.Header
	if !resp.Success() {
		return headers, fmt.Errorf("删除块失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return headers, nil
}

// BatchUpdateBlocksOptions contains options for batch updating blocks
type BatchUpdateBlocksOptions struct {
	DocumentRevisionID int
	ClientToken        string
	UserIDType         string
	UserAccessToken    string
}

// BatchUpdateBlocksResult contains the result of batch updating blocks
type BatchUpdateBlocksResult struct {
	BlockIDs         []string `json:"block_ids"`
	DocumentRevision int      `json:"document_revision_id"`
}

// BatchUpdateBlocks batch updates blocks in a document.
// 受单文档 3 QPS 写限制：调用 SDK 之前先过 docWriteLimiter（issue #159）。
// 返回 http.Header 让 retry 层拿到 x-ogw-ratelimit-reset，做精确退避而非随机 jitter。
func BatchUpdateBlocks(documentID string, requestsJSON string, opts BatchUpdateBlocksOptions) (*BatchUpdateBlocksResult, http.Header, error) {
	client, err := GetClient()
	if err != nil {
		return nil, nil, err
	}

	// Default values
	if opts.DocumentRevisionID == 0 {
		opts.DocumentRevisionID = -1
	}
	if opts.UserIDType == "" {
		opts.UserIDType = "open_id"
	}

	// Parse requests
	var requests []*larkdocx.UpdateBlockRequest
	if err := json.Unmarshal([]byte(requestsJSON), &requests); err != nil {
		return nil, nil, fmt.Errorf("解析请求 JSON 失败: %w", err)
	}

	reqBuilder := larkdocx.NewBatchUpdateDocumentBlockReqBuilder().
		DocumentId(documentID).
		DocumentRevisionId(opts.DocumentRevisionID).
		UserIdType(opts.UserIDType).
		Body(larkdocx.NewBatchUpdateDocumentBlockReqBodyBuilder().
			Requests(requests).
			Build())

	if opts.ClientToken != "" {
		reqBuilder.ClientToken(opts.ClientToken)
	}

	if err := AcquireDocWriteSlot(ContextWithTimeout(DocWriteSlotAcquireTimeout), documentID); err != nil {
		return nil, nil, fmt.Errorf("等待文档写入配额失败: %w", err)
	}
	resp, err := client.Docx.DocumentBlock.BatchUpdate(Context(), reqBuilder.Build(), UserTokenOption(opts.UserAccessToken)...)
	if err != nil {
		return nil, nil, fmt.Errorf("批量更新块失败: %w", err)
	}

	headers := resp.ApiResp.Header
	if !resp.Success() {
		return nil, headers, fmt.Errorf("批量更新块失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	result := &BatchUpdateBlocksResult{}
	for _, block := range resp.Data.Blocks {
		if id := StringVal(block.BlockId); id != "" {
			result.BlockIDs = append(result.BlockIDs, id)
		}
	}
	result.DocumentRevision = IntVal(resp.Data.DocumentRevisionId)

	return result, headers, nil
}

// GetBlockChildren retrieves children of a block (first page only)
func GetBlockChildren(documentID string, blockID string, userAccessToken ...string) ([]*larkdocx.Block, http.Header, error) {
	client, err := GetClient()
	if err != nil {
		return nil, nil, err
	}

	req := larkdocx.NewGetDocumentBlockChildrenReqBuilder().
		DocumentId(documentID).
		BlockId(blockID).
		Build()

	resp, err := client.Docx.DocumentBlockChildren.Get(Context(), req, UserTokenOption(firstString(userAccessToken))...)
	if err != nil {
		return nil, nil, fmt.Errorf("获取子块失败: %w", err)
	}

	headers := resp.ApiResp.Header
	if !resp.Success() {
		return nil, headers, fmt.Errorf("获取子块失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data.Items, headers, nil
}

// GetAllBlockChildren retrieves all direct children of a block with pagination
func GetAllBlockChildren(documentID string, blockID string, userAccessToken ...string) ([]*larkdocx.Block, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	var allChildren []*larkdocx.Block
	pageToken := ""
	pageSize := 500
	const maxPages = 1000

	for page := 0; page < maxPages; page++ {
		reqBuilder := larkdocx.NewGetDocumentBlockChildrenReqBuilder().
			DocumentId(documentID).
			BlockId(blockID).
			PageSize(pageSize).
			DocumentRevisionId(-1)

		if pageToken != "" {
			reqBuilder.PageToken(pageToken)
		}

		resp, err := client.Docx.DocumentBlockChildren.Get(Context(), reqBuilder.Build(), UserTokenOption(firstString(userAccessToken))...)
		if err != nil {
			return nil, fmt.Errorf("获取子块失败: %w", err)
		}

		if !resp.Success() {
			return nil, fmt.Errorf("获取子块失败: code=%d, msg=%s", resp.Code, resp.Msg)
		}

		allChildren = append(allChildren, resp.Data.Items...)

		if !BoolVal(resp.Data.HasMore) {
			break
		}
		if next := StringVal(resp.Data.PageToken); next == "" {
			break
		} else {
			pageToken = next
		}
	}

	return allChildren, nil
}

// AddBoardResult contains the result of adding a board to document
type AddBoardResult struct {
	BlockID      string `json:"block_id"`
	WhiteboardID string `json:"whiteboard_id"`
}

// AddBoard adds a board block to document and returns the whiteboard ID.
// userAccessToken 非空时使用用户身份创建，避免需要用户权限的画板块误回退到租户身份。
func AddBoard(documentID string, parentID string, index int, userAccessToken ...string) (*AddBoardResult, http.Header, error) {
	if parentID == "" {
		parentID = documentID
	}

	// 构建画板块
	blockType := blockTypeBoard
	boardBlock := &larkdocx.Block{
		BlockType: &blockType,
		Board:     &larkdocx.Board{},
	}

	// 创建画板块
	createdBlocks, headers, err := CreateBlock(documentID, parentID, []*larkdocx.Block{boardBlock}, index, userAccessToken...)
	if err != nil {
		return nil, headers, fmt.Errorf("创建画板块失败: %w", err)
	}

	if len(createdBlocks) == 0 {
		return nil, headers, fmt.Errorf("创建画板块失败：未返回块信息")
	}

	result := &AddBoardResult{
		BlockID: StringVal(createdBlocks[0].BlockId),
	}
	if createdBlocks[0].Board != nil {
		result.WhiteboardID = StringVal(createdBlocks[0].Board.Token)
	}

	return result, headers, nil
}

// FillTableCells fills table cells with plain text content.
// cellIDs: cell block IDs from the created table
// contents: cell content strings (in row-major order)
// Note: Feishu API automatically creates an empty text block in each cell when creating a table,
// so we need to update the existing block instead of creating a new one to avoid duplicate rows.
func FillTableCells(documentID string, cellIDs []string, contents []string, userAccessToken ...string) error {
	if len(cellIDs) == 0 || len(contents) == 0 {
		return nil
	}

	cellCount := min(len(cellIDs), len(contents))

	// 构建每个单元格的元素
	cellElements := make([][]*larkdocx.TextElement, cellCount)
	for i := 0; i < cellCount; i++ {
		if contents[i] != "" {
			content := contents[i]
			cellElements[i] = []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: &content}},
			}
		}
	}

	return fillTableCellsInternal(documentID, cellIDs[:cellCount], cellElements, nil, firstString(userAccessToken))
}

// FillTableCellsRich fills table cells with rich text elements (preserving links, styles, etc.)
// cellIDs: cell block IDs from the created table
// cellElements: each cell's text elements (in row-major order)
// fallbackContents: plain text fallback for cells without elements
func FillTableCellsRich(documentID string, cellIDs []string, cellElements [][]*larkdocx.TextElement, fallbackContents []string, userAccessToken ...string) error {
	if len(cellIDs) == 0 {
		return nil
	}

	// 合并 elements 和 fallback
	merged := make([][]*larkdocx.TextElement, len(cellIDs))
	for i := range cellIDs {
		if i < len(cellElements) && len(cellElements[i]) > 0 {
			merged[i] = cellElements[i]
		} else if i < len(fallbackContents) && fallbackContents[i] != "" {
			content := fallbackContents[i]
			merged[i] = []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: &content}},
			}
		}
	}

	return fillTableCellsInternal(documentID, cellIDs, merged, nil, firstString(userAccessToken))
}

// FillTableCellsRichWithMap 是 FillTableCellsRich 的扩展版本，
// 接受预构建的 cellID -> textBlockID 映射（cmd 层在 import 阶段二开头一次性 GetAllBlocks 后传入），
// 让单 cell 路径直接走 batch_update，避免逐 cell GetBlockChildren。
// 当 cellMap == nil 或缺失某个 cellID 时，对应 cell 自动降级到旧路径。
func FillTableCellsRichWithMap(documentID string, cellIDs []string, cellElements [][]*larkdocx.TextElement, fallbackContents []string, cellMap map[string]string, userAccessToken ...string) error {
	if len(cellIDs) == 0 {
		return nil
	}

	merged := make([][]*larkdocx.TextElement, len(cellIDs))
	for i := range cellIDs {
		if i < len(cellElements) && len(cellElements[i]) > 0 {
			merged[i] = cellElements[i]
		} else if i < len(fallbackContents) && fallbackContents[i] != "" {
			content := fallbackContents[i]
			merged[i] = []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: &content}},
			}
		}
	}

	return fillTableCellsInternal(documentID, cellIDs, merged, cellMap, firstString(userAccessToken))
}

// fillBatchSize 是 batch_update 单次请求最多包含的 cell 个数。
// 飞书官方上限是 200，但每个 update_text_elements 体积可能很大，
// 加上单文档 3 QPS 限制，30 是经验上的甜蜜点（请求体小、批次粒度细、失败回滚损失小）。
const fillBatchSize = 30

// fillTableCellsInternal 是 FillTableCells / FillTableCellsRich / FillTableCellsRichWithMap 的统一实现。
//
// 改造前（v1.28.x 及以前）：每个 cell 串行 1 次 GetBlockChildren + 1 次 UpdateBlock + throttlePer5 sleep，
// 120 cells 大约 240 次 API + 20s 节流（issue #159 实测约 69s）。
//
// 改造后：
//  1. 用 splitCellElements 把 cell 分桶为 single-group（占绝大多数）和 multi-group（含 <br/> 等）；
//  2. single-group 优先走 BatchUpdateBlocks（≤30/批），整批失败时降级 per-cell；
//  3. multi-group 仍走 fillCellMultiBlocks 旧路径；
//  4. cellMap 命中时跳过 GetBlockChildren；缺失时自动 fallback；
//  5. 所有写请求前过 AcquireDocWriteSlot 做文档级 3 QPS 节流，多 worker 共用。
func fillTableCellsInternal(documentID string, cellIDs []string, cellElements [][]*larkdocx.TextElement, cellMap map[string]string, userAccessToken string) error {
	const maxRetries = 5

	var singles []singleCellTask
	type multiTask struct {
		cellID string
		groups []cellBlockGroup
		index  int
	}
	var multis []multiTask

	for i, cellID := range cellIDs {
		var elements []*larkdocx.TextElement
		if i < len(cellElements) {
			elements = cellElements[i]
		}
		if len(elements) == 0 {
			continue
		}

		groups := splitCellElements(elements)
		if len(groups) > 1 {
			multis = append(multis, multiTask{cellID: cellID, groups: groups, index: i})
			continue
		}
		tb := ""
		if cellMap != nil {
			tb = cellMap[cellID]
		}
		singles = append(singles, singleCellTask{
			cellID:      cellID,
			textBlockID: tb,
			elements:    elements,
			index:       i,
		})
	}

	// 1) single-group：批量优先；缺映射或批量失败时降级 per-cell
	if err := fillSingleCellsBatched(documentID, singles, maxRetries, userAccessToken); err != nil {
		return err
	}

	// 2) multi-group：保留旧路径（element 含 \n 的占比极少）
	for _, m := range multis {
		if err := fillCellMultiBlocks(documentID, m.cellID, m.groups, maxRetries, userAccessToken); err != nil {
			return fmt.Errorf("填充单元格 %d 失败: %w", m.index, err)
		}
	}

	return nil
}

// fillSingleCellsBatched 把 single-group cell 分批走 batch_update；
// 缺映射或整批失败时退回单 cell 路径，保留旧的 update-first-empty 语义。
//
// 限流：每次写 API（BatchUpdateBlocks/UpdateBlock/CreateBlock）在底层会自带 docLimiter，
// 本函数本身不再 acquire，避免 double。
func fillSingleCellsBatched(documentID string, tasks []singleCellTask, maxRetries int, userAccessToken string) error {
	if len(tasks) == 0 {
		return nil
	}

	// 分桶：有 textBlockID 走 batch；缺 textBlockID 立即走单 cell 路径
	batchable, fallbackOnly := partitionSingleCellTasks(tasks)

	for start := 0; start < len(batchable); start += fillBatchSize {
		end := min(start+fillBatchSize, len(batchable))
		chunk := batchable[start:end]

		if err := batchUpdateSingleCells(documentID, chunk, maxRetries, userAccessToken); err == nil {
			continue
		}
		// 整批失败：降级 per-cell（fillCellSingleBlock 走底层 UpdateBlock/CreateBlock，自带 acquire）
		fallbackOnly = append(fallbackOnly, chunk...)
	}

	for _, t := range fallbackOnly {
		if err := fillCellSingleBlock(documentID, t.cellID, t.elements, maxRetries, userAccessToken); err != nil {
			return fmt.Errorf("填充单元格 %d 失败: %w", t.index, err)
		}
	}
	return nil
}

// partitionSingleCellTasks 把 single-group cell 分为可批量（有 textBlockID）和必须降级（缺映射）两桶。
// 抽成纯函数便于单测验证分桶逻辑。
func partitionSingleCellTasks(tasks []singleCellTask) (batchable, fallback []singleCellTask) {
	for _, t := range tasks {
		if t.textBlockID != "" {
			batchable = append(batchable, t)
		} else {
			fallback = append(fallback, t)
		}
	}
	return
}

// singleCellTask 与 fillTableCellsInternal 内的 singleTask 同形，
// 抽出一个包级类型给 fillSingleCellsBatched / batchUpdateSingleCells 共用，避免函数签名里塞匿名 struct。
type singleCellTask struct {
	cellID      string
	textBlockID string
	elements    []*larkdocx.TextElement
	index       int
}

// batchUpdateSingleCells 调用 BatchUpdateBlocks 一次更新多个 cell 的文本块。
// 飞书 batch_update 限制：同一 block_id 不能重复出现 —— textBlockID 天然唯一，OK。
func batchUpdateSingleCells(documentID string, tasks []singleCellTask, maxRetries int, userAccessToken string) error {
	if len(tasks) == 0 {
		return nil
	}

	requests := make([]map[string]any, 0, len(tasks))
	for _, t := range tasks {
		requests = append(requests, map[string]any{
			"block_id":             t.textBlockID,
			"update_text_elements": map[string]any{"elements": buildElementsJSON(t.elements)},
		})
	}
	payload, err := json.Marshal(requests)
	if err != nil {
		return fmt.Errorf("序列化批量更新请求失败: %w", err)
	}

	cfg := RetryConfig{
		MaxRetries:       maxRetries,
		MaxTotalAttempts: maxRetries + 5,
		RetryOnRateLimit: true,
	}
	res := DoVoidWithRetry(func() (http.Header, error) {
		_, headers, err := BatchUpdateBlocks(documentID, string(payload), BatchUpdateBlocksOptions{
			UserAccessToken: userAccessToken,
		})
		return headers, err
	}, cfg)
	return res.Err
}

// fillCellSingleBlock 用单个文本块填充单元格（优先更新已有空块）。
// 限流由底层 UpdateBlock / CreateBlock 自动接管，本函数不再 acquire。
func fillCellSingleBlock(documentID, cellID string, elements []*larkdocx.TextElement, maxRetries int, userAccessToken string) error {
	retryCfg := RetryConfig{
		MaxRetries:       maxRetries,
		MaxTotalAttempts: maxRetries + 5,
		RetryOnRateLimit: true,
	}

	// 尝试更新已有子块（飞书创建表格时自动生成空文本块）
	children, _, childErr := GetBlockChildren(documentID, cellID, userAccessToken)
	if childErr == nil && len(children) > 0 {
		existingBlockID := StringVal(children[0].BlockId)
		if existingBlockID != "" {
			updateContent := buildCellUpdateContent(elements)
			result := DoVoidWithRetry(func() (http.Header, error) {
				return UpdateBlock(documentID, existingBlockID, updateContent, userAccessToken)
			}, retryCfg)
			if result.Err == nil {
				return nil
			}
		}
	}

	// 降级：创建新文本块
	blockType := blockTypeText
	textBlock := &larkdocx.Block{
		BlockType: &blockType,
		Text:      &larkdocx.Text{Elements: elements},
	}
	result := DoVoidWithRetry(func() (http.Header, error) {
		_, headers, err := CreateBlock(documentID, cellID, []*larkdocx.Block{textBlock}, 0, userAccessToken)
		return headers, err
	}, retryCfg)
	return result.Err
}

// fillCellMultiBlocks 用多个块填充单元格（支持 bullet/heading/text 混合）。
// 限流由底层 UpdateBlock / CreateBlock 自动接管，本函数不再 acquire。
func fillCellMultiBlocks(documentID, cellID string, groups []cellBlockGroup, maxRetries int, userAccessToken string) error {
	retryCfg := RetryConfig{
		MaxRetries:       maxRetries,
		MaxTotalAttempts: maxRetries + 5,
		RetryOnRateLimit: true,
	}

	// 获取飞书自动创建的空文本块，更新其内容以避免留下空块
	startIdx := 0
	children, _, childErr := GetBlockChildren(documentID, cellID, userAccessToken)
	if childErr == nil && len(children) > 0 && len(groups) > 0 && len(groups[0].elements) > 0 {
		existingBlockID := StringVal(children[0].BlockId)
		if existingBlockID != "" {
			updateContent := buildCellUpdateContent(groups[0].elements)
			result := DoVoidWithRetry(func() (http.Header, error) {
				return UpdateBlock(documentID, existingBlockID, updateContent, userAccessToken)
			}, retryCfg)
			if result.Err == nil {
				startIdx = 1 // 第一组已通过更新处理
			}
		}
	}

	// 创建剩余的块（使用正确的块类型）
	for j := startIdx; j < len(groups); j++ {
		group := groups[j]
		if len(group.elements) == 0 {
			continue
		}
		block := buildCellBlock(group)
		result := DoVoidWithRetry(func() (http.Header, error) {
			_, headers, err := CreateBlock(documentID, cellID, []*larkdocx.Block{block}, -1, userAccessToken)
			return headers, err
		}, retryCfg)
		if result.Err != nil {
			return result.Err
		}
	}
	return nil
}

// cellBlockGroup 表示表格单元格内的一个逻辑块
type cellBlockGroup struct {
	blockType int
	elements  []*larkdocx.TextElement
}

// splitCellElements 将 TextElement 按 \n 分隔符拆分为多个块组，
// 并根据内容前缀分类块类型（text/bullet/heading）
func splitCellElements(elements []*larkdocx.TextElement) []cellBlockGroup {
	// 快速检查是否包含 \n 分隔符
	hasNewline := false
	for _, elem := range elements {
		if elem != nil && elem.TextRun != nil && elem.TextRun.Content != nil && *elem.TextRun.Content == "\n" {
			hasNewline = true
			break
		}
	}
	if !hasNewline {
		return []cellBlockGroup{{blockType: blockTypeText, elements: elements}}
	}

	var groups []cellBlockGroup
	var current []*larkdocx.TextElement

	for _, elem := range elements {
		if elem != nil && elem.TextRun != nil && elem.TextRun.Content != nil && *elem.TextRun.Content == "\n" {
			if len(current) > 0 {
				groups = append(groups, classifyCellGroup(current))
				current = nil
			}
			continue
		}
		current = append(current, elem)
	}
	if len(current) > 0 {
		groups = append(groups, classifyCellGroup(current))
	}

	if len(groups) == 0 {
		return []cellBlockGroup{{blockType: blockTypeText, elements: elements}}
	}
	return groups
}

// classifyCellGroup 根据第一个元素的内容前缀判断块类型
func classifyCellGroup(elements []*larkdocx.TextElement) cellBlockGroup {
	if len(elements) == 0 {
		return cellBlockGroup{blockType: blockTypeText, elements: elements}
	}
	first := elements[0]
	if first == nil || first.TextRun == nil || first.TextRun.Content == nil {
		return cellBlockGroup{blockType: blockTypeText, elements: elements}
	}

	content := *first.TextRun.Content

	// 无序列表 "- "
	if trimmed, ok := strings.CutPrefix(content, "- "); ok {
		newFirst := cloneTextElement(first)
		*newFirst.TextRun.Content = trimmed
		return cellBlockGroup{blockType: blockTypeBullet, elements: append([]*larkdocx.TextElement{newFirst}, elements[1:]...)}
	}

	// 标题 "### " 等（heading1=3, heading2=4, ..., heading6=8）
	for level := 6; level >= 1; level-- {
		prefix := strings.Repeat("#", level) + " "
		if trimmed, ok := strings.CutPrefix(content, prefix); ok {
			newFirst := cloneTextElement(first)
			*newFirst.TextRun.Content = trimmed
			return cellBlockGroup{blockType: 2 + level, elements: append([]*larkdocx.TextElement{newFirst}, elements[1:]...)}
		}
	}

	return cellBlockGroup{blockType: blockTypeText, elements: elements}
}

// cloneTextElement 浅拷贝 TextElement，避免修改原始数据
func cloneTextElement(elem *larkdocx.TextElement) *larkdocx.TextElement {
	if elem == nil {
		return nil
	}
	clone := *elem
	if elem.TextRun != nil {
		tr := *elem.TextRun
		if elem.TextRun.Content != nil {
			content := *elem.TextRun.Content
			tr.Content = &content
		}
		clone.TextRun = &tr
	}
	return &clone
}

// buildCellBlock 根据块类型和元素构建飞书块
func buildCellBlock(group cellBlockGroup) *larkdocx.Block {
	bt := group.blockType
	text := &larkdocx.Text{Elements: group.elements}
	block := &larkdocx.Block{BlockType: &bt}

	switch bt {
	case 3:
		block.Heading1 = text
	case 4:
		block.Heading2 = text
	case 5:
		block.Heading3 = text
	case 6:
		block.Heading4 = text
	case 7:
		block.Heading5 = text
	case 8:
		block.Heading6 = text
	case 9:
		block.Heading7 = text
	case 10:
		block.Heading8 = text
	case 11:
		block.Heading9 = text
	case 12:
		block.Bullet = text
	case 13:
		block.Ordered = text
	default:
		block.Text = text
	}

	return block
}

// buildCellUpdateContent 构建单元格更新请求
func buildCellUpdateContent(elements []*larkdocx.TextElement) map[string]any {
	return map[string]any{
		"update_text_elements": map[string]any{
			"elements": buildElementsJSON(elements),
		},
	}
}

// buildElementsJSON 将 TextElement 转换为 UpdateBlock API 所需的 JSON 格式
func buildElementsJSON(elements []*larkdocx.TextElement) []map[string]any {
	var result []map[string]any
	for _, elem := range elements {
		if elem == nil || elem.TextRun == nil || elem.TextRun.Content == nil {
			continue
		}

		textRunMap := map[string]any{
			"content": *elem.TextRun.Content,
		}

		if style := elem.TextRun.TextElementStyle; style != nil {
			styleMap := map[string]any{}
			if style.Bold != nil && *style.Bold {
				styleMap["bold"] = true
			}
			if style.Italic != nil && *style.Italic {
				styleMap["italic"] = true
			}
			if style.Strikethrough != nil && *style.Strikethrough {
				styleMap["strikethrough"] = true
			}
			if style.Underline != nil && *style.Underline {
				styleMap["underline"] = true
			}
			if style.InlineCode != nil && *style.InlineCode {
				styleMap["inline_code"] = true
			}
			if style.Link != nil && style.Link.Url != nil {
				styleMap["link"] = map[string]any{
					"url": *style.Link.Url,
				}
			}
			if len(styleMap) > 0 {
				textRunMap["text_element_style"] = styleMap
			}
		}

		result = append(result, map[string]any{
			"text_run": textRunMap,
		})
	}
	return result
}

// GetTableCellIDs retrieves cell block IDs from a table block
func GetTableCellIDs(documentID string, tableBlockID string, userAccessToken ...string) ([]string, error) {
	block, err := GetBlock(documentID, tableBlockID, firstString(userAccessToken))
	if err != nil {
		return nil, err
	}

	if block.Table == nil || len(block.Table.Cells) == 0 {
		return nil, fmt.Errorf("块不是表格或没有单元格")
	}

	return block.Table.Cells, nil
}

// ============================================================
// 文档表格操作（Block 类型 31）
// ============================================================

// InsertTableRow 在表格中插入一行
// index = -1 表示插入到表格末尾
func InsertTableRow(documentID, tableBlockID string, index int, userAccessToken ...string) error {
	_, err := UpdateBlock(documentID, tableBlockID, map[string]any{
		"insert_table_row": map[string]any{
			"row_index": index,
		},
	}, userAccessToken...)
	return err
}

// InsertRowProgressFunc 为 AppendTableRows 可选进度回调：
// appended 为已成功追加的行数（1-based），total 为本次计划追加的总行数。
type InsertRowProgressFunc func(appended, total int)

// AppendTableRows 在表格末尾追加 count 行，用于突破 create_block 的 9 行上限。
// 将大表保持为单个 table block，避免视觉上被切成多个独立表格。
// 每行独立调用 insert_table_row（batch_update 不支持同 block 多操作）；progress 可为 nil。
func AppendTableRows(documentID, tableBlockID string, count int, progress InsertRowProgressFunc, userAccessToken string) error {
	return appendRowsLoop(count, progress, func() error {
		return InsertTableRow(documentID, tableBlockID, -1, userAccessToken)
	})
}

// appendRowsLoop 从网络调用中解耦的纯循环，便于单测。
func appendRowsLoop(count int, progress InsertRowProgressFunc, insertOne func() error) error {
	if count <= 0 {
		return nil
	}
	for i := 0; i < count; i++ {
		if err := insertOne(); err != nil {
			return fmt.Errorf("追加第 %d 行失败: %w", i+1, err)
		}
		if progress != nil {
			progress(i+1, count)
		}
	}
	return nil
}

// InsertTableColumn 在表格中插入一列
// index = -1 表示插入到表格末尾
func InsertTableColumn(documentID, tableBlockID string, index int, userAccessToken ...string) error {
	_, err := UpdateBlock(documentID, tableBlockID, map[string]any{
		"insert_table_column": map[string]any{
			"column_index": index,
		},
	}, userAccessToken...)
	return err
}

// DeleteTableRows 删除表格中的行（左闭右开区间）
// rowStartIndex: 起始行索引（包含，0 表示第一行）
// rowEndIndex: 结束行索引（不包含）
func DeleteTableRows(documentID, tableBlockID string, rowStartIndex, rowEndIndex int, userAccessToken ...string) error {
	_, err := UpdateBlock(documentID, tableBlockID, map[string]any{
		"delete_table_rows": map[string]any{
			"row_start_index": rowStartIndex,
			"row_end_index":   rowEndIndex,
		},
	}, userAccessToken...)
	return err
}

// DeleteTableColumns 删除表格中的列（左闭右开区间）
// columnStartIndex: 起始列索引（包含，0 表示第一列）
// columnEndIndex: 结束列索引（不包含）
func DeleteTableColumns(documentID, tableBlockID string, columnStartIndex, columnEndIndex int, userAccessToken ...string) error {
	_, err := UpdateBlock(documentID, tableBlockID, map[string]any{
		"delete_table_columns": map[string]any{
			"column_start_index": columnStartIndex,
			"column_end_index":   columnEndIndex,
		},
	}, userAccessToken...)
	return err
}

// MergeTableCells 合并表格单元格（左闭右开区间）
// rowStartIndex, rowEndIndex: 行范围（左闭右开）
// columnStartIndex, columnEndIndex: 列范围（左闭右开）
func MergeTableCells(documentID, tableBlockID string, rowStartIndex, rowEndIndex, columnStartIndex, columnEndIndex int, userAccessToken ...string) error {
	_, err := UpdateBlock(documentID, tableBlockID, map[string]any{
		"merge_table_cells": map[string]any{
			"row_start_index":    rowStartIndex,
			"row_end_index":      rowEndIndex,
			"column_start_index": columnStartIndex,
			"column_end_index":   columnEndIndex,
		},
	}, userAccessToken...)
	return err
}

// UnmergeTableCells 取消合并单元格
// rowIndex, columnIndex: 单元格位置
func UnmergeTableCells(documentID, tableBlockID string, rowIndex, columnIndex int, userAccessToken ...string) error {
	_, err := UpdateBlock(documentID, tableBlockID, map[string]any{
		"unmerge_table_cells": map[string]any{
			"row_index":    rowIndex,
			"column_index": columnIndex,
		},
	}, userAccessToken...)
	return err
}
