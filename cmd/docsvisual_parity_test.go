package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
)

// docsVisualFixtures 在临时目录生成 dry-run 需要的本地文件（图片、图表源码、节点 JSON、Markdown）。
func docsVisualFixtures(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\rIDATx\x9cc\xf8\x0f\x00\x00\x01\x01\x00\x05\x18\xd8N\x00\x00\x00\x00IEND\xaeB`\x82")
	files := map[string][]byte{
		"a.png":      png,
		"a.mmd":      []byte("graph TD\nA-->B\n"),
		"nodes.json": []byte(`[{"type":"text_shape","x":1,"y":1}]`),
		"a.md":       []byte("# t\n\nhello\n"),
		"a.svg":      []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><rect width="5" height="5"/></svg>`),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// A2 / D1：无凭据时静态预览类 --dry-run exit 0、不联网，预览含关键请求。
func TestDocsVisualDryRunWithoutCredentials(t *testing.T) {
	dir := docsVisualFixtures(t)
	f := func(name string) string { return filepath.Join(dir, name) }
	cases := []struct {
		args []string
		want []string
	}{
		{[]string{"doc", "create", "--title", "e2e", "--content", "<p>x</p>"}, nil},
		{[]string{"doc", "create", "--title", "e2e"}, []string{"/open-apis/docx/v1/documents"}},
		{[]string{"doc", "content-update", "doc123", "--mode", "append", "--markdown", "x"}, nil},
		{[]string{"doc", "history", "revert", "doc123", "--history-version-id", "3"}, []string{"history/revert"}},
		{[]string{"doc", "resource", "delete", "doc123", "--type", "cover"}, []string{"update_cover"}},
		{[]string{"doc", "resource", "update", "doc123", "--type", "cover", "--file", f("a.png")}, []string{"upload_all"}},
		{[]string{"doc", "resource", "download", "doc123", "--type", "cover", "-o", filepath.Join(dir, "cover.png")}, []string{"/download"}},
		{[]string{"doc", "media-insert", "doc123", "--file", f("a.png")}, []string{`"docx_image"`, "upload_all", "replace_image", "/children"}},
		{[]string{"doc", "media-insert", "https://xxx.feishu.cn/wiki/wikcnAbc", "--type", "file", "--file", f("a.md"), "--file-view", "preview"}, []string{"node_by_token", `"docx_file"`, "replace_file", `"view_type": 2`}},
		{[]string{"markdown", "create", "--name", "a.md", "--content", "x"}, []string{"upload_all"}},
		{[]string{"markdown", "diff", "--file-token", "boxcnxxx", "--file", f("a.md")}, nil},
		{[]string{"markdown", "fetch", "--file-token", "boxcnxxx"}, nil},
		{[]string{"markdown", "overwrite", "--file-token", "boxcnxxx", "--file", f("a.md")}, nil},
		{[]string{"markdown", "patch", "--file-token", "boxcnxxx", "--pattern", "hello", "--content", "hi"}, nil},
		{[]string{"board", "import", "wb123", f("a.mmd"), "--syntax", "mermaid", "-o", "json"}, []string{"/nodes/plantuml"}},
		{[]string{"board", "update", "wb123", f("nodes.json")}, nil},
		{[]string{"board", "delete", "wb123", "--node-ids", "n1,n2"}, []string{`"DELETE"`, "/nodes/batch_delete", `"n2"`}},
		{[]string{"board", "delete", "wb123", "--all"}, []string{`"GET"`, "<all node ids>"}},
		{[]string{"board", "create-notes", "wb123", f("nodes.json")}, []string{`"POST"`, "/whiteboards/wb123/nodes?user_id_type=open_id"}},
		{[]string{"slides", "media-upload", "--presentation-token", "pres123", "--file", f("a.png")}, []string{"upload_all", `"slide_file"`, `"parent_node": "pres123"`}},
		{[]string{"slides", "history", "list", "pres123"}, []string{"/histories", `"page_size": 20`}},
		{[]string{"slides", "history", "revert", "pres123", "--history-version-id", "7"}, []string{"/history/revert", `"history_version_id": "7"`}},
		{[]string{"slides", "history", "revert-status", "https://xxx.feishu.cn/wiki/wikcnAbc", "--task-id", "task_x"}, []string{"node_by_token", "/history/revert_status", `"task_id": "task_x"`}},
	}
	for _, tc := range cases {
		hits := isolateBitableNoCredEnv(t)
		args := append(append([]string{}, tc.args...), "--dry-run")
		stdout, stderr, err := runCLI(t, args...)
		if err != nil {
			t.Errorf("%v: 无凭据 dry-run 应成功，exit=%d err=%v stderr=%s", tc.args, exitCodeFor(err), err, stderr)
			continue
		}
		for _, w := range tc.want {
			if !strings.Contains(stdout+stderr, w) {
				t.Errorf("%v: 预览缺少 %q:\n%s", tc.args, w, stdout)
			}
		}
		if n := atomic.LoadInt32(hits); n != 0 {
			t.Errorf("%v: dry-run 不应联网，命中 %d 次", tc.args, n)
		}
	}
}

// 去掉 --dry-run 后仍需凭据：无凭据 exit 3、不联网。
func TestDocsVisualWithoutDryRunStillNeedsCredentials(t *testing.T) {
	dir := docsVisualFixtures(t)
	for _, args := range [][]string{
		{"doc", "create", "--title", "e2e"},
		{"doc", "media-insert", "doc123", "--file", filepath.Join(dir, "a.png")},
		{"markdown", "fetch", "--file-token", "boxcnxxx"},
		{"board", "delete", "wb123", "--node-ids", "n1"},
		{"board", "update", "wb123", filepath.Join(dir, "nodes.json")},
		{"slides", "media-upload", "--presentation-token", "pres123", "--file", filepath.Join(dir, "a.png")},
		{"slides", "history", "list", "pres123"},
		{"slides", "history", "revert", "pres123", "--history-version-id", "7", "--yes"},
	} {
		hits := isolateBitableNoCredEnv(t)
		_, _, err := runCLI(t, args...)
		if got := exitCodeFor(err); got != clierr.ExitAuth {
			t.Errorf("%v: exit=%d want %d (err=%v)", args, got, clierr.ExitAuth, err)
		}
		if n := atomic.LoadInt32(hits); n != 0 {
			t.Errorf("%v: 缺凭据不应联网，命中 %d 次", args, n)
		}
	}
}

// C：参数错误 exit 2（文本不变），先于凭据检查：有凭据、无凭据都是 2，且不联网。
func TestDocsVisualParamErrorsAreUsageErrors(t *testing.T) {
	dir := docsVisualFixtures(t)
	cases := []struct {
		args []string
		msg  string
	}{
		{[]string{"board", "delete", "wb123"}, "请指定 --node-ids 或 --all"},
		{[]string{"board", "update", "wb123"}, "请提供节点 JSON 文件路径"},
		{[]string{"board", "update", "wb123", filepath.Join(dir, "a.md"), "--dry-run"}, "解析节点 JSON 失败"},
		{[]string{"board", "create-notes", "wb123", filepath.Join(dir, "nodes.json"), "--client-token", "short"}, "--client-token 长度至少为 10 个字符"},
		{[]string{"board", "update", "wb123", filepath.Join(dir, "nodes.json"), "--as", "robot"}, "--as 仅支持 bot|user|auto"},
		{[]string{"slides", "media-upload", "--presentation-token", "pres123"}, "--file 不能为空"},
		{[]string{"slides", "media-upload", "--file", filepath.Join(dir, "a.png")}, "--presentation-token 不能为空"},
		{[]string{"slides", "history", "revert", "pres123", "--history-version-id", "0"}, "--history-version-id 必须是 slides history list 返回的正整数"},
		{[]string{"slides", "history", "revert", "pres123", "--history-version-id", "abc"}, "不要传 revision_id"},
		{[]string{"slides", "history", "list", "pres123", "--page-size", "21"}, "--page-size 必须在 1-20 之间"},
		{[]string{"slides", "history", "revert-status", "pres123", "--task-id", " "}, "--task-id 不能为空"},
		{[]string{"slides", "history", "list", "https://xxx.feishu.cn/docx/doxcnAbc"}, ""},
		{[]string{"doc", "table", "insert-row", "doc123", "tbl123", "--index", "-2"}, "索引必须 >= 0 或 -1"},
		{[]string{"doc", "table", "delete-rows", "doc123", "tbl123", "--start", "3", "--end", "1"}, "结束索引必须大于起始索引"},
		{[]string{"doc", "delete", "doc123", "blk123", "--start", "0"}, "必须指定 --all 或 --end"},
		{[]string{"doc", "read", "doc123", "--keyword", "("}, "--keyword 正则无效"},
		{[]string{"doc", "export", "doc123", "--engine", "docs_ai", "--detail", "bogus"}, "不支持的 --detail"},
		{[]string{"doc", "media-download", "boxcnxxx", "-o", filepath.Join(dir, "x.bin"), "--timeout", "soon"}, "无效的超时时间格式"},
		{[]string{"doc", "import", filepath.Join(dir, "a.md"), "--table-column-width", "80,abc"}, "--table-column-width 第 2 项"},
		{[]string{"doc", "media-insert", "doc123", "--file", filepath.Join(dir, "missing.png")}, "--file 无效"},
	}
	for _, env := range []string{"fake-creds", "no-creds"} {
		for _, tc := range cases {
			var hits *int32
			if env == "fake-creds" {
				hits = isolatePlatformCLIEnv(t)
			} else {
				hits = isolateBitableNoCredEnv(t)
			}
			_, stderr, err := runCLI(t, tc.args...)
			if got := exitCodeFor(err); got != clierr.ExitUsage {
				t.Errorf("[%s] %v: exit=%d want 2 (err=%v)", env, tc.args, got, err)
				continue
			}
			if tc.msg != "" && !strings.Contains(err.Error()+stderr, tc.msg) {
				t.Errorf("[%s] %v: 错误应包含 %q，得到 %v", env, tc.args, tc.msg, err)
			}
			if n := atomic.LoadInt32(hits); n != 0 {
				t.Errorf("[%s] %v: 用法错误不应联网，命中 %d 次", env, tc.args, n)
			}
		}
	}
}

// docsVisualRecorder 是带凭据的 mock 服务端：tenant token 固定为 t-mock-token，
// 记录每个业务请求的 method/path/Authorization，按路径后缀返回预置响应。
type docsVisualRecorder struct {
	mu     sync.Mutex
	reqs   []docsVisualReq
	routes []docsVisualRoute
}

type docsVisualReq struct {
	method, path, auth string
	query              string
	body               map[string]any
}

type docsVisualRoute struct {
	method, suffix string
	fn             func(n int) (int, string) // n：该路由第几次命中（从 1 开始）
	hits           int
}

func (r *docsVisualRecorder) on(method, suffix string, fn func(n int) (int, string)) {
	r.routes = append(r.routes, docsVisualRoute{method: method, suffix: suffix, fn: fn})
}

func (r *docsVisualRecorder) count(method, suffix string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, q := range r.reqs {
		if q.method == method && strings.HasSuffix(q.path, suffix) {
			n++
		}
	}
	return n
}

func newDocsVisualEnv(t *testing.T, userToken string) *docsVisualRecorder {
	t.Helper()
	isolatePlatformCLIEnv(t)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", userToken)
	rec := &docsVisualRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if mockAuthHandler(w, req) {
			return
		}
		raw, _ := io.ReadAll(req.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		rec.mu.Lock()
		rec.reqs = append(rec.reqs, docsVisualReq{method: req.Method, path: req.URL.Path, auth: req.Header.Get("Authorization"), query: req.URL.RawQuery, body: body})
		var route *docsVisualRoute
		for i := range rec.routes {
			if rec.routes[i].method == req.Method && strings.HasSuffix(req.URL.Path, rec.routes[i].suffix) {
				route = &rec.routes[i]
				break
			}
		}
		if route != nil {
			route.hits++
		}
		rec.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if route == nil {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"code":404,"msg":"no route"}`)
			return
		}
		status, resp := route.fn(route.hits)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, resp)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("FEISHU_BASE_URL", srv.URL)
	t.Cleanup(func() { config.Get().BaseURL = "" })
	return rec
}

// D7 / 拍板 #9：slides / board 写命令新增 --as，默认仍是旧行为（Bot，显式 user token 时用 User）。
func TestVisualWriteAsIdentity(t *testing.T) {
	cases := []struct {
		name, envUser string
		as            []string
		wantAuth      string
	}{
		{"默认无 user token → Bot", "", nil, "Bearer t-mock-token"},
		{"默认 + FEISHU_USER_ACCESS_TOKEN → User（与旧版一致）", "u-env-token", nil, "Bearer u-env-token"},
		{"--as bot 强制 App Token", "u-env-token", []string{"--as", "bot"}, "Bearer t-mock-token"},
		{"--as user", "u-env-token", []string{"--as", "user"}, "Bearer u-env-token"},
		{"--as auto 未配置 user → Bot", "", []string{"--as", "auto"}, "Bearer t-mock-token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := newDocsVisualEnv(t, tc.envUser)
			rec.on("DELETE", "/nodes/batch_delete", func(int) (int, string) { return 200, `{"code":0,"data":{}}` })
			args := append([]string{"board", "delete", "wb123", "--node-ids", "n1"}, tc.as...)
			if _, stderr, err := runCLI(t, args...); err != nil {
				t.Fatalf("%v: %v stderr=%s", args, err, stderr)
			}
			if len(rec.reqs) != 1 || rec.reqs[0].auth != tc.wantAuth {
				t.Fatalf("身份不符：want %s，requests=%+v", tc.wantAuth, rec.reqs)
			}
		})
	}

	// --as user 但没有任何 User Token：报错且不发写请求
	rec := newDocsVisualEnv(t, "")
	_, _, err := runCLI(t, "board", "delete", "wb123", "--node-ids", "n1", "--as", "user")
	if err == nil || !strings.Contains(err.Error(), "--as user 需要 User Access Token") {
		t.Fatalf("--as user 无 token 应报错，got %v", err)
	}
	if rec.count("DELETE", "/nodes/batch_delete") != 0 {
		t.Fatal("身份解析失败不应发写请求")
	}

	// dry-run：只有显式 --as 时才在预览里展示 identity，默认输出结构不变
	isolateBitableNoCredEnv(t)
	out, _, err := runCLI(t, "slides", "update-slide", "pres123", "--slide-id", "s1", "--content", testSlideXML, "--dry-run")
	if err != nil || strings.Contains(out, `"identity"`) {
		t.Fatalf("默认 dry-run 不应出现 identity 字段：err=%v out=%s", err, out)
	}
	out, _, err = runCLI(t, "slides", "update-slide", "pres123", "--slide-id", "s1", "--content", testSlideXML, "--dry-run", "--as", "user")
	if err != nil || !strings.Contains(out, `"identity": "user"`) {
		t.Fatalf("--as user dry-run 应展示 identity=user：err=%v out=%s", err, out)
	}
}

// 所有 slides / board 写命令都注册了 --as，默认值 bot；读命令不受影响。
func TestVisualWriteCommandsHaveAsFlag(t *testing.T) {
	for _, path := range [][]string{
		{"slides", "create"}, {"slides", "add-slide"}, {"slides", "delete-slide"}, {"slides", "replace-slide"},
		{"slides", "update-slide"}, {"slides", "media-upload"}, {"slides", "history", "revert"}, {"slides", "history", "revert-status"},
		{"board", "import"}, {"board", "update"}, {"board", "clone"}, {"board", "svg-import"}, {"board", "upload-image"},
		{"board", "delete"}, {"board", "create-notes"},
	} {
		c, _, err := rootCmd.Find(path)
		if err != nil {
			t.Fatalf("%v: %v", path, err)
		}
		f := c.Flags().Lookup("as")
		if f == nil || f.DefValue != "bot" {
			t.Errorf("%v: 应有 --as 且默认 bot，got %+v", path, f)
		}
	}
}

func withFastSlidesRevertWait(t *testing.T, limit time.Duration) *[]time.Duration {
	t.Helper()
	var slept []time.Duration
	origSleep, origLimit := slidesRevertSleep, slidesRevertWaitLimit
	slidesRevertSleep = func(d time.Duration) { slept = append(slept, d) }
	slidesRevertWaitLimit = limit
	t.Cleanup(func() { slidesRevertSleep, slidesRevertWaitLimit = origSleep, origLimit })
	return &slept
}

// slides history revert --wait：只发一次 revert，按 poll_after_ms 轮询状态直到 done。
func TestSlidesHistoryRevertWaitPollsUntilDone(t *testing.T) {
	slept := withFastSlidesRevertWait(t, 5*time.Minute)
	rec := newDocsVisualEnv(t, "")
	rec.on("POST", "/history/revert", func(int) (int, string) {
		return 200, `{"code":0,"data":{"task_id":"task_1","status":"running","history_version_id":"7","poll_after_ms":1500}}`
	})
	rec.on("GET", "/history/revert_status", func(n int) (int, string) {
		if n < 3 {
			return 200, `{"code":0,"data":{"status":"running","history_version_id":"7"}}`
		}
		return 200, `{"code":0,"data":{"status":"done","history_version_id":"7"}}`
	})
	out, stderr, err := runCLI(t, "slides", "history", "revert", "pres123", "--history-version-id", "7", "--wait", "--yes")
	if err != nil {
		t.Fatalf("err=%v stderr=%s", err, stderr)
	}
	if got := rec.count("POST", "/history/revert"); got != 1 {
		t.Fatalf("revert 只能发一次，实际 %d", got)
	}
	if got := rec.count("GET", "/history/revert_status"); got != 3 {
		t.Fatalf("应轮询 3 次状态，实际 %d", got)
	}
	if !strings.Contains(out, `"status": "done"`) || !strings.Contains(out, `"task_id": "task_1"`) {
		t.Fatalf("输出应为最终状态并保留 task_id：%s", out)
	}
	for _, d := range *slept {
		if d != 1500*time.Millisecond {
			t.Fatalf("应按 poll_after_ms=1500 等待，实际 %v", *slept)
		}
	}
	for _, q := range rec.reqs {
		if strings.HasSuffix(q.path, "/history/revert_status") && !strings.Contains(q.query, "task_id=task_1") {
			t.Fatalf("状态查询应带 task_id：%s", q.query)
		}
		if strings.HasSuffix(q.path, "/history/revert") && q.body["history_version_id"] != "7" {
			t.Fatalf("revert body 应为字符串 history_version_id：%v", q.body)
		}
	}
}

// 状态查询连续失败超过 3 次：停止轮询、返回错误，但绝不重新发起 revert。
func TestSlidesHistoryRevertWaitNeverReissuesRevert(t *testing.T) {
	slept := withFastSlidesRevertWait(t, 5*time.Minute)
	rec := newDocsVisualEnv(t, "")
	rec.on("POST", "/history/revert", func(int) (int, string) {
		return 200, `{"code":0,"data":{"task_id":"task_2","status":"running"}}`
	})
	rec.on("GET", "/history/revert_status", func(int) (int, string) {
		return 200, `{"code":1254000,"msg":"internal error"}`
	})
	_, _, err := runCLI(t, "slides", "history", "revert", "pres123", "--history-version-id", "7", "--wait", "--yes")
	if err == nil || !strings.Contains(err.Error(), "task_id=task_2") {
		t.Fatalf("应报错并给出 task_id，got %v", err)
	}
	if got := rec.count("POST", "/history/revert"); got != 1 {
		t.Fatalf("状态查询失败不得重新发起 revert，实际 %d 次", got)
	}
	if got := rec.count("GET", "/history/revert_status"); got != slidesRevertMaxStatusRetries+1 {
		t.Fatalf("状态查询应重试到 %d 次，实际 %d", slidesRevertMaxStatusRetries+1, got)
	}
	if len(*slept) == 0 || (*slept)[0] != slidesRevertDefaultPoll {
		t.Fatalf("缺 poll_after_ms 时应默认等待 10s，实际 %v", *slept)
	}
}

// 超过整体上限仍 running：exit 0，标注 wait_timed_out，不当作失败。
func TestSlidesHistoryRevertWaitTimeoutIsNotFailure(t *testing.T) {
	withFastSlidesRevertWait(t, 25*time.Second)
	rec := newDocsVisualEnv(t, "")
	rec.on("POST", "/history/revert", func(int) (int, string) {
		return 200, `{"code":0,"data":{"task_id":"task_3","status":"running","poll_after_ms":10000}}`
	})
	rec.on("GET", "/history/revert_status", func(int) (int, string) {
		return 200, `{"code":0,"data":{"status":"running"}}`
	})
	out, stderr, err := runCLI(t, "slides", "history", "revert", "pres123", "--history-version-id", "7", "--wait", "--yes")
	if err != nil {
		t.Fatalf("超时仍 running 不应失败：%v", err)
	}
	if !strings.Contains(out, `"wait_timed_out": true`) || !strings.Contains(stderr, "revert-status pres123 --task-id task_3") {
		t.Fatalf("应标注超时并提示 revert-status：out=%s stderr=%s", out, stderr)
	}
	if got := rec.count("POST", "/history/revert"); got != 1 {
		t.Fatalf("revert 只能发一次，实际 %d", got)
	}
	// 上限 25s、间隔 10s：只能轮询 2 次（按累计等待计时，而不是空转到墙钟超时）
	if got := rec.count("GET", "/history/revert_status"); got != 2 {
		t.Fatalf("应轮询 2 次后停止，实际 %d", got)
	}
}

// 不带 --wait：只发 revert，不查状态；partial_failed / failed 非零退出。
func TestSlidesHistoryRevertStatusOutcomes(t *testing.T) {
	rec := newDocsVisualEnv(t, "")
	rec.on("POST", "/history/revert", func(int) (int, string) {
		return 200, `{"code":0,"data":{"task_id":"task_4","status":"running","poll_after_ms":10000}}`
	})
	if _, _, err := runCLI(t, "slides", "history", "revert", "pres123", "--history-version-id", "7", "--yes"); err != nil {
		t.Fatalf("running 不带 --wait 应 exit 0：%v", err)
	}
	if rec.count("GET", "/history/revert_status") != 0 {
		t.Fatal("不带 --wait 不应查询状态")
	}

	rec = newDocsVisualEnv(t, "")
	rec.on("GET", "/history/revert_status", func(int) (int, string) {
		return 200, `{"code":0,"data":{"status":"partial_failed","failed_block_tokens":["blk_x"]}}`
	})
	out, _, err := runCLI(t, "slides", "history", "revert-status", "pres123", "--task-id", "task_4")
	if err == nil || !strings.Contains(err.Error(), "blk_x") || !strings.Contains(out, `"task_id": "task_4"`) {
		t.Fatalf("partial_failed 应非零退出并给出 failed_block_tokens：err=%v out=%s", err, out)
	}
}

// revert 带确认门禁：非交互未 --yes 时 exit 10，不发写请求。
func TestSlidesHistoryRevertRequiresConfirmation(t *testing.T) {
	rec := newDocsVisualEnv(t, "")
	rec.on("POST", "/history/revert", func(int) (int, string) { return 200, `{"code":0,"data":{"status":"done"}}` })
	_, _, err := runCLIWithStdin(t, "", "slides", "history", "revert", "pres123", "--history-version-id", "7")
	if got := exitCodeFor(err); got != clierr.ExitConfirmationRequired {
		t.Fatalf("exit=%d want %d (err=%v)", got, clierr.ExitConfirmationRequired, err)
	}
	if rec.count("POST", "/history/revert") != 0 {
		t.Fatal("未确认不应发 revert")
	}
}

// slides history list：透传接口 data，has_more 时 stderr 提示翻页；默认身份与 slides get 一致（未登录 Bot）。
func TestSlidesHistoryList(t *testing.T) {
	rec := newDocsVisualEnv(t, "")
	rec.on("GET", "/histories", func(int) (int, string) {
		return 200, `{"code":0,"data":{"entries":[{"revision_id":3,"history_version_id":"11","edit_time":"2026-06-22T12:24:45Z"}],"has_more":true,"page_token":"pt_2"}}`
	})
	out, stderr, err := runCLI(t, "slides", "history", "list", "pres123", "--page-size", "5", "--page-token", "pt_1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"history_version_id": "11"`) || !strings.Contains(stderr, "--page-token pt_2") {
		t.Fatalf("out=%s stderr=%s", out, stderr)
	}
	q := rec.reqs[0]
	if !strings.Contains(q.query, "page_size=5") || !strings.Contains(q.query, "page_token=pt_1") || q.auth != "Bearer t-mock-token" {
		t.Fatalf("请求不符：%+v", q)
	}
}
