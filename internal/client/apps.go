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

// SparkBasePath 是妙搭（Miaoda）应用 OpenAPI 的统一前缀。
// 妙搭后端在飞书开放平台注册为 spark 域，feishu / lark 双品牌路径一致，
// 仅 host 由 SDK 按 BaseURL 切换。
const SparkBasePath = "/open-apis/spark/v1"

// SparkCall 调用妙搭（Miaoda）JSON 端点，强制 User 身份。
//
// 妙搭应用归属个人，仅支持 user_access_token（scope: spark:app:read / spark:app:write）。
// 镜像 BaseV3Call 的错误处理：HTTP 4xx 透出原始 body，业务 code!=0 透出
// msg / data.error.hint。成功返回 data 子对象（不存在则返回整个响应）。
func SparkCall(method, path string, params map[string]any, body any, userAccessToken string) (map[string]any, error) {
	cli, err := GetClient()
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

	req := &larkcore.ApiReq{
		HttpMethod:                strings.ToUpper(method),
		ApiPath:                   path,
		Body:                      body,
		QueryParams:               queryParams,
		SupportedAccessTokenTypes: []larkcore.AccessTokenType{larkcore.AccessTokenTypeUser},
	}

	var opts []larkcore.RequestOptionFunc
	if userAccessToken != "" {
		opts = append(opts, larkcore.WithUserAccessToken(userAccessToken))
	}

	resp, err := cli.Do(Context(), req, opts...)
	if err != nil {
		return nil, fmt.Errorf("妙搭 API 调用失败: %w", err)
	}
	return parseSparkResponse(resp.StatusCode, resp.RawBody)
}

// parseSparkResponse 解析妙搭响应：先解析业务信封（业务错误常随 HTTP 4xx 下发，按 code 才能给出专门提示），
// 再看 HTTP 状态；成功返回 data 子对象。
func parseSparkResponse(statusCode int, raw []byte) (map[string]any, error) {
	var result map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	decodeErr := dec.Decode(&result)
	if decodeErr == nil {
		if _, hasCode := result["code"]; hasCode {
			if code := toInt(result["code"]); code != 0 {
				return nil, fmt.Errorf("妙搭 API 失败: code=%d, msg=%s", code, apiErrorDetail(result))
			}
		}
	}
	if statusCode >= http.StatusBadRequest {
		bodyPreview := strings.TrimSpace(string(raw))
		if bodyPreview == "" {
			return nil, fmt.Errorf("妙搭 API HTTP %d", statusCode)
		}
		return nil, fmt.Errorf("妙搭 API HTTP %d: %s", statusCode, bodyPreview)
	}
	if decodeErr != nil {
		return nil, fmt.Errorf("妙搭 API 响应解析失败: %w", decodeErr)
	}

	if data, ok := result["data"].(map[string]any); ok {
		return data, nil
	}
	return result, nil
}

// SparkAppGetPath 返回 GET /apps/{id}（查 app_type 等元数据）。
func SparkAppGetPath(appID string) string {
	return fmt.Sprintf("%s/apps/%s", SparkBasePath, url.PathEscape(appID))
}

// SparkPreReleasePath 返回 GET /apps/{id}/pre_release（取 TOS 预签名 upload_url / tos_path）。
func SparkPreReleasePath(appID string) string {
	return SparkAppGetPath(appID) + "/pre_release"
}

// SparkReleaseCreatePath 返回 POST /apps/{id}/releases（body.tos_path 触发发布）。
func SparkReleaseCreatePath(appID string) string {
	return SparkAppGetPath(appID) + "/releases"
}

// SparkReleaseGetPath 返回 GET /apps/{id}/releases/{release_id}（单次发布详情）。
func SparkReleaseGetPath(appID, releaseID string) string {
	return SparkReleaseCreatePath(appID) + "/" + url.PathEscape(releaseID)
}

// SparkReleaseListPath 返回 GET /apps/{id}/releases（发布历史，最近的在前）。
func SparkReleaseListPath(appID string) string {
	return SparkReleaseCreatePath(appID)
}
