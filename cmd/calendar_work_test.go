package cmd

import (
	"net/http"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

// TestFreebusyDefaultsToCurrentUser 不传 --user-id 时默认查当前登录用户（回归：此前必报 190002）
func TestFreebusyDefaultsToCurrentUser(t *testing.T) {
	rec := setupWorkCmdTest(t, "u-test", func(w http.ResponseWriter, r *http.Request, body string) {
		switch r.URL.Path {
		case "/open-apis/authen/v1/user_info":
			_, _ = w.Write([]byte(`{"code":0,"data":{"open_id":"ou_me","name":"me"}}`))
		case "/open-apis/calendar/v4/freebusy/batch":
			_, _ = w.Write([]byte(`{"code":0,"data":{"freebusy_lists":[{"user_id":"ou_me","freebusy_items":[
				{"start_time":"2026-10-09T16:00:00+08:00","end_time":"2026-10-09T17:00:00+08:00"},
				{"start_time":"2026-10-09T15:30:00+08:00","end_time":"2026-10-09T16:00:00+08:00"}]}]}}`))
		default:
			http.Error(w, `{"code":404}`, http.StatusNotFound)
		}
	})
	out, err := runWorkCmd(t, calendarFreebusyCmd, nil, map[string]string{
		"start": "2026-10-09", "end": "2026-10-09", "output": "json",
	})
	if err != nil {
		t.Fatalf("freebusy: %v", err)
	}
	if !strings.Contains(out, `"start_time": "2026-10-09T15:30:00+08:00"`) || !strings.Contains(out, `"end_time": "2026-10-09T17:00:00+08:00"`) ||
		strings.Count(out, "start_time") != 1 {
		t.Fatalf("应输出合并后的单个区间（兼容旧数组形状）: %s", out)
	}
	var batch workReq
	for _, q := range rec.apiReqs() {
		if q.Path == "/open-apis/calendar/v4/freebusy/batch" {
			batch = q
		}
	}
	if !strings.Contains(batch.Body, `"user_ids":["ou_me"]`) {
		t.Fatalf("应默认查询当前用户 ou_me: %+v", batch)
	}
}

// TestFreebusyBotRequiresUserID Bot 身份没有"本人"，不传 --user-id 时本地报用法错误、不发请求
func TestFreebusyBotRequiresUserID(t *testing.T) {
	rec := setupWorkCmdTest(t, "", nil)
	_, err := runWorkCmd(t, calendarFreebusyCmd, nil, map[string]string{"start": "2026-10-09"})
	if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("期望用法错误，得到 %v", err)
	}
	if len(rec.apiReqs()) != 0 {
		t.Fatalf("不应发出业务请求: %+v", rec.apiReqs())
	}
}

// TestUpdateEventRequiresPairedTimes 实测只传一端时服务端返回成功但不生效 → 本地拒绝
func TestUpdateEventRequiresPairedTimes(t *testing.T) {
	rec := setupWorkCmdTest(t, "u-test", nil)
	_, err := runWorkCmd(t, updateEventCmd, []string{"evt_0"}, map[string]string{"start": "2026-10-09T10:00:00+08:00"})
	if err == nil || !clierr.HasKind(err, clierr.KindUsage) || !strings.Contains(err.Error(), "--start 与 --end 必须同时指定") {
		t.Fatalf("期望成对校验错误，得到 %v", err)
	}
	if len(rec.all()) != 0 {
		t.Fatalf("校验失败不应联网: %+v", rec.all())
	}
}

// TestCalendarWriteDefaultsToUserIdentity 写命令默认 auto：已登录时用 User Token（此前默认 Bot）
func TestCalendarWriteDefaultsToUserIdentity(t *testing.T) {
	rec := setupWorkCmdTest(t, "u-test", func(w http.ResponseWriter, r *http.Request, body string) {
		switch {
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"code":0,"data":{"event":{"event_id":"evt_0","summary":"x","start_time":{"timestamp":"1794783600"},"end_time":{"timestamp":"1794785400"}}}}`))
		case r.Method == http.MethodDelete:
			_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
		default:
			http.Error(w, `{"code":404}`, http.StatusNotFound)
		}
	})
	if _, err := runWorkCmd(t, deleteEventCmd, []string{"evt_0"}, nil); err != nil {
		t.Fatalf("delete-event: %v", err)
	}
	reqs := rec.apiReqs()
	if len(reqs) != 2 || reqs[1].Method != http.MethodDelete || reqs[1].Auth != "Bearer u-test" {
		t.Fatalf("默认应以 User Token 读取后删除: %+v", reqs)
	}
	if !strings.Contains(reqs[1].Path, "/calendars/primary/events/evt_0") {
		t.Fatalf("只传 event_id 时应使用 primary: %s", reqs[1].Path)
	}

	// --as bot 显式走 Tenant Token
	rec2 := setupWorkCmdTest(t, "u-test", func(w http.ResponseWriter, r *http.Request, body string) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"code":0,"data":{"event":{"event_id":"evt_0"}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
	})
	if _, err := runWorkCmd(t, deleteEventCmd, []string{"evt_0"}, map[string]string{"as": "bot"}); err != nil {
		t.Fatalf("delete-event --as bot: %v", err)
	}
	for _, q := range rec2.apiReqs() {
		if q.Auth != "Bearer t-bot" {
			t.Fatalf("--as bot 应使用 Tenant Token: %+v", q)
		}
	}
}

// TestDeleteEventApplyToAllNeedsConfirmation --apply-to all 是扩大的破坏性操作：非交互且无 --yes → exit 10，不删除
func TestDeleteEventApplyToAllNeedsConfirmation(t *testing.T) {
	rec := setupWorkCmdTest(t, "u-test", func(w http.ResponseWriter, r *http.Request, body string) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"code":0,"data":{"event":{"event_id":"u_0","recurrence":"FREQ=DAILY","start_time":{"timestamp":"1794783600"},"end_time":{"timestamp":"1794785400"}}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
	})
	_, err := runWorkCmd(t, deleteEventCmd, []string{"u_0"}, map[string]string{"apply-to": "all"})
	if err == nil || !clierr.HasKind(err, clierr.KindConfirmationRequired) {
		t.Fatalf("期望需要确认，得到 %v", err)
	}
	for _, q := range rec.apiReqs() {
		if q.Method == http.MethodDelete {
			t.Fatalf("未确认不应删除: %+v", q)
		}
	}
}

// TestDeleteEventDryRunNoNetwork dry-run 不联网、不解析身份
func TestDeleteEventDryRunNoNetwork(t *testing.T) {
	rec := setupWorkCmdTest(t, "u-test", nil)
	out, err := runWorkCmd(t, deleteEventCmd, []string{"CAL", "u_1794870000"}, map[string]string{"apply-to": "this-and-following", "dry-run": "true"})
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if !strings.Contains(out, `"dry_run": true`) || !strings.Contains(out, "UNTIL") {
		t.Fatalf("dry-run 输出不完整: %s", out)
	}
	if len(rec.all()) != 0 {
		t.Fatalf("dry-run 不应联网: %+v", rec.all())
	}
}

// TestEventReplyRequiresUserToken event-reply 与 rsvp 一致必需 User Token（此前默认 Bot 必然答复失败）
func TestEventReplyRequiresUserToken(t *testing.T) {
	rec := setupWorkCmdTest(t, "", nil)
	_, err := runWorkCmd(t, calendarEventReplyCmd, []string{"CAL", "evt_0"}, map[string]string{"status": "accept"})
	if err == nil || !strings.Contains(err.Error(), "需要 User Access Token") {
		t.Fatalf("期望要求 User Token，得到 %v", err)
	}
	if len(rec.apiReqs()) != 0 {
		t.Fatalf("不应以 Bot 身份发请求: %+v", rec.apiReqs())
	}
}

// TestCreateEventAttendeeRollback 参与人添加失败时删除刚创建的日程
func TestCreateEventAttendeeRollback(t *testing.T) {
	rec := setupWorkCmdTest(t, "", func(w http.ResponseWriter, r *http.Request, body string) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/events"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"event":{"event_id":"new_0","summary":"x"}}}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/attendees"):
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":99992375,"msg":"invalid room"}`))
		case r.Method == http.MethodDelete:
			_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
		default:
			http.Error(w, `{"code":404}`, http.StatusNotFound)
		}
	})
	_, err := runWorkCmd(t, createEventCmd, nil, map[string]string{
		"summary": "x", "start": "2026-11-30T07:00:00+08:00", "end": "2026-11-30T07:30:00+08:00", "attendee-ids": "omm_bad",
	})
	if err == nil || !strings.Contains(err.Error(), "回滚成功") {
		t.Fatalf("期望回滚提示，得到 %v", err)
	}
	var deleted bool
	for _, q := range rec.apiReqs() {
		if q.Method == http.MethodDelete && strings.HasSuffix(q.Path, "/calendars/primary/events/new_0") && strings.Contains(q.Query, "need_notification=false") {
			deleted = true
		}
	}
	if !deleted {
		t.Fatalf("未回滚删除日程: %+v", rec.apiReqs())
	}
}
