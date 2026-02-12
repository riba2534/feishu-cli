package cmd

import (
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/converter"
)

// FeishuUserResolver 实现 converter.UserResolver 接口，通过飞书 API 批量解析用户信息
// 放在 cmd 包中以避免 client ↔ converter 循环依赖
type FeishuUserResolver struct{}

// BatchResolve 批量解析用户 ID 为用户信息，失败时静默降级返回空 map
func (r *FeishuUserResolver) BatchResolve(userIDs []string) map[string]converter.MentionUserInfo {
	result := make(map[string]converter.MentionUserInfo)
	if len(userIDs) == 0 {
		return result
	}

	users, err := client.BatchGetUserInfo(userIDs, "open_id")
	if err != nil {
		// 静默降级，不中断导出流程
		return result
	}

	for _, u := range users {
		if u == nil || u.OpenID == "" {
			continue
		}
		result[u.OpenID] = converter.MentionUserInfo{
			Name:  u.Name,
			Email: u.Email,
		}
	}
	return result
}
