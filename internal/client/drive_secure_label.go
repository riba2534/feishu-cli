package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// SecureLabel 云文档密级标签。
type SecureLabel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListSecureLabels 查询当前用户可用的密级标签。
//
// 底层接口 GET /open-apis/drive/v2/my_secure_labels。pageSize 取值 1-10；
// lang 可选 zh/en/ja；返回标签列表、下一页 page_token、是否还有更多。
// 该接口仅支持用户身份，userAccessToken 不能为空。
func ListSecureLabels(pageSize int, pageToken, lang, userAccessToken string) ([]SecureLabel, string, bool, error) {
	c, err := GetClient()
	if err != nil {
		return nil, "", false, err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	apiPath := fmt.Sprintf("/open-apis/drive/v2/my_secure_labels?page_size=%d", pageSize)
	if pageToken != "" {
		apiPath += "&page_token=" + url.QueryEscape(pageToken)
	}
	if lang != "" {
		apiPath += "&lang=" + url.QueryEscape(lang)
	}

	resp, err := c.Get(Context(), apiPath, nil, tokenType, opts...)
	if err != nil {
		return nil, "", false, fmt.Errorf("查询密级标签失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", false, fmt.Errorf("查询密级标签失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Items     []SecureLabel `json:"items"`
			PageToken string        `json:"page_token"`
			HasMore   bool          `json:"has_more"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &parsed); err != nil {
		return nil, "", false, fmt.Errorf("密级标签响应解析失败: %w", err)
	}
	if parsed.Code != 0 {
		return nil, "", false, fmt.Errorf("查询密级标签失败: code=%d, msg=%s", parsed.Code, parsed.Msg)
	}
	return parsed.Data.Items, parsed.Data.PageToken, parsed.Data.HasMore, nil
}

// SetSecureLabel 设置云文档的密级标签。
//
// 底层接口 PATCH /open-apis/drive/v2/files/{file_token}/secure_label，query 参数 type，
// 请求体 {"id": labelID}。docType 可选 doc/docx/sheet/file/bitable/mindnote/slides。
// 该接口仅支持用户身份，userAccessToken 不能为空。
func SetSecureLabel(fileToken, docType, labelID, userAccessToken string) error {
	c, err := GetClient()
	if err != nil {
		return err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	apiPath := fmt.Sprintf("/open-apis/drive/v2/files/%s/secure_label?type=%s",
		url.PathEscape(fileToken), url.QueryEscape(docType))
	body := map[string]interface{}{"id": labelID}

	resp, err := c.Patch(Context(), apiPath, body, tokenType, opts...)
	if err != nil {
		return fmt.Errorf("设置密级标签失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("设置密级标签失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}
	var parsed struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(resp.RawBody, &parsed); err != nil {
		return fmt.Errorf("设置密级标签响应解析失败: %w", err)
	}
	if parsed.Code != 0 {
		// 1063013：密级降级需在文档界面完成审批流程，重试 API 无效。
		if parsed.Code == 1063013 {
			return fmt.Errorf("设置密级标签失败: code=%d, msg=%s（密级降级需要审批，请在文档界面完成密级降级审批后重试，重试 API 不会绕过审批）", parsed.Code, parsed.Msg)
		}
		return fmt.Errorf("设置密级标签失败: code=%d, msg=%s", parsed.Code, parsed.Msg)
	}
	return nil
}
