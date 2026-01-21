package client

import (
	"encoding/json"
	"fmt"

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
