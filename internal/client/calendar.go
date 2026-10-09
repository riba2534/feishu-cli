package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkcalendar "github.com/larksuite/oapi-sdk-go/v3/service/calendar/v4"
)

// Calendar 日历信息
type Calendar struct {
	CalendarID   string `json:"calendar_id"`
	Summary      string `json:"summary"`
	Description  string `json:"description,omitempty"`
	Permissions  string `json:"permissions,omitempty"`
	Type         string `json:"type,omitempty"`
	Color        int    `json:"color,omitempty"`
	Role         string `json:"role,omitempty"`
	SummaryAlias string `json:"summary_alias,omitempty"`
	IsDeleted    bool   `json:"is_deleted,omitempty"`
	IsThirdParty bool   `json:"is_third_party,omitempty"`
}

// CalendarEvent 日程信息
type CalendarEvent struct {
	EventID     string `json:"event_id"`
	OrganizerID string `json:"organizer_calendar_id,omitempty"`
	Summary     string `json:"summary"`
	Description string `json:"description,omitempty"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	TimeZone    string `json:"time_zone,omitempty"`
	Location    string `json:"location,omitempty"`
	Status      string `json:"status,omitempty"`
	Visibility  string `json:"visibility,omitempty"`
	CreateTime  string `json:"create_time,omitempty"`
	RecurringID string `json:"recurring_event_id,omitempty"`
	Recurrence  string `json:"recurrence,omitempty"` // 重复日程规则（RFC5545 RRULE）
	IsException bool   `json:"is_exception,omitempty"`
	// AppLink 是带**查看者本人** calendarId 的客户端跳转链接，不能用于分享给他人；
	// 分享日程请用 ShareLink（events/share_info）。保留字段仅为 JSON 兼容。
	AppLink string `json:"app_link,omitempty"`
	Color   int    `json:"color,omitempty"`
	// IsAllDay 标记全天日程。为 true 时 StartTime/EndTime 是 YYYY-MM-DD 日期，
	// 且 EndTime 已归一化为**包含端**日期（服务端 end.date 是排他的次日），
	// 否则用户会看到比实际晚一天的结束日。
	IsAllDay bool `json:"is_all_day,omitempty"`

	FreeBusyStatus  string          `json:"free_busy_status,omitempty"`
	SelfRSVPStatus  string          `json:"self_rsvp_status,omitempty"` // 当前身份对该日程的答复状态
	AttendeeAbility string          `json:"attendee_ability,omitempty"`
	Vchat           *EventVchat     `json:"vchat,omitempty"`
	Reminders       []EventReminder `json:"reminders,omitempty"`
	EventOrganizer  *EventOrganizer `json:"event_organizer,omitempty"`
	ShareLink       string          `json:"share_link,omitempty"` // 日程分享链接（需显式请求，见 GetEventShareLink）

	// 以下为重复日程分类/截断所需的原始时间，不输出到 JSON
	rawStart *larkcalendar.TimeInfo
	rawEnd   *larkcalendar.TimeInfo
}

// EventVchat 日程上的视频会议信息
type EventVchat struct {
	VcType      string `json:"vc_type,omitempty"`
	IconType    string `json:"icon_type,omitempty"`
	Description string `json:"description,omitempty"`
	MeetingURL  string `json:"meeting_url,omitempty"`
}

// EventReminder 日程提醒（开始前 N 分钟；负数表示开始后）
type EventReminder struct {
	Minutes int `json:"minutes"`
}

// EventOrganizer 日程组织者
type EventOrganizer struct {
	UserID      string `json:"user_id,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

// ListCalendars 列出日历
func ListCalendars(pageSize int, pageToken string, userAccessToken string) ([]*Calendar, string, bool, error) {
	client, err := GetClient()
	if err != nil {
		return nil, "", false, err
	}

	reqBuilder := larkcalendar.NewListCalendarReqBuilder()
	if pageSize > 0 {
		reqBuilder.PageSize(pageSize)
	}
	if pageToken != "" {
		reqBuilder.PageToken(pageToken)
	}

	resp, err := client.Calendar.Calendar.List(Context(), reqBuilder.Build(), UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, "", false, fmt.Errorf("获取日历列表失败: %w", err)
	}

	if !resp.Success() {
		return nil, "", false, fmt.Errorf("获取日历列表失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	var calendars []*Calendar
	if resp.Data != nil && resp.Data.CalendarList != nil {
		for _, item := range resp.Data.CalendarList {
			calendars = append(calendars, &Calendar{
				CalendarID:   StringVal(item.CalendarId),
				Summary:      StringVal(item.Summary),
				Description:  StringVal(item.Description),
				Permissions:  StringVal(item.Permissions),
				Type:         StringVal(item.Type),
				Color:        IntVal(item.Color),
				Role:         StringVal(item.Role),
				SummaryAlias: StringVal(item.SummaryAlias),
				IsDeleted:    BoolVal(item.IsDeleted),
				IsThirdParty: BoolVal(item.IsThirdParty),
			})
		}
	}

	var nextPageToken string
	var hasMore bool
	if resp.Data != nil {
		nextPageToken = StringVal(resp.Data.PageToken)
		hasMore = BoolVal(resp.Data.HasMore)
	}

	return calendars, nextPageToken, hasMore, nil
}

// CreateEventParams 创建日程的参数
type CreateEventParams struct {
	CalendarID  string
	Summary     string
	Description string
	StartTime   string // RFC3339 格式
	EndTime     string // RFC3339 格式
	TimeZone    string
	Location    string
	Recurrence  string // 重复日程规则（RFC5545 RRULE），如 FREQ=WEEKLY;BYDAY=MO
	// WithVChat 为 true 时同时创建飞书视频会议（vchat.vc_type=vc），日程详情会带会议链接
	WithVChat bool
}

// CreateEvent 创建日程
func CreateEvent(params *CreateEventParams, userAccessToken string) (*CalendarEvent, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	startTs, err := parseTimeToTimestamp(params.StartTime)
	if err != nil {
		return nil, fmt.Errorf("解析开始时间失败: %w", err)
	}
	endTs, err := parseTimeToTimestamp(params.EndTime)
	if err != nil {
		return nil, fmt.Errorf("解析结束时间失败: %w", err)
	}

	startTime := larkcalendar.NewTimeInfoBuilder().
		Timestamp(startTs).
		Build()
	endTime := larkcalendar.NewTimeInfoBuilder().
		Timestamp(endTs).
		Build()

	eventBuilder := larkcalendar.NewCalendarEventBuilder().
		Summary(params.Summary).
		StartTime(startTime).
		EndTime(endTime)

	if params.Description != "" {
		eventBuilder.Description(params.Description)
	}

	if params.Location != "" {
		location := larkcalendar.NewEventLocationBuilder().
			Name(params.Location).
			Build()
		eventBuilder.Location(location)
	}

	if params.Recurrence != "" {
		eventBuilder.Recurrence(params.Recurrence)
	}

	if params.WithVChat {
		eventBuilder.Vchat(larkcalendar.NewVchatBuilder().VcType("vc").Build())
	}

	req := larkcalendar.NewCreateCalendarEventReqBuilder().
		CalendarId(params.CalendarID).
		CalendarEvent(eventBuilder.Build()).
		Build()

	resp, err := client.Calendar.CalendarEvent.Create(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("创建日程失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("创建日程失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data == nil || resp.Data.Event == nil {
		return nil, fmt.Errorf("创建日程成功但未返回日程信息")
	}

	return convertEvent(resp.Data.Event), nil
}

// GetEvent 获取日程详情（GET /calendars/{calendar_id}/events/{event_id}）。
//
// 直接走 HTTP 而非 SDK：SDK 结构缺 self_rsvp_status；业务错误（如 193001 日程不存在）
// 随 HTTP 400 下发时也能先按业务码解析。重复日程的实例 ID（{uid}_{原始时间戳}）同样可读，
// 返回体带 recurring_event_id / is_exception，供删除/更新前判断重复日程类型。
func GetEvent(calendarID, eventID string, userAccessToken string) (*CalendarEvent, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Get(Context(), calendarEventPath(calendarID, eventID), nil, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("获取日程详情失败: %w", err)
	}
	if err := CheckAPIResponse("获取日程详情", resp); err != nil {
		return nil, err
	}
	var apiResp struct {
		Data struct {
			Event *calendarEventWire `json:"event"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析日程详情失败: %w", err)
	}
	if apiResp.Data.Event == nil {
		return nil, fmt.Errorf("日程不存在")
	}
	ev := convertEvent(&apiResp.Data.Event.CalendarEvent)
	ev.SelfRSVPStatus = apiResp.Data.Event.SelfRsvpStatus
	return ev, nil
}

// GetEventShareLink 获取日程分享链接（POST /calendars/{calendar_id}/events/{event_id}/share_info）。
//
// 分享日程给他人/群/文档要用这个链接；app_link 带查看者本人的 calendarId，不适合分享。
func GetEventShareLink(calendarID, eventID string, userAccessToken string) (string, error) {
	cli, err := GetClient()
	if err != nil {
		return "", err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	var resp *larkcore.ApiResp
	// share_info 实测容易触发 190010（日历操作限流），只读接口，退避重试几次
	err = withCalendarRateLimitRetry(func() error {
		r, err := cli.Post(Context(), calendarEventPath(calendarID, eventID)+"/share_info", map[string]any{}, tokenType, opts...)
		if err != nil {
			return fmt.Errorf("获取日程分享链接失败: %w", err)
		}
		if err := CheckAPIResponse("获取日程分享链接", r); err != nil {
			return err
		}
		resp = r
		return nil
	})
	if err != nil {
		return "", err
	}
	var apiResp struct {
		Data struct {
			ShareLink string `json:"share_link"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return "", fmt.Errorf("解析日程分享链接失败: %w", err)
	}
	if apiResp.Data.ShareLink == "" {
		return "", fmt.Errorf("获取日程分享链接失败: 服务端未返回 share_link")
	}
	return apiResp.Data.ShareLink, nil
}

// ListEventsParams 列出日程的参数
type ListEventsParams struct {
	CalendarID string
	StartTime  string // RFC3339 格式，可选
	EndTime    string // RFC3339 格式，可选
	PageSize   int
	PageToken  string
}

// ListEvents 列出日程
func ListEvents(params *ListEventsParams, userAccessToken string) ([]*CalendarEvent, string, bool, error) {
	client, err := GetClient()
	if err != nil {
		return nil, "", false, err
	}

	reqBuilder := larkcalendar.NewListCalendarEventReqBuilder().
		CalendarId(params.CalendarID)

	if params.StartTime != "" {
		startTs, err := parseTimeToTimestamp(params.StartTime)
		if err != nil {
			return nil, "", false, fmt.Errorf("解析开始时间失败: %w", err)
		}
		reqBuilder.StartTime(startTs)
	}

	if params.EndTime != "" {
		endTs, err := parseTimeToTimestamp(params.EndTime)
		if err != nil {
			return nil, "", false, fmt.Errorf("解析结束时间失败: %w", err)
		}
		reqBuilder.EndTime(endTs)
	}

	if params.PageSize > 0 {
		reqBuilder.PageSize(params.PageSize)
	}

	if params.PageToken != "" {
		reqBuilder.PageToken(params.PageToken)
	}

	resp, err := client.Calendar.CalendarEvent.List(Context(), reqBuilder.Build(), UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, "", false, fmt.Errorf("获取日程列表失败: %w", err)
	}

	if !resp.Success() {
		return nil, "", false, fmt.Errorf("获取日程列表失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	var events []*CalendarEvent
	if resp.Data != nil && resp.Data.Items != nil {
		for _, item := range resp.Data.Items {
			events = append(events, convertEvent(item))
		}
	}

	var nextPageToken string
	var hasMore bool
	if resp.Data != nil {
		nextPageToken = StringVal(resp.Data.PageToken)
		hasMore = BoolVal(resp.Data.HasMore)
	}

	return events, nextPageToken, hasMore, nil
}

// UpdateEventParams 更新日程的参数
type UpdateEventParams struct {
	CalendarID  string
	EventID     string
	Summary     string
	Description string
	StartTime   string // RFC3339 格式
	EndTime     string // RFC3339 格式
	Location    string
	Recurrence  string // 重复日程规则（RFC5545 RRULE），如 FREQ=WEEKLY;BYDAY=MO
	// NeedNotification 为 nil 时不传（服务端默认通知参与人）
	NeedNotification *bool
}

// HasFields 报告是否至少有一个待更新字段（NeedNotification 不算）。
func (p *UpdateEventParams) HasFields() bool {
	return p.Summary != "" || p.Description != "" || p.StartTime != "" || p.EndTime != "" ||
		p.Location != "" || p.Recurrence != ""
}

// UpdateEvent 更新日程（使用 Patch 方式）
func UpdateEvent(params *UpdateEventParams, userAccessToken string) (*CalendarEvent, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	eventBuilder := larkcalendar.NewCalendarEventBuilder()

	if params.Summary != "" {
		eventBuilder.Summary(params.Summary)
	}

	if params.Description != "" {
		eventBuilder.Description(params.Description)
	}

	if params.StartTime != "" {
		startTs, err := parseTimeToTimestamp(params.StartTime)
		if err != nil {
			return nil, fmt.Errorf("解析开始时间失败: %w", err)
		}
		startTime := larkcalendar.NewTimeInfoBuilder().
			Timestamp(startTs).
			Build()
		eventBuilder.StartTime(startTime)
	}

	if params.EndTime != "" {
		endTs, err := parseTimeToTimestamp(params.EndTime)
		if err != nil {
			return nil, fmt.Errorf("解析结束时间失败: %w", err)
		}
		endTime := larkcalendar.NewTimeInfoBuilder().
			Timestamp(endTs).
			Build()
		eventBuilder.EndTime(endTime)
	}

	if params.Location != "" {
		location := larkcalendar.NewEventLocationBuilder().
			Name(params.Location).
			Build()
		eventBuilder.Location(location)
	}

	if params.Recurrence != "" {
		eventBuilder.Recurrence(params.Recurrence)
	}

	if params.NeedNotification != nil {
		eventBuilder.NeedNotification(*params.NeedNotification)
	}

	req := larkcalendar.NewPatchCalendarEventReqBuilder().
		CalendarId(params.CalendarID).
		EventId(params.EventID).
		CalendarEvent(eventBuilder.Build()).
		Build()

	resp, err := client.Calendar.CalendarEvent.Patch(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("更新日程失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("更新日程失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data == nil || resp.Data.Event == nil {
		return nil, fmt.Errorf("更新日程成功但未返回日程信息")
	}

	return convertEvent(resp.Data.Event), nil
}

// DeleteEventOptions 删除日程的可选参数
type DeleteEventOptions struct {
	// NeedNotification 为 nil 时不传（服务端默认通知参与人）
	NeedNotification *bool
	// DeleteException 为 true 时把例外日程彻底销毁（is_deleted=true），而不是留下 cancelled 占位；
	// 仅用于 --apply-to all / this-and-following 的例外清理。
	DeleteException bool
}

// larkErrCalendarEventDeleted 日程已被删除（重复清理时视为成功，保持幂等）
const larkErrCalendarEventDeleted = 193003

// DeleteEvent 删除日程。
//
// 语义（实测）：传主日程 ID（{uid}_0）删除整条重复序列，但**不级联**已单独修改过的例外日程；
// 传实例 ID（{uid}_{原始时间戳}）只删除这一次（服务端把它标记为 cancelled 例外）。
func DeleteEvent(calendarID, eventID string, userAccessToken string) error {
	return DeleteEventWithOptions(calendarID, eventID, DeleteEventOptions{}, userAccessToken)
}

// DeleteEventWithOptions 删除日程（DELETE /calendars/{calendar_id}/events/{event_id}）。
// 193003（日程已删除）视为成功，使重复执行/并发清理幂等。
func DeleteEventWithOptions(calendarID, eventID string, opts DeleteEventOptions, userAccessToken string) error {
	cli, err := GetClient()
	if err != nil {
		return err
	}
	q := url.Values{}
	if opts.NeedNotification != nil {
		q.Set("need_notification", strconv.FormatBool(*opts.NeedNotification))
	}
	if opts.DeleteException {
		q.Set("delete_exception", "true")
	}
	apiPath := calendarEventPath(calendarID, eventID)
	if len(q) > 0 {
		apiPath += "?" + q.Encode()
	}
	tokenType, reqOpts := resolveTokenOpts(userAccessToken)
	return withCalendarRateLimitRetry(func() error {
		resp, err := cli.Delete(Context(), apiPath, nil, tokenType, reqOpts...)
		if err != nil {
			return fmt.Errorf("删除日程失败: %w", err)
		}
		if err := CheckAPIResponse("删除日程", resp); err != nil {
			if apiErr, ok := AsAPIError(err); ok && apiErr.Code == larkErrCalendarEventDeleted {
				return nil
			}
			return err
		}
		return nil
	})
}

// 辅助函数：将时间输入转换为秒级时间戳字符串。
// 支持 RFC3339（推荐，带时区）、不带时区的 "YYYY-MM-DD HH:MM[:SS]" / "YYYY-MM-DDTHH:MM[:SS]"
// （按本地时区）、YYYY-MM-DD（当天 00:00）以及 Unix 秒/毫秒。
func parseTimeToTimestamp(timeStr string) (string, error) {
	t, err := ParseTimeInput(timeStr, false)
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(t.Unix(), 10), nil
}

// ParseEventTimeToUnix 解析日历写命令的时间参数（规则同 parseTimeToTimestamp），返回 Unix 秒。
func ParseEventTimeToUnix(timeStr string) (int64, error) {
	t, err := ParseTimeInput(timeStr, false)
	if err != nil {
		return 0, err
	}
	return t.Unix(), nil
}

// 辅助函数：将时间戳字符串转换为 RFC3339 格式
func timestampToRFC3339(ts string, tz string) string {
	if ts == "" {
		return ""
	}
	timestamp, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return ts
	}

	loc := time.Local
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}

	return time.Unix(timestamp, 0).In(loc).Format(time.RFC3339)
}

// EventAttendee 日程参与人
type EventAttendee struct {
	Type            string `json:"type"`                        // user/chat/resource/third_party
	AttendeeID      string `json:"attendee_id,omitempty"`       // 参与人 ID
	UserID          string `json:"user_id,omitempty"`           // 用户 ID
	ChatID          string `json:"chat_id,omitempty"`           // 群 ID
	RoomID          string `json:"room_id,omitempty"`           // 会议室 ID
	ThirdPartyEmail string `json:"third_party_email,omitempty"` // 第三方邮箱
	DisplayName     string `json:"display_name,omitempty"`      // 显示名称
	RsvpStatus      string `json:"rsvp_status,omitempty"`       // 响应状态
	IsOptional      bool   `json:"is_optional,omitempty"`       // 是否可选参加
	IsOrganizer     bool   `json:"is_organizer,omitempty"`      // 是否组织者
	IsExternal      bool   `json:"is_external,omitempty"`       // 是否外部参与人
}

// FreebusyInfo 忙闲信息
type FreebusyInfo struct {
	StartTime string `json:"start_time"` // RFC3339
	EndTime   string `json:"end_time"`   // RFC3339
}

// GetCalendar 获取日历详情
func GetCalendar(calendarID string, userAccessToken string) (*Calendar, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larkcalendar.NewGetCalendarReqBuilder().
		CalendarId(calendarID).
		Build()

	resp, err := client.Calendar.Calendar.Get(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("获取日历详情失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取日历详情失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data == nil {
		return nil, fmt.Errorf("日历不存在")
	}

	return &Calendar{
		CalendarID:   StringVal(resp.Data.CalendarId),
		Summary:      StringVal(resp.Data.Summary),
		Description:  StringVal(resp.Data.Description),
		Permissions:  StringVal(resp.Data.Permissions),
		Type:         StringVal(resp.Data.Type),
		Color:        IntVal(resp.Data.Color),
		Role:         StringVal(resp.Data.Role),
		SummaryAlias: StringVal(resp.Data.SummaryAlias),
		IsDeleted:    BoolVal(resp.Data.IsDeleted),
		IsThirdParty: BoolVal(resp.Data.IsThirdParty),
	}, nil
}

// GetPrimaryCalendar 获取主日历
func GetPrimaryCalendar(userAccessToken string) (*Calendar, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larkcalendar.NewPrimaryCalendarReqBuilder().Build()

	resp, err := client.Calendar.Calendar.Primary(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, fmt.Errorf("获取主日历失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取主日历失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data == nil || len(resp.Data.Calendars) == 0 {
		return nil, fmt.Errorf("未找到主日历")
	}

	cal := resp.Data.Calendars[0].Calendar
	if cal == nil {
		return nil, fmt.Errorf("主日历数据为空")
	}

	return &Calendar{
		CalendarID:   StringVal(cal.CalendarId),
		Summary:      StringVal(cal.Summary),
		Description:  StringVal(cal.Description),
		Permissions:  StringVal(cal.Permissions),
		Type:         StringVal(cal.Type),
		Color:        IntVal(cal.Color),
		Role:         StringVal(cal.Role),
		SummaryAlias: StringVal(cal.SummaryAlias),
		IsDeleted:    BoolVal(cal.IsDeleted),
		IsThirdParty: BoolVal(cal.IsThirdParty),
	}, nil
}

// InstanceRelationInfo 日历事件实例的关联信息（会议实例 ID + 妙记 token）
type InstanceRelationInfo struct {
	MeetingInstanceIDs []string `json:"meeting_instance_ids,omitempty"`
	MeetingNotes       []string `json:"meeting_notes,omitempty"` // minute_tokens
}

// MgetInstanceRelationInfo 批量查询日历事件实例的会议/妙记关联信息
// API: POST /open-apis/calendar/v4/calendars/{calendar_id}/events/mget_instance_relation_info
// 返回 map[instance_id]InstanceRelationInfo
func MgetInstanceRelationInfo(calendarID string, instanceIDs []string, needNotes bool, userAccessToken string) (map[string]*InstanceRelationInfo, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	body := map[string]any{
		"instance_ids":              instanceIDs,
		"need_meeting_instance_ids": true,
	}
	if needNotes {
		body["need_meeting_notes"] = true
	}

	tokenType, opts := resolveTokenOpts(userAccessToken)
	apiPath := fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/mget_instance_relation_info", calendarID)

	resp, err := client.Post(Context(), apiPath, body, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("查询日历事件实例关联信息失败: %w", err)
	}

	if err := CheckAPIResponse("查询日历事件实例关联信息", resp); err != nil {
		return nil, err
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			InstanceRelationInfos []struct {
				InstanceID         string   `json:"instance_id"`
				MeetingInstanceIDs []string `json:"meeting_instance_ids"`
				MeetingNotes       []string `json:"meeting_notes"`
			} `json:"instance_relation_infos"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("查询日历事件实例关联信息失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	result := make(map[string]*InstanceRelationInfo, len(apiResp.Data.InstanceRelationInfos))
	for _, info := range apiResp.Data.InstanceRelationInfos {
		result[info.InstanceID] = &InstanceRelationInfo{
			MeetingInstanceIDs: info.MeetingInstanceIDs,
			MeetingNotes:       info.MeetingNotes,
		}
	}
	return result, nil
}

const (
	searchEventDefaultPageSize = 20
	searchEventMaxPageSize     = 30
)

// SearchEventsParams 是 current search_event 端点的请求参数。
type SearchEventsParams struct {
	CalendarID  string
	Query       string
	StartTime   string // RFC3339，写入 filter.time_range.start_time
	EndTime     string // RFC3339，写入 filter.time_range.end_time
	AttendeeIDs []string
	PageToken   string
	PageSize    int
}

type searchEventTimeRange struct {
	StartTime string `json:"start_time,omitempty"`
	EndTime   string `json:"end_time,omitempty"`
}

type searchEventFilter struct {
	AttendeeUserIDs []string              `json:"attendee_user_ids,omitempty"`
	AttendeeChatIDs []string              `json:"attendee_chat_ids,omitempty"`
	MeetingRoomIDs  []string              `json:"meeting_room_ids,omitempty"`
	TimeRange       *searchEventTimeRange `json:"time_range,omitempty"`
}

type searchEventRequestBody struct {
	Query  string             `json:"query"`
	Filter *searchEventFilter `json:"filter,omitempty"`
}

func buildSearchEventFilter(startTime, endTime string, attendeeIDs []string) *searchEventFilter {
	var userIDs, chatIDs, roomIDs []string
	for _, id := range attendeeIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		switch {
		case strings.HasPrefix(id, "ou_"):
			userIDs = append(userIDs, id)
		case strings.HasPrefix(id, "oc_"):
			chatIDs = append(chatIDs, id)
		case strings.HasPrefix(id, "omm_"):
			roomIDs = append(roomIDs, id)
		default:
			userIDs = append(userIDs, id)
		}
	}
	var tr *searchEventTimeRange
	if startTime != "" || endTime != "" {
		tr = &searchEventTimeRange{StartTime: startTime, EndTime: endTime}
	}
	if len(userIDs) == 0 && len(chatIDs) == 0 && len(roomIDs) == 0 && tr == nil {
		return nil
	}
	return &searchEventFilter{
		AttendeeUserIDs: userIDs,
		AttendeeChatIDs: chatIDs,
		MeetingRoomIDs:  roomIDs,
		TimeRange:       tr,
	}
}

// allDayInclusiveEndDate 把全天日程的**排他** end.date（YYYY-MM-DD，实为次日）
// 换成包含端（减一天）。解析失败时原样返回，避免把无法识别的格式改坏。
func allDayInclusiveEndDate(date string) string {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(date))
	if err != nil {
		return date
	}
	return t.AddDate(0, 0, -1).Format("2006-01-02")
}

func searchEventTimeText(info *struct {
	Date     string `json:"date"`
	DateTime string `json:"date_time"`
	Timezone string `json:"timezone"`
}) string {
	if info == nil {
		return ""
	}
	if info.DateTime != "" {
		return info.DateTime
	}
	return info.Date
}

// SearchEventsResult 是 current search_event 的完整结果（含 has_more）。
type SearchEventsResult struct {
	Events    []*CalendarEvent
	PageToken string
	HasMore   bool
}

// SearchEvents 搜索日程（POST /calendars/{id}/events/search_event）。
// 保留旧签名：只返回 events 与 page_token；完整结果用 SearchEventsWithParams。
func SearchEvents(calendarID, query string, startTime, endTime string, pageToken string, pageSize int, userAccessToken string) ([]*CalendarEvent, string, error) {
	res, err := SearchEventsWithParams(SearchEventsParams{
		CalendarID: calendarID,
		Query:      query,
		StartTime:  startTime,
		EndTime:    endTime,
		PageToken:  pageToken,
		PageSize:   pageSize,
	}, userAccessToken)
	if err != nil {
		return nil, "", err
	}
	return res.Events, res.PageToken, nil
}

// SearchEventsWithParams 按 current search_event 契约搜索日程。
func SearchEventsWithParams(params SearchEventsParams, userAccessToken string) (*SearchEventsResult, error) {
	if strings.TrimSpace(params.CalendarID) == "" {
		params.CalendarID = "primary"
	}
	pageSize, err := ResolvePageSize(params.PageSize, searchEventDefaultPageSize, 1, searchEventMaxPageSize)
	if err != nil {
		return nil, fmt.Errorf("搜索日程失败: %w", err)
	}

	cli, err := GetClient()
	if err != nil {
		return nil, err
	}

	body := searchEventRequestBody{
		Query:  params.Query,
		Filter: buildSearchEventFilter(params.StartTime, params.EndTime, params.AttendeeIDs),
	}

	q := url.Values{}
	q.Set("page_size", strconv.Itoa(pageSize))
	if params.PageToken != "" {
		q.Set("page_token", params.PageToken)
	}
	apiPath := fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/search_event?%s",
		url.PathEscape(params.CalendarID), q.Encode())

	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Post(Context(), apiPath, body, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("搜索日程失败: %w", err)
	}
	if err := CheckAPIResponse("搜索日程", resp); err != nil {
		return nil, err
	}

	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Items []struct {
				MetaData *struct {
					EventID  string `json:"event_id"`
					Summary  string `json:"summary"`
					AppLink  string `json:"app_link"`
					IsAllDay bool   `json:"is_all_day"`
					Start    *struct {
						Date     string `json:"date"`
						DateTime string `json:"date_time"`
						Timezone string `json:"timezone"`
					} `json:"start"`
					End *struct {
						Date     string `json:"date"`
						DateTime string `json:"date_time"`
						Timezone string `json:"timezone"`
					} `json:"end"`
				} `json:"meta_data"`
			} `json:"items"`
			PageToken string `json:"page_token"`
			HasMore   bool   `json:"has_more"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析搜索日程响应失败: %w", err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("搜索日程失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}

	var events []*CalendarEvent
	for _, item := range apiResp.Data.Items {
		if item.MetaData == nil {
			continue
		}
		meta := item.MetaData
		ev := &CalendarEvent{
			EventID:   meta.EventID,
			Summary:   meta.Summary,
			AppLink:   meta.AppLink,
			StartTime: searchEventTimeText(meta.Start),
			EndTime:   searchEventTimeText(meta.End),
		}
		if meta.Start != nil && meta.Start.Timezone != "" {
			ev.TimeZone = meta.Start.Timezone
		}
		// 全天日程：服务端 end.date 是**排他**日期（次日），直接展示会比实际晚一天。
		// 回填 IsAllDay 并把 end 换成包含端，否则用户无从分辨（此前该字段被解析后丢弃）。
		if meta.IsAllDay {
			ev.IsAllDay = true
			if meta.End != nil && meta.End.DateTime == "" && meta.End.Date != "" {
				ev.EndTime = allDayInclusiveEndDate(meta.End.Date)
			}
		}
		events = append(events, ev)
	}
	return &SearchEventsResult{
		Events:    events,
		PageToken: apiResp.Data.PageToken,
		HasMore:   apiResp.Data.HasMore,
	}, nil
}

// AddEventAttendees 添加日程参与人
func AddEventAttendees(calendarID, eventID string, attendees []*EventAttendee, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	var sdkAttendees []*larkcalendar.CalendarEventAttendee
	for _, a := range attendees {
		builder := larkcalendar.NewCalendarEventAttendeeBuilder().
			Type(a.Type)
		if a.UserID != "" {
			builder.UserId(a.UserID)
		}
		if a.ChatID != "" {
			builder.ChatId(a.ChatID)
		}
		if a.RoomID != "" {
			builder.RoomId(a.RoomID)
		}
		if a.ThirdPartyEmail != "" {
			builder.ThirdPartyEmail(a.ThirdPartyEmail)
		}
		sdkAttendees = append(sdkAttendees, builder.Build())
	}

	body := larkcalendar.NewCreateCalendarEventAttendeeReqBodyBuilder().
		Attendees(sdkAttendees).
		NeedNotification(true).
		Build()

	req := larkcalendar.NewCreateCalendarEventAttendeeReqBuilder().
		CalendarId(calendarID).
		EventId(eventID).
		Body(body).
		Build()

	resp, err := client.Calendar.CalendarEventAttendee.Create(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return fmt.Errorf("添加日程参与人失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("添加日程参与人失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// AttendeeRef 待添加/移除的参与人（按 ID 前缀识别类型）
type AttendeeRef struct {
	Type            string `json:"type"`                        // user / chat / resource / third_party
	UserID          string `json:"user_id,omitempty"`           // ou_
	ChatID          string `json:"chat_id,omitempty"`           // oc_
	RoomID          string `json:"room_id,omitempty"`           // omm_
	ThirdPartyEmail string `json:"third_party_email,omitempty"` // 外部邮箱
	AttendeeID      string `json:"-"`                           // attendee list 返回的 attendee_id（仅移除时使用）
}

// ParseAttendeeRefs 按前缀把 ID 列表解析为参与人：ou_→用户、oc_→群、omm_→会议室、含 @ →外部邮箱。
// allowAttendeeID 为 true 时，其余 ID 视为 attendee list 返回的 attendee_id（仅移除场景可用）；
// 否则报错。重复 ID 自动去重。
func ParseAttendeeRefs(ids []string, allowAttendeeID bool) ([]*AttendeeRef, error) {
	seen := map[string]bool{}
	var out []*AttendeeRef
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		switch {
		case strings.HasPrefix(id, "ou_"):
			out = append(out, &AttendeeRef{Type: "user", UserID: id})
		case strings.HasPrefix(id, "oc_"):
			out = append(out, &AttendeeRef{Type: "chat", ChatID: id})
		case strings.HasPrefix(id, "omm_"):
			out = append(out, &AttendeeRef{Type: "resource", RoomID: id})
		case strings.Contains(id, "@"):
			out = append(out, &AttendeeRef{Type: "third_party", ThirdPartyEmail: id})
		case allowAttendeeID:
			out = append(out, &AttendeeRef{AttendeeID: id})
		default:
			return nil, fmt.Errorf("无法识别的参与人 ID %q：用户用 ou_ 开头的 open_id，群用 oc_，会议室用 omm_，外部参与人用邮箱", id)
		}
	}
	return out, nil
}

// AttendeeRefsToEventAttendees 转为 AddEventAttendees 的入参
func AttendeeRefsToEventAttendees(refs []*AttendeeRef) []*EventAttendee {
	out := make([]*EventAttendee, 0, len(refs))
	for _, r := range refs {
		if r == nil || r.Type == "" {
			continue
		}
		out = append(out, &EventAttendee{
			Type:            r.Type,
			UserID:          r.UserID,
			ChatID:          r.ChatID,
			RoomID:          r.RoomID,
			ThirdPartyEmail: r.ThirdPartyEmail,
		})
	}
	return out
}

// RemoveEventAttendeesPath 返回移除日程参与人接口路径（不含 query）。
func RemoveEventAttendeesPath(calendarID, eventID string) string {
	return calendarEventPath(calendarID, eventID) + "/attendees/batch_delete"
}

// RemoveEventAttendeesParams 是移除日程参与人请求固定携带的 query 参数。
func RemoveEventAttendeesParams() map[string]any {
	return map[string]any{"user_id_type": "open_id"}
}

// BuildRemoveEventAttendeesBody 构造 batch_delete 请求体（dry-run 预览与真实请求共用）：
// 前缀可识别的 ID 走 delete_ids，其余视为 attendee list 返回的 attendee_id 走 attendee_ids。
func BuildRemoveEventAttendeesBody(refs []*AttendeeRef, needNotification bool) (map[string]any, error) {
	var deleteIDs []map[string]string
	var attendeeIDs []string
	for _, r := range refs {
		if r == nil {
			continue
		}
		switch r.Type {
		case "user":
			deleteIDs = append(deleteIDs, map[string]string{"type": "user", "user_id": r.UserID})
		case "chat":
			deleteIDs = append(deleteIDs, map[string]string{"type": "chat", "chat_id": r.ChatID})
		case "resource":
			deleteIDs = append(deleteIDs, map[string]string{"type": "resource", "room_id": r.RoomID})
		case "third_party":
			deleteIDs = append(deleteIDs, map[string]string{"type": "third_party", "third_party_email": r.ThirdPartyEmail})
		default:
			if r.AttendeeID != "" {
				attendeeIDs = append(attendeeIDs, r.AttendeeID)
			}
		}
	}
	if len(deleteIDs) == 0 && len(attendeeIDs) == 0 {
		return nil, fmt.Errorf("移除日程参与人失败: 没有可移除的参与人")
	}
	body := map[string]any{"need_notification": needNotification}
	if len(deleteIDs) > 0 {
		body["delete_ids"] = deleteIDs
	}
	if len(attendeeIDs) > 0 {
		body["attendee_ids"] = attendeeIDs
	}
	return body, nil
}

// RemoveEventAttendees 移除日程参与人
// （POST /calendars/{calendar_id}/events/{event_id}/attendees/batch_delete），请求体见 BuildRemoveEventAttendeesBody。
func RemoveEventAttendees(calendarID, eventID string, refs []*AttendeeRef, needNotification bool, userAccessToken string) error {
	body, err := BuildRemoveEventAttendeesBody(refs, needNotification)
	if err != nil {
		return err
	}
	cli, err := GetClient()
	if err != nil {
		return err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Post(Context(), RemoveEventAttendeesPath(calendarID, eventID)+"?user_id_type=open_id", body, tokenType, opts...)
	if err != nil {
		return fmt.Errorf("移除日程参与人失败: %w", err)
	}
	return CheckAPIResponse("移除日程参与人", resp)
}

// ListEventAttendees 列出日程参与人
func ListEventAttendees(calendarID, eventID string, pageSize int, pageToken string, userAccessToken string) ([]*EventAttendee, string, bool, error) {
	client, err := GetClient()
	if err != nil {
		return nil, "", false, err
	}

	reqBuilder := larkcalendar.NewListCalendarEventAttendeeReqBuilder().
		CalendarId(calendarID).
		EventId(eventID)

	if pageSize > 0 {
		reqBuilder.PageSize(pageSize)
	}
	if pageToken != "" {
		reqBuilder.PageToken(pageToken)
	}

	resp, err := client.Calendar.CalendarEventAttendee.List(Context(), reqBuilder.Build(), UserTokenOption(userAccessToken)...)
	if err != nil {
		return nil, "", false, fmt.Errorf("获取日程参与人列表失败: %w", err)
	}

	if !resp.Success() {
		return nil, "", false, fmt.Errorf("获取日程参与人列表失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	var attendees []*EventAttendee
	if resp.Data != nil && resp.Data.Items != nil {
		for _, item := range resp.Data.Items {
			// 群参与人的 rsvp_status 没有语义（服务端恒返回 needs_action，成员各自的答复要查群成员），
			// 不输出以免被误读为"群未答复"（对齐官方 calendar_list_attendees projectAttendee）。
			rsvp := StringVal(item.RsvpStatus)
			if StringVal(item.Type) == "chat" {
				rsvp = ""
			}
			attendees = append(attendees, &EventAttendee{
				Type:            StringVal(item.Type),
				AttendeeID:      StringVal(item.AttendeeId),
				UserID:          StringVal(item.UserId),
				ChatID:          StringVal(item.ChatId),
				RoomID:          StringVal(item.RoomId),
				ThirdPartyEmail: StringVal(item.ThirdPartyEmail),
				DisplayName:     StringVal(item.DisplayName),
				RsvpStatus:      rsvp,
				IsOptional:      BoolVal(item.IsOptional),
				IsOrganizer:     BoolVal(item.IsOrganizer),
				IsExternal:      BoolVal(item.IsExternal),
			})
		}
	}

	var nextPageToken string
	var hasMore bool
	if resp.Data != nil {
		nextPageToken = StringVal(resp.Data.PageToken)
		hasMore = BoolVal(resp.Data.HasMore)
	}

	return attendees, nextPageToken, hasMore, nil
}

// ListFreebusy 查询单个用户的忙闲（已按开始时间排序并合并重叠/相邻区间）。
// startTime/endTime 为 RFC3339；userID 为 open_id。
func ListFreebusy(startTime, endTime string, userID string, userAccessToken string) ([]*FreebusyInfo, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("查询忙闲信息失败: 缺少用户 ID")
	}
	raw, err := ListFreebusyBatch(startTime, endTime, []string{userID}, userAccessToken)
	if err != nil {
		return nil, err
	}
	return MergeFreebusyIntervals(raw[userID]), nil
}

// FreebusyRawItem 未合并的忙碌时段（含当前用户对该日程的答复状态）
type FreebusyRawItem struct {
	StartTime  string `json:"start_time"`
	EndTime    string `json:"end_time"`
	RSVPStatus string `json:"rsvp_status,omitempty"`
}

// ListFreebusyBatch 批量查询多个用户的忙闲（POST /open-apis/calendar/v4/freebusy/batch）。
// 返回 user_id → 原始忙碌时段（服务端顺序，未合并，可能重叠）。userIDs 为 open_id。
func ListFreebusyBatch(startTime, endTime string, userIDs []string, userAccessToken string) (map[string][]*FreebusyRawItem, error) {
	if len(userIDs) == 0 {
		return nil, fmt.Errorf("查询忙闲信息失败: 缺少用户 ID")
	}
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"time_min":         startTime,
		"time_max":         endTime,
		"user_ids":         userIDs,
		"need_rsvp_status": true,
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Post(Context(), "/open-apis/calendar/v4/freebusy/batch?user_id_type=open_id", body, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("查询忙闲信息失败: %w", err)
	}
	if err := CheckAPIResponse("查询忙闲信息", resp); err != nil {
		return nil, err
	}
	var apiResp struct {
		Data struct {
			FreebusyLists []struct {
				UserID        string             `json:"user_id"`
				FreebusyItems []*FreebusyRawItem `json:"freebusy_items"`
			} `json:"freebusy_lists"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析忙闲信息失败: %w", err)
	}
	out := make(map[string][]*FreebusyRawItem, len(userIDs))
	for _, u := range userIDs {
		out[u] = nil
	}
	for _, l := range apiResp.Data.FreebusyLists {
		if l.UserID == "" {
			continue
		}
		items := make([]*FreebusyRawItem, 0, len(l.FreebusyItems))
		for _, it := range l.FreebusyItems {
			if it == nil || it.StartTime == "" || it.EndTime == "" {
				continue
			}
			items = append(items, it)
		}
		sort.SliceStable(items, func(i, j int) bool { return items[i].StartTime < items[j].StartTime })
		out[l.UserID] = items
	}
	return out, nil
}

// MergeFreebusyIntervals 按开始时间排序并合并重叠或首尾相接的忙碌区间。
// 合并不改变"某一时刻是否忙"的结论，但区间数 != 日程数。无法解析的条目被丢弃。
func MergeFreebusyIntervals(items []*FreebusyRawItem) []*FreebusyInfo {
	type span struct{ s, e time.Time }
	arr := make([]span, 0, len(items))
	for _, it := range items {
		if it == nil {
			continue
		}
		s, err1 := time.Parse(time.RFC3339, it.StartTime)
		e, err2 := time.Parse(time.RFC3339, it.EndTime)
		if err1 != nil || err2 != nil || !e.After(s) {
			continue
		}
		arr = append(arr, span{s, e})
	}
	out := make([]*FreebusyInfo, 0, len(arr))
	if len(arr) == 0 {
		return out
	}
	sort.SliceStable(arr, func(i, j int) bool {
		if !arr[i].s.Equal(arr[j].s) {
			return arr[i].s.Before(arr[j].s)
		}
		return arr[i].e.Before(arr[j].e)
	})
	loc := arr[0].s.Location()
	merged := []span{arr[0]}
	for _, cur := range arr[1:] {
		last := &merged[len(merged)-1]
		if !cur.s.After(last.e) {
			if cur.e.After(last.e) {
				last.e = cur.e
			}
			continue
		}
		merged = append(merged, cur)
	}
	for _, m := range merged {
		out = append(out, &FreebusyInfo{
			StartTime: m.s.In(loc).Format(time.RFC3339),
			EndTime:   m.e.In(loc).Format(time.RFC3339),
		})
	}
	return out
}

// FreeSlot 空闲时段
type FreeSlot struct {
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Duration  string `json:"duration"`
}

// ComputeFreeSlots 计算 [winStart, winEnd] 内不被任一用户忙碌区间覆盖的空闲时段
// （传入单个用户即为个人空闲，多个用户即为共同空闲）。短于 minDur 的时段被丢弃。
func ComputeFreeSlots(usersBusy [][]*FreebusyInfo, winStart, winEnd time.Time, minDur time.Duration) []*FreeSlot {
	var all []*FreebusyRawItem
	for _, busy := range usersBusy {
		for _, b := range busy {
			if b != nil {
				all = append(all, &FreebusyRawItem{StartTime: b.StartTime, EndTime: b.EndTime})
			}
		}
	}
	merged := MergeFreebusyIntervals(all)
	loc := winStart.Location()
	out := make([]*FreeSlot, 0)
	emit := func(from, to time.Time) {
		if !to.After(from) {
			return
		}
		d := to.Sub(from)
		if minDur > 0 && d < minDur {
			return
		}
		out = append(out, &FreeSlot{
			StartTime: from.In(loc).Format(time.RFC3339),
			EndTime:   to.In(loc).Format(time.RFC3339),
			Duration:  d.String(),
		})
	}
	cursor := winStart
	for _, b := range merged {
		s, _ := time.Parse(time.RFC3339, b.StartTime)
		e, _ := time.Parse(time.RFC3339, b.EndTime)
		if s.Before(winStart) {
			s = winStart
		}
		if e.After(winEnd) {
			e = winEnd
		}
		if !e.After(s) {
			continue
		}
		emit(cursor, s)
		if e.After(cursor) {
			cursor = e
		}
	}
	emit(cursor, winEnd)
	return out
}

// ReplyEvent 回复日程（接受/拒绝/待定）
func ReplyEvent(calendarID, eventID, rsvpStatus string, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	body := larkcalendar.NewReplyCalendarEventReqBodyBuilder().
		RsvpStatus(rsvpStatus).
		Build()

	req := larkcalendar.NewReplyCalendarEventReqBuilder().
		CalendarId(calendarID).
		EventId(eventID).
		Body(body).
		Build()

	resp, err := client.Calendar.CalendarEvent.Reply(Context(), req, UserTokenOption(userAccessToken)...)
	if err != nil {
		return fmt.Errorf("回复日程失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("回复日程失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// AgendaEvent 日程视图中的事件实例（展开重复日程后的独立实例）
type AgendaEvent struct {
	EventID        string `json:"event_id"`
	Summary        string `json:"summary"`
	StartTime      string `json:"start_time"`
	EndTime        string `json:"end_time"`
	Status         string `json:"status,omitempty"`
	FreeBusyStatus string `json:"free_busy_status,omitempty"`
	SelfRSVP       string `json:"self_rsvp_status,omitempty"`
	IsAllDay       bool   `json:"is_all_day,omitempty"`
}

const (
	maxInstanceViewSpanSeconds       = 40 * 24 * 60 * 60
	minSplitWindowSeconds            = 2 * 60 * 60
	maxInstanceViewSplitDepth        = 10
	larkErrCalendarTimeRangeExceeded = 193103 // instance_view 查询窗口超过 40 天
	larkErrCalendarTooManyInstances  = 193104 // instance_view 单窗口超过 1000 个实例
)

type agendaTimeInfo struct {
	Timestamp string `json:"timestamp"`
	Date      string `json:"date"`
	Timezone  string `json:"timezone"`
}

type agendaRawItem struct {
	EventID        string          `json:"event_id"`
	Summary        string          `json:"summary"`
	StartTime      *agendaTimeInfo `json:"start_time"`
	EndTime        *agendaTimeInfo `json:"end_time"`
	Status         string          `json:"status"`
	FreeBusyStatus string          `json:"free_busy_status"`
	SelfRSVP       string          `json:"self_rsvp_status"`
}

func instanceViewPath(calendarID string, startTime, endTime int64) string {
	q := url.Values{}
	q.Set("start_time", strconv.FormatInt(startTime, 10))
	q.Set("end_time", strconv.FormatInt(endTime, 10))
	return fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/instance_view?%s",
		url.PathEscape(calendarID), q.Encode())
}

func fetchInstanceViewRange(calendarID string, startTime, endTime int64, depth int, userAccessToken string) ([]agendaRawItem, error) {
	if depth > maxInstanceViewSplitDepth {
		return nil, fmt.Errorf("获取日程视图失败: 拆分次数过多")
	}
	if startTime > endTime {
		return nil, nil
	}
	span := endTime - startTime
	if span > maxInstanceViewSpanSeconds {
		mid := startTime + span/2
		return fetchInstanceViewSplit(calendarID, startTime, mid, endTime, depth, userAccessToken)
	}

	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Get(Context(), instanceViewPath(calendarID, startTime, endTime), nil, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("获取日程视图失败: %w", err)
	}
	// 先解析 body 取业务码，再判 HTTP 状态：193103/193104 是随 HTTP 400 下发的，
	// 若先按 StatusCode != 200 返回，下面的自动切分恢复分支永远不会执行。
	var apiResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Items []agendaRawItem `json:"items"`
		} `json:"data"`
	}
	parseErr := json.Unmarshal(resp.RawBody, &apiResp)

	if parseErr != nil {
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("获取日程视图失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
		}
		return nil, fmt.Errorf("解析响应失败: %w", parseErr)
	}
	if apiResp.Code != 0 {
		apiErr := fmt.Errorf("获取日程视图失败: code=%d, msg=%s", apiResp.Code, apiResp.Msg)
		switch apiResp.Code {
		case larkErrCalendarTimeRangeExceeded:
			mid := startTime + span/2
			if mid <= startTime {
				return nil, apiErr
			}
			return fetchInstanceViewSplit(calendarID, startTime, mid, endTime, depth, userAccessToken)
		case larkErrCalendarTooManyInstances:
			if span <= minSplitWindowSeconds {
				return nil, apiErr
			}
			mid := startTime + span/2
			return fetchInstanceViewSplit(calendarID, startTime, mid, endTime, depth, userAccessToken)
		default:
			return nil, apiErr
		}
	}
	// 业务码为 0 但 HTTP 非 200：不可当成功，fail-closed
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取日程视图失败: HTTP %d, body: %s", resp.StatusCode, string(resp.RawBody))
	}
	return apiResp.Data.Items, nil
}

func fetchInstanceViewSplit(calendarID string, startTime, mid, endTime int64, depth int, userAccessToken string) ([]agendaRawItem, error) {
	left, err := fetchInstanceViewRange(calendarID, startTime, mid, depth+1, userAccessToken)
	if err != nil {
		return nil, err
	}
	right, err := fetchInstanceViewRange(calendarID, mid+1, endTime, depth+1, userAccessToken)
	if err != nil {
		return nil, err
	}
	return append(left, right...), nil
}

func agendaTimeKey(info *agendaTimeInfo) string {
	if info == nil {
		return ""
	}
	if info.Timestamp != "" {
		return info.Timestamp
	}
	return info.Date
}

func agendaStartUnix(info *agendaTimeInfo) int64 {
	if info == nil {
		return 0
	}
	if info.Timestamp != "" {
		n, err := strconv.ParseInt(info.Timestamp, 10, 64)
		if err == nil {
			return n
		}
	}
	if info.Date != "" {
		if t, err := time.ParseInLocation("2006-01-02", info.Date, time.UTC); err == nil {
			return t.Unix()
		}
	}
	return 0
}

func exclusiveAllDayEndDate(dateStr string) string {
	t, err := time.ParseInLocation("2006-01-02", dateStr, time.UTC)
	if err != nil {
		return dateStr
	}
	return t.Add(-1 * time.Second).Format("2006-01-02")
}

func convertAgendaRawItem(item agendaRawItem) *AgendaEvent {
	event := &AgendaEvent{
		EventID:        item.EventID,
		Summary:        item.Summary,
		Status:         item.Status,
		FreeBusyStatus: item.FreeBusyStatus,
		SelfRSVP:       item.SelfRSVP,
	}
	if item.StartTime != nil {
		tz := item.StartTime.Timezone
		if item.StartTime.Timestamp != "" {
			event.StartTime = timestampToRFC3339(item.StartTime.Timestamp, tz)
		} else if item.StartTime.Date != "" {
			event.StartTime = item.StartTime.Date
			event.IsAllDay = true
		}
	}
	if item.EndTime != nil {
		tz := item.EndTime.Timezone
		if item.EndTime.Timestamp != "" {
			event.EndTime = timestampToRFC3339(item.EndTime.Timestamp, tz)
		} else if item.EndTime.Date != "" {
			event.EndTime = exclusiveAllDayEndDate(item.EndTime.Date)
		}
	}
	return event
}

func dedupeAndSortAgendaItems(items []agendaRawItem) []agendaRawItem {
	seen := make(map[string]bool, len(items))
	out := make([]agendaRawItem, 0, len(items))
	for _, item := range items {
		key := item.EventID + "|" + agendaTimeKey(item.StartTime) + "|" + agendaTimeKey(item.EndTime)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return agendaStartUnix(out[i].StartTime) < agendaStartUnix(out[j].StartTime)
	})
	return out
}

// ListCalendarAgenda 获取日程实例视图（展开重复日程为独立实例）。
// instance_view 无服务端分页：pageSize/pageToken 被忽略，窗口超过 40 天或命中
// 193104 时由客户端拆分、去重；全天结束日按排他日期转为含当日。
func ListCalendarAgenda(calendarID string, startTime, endTime int64, pageSize int, pageToken string, userAccessToken string) ([]*AgendaEvent, string, bool, error) {
	_ = pageSize
	_ = pageToken
	if strings.TrimSpace(calendarID) == "" {
		calendarID = "primary"
	}

	items, err := fetchInstanceViewRange(calendarID, startTime, endTime, 0, userAccessToken)
	if err != nil {
		return nil, "", false, err
	}
	items = dedupeAndSortAgendaItems(items)

	events := make([]*AgendaEvent, 0, len(items))
	for _, item := range items {
		if item.Status == "cancelled" {
			continue
		}
		events = append(events, convertAgendaRawItem(item))
	}
	// instance_view 无伪分页：切分在客户端完成，对外始终一页。
	return events, "", false, nil
}

// 辅助函数：转换日程对象
func convertEvent(event *larkcalendar.CalendarEvent) *CalendarEvent {
	if event == nil {
		return nil
	}

	result := &CalendarEvent{
		EventID:     StringVal(event.EventId),
		OrganizerID: StringVal(event.OrganizerCalendarId),
		Summary:     StringVal(event.Summary),
		Description: StringVal(event.Description),
		Status:      StringVal(event.Status),
		Visibility:  StringVal(event.Visibility),
		RecurringID: StringVal(event.RecurringEventId),
		Recurrence:  StringVal(event.Recurrence),
		IsException: BoolVal(event.IsException),
		AppLink:     StringVal(event.AppLink),
		Color:       IntVal(event.Color),
	}

	// 时区
	tz := ""
	if event.StartTime != nil && event.StartTime.Timezone != nil {
		tz = *event.StartTime.Timezone
		result.TimeZone = tz
	}
	result.rawStart = event.StartTime
	result.rawEnd = event.EndTime

	// 时间转换：普通日程是秒级 timestamp；全天日程只有 date（YYYY-MM-DD，时区固定 UTC），
	// 且 end.date 是**排他**的次日。此前只处理 timestamp，全天日程（如请假）起止输出为空。
	if event.StartTime != nil {
		if ts := StringVal(event.StartTime.Timestamp); ts != "" {
			result.StartTime = timestampToRFC3339(ts, tz)
		} else if date := StringVal(event.StartTime.Date); date != "" {
			result.StartTime = date
			result.IsAllDay = true
		}
	}
	if event.EndTime != nil {
		if ts := StringVal(event.EndTime.Timestamp); ts != "" {
			result.EndTime = timestampToRFC3339(ts, tz)
		} else if date := StringVal(event.EndTime.Date); date != "" {
			result.EndTime = exclusiveAllDayEndDate(date)
			result.IsAllDay = true
		}
	}
	if event.Location != nil && event.Location.Name != nil {
		result.Location = *event.Location.Name
	}
	if event.CreateTime != nil {
		result.CreateTime = timestampToRFC3339(*event.CreateTime, tz)
	}

	result.FreeBusyStatus = StringVal(event.FreeBusyStatus)
	result.AttendeeAbility = StringVal(event.AttendeeAbility)
	if v := event.Vchat; v != nil {
		vc := &EventVchat{
			VcType:      StringVal(v.VcType),
			IconType:    StringVal(v.IconType),
			Description: StringVal(v.Description),
			MeetingURL:  StringVal(v.MeetingUrl),
		}
		if *vc != (EventVchat{}) {
			result.Vchat = vc
		}
	}
	for _, r := range event.Reminders {
		if r != nil && r.Minutes != nil {
			result.Reminders = append(result.Reminders, EventReminder{Minutes: *r.Minutes})
		}
	}
	if o := event.EventOrganizer; o != nil && (StringVal(o.UserId) != "" || StringVal(o.DisplayName) != "") {
		result.EventOrganizer = &EventOrganizer{UserID: StringVal(o.UserId), DisplayName: StringVal(o.DisplayName)}
	}

	return result
}

// calendarEventWire 是 GET /events/{id} 的 data.event：SDK 结构缺 self_rsvp_status，额外补一个字段。
type calendarEventWire struct {
	larkcalendar.CalendarEvent
	SelfRsvpStatus string `json:"self_rsvp_status,omitempty"`
}

func calendarEventPath(calendarID, eventID string) string {
	return fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/%s",
		url.PathEscape(calendarID), url.PathEscape(eventID))
}
