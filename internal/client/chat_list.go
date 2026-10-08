package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// ChatListItem 群列表项（当前身份加入的群）
type ChatListItem struct {
	ChatID        string `json:"chat_id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	OwnerID       string `json:"owner_id,omitempty"`
	OwnerIDType   string `json:"owner_id_type,omitempty"`
	External      bool   `json:"external"`
	TenantKey     string `json:"tenant_key,omitempty"`
	ChatStatus    string `json:"chat_status,omitempty"`
	Avatar        string `json:"avatar,omitempty"`
	ChatMode      string `json:"chat_mode,omitempty"`       // group / topic / p2p
	P2PTargetID   string `json:"p2p_target_id,omitempty"`   // 单聊对端 ID（仅 --types 含 p2p）
	P2PTargetType string `json:"p2p_target_type,omitempty"` // user / bot
}

// ListChatsResult 群列表结果（单页）
type ListChatsResult struct {
	Items     []*ChatListItem `json:"items"`
	PageToken string          `json:"page_token,omitempty"`
	HasMore   bool            `json:"has_more"`
}

// ListChatsOptions GET /open-apis/im/v1/chats 的查询参数。
type ListChatsOptions struct {
	UserIDType string
	SortType   string // ByCreateTimeAsc / ByActiveTimeDesc
	Types      string // "group" / "p2p" / "p2p,group"；空 = 服务端默认（仅群）
	PageSize   int
	PageToken  string
}

// ListChats 列出当前身份（User 或 Tenant）加入的群，单页返回（兼容旧签名）。
func ListChats(userIDType, sortType string, pageSize int, pageToken string, userAccessToken string) (*ListChatsResult, error) {
	return ListChatsWithOptions(ListChatsOptions{
		UserIDType: userIDType, SortType: sortType, PageSize: pageSize, PageToken: pageToken,
	}, userAccessToken)
}

// ListChatsWithOptions 列出当前身份加入的会话，单页返回。
// userAccessToken 为空时走 App/Tenant Token（列 Bot 加入的群），非空时列该用户加入的会话。
// Types 含 p2p 时可列出单聊（仅 User 身份；Bot 身份出于隐私不能列单聊）。
func ListChatsWithOptions(opts ListChatsOptions, userAccessToken string) (*ListChatsResult, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	if opts.UserIDType != "" {
		q.Set("user_id_type", opts.UserIDType)
	}
	if opts.SortType != "" {
		q.Set("sort_type", opts.SortType)
	}
	if opts.Types != "" {
		q.Set("types", opts.Types)
	}
	if opts.PageSize > 0 {
		q.Set("page_size", strconv.Itoa(opts.PageSize))
	}
	if opts.PageToken != "" {
		q.Set("page_token", opts.PageToken)
	}
	apiPath := "/open-apis/im/v1/chats"
	if enc := q.Encode(); enc != "" {
		apiPath += "?" + enc
	}
	tokenType, tokenOpts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Get(Context(), apiPath, nil, tokenType, tokenOpts...)
	if err != nil {
		return nil, fmt.Errorf("获取群列表失败: %w", err)
	}
	if err := CheckAPIResponse("获取群列表", resp); err != nil {
		return nil, err
	}
	var env struct {
		Data struct {
			Items     []*ChatListItem `json:"items"`
			PageToken string          `json:"page_token"`
			HasMore   bool            `json:"has_more"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &env); err != nil {
		return nil, fmt.Errorf("获取群列表失败: 解析响应失败: %w", err)
	}
	return &ListChatsResult{
		Items:     env.Data.Items,
		PageToken: env.Data.PageToken,
		HasMore:   env.Data.HasMore,
	}, nil
}

// maxMuteStatusBatch batch_get_mute_status 单次最多 100 个 chat_id。
const maxMuteStatusBatch = 100

// BatchGetMuteStatus 查询当前用户对一批会话的免打扰状态
// （POST /open-apis/im/v1/chat_user_setting/batch_get_mute_status，仅 User 身份）。
// 返回 muted（chat_id → 是否免打扰）与 unknown（非成员/无效 ID，无法判断的会话）。
func BatchGetMuteStatus(chatIDs []string, userAccessToken string) (map[string]bool, []string, error) {
	if userAccessToken == "" {
		return nil, nil, fmt.Errorf("查询免打扰状态需要 User Access Token")
	}
	cli, err := GetClient()
	if err != nil {
		return nil, nil, err
	}
	muted := map[string]bool{}
	var unknown []string
	seen := map[string]bool{}
	var ids []string
	for _, id := range chatIDs {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for start := 0; start < len(ids); start += maxMuteStatusBatch {
		end := start + maxMuteStatusBatch
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[start:end]
		tokenType, tokenOpts := resolveTokenOpts(userAccessToken)
		resp, err := cli.Post(Context(), "/open-apis/im/v1/chat_user_setting/batch_get_mute_status",
			map[string]any{"chat_ids": batch}, tokenType, tokenOpts...)
		if err != nil {
			return nil, nil, fmt.Errorf("查询免打扰状态失败: %w", err)
		}
		if err := CheckAPIResponse("查询免打扰状态", resp); err != nil {
			return nil, nil, err
		}
		var env struct {
			Data struct {
				Items []struct {
					ChatID  string `json:"chat_id"`
					IsMuted bool   `json:"is_muted"`
				} `json:"items"`
				InvalidIDList []struct {
					ID string `json:"id"`
				} `json:"invalid_id_list"`
			} `json:"data"`
		}
		if err := json.Unmarshal(resp.RawBody, &env); err != nil {
			return nil, nil, fmt.Errorf("查询免打扰状态失败: 解析响应失败: %w", err)
		}
		got := map[string]bool{}
		for _, it := range env.Data.Items {
			muted[it.ChatID] = it.IsMuted
			got[it.ChatID] = true
		}
		for _, id := range batch {
			if !got[id] {
				unknown = append(unknown, id)
			}
		}
	}
	return muted, unknown, nil
}
