package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// SearchMinutesReq 妙记搜索请求参数
// Query 为空表示不按关键词过滤；OwnerIDs / 时间范围为空表示不加对应过滤条件。
// StartRFC3339 / EndRFC3339 使用 RFC3339 时间字符串（create_time 过滤）。
type SearchMinutesReq struct {
	Query        string
	OwnerIDs     []string
	StartRFC3339 string
	EndRFC3339   string
	PageSize     int
	PageToken    string
}

// SearchMinutes 搜索妙记列表
// API: POST /open-apis/minutes/v1/minutes/search
// body: { query, filter:{ owner_ids[], create_time:{start_time,end_time} } }
// 分页通过 query 参数 page_size / page_token 控制。
// 至少一个过滤条件由调用方保证。
func SearchMinutes(req SearchMinutesReq, userAccessToken string) (json.RawMessage, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	filter := map[string]any{}
	if len(req.OwnerIDs) > 0 {
		filter["owner_ids"] = req.OwnerIDs
	}
	if req.StartRFC3339 != "" || req.EndRFC3339 != "" {
		createTime := map[string]string{}
		if req.StartRFC3339 != "" {
			createTime["start_time"] = req.StartRFC3339
		}
		if req.EndRFC3339 != "" {
			createTime["end_time"] = req.EndRFC3339
		}
		filter["create_time"] = createTime
	}

	body := map[string]any{}
	if req.Query != "" {
		body["query"] = req.Query
	}
	if len(filter) > 0 {
		body["filter"] = filter
	}

	apiPath := fmt.Sprintf("%s/minutes/search", minutesBase)
	params := url.Values{}
	if req.PageSize > 0 {
		params.Set("page_size", strconv.Itoa(req.PageSize))
	}
	if req.PageToken != "" {
		params.Set("page_token", req.PageToken)
	}
	if encoded := params.Encode(); encoded != "" {
		apiPath += "?" + encoded
	}

	tokenType, opts := resolveTokenOpts(userAccessToken)

	resp, err := client.Post(Context(), apiPath, body, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("搜索妙记失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("搜索妙记失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}

	var apiResp struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("搜索妙记失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	return apiResp.Data, nil
}

// ApplyMinutePermission 申请妙记的查看 / 编辑权限
// API: POST /open-apis/minutes/v1/minutes/{minute_token}/permissions/apply
// body: {"perm": "view"|"edit"}
// 权限：User Token，需 minutes:permission:apply。
// 幂等：已拥有目标权限时重复申请安全，不会重复发起。
func ApplyMinutePermission(minuteToken, perm, userAccessToken string) (json.RawMessage, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	body := map[string]any{"perm": perm}
	apiPath := fmt.Sprintf("%s/minutes/%s/permissions/apply", minutesBase, url.PathEscape(minuteToken))

	tokenType, opts := resolveTokenOpts(userAccessToken)

	resp, err := client.Post(Context(), apiPath, body, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("申请妙记权限失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("申请妙记权限失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}

	var apiResp struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("申请妙记权限失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	return apiResp.Data, nil
}
