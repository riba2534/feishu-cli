package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// bitable/v1 旧版多维表格 API 服务路径前缀。
// 部分能力（dashboard copy / role-member / app update）base/v3 无对应路径，
// 仍需走 bitable/v1（与 base/v3 不同：无需 X-App-Id header，资源占位符用 app_token）。
const bitableV1ServicePath = "/open-apis/bitable/v1"

// BitableV1Path 构造 bitable/v1 API 路径。
// 示例: BitableV1Path("apps", appToken, "roles", roleID, "members") → /open-apis/bitable/v1/apps/{app_token}/roles/{role_id}/members
func BitableV1Path(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, "/")
		if part != "" {
			clean = append(clean, url.PathEscape(part))
		}
	}
	return bitableV1ServicePath + "/" + strings.Join(clean, "/")
}

// BuildQueryParams 把 map[string]any 转成 SDK 的 QueryParams。
// 值支持 string / []string / []any / 标量（fmt.Sprintf）；nil 跳过。
func BuildQueryParams(params map[string]any) larkcore.QueryParams {
	qp := make(larkcore.QueryParams)
	for k, v := range params {
		switch val := v.(type) {
		case []string:
			for _, item := range val {
				qp.Add(k, item)
			}
		case []any:
			for _, item := range val {
				qp.Add(k, fmt.Sprintf("%v", item))
			}
		case nil:
			// 跳过
		default:
			qp.Set(k, fmt.Sprintf("%v", v))
		}
	}
	return qp
}

// BitableV1Call 调用 bitable/v1 API（与 BaseV3Call 同构，但不带 X-App-Id header）。
// 返回 data 字段的 map；code != 0 时返回错误。
func BitableV1Call(method, path string, params map[string]any, body any, userAccessToken string) (map[string]any, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := &larkcore.ApiReq{
		HttpMethod:                strings.ToUpper(method),
		ApiPath:                   path,
		Body:                      body,
		QueryParams:               BuildQueryParams(params),
		SupportedAccessTokenTypes: []larkcore.AccessTokenType{larkcore.AccessTokenTypeUser, larkcore.AccessTokenTypeTenant},
	}

	var opts []larkcore.RequestOptionFunc
	if userAccessToken != "" {
		opts = append(opts, larkcore.WithUserAccessToken(userAccessToken))
	}

	resp, err := cli.Do(Context(), req, opts...)
	if err != nil {
		return nil, fmt.Errorf("bitable/v1 API 调用失败: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		bodyPreview := strings.TrimSpace(string(resp.RawBody))
		if bodyPreview == "" {
			return nil, fmt.Errorf("bitable/v1 API HTTP %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("bitable/v1 API HTTP %d: %s", resp.StatusCode, bodyPreview)
	}

	var result map[string]any
	dec := json.NewDecoder(bytes.NewReader(resp.RawBody))
	dec.UseNumber()
	if err := dec.Decode(&result); err != nil {
		return nil, fmt.Errorf("bitable/v1 API 响应解析失败: %w", err)
	}
	if code := toInt(result["code"]); code != 0 {
		return nil, fmt.Errorf("bitable/v1 API 失败: code=%d, msg=%s", code, apiErrorDetail(result))
	}
	if data, ok := result["data"].(map[string]any); ok {
		return data, nil
	}
	return result, nil
}
