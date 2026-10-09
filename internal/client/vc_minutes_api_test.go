package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

const vcTestUserToken = "u-vc-test"

// TestVCMinutesClientParsesBizCodeOnHTTPError 锁住"先解析业务码再判 HTTP 状态"：
// 飞书 VC / 妙记接口的业务错误常随 HTTP 4xx 下发（实测 2091005 随 403、121002 随 400）。
// 旧实现先判 StatusCode!=200 返回 "HTTP 403, body: ..."，命令层按业务码给的专门提示走不到。
func TestVCMinutesClientParsesBizCodeOnHTTPError(t *testing.T) {
	cases := []struct {
		name   string
		status int
		code   int
		action string
		call   func() error
	}{
		{"GetMinute", 403, 2091005, "获取妙记信息", func() error { _, err := GetMinute("obcnxxxx", vcTestUserToken); return err }},
		{"GetMinuteArtifacts", 403, 2091005, "获取妙记 AI 产物", func() error { _, err := GetMinuteArtifacts("obcnxxxx", vcTestUserToken); return err }},
		{"GetMinuteTranscript", 403, 2091005, "获取妙记文字稿", func() error { _, err := GetMinuteTranscript("obcnxxxx", vcTestUserToken); return err }},
		{"GetMinuteMediaURL", 403, 2091005, "获取妙记媒体下载链接", func() error { _, err := GetMinuteMediaURL("obcnxxxx", vcTestUserToken); return err }},
		{"SearchMinutes", 400, 99992402, "搜索妙记", func() error {
			_, err := SearchMinutes(SearchMinutesReq{Query: "q"}, vcTestUserToken)
			return err
		}},
		{"ApplyMinutePermission", 400, 2091006, "申请妙记权限", func() error { _, err := ApplyMinutePermission("obcnxxxx", "view", vcTestUserToken); return err }},
		{"SearchMeetings", 400, 99992402, "搜索会议", func() error { _, err := SearchMeetings(SearchMeetingsReq{Query: "q"}, vcTestUserToken); return err }},
		{"GetMeeting", 403, 121005, "获取会议详情", func() error { _, err := GetMeeting("6911188411932033028", vcTestUserToken); return err }},
		{"GetMeetingRecording", 400, 121004, "获取会议录制", func() error { _, err := GetMeetingRecording("6911188411932033028", vcTestUserToken); return err }},
		{"ListMeetingsByNo", 400, 121001, "按会议号查询会议", func() error { _, err := ListMeetingsByNo("123456789", 1, 2, vcTestUserToken); return err }},
		{"VCBotMeetingEvents", 403, 120003, "查询机器人会议事件", func() error {
			_, err := VCBotMeetingEvents(VCBotEventsReq{MeetingID: "6911188411932033028"}, vcTestUserToken)
			return err
		}},
		{"GetMeetingNote", 403, 121005, "获取会议纪要", func() error { _, err := GetMeetingNote("7000000000000000001", vcTestUserToken); return err }},
		{"GetUnifiedNoteTranscript", 400, 121002, "获取统一逐字稿", func() error {
			_, err := GetUnifiedNoteTranscript("7000000000000000001", map[string]string{"format": "markdown"}, vcTestUserToken)
			return err
		}},
		{"ListActiveMeetings", 403, 120003, "查询进行中的会议", func() error { _, err := ListActiveMeetings("", vcTestUserToken); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprintf(w, `{"code":%d,"msg":"biz error","error":{"log_id":"20260101000000ABCDEF%d"}}`, tc.code, tc.code)
			})
			defer cleanup()

			err := tc.call()
			if err == nil {
				t.Fatalf("应返回错误")
			}
			if !HasAPICode(err, tc.code) {
				t.Fatalf("HasAPICode(%d) = false，错误: %v", tc.code, err)
			}
			apiErr, ok := AsAPIError(err)
			if !ok || apiErr.Code != tc.code || apiErr.HTTPStatus != tc.status {
				t.Fatalf("AsAPIError = %+v ok=%v，want code=%d status=%d", apiErr, ok, tc.code, tc.status)
			}
			if !strings.HasPrefix(err.Error(), tc.action+"失败: code=") {
				t.Fatalf("错误前缀 = %q，want %q", err.Error(), tc.action+"失败: code=")
			}
			if strings.Contains(err.Error(), "HTTP ") {
				t.Fatalf("不应再以 HTTP 状态码短路: %v", err)
			}
		})
	}
}

// TestSearchMinutesFixedSorter 锁住官方 #2714：妙记搜索请求体固定 sorter=create_time_desc，
// 否则服务端相关度排序在翻页时会漏条（实测每页 6 条漏 2 条）。
func TestSearchMinutesFixedSorter(t *testing.T) {
	var body map[string]any
	var query string
	_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/minutes/v1/minutes/search" {
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
			return
		}
		query = r.URL.RawQuery
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"items":[],"has_more":false}}`)
	})
	defer cleanup()

	_, err := SearchMinutes(SearchMinutesReq{
		Query:          "周会",
		OwnerIDs:       []string{"ou_a"},
		ParticipantIDs: []string{"ou_b"},
		PageSize:       15,
		PageToken:      "pt",
	}, vcTestUserToken)
	if err != nil {
		t.Fatalf("SearchMinutes: %v", err)
	}
	if body["sorter"] != "create_time_desc" {
		t.Fatalf("sorter = %v, want create_time_desc（body=%v）", body["sorter"], body)
	}
	filter, _ := body["filter"].(map[string]any)
	if got := fmt.Sprint(filter["participant_ids"]); got != "[ou_b]" {
		t.Fatalf("filter.participant_ids = %v", filter["participant_ids"])
	}
	if got := fmt.Sprint(filter["owner_ids"]); got != "[ou_a]" {
		t.Fatalf("filter.owner_ids = %v", filter["owner_ids"])
	}
	if !strings.Contains(query, "page_token=pt") || !strings.Contains(query, "page_size=15") {
		t.Fatalf("分页参数应走 query，实际 %q", query)
	}
}

// TestGetUnifiedNoteTranscriptKeepsLargeCursor 数字游标超过 2^53 时不能丢精度。
func TestGetUnifiedNoteTranscriptKeepsLargeCursor(t *testing.T) {
	var gotQuery map[string][]string
	_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"transcript":{"markdown":"a"},"has_more":true,"next_cursor_id":7123456789012345678}}`)
	})
	defer cleanup()

	data, err := GetUnifiedNoteTranscript("7000000000000000001", map[string]string{
		"format": "markdown", "page_size": "200", "locale": "zh_cn",
	}, vcTestUserToken)
	if err != nil {
		t.Fatalf("GetUnifiedNoteTranscript: %v", err)
	}
	n, ok := data["next_cursor_id"].(json.Number)
	if !ok || n.String() != "7123456789012345678" {
		t.Fatalf("next_cursor_id = %#v，应保留为 json.Number 精确值", data["next_cursor_id"])
	}
	for k, v := range map[string]string{"format": "markdown", "page_size": "200", "locale": "zh_cn"} {
		if got := gotQuery[k]; len(got) != 1 || got[0] != v {
			t.Fatalf("query %s = %v, want %s", k, got, v)
		}
	}
	if _, has := gotQuery["transcript_format"]; has {
		t.Fatalf("不应再发送错误的 transcript_format 参数")
	}
}

// TestGetMinuteTranscriptReturnsTextBody 成功时直接返回文本字节。
func TestGetMinuteTranscriptReturnsTextBody(t *testing.T) {
	_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprint(w, "说话人1 00:00\n你好")
	})
	defer cleanup()
	b, err := GetMinuteTranscript("obcnxxxx", vcTestUserToken)
	if err != nil || string(b) != "说话人1 00:00\n你好" {
		t.Fatalf("GetMinuteTranscript = %q, %v", b, err)
	}
}

// TestListActiveMeetingsUserIDQuery Bot 身份下 user_id 走 query。
func TestListActiveMeetingsUserIDQuery(t *testing.T) {
	var gotPath, gotUserID string
	_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotUserID = r.URL.Query().Get("user_id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"meetings":[{"meeting_id":"6911188411932033028","meeting_no":"123456789","meeting_title":"周会"}]}}`)
	})
	defer cleanup()
	data, err := ListActiveMeetings("ou_xxx", vcTestUserToken)
	if err != nil {
		t.Fatalf("ListActiveMeetings: %v", err)
	}
	if gotPath != "/open-apis/vc/v1/bots/user_active_meeting" || gotUserID != "ou_xxx" {
		t.Fatalf("path=%q user_id=%q", gotPath, gotUserID)
	}
	if !strings.Contains(string(data), "6911188411932033028") {
		t.Fatalf("data = %s", data)
	}
}
