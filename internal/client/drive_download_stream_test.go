package client

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// shrinkDownloadKnobs 缩小分片/空闲超时/退避，返回恢复函数。
func shrinkDownloadKnobs(t *testing.T, chunk int64, idle time.Duration) {
	t.Helper()
	oldChunk, oldIdle, oldDelay, oldRetries := rangeDownloadChunkSize, downloadIdleTimeout, downloadRetryBaseDelay, downloadPartRetries
	rangeDownloadChunkSize = chunk
	downloadIdleTimeout = idle
	downloadRetryBaseDelay = time.Millisecond
	downloadPartRetries = 3
	t.Cleanup(func() {
		rangeDownloadChunkSize, downloadIdleTimeout, downloadRetryBaseDelay, downloadPartRetries = oldChunk, oldIdle, oldDelay, oldRetries
	})
}

func serveRange(w http.ResponseWriter, r *http.Request, body []byte) {
	var start, end int
	if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
		return
	}
	if start >= len(body) {
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}
	if end >= len(body) {
		end = len(body) - 1
	}
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(body)))
	w.WriteHeader(http.StatusPartialContent)
	_, _ = w.Write(body[start : end+1])
}

func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("残留临时文件: %s", e.Name())
		}
	}
}

// 分片下载中某片第一次 500，应只重试该片并完整成功（旧实现任一分片失败即整体失败）。
func TestDownloadBearerURLByRange_RetriesFailedPart(t *testing.T) {
	shrinkDownloadKnobs(t, 4, 5*time.Second)
	body := []byte("0123456789abcdef")
	var mu sync.Mutex
	failures := map[string]int{}
	var ranges []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rg := r.Header.Get("Range")
		mu.Lock()
		ranges = append(ranges, rg)
		first := failures[rg] == 0
		failures[rg]++
		mu.Unlock()
		if rg == "bytes=8-11" && first {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(w, `{"code":1061001,"msg":"internal error"}`)
			return
		}
		serveRange(w, r, body)
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "f.bin")
	if err := downloadBearerURLByRange("下载文件", srv.URL, out, "u-x", 0); err != nil {
		t.Fatalf("单片瞬时失败应重试成功: %v", err)
	}
	got, _ := os.ReadFile(out)
	if string(got) != string(body) {
		t.Fatalf("内容 = %q", got)
	}
	want := "bytes=0-3,bytes=4-7,bytes=8-11,bytes=8-11,bytes=12-15"
	if strings.Join(ranges, ",") != want {
		t.Fatalf("Range 序列 = %v, want %s", ranges, want)
	}
}

// 4xx 业务错误不重试，且不残留文件。
func TestDownloadBearerURLByRange_PermanentErrorNotRetried(t *testing.T) {
	shrinkDownloadKnobs(t, 4, 5*time.Second)
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, `{"code":1061004,"msg":"forbidden"}`)
	}))
	defer srv.Close()

	dir := t.TempDir()
	out := filepath.Join(dir, "f.bin")
	err := downloadBearerURLByRange("下载文件", srv.URL, out, "u-x", 0)
	if err == nil || !HasAPICode(err, 1061004) {
		t.Fatalf("want 1061004 error, got %v", err)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("4xx 不应重试，请求次数 = %d", n)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatalf("失败不应落盘: %v", statErr)
	}
	assertNoTempFiles(t, dir)
}

// 下载失败时已存在的同名文件必须保持原样（旧实现 os.Create 直写目标，失败后 os.Remove 删掉用户原文件）。
func TestDownloadBearerURLByRange_FailureKeepsExistingFile(t *testing.T) {
	shrinkDownloadKnobs(t, 4, 5*time.Second)
	body := []byte("0123456789abcdef")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") == "bytes=4-7" {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		serveRange(w, r, body)
	}))
	defer srv.Close()

	dir := t.TempDir()
	out := filepath.Join(dir, "keep.bin")
	if err := os.WriteFile(out, []byte("user original"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := downloadBearerURLByRange("下载文件", srv.URL, out, "u-x", 0)
	if err == nil || !strings.Contains(err.Error(), "已重试") {
		t.Fatalf("分片持续失败应在重试耗尽后报错，got %v", err)
	}
	got, _ := os.ReadFile(out)
	if string(got) != "user original" {
		t.Fatalf("用户原文件被破坏: %q", got)
	}
	assertNoTempFiles(t, dir)
}

// 空闲超时：服务端第一次挂起超过空闲时间，第二次正常 → 重试成功；不再有固定 5 分钟总时长。
func TestDownloadStream_IdleTimeoutRetries(t *testing.T) {
	shrinkDownloadKnobs(t, 1<<20, 80*time.Millisecond)
	body := []byte("hello idle")
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			select {
			case <-r.Context().Done():
			case <-time.After(2 * time.Second):
			}
			return
		}
		serveRange(w, r, body)
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "idle.bin")
	if err := downloadBearerURLByRange("下载文件", srv.URL, out, "u-x", 0); err != nil {
		t.Fatalf("空闲超时后应重试成功: %v", err)
	}
	got, _ := os.ReadFile(out)
	if string(got) != string(body) {
		t.Fatalf("内容 = %q", got)
	}
}

// 慢但持续有数据的下载不应被空闲超时打断（总时长远超空闲超时）。
func TestDownloadStream_SlowButProgressingNotTimedOut(t *testing.T) {
	shrinkDownloadKnobs(t, 1<<20, 150*time.Millisecond)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "6")
		w.WriteHeader(http.StatusOK)
		for i := 0; i < 6; i++ {
			_, _ = w.Write([]byte{byte('a' + i)})
			w.(http.Flusher).Flush()
			time.Sleep(60 * time.Millisecond)
		}
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "slow.bin")
	if _, _, err := downloadStreamToFile(downloadStreamSpec{Action: "下载文件", URL: srv.URL, Bearer: "u-x"}, out, 0); err != nil {
		t.Fatalf("持续有进展的慢下载不应超时: %v", err)
	}
	got, _ := os.ReadFile(out)
	if string(got) != "abcdef" {
		t.Fatalf("内容 = %q", got)
	}
}

// 整包 GET 中途断流：用 Range 从断点续传。
func TestDownloadStream_ResumesSingleStreamWithRange(t *testing.T) {
	shrinkDownloadKnobs(t, 1<<20, 5*time.Second)
	body := []byte("0123456789")
	var ranges []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rg := r.Header.Get("Range")
		mu.Lock()
		ranges = append(ranges, rg)
		mu.Unlock()
		if rg == "" {
			// 声明 10 字节但只写 4 字节后断开
			w.Header().Set("Content-Length", "10")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body[:4])
			w.(http.Flusher).Flush()
			hj, ok := w.(http.Hijacker)
			if ok {
				conn, _, _ := hj.Hijack()
				_ = conn.Close()
			}
			return
		}
		serveRange(w, r, body)
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "resume.bin")
	if _, _, err := downloadStreamToFile(downloadStreamSpec{Action: "下载文件", URL: srv.URL, Bearer: "u-x"}, out, 0); err != nil {
		t.Fatalf("断流后应续传成功: %v", err)
	}
	got, _ := os.ReadFile(out)
	if string(got) != string(body) {
		t.Fatalf("内容 = %q, want %q", got, body)
	}
	if len(ranges) < 2 || ranges[0] != "" || !strings.HasPrefix(ranges[1], "bytes=4-") {
		t.Fatalf("续传应从 offset 4 开始，Range 序列 = %v", ranges)
	}
}

// Bot 身份（无 User Token）走流式 Bearer 下载：使用 tenant token，且进程内缓存（多次下载只换一次 token）。
func TestDownloadFileWithToken_BotStreamsWithCachedTenantToken(t *testing.T) {
	resetTenantTokenCacheForTest()
	defer resetTenantTokenCacheForTest()
	var tokenCalls int32
	var auths []string
	var mu sync.Mutex
	handler := func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "tenant_access_token") {
			atomic.AddInt32(&tokenCalls, 1)
			writeTenantToken(w)
			return
		}
		mu.Lock()
		auths = append(auths, r.Header.Get("Authorization"))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("bot-content"))
	}
	_, cleanup := stubFeishuServer(t, handler)
	defer cleanup()

	dir := t.TempDir()
	for i := 0; i < 3; i++ {
		out := filepath.Join(dir, fmt.Sprintf("bot%d.bin", i))
		if err := DownloadFileWithToken("boxcn_bot", out, ""); err != nil {
			t.Fatalf("Bot 下载失败: %v", err)
		}
		got, _ := os.ReadFile(out)
		if string(got) != "bot-content" {
			t.Fatalf("内容 = %q", got)
		}
	}
	if n := atomic.LoadInt32(&tokenCalls); n != 1 {
		t.Fatalf("tenant token 应缓存，换取次数 = %d", n)
	}
	for _, a := range auths {
		if a != "Bearer t-test" {
			t.Fatalf("Bot 下载 Authorization = %q", a)
		}
	}
}

// HTTP 200 + JSON 业务错误（Bot 路径）不应落盘，并保留业务码。
func TestDownloadFileWithToken_BotHTTP200JSONError(t *testing.T) {
	resetTenantTokenCacheForTest()
	defer resetTenantTokenCacheForTest()
	handler := func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "tenant_access_token") {
			writeTenantToken(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"code":1061004,"msg":"forbidden"}`)
	}
	_, cleanup := stubFeishuServer(t, handler)
	defer cleanup()

	out := filepath.Join(t.TempDir(), "x.bin")
	err := DownloadFileWithToken("boxcn_bot", out, "")
	if err == nil || !HasAPICode(err, 1061004) {
		t.Fatalf("want 1061004, got %v", err)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatal("业务错误不应落盘")
	}
}

// 版本下载走流式链路并带 version 查询参数。
func TestDownloadFileVersion_Streams(t *testing.T) {
	var gotVersion, gotPath string
	handler := func(w http.ResponseWriter, r *http.Request) {
		gotVersion = r.URL.Query().Get("version")
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("v2-content"))
	}
	_, cleanup := stubFeishuServer(t, handler)
	defer cleanup()

	out := filepath.Join(t.TempDir(), "v.md")
	if err := DownloadFileVersion("boxcn_v", "7", out, "u-x"); err != nil {
		t.Fatal(err)
	}
	if gotVersion != "7" || gotPath != "/open-apis/drive/v1/files/boxcn_v/download" {
		t.Fatalf("version=%q path=%q", gotVersion, gotPath)
	}
}

// DownloadFromURL：原子写 + 100MB 上限语义保持；失败不破坏已有文件。
func TestDownloadFromURL_AtomicAndRetry(t *testing.T) {
	shrinkDownloadKnobs(t, 1<<20, 5*time.Second)
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("img"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	out := filepath.Join(dir, "a.png")
	if err := DownloadFromURL(srv.URL, out); err != nil {
		t.Fatalf("503 后应重试成功: %v", err)
	}
	got, _ := os.ReadFile(out)
	if string(got) != "img" {
		t.Fatalf("内容 = %q", got)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer bad.Close()
	if err := DownloadFromURL(bad.URL, out); err == nil {
		t.Fatal("404 应失败")
	}
	got, _ = os.ReadFile(out)
	if string(got) != "img" {
		t.Fatalf("失败不应破坏已有文件: %q", got)
	}
	assertNoTempFiles(t, dir)
}

func TestContentDispositionFileName(t *testing.T) {
	cases := map[string]string{
		`attachment; filename="report..v2.pdf"`:               "report..v2.pdf",
		`attachment; filename*=UTF-8''%E6%8A%A5%E5%91%8A.pdf`: "报告.pdf",
		`attachment; filename="../../etc/passwd"`:             "passwd",
		``: "",
	}
	for cd, want := range cases {
		h := http.Header{}
		if cd != "" {
			h.Set("Content-Disposition", cd)
		}
		if got := contentDispositionFileName(h); got != want {
			t.Errorf("%q → %q, want %q", cd, got, want)
		}
	}
}
