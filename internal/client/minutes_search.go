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
	Query          string
	OwnerIDs       []string
	ParticipantIDs []string
	StartRFC3339   string
	EndRFC3339     string
	PageSize       int
	PageToken      string
}

// SearchMinutes 搜索妙记列表
// API: POST /open-apis/minutes/v1/minutes/search
// body: { query, filter:{ owner_ids[], participant_ids[], create_time:{start_time,end_time} } }
// 分页通过 query 参数 page_size / page_token 控制。
// 至少一个过滤条件由调用方保证。
func SearchMinutes(req SearchMinutesReq, userAccessToken string) (json.RawMessage, error) {
	filter := map[string]any{}
	if len(req.OwnerIDs) > 0 {
		filter["owner_ids"] = req.OwnerIDs
	}
	if len(req.ParticipantIDs) > 0 {
		filter["participant_ids"] = req.ParticipantIDs
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

	return vcCallAPI("搜索妙记", http.MethodPost, apiPath, body, userAccessToken)
}

// ApplyMinutePermission 申请妙记的查看 / 编辑权限
// API: POST /open-apis/minutes/v1/minutes/{minute_token}/permissions/apply
// body: {"perm": "view"|"edit"}
// 权限：User Token，需 minutes:permission:apply。
// 幂等：已拥有目标权限时重复申请安全，不会重复发起。
func ApplyMinutePermission(minuteToken, perm, userAccessToken string) (json.RawMessage, error) {
	body := map[string]any{"perm": perm}
	apiPath := fmt.Sprintf("%s/minutes/%s/permissions/apply", minutesBase, url.PathEscape(minuteToken))

	return vcCallAPI("申请妙记权限", http.MethodPost, apiPath, body, userAccessToken)
}
