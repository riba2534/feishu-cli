package client

import (
	"net/http"
	"strings"
	"testing"
)

// TestCompleteTaskSkipsWhenAlreadyCompleted 已完成任务不再 PATCH（回归：此前每次都改写完成时间）
func TestCompleteTaskSkipsWhenAlreadyCompleted(t *testing.T) {
	got := captureAPI(t, func(w http.ResponseWriter, r *http.Request, cap *capturedHTTPRequest) {
		writeJSON(w, http.StatusOK, `{"code":0,"data":{"task":{"guid":"t1","summary":"x","completed_at":"1790000000000"}}}`)
	})
	info, err := CompleteTask("t1", testUserToken)
	if err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}
	if !info.AlreadyCompleted || info.CompletedAt == "" {
		t.Fatalf("应标记 already_completed: %+v", info)
	}
	for _, r := range got() {
		if r.Method == http.MethodPatch {
			t.Fatalf("已完成任务不应 PATCH: %+v", r)
		}
	}
}

// TestCompleteTaskPatchesWhenOpen 未完成任务正常 PATCH completed_at
func TestCompleteTaskPatchesWhenOpen(t *testing.T) {
	got := captureAPI(t, func(w http.ResponseWriter, r *http.Request, cap *capturedHTTPRequest) {
		if r.Method == http.MethodGet {
			writeJSON(w, http.StatusOK, `{"code":0,"data":{"task":{"guid":"t1","summary":"x","completed_at":"0"}}}`)
			return
		}
		writeJSON(w, http.StatusOK, `{"code":0,"data":{"task":{"guid":"t1","summary":"x","completed_at":"1790000000000"}}}`)
	})
	info, err := CompleteTask("t1", testUserToken)
	if err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}
	reqs := got()
	if len(reqs) != 2 || reqs[1].Method != http.MethodPatch || info.AlreadyCompleted {
		t.Fatalf("应先 GET 再 PATCH: %+v / %+v", reqs, info)
	}
}

func TestParseTaskGUID(t *testing.T) {
	cases := map[string]string{
		"a66b960c-a57d-4189-859d-3e56d722c72e":                                          "a66b960c-a57d-4189-859d-3e56d722c72e",
		"https://applink.feishu.cn/client/todo/detail?guid=abc-123&suite_entity_num=t1": "abc-123",
	}
	for in, want := range cases {
		got, err := ParseTaskGUID(in)
		if err != nil || got != want {
			t.Fatalf("ParseTaskGUID(%q)=%q,%v want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "t39858587", "https://applink.feishu.cn/client/todo/detail?x=1"} {
		if _, err := ParseTaskGUID(bad); err == nil {
			t.Fatalf("ParseTaskGUID(%q) 应报错", bad)
		}
	}
}

// TestTaskAllDayDueUsesUTCDate 全天截止服务端用 UTC 零点表示日期，按 UTC 取日期（不能按本地零点，否则西半球差一天）
func TestTaskAllDayDueUsesUTCDate(t *testing.T) {
	captureAPI(t, func(w http.ResponseWriter, r *http.Request, cap *capturedHTTPRequest) {
		writeJSON(w, http.StatusOK, `{"code":0,"data":{"task":{"guid":"t1","summary":"x","due":{"timestamp":"1796428800000","is_all_day":true},
			"reminders":[{"id":"r1","relative_fire_minute":30}],"members":[{"id":"ou_a","type":"user","role":"assignee"}],"parent_task_guid":"p1"}}}`)
	})
	info, err := GetTask("t1", testUserToken)
	if err != nil {
		t.Fatal(err)
	}
	if info.DueTime != "2026-12-05" || !info.DueIsAllDay {
		t.Fatalf("全天截止应为 2026-12-05: %+v", info)
	}
	if len(info.Reminders) != 1 || info.Reminders[0].ID != "r1" || info.Reminders[0].RelativeFireMinute != 30 ||
		len(info.Members) != 1 || info.ParentTaskGuid != "p1" {
		t.Fatalf("提醒/成员/父任务未输出: %+v", info)
	}
}

func TestTaskMoreRequests(t *testing.T) {
	got := captureAPI(t, func(w http.ResponseWriter, r *http.Request, cap *capturedHTTPRequest) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/list_related_task"):
			writeJSON(w, http.StatusOK, `{"code":0,"data":{"items":[{"guid":"t1","summary":"a","completed_at":"0"}],"has_more":true,"page_token":"1671246929172722"}}`)
		case strings.HasSuffix(r.URL.Path, "/set_ancestor_task"):
			writeJSON(w, http.StatusOK, `{"code":0,"data":{}}`)
		case r.URL.Path == "/open-apis/task/v2/sections":
			writeJSON(w, http.StatusOK, `{"code":0,"data":{"items":[{"guid":"s1","name":"工作","is_default":false}],"has_more":false}}`)
		default:
			writeJSON(w, http.StatusNotFound, `{"code":404,"msg":"x"}`)
		}
	})
	page, err := ListRelatedTasks(false, "", 50, testUserToken)
	if err != nil || len(page.Tasks) != 1 || page.PageToken != "1671246929172722" || !page.HasMore {
		t.Fatalf("related: %+v %v", page, err)
	}
	if err := SetTaskAncestor("c1", "", testUserToken); err != nil {
		t.Fatal(err)
	}
	secs, _, _, err := ListTaskSections("my_tasks", "", "", 50, testUserToken)
	if err != nil || len(secs) != 1 || secs[0].Name != "工作" {
		t.Fatalf("sections: %+v %v", secs, err)
	}
	reqs := got()
	if reqs[0].Query.Get("completed") != "false" || reqs[0].Query.Get("user_id_type") != "open_id" {
		t.Fatalf("related 参数错误: %v", reqs[0].Query)
	}
	if string(reqs[1].Body) != "{}" {
		t.Fatalf("解除父任务应发送空 body: %s", reqs[1].Body)
	}
	if reqs[2].Query.Get("resource_type") != "my_tasks" {
		t.Fatalf("sections 参数错误: %v", reqs[2].Query)
	}
}
