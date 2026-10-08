package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// DocsAIResultError 表示 docs_ai 接口 HTTP/业务码都成功（code=0），但 data.result 不是 success：
//   - result=failed：本次指令未生效（如 str_replace 未命中 degrade_code=1013、命中多处 1014）；
//   - result=partial_success：指令已部分写入，warnings 列出被丢弃/降级的内容。
//
// 两种情况都必须让命令以非零退出码结束并透出 warnings 与 log_id——此前文本模式只在 failed 时
// 报一句 "result=failed"，partial_success 直接被当成功，warnings 与 log_id 全部丢失。
type DocsAIResultError struct {
	Action   string         // 中文动作，如 "更新文档内容"
	Result   string         // failed / partial_success / 其他非 success 值
	Warnings []string       // 服务端 warnings 原文
	LogID    string         // 响应头 X-Tt-Logid
	Data     map[string]any // 完整 data，便于 JSON 模式原样输出
}

func (e *DocsAIResultError) Error() string {
	var b strings.Builder
	if e.Action != "" {
		b.WriteString(e.Action)
	}
	switch e.Result {
	case "partial_success":
		b.WriteString("仅部分成功（result=partial_success），部分内容未按预期写入")
	case "failed":
		b.WriteString("失败（result=failed），文档未被修改")
	default:
		fmt.Fprintf(&b, "未成功（result=%s）", e.Result)
	}
	if len(e.Warnings) > 0 {
		b.WriteString("；服务端警告: ")
		b.WriteString(strings.Join(e.Warnings, " | "))
	}
	if e.LogID != "" {
		fmt.Fprintf(&b, "；log_id=%s", e.LogID)
	}
	return b.String()
}

var degradeCodeRe = regexp.MustCompile(`degrade_code=(\d+)`)

// DegradeCodes 解析 warnings 中的 degrade_code（如 1013 未命中、1014 多处命中、2105 画板克隆失败）。
func (e *DocsAIResultError) DegradeCodes() []int {
	return DocsAIDegradeCodes(e.Warnings)
}

// HasDegradeCode 判断 warnings 是否包含指定 degrade_code。
func (e *DocsAIResultError) HasDegradeCode(code int) bool {
	for _, c := range e.DegradeCodes() {
		if c == code {
			return true
		}
	}
	return false
}

// DocsAIDegradeCodes 从 warnings 文本中提取 degrade_code。
func DocsAIDegradeCodes(warnings []string) []int {
	var codes []int
	for _, w := range warnings {
		for _, m := range degradeCodeRe.FindAllStringSubmatch(w, -1) {
			if n, err := strconv.Atoi(m[1]); err == nil {
				codes = append(codes, n)
			}
		}
	}
	return codes
}

// AsDocsAIResultError 从错误链取出 *DocsAIResultError。
func AsDocsAIResultError(err error) (*DocsAIResultError, bool) {
	var e *DocsAIResultError
	if errors.As(err, &e) && e != nil {
		return e, true
	}
	return nil, false
}

// DocsAIWarnings 把 data.warnings 统一转为字符串切片（服务端可能给字符串或对象）。
func DocsAIWarnings(data map[string]any) []string {
	raw, ok := data["warnings"].([]any)
	if !ok {
		if ss, ok2 := data["warnings"].([]string); ok2 {
			return ss
		}
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, w := range raw {
		switch v := w.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				out = append(out, v)
			}
		case nil:
		default:
			b, _ := json.Marshal(v)
			out = append(out, string(b))
		}
	}
	return out
}

// classifyDocsAIResult 根据 data.result 生成 DocsAIResultError；success/空值返回 nil。
func classifyDocsAIResult(action string, data map[string]any, logID string) error {
	res, _ := data["result"].(string)
	res = strings.ToLower(strings.TrimSpace(res))
	if res == "" || res == "success" {
		return nil
	}
	return &DocsAIResultError{
		Action:   action,
		Result:   res,
		Warnings: DocsAIWarnings(data),
		LogID:    logID,
		Data:     data,
	}
}

// decodeDocsAIData 解析 docs_ai 响应信封，返回 data；业务码非 0 时返回 *APIError（先解析信封再看 HTTP 状态）。
func decodeDocsAIData(action string, status int, header http.Header, body []byte) (map[string]any, string, error) {
	if err := ParseAPIResponse(action, status, header, body); err != nil {
		return nil, "", err
	}
	var parsed struct {
		Code int            `json:"code"`
		Msg  string         `json:"msg"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, "", fmt.Errorf("解析%s响应失败: %w", action, err)
	}
	logID := headerValue(header, "X-Tt-Logid")
	if len(parsed.Data) == 0 {
		return nil, logID, fmt.Errorf("%s接口返回空数据对象", action)
	}
	return parsed.Data, logID, nil
}

// FetchDocsAI 调用 POST /open-apis/docs_ai/v1/documents/{document_id}/fetch。
// body 由调用方按官方协议组装（format / export_option / read_option / revision_id 等）。
// 返回的 data 中额外写入 log_id（若响应头提供）。
func FetchDocsAI(documentID string, body map[string]any, userAccessToken string) (map[string]any, error) {
	c, err := GetClient()
	if err != nil {
		return nil, err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	apiPath := fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s/fetch", url.PathEscape(documentID))
	resp, err := c.Post(Context(), apiPath, body, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("读取文档内容失败: %w", err)
	}
	data, logID, err := decodeDocsAIData("读取文档内容", resp.StatusCode, resp.Header, resp.RawBody)
	if err != nil {
		return nil, err
	}
	if logID != "" {
		data["log_id"] = logID
	}
	return data, nil
}

// DocsAIDocumentContent 取 data.document.content 与 data.document.revision_id。
func DocsAIDocumentContent(data map[string]any) (string, int) {
	doc, _ := data["document"].(map[string]any)
	if doc == nil {
		return "", -1
	}
	content, _ := doc["content"].(string)
	rev := -1
	switch v := doc["revision_id"].(type) {
	case float64:
		rev = int(v)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			rev = int(n)
		}
	case int:
		rev = v
	}
	return content, rev
}

// DocsAIRequest 通用 docs_ai 请求（历史版本等）：先解析业务信封再看 HTTP 状态，返回 data（附 log_id）。
func DocsAIRequest(method, apiPath string, params map[string]any, body any, action, userAccessToken string) (map[string]any, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	req := &larkcore.ApiReq{
		HttpMethod:                strings.ToUpper(method),
		ApiPath:                   apiPath,
		Body:                      body,
		QueryParams:               BuildQueryParams(params),
		SupportedAccessTokenTypes: []larkcore.AccessTokenType{tokenType},
	}
	resp, err := cli.Do(Context(), req, opts...)
	if err != nil {
		return nil, fmt.Errorf("%s失败: %w", action, err)
	}
	data, logID, err := decodeDocsAIData(action, resp.StatusCode, resp.Header, resp.RawBody)
	if err != nil {
		return nil, err
	}
	if logID != "" {
		data["log_id"] = logID
	}
	return data, nil
}
