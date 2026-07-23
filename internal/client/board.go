package client

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// boardImageContentTypeExt 画板缩略图响应 Content-Type → 文件扩展名。
// download_as_image 端点实际返回 JPEG（服务端不保证 PNG），扩展名必须跟随实际格式。
var boardImageContentTypeExt = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
}

// GetBoardImage 下载画板缩略图并保存，返回实际保存路径。
// 扩展名按响应 Content-Type（缺失时按文件头嗅探）决定：
//   - outputPath 为目录：保存为 <whiteboard_id><实际扩展名>
//   - outputPath 无扩展名：自动补实际扩展名（推荐用法）
//   - outputPath 带 .png/.jpg/.jpeg：与实际格式不符时报错，避免写出扩展名与内容不符的文件
func GetBoardImage(whiteboardID string, outputPath string, userAccessToken ...string) (string, error) {
	client, err := GetClient()
	if err != nil {
		return "", err
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
		return "", fmt.Errorf("获取画板图片失败: %w", err)
	}

	// Check response
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("获取画板图片失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}

	ext, err := boardImageExt(resp.Header.Get("Content-Type"), resp.RawBody)
	if err != nil {
		return "", err
	}
	savePath, err := resolveBoardImagePath(outputPath, whiteboardID, ext)
	if err != nil {
		return "", err
	}

	// Ensure directory exists
	dir := filepath.Dir(savePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}

	// Write to file
	if err := os.WriteFile(savePath, resp.RawBody, 0644); err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}

	return savePath, nil
}

// boardImageExt 由响应 Content-Type 决定扩展名；header 缺失或不认识时按文件头嗅探兜底。
func boardImageExt(contentType string, body []byte) (string, error) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.TrimSpace(strings.Split(contentType, ";")[0])
	}
	if ext, ok := boardImageContentTypeExt[strings.ToLower(mediaType)]; ok {
		return ext, nil
	}
	if ext, ok := boardImageContentTypeExt[http.DetectContentType(body)]; ok {
		return ext, nil
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "<空>"
	}
	return "", fmt.Errorf("获取画板图片失败: 响应不是 PNG/JPEG 图片（Content-Type: %s）", contentType)
}

// resolveBoardImagePath 按输出路径形态与实际图片格式决定最终落盘路径。
func resolveBoardImagePath(outputPath, whiteboardID, ext string) (string, error) {
	if info, err := os.Stat(outputPath); err == nil && info.IsDir() {
		return filepath.Join(outputPath, whiteboardID+ext), nil
	}
	// 尾斜杠视为目录意图（目录可能尚不存在，后续 MkdirAll 会创建）
	if strings.HasSuffix(outputPath, "/") || strings.HasSuffix(outputPath, string(os.PathSeparator)) {
		return filepath.Join(outputPath, whiteboardID+ext), nil
	}
	current := strings.ToLower(filepath.Ext(outputPath))
	switch current {
	case "", ".":
		return strings.TrimSuffix(outputPath, ".") + ext, nil
	case ".png", ".jpg", ".jpeg":
		actual := current
		if actual == ".jpeg" {
			actual = ".jpg"
		}
		if actual != ext {
			return "", fmt.Errorf("服务端返回 %s 格式图片，但输出路径扩展名是 %s；请改用 %s 或省略扩展名（自动按实际格式命名）", ext, current, ext)
		}
		return outputPath, nil
	default:
		return "", fmt.Errorf("输出扩展名 %q 不受支持；请用 .png/.jpg/.jpeg、目录或不带扩展名的路径", current)
	}
}

// ExportWhiteboardSVGResult 是导出画板 SVG 的结果。
type ExportWhiteboardSVGResult struct {
	SVG      string // base64 解码后的 SVG 文本
	MimeType string // 服务端返回的 mime_type（通常 image/svg+xml）
}

// ExportWhiteboardSVG 调用 POST /open-apis/board/v1/whiteboards/{id}/export（export_type=svg），
// 返回服务端整板渲染的 SVG 视觉快照（base64 解码后）。对任意画板有效（不限于 svg 节点），
// 适用于「导出 SVG → 本地编辑 → board import / svg_to_board.py 回写」闭环。
func ExportWhiteboardSVG(whiteboardID string, userAccessToken ...string) (*ExportWhiteboardSVGResult, error) {
	c, err := GetClient()
	if err != nil {
		return nil, err
	}
	apiPath := fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/export", whiteboardID)
	body := map[string]any{"export_type": "svg"}

	tokenType := larkcore.AccessTokenTypeTenant
	var opts []larkcore.RequestOptionFunc
	if len(userAccessToken) > 0 && userAccessToken[0] != "" {
		tokenType = larkcore.AccessTokenTypeUser
		opts = UserTokenOption(userAccessToken[0])
	}

	resp, err := c.Post(Context(), apiPath, body, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("导出画板 SVG 失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("导出画板 SVG 失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}
	return parseExportWhiteboardSVGResponse(resp.RawBody)
}

// parseExportWhiteboardSVGResponse 解析 /export 响应信封并 base64 解码 SVG。
// 抽出便于单测（无需真实网络）。
func parseExportWhiteboardSVGResponse(rawBody []byte) (*ExportWhiteboardSVGResult, error) {
	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Content  string `json:"content"`
			MimeType string `json:"mime_type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析导出响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("导出画板 SVG 失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	if apiResp.Data.Content == "" {
		return nil, fmt.Errorf("导出响应 data.content 为空（画板可能为空或无导出权限）")
	}
	decoded, err := base64.StdEncoding.DecodeString(apiResp.Data.Content)
	if err != nil {
		return nil, fmt.Errorf("base64 解码 SVG 失败: %w", err)
	}
	return &ExportWhiteboardSVGResult{
		SVG:      string(decoded),
		MimeType: apiResp.Data.MimeType,
	}, nil
}

// ImportDiagramOptions contains options for importing diagram to whiteboard
type ImportDiagramOptions struct {
	SourceType      string // file or content
	Syntax          string // plantuml or mermaid
	DiagramType     string // auto, mindmap, sequence, activity, class, er, flowchart, state, component
	Style           string // board or classic
	UserAccessToken string // optional user access token
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

	// Build request body for Feishu board PlantUML/Mermaid import endpoint
	reqBody := map[string]any{
		"plant_uml_code": content,
		"syntax_type":    syntaxType,
		"style_type":     styleType,
		"diagram_type":   diagramType,
	}

	// 正确的 API 路径是 /nodes/plantuml
	apiPath := fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes/plantuml", whiteboardID)

	tokenType := larkcore.AccessTokenTypeTenant
	var reqOpts []larkcore.RequestOptionFunc
	if opts.UserAccessToken != "" {
		tokenType = larkcore.AccessTokenTypeUser
		reqOpts = UserTokenOption(opts.UserAccessToken)
	}

	resp, err := client.Post(Context(), apiPath, reqBody, tokenType, reqOpts...)
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
	ClientToken     string
	UserIDType      string // open_id, union_id, user_id
	UserAccessToken string // optional user access token
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

	tokenType := larkcore.AccessTokenTypeTenant
	var reqOpts []larkcore.RequestOptionFunc
	if opts.UserAccessToken != "" {
		tokenType = larkcore.AccessTokenTypeUser
		reqOpts = UserTokenOption(opts.UserAccessToken)
	}

	resp, err := client.Post(Context(), apiPath, reqBody, tokenType, reqOpts...)
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
func GetBoardNodes(whiteboardID string, userAccessToken ...string) (json.RawMessage, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	apiPath := fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes", whiteboardID)

	tokenType := larkcore.AccessTokenTypeTenant
	var reqOpts []larkcore.RequestOptionFunc
	if token := firstString(userAccessToken); token != "" {
		tokenType = larkcore.AccessTokenTypeUser
		reqOpts = UserTokenOption(token)
	}

	resp, err := client.Get(Context(), apiPath, nil, tokenType, reqOpts...)
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
func DeleteBoardNodes(whiteboardID string, nodeIDs []string, userAccessToken ...string) error {
	if len(nodeIDs) == 0 {
		return nil
	}

	client, err := GetClient()
	if err != nil {
		return err
	}

	apiPath := fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes/batch_delete", whiteboardID)
	tokenType := larkcore.AccessTokenTypeTenant
	var reqOpts []larkcore.RequestOptionFunc
	if token := firstString(userAccessToken); token != "" {
		tokenType = larkcore.AccessTokenTypeUser
		reqOpts = UserTokenOption(token)
	}

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

		resp, err := client.Delete(Context(), apiPath, reqBody, tokenType, reqOpts...)
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
