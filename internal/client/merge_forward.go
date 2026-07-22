package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// merge_forward 子消息展开相关常量。
//
// 飞书"合并转发"（msg_type=merge_forward）消息的 body.content 是固定占位符
// "Merged and Forwarded Message"，子消息不出现在普通 messages API 响应里。
// 通过给 GET /open-apis/im/v1/messages/{id} 加 query 参数
// card_msg_content_type=raw_card_content，API 会改在 data.items[] 里返回
// 容器自身 + 全部子消息（每条子消息带 upper_message_id 重建父子关系）。
// 该行为飞书未文档化，已实测验证。
const (
	// mergeForwardMaxDepth 限制 merge_forward 递归展开层数（防呆，正常 1-2 层）。
	mergeForwardMaxDepth = 10
	// mergeForwardMaxConcurrency 限制 list/history 入口并发展开 merge_forward 容器的数量。
	mergeForwardMaxConcurrency = 5
	// mergeForwardDisableEnv 应急逃生开关：设为 "1" 后禁用所有自动展开行为。
	mergeForwardDisableEnv = "FEISHU_DISABLE_MERGE_FORWARD_EXPAND"
)

// mergeForwardExpansionDisabled 返回环境变量是否设置了禁用展开。
func mergeForwardExpansionDisabled() bool {
	return os.Getenv(mergeForwardDisableEnv) == "1"
}

// getMessageItemsRaw 直拿 GET /open-apis/im/v1/messages/{id} 的 data.items 列表。
// 对 merge_forward 类型，API 会在 items 里返回容器自身 + 全部子消息。
func getMessageItemsRaw(messageID, cardContentType, userAccessToken string) ([]*larkim.Message, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := &larkcore.ApiReq{
		HttpMethod:  http.MethodGet,
		ApiPath:     "/open-apis/im/v1/messages/:message_id",
		PathParams:  larkcore.PathParams{},
		QueryParams: larkcore.QueryParams{},
		SupportedAccessTokenTypes: []larkcore.AccessTokenType{
			larkcore.AccessTokenTypeTenant,
			larkcore.AccessTokenTypeUser,
		},
	}
	req.PathParams.Set("message_id", messageID)
	if cardContentType != "" {
		req.QueryParams.Set("card_msg_content_type", cardContentType)
	}
	req.QueryParams.Set("with_sender_name", "true")

	apiResp, err := cli.Do(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("获取消息详情失败: %w", err)
	}
	if apiResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取消息详情失败: HTTP %d, body: %s", apiResp.StatusCode, string(apiResp.RawBody))
	}

	var resp listMessagesRawResponse
	if err := json.Unmarshal(apiResp.RawBody, &resp); err != nil {
		return nil, fmt.Errorf("获取消息详情失败: 解析响应失败: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("获取消息详情失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}
	harvestSenderNames(apiResp.RawBody)
	return resp.Data.Items, nil
}

// expandMergeForward 递归展开一条 merge_forward 容器，返回扁平的子消息列表（深度优先）。
// 每条子消息保留 upper_message_id，调用方据此可重建嵌套树。
//
// 容错策略：
//   - 触顶（depth >= mergeForwardMaxDepth）→ 不再递归，stderr warn
//   - cycle（visited 命中）→ 直接跳过
//   - 嵌套子级展开失败 → 跳过该子级递归，不阻断外层平铺
//
// visited 由调用方初始化，每个独立的"展开根"应使用独立 map（不同根之间不共享）。
func expandMergeForward(messageID, userAccessToken string, depth int, visited map[string]bool) ([]*larkim.Message, error) {
	if depth >= mergeForwardMaxDepth {
		fmt.Fprintf(os.Stderr, "warn: merge_forward 嵌套深度超过 %d，停止展开 %s\n", mergeForwardMaxDepth, messageID)
		return nil, nil
	}
	if visited[messageID] {
		return nil, nil
	}
	visited[messageID] = true

	items, err := getMessageItemsRaw(messageID, CardMsgContentTypeRaw, userAccessToken)
	if err != nil {
		return nil, err
	}
	if len(items) <= 1 {
		// 仅容器自身或空：无子消息可展开
		return nil, nil
	}

	// items[0] 是容器自身，跳过；items[1:] 是直接子消息
	var flat []*larkim.Message
	for _, sub := range items[1:] {
		if sub == nil {
			continue
		}
		flat = append(flat, sub)
		// 嵌套合并转发：递归展开
		if StringVal(sub.MsgType) == "merge_forward" {
			subID := StringVal(sub.MessageId)
			if subID == "" {
				continue
			}
			nested, err := expandMergeForward(subID, userAccessToken, depth+1, visited)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warn: 嵌套 merge_forward %s 展开失败: %v\n", subID, err)
				continue
			}
			flat = append(flat, nested...)
		}
	}
	return flat, nil
}

// applyMergeForwardExpansion 若 result 是 merge_forward 容器，自动展开子消息填充 SubMessages。
// 失败时 stderr warn 但不阻断（result.SubMessages 保持 nil）。
// 命中逃生开关或非 merge_forward 时直接返回。
func applyMergeForwardExpansion(result *GetMessageResult, messageID, userAccessToken string) {
	if result == nil || result.Message == nil {
		return
	}
	if StringVal(result.Message.MsgType) != "merge_forward" {
		return
	}
	if mergeForwardExpansionDisabled() {
		return
	}
	visited := make(map[string]bool)
	subs, err := expandMergeForward(messageID, userAccessToken, 0, visited)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warn: 展开 merge_forward %s 失败: %v\n", messageID, err)
		return
	}
	if len(subs) > 0 {
		result.SubMessages = subs
	}
}

// expandMergeForwardForContainers 并发展开 messages 中所有 merge_forward 容器，
// 返回 message_id → 平铺子消息（含递归）的映射。
// 用于 list/history/mget 入口。并发上限 mergeForwardMaxConcurrency，单容器失败不阻断其他。
// 逃生开关命中或无 merge_forward 容器时返回 nil。
func expandMergeForwardForContainers(messages []*larkim.Message, userAccessToken string) map[string][]*larkim.Message {
	if mergeForwardExpansionDisabled() {
		return nil
	}

	var containerIDs []string
	seen := make(map[string]bool)
	for _, msg := range messages {
		if msg == nil || StringVal(msg.MsgType) != "merge_forward" {
			continue
		}
		id := StringVal(msg.MessageId)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		containerIDs = append(containerIDs, id)
	}
	if len(containerIDs) == 0 {
		return nil
	}

	result := make(map[string][]*larkim.Message, len(containerIDs))
	var mu sync.Mutex
	sem := make(chan struct{}, mergeForwardMaxConcurrency)
	var wg sync.WaitGroup

	for _, id := range containerIDs {
		wg.Add(1)
		go func(mid string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			visited := make(map[string]bool)
			subs, err := expandMergeForward(mid, userAccessToken, 0, visited)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warn: 展开 merge_forward %s 失败: %v\n", mid, err)
				return
			}
			if len(subs) == 0 {
				return
			}
			mu.Lock()
			result[mid] = subs
			mu.Unlock()
		}(id)
	}
	wg.Wait()

	if len(result) == 0 {
		return nil
	}
	return result
}
