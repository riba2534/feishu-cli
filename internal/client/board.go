package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// GetBoardImage downloads whiteboard image and saves to file
func GetBoardImage(whiteboardID string, outputPath string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	// 使用通用 HTTP 请求方式
	apiPath := fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/download_as_image", whiteboardID)

	resp, err := client.Get(Context(), apiPath, nil, larkcore.AccessTokenTypeTenant)
	if err != nil {
		return fmt.Errorf("获取画板图片失败: %w", err)
	}

	// Check response
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("获取画板图片失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}

	// Check if outputPath is a directory
	fileInfo, err := os.Stat(outputPath)
	if err == nil && fileInfo.IsDir() {
		// Use whiteboard ID as filename
		outputPath = filepath.Join(outputPath, whiteboardID+".png")
	}

	// Ensure directory exists
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// Write to file
	if err := os.WriteFile(outputPath, resp.RawBody, 0644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	return nil
}

// ImportDiagramOptions contains options for importing diagram to whiteboard
type ImportDiagramOptions struct {
	SourceType  string // file or content
	Syntax      string // plantuml or mermaid
	DiagramType string // auto, mindmap, sequence, activity, class, er, flowchart, usecase, component
	Style       string // board or classic
}

// ImportDiagramResult contains the result of importing diagram
type ImportDiagramResult struct {
	TicketID string `json:"ticket_id"`
}

// ImportDiagram imports a diagram to whiteboard
func ImportDiagram(whiteboardID string, source string, opts ImportDiagramOptions) (*ImportDiagramResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	// Default values
	if opts.Syntax == "" {
		opts.Syntax = "plantuml"
	}
	if opts.DiagramType == "" {
		opts.DiagramType = "auto"
	}
	if opts.Style == "" {
		opts.Style = "board"
	}

	// Get content
	var content string
	if opts.SourceType == "file" || opts.SourceType == "" {
		// Read from file
		data, err := os.ReadFile(source)
		if err != nil {
			return nil, fmt.Errorf("读取图表文件失败: %w", err)
		}
		content = string(data)
	} else {
		content = source
	}

	// Map syntax to API value
	var syntaxType int
	switch strings.ToLower(opts.Syntax) {
	case "plantuml":
		syntaxType = 1
	case "mermaid":
		syntaxType = 2
	default:
		syntaxType = 1
	}

	// Map style to API value
	var styleType int
	switch strings.ToLower(opts.Style) {
	case "board":
		styleType = 1
	case "classic":
		styleType = 2
	default:
		styleType = 1
	}

	// Build request body
	reqBody := map[string]interface{}{
		"diagram_content": content,
		"diagram_syntax":  syntaxType,
		"diagram_style":   styleType,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	apiPath := fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/diagrams/import", whiteboardID)

	resp, err := client.Post(Context(), apiPath, bodyBytes, larkcore.AccessTokenTypeTenant)
	if err != nil {
		return nil, fmt.Errorf("导入图表失败: %w", err)
	}

	// Parse response
	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			TicketID string `json:"ticket_id"`
		} `json:"data"`
	}

	// 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("导入图表失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}

	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("导入图表失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	return &ImportDiagramResult{
		TicketID: apiResp.Data.TicketID,
	}, nil
}

// CreateBoardNotesOptions contains options for creating board nodes
type CreateBoardNotesOptions struct {
	ClientToken string
	UserIDType  string // open_id, union_id, user_id
}

// CreateBoardNodes creates nodes on a whiteboard
func CreateBoardNodes(whiteboardID string, nodesJSON string, opts CreateBoardNotesOptions) ([]string, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	// Default user ID type
	if opts.UserIDType == "" {
		opts.UserIDType = "open_id"
	}

	// Build request body - nodes_str is a JSON string
	reqBody := map[string]interface{}{
		"nodes_str": nodesJSON,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	apiPath := fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes?user_id_type=%s", whiteboardID, opts.UserIDType)
	if opts.ClientToken != "" {
		apiPath += "&client_token=" + opts.ClientToken
	}

	resp, err := client.Post(Context(), apiPath, bodyBytes, larkcore.AccessTokenTypeTenant)
	if err != nil {
		return nil, fmt.Errorf("创建画板节点失败: %w", err)
	}

	// 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("创建画板节点失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}

	// Parse response
	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			AddedNodes []struct {
				NodeID string `json:"node_id"`
			} `json:"added_nodes"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("创建画板节点失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	var nodeIDs []string
	for _, node := range apiResp.Data.AddedNodes {
		nodeIDs = append(nodeIDs, node.NodeID)
	}

	return nodeIDs, nil
}

// DownloadBoardImageByURL downloads image from URL and saves to file
func DownloadBoardImageByURL(imageURL string, outputPath string) error {
	resp, err := http.Get(imageURL)
	if err != nil {
		return fmt.Errorf("下载图片失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载图片失败: HTTP %d", resp.StatusCode)
	}

	// Ensure directory exists
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// Write to file
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	return nil
}
