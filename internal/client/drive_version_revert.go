package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// RevertFileVersion 将文件回滚到指定历史版本。
//
// 底层接口 POST /open-apis/drive/v1/files/{file_token}/revert，请求体 {"version": version}。
// SDK v3.5.3 未封装该接口，故走通用 HTTP。version 为 file version list 返回的长数字版本号，不是 tag。
func RevertFileVersion(fileToken, version, userAccessToken string) error {
	c, err := GetClient()
	if err != nil {
		return err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	apiPath := fmt.Sprintf("/open-apis/drive/v1/files/%s/revert", url.PathEscape(fileToken))
	body := map[string]interface{}{"version": version}

	resp, err := c.Post(Context(), apiPath, body, tokenType, opts...)
	if err != nil {
		return fmt.Errorf("回滚文件版本失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("回滚文件版本失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(resp.RawBody, &parsed); err != nil {
		return fmt.Errorf("回滚文件版本响应解析失败: %w", err)
	}
	if parsed.Code != 0 {
		return fmt.Errorf("回滚文件版本失败: code=%d, msg=%s", parsed.Code, parsed.Msg)
	}
	return nil
}
