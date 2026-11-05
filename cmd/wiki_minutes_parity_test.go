package cmd

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

// TestWikiMinutes_DryRunWithoutCredentials：wiki 写命令与妙记写命令的 --dry-run 在无凭据（TestMain 空 HOME）时
// exit 0 并输出 dry_run；去掉 --dry-run 仍是鉴权类错误（exit 3）。
func TestWikiMinutes_DryRunWithoutCredentials(t *testing.T) {
	cases := [][]string{
		{"wiki", "create", "--space-id", "701", "--title", "t"},
		{"wiki", "update", "wikcnAbc", "--title", "t"},
		{"wiki", "delete", "https://example.feishu.cn/wiki/wikcnAbc", "--force"},
		{"wiki", "delete-space", "701", "--yes"},
		{"wiki", "move", "wikcnAbc", "--target-space", "701"},
		{"wiki", "move-docs", "doxAbc", "--space-id", "701"},
		{"wiki", "move-to-drive", "--node-token", "wikcnAbc"},
		{"minutes", "upload", "--file-token", "boxAbc"},
		{"minutes", "update", "https://example.feishu.cn/minutes/obcnAbc", "--topic", "t"},
		{"minutes", "summary", "obcnAbc", "--summary", "x"},
		{"minutes", "todo", "obcnAbc", "--operation", "add", "--todo", "x", "--is-done=false"},
		{"minutes", "speaker-replace", "obcnAbc", "--from-speaker-id", "s1", "--to-user-id", "ou_x"},
		{"minutes", "word-replace", "obcnAbc", "--replace-words", `[{"source_word":"a","target_word":"b"}]`},
	}
	for _, args := range cases {
		name := strings.Join(args[:2], " ")
		out, _, err := runCLI(t, append(append([]string{}, args...), "--dry-run")...)
		if err != nil {
			t.Fatalf("%s --dry-run 无凭据应成功: %v", name, err)
		}
		if !strings.Contains(out, `"dry_run": true`) {
			t.Fatalf("%s --dry-run 输出缺 dry_run:\n%s", name, out)
		}
		_, _, err = runCLI(t, args...)
		if code := exitCodeFor(err); code != clierr.ExitAuth {
			t.Fatalf("%s 去掉 --dry-run 无凭据应 exit 3，got %d: %v", name, code, err)
		}
	}
}

// TestWikiMinutes_UsageErrorsExit2：参数错误即使无凭据也先报用法错误（exit 2）。
func TestWikiMinutes_UsageErrorsExit2(t *testing.T) {
	cases := [][]string{
		{"wiki", "update", "https://example.feishu.cn/unknown/x", "--title", "t", "--dry-run"},
		{"wiki", "move", "wikcnAbc", "--dry-run"},
		{"minutes", "upload", "--file-token", "", "--dry-run"},
		{"minutes", "update", "obcnAbc", "--topic", " ", "--dry-run"},
		{"minutes", "summary", "obcnAbc", "--summary", "  ", "--dry-run"},
		{"minutes", "todo", "obcnAbc", "--dry-run"},
		{"minutes", "todo", "obcnAbc", "--operation", "delete", "--todo-id", "t1", "--todo", "x", "--dry-run"},
		{"minutes", "todo", "obcnAbc", "--operation", "add", "--todo", "x", "--todos", `[]`, "--dry-run"},
		{"minutes", "speaker-replace", "obcnAbc", "--from-speaker-id", "s1", "--to-user-id", "u1", "--dry-run"},
		{"minutes", "word-replace", "obcnAbc", "--replace-words", `[{"source_word":"a","target_word":"b"},{"source_word":" a ","target_word":"c"}]`, "--dry-run"},
		{"minutes", "word-replace", "obcnAbc", "--replace-words", `[{"source_word":"","target_word":"b"}]`},
		{"minutes", "update", "https://example.feishu.cn/docx/x", "--topic", "t"},
	}
	for _, args := range cases {
		_, _, err := runCLI(t, args...)
		if code := exitCodeFor(err); code != clierr.ExitUsage {
			t.Fatalf("%v 应 exit 2，got %d: %v", args, code, err)
		}
	}
}

func TestMinutesParseReplaceWords(t *testing.T) {
	got, err := parseMinuteReplaceWords(`[{"source_word":" 飞树 ","target_word":"飞书"},{"source_word":"b","target_word":""}]`)
	if err != nil {
		t.Fatal(err)
	}
	want := []map[string]string{{"source_word": "飞树", "target_word": "飞书"}, {"source_word": "b", "target_word": ""}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if _, err := parseMinuteReplaceWords(`[{"source_word":"a"},{"source_word":"a "}]`); err == nil || !strings.Contains(err.Error(), "重复") {
		t.Fatalf("重复 source_word 应报错，got %v", err)
	}
}

func TestMinutesBuildTodoItem(t *testing.T) {
	f := false
	item, err := buildMinuteTodoItem(minuteTodoSpec{Operation: "add", Content: " x ", IsDone: &f})
	if err != nil || !reflect.DeepEqual(item, map[string]any{"operation": "add", "content": "x", "is_done": false}) {
		t.Fatalf("add item = %v, %v", item, err)
	}
	item, err = buildMinuteTodoItem(minuteTodoSpec{Operation: "delete", TodoID: "t1"})
	if err != nil || !reflect.DeepEqual(item, map[string]any{"operation": "delete", "todo_id": "t1"}) {
		t.Fatalf("delete item = %v, %v", item, err)
	}
	for _, bad := range []minuteTodoSpec{
		{Operation: "add", Content: "x"},
		{Operation: "add", Content: "x", IsDone: &f, TodoID: "t"},
		{Operation: "update", Content: "x", IsDone: &f},
		{Operation: "delete", TodoID: "t", IsDone: &f},
		{Operation: "nope"},
	} {
		if _, err := buildMinuteTodoItem(bad); err == nil {
			t.Fatalf("%+v 应报错", bad)
		}
	}
}

func TestMinutesSplitWordReplaceResults(t *testing.T) {
	words := []map[string]string{{"source_word": "a"}, {"source_word": "b"}}
	ok1, fail1, ok := splitMinuteWordReplaceResults(words, json.RawMessage(`{"replace_word_counts":[{"source_word":"a","replace_count":2},{"source_word":"b","replace_count":0}]}`))
	if !ok || !reflect.DeepEqual(ok1, []string{"a"}) || !reflect.DeepEqual(fail1, []string{"b"}) {
		t.Fatalf("got %v %v %v", ok1, fail1, ok)
	}
	if _, _, ok := splitMinuteWordReplaceResults(words, json.RawMessage(`{}`)); ok {
		t.Fatal("缺 replace_word_counts 时 ok 应为 false")
	}
}

func TestMinutesExtractUploadedMinuteToken(t *testing.T) {
	if got := extractUploadedMinuteToken("https://example.feishu.cn/minutes/obcnAbc?from=x"); got != "obcnAbc" {
		t.Fatalf("got %q", got)
	}
	if got := extractUploadedMinuteToken(""); got != "" {
		t.Fatalf("got %q", got)
	}
}

// TestMinutesWriteRequests：真实执行路径（mock server）下各写命令的 method/path/body 与 dry-run 一致。
func TestMinutesWriteRequests(t *testing.T) {
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	ok := func(*http.Request, map[string]any) (int, string) { return 200, `{"code":0,"data":{}}` }
	m.on("PATCH", "/minutes/v1/minutes/obcnAbc", ok)
	m.on("PUT", "/minutes/obcnAbc/summary", ok)
	m.on("POST", "/minutes/obcnAbc/todo", ok)
	m.on("PUT", "/minutes/obcnAbc/transcript/speaker", ok)
	m.on("PUT", "/minutes/obcnAbc/transcript/word", func(*http.Request, map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"replace_word_counts":[{"source_word":"a","replace_count":1}]}}`
	})
	m.on("POST", "/minutes/v1/minutes/upload", func(*http.Request, map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"minute_url":"https://example.feishu.cn/minutes/obcnNew"}}`
	})
	with := func(kv ...string) map[string]string {
		f := map[string]string{"user-access-token": "u-test"}
		for i := 0; i+1 < len(kv); i += 2 {
			f[kv[i]] = kv[i+1]
		}
		return f
	}
	if _, err := runSlidesCmd(t, minutesUpdateCmd, []string{"obcnAbc"}, with("topic", "新标题")); err != nil {
		t.Fatal(err)
	}
	if _, err := runSlidesCmd(t, minutesSummaryCmd, []string{"obcnAbc"}, with("summary", "## s")); err != nil {
		t.Fatal(err)
	}
	if _, err := runSlidesCmd(t, minutesTodoCmd, []string{"obcnAbc"}, with("todos", `[{"operation":"delete","todo_id":"t1"}]`)); err != nil {
		t.Fatal(err)
	}
	if _, err := runSlidesCmd(t, minutesSpeakerReplaceCmd, []string{"obcnAbc"}, with("from-speaker-id", "s1", "to-user-id", "ou_x")); err != nil {
		t.Fatal(err)
	}
	if _, err := runSlidesCmd(t, minutesWordReplaceCmd, []string{"obcnAbc"}, with("replace-words", `[{"source_word":"a","target_word":"b"}]`)); err != nil {
		t.Fatal(err)
	}
	out, err := runSlidesCmd(t, minutesUploadCmd, nil, with("file-token", "boxAbc", "output", "json"))
	if err != nil || !strings.Contains(out, `"minute_token": "obcnNew"`) {
		t.Fatalf("upload out=%s err=%v", out, err)
	}

	check := func(method, suffix string, want map[string]any) {
		t.Helper()
		reqs := m.find(method, suffix)
		if len(reqs) != 1 {
			t.Fatalf("%s %s 请求数 = %d", method, suffix, len(reqs))
		}
		gotJSON, _ := json.Marshal(reqs[0].Body)
		wantJSON, _ := json.Marshal(want)
		if string(gotJSON) != string(wantJSON) {
			t.Fatalf("%s %s body = %s, want %s", method, suffix, gotJSON, wantJSON)
		}
	}
	check("PATCH", "/minutes/v1/minutes/obcnAbc", map[string]any{"topic": "新标题"})
	check("PUT", "/minutes/obcnAbc/summary", map[string]any{"summary": "## s"})
	check("POST", "/minutes/obcnAbc/todo", map[string]any{"todo_items": []any{map[string]any{"operation": "delete", "todo_id": "t1"}}})
	check("PUT", "/minutes/obcnAbc/transcript/speaker", map[string]any{"from_speaker_id": "s1", "to_user_id": "ou_x"})
	check("PUT", "/minutes/obcnAbc/transcript/word", map[string]any{"minute_token": "obcnAbc", "replace_words": []any{map[string]any{"source_word": "a", "target_word": "b"}}})
	check("POST", "/minutes/v1/minutes/upload", map[string]any{"file_token": "boxAbc"})
	if q := m.find("PUT", "/transcript/speaker")[0].Query.Get("user_id_type"); q != "open_id" {
		t.Fatalf("speaker-replace user_id_type = %q", q)
	}
}

// TestMinutesWordReplaceAllMissed：全部未命中时报错（不当成功）。
func TestMinutesWordReplaceAllMissed(t *testing.T) {
	m, srv := newSlidesMockServer(t)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	m.on("PUT", "/transcript/word", func(*http.Request, map[string]any) (int, string) {
		return 200, `{"code":0,"data":{"replace_word_counts":[{"source_word":"a","replace_count":0}]}}`
	})
	_, err := runSlidesCmd(t, minutesWordReplaceCmd, []string{"obcnAbc"}, map[string]string{"user-access-token": "u-test", "replace-words": `[{"source_word":"a","target_word":"b"}]`})
	if err == nil || !strings.Contains(err.Error(), "未在妙记") {
		t.Fatalf("全部未命中应报错，got %v", err)
	}
}
