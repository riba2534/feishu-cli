package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// GetBoardImage downloads whiteboard image and saves to file
func GetBoardImage(whiteboardID string, outputPath string, userAccessToken ...string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	// 使用通用 HTTP 请求方式
	apiPath := fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/download_as_image", whiteboardID)

	tokenType := larkcore.AccessTokenTypeTenant
	var opts []larkcore.RequestOptionFunc
	if len(userAccessToken) > 0 && userAccessToken[0] != "" {
		tokenType = larkcore.AccessTokenTypeUser
		opts = UserTokenOption(userAccessToken[0])
	}

	resp, err := client.Get(Context(), apiPath, nil, tokenType, opts...)
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
	DiagramType string // auto, mindmap, sequence, activity, class, er, flowchart, state, component
	Style       string // board or classic
}

// ImportDiagramResult contains the result of importing diagram
type ImportDiagramResult struct {
	TicketID string `json:"ticket_id"`
}

// ImportDiagram imports a diagram to whiteboard
func ImportDiagram(whiteboardID string, source string, opts ImportDiagramOptions) (*ImportDiagramResult, http.Header, error) {
	client, err := GetClient()
	if err != nil {
		return nil, nil, err
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
			return nil, nil, fmt.Errorf("读取图表文件失败: %w", err)
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

	// Map diagram type to API value (integer)
	// Options: [0,1,2,3,4,5,6,7,8,101,102,201]
	// auto=0, mindmap=1, sequence=2, activity=3, class=4, er=5, flowchart=6, state=7, component=8
	var diagramType int
	switch strings.ToLower(opts.DiagramType) {
	case "mindmap":
		diagramType = 1
	case "sequence":
		diagramType = 2
	case "activity":
		diagramType = 3
	case "class":
		diagramType = 4
	case "er":
		diagramType = 5
	case "flowchart":
		diagramType = 6
	case "state":
		diagramType = 7
	case "component":
		diagramType = 8
	default:
		diagramType = 0 // auto
	}

	// Build request body - use field names that match lark-cli
	reqBody := map[string]any{
		"plant_uml_code": content,
		"syntax_type":    syntaxType,
		"style_type":     styleType,
		"diagram_type":   diagramType,
	}

	// 正确的 API 路径是 /nodes/plantuml
	apiPath := fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes/plantuml", whiteboardID)

	resp, err := client.Post(Context(), apiPath, reqBody, larkcore.AccessTokenTypeTenant)
	if err != nil {
		return nil, nil, fmt.Errorf("导入图表失败: %w", err)
	}

	headers := resp.Header

	// 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK {
		return nil, headers, fmt.Errorf("导入图表失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}

	// Parse response
	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			TicketID string `json:"ticket_id"`
			NodeID   string `json:"node_id"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, headers, fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, headers, fmt.Errorf("导入图表失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	nodeID := apiResp.Data.NodeID
	if nodeID == "" {
		nodeID = apiResp.Data.TicketID
	}

	return &ImportDiagramResult{
		TicketID: nodeID,
	}, headers, nil
}

// CreateBoardNotesOptions contains options for creating board nodes
type CreateBoardNotesOptions struct {
	ClientToken string
	UserIDType  string // open_id, union_id, user_id
}

// CreateBoardNodes creates nodes on a whiteboard.
// nodesJSON should be a JSON array of node objects, e.g. [{"type":"composite_shape",...}]
func CreateBoardNodes(whiteboardID string, nodesJSON string, opts CreateBoardNotesOptions) ([]string, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	// Default user ID type
	if opts.UserIDType == "" {
		opts.UserIDType = "open_id"
	}

	// Parse nodesJSON as a JSON array so it gets sent as {"nodes": [...]}
	var nodes []json.RawMessage
	if err := json.Unmarshal([]byte(nodesJSON), &nodes); err != nil {
		return nil, fmt.Errorf("解析节点 JSON 失败（需要 JSON 数组格式）: %w", err)
	}

	// Build request body with parsed nodes array
	reqBody := map[string]any{
		"nodes": nodes,
	}

	apiPath := fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes?user_id_type=%s", whiteboardID, opts.UserIDType)
	if opts.ClientToken != "" {
		apiPath += "&client_token=" + opts.ClientToken
	}

	resp, err := client.Post(Context(), apiPath, reqBody, larkcore.AccessTokenTypeTenant)
	if err != nil {
		return nil, fmt.Errorf("创建画板节点失败: %w", err)
	}

	// 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("创建画板节点失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}

	// Parse response — API returns {"data": {"ids": ["id1", "id2", ...]}}
	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			IDs []string `json:"ids"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("创建画板节点失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	return apiResp.Data.IDs, nil
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

// GetBoardNodes 获取画板的所有节点列表
func GetBoardNodes(whiteboardID string) (json.RawMessage, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	apiPath := fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes", whiteboardID)

	resp, err := client.Get(Context(), apiPath, nil, larkcore.AccessTokenTypeTenant)
	if err != nil {
		return nil, fmt.Errorf("获取画板节点失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取画板节点失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}

	return resp.RawBody, nil
}

// DeleteBoardNodes 批量删除画板节点
// 每批最多 100 个，间隔 1s 避免限流
func DeleteBoardNodes(whiteboardID string, nodeIDs []string) error {
	if len(nodeIDs) == 0 {
		return nil
	}

	client, err := GetClient()
	if err != nil {
		return err
	}

	apiPath := fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes/batch_delete", whiteboardID)

	// 分批删除，每批 100 个
	batchSize := 100
	for i := 0; i < len(nodeIDs); i += batchSize {
		end := i + batchSize
		if end > len(nodeIDs) {
			end = len(nodeIDs)
		}
		batch := nodeIDs[i:end]

		reqBody := map[string]any{
			"ids": batch,
		}

		resp, err := client.Delete(Context(), apiPath, reqBody, larkcore.AccessTokenTypeTenant)
		if err != nil {
			return fmt.Errorf("删除画板节点失败: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("删除画板节点失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
		}

		// 解析响应检查业务错误
		var apiResp struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
			return fmt.Errorf("解析响应失败: %w", err)
		}
		if apiResp.Code != 0 {
			return fmt.Errorf("删除画板节点失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
		}

		// 多批次时间隔 1s 避免限流
		if end < len(nodeIDs) {
			time.Sleep(1 * time.Second)
		}
	}

	return nil
}
