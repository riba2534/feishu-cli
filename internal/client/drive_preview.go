package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

// 上传文件的预览产物与封面（对齐官方 drive +preview / +cover）。

// DrivePreviewResult 是 POST /open-apis/drive/v1/medias/:file_token/preview_result 的 data。
type DrivePreviewResult struct {
	Version        string                   `json:"version,omitempty"`
	DataVersion    string                   `json:"data_version,omitempty"`
	FileSize       int64                    `json:"file_size,omitempty"`
	PreviewResults []map[string]interface{} `json:"preview_results"`
}

// DrivePreviewResultPath 返回 preview_result 接口路径（--dry-run 与真实请求共用）。
func DrivePreviewResultPath(fileToken string) string {
	return "/open-apis/drive/v1/medias/" + url.PathEscape(fileToken) + "/preview_result"
}

// DrivePreviewDownloadPath 返回 preview_download 接口路径（不含 query）。
func DrivePreviewDownloadPath(fileToken string) string {
	return "/open-apis/drive/v1/medias/" + url.PathEscape(fileToken) + "/preview_download"
}

// GetDrivePreviewResult 列出上传文件的预览候选；version 为空表示最新版本。
func GetDrivePreviewResult(fileToken, version, userAccessToken string) (*DrivePreviewResult, error) {
	const action = "获取预览候选"
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	body := map[string]any{}
	if version != "" {
		body["version"] = version
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Post(Context(), DrivePreviewResultPath(fileToken), body, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("%s失败: %w", action, err)
	}
	if err := CheckAPIResponse(action, resp); err != nil {
		return nil, err
	}
	var out struct {
		Data struct {
			Version        json.RawMessage          `json:"version"`
			DataVersion    json.RawMessage          `json:"data_version"`
			FileSize       int64                    `json:"file_size"`
			PreviewResults []map[string]interface{} `json:"preview_results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &out); err != nil {
		return nil, fmt.Errorf("%s失败: 解析响应失败: %w", action, err)
	}
	return &DrivePreviewResult{
		Version:        rawScalarString(out.Data.Version),
		DataVersion:    rawScalarString(out.Data.DataVersion),
		FileSize:       out.Data.FileSize,
		PreviewResults: out.Data.PreviewResults,
	}, nil
}

// OpenDrivePreviewDownload 打开 preview_download 下载流（params 为 preview_type / version / bus_type 等 query）。
func OpenDrivePreviewDownload(fileToken string, params url.Values, userAccessToken string, timeout time.Duration) (*DriveDownload, error) {
	apiPath := DrivePreviewDownloadPath(fileToken)
	if q := params.Encode(); q != "" {
		apiPath += "?" + q
	}
	return openBearerMediaDownload("下载预览产物", apiPath, userAccessToken, timeout)
}

// rawScalarString 把 JSON 字符串或数字统一转为字符串（version 字段两种形态都出现过）。
func rawScalarString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String()
	}
	return ""
}
