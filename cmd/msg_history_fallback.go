package cmd

import (
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/internal/client"
)

// listMessagesViaSearch 通过搜索+逐条获取的方式获取消息列表
// 当 ListMessages API 返回空结果（bot 不在群）时作为降级方案
func listMessagesViaSearch(chatID string, pageSize int, pageToken string, userAccessToken string) (*client.ListMessagesResult, error) {
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

	searchResult, err := client.SearchMessages(searchOpts, userAccessToken)
	if err != nil {
		return nil, fmt.Errorf("搜索消息失败: %w", err)
	}

	if len(searchResult.MessageIDs) == 0 {
		return &client.ListMessagesResult{}, nil
	}

	// 逐条获取消息内容
	result := &client.ListMessagesResult{
		HasMore:   searchResult.HasMore,
		PageToken: searchResult.PageToken,
	}
	for _, msgID := range searchResult.MessageIDs {
		msgResult, err := client.GetMessage(msgID, userAccessToken, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "[警告] 获取消息 %s 失败: %v\n", msgID, err)
			continue
		}
		result.Items = append(result.Items, msgResult.Message)
	}

	return result, nil
}
