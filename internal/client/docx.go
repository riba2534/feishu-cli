package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
)

// 块类型常量（避免魔术数字）
const (
	blockTypeText   = 2
	blockTypeBullet = 12
	blockTypeBoard  = 43
)

// CreateDocument creates a new document
func CreateDocument(title string, folderToken string) (*larkdocx.Document, error) {
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

	resp, err := client.Docx.Document.Create(Context(), req)
	if err != nil {
		return nil, fmt.Errorf("创建文档失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("创建文档失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data.Document, nil
}

// GetDocument retrieves document information
func GetDocument(documentID string) (*larkdocx.Document, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larkdocx.NewGetDocumentReqBuilder().
		DocumentId(documentID).
		Build()

	resp, err := client.Docx.Document.Get(Context(), req)
	if err != nil {
		return nil, fmt.Errorf("获取文档失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取文档失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data.Document, nil
}

// GetRawContent retrieves raw JSON content of a document
func GetRawContent(documentID string) (string, error) {
	client, err := GetClient()
	if err != nil {
		return "", err
	}

	req := larkdocx.NewRawContentDocumentReqBuilder().
		DocumentId(documentID).
		Build()

	resp, err := client.Docx.Document.RawContent(Context(), req)
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

	resp, err := client.Docx.DocumentBlock.List(Context(), reqBuilder.Build())
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
	var allBlocks []*larkdocx.Block
	pageToken := ""
	pageSize := 500
	pageCount := 0
	const maxPages = 1000 // 防止无限分页

	for {
		if pageCount >= maxPages {
			return nil, fmt.Errorf("超过最大分页限制 %d，文档可能有异常", maxPages)
		}
		blocks, nextToken, err := ListBlocks(documentID, pageToken, pageSize)
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
func GetBlock(documentID string, blockID string) (*larkdocx.Block, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larkdocx.NewGetDocumentBlockReqBuilder().
		DocumentId(documentID).
		BlockId(blockID).
		Build()

	resp, err := client.Docx.DocumentBlock.Get(Context(), req)
	if err != nil {
		return nil, fmt.Errorf("获取块失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取块失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data.Block, nil
}

// CreateBlock creates a new block under a parent block
func CreateBlock(documentID string, blockID string, children []*larkdocx.Block, index int) ([]*larkdocx.Block, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
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

	resp, err := client.Docx.DocumentBlockChildren.Create(Context(), req)
	if err != nil {
		return nil, fmt.Errorf("创建块失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("创建块失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data.Children, nil
}

// UpdateBlock updates an existing block
func UpdateBlock(documentID string, blockID string, updateContent any) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	// The updateContent should be marshaled to the appropriate update request body
	contentBytes, err := json.Marshal(updateContent)
	if err != nil {
		return fmt.Errorf("序列化更新内容失败: %w", err)
	}

	var updateBody larkdocx.UpdateBlockRequest
	if err := json.Unmarshal(contentBytes, &updateBody); err != nil {
		return fmt.Errorf("反序列化更新内容失败: %w", err)
	}

	req := larkdocx.NewPatchDocumentBlockReqBuilder().
		DocumentId(documentID).
		BlockId(blockID).
		DocumentRevisionId(-1).
		UpdateBlockRequest(&updateBody).
		Build()

	resp, err := client.Docx.DocumentBlock.Patch(Context(), req)
	if err != nil {
		return fmt.Errorf("更新块失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("更新块失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// DeleteBlocks deletes child blocks from a parent block by index range
// startIndex is the starting index (0-based), endIndex is exclusive
func DeleteBlocks(documentID string, blockID string, startIndex int, endIndex int) error {
	client, err := GetClient()
	if err != nil {
		return err
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

	resp, err := client.Docx.DocumentBlockChildren.BatchDelete(Context(), req)
	if err != nil {
		return fmt.Errorf("删除块失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("删除块失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// BatchUpdateBlocksOptions contains options for batch updating blocks
type BatchUpdateBlocksOptions struct {
	DocumentRevisionID int
	ClientToken        string
	UserIDType         string
}

// BatchUpdateBlocksResult contains the result of batch updating blocks
type BatchUpdateBlocksResult struct {
	BlockIDs         []string `json:"block_ids"`
	DocumentRevision int      `json:"document_revision_id"`
}

// BatchUpdateBlocks batch updates blocks in a document
func BatchUpdateBlocks(documentID string, requestsJSON string, opts BatchUpdateBlocksOptions) (*BatchUpdateBlocksResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("解析请求 JSON 失败: %w", err)
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

	resp, err := client.Docx.DocumentBlock.BatchUpdate(Context(), reqBuilder.Build())
	if err != nil {
		return nil, fmt.Errorf("批量更新块失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("批量更新块失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	result := &BatchUpdateBlocksResult{}
	for _, block := range resp.Data.Blocks {
		if id := StringVal(block.BlockId); id != "" {
			result.BlockIDs = append(result.BlockIDs, id)
		}
	}
	result.DocumentRevision = IntVal(resp.Data.DocumentRevisionId)

	return result, nil
}

// GetBlockChildren retrieves children of a block (first page only)
func GetBlockChildren(documentID string, blockID string) ([]*larkdocx.Block, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larkdocx.NewGetDocumentBlockChildrenReqBuilder().
		DocumentId(documentID).
		BlockId(blockID).
		Build()

	resp, err := client.Docx.DocumentBlockChildren.Get(Context(), req)
	if err != nil {
		return nil, fmt.Errorf("获取子块失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取子块失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data.Items, nil
}

// GetAllBlockChildren retrieves all direct children of a block with pagination
func GetAllBlockChildren(documentID string, blockID string) ([]*larkdocx.Block, error) {
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

		resp, err := client.Docx.DocumentBlockChildren.Get(Context(), reqBuilder.Build())
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

// AddBoard adds a board block to document and returns the whiteboard ID
func AddBoard(documentID string, parentID string, index int) (*AddBoardResult, error) {
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
	createdBlocks, err := CreateBlock(documentID, parentID, []*larkdocx.Block{boardBlock}, index)
	if err != nil {
		return nil, fmt.Errorf("创建画板块失败: %w", err)
	}

	if len(createdBlocks) == 0 {
		return nil, fmt.Errorf("创建画板块失败：未返回块信息")
	}

	result := &AddBoardResult{
		BlockID: StringVal(createdBlocks[0].BlockId),
	}
	if createdBlocks[0].Board != nil {
		result.WhiteboardID = StringVal(createdBlocks[0].Board.Token)
	}

	return result, nil
}

// FillTableCells fills table cells with plain text content.
// cellIDs: cell block IDs from the created table
// contents: cell content strings (in row-major order)
// Note: Feishu API automatically creates an empty text block in each cell when creating a table,
// so we need to update the existing block instead of creating a new one to avoid duplicate rows.
func FillTableCells(documentID string, cellIDs []string, contents []string) error {
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

	return fillTableCellsInternal(documentID, cellIDs[:cellCount], cellElements)
}

// FillTableCellsRich fills table cells with rich text elements (preserving links, styles, etc.)
// cellIDs: cell block IDs from the created table
// cellElements: each cell's text elements (in row-major order)
// fallbackContents: plain text fallback for cells without elements
func FillTableCellsRich(documentID string, cellIDs []string, cellElements [][]*larkdocx.TextElement, fallbackContents []string) error {
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

	return fillTableCellsInternal(documentID, cellIDs, merged)
}

// fillTableCellsInternal 是 FillTableCells 和 FillTableCellsRich 的统一实现
func fillTableCellsInternal(documentID string, cellIDs []string, cellElements [][]*larkdocx.TextElement) error {
	const maxRetries = 5

	for i, cellID := range cellIDs {
		var elements []*larkdocx.TextElement
		if i < len(cellElements) {
			elements = cellElements[i]
		}
		if len(elements) == 0 {
			continue
		}

		groups := splitCellElements(elements)

		var err error
		if len(groups) > 1 {
			// 多块：删除已有空块后创建多个正确类型的块（支持标题、列表等）
			err = fillCellMultiBlocks(documentID, cellID, groups, maxRetries)
		} else {
			// 单块：更新已有空块（飞书创建表格时自动生成）
			err = fillCellSingleBlock(documentID, cellID, elements, maxRetries)
		}
		if err != nil {
			return fmt.Errorf("填充单元格 %d 失败: %w", i, err)
		}

		throttlePer5(i)
	}

	return nil
}

// fillCellSingleBlock 用单个文本块填充单元格（优先更新已有空块）
func fillCellSingleBlock(documentID, cellID string, elements []*larkdocx.TextElement, maxRetries int) error {
	retryCfg := RetryConfig{
		MaxRetries:       maxRetries,
		MaxTotalAttempts: maxRetries + 5,
		RetryOnRateLimit: true,
	}

	// 尝试更新已有子块（飞书创建表格时自动生成空文本块）
	children, childErr := GetBlockChildren(documentID, cellID)
	if childErr == nil && len(children) > 0 {
		existingBlockID := StringVal(children[0].BlockId)
		if existingBlockID != "" {
			updateContent := buildCellUpdateContent(elements)
			result := DoVoidWithRetry(func() (http.Header, error) {
				return nil, UpdateBlock(documentID, existingBlockID, updateContent)
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
		_, err := CreateBlock(documentID, cellID, []*larkdocx.Block{textBlock}, 0)
		return nil, err
	}, retryCfg)
	return result.Err
}

// fillCellMultiBlocks 用多个块填充单元格（支持 bullet/heading/text 混合）
func fillCellMultiBlocks(documentID, cellID string, groups []cellBlockGroup, maxRetries int) error {
	retryCfg := RetryConfig{
		MaxRetries:       maxRetries,
		MaxTotalAttempts: maxRetries + 5,
		RetryOnRateLimit: true,
	}

	// 获取飞书自动创建的空文本块，更新其内容以避免留下空块
	startIdx := 0
	children, childErr := GetBlockChildren(documentID, cellID)
	if childErr == nil && len(children) > 0 && len(groups) > 0 && len(groups[0].elements) > 0 {
		existingBlockID := StringVal(children[0].BlockId)
		if existingBlockID != "" {
			updateContent := buildCellUpdateContent(groups[0].elements)
			result := DoVoidWithRetry(func() (http.Header, error) {
				return nil, UpdateBlock(documentID, existingBlockID, updateContent)
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
			_, err := CreateBlock(documentID, cellID, []*larkdocx.Block{block}, -1)
			return nil, err
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
	if strings.HasPrefix(content, "- ") {
		trimmed := strings.TrimPrefix(content, "- ")
		newFirst := cloneTextElement(first)
		*newFirst.TextRun.Content = trimmed
		return cellBlockGroup{blockType: blockTypeBullet, elements: append([]*larkdocx.TextElement{newFirst}, elements[1:]...)}
	}

	// 标题 "### " 等（heading1=3, heading2=4, ..., heading6=8）
	for level := 6; level >= 1; level-- {
		prefix := strings.Repeat("#", level) + " "
		if strings.HasPrefix(content, prefix) {
			trimmed := strings.TrimPrefix(content, prefix)
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

// throttlePer3 每 3 个单元格暂停 500ms，避免触发频率限制
func throttlePer5(index int) {
	if index%3 == 2 {
		time.Sleep(500 * time.Millisecond)
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
func GetTableCellIDs(documentID string, tableBlockID string) ([]string, error) {
	block, err := GetBlock(documentID, tableBlockID)
	if err != nil {
		return nil, err
	}

	if block.Table == nil || len(block.Table.Cells) == 0 {
		return nil, fmt.Errorf("块不是表格或没有单元格")
	}

	return block.Table.Cells, nil
}
