package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const testNotesMinuteToken = "obcnabcdef"

// vcNotesStub 假 VC / 妙记 / 日历服务，记录各端点命中次数。
type vcNotesStub struct {
	mu           sync.Mutex
	hits         map[string]int
	hasRecording bool
}

func (s *vcNotesStub) hit(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hits[key]++
}

func (s *vcNotesStub) count(key string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits[key]
}

func startVCNotesStub(t *testing.T, hasRecording bool) (*vcNotesStub, string) {
	t.Helper()
	stub := &vcNotesStub{hits: map[string]int{}, hasRecording: hasRecording}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-fake","expire":7200}`)
		case "/open-apis/vc/v1/meetings/" + testVCMeetingID:
			stub.hit("meeting")
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"meeting":{"id":"`+testVCMeetingID+`","topic":"周会","start_time":"1790000000"}}}`)
		case "/open-apis/vc/v1/meetings/" + testVCMeetingID + "/recording":
			stub.hit("recording")
			if !stub.hasRecording {
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprint(w, `{"code":121001,"msg":"recording not found"}`)
				return
			}
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"recording":{"url":"https://example.feishu.cn/minutes/`+testNotesMinuteToken+`","duration":"60"}}}`)
		case "/open-apis/minutes/v1/minutes/" + testNotesMinuteToken + "/artifacts":
			stub.hit("artifacts")
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"summary":"摘要内容","minute_todos":[]}}`)
		case "/open-apis/minutes/v1/minutes/" + testNotesMinuteToken + "/transcript":
			stub.hit("transcript")
			w.Header().Set("Content-Type", "text/plain")
			_, _ = fmt.Fprint(w, "说话人1 00:00\n逐字稿正文")
		case "/open-apis/calendar/v4/calendars/primary":
			stub.hit("primary")
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"calendars":[{"calendar":{"calendar_id":"cal_fp_test"}}]}}`)
		case "/open-apis/calendar/v4/calendars/cal_fp_test/events/mget_instance_relation_info":
			stub.hit("relation")
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"instance_relation_infos":[{"instance_id":"evt_fp_test","meeting_instance_ids":["`+testVCMeetingID+`"],"meeting_notes":["`+testNotesMinuteToken+`"]}]}}`)
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	content := fmt.Sprintf("app_id: \"test_app_id\"\napp_secret: \"test_app_secret\"\nbase_url: \"%s\"\n", srv.URL)
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return stub, cfgPath
}

type notesJSON struct {
	Items []struct {
		ID    string          `json:"id"`
		OK    bool            `json:"ok"`
		Error string          `json:"error"`
		Data  json.RawMessage `json:"data"`
	} `json:"items"`
	Summary vcBatchSummary `json:"summary"`
}

func parseNotesJSON(t *testing.T, stdout string) notesJSON {
	t.Helper()
	var out notesJSON
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("输出不是 JSON: %v\n%s", err, stdout)
	}
	return out
}

// TestVCNotesMeetingPathFetchesExtras --meeting-ids 路径按录制解析出的 minute_token
// 补拉 AI 产物与逐字稿（以前静默忽略这两个开关）。
func TestVCNotesMeetingPathFetchesExtras(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
	stub, cfg := startVCNotesStub(t, true)
	dir := t.TempDir()
	stdout, stderr, err := runCLI(t, "vc", "notes", "--meeting-ids", testVCMeetingID, "--with-artifacts",
		"--download-transcript", "--output-dir", dir, "-o", "json", "--config", cfg)
	if err != nil {
		t.Fatalf("vc notes 失败: %v\nstderr=%s", err, stderr)
	}
	out := parseNotesJSON(t, stdout)
	if len(out.Items) != 1 || !out.Items[0].OK {
		t.Fatalf("items = %+v", out.Items)
	}
	var view struct {
		MinuteToken    string         `json:"minute_token"`
		Artifacts      map[string]any `json:"artifacts"`
		TranscriptPath string         `json:"transcript_path"`
	}
	if err := json.Unmarshal(out.Items[0].Data, &view); err != nil {
		t.Fatal(err)
	}
	if view.MinuteToken != testNotesMinuteToken || view.Artifacts["summary"] != "摘要内容" {
		t.Fatalf("meeting 路径应补拉 AI 产物: %+v", view)
	}
	wantPath := filepath.Join(dir, "artifact-周会-"+testNotesMinuteToken, "transcript.txt")
	if view.TranscriptPath != wantPath {
		t.Fatalf("transcript_path = %q, want %q", view.TranscriptPath, wantPath)
	}
	if b, err := os.ReadFile(wantPath); err != nil || string(b) != "说话人1 00:00\n逐字稿正文" {
		t.Fatalf("逐字稿内容 = %q, %v", b, err)
	}
	if stub.count("artifacts") != 1 || stub.count("transcript") != 1 {
		t.Fatalf("hits = %v", stub.hits)
	}
}

// TestVCNotesMeetingWithoutRecordingWarnsExtras 会议没有录制时不静默：hint + stderr 告警，
// 不调用妙记产物接口，也不算失败（没有可下载的妙记）。
func TestVCNotesMeetingWithoutRecordingWarnsExtras(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
	stub, cfg := startVCNotesStub(t, false)
	stdout, stderr, err := runCLI(t, "vc", "notes", "--meeting-ids", testVCMeetingID, "--with-artifacts", "-o", "json", "--config", cfg)
	if err != nil {
		t.Fatalf("无录制不应算失败: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stderr, "未能从会议录制解析出 minute_token") || !strings.Contains(stderr, "--with-artifacts") {
		t.Fatalf("stderr 应告警未获取 AI 产物: %q", stderr)
	}
	out := parseNotesJSON(t, stdout)
	if !strings.Contains(string(out.Items[0].Data), "未获取 --with-artifacts") {
		t.Fatalf("hint 应说明未获取: %s", out.Items[0].Data)
	}
	if stub.count("artifacts") != 0 {
		t.Fatalf("解析不到 minute_token 不应请求 artifacts")
	}
}

// TestVCNotesTranscriptWriteFailureExitsNonZero 逐字稿写文件失败：条目 ok=false、保留已取到的数据，
// 命令以 exit 1 结束（以前 transcript_path 里塞 "下载失败: ..." 且 ok=true、exit 0）。
func TestVCNotesTranscriptWriteFailureExitsNonZero(t *testing.T) {
	for _, path := range []string{"minute-tokens", "meeting-ids"} {
		t.Run(path, func(t *testing.T) {
			isolateMsgTokenTestEnv(t)
			t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
			_, cfg := startVCNotesStub(t, true)
			dir := t.TempDir()
			// 让逐字稿目录无法创建：同名位置已有一个普通文件
			title := "周会"
			if path == "minute-tokens" {
				title = testNotesMinuteToken // minute 基础信息接口不可用时标题退化为 token
			}
			blocker := filepath.Join(dir, "artifact-"+title+"-"+testNotesMinuteToken)
			if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
			id := testVCMeetingID
			if path == "minute-tokens" {
				id = testNotesMinuteToken
			}
			stdout, stderr, err := runCLI(t, "vc", "notes", "--"+path, id, "--download-transcript", "--output-dir", dir, "-o", "json", "--config", cfg)
			if err == nil {
				t.Fatalf("逐字稿写入失败应非零退出\nstdout=%s\nstderr=%s", stdout, stderr)
			}
			if code := exitCodeFor(err); code != 1 {
				t.Fatalf("部分失败退出码 = %d, want 1 (err=%v)", code, err)
			}
			out := parseNotesJSON(t, stdout)
			if len(out.Items) != 1 || out.Items[0].OK || !strings.Contains(out.Items[0].Error, "逐字稿下载失败") {
				t.Fatalf("条目应 ok=false 并说明逐字稿失败: %+v", out.Items)
			}
			data := string(out.Items[0].Data)
			if !strings.Contains(data, `"transcript_error"`) || strings.Contains(data, `"transcript_path"`) {
				t.Fatalf("失败原因应放 transcript_error 而不是 transcript_path: %s", data)
			}
			if !strings.Contains(data, testNotesMinuteToken) {
				t.Fatalf("已取到的数据应保留: %s", data)
			}
			if out.Summary.Failed != 1 {
				t.Fatalf("summary = %+v", out.Summary)
			}
		})
	}
}

// TestVCNotesCalendarPathFetchesExtrasOnce 日历路径：会议路径解析出的 minute_token 补拉产物，
// 同一 minute_token 不在 minute 路径重复处理。
func TestVCNotesCalendarPathFetchesExtrasOnce(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
	stub, cfg := startVCNotesStub(t, true)
	dir := t.TempDir()
	stdout, stderr, err := runCLI(t, "vc", "notes", "--calendar-event-ids", "evt_fp_test", "--with-artifacts",
		"--download-transcript", "--output-dir", dir, "-o", "json", "--config", cfg)
	if err != nil {
		t.Fatalf("vc notes 日历路径失败: %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
	}
	out := parseNotesJSON(t, stdout)
	if len(out.Items) != 1 || !out.Items[0].OK {
		t.Fatalf("items = %+v", out.Items)
	}
	var views []struct {
		Source         string         `json:"source"`
		Artifacts      map[string]any `json:"artifacts"`
		TranscriptPath string         `json:"transcript_path"`
	}
	if err := json.Unmarshal(out.Items[0].Data, &views); err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].Source != "meeting_id" || views[0].Artifacts["summary"] != "摘要内容" || views[0].TranscriptPath == "" {
		t.Fatalf("日历路径应经会议补拉产物: %+v", views)
	}
	if stub.count("artifacts") != 1 || stub.count("transcript") != 1 {
		t.Fatalf("同一 minute_token 只应拉取一次: %v", stub.hits)
	}
}
