package client

import (
	"fmt"
	"strconv"
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
	IsException bool   `json:"is_exception,omitempty"`
	AppLink     string `json:"app_link,omitempty"`
	Color       int    `json:"color,omitempty"`
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

	resp, err := client.Calendar.Calendar.List(Context(), reqBuilder.Build(), larkcore.WithUserAccessToken(userAccessToken))
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

	req := larkcalendar.NewCreateCalendarEventReqBuilder().
		CalendarId(params.CalendarID).
		CalendarEvent(eventBuilder.Build()).
		Build()

	resp, err := client.Calendar.CalendarEvent.Create(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
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

// GetEvent 获取日程详情
func GetEvent(calendarID, eventID string, userAccessToken string) (*CalendarEvent, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	req := larkcalendar.NewGetCalendarEventReqBuilder().
		CalendarId(calendarID).
		EventId(eventID).
		Build()

	resp, err := client.Calendar.CalendarEvent.Get(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
	if err != nil {
		return nil, fmt.Errorf("获取日程详情失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("获取日程详情失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data == nil || resp.Data.Event == nil {
		return nil, fmt.Errorf("日程不存在")
	}

	return convertEvent(resp.Data.Event), nil
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

	resp, err := client.Calendar.CalendarEvent.List(Context(), reqBuilder.Build(), larkcore.WithUserAccessToken(userAccessToken))
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

	req := larkcalendar.NewPatchCalendarEventReqBuilder().
		CalendarId(params.CalendarID).
		EventId(params.EventID).
		CalendarEvent(eventBuilder.Build()).
		Build()

	resp, err := client.Calendar.CalendarEvent.Patch(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
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

// DeleteEvent 删除日程
func DeleteEvent(calendarID, eventID string, userAccessToken string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	req := larkcalendar.NewDeleteCalendarEventReqBuilder().
		CalendarId(calendarID).
		EventId(eventID).
		Build()

	resp, err := client.Calendar.CalendarEvent.Delete(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
	if err != nil {
		return fmt.Errorf("删除日程失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("删除日程失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// 辅助函数：将 RFC3339 时间格式转换为时间戳字符串
func parseTimeToTimestamp(timeStr string) (string, error) {
	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(t.Unix(), 10), nil
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

	resp, err := client.Calendar.Calendar.Get(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
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

	resp, err := client.Calendar.Calendar.Primary(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
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

// SearchEvents 搜索日程
func SearchEvents(calendarID, query string, startTime, endTime string, pageToken string, pageSize int, userAccessToken string) ([]*CalendarEvent, string, error) {
	client, err := GetClient()
	if err != nil {
		return nil, "", err
	}

	bodyBuilder := larkcalendar.NewSearchCalendarEventReqBodyBuilder().
		Query(query)

	filterBuilder := larkcalendar.NewEventSearchFilterBuilder()
	hasFilter := false

	if startTime != "" {
		startTs, err := parseTimeToTimestamp(startTime)
		if err != nil {
			return nil, "", fmt.Errorf("解析开始时间失败: %w", err)
		}
		startTimeInfo := larkcalendar.NewTimeInfoBuilder().Timestamp(startTs).Build()
		filterBuilder.StartTime(startTimeInfo)
		hasFilter = true
	}

	if endTime != "" {
		endTs, err := parseTimeToTimestamp(endTime)
		if err != nil {
			return nil, "", fmt.Errorf("解析结束时间失败: %w", err)
		}
		endTimeInfo := larkcalendar.NewTimeInfoBuilder().Timestamp(endTs).Build()
		filterBuilder.EndTime(endTimeInfo)
		hasFilter = true
	}

	if hasFilter {
		bodyBuilder.Filter(filterBuilder.Build())
	}

	reqBuilder := larkcalendar.NewSearchCalendarEventReqBuilder().
		CalendarId(calendarID).
		Body(bodyBuilder.Build())

	if pageSize > 0 {
		reqBuilder.PageSize(pageSize)
	}
	if pageToken != "" {
		reqBuilder.PageToken(pageToken)
	}

	resp, err := client.Calendar.CalendarEvent.Search(Context(), reqBuilder.Build(), larkcore.WithUserAccessToken(userAccessToken))
	if err != nil {
		return nil, "", fmt.Errorf("搜索日程失败: %w", err)
	}

	if !resp.Success() {
		return nil, "", fmt.Errorf("搜索日程失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	var events []*CalendarEvent
	if resp.Data != nil && resp.Data.Items != nil {
		for _, item := range resp.Data.Items {
			events = append(events, convertEvent(item))
		}
	}

	var nextPageToken string
	if resp.Data != nil {
		nextPageToken = StringVal(resp.Data.PageToken)
	}

	return events, nextPageToken, nil
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

	resp, err := client.Calendar.CalendarEventAttendee.Create(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
	if err != nil {
		return fmt.Errorf("添加日程参与人失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("添加日程参与人失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
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

	resp, err := client.Calendar.CalendarEventAttendee.List(Context(), reqBuilder.Build(), larkcore.WithUserAccessToken(userAccessToken))
	if err != nil {
		return nil, "", false, fmt.Errorf("获取日程参与人列表失败: %w", err)
	}

	if !resp.Success() {
		return nil, "", false, fmt.Errorf("获取日程参与人列表失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	var attendees []*EventAttendee
	if resp.Data != nil && resp.Data.Items != nil {
		for _, item := range resp.Data.Items {
			attendees = append(attendees, &EventAttendee{
				Type:            StringVal(item.Type),
				AttendeeID:      StringVal(item.AttendeeId),
				UserID:          StringVal(item.UserId),
				ChatID:          StringVal(item.ChatId),
				RoomID:          StringVal(item.RoomId),
				ThirdPartyEmail: StringVal(item.ThirdPartyEmail),
				DisplayName:     StringVal(item.DisplayName),
				RsvpStatus:      StringVal(item.RsvpStatus),
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

// ListFreebusy 查询忙闲信息
func ListFreebusy(startTime, endTime string, userID string, userAccessToken string) ([]*FreebusyInfo, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	bodyBuilder := larkcalendar.NewListFreebusyReqBodyBuilder().
		TimeMin(startTime).
		TimeMax(endTime)

	if userID != "" {
		bodyBuilder.UserId(userID)
	}

	req := larkcalendar.NewListFreebusyReqBuilder().
		Body(bodyBuilder.Build()).
		Build()

	resp, err := client.Calendar.Freebusy.List(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
	if err != nil {
		return nil, fmt.Errorf("查询忙闲信息失败: %w", err)
	}

	if !resp.Success() {
		return nil, fmt.Errorf("查询忙闲信息失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	var result []*FreebusyInfo
	if resp.Data != nil && resp.Data.FreebusyList != nil {
		for _, item := range resp.Data.FreebusyList {
			result = append(result, &FreebusyInfo{
				StartTime: StringVal(item.StartTime),
				EndTime:   StringVal(item.EndTime),
			})
		}
	}

	return result, nil
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

	resp, err := client.Calendar.CalendarEvent.Reply(Context(), req, larkcore.WithUserAccessToken(userAccessToken))
	if err != nil {
		return fmt.Errorf("回复日程失败: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("回复日程失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil
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

	// 时间转换
	if event.StartTime != nil && event.StartTime.Timestamp != nil {
		result.StartTime = timestampToRFC3339(*event.StartTime.Timestamp, tz)
	}
	if event.EndTime != nil && event.EndTime.Timestamp != nil {
		result.EndTime = timestampToRFC3339(*event.EndTime.Timestamp, tz)
	}
	if event.Location != nil && event.Location.Name != nil {
		result.Location = *event.Location.Name
	}
	if event.CreateTime != nil {
		result.CreateTime = timestampToRFC3339(*event.CreateTime, tz)
	}

	return result
}
