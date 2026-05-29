package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
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
// 返回 data 字段的 map
func BaseV3Call(method, path string, params map[string]any, body any, userAccessToken string) (map[string]any, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	queryParams := make(larkcore.QueryParams)
	for k, v := range params {
		switch val := v.(type) {
		case []string:
			for _, item := range val {
				queryParams.Add(k, item)
			}
		case []any:
			for _, item := range val {
				queryParams.Add(k, fmt.Sprintf("%v", item))
			}
		case nil:
			// 跳过
		default:
			queryParams.Set(k, fmt.Sprintf("%v", v))
		}
	}

	// SupportedAccessTokenTypes 让 SDK 知道本次请求支持哪些身份。
	// 列出 User 优先、Tenant 兜底 — SDK 会根据 options 里是否传了 WithUserAccessToken 选择
	req := &larkcore.ApiReq{
		HttpMethod:                strings.ToUpper(method),
		ApiPath:                   path,
		Body:                      body,
		QueryParams:               queryParams,
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

	resp, err := client.Do(Context(), req, opts...)
	if err != nil {
		return nil, fmt.Errorf("base/v3 API 调用失败: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		bodyPreview := strings.TrimSpace(string(resp.RawBody))
		if bodyPreview == "" {
			return nil, fmt.Errorf("base/v3 API HTTP %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("base/v3 API HTTP %d: %s", resp.StatusCode, bodyPreview)
	}

	var result map[string]any
	dec := json.NewDecoder(bytes.NewReader(resp.RawBody))
	dec.UseNumber()
	if err := dec.Decode(&result); err != nil {
		return nil, fmt.Errorf("base/v3 API 响应解析失败: %w", err)
	}

	// 检查 code
	code := toInt(result["code"])
	if code != 0 {
		return nil, fmt.Errorf("base/v3 API 失败: code=%d, msg=%s", code, apiErrorDetail(result))
	}

	// 返回 data 子对象（若存在）
	if data, ok := result["data"].(map[string]any); ok {
		// 部分 v3 端点（如 roles）的 data 内嵌了二次序列化的 JSON 字符串，
		// 表现为 {"data": "{\"key\":...}"}，这里自动解析还原。
		for k, v := range data {
			if s, ok := v.(string); ok && len(s) > 1 && s[0] == '{' {
				var nested map[string]any
				if err := json.Unmarshal([]byte(s), &nested); err == nil {
					data[k] = nested
				}
			}
		}
		return data, nil
	}
	return result, nil
}

// apiErrorDetail 从飞书响应里提取最有信息量的错误文案。
// base/v3 业务错误（HTTP 200 + code!=0）的顶层 msg 常为空，真正的原因藏在
// data.error.{hint,message}（如缺 name / 缺 role_type 的 field validation）。
// 优先级：顶层 msg → data.error.hint → data.error.message；都没有返回空串。
func apiErrorDetail(result map[string]any) string {
	if msg, _ := result["msg"].(string); msg != "" {
		return msg
	}
	data, ok := result["data"].(map[string]any)
	if !ok {
		return ""
	}
	errObj, ok := data["error"].(map[string]any)
	if !ok {
		return ""
	}
	if hint, _ := errObj["hint"].(string); hint != "" {
		return hint
	}
	if m, _ := errObj["message"].(string); m != "" {
		return m
	}
	return ""
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
