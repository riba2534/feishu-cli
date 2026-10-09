package client

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/riba2534/feishu-cli/v2/internal/safefile"
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
	// 请求前先拒绝敏感目录（~/.ssh、~/.feishu-cli、/etc 等），不发出任何网络请求
	if err := validatePath(outputPath); err != nil {
		return "", err
	}
	client, err := GetClient()
	if err != nil {
		return "", err
	}

	// 使用通用 HTTP 请求方式
	apiPath := fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/download_as_image", url.PathEscape(whiteboardID))

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

	// 先解析 JSON 业务错误信封（含随 HTTP 4xx 下发的业务码、HTTP 200 返回 JSON 错误体），再看 HTTP 状态
	if err := checkBoardResponse("获取画板图片", resp); err != nil {
		return "", err
	}

	ext, err := boardImageExt(resp.Header.Get("Content-Type"), resp.RawBody)
	if err != nil {
		return "", err
	}
	savePath, err := resolveBoardImagePath(outputPath, whiteboardID, ext)
	if err != nil {
		return "", err
	}

	// Ensure directory exists（目录与最终文件都经 safefile 校验：outputPath 是目录时文件名由服务端格式决定）
	dir := filepath.Dir(savePath)
	if err := safefile.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}

	// Write to file
	if err := safefile.AtomicWriteFile(savePath, resp.RawBody, 0o644); err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}

	return savePath, nil
}

// checkBoardResponse 先按飞书业务信封解析响应、再看 HTTP 状态（飞书大量业务错误随 HTTP 400 下发）。
// HTTP 非 2xx 且是业务信封时，在错误末尾补 "（HTTP <status>）"：重试分类（IsRetryableError）
// 依赖 HTTP 状态区分 4xx 永久错误与 5xx 临时错误，补上后与旧的 "HTTP 500, body: ..." 分类结果一致。
func checkBoardResponse(action string, resp *larkcore.ApiResp) error {
	err := CheckAPIResponse(action, resp)
	if err == nil {
		return nil
	}
	if _, ok := AsAPIError(err); ok && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		return fmt.Errorf("%w（HTTP %d）", err, resp.StatusCode)
	}
	return err
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
	apiPath := fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/export", url.PathEscape(whiteboardID))
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
	if err := checkBoardResponse("导出画板 SVG", resp); err != nil {
		return nil, err
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
	Syntax          string // plantuml / mermaid / svg
	DiagramType     string // auto, mindmap, sequence, activity, class, er, flowchart, state, component
	Style           string // board or classic
	ParseMode       int    // 解析模式，默认 1
	Overwrite       bool   // 是否覆盖画板内容：true=覆盖，false=不覆盖
	ClientToken     string // 幂等键（≥10 字符）；同一次逻辑写入的重试复用同一个值，避免超时重试产生重复图
	UserAccessToken string // optional user access token
}

// ImportDiagramResult contains the result of importing diagram
type ImportDiagramResult struct {
	TicketID string `json:"ticket_id"`
	// DegradedAttributes 服务端解析时降级/忽略的属性（data.extra.degradedAttributes），
	// SVG 路线常见：fill=url(#id) 渐变、自定义 stroke-dasharray 等。非空表示画面与源码有出入。
	DegradedAttributes []string `json:"degraded_attributes,omitempty"`
}

// 画板图表导入（POST /nodes/plantuml）的枚举白名单。
// syntax_type 对齐官方 whiteboard_update.go 的 formatCodeMap：plantuml=1、mermaid=2、svg=3。
// 过去 switch 的 default 分支把未知语法（含 svg）静默当成 PlantUML 发出去，服务端解析失败或画出错图，
// 现在未知取值一律报错。
var (
	boardSyntaxTypes = map[string]int{"plantuml": 1, "mermaid": 2, "svg": 3}
	boardStyleTypes  = map[string]int{"board": 1, "classic": 2}
	// diagram_type: auto=0, mindmap=1, sequence=2, activity=3, class=4, er=5, flowchart=6, state=7, component=8
	boardDiagramTypes = map[string]int{
		"auto": 0, "mindmap": 1, "sequence": 2, "activity": 3, "class": 4,
		"er": 5, "flowchart": 6, "state": 7, "component": 8,
	}
)

// BoardSyntaxNames / BoardStyleNames / BoardDiagramTypeNames 供命令帮助与报错列出合法取值。
const (
	BoardSyntaxNames      = "plantuml / mermaid / svg"
	BoardStyleNames       = "board / classic"
	BoardDiagramTypeNames = "auto / mindmap / sequence / activity / class / er / flowchart / state / component"
)

func lookupBoardEnum(table map[string]int, value, defaultValue, flag, names string) (int, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		v = defaultValue
	}
	code, ok := table[v]
	if !ok {
		return 0, fmt.Errorf("%s 取值 %q 不受支持，可选值: %s", flag, value, names)
	}
	return code, nil
}

// BoardSyntaxType 把 --syntax 映射为画板 API 的 syntax_type（空值按 plantuml）。
func BoardSyntaxType(syntax string) (int, error) {
	return lookupBoardEnum(boardSyntaxTypes, syntax, "plantuml", "--syntax", BoardSyntaxNames)
}

// BoardStyleType 把 --style 映射为 style_type（空值按 board）。
func BoardStyleType(style string) (int, error) {
	return lookupBoardEnum(boardStyleTypes, style, "board", "--style", BoardStyleNames)
}

// BoardDiagramType 把 --diagram-type 映射为 diagram_type（空值按 auto）。
func BoardDiagramType(diagramType string) (int, error) {
	return lookupBoardEnum(boardDiagramTypes, diagramType, "auto", "--diagram-type", BoardDiagramTypeNames)
}

// ValidateImportDiagramOptions 在发请求（以及 dry-run）之前校验枚举取值。
func ValidateImportDiagramOptions(opts ImportDiagramOptions) error {
	if _, err := BoardSyntaxType(opts.Syntax); err != nil {
		return err
	}
	if _, err := BoardStyleType(opts.Style); err != nil {
		return err
	}
	if _, err := BoardDiagramType(opts.DiagramType); err != nil {
		return err
	}
	return ValidateBoardClientToken(opts.ClientToken)
}

// ValidateBoardClientToken 校验画板写接口的 client_token（飞书要求 ≥10 字符；空值表示不带幂等键）。
func ValidateBoardClientToken(token string) error {
	if token != "" && len(token) < 10 {
		return fmt.Errorf("client_token 长度至少为 10 个字符: %s", token)
	}
	return nil
}

// BuildImportDiagramBody 构造 POST /nodes/plantuml 的请求体（dry-run 与实调共用，保证两者一致）。
// svg（syntax_type=3）对齐官方只发 plant_uml_code/syntax_type/parse_mode/overwrite：
// style_type / diagram_type 是 PlantUML/Mermaid 的排版参数，对 SVG 没有意义。
func BuildImportDiagramBody(content string, opts ImportDiagramOptions) (map[string]any, error) {
	syntaxType, err := BoardSyntaxType(opts.Syntax)
	if err != nil {
		return nil, err
	}
	styleType, err := BoardStyleType(opts.Style)
	if err != nil {
		return nil, err
	}
	diagramType, err := BoardDiagramType(opts.DiagramType)
	if err != nil {
		return nil, err
	}
	parseMode := opts.ParseMode
	if parseMode <= 0 {
		parseMode = 1
	}
	body := map[string]any{
		"plant_uml_code": content,
		"syntax_type":    syntaxType,
		"parse_mode":     parseMode,
	}
	if syntaxType != boardSyntaxTypes["svg"] {
		body["style_type"] = styleType
		body["diagram_type"] = diagramType
	}
	if opts.Overwrite {
		body["overwrite"] = true
	}
	return body, nil
}

// ImportDiagram imports a diagram to whiteboard
// API: POST /open-apis/board/v1/whiteboards/{id}/nodes/plantuml（syntax_type: 1=PlantUML 2=Mermaid 3=SVG）
// opts.ClientToken 非空时作为 query 参数 client_token 下发（官方 whiteboard_update.go 同款幂等键）。
func ImportDiagram(whiteboardID string, source string, opts ImportDiagramOptions) (*ImportDiagramResult, http.Header, error) {
	if err := ValidateBoardClientToken(opts.ClientToken); err != nil {
		return nil, nil, err
	}

	// Get content
	var content string
	if opts.SourceType == "file" || opts.SourceType == "" {
		// 本地文件内容会发往服务端：拒绝敏感目录，不存在/是目录归为用法错误
		data, err := safefile.ReadInputFile(source)
		if err != nil {
			return nil, nil, fmt.Errorf("读取图表文件失败: %w", err)
		}
		content = string(data)
	} else {
		content = source
	}

	reqBody, err := BuildImportDiagramBody(content, opts)
	if err != nil {
		return nil, nil, err
	}

	client, err := GetClient()
	if err != nil {
		return nil, nil, err
	}

	apiPath := fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes/plantuml", url.PathEscape(whiteboardID))
	if opts.ClientToken != "" {
		apiPath += "?" + url.Values{"client_token": []string{opts.ClientToken}}.Encode()
	}

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

	// 先解析业务信封再看 HTTP 状态：飞书大量业务错误随 HTTP 400 下发
	if err := checkBoardResponse("导入图表", resp); err != nil {
		return nil, headers, err
	}

	var apiResp struct {
		Data struct {
			TicketID string `json:"ticket_id"`
			NodeID   string `json:"node_id"`
			Extra    struct {
				DegradedAttributes []string `json:"degradedAttributes"`
			} `json:"extra"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, headers, fmt.Errorf("解析响应失败: %w", err)
	}

	nodeID := apiResp.Data.NodeID
	if nodeID == "" {
		nodeID = apiResp.Data.TicketID
	}

	return &ImportDiagramResult{
		TicketID:           nodeID,
		DegradedAttributes: apiResp.Data.Extra.DegradedAttributes,
	}, headers, nil
}

// CreateBoardNotesOptions contains options for creating board nodes
type CreateBoardNotesOptions struct {
	ClientToken     string
	UserIDType      string // open_id, union_id, user_id
	Overwrite       bool   // 是否覆盖画板内容：true=服务端原子覆盖清空再写入，false=直接追加写入
	UserAccessToken string // optional user access token
}

// CreateBoardNodes creates nodes on a whiteboard.
// nodesJSON should be a JSON array of node objects, e.g. [{"type":"composite_shape",...}]
func CreateBoardNodes(whiteboardID string, nodesJSON string, opts CreateBoardNotesOptions) ([]string, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	// 校验 client_token 最小长度（飞书开放平台要求至少 10 字符）
	if opts.ClientToken != "" && len(opts.ClientToken) < 10 {
		return nil, fmt.Errorf("client_token 长度至少为 10 个字符: %s", opts.ClientToken)
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
	if opts.Overwrite {
		reqBody["overwrite"] = true
	}

	q := url.Values{}
	q.Set("user_id_type", opts.UserIDType)
	if opts.ClientToken != "" {
		q.Set("client_token", opts.ClientToken)
	}

	apiPath := fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes?%s", url.PathEscape(whiteboardID), q.Encode())

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

	if err := checkBoardResponse("创建画板节点", resp); err != nil {
		return nil, err
	}

	// Parse response — API returns {"data": {"ids": ["id1", "id2", ...]}}
	var apiResp struct {
		Data struct {
			IDs []string `json:"ids"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return apiResp.Data.IDs, nil
}

// GetBoardNodes 获取画板的所有节点列表
func GetBoardNodes(whiteboardID string, userAccessToken ...string) (json.RawMessage, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	apiPath := fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes", url.PathEscape(whiteboardID))

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

	// 检查业务错误 code != 0（确保 fail closed）
	if err := checkBoardResponse("获取画板节点", resp); err != nil {
		return nil, err
	}
	var apiResp struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析画板节点响应失败: %w", err)
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

	apiPath := fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes/batch_delete", url.PathEscape(whiteboardID))
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

		// 解析响应检查业务错误（先业务码、后 HTTP 状态）
		if err := checkBoardResponse("删除画板节点", resp); err != nil {
			return err
		}

		// 多批次时间隔 1s 避免限流
		if end < len(nodeIDs) {
			time.Sleep(1 * time.Second)
		}
	}

	return nil
}
