package client

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
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
		return nil, fmt.Errorf("failed to create document: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("failed to create document: code=%d, msg=%s", resp.Code, resp.Msg)
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
		return nil, fmt.Errorf("failed to get document: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("failed to get document: code=%d, msg=%s", resp.Code, resp.Msg)
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
		return "", fmt.Errorf("failed to get raw content: %w", err)
	}

	if !resp.Success() {
		return "", fmt.Errorf("failed to get raw content: code=%d, msg=%s", resp.Code, resp.Msg)
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
		return nil, "", fmt.Errorf("failed to list blocks: %w", err)
	}

	if !resp.Success() {
		return nil, "", fmt.Errorf("failed to list blocks: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	nextPageToken := ""
	if resp.Data.PageToken != nil {
		nextPageToken = *resp.Data.PageToken
	}

	return resp.Data.Items, nextPageToken, nil
}

// GetAllBlocks retrieves all blocks in a document with pagination
func GetAllBlocks(documentID string) ([]*larkdocx.Block, error) {
	var allBlocks []*larkdocx.Block
	pageToken := ""
	pageSize := 500

	for {
		blocks, nextToken, err := ListBlocks(documentID, pageToken, pageSize)
		if err != nil {
			return nil, err
		}

		allBlocks = append(allBlocks, blocks...)

		if nextToken == "" {
			break
		}
		pageToken = nextToken
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
		return nil, fmt.Errorf("failed to get block: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("failed to get block: code=%d, msg=%s", resp.Code, resp.Msg)
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
		return nil, fmt.Errorf("failed to create block: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("failed to create block: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data.Children, nil
}

// UpdateBlock updates an existing block
func UpdateBlock(documentID string, blockID string, updateContent interface{}) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	// The updateContent should be marshaled to the appropriate update request body
	contentBytes, err := json.Marshal(updateContent)
	if err != nil {
		return fmt.Errorf("failed to marshal update content: %w", err)
	}

	var updateBody larkdocx.UpdateBlockRequest
	if err := json.Unmarshal(contentBytes, &updateBody); err != nil {
		return fmt.Errorf("failed to unmarshal update content: %w", err)
	}

	req := larkdocx.NewPatchDocumentBlockReqBuilder().
		DocumentId(documentID).
		BlockId(blockID).
		DocumentRevisionId(-1).
		UpdateBlockRequest(&updateBody).
		Build()

	resp, err := client.Docx.DocumentBlock.Patch(Context(), req)
	if err != nil {
		return fmt.Errorf("failed to update block: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("failed to update block: code=%d, msg=%s", resp.Code, resp.Msg)
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
		return fmt.Errorf("failed to delete blocks: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("failed to delete blocks: code=%d, msg=%s", resp.Code, resp.Msg)
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
		if block.BlockId != nil {
			result.BlockIDs = append(result.BlockIDs, *block.BlockId)
		}
	}
	if resp.Data.DocumentRevisionId != nil {
		result.DocumentRevision = *resp.Data.DocumentRevisionId
	}

	return result, nil
}

// GetBlockChildren retrieves children of a block
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
		return nil, fmt.Errorf("failed to get block children: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("failed to get block children: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data.Items, nil
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

	// 构建画板块 (block_type = 43)
	blockType := 43 // Board
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

	result := &AddBoardResult{}
	if createdBlocks[0].BlockId != nil {
		result.BlockID = *createdBlocks[0].BlockId
	}
	if createdBlocks[0].Board != nil && createdBlocks[0].Board.Token != nil {
		result.WhiteboardID = *createdBlocks[0].Board.Token
	}

	return result, nil
}

// FillTableCells fills table cells with content by updating existing text blocks or creating new ones
// cellIDs: cell block IDs from the created table
// contents: cell content strings (in row-major order)
// Note: Feishu API automatically creates an empty text block in each cell when creating a table,
// so we need to update the existing block instead of creating a new one to avoid duplicate rows.
func FillTableCells(documentID string, cellIDs []string, contents []string) error {
	if len(cellIDs) == 0 || len(contents) == 0 {
		return nil
	}

	for i, cellID := range cellIDs {
		if i >= len(contents) {
			break
		}
		content := contents[i]
		if content == "" {
			continue
		}

		var err error
		maxRetries := 3

		// Get existing children of the cell (Feishu auto-creates an empty text block)
		children, childErr := GetBlockChildren(documentID, cellID)
		if childErr == nil && len(children) > 0 {
			// Update the first existing text block instead of creating a new one
			existingBlockID := ""
			if children[0].BlockId != nil {
				existingBlockID = *children[0].BlockId
			}

			if existingBlockID != "" {
				// Update the existing text block
				updateContent := map[string]interface{}{
					"update_text_elements": map[string]interface{}{
						"elements": []map[string]interface{}{
							{
								"text_run": map[string]interface{}{
									"content": content,
								},
							},
						},
					},
				}

				for attempt := 0; attempt < maxRetries; attempt++ {
					err = UpdateBlock(documentID, existingBlockID, updateContent)
					if err == nil {
						break
					}
					if strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "rate") {
						sleepTime := time.Duration(1<<attempt) * time.Second
						time.Sleep(sleepTime)
						continue
					}
					break
				}
				if err == nil {
					// Successfully updated, skip to next cell
					if i%10 == 9 {
						time.Sleep(100 * time.Millisecond)
					}
					continue
				}
				// If update failed, fall through to create new block
			}
		}

		// Create a new text block if no existing block to update
		blockType := 2 // Text block
		textBlock := &larkdocx.Block{
			BlockType: &blockType,
			Text: &larkdocx.Text{
				Elements: []*larkdocx.TextElement{
					{
						TextRun: &larkdocx.TextRun{
							Content: &content,
						},
					},
				},
			},
		}

		for attempt := 0; attempt < maxRetries; attempt++ {
			_, err = CreateBlock(documentID, cellID, []*larkdocx.Block{textBlock}, 0)
			if err == nil {
				break
			}
			if strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "rate") {
				sleepTime := time.Duration(1<<attempt) * time.Second
				time.Sleep(sleepTime)
				continue
			}
			break
		}
		if err != nil {
			return fmt.Errorf("填充单元格 %d 失败: %w", i, err)
		}

		// Small delay between cells to avoid rate limiting
		if i%10 == 9 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	return nil
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
