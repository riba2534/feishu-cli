package client

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestGetEventAllDayDates 全天日程只有 date（无 timestamp）：起止必须输出日期且结束日换成包含端
// （回归：此前 convertEvent 只处理 timestamp，休假等全天日程 start_time/end_time 为空）。
func TestGetEventAllDayDates(t *testing.T) {
	captureAPI(t, func(w http.ResponseWriter, r *http.Request, cap *capturedHTTPRequest) {
		writeJSON(w, http.StatusOK, `{"code":0,"msg":"ok","data":{"event":{
			"event_id":"evt_0","summary":"休假",
			"start_time":{"date":"2026-04-17","timezone":"UTC"},
			"end_time":{"date":"2026-05-01","timezone":"UTC"},
			"self_rsvp_status":"accept","free_busy_status":"busy",
			"vchat":{"vc_type":"vc","meeting_url":"https://vc.example.com/j/1"},
			"reminders":[{"minutes":5}]
		}}}`)
	})
	ev, err := GetEvent("primary", "evt_0", testUserToken)
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	if ev.StartTime != "2026-04-17" || ev.EndTime != "2026-04-30" || !ev.IsAllDay {
		t.Fatalf("全天日程起止错误: start=%q end=%q allDay=%v", ev.StartTime, ev.EndTime, ev.IsAllDay)
	}
	if ev.SelfRSVPStatus != "accept" || ev.FreeBusyStatus != "busy" {
		t.Fatalf("self_rsvp/free_busy 未解析: %+v", ev)
	}
	if ev.Vchat == nil || ev.Vchat.MeetingURL != "https://vc.example.com/j/1" {
		t.Fatalf("vchat.meeting_url 未解析: %+v", ev.Vchat)
	}
	if len(ev.Reminders) != 1 || ev.Reminders[0].Minutes != 5 {
		t.Fatalf("reminders 未解析: %+v", ev.Reminders)
	}
}

// TestListEventsAllDay list-events 走同一个 convertEvent，全天日程同样不能为空
func TestListEventsAllDay(t *testing.T) {
	captureAPI(t, func(w http.ResponseWriter, r *http.Request, cap *capturedHTTPRequest) {
		writeJSON(w, http.StatusOK, `{"code":0,"msg":"ok","data":{"items":[
			{"event_id":"a_0","summary":"全天","start_time":{"date":"2026-04-17"},"end_time":{"date":"2026-04-18"}},
			{"event_id":"b_0","summary":"普通","start_time":{"timestamp":"1776384000","timezone":"Asia/Shanghai"},"end_time":{"timestamp":"1776387600","timezone":"Asia/Shanghai"}}
		]}}`)
	})
	events, _, _, err := ListEvents(&ListEventsParams{CalendarID: "primary"}, testUserToken)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if events[0].StartTime != "2026-04-17" || events[0].EndTime != "2026-04-17" || !events[0].IsAllDay {
		t.Fatalf("单日全天日程应为 2026-04-17~2026-04-17: %+v", events[0])
	}
	if events[1].IsAllDay || events[1].StartTime != "2026-04-17T08:00:00+08:00" {
		t.Fatalf("普通日程转换错误: %+v", events[1])
	}
}

// TestGetEventBusinessErrorOn400 业务错误随 HTTP 400 下发时按业务码返回 APIError
func TestGetEventBusinessErrorOn400(t *testing.T) {
	captureAPI(t, func(w http.ResponseWriter, r *http.Request, cap *capturedHTTPRequest) {
		writeJSON(w, http.StatusBadRequest, `{"code":193001,"msg":"event not found"}`)
	})
	_, err := GetEvent("primary", "x_0", testUserToken)
	if apiErr, ok := AsAPIError(err); !ok || apiErr.Code != 193001 {
		t.Fatalf("期望 APIError code=193001，得到 %v", err)
	}
}

// TestMergeFreebusyIntervals 忙闲区间排序并合并重叠/相邻区间（实测服务端返回乱序且相邻不合并）
func TestMergeFreebusyIntervals(t *testing.T) {
	items := []*FreebusyRawItem{
		{StartTime: "2026-10-09T10:10:00+08:00", EndTime: "2026-10-09T10:30:00+08:00"},
		{StartTime: "2026-10-09T16:00:00+08:00", EndTime: "2026-10-09T17:00:00+08:00"},
		{StartTime: "2026-10-09T11:00:00+08:00", EndTime: "2026-10-09T12:00:00+08:00"},
		{StartTime: "2026-10-09T15:30:00+08:00", EndTime: "2026-10-09T16:00:00+08:00"},
		{StartTime: "2026-10-09T11:30:00+08:00", EndTime: "2026-10-09T11:45:00+08:00"},
		{StartTime: "bad", EndTime: "2026-10-09T11:45:00+08:00"},
	}
	got := MergeFreebusyIntervals(items)
	want := [][2]string{
		{"2026-10-09T10:10:00+08:00", "2026-10-09T10:30:00+08:00"},
		{"2026-10-09T11:00:00+08:00", "2026-10-09T12:00:00+08:00"},
		{"2026-10-09T15:30:00+08:00", "2026-10-09T17:00:00+08:00"},
	}
	if len(got) != len(want) {
		t.Fatalf("合并后区间数 %d，期望 %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].StartTime != w[0] || got[i].EndTime != w[1] {
			t.Fatalf("区间 %d = %s~%s，期望 %s~%s", i, got[i].StartTime, got[i].EndTime, w[0], w[1])
		}
	}
}

func TestComputeFreeSlotsCommon(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	winStart := time.Date(2026, 10, 9, 9, 0, 0, 0, loc)
	winEnd := time.Date(2026, 10, 9, 18, 0, 0, 0, loc)
	a := []*FreebusyInfo{{StartTime: "2026-10-09T10:00:00+08:00", EndTime: "2026-10-09T11:00:00+08:00"}}
	b := []*FreebusyInfo{{StartTime: "2026-10-09T10:30:00+08:00", EndTime: "2026-10-09T12:00:00+08:00"},
		{StartTime: "2026-10-09T17:50:00+08:00", EndTime: "2026-10-09T19:00:00+08:00"}}
	got := ComputeFreeSlots([][]*FreebusyInfo{a, b}, winStart, winEnd, 30*time.Minute)
	if len(got) != 2 || got[0].StartTime != "2026-10-09T09:00:00+08:00" || got[0].EndTime != "2026-10-09T10:00:00+08:00" ||
		got[1].StartTime != "2026-10-09T12:00:00+08:00" || got[1].EndTime != "2026-10-09T17:50:00+08:00" {
		t.Fatalf("共同空闲计算错误: %+v", got)
	}
}

// TestListFreebusyBatchRequest freebusy 走 batch，user_ids 透传、结果按开始时间排序
func TestListFreebusyBatchRequest(t *testing.T) {
	got := captureAPI(t, func(w http.ResponseWriter, r *http.Request, cap *capturedHTTPRequest) {
		writeJSON(w, http.StatusOK, `{"code":0,"msg":"ok","data":{"freebusy_lists":[{"user_id":"ou_a","freebusy_items":[
			{"start_time":"2026-10-09T16:00:00+08:00","end_time":"2026-10-09T17:00:00+08:00","rsvp_status":"accept"},
			{"start_time":"2026-10-09T10:00:00+08:00","end_time":"2026-10-09T11:00:00+08:00","rsvp_status":"needs_action"}]}]}}`)
	})
	res, err := ListFreebusy("2026-10-09T00:00:00+08:00", "2026-10-09T23:59:59+08:00", "ou_a", testUserToken)
	if err != nil {
		t.Fatalf("ListFreebusy: %v", err)
	}
	if len(res) != 2 || res[0].StartTime != "2026-10-09T10:00:00+08:00" {
		t.Fatalf("结果未排序: %+v", res)
	}
	reqs := got()
	if len(reqs) != 1 || reqs[0].Path != "/open-apis/calendar/v4/freebusy/batch" || reqs[0].Query.Get("user_id_type") != "open_id" {
		t.Fatalf("请求错误: %+v", reqs)
	}
	var body map[string]any
	_ = json.Unmarshal(reqs[0].Body, &body)
	if ids, _ := body["user_ids"].([]any); len(ids) != 1 || ids[0] != "ou_a" {
		t.Fatalf("user_ids 未透传: %s", reqs[0].Body)
	}
}

func TestClassifyEventAndValidateApplyTo(t *testing.T) {
	cases := []struct {
		name string
		ev   *CalendarEvent
		kind RecurringKind
	}{
		{"normal", &CalendarEvent{EventID: "u_0"}, RecurringKindNormal},
		{"master", &CalendarEvent{EventID: "u_0", Recurrence: "FREQ=DAILY"}, RecurringKindMaster},
		{"instance", &CalendarEvent{EventID: "u_1794870000", RecurringID: "u_0"}, RecurringKindInstance},
		{"exception", &CalendarEvent{EventID: "u_1794870000", IsException: true}, RecurringKindException},
	}
	for _, c := range cases {
		if got := ClassifyEvent(c.ev); got != c.kind {
			t.Fatalf("%s: kind=%s want %s", c.name, got, c.kind)
		}
	}
	type vc struct {
		kind    RecurringKind
		req     string
		want    string
		wantErr bool
	}
	for _, v := range []vc{
		{RecurringKindNormal, "", ApplyToSingle, false},
		{RecurringKindNormal, ApplyToAll, "", true},
		{RecurringKindMaster, "", "", false},
		{RecurringKindMaster, ApplyToAll, ApplyToAll, false},
		{RecurringKindMaster, ApplyToSingle, "", true},
		{RecurringKindMaster, ApplyToThisAndFollowing, "", true},
		{RecurringKindInstance, ApplyToThisAndFollowing, ApplyToThisAndFollowing, false},
		{RecurringKindException, ApplyToThisAndFollowing, "", true},
		{RecurringKindException, ApplyToSingle, ApplyToSingle, false},
		{RecurringKindInstance, "bogus", "", true},
	} {
		got, err := ValidateApplyTo(v.kind, v.req)
		if (err != nil) != v.wantErr || got != v.want {
			t.Fatalf("ValidateApplyTo(%s,%q)=%q,%v want %q,err=%v", v.kind, v.req, got, err, v.want, v.wantErr)
		}
	}
}

func TestTruncateAndInheritRRule(t *testing.T) {
	until := time.Date(2026, 11, 19, 15, 59, 59, 0, time.UTC)
	if got := TruncateRecurrenceUntil("FREQ=DAILY;COUNT=6", until); got != "FREQ=DAILY;UNTIL=20261119T155959Z" {
		t.Fatalf("COUNT 应被 UNTIL 替换: %s", got)
	}
	if got := TruncateRecurrenceUntil("RRULE:FREQ=WEEKLY;UNTIL=20270101T000000Z;BYDAY=MO", until); got != "RRULE:FREQ=WEEKLY;UNTIL=20261119T155959Z;BYDAY=MO" {
		t.Fatalf("UNTIL 应原位替换: %s", got)
	}
	if got := InheritRRuleForFollowing("FREQ=DAILY;COUNT=4;INTERVAL=2"); got != "FREQ=DAILY;INTERVAL=2" {
		t.Fatalf("继承规则应去掉 COUNT: %s", got)
	}
	loc := time.FixedZone("CST", 8*3600)
	if got := PivotDayCutoff(1795129200, loc); !got.Equal(time.Date(2026, 11, 19, 23, 59, 59, 0, loc)) {
		t.Fatalf("截断点应为前一天 23:59:59: %s", got)
	}
}

// TestDeleteEventTreats193003AsSuccess 日程已删除（193003）视为成功，重复清理幂等
func TestDeleteEventTreats193003AsSuccess(t *testing.T) {
	got := captureAPI(t, func(w http.ResponseWriter, r *http.Request, cap *capturedHTTPRequest) {
		writeJSON(w, http.StatusBadRequest, `{"code":193003,"msg":"event is deleted"}`)
	})
	notify := false
	if err := DeleteEventWithOptions("primary", "u_0", DeleteEventOptions{NeedNotification: &notify, DeleteException: true}, testUserToken); err != nil {
		t.Fatalf("193003 应视为成功: %v", err)
	}
	q := got()[0].Query
	if q.Get("need_notification") != "false" || q.Get("delete_exception") != "true" {
		t.Fatalf("query 参数错误: %v", q)
	}
}

// TestShareLinkRetriesOnCalendarRateLimit share_info 实测常见 190010，需退避重试
func TestShareLinkRetriesOnCalendarRateLimit(t *testing.T) {
	orig := calendarRetrySleep
	calendarRetrySleep = func(time.Duration) bool { return true }
	t.Cleanup(func() { calendarRetrySleep = orig })
	calls := 0
	captureAPI(t, func(w http.ResponseWriter, r *http.Request, cap *capturedHTTPRequest) {
		calls++
		if calls == 1 {
			writeJSON(w, http.StatusBadRequest, `{"code":190010,"msg":"current operation rate limited"}`)
			return
		}
		writeJSON(w, http.StatusOK, `{"code":0,"msg":"ok","data":{"share_link":"https://www.feishu.cn/calendar/share?token=abc"}}`)
	})
	link, err := GetEventShareLink("primary", "u_0", testUserToken)
	if err != nil || link != "https://www.feishu.cn/calendar/share?token=abc" || calls != 2 {
		t.Fatalf("link=%q err=%v calls=%d", link, err, calls)
	}
}

// TestDeleteEventScopedAll --apply-to all：先销毁全部例外（含 cancelled 占位，不通知），最后删主体
func TestDeleteEventScopedAll(t *testing.T) {
	got := captureAPI(t, func(w http.ResponseWriter, r *http.Request, cap *capturedHTTPRequest) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/events/u_0"):
			writeJSON(w, http.StatusOK, `{"code":0,"data":{"event":{"event_id":"u_0","recurrence":"FREQ=DAILY;COUNT=5","start_time":{"timestamp":"1794783600","timezone":"Asia/Shanghai"},"end_time":{"timestamp":"1794785400","timezone":"Asia/Shanghai"}}}}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/instances"):
			writeJSON(w, http.StatusOK, `{"code":0,"data":{"items":[
				{"event_id":"u_1794783600","is_exception":false},
				{"event_id":"u_1794870000","is_exception":true,"status":"confirmed","start_time":{"timestamp":"1794870000"}},
				{"event_id":"u_1795042800","is_exception":true,"status":"cancelled"}],"has_more":false}}`)
		case r.Method == http.MethodDelete:
			writeJSON(w, http.StatusOK, `{"code":0,"data":{}}`)
		default:
			writeJSON(w, http.StatusNotFound, `{"code":404,"msg":"unexpected"}`)
		}
	})
	current := &CalendarEvent{EventID: "u_1794956400", RecurringID: "u_0"}
	res, err := DeleteEventScoped("primary", "u_1794956400", current, ApplyToAll, true, nil, testUserToken)
	if err != nil {
		t.Fatalf("DeleteEventScoped: %v", err)
	}
	if res.Exceptions == nil || res.Exceptions.Total != 2 || res.Exceptions.Succeeded != 2 || res.MasterEventID != "u_0" {
		t.Fatalf("结果错误: %+v %+v", res, res.Exceptions)
	}
	var deletes []string
	for _, r := range got() {
		if r.Method == http.MethodDelete {
			deletes = append(deletes, r.Path[strings.LastIndex(r.Path, "/")+1:]+"?"+r.Query.Encode())
		}
	}
	want := []string{
		"u_1794870000?delete_exception=true&need_notification=false",
		"u_1795042800?delete_exception=true&need_notification=false",
		"u_0?need_notification=true",
	}
	if strings.Join(deletes, ",") != strings.Join(want, ",") {
		t.Fatalf("删除顺序/参数错误:\n got  %v\n want %v", deletes, want)
	}
}

// TestUpdateThisAndFollowingConvertsCountToUntil 原规则带 COUNT 时，新序列用原序列最后一次换算 UNTIL，
// 不能像官方那样直接丢 COUNT 变成无限重复。
func TestUpdateThisAndFollowingConvertsCountToUntil(t *testing.T) {
	var created map[string]any
	var masterPatch map[string]any
	captureAPI(t, func(w http.ResponseWriter, r *http.Request, cap *capturedHTTPRequest) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/events/u_0"):
			writeJSON(w, http.StatusOK, `{"code":0,"data":{"event":{"event_id":"u_0","summary":"周会","recurrence":"FREQ=DAILY;COUNT=4","start_time":{"timestamp":"1795388400","timezone":"Asia/Shanghai"},"end_time":{"timestamp":"1795390200","timezone":"Asia/Shanghai"}}}}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/instances"):
			writeJSON(w, http.StatusOK, `{"code":0,"data":{"items":[
				{"event_id":"u_1795388400"},{"event_id":"u_1795474800"},{"event_id":"u_1795561200"},{"event_id":"u_1795647600"}],"has_more":false}}`)
		case r.Method == http.MethodPatch && strings.HasSuffix(r.URL.Path, "/events/u_0"):
			_ = json.Unmarshal(cap.Body, &masterPatch)
			writeJSON(w, http.StatusOK, `{"code":0,"data":{"event":{"event_id":"u_0"}}}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/events"):
			_ = json.Unmarshal(cap.Body, &created)
			writeJSON(w, http.StatusOK, `{"code":0,"data":{"event":{"event_id":"n_0","summary":"新周会"}}}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/attendees"):
			writeJSON(w, http.StatusOK, `{"code":0,"data":{"items":[],"has_more":false}}`)
		default:
			writeJSON(w, http.StatusNotFound, `{"code":404,"msg":"unexpected `+r.Method+` `+r.URL.Path+`"}`)
		}
	})
	pivot := &CalendarEvent{EventID: "u_1795561200", RecurringID: "u_0"}
	res, err := UpdateEventScoped("primary", "u_1795561200", pivot, ApplyToThisAndFollowing, UpdateEventParams{Summary: "新周会"}, nil, testUserToken)
	if err != nil {
		t.Fatalf("UpdateEventScoped: %v", err)
	}
	if masterPatch["recurrence"] != "FREQ=DAILY;UNTIL=20261124T155959Z" {
		t.Fatalf("原序列截断规则错误: %v", masterPatch["recurrence"])
	}
	if created["recurrence"] != "FREQ=DAILY;UNTIL=20261125T230000Z" {
		t.Fatalf("新序列应以原序列最后一次换算 UNTIL，得到 %v", created["recurrence"])
	}
	if created["summary"] != "新周会" || res.FollowEvent == nil || res.FollowEvent.EventID != "n_0" {
		t.Fatalf("新序列未带修改: %+v / %+v", created, res.FollowEvent)
	}
}

// TestRemoveEventAttendeesBody 按前缀拆 delete_ids，未知 ID 走 attendee_ids
func TestRemoveEventAttendeesBody(t *testing.T) {
	got := captureAPI(t, func(w http.ResponseWriter, r *http.Request, cap *capturedHTTPRequest) {
		writeJSON(w, http.StatusOK, `{"code":0,"data":{}}`)
	})
	refs, err := ParseAttendeeRefs([]string{"ou_a", "oc_b", "omm_c", "user@example.com", "user_123", "ou_a"}, true)
	if err != nil {
		t.Fatalf("ParseAttendeeRefs: %v", err)
	}
	if err := RemoveEventAttendees("primary", "u_0", refs, false, testUserToken); err != nil {
		t.Fatalf("RemoveEventAttendees: %v", err)
	}
	req := got()[0]
	if !strings.HasSuffix(req.Path, "/attendees/batch_delete") {
		t.Fatalf("path=%s", req.Path)
	}
	var body struct {
		DeleteIDs   []map[string]string `json:"delete_ids"`
		AttendeeIDs []string            `json:"attendee_ids"`
		Notify      bool                `json:"need_notification"`
	}
	_ = json.Unmarshal(req.Body, &body)
	if len(body.DeleteIDs) != 4 || body.DeleteIDs[2]["type"] != "resource" || body.DeleteIDs[2]["room_id"] != "omm_c" ||
		len(body.AttendeeIDs) != 1 || body.AttendeeIDs[0] != "user_123" {
		t.Fatalf("请求体错误: %s", req.Body)
	}
	if _, err := ParseAttendeeRefs([]string{"user_123"}, false); err == nil {
		t.Fatal("添加场景不应接受无前缀 ID")
	}
}

// TestListEventAttendeesDropsChatRSVP 群参与人的 rsvp_status 恒为 needs_action，无意义，不输出
func TestListEventAttendeesDropsChatRSVP(t *testing.T) {
	captureAPI(t, func(w http.ResponseWriter, r *http.Request, cap *capturedHTTPRequest) {
		writeJSON(w, http.StatusOK, `{"code":0,"data":{"items":[
			{"type":"chat","chat_id":"oc_1","rsvp_status":"needs_action"},
			{"type":"user","user_id":"ou_1","rsvp_status":"accept"}],"has_more":false}}`)
	})
	items, _, _, err := ListEventAttendees("primary", "u_0", 0, "", testUserToken)
	if err != nil {
		t.Fatalf("ListEventAttendees: %v", err)
	}
	if items[0].RsvpStatus != "" || items[1].RsvpStatus != "accept" {
		t.Fatalf("rsvp 处理错误: %+v %+v", items[0], items[1])
	}
}
