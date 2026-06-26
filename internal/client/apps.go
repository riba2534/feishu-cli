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

// parseSparkResponse 解析妙搭响应：HTTP 错误 / 业务 code!=0 → error；否则返回 data 子对象。
func parseSparkResponse(statusCode int, raw []byte) (map[string]any, error) {
	if statusCode >= http.StatusBadRequest {
		bodyPreview := strings.TrimSpace(string(raw))
		if bodyPreview == "" {
			return nil, fmt.Errorf("妙搭 API HTTP %d", statusCode)
		}
		return nil, fmt.Errorf("妙搭 API HTTP %d: %s", statusCode, bodyPreview)
	}

	var result map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&result); err != nil {
		return nil, fmt.Errorf("妙搭 API 响应解析失败: %w", err)
	}

	if code := toInt(result["code"]); code != 0 {
		return nil, fmt.Errorf("妙搭 API 失败: code=%d, msg=%s", code, apiErrorDetail(result))
	}

	if data, ok := result["data"].(map[string]any); ok {
		return data, nil
	}
	return result, nil
}

// 妙搭 html-publish 端点的业务错误码（后端 owns，文档更新时同步）。
const (
	sparkErrCodeBuildFailed = 90001 // tar.gz 上传成功但服务端构建失败
	sparkErrCodeAppNotFound = 90002 // app_id 不存在或无权访问
)

// SparkHTMLPublish 把打包好的 tar.gz 以单次 multipart POST 上传并发布，返回 data（含访问 url）。
//
// 复用 SDK 的 Formdata 原语 —— 与官方 lark-cli 完全一致的线格式：单个 file part，
// field name="file"，part body 即 tar.gz 字节；app_id 走 URL path，不放 body。
// 不手搓 multipart。
func SparkHTMLPublish(appID string, tarball []byte, userAccessToken string) (map[string]any, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}

	fd := larkcore.NewFormdata()
	fd.AddFile("file", bytes.NewReader(tarball))

	req := &larkcore.ApiReq{
		HttpMethod:                http.MethodPost,
		ApiPath:                   fmt.Sprintf("%s/apps/%s/upload_and_release_html_code", SparkBasePath, url.PathEscape(appID)),
		Body:                      fd,
		SupportedAccessTokenTypes: []larkcore.AccessTokenType{larkcore.AccessTokenTypeUser},
	}

	opts := []larkcore.RequestOptionFunc{larkcore.WithFileUpload()}
	if userAccessToken != "" {
		opts = append(opts, larkcore.WithUserAccessToken(userAccessToken))
	}

	resp, err := cli.Do(Context(), req, opts...)
	if err != nil {
		return nil, fmt.Errorf("妙搭 html-publish 上传失败: %w", err)
	}
	return parseHTMLPublishResponse(resp.StatusCode, resp.RawBody)
}

// parseHTMLPublishResponse 解析 html-publish 响应：HTTP 4xx/5xx 透出原始 body（与
// parseSparkResponse 一致，避免网关级失败被笼统的「解析失败」掩盖真实状态码）；
// 业务 code!=0 → 带 hint 的 error；成功只白名单提取 data.url（对齐官方 lark-cli，
// 刻意丢掉 status/release_id 等兄弟字段，后端新增字段不会无意泄漏到输出）。
func parseHTMLPublishResponse(statusCode int, raw []byte) (map[string]any, error) {
	if statusCode >= http.StatusBadRequest {
		bodyPreview := strings.TrimSpace(string(raw))
		if bodyPreview == "" {
			return nil, fmt.Errorf("妙搭 html-publish HTTP %d", statusCode)
		}
		return nil, fmt.Errorf("妙搭 html-publish HTTP %d: %s", statusCode, bodyPreview)
	}

	var env struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("解析 html-publish 响应失败: %w", err)
	}
	if env.Code != 0 {
		msg := fmt.Sprintf("妙搭 html-publish 失败: code=%d, msg=%s", env.Code, env.Msg)
		if hint := sparkHTMLPublishHint(env.Code); hint != "" {
			msg += "\n" + hint
		}
		return nil, fmt.Errorf("%s", msg)
	}
	out := map[string]any{}
	if env.Data.URL != "" {
		out["url"] = env.Data.URL
	}
	return out, nil
}

func sparkHTMLPublishHint(code int) string {
	switch code {
	case sparkErrCodeBuildFailed:
		return "构建失败：用 `feishu-cli apps html-publish --app-id <id> --path <path> --dry-run` 检查打包文件清单"
	case sparkErrCodeAppNotFound:
		return "应用不存在或无权访问；确认 app_id（从妙搭应用链接 https://miaoda.feishu.cn/app/app_xxx 的 /app/ 后提取，或直接给 app_xxx 字符串）"
	default:
		return ""
	}
}
