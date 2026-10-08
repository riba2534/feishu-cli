package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const vcBase = "/open-apis/vc/v1"

// vcCallAPI 统一 VC / 妙记 / 智能纪要 OpenAPI 调用：先按飞书业务信封解析（大量业务错误随
// HTTP 4xx 下发，如 2091005 无权限随 403 下发），再取 data 字段。
// action 为中文动作（不含"失败"），错误形如 "<action>失败: code=N, msg=..., log_id=..."，
// 调用方可用 HasAPICode / AsAPIError 按业务码分支。
func vcCallAPI(action, method, apiPath string, body any, userAccessToken string) (json.RawMessage, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)

	var resp *larkcore.ApiResp
	switch method {
	case http.MethodGet:
		resp, err = cli.Get(Context(), apiPath, body, tokenType, opts...)
	case http.MethodPost:
		resp, err = cli.Post(Context(), apiPath, body, tokenType, opts...)
	case http.MethodPut:
		resp, err = cli.Put(Context(), apiPath, body, tokenType, opts...)
	case http.MethodPatch:
		resp, err = cli.Patch(Context(), apiPath, body, tokenType, opts...)
	case http.MethodDelete:
		resp, err = cli.Delete(Context(), apiPath, body, tokenType, opts...)
	default:
		return nil, fmt.Errorf("不支持的 HTTP 方法: %s", method)
	}
	if err != nil {
		return nil, fmt.Errorf("%s失败: %w", action, err)
	}
	if err := CheckAPIResponse(action, resp); err != nil {
		return nil, err
	}

	var apiResp struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("%s失败: 解析响应失败: %w", action, err)
	}
	return apiResp.Data, nil
}

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

	return vcCallAPI("搜索会议", http.MethodPost, apiPath, body, userAccessToken)
}

// GetMeeting 获取会议详情
// API: GET /open-apis/vc/v1/meetings/{meeting_id}?with_participants=false&query_mode=0
// 返回 data 字段原始 JSON（含 meeting.note_id 等）
func GetMeeting(meetingID string, userAccessToken string) (json.RawMessage, error) {
	apiPath := fmt.Sprintf("%s/meetings/%s?with_participants=false&query_mode=0",
		vcBase, url.PathEscape(meetingID))

	return vcCallAPI("获取会议详情", http.MethodGet, apiPath, nil, userAccessToken)
}

// GetMeetingRecording 获取会议录制信息（含 minute 链接，可提取 minute_token）
// API: GET /open-apis/vc/v1/meetings/{meeting_id}/recording
func GetMeetingRecording(meetingID string, userAccessToken string) (json.RawMessage, error) {
	apiPath := fmt.Sprintf("%s/meetings/%s/recording", vcBase, url.PathEscape(meetingID))

	return vcCallAPI("获取会议录制", http.MethodGet, apiPath, nil, userAccessToken)
}

// VCBotJoinReq 会议机器人入会请求参数
// MeetingNo 会议号（必填，9 位数字）；Password 会议密码（可选）；
// CallID 邀请事件透传的关联 ID（可选）；Start=true 表示发起日程会议（action=2，对齐官方 --action start）。
type VCBotJoinReq struct {
	MeetingNo string
	Password  string
	CallID    string
	Start     bool
}

// vcBotJoinActionStart 官方 bots/join 的 action 取值：2 = 发起日程会议（默认不传 = 加入）
const vcBotJoinActionStart = 2

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
	if req.CallID != "" {
		body["call_id"] = req.CallID
	}
	if req.Start {
		body["action"] = vcBotJoinActionStart
	}
	return body
}

// VCBotJoinMeeting 让机器人加入会议
// API: POST /open-apis/vc/v1/bots/join
// 权限: tenant_access_token + vc:meeting.bot.join:write
// 返回 data 字段原始 JSON（含 meeting_id 等）
//
// 请求体结构（已实测验证）：
//
//	{ "join_type": 1, "join_identify": {"meeting_no": "<9位会议号>"}[, "password": "..."][, "call_id": "..."][, "action": 2] }
//
// 易错点：join_type 固定为整数 1（按会议号入会）；join_identify 是嵌套对象而非枚举值；
// password 在顶层，不嵌进 join_identify。漏掉 join_type/join_identify 会被 server 拒为
// 99992402 field validation failed。
func VCBotJoinMeeting(req VCBotJoinReq, userAccessToken string) (json.RawMessage, error) {
	body := BuildVCBotJoinBody(req)

	apiPath := fmt.Sprintf("%s/bots/join", vcBase)
	return vcCallAPI("机器人入会", http.MethodPost, apiPath, body, userAccessToken)
}

// VCBotLeaveMeeting 让机器人离开会议
// API: POST /open-apis/vc/v1/bots/leave
// 权限: tenant_access_token + vc:meeting.bot.join:write（与入会同一 scope）
func VCBotLeaveMeeting(meetingID string, userAccessToken string) (json.RawMessage, error) {
	body := map[string]any{
		"meeting_id": meetingID,
	}

	apiPath := fmt.Sprintf("%s/bots/leave", vcBase)
	return vcCallAPI("机器人离会", http.MethodPost, apiPath, body, userAccessToken)
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
// 权限:
//   - User：vc:meeting.meetingevent:read
//   - Bot：vc:meeting.bot.join:write（机器人须在会中）
//
// 身份由 CLI --as bot|user|auto 显式选择，禁止静默回落。
//
// 返回 data 字段原始 JSON（事件列表字段为 events，另含 page_token、has_more；对齐官方 vc_meeting_events.go）
func VCBotMeetingEvents(req VCBotEventsReq, userAccessToken string) (json.RawMessage, error) {
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

	return vcCallAPI("查询机器人会议事件", http.MethodGet, apiPath, nil, userAccessToken)
}

// GetMeetingNote 获取会议纪要文档引用
// API: GET /open-apis/vc/v1/notes/{note_id}
// 返回 data.note 原始 JSON（含 artifacts[].artifact_type/doc_token 和 references[].doc_token）
func GetMeetingNote(noteID string, userAccessToken string) (json.RawMessage, error) {
	apiPath := fmt.Sprintf("%s/notes/%s", vcBase, url.PathEscape(noteID))

	return vcCallAPI("获取会议纪要", http.MethodGet, apiPath, nil, userAccessToken)
}

// ListActiveMeetings 查询当前身份（或 Bot 身份下指定用户）正在参加的会议
// API: GET /open-apis/vc/v1/bots/user_active_meeting
// 权限：User 需 vc:meeting.meetingevent:read；Bot 需 vc:meeting.bot.join:write，且必须传 user_id（open_id）。
// 返回 data 原始 JSON（含 meetings[].meeting_id / meeting_no / meeting_title）。
func ListActiveMeetings(userID, userAccessToken string) (json.RawMessage, error) {
	apiPath := fmt.Sprintf("%s/bots/user_active_meeting", vcBase)
	if userID != "" {
		apiPath += "?" + url.Values{"user_id": []string{userID}}.Encode()
	}
	return vcCallAPI("查询进行中的会议", http.MethodGet, apiPath, nil, userAccessToken)
}
