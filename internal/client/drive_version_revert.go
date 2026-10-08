package client

import (
	"fmt"
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
	// 业务错误常随 HTTP 400 下发：先解析飞书信封里的 code，再看 HTTP 状态
	return CheckAPIResponse("回滚文件版本", resp)
}
