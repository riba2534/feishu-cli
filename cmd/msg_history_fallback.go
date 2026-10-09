package cmd

import (
	"fmt"
	"io"

	"github.com/riba2534/feishu-cli/internal/client"
)

type listFallbackParams struct {
	token           string
	chatID          string
	pageSize        int
	pageToken       string
	cardContentType string
	startTime       string
	endTime         string
}

// applyListSearchFallback msg history / msg list 共用的搜索降级判断（仅 User 身份）：
//   - list 接口失败 → 改用搜索接口；
//   - 首页（未带 --page-token）为空但 has_more → 改用搜索接口；
//   - 带 --page-token 续翻时空页属正常（服务端过滤不可见消息），只提示继续翻页——
//     list 的 page_token 喂给搜索接口会报 1020 page_token invalid。
//
// 提示语与身份一致（降级只发生在 User 身份），并说明搜索模式下排序不生效、时间范围按搜索过滤。
func applyListSearchFallback(errOut io.Writer, result *client.ListMessagesResult, listErr error, p listFallbackParams) (*client.ListMessagesResult, error) {
	var reason string
	switch {
	case listErr != nil && p.token != "":
		reason = fmt.Sprintf("以当前用户身份读取消息列表失败（%v）", listErr)
	case listErr != nil:
		return nil, listErr
	case p.token != "" && p.pageToken == "" && len(result.Items) == 0 && result.HasMore:
		reason = "以当前用户身份读取的消息列表首页为空"
	default:
		if len(result.Items) == 0 && result.HasMore {
			fmt.Fprintf(errOut, "[提示] 本页无可见消息但 has_more=true（服务端过滤了不可见消息），请带 --page-token %s 继续翻页\n", result.PageToken)
		}
		return result, nil
	}

	fmt.Fprintf(errOut, "[提示] %s，改用消息搜索接口获取。搜索模式下 --sort-type 不生效（按搜索结果顺序），"+
		"--start-time/--end-time 按搜索的时间范围过滤；翻页请使用本次返回的 page_token\n", reason)
	fallbackResult, fallbackErr := listMessagesViaSearch(p.chatID, p.pageSize, p.pageToken, p.token, p.cardContentType, p.startTime, p.endTime)
	if fallbackErr != nil {
		if listErr != nil {
			return nil, listErr
		}
		return nil, fmt.Errorf("搜索降级失败: %w", fallbackErr)
	}
	return fallbackResult, nil
}

// listMessagesViaSearch 通过搜索 + mget 获取消息列表。
// 当 ListMessages API 返回空结果（bot 不在群）时作为降级方案。
//
// cardContentType 透传到 /im/v1/messages/mget，fallback 路径与主路径保持一致。
// startTime/endTime（秒）透传为搜索的 time_range，排序在搜索模式下不可控。
func listMessagesViaSearch(chatID string, pageSize int, pageToken, userAccessToken, cardContentType string, startEnd ...string) (*client.ListMessagesResult, error) {
	if pageSize <= 0 {
		pageSize = 20
	}

	// Search API query 参数不能为空，传空格作为通配
	searchOpts := client.SearchMessagesOptions{
		Query:     " ",
		ChatIDs:   []string{chatID},
		PageSize:  pageSize,
		PageToken: pageToken,
	}
	if len(startEnd) > 0 {
		searchOpts.StartTime = startEnd[0]
	}
	if len(startEnd) > 1 {
		searchOpts.EndTime = startEnd[1]
	}

	searchResult, err := client.SearchMessages(searchOpts, userAccessToken)
	if err != nil {
		return nil, err
	}

	if len(searchResult.MessageIDs) == 0 {
		return &client.ListMessagesResult{}, nil
	}

	batch, err := client.BatchGetMessagesBestEffort(searchResult.MessageIDs, userAccessToken, cardContentType)
	if err != nil {
		return nil, fmt.Errorf("批量获取消息失败: %w", err)
	}

	result := &client.ListMessagesResult{
		HasMore:   searchResult.HasMore,
		PageToken: searchResult.PageToken,
	}
	if batch != nil {
		for _, msg := range batch.Messages {
			if msg != nil {
				result.Items = append(result.Items, msg)
			}
		}
		result.MergeForwardSubMessages = batch.MergeForwardSubMessages
	}
	return result, nil
}
