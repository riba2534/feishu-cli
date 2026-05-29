package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

const vcBase = "/open-apis/vc/v1"

// SearchMeetingsReq 会议搜索请求参数
// StartRFC3339/EndRFC3339 为空字符串表示不传；三个 ID 切片同理
type SearchMeetingsReq struct {
	Query          string
	StartRFC3339   string
	EndRFC3339     string
	OrganizerIDs   []string
	ParticipantIDs []string
	RoomIDs        []string
	PageSize       int
	PageToken      string
}

// SearchMeetings 搜索历史会议记录
// API: POST /open-apis/vc/v1/meetings/search
// 支持 query + 时间范围 + organizer_ids + participant_ids + open_room_ids 多维过滤
// 至少一个过滤条件由调用方保证
func SearchMeetings(req SearchMeetingsReq, userAccessToken string) (json.RawMessage, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	// 构造请求体
	filter := map[string]any{}
	if req.StartRFC3339 != "" || req.EndRFC3339 != "" {
		startTime := map[string]string{}
		if req.StartRFC3339 != "" {
			startTime["start_time"] = req.StartRFC3339
		}
		if req.EndRFC3339 != "" {
			startTime["end_time"] = req.EndRFC3339
		}
		filter["start_time"] = startTime
	}
	if len(req.OrganizerIDs) > 0 {
		filter["organizer_ids"] = req.OrganizerIDs
	}
	if len(req.ParticipantIDs) > 0 {
		filter["participant_ids"] = req.ParticipantIDs
	}
	if len(req.RoomIDs) > 0 {
		filter["open_room_ids"] = req.RoomIDs
	}

	body := map[string]any{}
	if req.Query != "" {
		body["query"] = req.Query
	}
	if len(filter) > 0 {
		body["meeting_filter"] = filter
	}

	// 构造查询参数（分页）
	apiPath := fmt.Sprintf("%s/meetings/search", vcBase)
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
		return nil, fmt.Errorf("搜索会议失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("搜索会议失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
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
		return nil, fmt.Errorf("搜索会议失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	return apiResp.Data, nil
}

// GetMeeting 获取会议详情
// API: GET /open-apis/vc/v1/meetings/{meeting_id}?with_participants=false&query_mode=0
// 返回 data 字段原始 JSON（含 meeting.note_id 等）
func GetMeeting(meetingID string, userAccessToken string) (json.RawMessage, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	tokenType, opts := resolveTokenOpts(userAccessToken)
	apiPath := fmt.Sprintf("%s/meetings/%s?with_participants=false&query_mode=0",
		vcBase, url.PathEscape(meetingID))

	resp, err := client.Get(Context(), apiPath, nil, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("获取会议详情失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取会议详情失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
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
		return nil, fmt.Errorf("获取会议详情失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	return apiResp.Data, nil
}

// GetMeetingRecording 获取会议录制信息（含 minute 链接，可提取 minute_token）
// API: GET /open-apis/vc/v1/meetings/{meeting_id}/recording
func GetMeetingRecording(meetingID string, userAccessToken string) (json.RawMessage, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	tokenType, opts := resolveTokenOpts(userAccessToken)
	apiPath := fmt.Sprintf("%s/meetings/%s/recording", vcBase, url.PathEscape(meetingID))

	resp, err := client.Get(Context(), apiPath, nil, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("获取会议录制失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取会议录制失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
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
		return nil, fmt.Errorf("获取会议录制失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	return apiResp.Data, nil
}

// VCBotJoinReq 会议机器人入会请求参数
// MeetingNo 会议号（必填）；Password 会议密码（可选）
type VCBotJoinReq struct {
	MeetingNo string
	Password  string
}

// BuildVCBotJoinBody 构造机器人入会请求体。导出供 cmd 层 dry-run 预览复用，
// 保证预览与真实请求体同源、不漂移。结构见 VCBotJoinMeeting 文档。
func BuildVCBotJoinBody(req VCBotJoinReq) map[string]any {
	body := map[string]any{
		"join_type": 1,
		"join_identify": map[string]any{
			"meeting_no": req.MeetingNo,
		},
	}
	if req.Password != "" {
		body["password"] = req.Password
	}
	return body
}

// VCBotJoinMeeting 让机器人加入会议
// API: POST /open-apis/vc/v1/bots/join
// 权限: tenant_access_token + vc:meeting.bot.join:write
// 返回 data 字段原始 JSON（含 meeting_id 等）
//
// 请求体结构与 lark-cli 官方实现（shortcuts/vc/vc_meeting_join.go + 单测）一致：
//
//	{ "join_type": 1, "join_identify": {"meeting_no": "<9位会议号>"}[, "password": "..."] }
//
// 易错点：join_type 固定为整数 1（按会议号入会）；join_identify 是嵌套对象而非枚举值；
// password 在顶层，不嵌进 join_identify。漏掉 join_type/join_identify 会被 server 拒为
// 99992402 field validation failed。
func VCBotJoinMeeting(req VCBotJoinReq, userAccessToken string) (json.RawMessage, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	body := BuildVCBotJoinBody(req)

	tokenType, opts := resolveTokenOpts(userAccessToken)
	apiPath := fmt.Sprintf("%s/bots/join", vcBase)
	resp, err := client.Post(Context(), apiPath, body, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("机器人入会失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("机器人入会失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
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
		return nil, fmt.Errorf("机器人入会失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	return apiResp.Data, nil
}

// VCBotLeaveMeeting 让机器人离开会议
// API: POST /open-apis/vc/v1/bots/leave
// 权限: tenant_access_token + vc:meeting.bot.leave:write
func VCBotLeaveMeeting(meetingID string, userAccessToken string) (json.RawMessage, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	body := map[string]any{
		"meeting_id": meetingID,
	}

	tokenType, opts := resolveTokenOpts(userAccessToken)
	apiPath := fmt.Sprintf("%s/bots/leave", vcBase)
	resp, err := client.Post(Context(), apiPath, body, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("机器人离会失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("机器人离会失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
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
		return nil, fmt.Errorf("机器人离会失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	return apiResp.Data, nil
}

// VCBotEventsReq 会议机器人事件查询参数
type VCBotEventsReq struct {
	MeetingID    string
	StartTimeSec string // Unix 秒（字符串），可空
	EndTimeSec   string // Unix 秒（字符串），可空
	PageSize     int
	PageToken    string
}

// VCBotMeetingEvents 查询机器人会议事件
// API: GET /open-apis/vc/v1/bots/events
// 权限: tenant_access_token
// 返回 data 字段原始 JSON（含 meeting_event_list、page_token、has_more）
func VCBotMeetingEvents(req VCBotEventsReq, userAccessToken string) (json.RawMessage, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	tokenType, opts := resolveTokenOpts(userAccessToken)
	params := url.Values{}
	if req.MeetingID != "" {
		params.Set("meeting_id", req.MeetingID)
	}
	if req.StartTimeSec != "" {
		params.Set("start_time", req.StartTimeSec)
	}
	if req.EndTimeSec != "" {
		params.Set("end_time", req.EndTimeSec)
	}
	if req.PageSize > 0 {
		params.Set("page_size", strconv.Itoa(req.PageSize))
	}
	if req.PageToken != "" {
		params.Set("page_token", req.PageToken)
	}

	apiPath := fmt.Sprintf("%s/bots/events", vcBase)
	if encoded := params.Encode(); encoded != "" {
		apiPath += "?" + encoded
	}

	resp, err := client.Get(Context(), apiPath, nil, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("查询机器人会议事件失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("查询机器人会议事件失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
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
		return nil, fmt.Errorf("查询机器人会议事件失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	return apiResp.Data, nil
}

// GetMeetingNote 获取会议纪要文档引用
// API: GET /open-apis/vc/v1/notes/{note_id}
// 返回 data.note 原始 JSON（含 artifacts[].artifact_type/doc_token 和 references[].doc_token）
func GetMeetingNote(noteID string, userAccessToken string) (json.RawMessage, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	tokenType, opts := resolveTokenOpts(userAccessToken)
	apiPath := fmt.Sprintf("%s/notes/%s", vcBase, url.PathEscape(noteID))

	resp, err := client.Get(Context(), apiPath, nil, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("获取会议纪要失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取会议纪要失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
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
		return nil, fmt.Errorf("获取会议纪要失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	return apiResp.Data, nil
}
