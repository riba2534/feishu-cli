package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/riba2534/feishu-cli/internal/apidiag"
	"github.com/riba2534/feishu-cli/internal/config"
)

// base/v3 API 服务路径前缀
const baseV3ServicePath = "/open-apis/base/v3"

// BaseV3Path 构造 base/v3 API 路径
// 示例: BaseV3Path("bases", baseToken, "tables", tableID) → /open-apis/base/v3/bases/{base_token}/tables/{table_id}
func BaseV3Path(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, "/")
		if part != "" {
			clean = append(clean, url.PathEscape(part))
		}
	}
	return baseV3ServicePath + "/" + strings.Join(clean, "/")
}

// BaseV3Call 调用 base/v3 API
// method: GET/POST/PUT/PATCH/DELETE
// path:   BaseV3Path 构造的完整路径
// params: query string 参数（支持 string / []string / 任意值 fmt.Sprintf）
// body:   请求体（GET/DELETE 时传 nil）
// userAccessToken: 为空则使用 Tenant Token
// 返回 data 字段的 map；data 不是对象（如视图 group/sort/visible_fields 返回数组）时
// 返回完整响应信封（兼容旧行为），需要拿到非对象 data 的调用方请用 BaseV3CallAny。
func BaseV3Call(method, path string, params map[string]any, body any, userAccessToken string) (map[string]any, error) {
	result, err := baseV3Envelope(method, path, params, body, userAccessToken)
	if err != nil {
		return nil, err
	}
	if data, ok := result["data"].(map[string]any); ok {
		return data, nil
	}
	return result, nil
}

// BaseV3CallAny 调用 base/v3 API 并原样返回 data 字段（对象、数组、标量或 nil）。
// 用于 data 可能不是对象的端点：视图 group/sort/visible_fields 配置读写返回的 data 是数组，
// 用 BaseV3Call 会拿到 {"code":0,"data":[...],"msg":""} 整个信封。
func BaseV3CallAny(method, path string, params map[string]any, body any, userAccessToken string) (any, error) {
	result, err := baseV3Envelope(method, path, params, body, userAccessToken)
	if err != nil {
		return nil, err
	}
	return result["data"], nil
}

// baseV3Envelope 发起 base/v3 请求并返回解码后的完整响应信封（已校验业务 code）。
func baseV3Envelope(method, path string, params map[string]any, body any, userAccessToken string) (map[string]any, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	// SupportedAccessTokenTypes 让 SDK 知道本次请求支持哪些身份。
	// 列出 User 优先、Tenant 兜底 — SDK 会根据 options 里是否传了 WithUserAccessToken 选择
	req := &larkcore.ApiReq{
		HttpMethod:                strings.ToUpper(method),
		ApiPath:                   path,
		Body:                      body,
		QueryParams:               BuildQueryParams(params),
		SupportedAccessTokenTypes: []larkcore.AccessTokenType{larkcore.AccessTokenTypeUser, larkcore.AccessTokenTypeTenant},
	}

	// base/v3 需要带 X-App-Id header
	headers := make(http.Header)
	headers.Set("X-App-Id", config.Get().AppID)

	// 身份处理：User Token 或 Tenant Token
	opts := []larkcore.RequestOptionFunc{larkcore.WithHeaders(headers)}
	if userAccessToken != "" {
		opts = append(opts, larkcore.WithUserAccessToken(userAccessToken))
	}

	for attempt := 0; ; attempt++ {
		resp, err := client.Do(Context(), req, opts...)
		if err != nil {
			return nil, fmt.Errorf("base/v3 API 调用失败: %w", err)
		}
		result, err := decodeBaseEnvelope("base/v3", resp)
		// 800004135 "the method：OpenAPIxxx limited" 是按接口方法的限流拒绝（请求未执行），
		// 实测连续写视图配置时偶发；按退避有限重试，不会造成重复写入。
		if err != nil && attempt < len(baseV3RateLimitBackoff) && HasAPICode(err, baseV3MethodLimitedCode) {
			time.Sleep(baseV3RateLimitBackoff[attempt])
			continue
		}
		return result, err
	}
}

// baseV3MethodLimitedCode base/v3 按接口方法限流的业务码。
const baseV3MethodLimitedCode = 800004135

// baseV3RateLimitBackoff 遇到 800004135 时的重试间隔（测试可替换）。
var baseV3RateLimitBackoff = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}

// BaseAPIError 是 base/v3、bitable/v1 的业务错误（code != 0）。
//
// Error() 保持 "<api> API 失败: code=<N>, msg=<msg: hint（字段路径: path）>" 形态：
// HasAPICode 与依赖该文案的调用方照常可用，base/v3 藏在 data.error.{hint,path} 里的
// 真实原因也不会丢；Unwrap 暴露 *APIError，供 AsAPIError 取 code/log_id/缺失 scope 等诊断。
type BaseAPIError struct {
	API    string // "base/v3" 或 "bitable/v1"
	Code   int
	Detail string // apiErrorDetail 提取的 msg + hint/path
	apiErr *APIError
}

func (e *BaseAPIError) Error() string {
	return fmt.Sprintf("%s API 失败: code=%d, msg=%s", e.API, e.Code, e.Detail)
}

// Unwrap 让 errors.As(err, *APIError) 可以取到结构化诊断。
func (e *BaseAPIError) Unwrap() error {
	if e.apiErr == nil {
		return nil
	}
	return e.apiErr
}

// decodeBaseEnvelope 解析 base/v3、bitable/v1 响应信封。
//
// 先解析业务信封再看 HTTP 状态码：飞书不少业务错误随 HTTP 400/403 下发，
// 先判状态码会把 code/hint 埋进原始 body 预览里，按业务码分支的处理（HasAPICode/AsAPIError）走不到。
func decodeBaseEnvelope(api string, resp *larkcore.ApiResp) (map[string]any, error) {
	var result map[string]any
	dec := json.NewDecoder(bytes.NewReader(resp.RawBody))
	dec.UseNumber()
	decodeErr := dec.Decode(&result)
	if decodeErr == nil {
		if code := toInt(result["code"]); code != 0 {
			return nil, newBaseAPIError(api, resp, result, code)
		}
	}
	if resp.StatusCode >= http.StatusBadRequest {
		bodyPreview := strings.TrimSpace(string(resp.RawBody))
		if bodyPreview == "" {
			return nil, fmt.Errorf("%s API HTTP %d", api, resp.StatusCode)
		}
		return nil, fmt.Errorf("%s API HTTP %d: %s", api, resp.StatusCode, bodyPreview)
	}
	if decodeErr != nil {
		return nil, fmt.Errorf("%s API 响应解析失败: %w", api, decodeErr)
	}
	return result, nil
}

func newBaseAPIError(api string, resp *larkcore.ApiResp, result map[string]any, code int) error {
	e := &BaseAPIError{API: api, Code: code, Detail: apiErrorDetail(result)}
	if info, ok := apidiag.Parse(resp.StatusCode, resp.Header, resp.RawBody); ok {
		e.apiErr = &APIError{Action: api + " API ", Info: info}
	}
	return e
}

// UnwrapBaseRoleData 解开 base/v3 角色接口的多层响应。
//
// 角色接口实测返回 {"code":0,"data":{"data":"{\"base_roles\":[...]}"}}：外层 data 里还套一层
// data，且内层是二次序列化的 JSON 字符串；按官方说明内层还可能带自己的 code/message（业务失败时
// 外层 code 仍为 0）。这里统一：内层 code != 0 返回错误；否则返回解码后的内层 data。
// 只用于角色接口——其余接口的字符串字段（名称、描述等）即使以 { 开头也必须原样保留。
func UnwrapBaseRoleData(data map[string]any) (any, error) {
	if data == nil {
		return map[string]any{}, nil
	}
	if rawCode, exists := data["code"]; exists {
		if code := toInt(rawCode); code != 0 {
			msg, _ := data["message"].(string)
			if msg == "" {
				msg, _ = data["msg"].(string)
			}
			return nil, &BaseAPIError{API: "base/v3", Code: code, Detail: msg}
		}
	}
	inner, ok := data["data"]
	if !ok {
		return data, nil
	}
	if s, isStr := inner.(string); isStr {
		trimmed := strings.TrimSpace(s)
		if trimmed == "" {
			return map[string]any{}, nil
		}
		var decoded any
		dec := json.NewDecoder(strings.NewReader(trimmed))
		dec.UseNumber()
		if err := dec.Decode(&decoded); err != nil {
			// 不是 JSON：原样返回，不擅自改写
			return data, nil
		}
		inner = decoded
	}
	if m, isMap := inner.(map[string]any); isMap {
		if rawCode, exists := m["code"]; exists {
			if code := toInt(rawCode); code != 0 {
				msg, _ := m["message"].(string)
				if msg == "" {
					msg, _ = m["msg"].(string)
				}
				return nil, &BaseAPIError{API: "base/v3", Code: code, Detail: msg}
			}
		}
		// 角色列表实测再套一层：base_roles 的每一项是序列化后的 JSON 字符串，逐项解开
		if roles, ok := m["base_roles"].([]any); ok {
			for i, item := range roles {
				if str, ok := item.(string); ok {
					if obj := decodeJSONObjectString(str); obj != nil {
						roles[i] = obj
					}
				}
			}
		}
	}
	return inner, nil
}

// decodeJSONObjectString 把形如 "{...}" 的字符串解码成对象；不是 JSON 对象时返回 nil。
func decodeJSONObjectString(s string) map[string]any {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") {
		return nil
	}
	var obj map[string]any
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	if err := dec.Decode(&obj); err != nil {
		return nil
	}
	return obj
}

// apiErrorDetail 从飞书响应里提取最有信息量的错误文案。
// base/v3 业务错误（HTTP 200 + code!=0）的顶层 msg 要么为空、要么只是错误类型的重复
// （如 "not_found"），真正的原因藏在 data.error.{hint,path,message}
// （如 select 未知选项时 hint 会列出可用选项、path 指出出错字段）。
// msg 与 hint 同时存在时拼接输出，避免丢掉 hint。
func apiErrorDetail(result map[string]any) string {
	msg, _ := result["msg"].(string)
	var hint, message, path string
	if data, ok := result["data"].(map[string]any); ok {
		if errObj, ok := data["error"].(map[string]any); ok {
			hint, _ = errObj["hint"].(string)
			message, _ = errObj["message"].(string)
			path, _ = errObj["path"].(string)
		}
	}
	detail := hint
	if detail == "" {
		detail = message
	}
	if path != "" && detail != "" {
		detail = detail + "（字段路径: " + path + "）"
	}
	switch {
	case msg != "" && detail != "" && detail != msg:
		return msg + ": " + detail
	case msg != "":
		return msg
	default:
		return detail
	}
}

// toInt 安全地把任意数字（json.Number/float64/int）转成 int
func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	case string:
		var i int
		_, _ = fmt.Sscanf(n, "%d", &i)
		return i
	}
	return 0
}
