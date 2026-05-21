package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// 消息书签 item_type / flag_type 常量
// 来源：lark-cli/shortcuts/im 中的 ItemType/FlagType 定义，与飞书 OpenAPI 服务端一致。
//
// 合法组合（其余组合会被服务端拒绝）：
//   - (default, message) → 消息层书签（最常见）
//   - (thread, feed)     → topic-style 话题群的 feed 层书签
//   - (msg_thread, feed) → 普通群消息线程的 feed 层书签
//
// 枚举值与服务端常量对齐（lark-cli shortcuts/im/helpers.go 实测）：
//
//	ItemType: Default=0, Thread=4, MsgThread=11
//	FlagType: Feed=1, Message=2
//
// 不要按 0/1/2 顺序臆造——服务端会返回错误或操作到错误资源。
const (
	flagItemTypeDefault   = 0  // 普通消息
	flagItemTypeThread    = 4  // topic-style 话题
	flagItemTypeMsgThread = 11 // 普通群消息线程

	flagFlagTypeFeed    = 1 // feed 层（侧边栏书签）
	flagFlagTypeMessage = 2 // 消息层
)

// ParseFlagItemType 将用户输入的 item-type 字符串映射到 OpenAPI 整数枚举。
// 空字符串默认为 "default"。
func ParseFlagItemType(s string) (int, error) {
	switch s {
	case "", "default":
		return flagItemTypeDefault, nil
	case "thread":
		return flagItemTypeThread, nil
	case "msg_thread":
		return flagItemTypeMsgThread, nil
	}
	return 0, fmt.Errorf("无效的 item-type: %q (支持 default | thread | msg_thread)", s)
}

// ParseFlagFlagType 将用户输入的 flag-type 字符串映射到 OpenAPI 整数枚举。
// 空字符串默认为 "message"。
func ParseFlagFlagType(s string) (int, error) {
	switch s {
	case "", "message":
		return flagFlagTypeMessage, nil
	case "feed":
		return flagFlagTypeFeed, nil
	}
	return 0, fmt.Errorf("无效的 flag-type: %q (支持 message | feed)", s)
}

// FlagItem 单条消息书签项，用于创建 / 取消请求体。
type FlagItem struct {
	ItemID   string `json:"item_id"`
	ItemType int    `json:"item_type"`
	FlagType int    `json:"flag_type"`
}

// FlagListResult 书签列表返回结构
type FlagListResult struct {
	FlagItems       []map[string]any `json:"flag_items,omitempty"`
	DeleteFlagItems []map[string]any `json:"delete_flag_items,omitempty"`
	Messages        []map[string]any `json:"messages,omitempty"`
	HasMore         bool             `json:"has_more"`
	PageToken       string           `json:"page_token,omitempty"`
}

// flagsAPIPath 飞书消息书签 API 路径
// 飞书 OpenAPI 当前版本为 v1，与 lark-cli 实现保持一致。
const flagsAPIPath = "/open-apis/im/v1/flags"

// CreateFlag 为指定消息创建书签。
// itemType / flagType 必须是合法组合，否则服务端会返回错误。
// 权限：user token，scope `im:feed.flag:write`。
func CreateFlag(messageID string, itemType, flagType int, userAccessToken string) (map[string]any, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	body := map[string]any{
		"flag_items": []FlagItem{
			{ItemID: messageID, ItemType: itemType, FlagType: flagType},
		},
	}

	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := client.Post(Context(), flagsAPIPath, body, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("创建书签失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("创建书签失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}

	var apiResp struct {
		Code int            `json:"code"`
		Msg  string         `json:"msg"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("创建书签失败: 解析响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("创建书签失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	return apiResp.Data, nil
}

// ListFlags 列出当前用户的消息书签。
// pageSize 取值范围 1-50，默认 50；pageToken 用于翻页（服务端要求即使首页也传该参数）。
// 权限：user token，scope `im:feed.flag:read`（feed 类型书签若需取消息正文，还需 im:message.* 相关 scope）。
func ListFlags(pageSize int, pageToken string, userAccessToken string) (*FlagListResult, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 50 {
		pageSize = 50
	}

	apiPath := flagsAPIPath + "?" + url.Values{
		"page_size":  []string{strconv.Itoa(pageSize)},
		"page_token": []string{pageToken},
	}.Encode()

	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := client.Get(Context(), apiPath, nil, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("获取书签列表失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取书签列表失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}

	var apiResp struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data *FlagListResult `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("获取书签列表失败: 解析响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("获取书签列表失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	if apiResp.Data == nil {
		return &FlagListResult{}, nil
	}
	return apiResp.Data, nil
}

// CancelFlag 取消（删除）指定消息的书签。
// 服务端使用 POST /open-apis/im/v1/flags/cancel，请求体与 Create 同构。
// 权限：user token，scope `im:feed.flag:write`。
func CancelFlag(messageID string, itemType, flagType int, userAccessToken string) (map[string]any, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	body := map[string]any{
		"flag_items": []FlagItem{
			{ItemID: messageID, ItemType: itemType, FlagType: flagType},
		},
	}

	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := client.Post(Context(), flagsAPIPath+"/cancel", body, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("取消书签失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("取消书签失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}

	var apiResp struct {
		Code int            `json:"code"`
		Msg  string         `json:"msg"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("取消书签失败: 解析响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("取消书签失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	return apiResp.Data, nil
}
