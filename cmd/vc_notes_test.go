package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// TestVCNotesMeetingPathMinuteTokenAndNoteType meeting-ids 路径补 minute_token（录制接口）
// 与 note_id / note_display_type；纪要无权限时记入 hint 而不是静默丢弃。
func TestVCNotesMeetingPathMinuteTokenAndNoteType(t *testing.T) {
	cases := []struct {
		name       string
		noteStatus int
		noteBody   string
		wantType   string
		wantHint   string
		wantVerb   string
	}{
		{"普通纪要", 200, noteDetailJSON(1), "normal", "", "doc_verbatim"},
		{"统一纪要", 200, noteDetailJSON(2), "unified", "", "doc_verbatim"},
		{"无纪要权限", 403, `{"code":121005,"msg":"no permission"}`, "", "121005", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateMsgTokenTestEnv(t)
			t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
			cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/open-apis/vc/v1/meetings/" + testVCMeetingID:
					_, _ = fmt.Fprint(w, `{"code":0,"data":{"meeting":{"id":"`+testVCMeetingID+`","topic":"周会","start_time":"1790000000","note_id":"`+testNoteID+`"}}}`)
				case "/open-apis/vc/v1/meetings/" + testVCMeetingID + "/recording":
					_, _ = fmt.Fprint(w, `{"code":0,"data":{"recording":{"url":"https://example.feishu.cn/minutes/obcnabcdef","duration":"60"}}}`)
				case "/open-apis/vc/v1/notes/" + testNoteID:
					w.WriteHeader(tc.noteStatus)
					_, _ = fmt.Fprint(w, tc.noteBody)
				default:
					http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
				}
			})
			defer cleanup()

			opts := &notesOptions{Token: "u-env-token", seenTranscripts: map[string]string{}}
			v, err := processMeetingID(testVCMeetingID, opts)
			if err != nil {
				t.Fatalf("processMeetingID: %v", err)
			}
			if v.MinuteToken != "obcnabcdef" {
				t.Fatalf("minute_token = %q，应从录制接口补齐", v.MinuteToken)
			}
			if v.NoteID != testNoteID || v.NoteDisplayType != tc.wantType || v.VerbatimDoc != tc.wantVerb {
				t.Fatalf("view = %+v", v)
			}
			if tc.wantHint != "" && !strings.Contains(v.Hint, tc.wantHint) {
				t.Fatalf("hint = %q, want 包含 %q", v.Hint, tc.wantHint)
			}
			b, _ := json.Marshal(v)
			if !strings.Contains(string(b), `"note_id":"`+testNoteID+`"`) {
				t.Fatalf("JSON 应包含 note_id: %s", b)
			}
		})
	}
}
