package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// 思维笔记节点接口（mindnote/v1，对齐官方 lark-cli mindnotes nodes list/create）：
//   - GET  /open-apis/mindnote/v1/mindnotes/{mindnote_id}/nodes  获取节点列表（accessTokens: user）
//   - POST /open-apis/mindnote/v1/mindnotes/{mindnote_id}/nodes  创建/更新节点（accessTokens: user, tenant）
//
// mindnote_id 是思维笔记文档 token（不是节点 ID，也不是 wiki node_token）。

// MindnoteNodesPath 返回思维笔记节点接口路径。
func MindnoteNodesPath(mindnoteID string) string {
	return fmt.Sprintf("/open-apis/mindnote/v1/mindnotes/%s/nodes", url.PathEscape(mindnoteID))
}

// ListMindnoteNodes 获取思维笔记节点列表，返回响应 data（含 nodes）。
// userIDType 为空时不传，由服务端按 open_id 处理。
func ListMindnoteNodes(mindnoteID, userIDType, userAccessToken string) (map[string]any, error) {
	return mindnoteNodesRequest("GET", mindnoteID, userIDType, nil, "获取思维笔记节点", userAccessToken)
}

// CreateMindnoteNodes 创建或更新思维笔记节点（nodes[].node_id 指向已有节点时为更新），返回响应 data（含 ids、client_token）。
func CreateMindnoteNodes(mindnoteID, userIDType string, body map[string]any, userAccessToken string) (map[string]any, error) {
	return mindnoteNodesRequest("POST", mindnoteID, userIDType, body, "创建或更新思维笔记节点", userAccessToken)
}

func mindnoteNodesRequest(method, mindnoteID, userIDType string, body any, action, userAccessToken string) (map[string]any, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	req := &larkcore.ApiReq{
		HttpMethod:                method,
		ApiPath:                   MindnoteNodesPath(mindnoteID),
		Body:                      body,
		QueryParams:               larkcore.QueryParams{},
		SupportedAccessTokenTypes: []larkcore.AccessTokenType{tokenType},
	}
	if v := strings.TrimSpace(userIDType); v != "" {
		req.QueryParams.Set("user_id_type", v)
	}
	resp, err := cli.Do(Context(), req, opts...)
	if err != nil {
		return nil, fmt.Errorf("%s失败: %w", action, err)
	}
	// 先解析业务信封再看 HTTP 状态：飞书大量业务错误随 HTTP 400 下发
	if err := CheckAPIResponse(action, resp); err != nil {
		return nil, err
	}
	var parsed struct {
		Data map[string]any `json:"data"`
	}
	dec := json.NewDecoder(bytes.NewReader(resp.RawBody))
	dec.UseNumber()
	if err := dec.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("解析%s响应失败: %w", action, err)
	}
	if parsed.Data == nil {
		parsed.Data = map[string]any{}
	}
	return parsed.Data, nil
}
