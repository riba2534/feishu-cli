package client

import (
	"encoding/json"
	"fmt"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// openAPIEnvelope 是飞书 OpenAPI 的通用 JSON 响应信封。
type openAPIEnvelope struct {
	Code  int             `json:"code"`
	Msg   string          `json:"msg"`
	Data  json.RawMessage `json:"data"`
	Error struct {
		LogID string `json:"log_id"`
	} `json:"error"`
}

// logIDFrom 优先取响应体 error.log_id，缺失时回退到响应头。
func (e *openAPIEnvelope) logIDFrom(resp *larkcore.ApiResp) string {
	if e != nil && e.Error.LogID != "" {
		return e.Error.LogID
	}
	if resp != nil && resp.Header != nil {
		if v := resp.Header.Get("X-Tt-Logid"); v != "" {
			return v
		}
	}
	return ""
}

// callOpenAPIJSON 发起一次 JSON OpenAPI 调用并解析通用信封。
//
// 业务错误（code != 0）不在这里转换成 error，由调用方按接口语义分类；
// 只有传输失败、响应体无法解析时才返回 error。飞书部分业务码随 HTTP 4xx 下发，
// 因此先尝试解析 JSON 信封，解析失败时才按 HTTP 状态报错，避免专门的错误码提示走不到。
// userAccessToken 为空时使用 App（Tenant）身份。
func callOpenAPIJSON(method, apiPath string, pathParams, query map[string]string, body any, userAccessToken string) (*openAPIEnvelope, string, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, "", err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	req := &larkcore.ApiReq{
		HttpMethod:                method,
		ApiPath:                   apiPath,
		PathParams:                larkcore.PathParams{},
		QueryParams:               larkcore.QueryParams{},
		Body:                      body,
		SupportedAccessTokenTypes: []larkcore.AccessTokenType{tokenType},
	}
	for k, v := range pathParams {
		req.PathParams.Set(k, v)
	}
	for k, v := range query {
		req.QueryParams.Set(k, v)
	}

	resp, err := cli.Do(Context(), req, opts...)
	if err != nil {
		return nil, "", fmt.Errorf("请求 %s 失败: %w", apiPath, err)
	}
	var env openAPIEnvelope
	if jsonErr := json.Unmarshal(resp.RawBody, &env); jsonErr != nil {
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, "", fmt.Errorf("请求 %s 失败: HTTP %d, body: %s", apiPath, resp.StatusCode, truncateForError(string(resp.RawBody)))
		}
		return nil, "", fmt.Errorf("解析 %s 响应失败: %w", apiPath, jsonErr)
	}
	return &env, env.logIDFrom(resp), nil
}

// truncateForError 截断过长的响应体，避免错误信息淹没终端。
func truncateForError(s string) string {
	s = strings.TrimSpace(s)
	const max = 500
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(已截断)"
}
