package registry

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/internal/profile"
)

// TestMain 置位 inTestBinary，替代生产代码里的 testing.Testing()：
// 让 remoteEnabled() 在测试二进制中默认不打真实网络，同时使发布二进制
// 不必 import "testing"。
func TestMain(m *testing.M) {
	inTestBinary = true
	os.Exit(m.Run())
}

func testRegistryJSON(name, version string) []byte {
	reg := MergedRegistry{
		Version: version,
		Services: []map[string]interface{}{
			{
				"name":        name,
				"version":     "v1",
				"title":       name + " API",
				"servicePath": "/open-apis/" + name + "/v1",
			},
		},
	}
	data, _ := json.Marshal(reg)
	return data
}

func testEnvelopeJSON(name, version string) []byte {
	reg := testRegistryJSON(name, version)
	env := remoteResponse{Msg: "succeeded", Data: reg}
	data, _ := json.Marshal(env)
	return data
}

func testEnvelopeUnchanged() []byte {
	data, _ := json.Marshal(map[string]interface{}{
		"msg":  "succeeded",
		"data": map[string]interface{}{},
	})
	return data
}

func seedCache(t *testing.T, dir, name, version, brand string) {
	t.Helper()
	cDir := filepath.Join(dir, "cache")
	if err := os.MkdirAll(cDir, 0700); err != nil {
		t.Fatal(err)
	}
	payload := testRegistryJSON(name, version)
	if err := os.WriteFile(filepath.Join(cDir, "remote_meta.json"), payload, 0644); err != nil {
		t.Fatal(err)
	}
	meta, _ := json.Marshal(CacheMeta{LastCheckAt: time.Now().Unix(), Version: version, Brand: brand, Digest: payloadDigest(payload)})
	if err := os.WriteFile(filepath.Join(cDir, "remote_meta.meta.json"), meta, 0644); err != nil {
		t.Fatal(err)
	}
}

func isolateRemote(t *testing.T) string {
	t.Helper()
	ResetForTest()
	tmp := t.TempDir()
	t.Setenv("FEISHU_CLI_CONFIG_DIR", tmp)
	t.Setenv("FEISHU_CLI_REMOTE_META", "on")
	t.Setenv("HOME", tmp)
	t.Cleanup(func() {
		ResetForTest()
	})
	return tmp
}

func TestRemoteOff_SkipsRemoteLogic(t *testing.T) {
	tmp := isolateRemote(t)
	t.Setenv("FEISHU_CLI_REMOTE_META", "off")
	seedCache(t, tmp, "fake_remote_svc", "2.0.0", brandFeishu)
	Init()
	if _, ok := mergedServices["fake_remote_svc"]; ok {
		t.Error("remote off 时不应加载 cache overlay")
	}
}

func TestFirstFetch_DoesNotBlockWhenEmbeddedExists(t *testing.T) {
	isolateRemote(t)
	// 本测试验证「后台刷新」机制：显式禁用首屏同步拉取，
	// 否则 Init 会先同步等这个刻意阻塞的 server（生产默认行为，见 firstFetchSyncBudget）。
	t.Setenv("FEISHU_CLI_META_FIRST_SYNC_MS", "0")
	started := make(chan struct{})
	release := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(200)
		_, _ = w.Write(testEnvelopeJSON("remote_calendar", "2.0.0"))
	}))
	defer ts.Close()
	defer close(release)
	testMetaURL = ts.URL

	start := time.Now()
	Init()
	elapsed := time.Since(start)
	if elapsed > 300*time.Millisecond {
		t.Fatalf("有 embedded baseline 时 Init 不应同步等待远端，耗时 %s", elapsed)
	}
	if _, ok := mergedServices["remote_calendar"]; ok {
		t.Fatal("首次 Init 不得阻塞等待 overlay")
	}
	if overlaySource == "runtime" {
		t.Fatal("有 embedded 时首次应为 background，不应 runtime overlay")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("background refresh 未启动")
	}
}

func TestFirstFetch_PersistsThenNextProcessOverlays(t *testing.T) {
	isolateRemote(t)
	var sawAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
		_, _ = w.Write(testEnvelopeJSON("remote_calendar", "2.0.0"))
	}))
	defer ts.Close()
	testMetaURL = ts.URL

	Init()
	waitBackgroundRefresh()
	if sawAuth != "" {
		t.Errorf("overlay 请求不得携带 Authorization，得到 %q", sawAuth)
	}
	reinitKeepingHooks()
	Init()
	if _, ok := mergedServices["remote_calendar"]; !ok {
		t.Fatal("下一进程应从完整 cache pair overlay remote_calendar")
	}
	if overlaySource != "cache" {
		t.Errorf("source = %q, want cache", overlaySource)
	}
}

func TestCacheHit_WithinTTL_NoNetwork(t *testing.T) {
	tmp := isolateRemote(t)
	t.Setenv("FEISHU_CLI_META_TTL", "3600")
	seedCache(t, tmp, "custom_svc", "9.0.0", brandFeishu)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("TTL 内不应访问远端")
		w.WriteHeader(500)
	}))
	defer ts.Close()
	testMetaURL = ts.URL
	Init()
	if _, ok := mergedServices["custom_svc"]; !ok {
		t.Error("TTL 内应从 cache overlay custom_svc")
	}
	if overlaySource != "cache" {
		t.Errorf("source = %q, want cache", overlaySource)
	}
}

func TestUnchanged_EmptyServicesAnd304(t *testing.T) {
	t.Run("empty_services", func(t *testing.T) {
		isolateRemote(t)
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			_, _ = w.Write(testEnvelopeUnchanged())
		}))
		defer ts.Close()
		testMetaURL = ts.URL
		data, reg, err := fetchRemoteMerged("1.0.0")
		if err != nil {
			t.Fatal(err)
		}
		if data != nil || reg != nil {
			t.Fatalf("unchanged 应返回 nil,nil，got data=%v reg=%v", data != nil, reg != nil)
		}
	})
	t.Run("http_304", func(t *testing.T) {
		isolateRemote(t)
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotModified)
		}))
		defer ts.Close()
		testMetaURL = ts.URL
		data, reg, err := fetchRemoteMerged("1.0.0")
		if err != nil || data != nil || reg != nil {
			t.Fatalf("304 应视为 unchanged: data=%v reg=%v err=%v", data != nil, reg != nil, err)
		}
	})
}

func TestRemote4xx(t *testing.T) {
	isolateRemote(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
	}))
	defer ts.Close()
	testMetaURL = ts.URL
	_, _, err := fetchRemoteMerged("")
	httpErr, ok := err.(*httpError)
	if !ok || httpErr.StatusCode != 403 {
		t.Fatalf("want httpError 403, got %v", err)
	}
	Init() // 不得失败
	if len(ListFromMetaProjects()) == 0 {
		t.Fatal("4xx 应回退 embedded")
	}
	if overlaySource == "runtime" {
		t.Error("4xx 不应标记 runtime overlay")
	}
}

func TestRemoteTimeout(t *testing.T) {
	isolateRemote(t)
	t.Setenv("FEISHU_CLI_META_TIMEOUT_MS", "80")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(400 * time.Millisecond)
		w.WriteHeader(200)
		_, _ = w.Write(testEnvelopeJSON("slow", "2.0.0"))
	}))
	defer ts.Close()
	testMetaURL = ts.URL
	_, _, err := fetchRemoteMerged("")
	if err == nil {
		t.Fatal("期望超时错误")
	}
	Init()
	if _, ok := mergedServices["slow"]; ok {
		t.Error("超时不得应用 overlay")
	}
}

func TestRemoteOversize(t *testing.T) {
	isolateRemote(t)
	maxResponseSize = 32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{"msg":"succeeded","data":{"version":"2.0.0","services":[{"name":"`+strings.Repeat("x", 80)+`"}]}}`)
	}))
	defer ts.Close()
	testMetaURL = ts.URL
	_, _, err := fetchRemoteMerged("")
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("期望超限错误，got %v", err)
	}
}

func TestRemoteBadJSON(t *testing.T) {
	isolateRemote(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("not json{{{"))
	}))
	defer ts.Close()
	testMetaURL = ts.URL
	_, _, err := fetchRemoteMerged("")
	if err == nil {
		t.Fatal("期望坏 JSON 错误")
	}
}

func TestCorruptedCache_FailClosed(t *testing.T) {
	tmp := isolateRemote(t)
	cDir := filepath.Join(tmp, "cache")
	_ = os.MkdirAll(cDir, 0700)
	_ = os.WriteFile(filepath.Join(cDir, "remote_meta.json"), []byte("not json{{{"), 0644)
	meta, _ := json.Marshal(CacheMeta{LastCheckAt: time.Now().Unix(), Version: "9.0.0", Brand: brandFeishu})
	_ = os.WriteFile(filepath.Join(cDir, "remote_meta.meta.json"), meta, 0644)

	_, _, err := loadCachedMerged()
	if err == nil {
		t.Fatal("损坏 cache 应报错")
	}
	if _, statErr := os.Stat(filepath.Join(cDir, "remote_meta.json")); !os.IsNotExist(statErr) {
		t.Error("损坏 remote_meta.json 应被删除")
	}
	Init()
	if overlaySource == "cache" {
		t.Error("损坏 cache 不得 overlay")
	}
}

func TestBrandIsolation(t *testing.T) {
	tmp := isolateRemote(t)
	t.Setenv("FEISHU_CLI_META_TTL", "3600")
	t.Setenv("FEISHU_CLI_BRAND", brandLark)
	seedCache(t, tmp, "feishu_only_svc", "9.0.0", brandFeishu)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write(testEnvelopeJSON("lark_svc", "9.1.0"))
	}))
	defer ts.Close()
	testMetaURL = ts.URL
	Init()
	if _, ok := mergedServices["feishu_only_svc"]; ok {
		t.Error("品牌不匹配时不得加载旧 cache")
	}
	if _, ok := mergedServices["lark_svc"]; !ok {
		t.Error("品牌切换后应 sync fetch 新 overlay")
	}
}

func TestVersionIsolation_OlderCacheNotOverlay(t *testing.T) {
	tmp := isolateRemote(t)
	t.Setenv("FEISHU_CLI_META_TTL", "3600")
	// embedded 当前是 1.0.0；写入更旧 cache
	seedCache(t, tmp, "stale_svc", "0.0.1", brandFeishu)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("TTL 内不应刷新")
		w.WriteHeader(500)
	}))
	defer ts.Close()
	testMetaURL = ts.URL
	Init()
	if _, ok := mergedServices["stale_svc"]; ok {
		t.Error("更旧 cache 不得覆盖 embedded")
	}
	if Status().Source != "embedded" {
		t.Errorf("source = %s, want embedded", Status().Source)
	}
}

func TestVersionIsolation_NewerCacheOverlays(t *testing.T) {
	tmp := isolateRemote(t)
	t.Setenv("FEISHU_CLI_META_TTL", "3600")
	seedCache(t, tmp, "fresh_svc", "9.9.9", brandFeishu)
	Init()
	if _, ok := mergedServices["fresh_svc"]; !ok {
		t.Error("更新 cache 应 overlay")
	}
}

func TestAtomicWriteFailure_FallsBackEmbedded(t *testing.T) {
	isolateRemote(t)
	atomicWriteFn = func(path string, data []byte, perm os.FileMode) error {
		return errors.New("disk full")
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write(testEnvelopeJSON("unsaved_svc", "9.0.0"))
	}))
	defer ts.Close()
	testMetaURL = ts.URL
	Init()
	waitBackgroundRefresh()
	if _, ok := mergedServices["unsaved_svc"]; ok {
		t.Error("原子写失败不得应用 overlay")
	}
	if _, err := os.Stat(cachePath()); err == nil {
		t.Error("原子写失败不得留下 cache 文件")
	}
}

func TestNoAuthorizationHeader(t *testing.T) {
	isolateRemote(t)
	var headers http.Header
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header.Clone()
		w.WriteHeader(200)
		_, _ = w.Write(testEnvelopeJSON("authcheck", "2.0.0"))
	}))
	defer ts.Close()
	testMetaURL = ts.URL
	_, _, err := fetchRemoteMerged("")
	if err != nil {
		t.Fatal(err)
	}
	if v := headers.Get("Authorization"); v != "" {
		t.Errorf("Authorization=%q, want empty", v)
	}
}

func TestShouldRefreshAndTTL(t *testing.T) {
	t.Setenv("FEISHU_CLI_META_TTL", "60")
	if !shouldRefresh(CacheMeta{}) {
		t.Error("zero LastCheckAt 应刷新")
	}
	if shouldRefresh(CacheMeta{LastCheckAt: time.Now().Unix()}) {
		t.Error("刚检查过不应刷新")
	}
	if !shouldRefresh(CacheMeta{LastCheckAt: time.Now().Add(-2 * time.Minute).Unix()}) {
		t.Error("过期应刷新")
	}
	t.Setenv("FEISHU_CLI_META_TTL", "")
	if ttl := metaTTL(); ttl != defaultMetaTTL*time.Second {
		t.Errorf("default TTL = %v", ttl)
	}
}

func TestIsNewer(t *testing.T) {
	if !isNewer("2.0.0", "1.0.0") {
		t.Error("2.0.0 should be newer than 1.0.0")
	}
	if isNewer("1.0.0", "1.0.0") {
		t.Error("equal is not newer")
	}
	if isNewer("not-semver", "1.0.0") {
		t.Error("unparseable a is not newer")
	}
	if !isNewer("1.0.0", "not-semver") {
		t.Error("parseable a vs unparseable b is newer")
	}
}

func TestRemoteMetaURL_BrandAndOverride(t *testing.T) {
	ResetForTest()
	configuredBrand = brandFeishu
	u, err := remoteMetaURL("")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u, "open.feishu.cn") || !strings.Contains(u, "protocol=meta") {
		t.Errorf("feishu url = %s", u)
	}
	if strings.Contains(u, "data_version") {
		t.Errorf("empty version 不应带 data_version: %s", u)
	}
	configuredBrand = brandLark
	u, err = remoteMetaURL("1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u, "open.larksuite.com") || !strings.Contains(u, "data_version=1.2.3") {
		t.Errorf("lark url = %s", u)
	}
	testMetaURL = "http://127.0.0.1:9/meta"
	u, err = remoteMetaURL("x")
	if err != nil {
		t.Fatal(err)
	}
	if u != "http://127.0.0.1:9/meta" {
		t.Error("testMetaURL 应覆盖")
	}
	testMetaURL = "https://evil.example/meta"
	if _, err := remoteMetaURL("x"); err == nil {
		t.Fatal("非 loopback META_URL 应被拒绝")
	}
}

func TestFetchPreservesUnmodeledKeys(t *testing.T) {
	isolateRemote(t)
	payload := `{"msg":"succeeded","data":{"version":"test-1.0","services":[{"name":"svc","resources":{"items":{"methods":{"list":{"httpMethod":"GET","enumName":"StatusEnum"}}}}}]}}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(payload))
	}))
	defer ts.Close()
	testMetaURL = ts.URL
	data, reg, err := fetchRemoteMerged("")
	if err != nil || reg == nil {
		t.Fatalf("err=%v reg=%v", err, reg)
	}
	if !strings.Contains(string(data), `"enumName":"StatusEnum"`) {
		t.Errorf("cache bytes 丢掉未建模字段: %s", data)
	}
}

func TestCacheJSONWithoutMetaNotOverlayed(t *testing.T) {
	tmp := isolateRemote(t)
	t.Setenv("FEISHU_CLI_META_TTL", "3600")
	cDir := filepath.Join(tmp, "cache")
	_ = os.MkdirAll(cDir, 0700)
	_ = os.WriteFile(filepath.Join(cDir, "remote_meta.json"), testRegistryJSON("orphan_svc", "9.0.0"), 0644)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer ts.Close()
	testMetaURL = ts.URL
	Init()
	if _, ok := mergedServices["orphan_svc"]; ok {
		t.Fatal("缺少 cache metadata 时不得 overlay JSON")
	}
}

func TestCacheVersionMismatchNotOverlayed(t *testing.T) {
	tmp := isolateRemote(t)
	t.Setenv("FEISHU_CLI_META_TTL", "3600")
	cDir := filepath.Join(tmp, "cache")
	_ = os.MkdirAll(cDir, 0700)
	payload := testRegistryJSON("mismatch_svc", "9.0.0")
	_ = os.WriteFile(filepath.Join(cDir, "remote_meta.json"), payload, 0644)
	meta, _ := json.Marshal(CacheMeta{LastCheckAt: time.Now().Unix(), Version: "8.0.0", Brand: brandFeishu, Digest: payloadDigest(payload)})
	_ = os.WriteFile(filepath.Join(cDir, "remote_meta.meta.json"), meta, 0644)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer ts.Close()
	testMetaURL = ts.URL
	Init()
	if _, ok := mergedServices["mismatch_svc"]; ok {
		t.Fatal("meta.Version 与 JSON version 不一致时不得 overlay")
	}
}

func TestPartialAtomicPairNextProcessNotOverlay(t *testing.T) {
	isolateRemote(t)
	atomicWriteFn = func(path string, data []byte, perm os.FileMode) error {
		if strings.HasSuffix(path, "remote_meta.meta.json") {
			return errors.New("meta write fail")
		}
		return atomicWriteFile(path, data, perm)
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write(testEnvelopeJSON("partial_svc", "9.0.0"))
	}))
	defer ts.Close()
	testMetaURL = ts.URL
	Init()
	waitBackgroundRefresh()
	if _, err := os.Stat(cachePath()); err != nil {
		t.Fatalf("data 写入应成功: %v", err)
	}
	if _, err := os.Stat(cacheMetaPath()); err == nil {
		t.Fatal("meta 写入失败后不应留下 meta 文件")
	}
	atomicWriteFn = atomicWriteFile
	reinitKeepingHooks()
	t.Setenv("FEISHU_CLI_META_TTL", "3600")
	Init()
	if _, ok := mergedServices["partial_svc"]; ok {
		t.Fatal("残缺 cache pair 下一进程不得 overlay")
	}
}

func TestPartialMetaWriteSameVersionOldMetaNotOverlay(t *testing.T) {
	tmp := isolateRemote(t)
	seedCache(t, tmp, "old_svc", "9.0.0", brandFeishu)
	atomicWriteFn = func(path string, data []byte, perm os.FileMode) error {
		if strings.HasSuffix(path, "remote_meta.meta.json") {
			return errors.New("meta write fail")
		}
		return atomicWriteFile(path, data, perm)
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write(testEnvelopeJSON("new_svc", "9.0.0"))
	}))
	defer ts.Close()
	testMetaURL = ts.URL
	t.Setenv("FEISHU_CLI_META_TTL", "0")
	Init()
	waitBackgroundRefresh()
	payload, err := os.ReadFile(cachePath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), "new_svc") {
		t.Fatal("data 应已换成 new_svc")
	}
	metaRaw, err := os.ReadFile(cacheMetaPath())
	if err != nil {
		t.Fatal(err)
	}
	var cm CacheMeta
	if err := json.Unmarshal(metaRaw, &cm); err != nil {
		t.Fatal(err)
	}
	if cm.Digest == payloadDigest(payload) {
		t.Fatal("meta 写入失败时应仍是旧 digest")
	}
	atomicWriteFn = atomicWriteFile
	reinitKeepingHooks()
	t.Setenv("FEISHU_CLI_META_TTL", "3600")
	Init()
	if _, ok := mergedServices["new_svc"]; ok {
		t.Fatal("同 version 旧 meta 不得让被替换的 data 被 overlay")
	}
	if _, ok := mergedServices["old_svc"]; ok {
		t.Fatal("data 已不是 old_svc，也不得 overlay 旧内容")
	}
}

func TestInterleavingCacheDigestMismatchNotOverlay(t *testing.T) {
	tmp := isolateRemote(t)
	seedCache(t, tmp, "orig_svc", "9.0.0", brandFeishu)
	if err := os.WriteFile(filepath.Join(tmp, "cache", "remote_meta.json"), testRegistryJSON("interleaved_svc", "9.0.0"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FEISHU_CLI_META_TTL", "3600")
	hits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(200)
		_, _ = w.Write(testEnvelopeJSON("refreshed_svc", "9.1.0"))
	}))
	defer ts.Close()
	testMetaURL = ts.URL
	Init()
	if _, ok := mergedServices["interleaved_svc"]; ok {
		t.Fatal("digest 不匹配的 interleaved data 不得 overlay")
	}
	if _, ok := mergedServices["orig_svc"]; ok {
		t.Fatal("被替换掉的 orig_svc 不得 overlay")
	}
	waitBackgroundRefresh()
	if hits == 0 {
		t.Fatal("digest 不匹配应触发 refresh")
	}
	reinitKeepingHooks()
	Init()
	if _, ok := mergedServices["refreshed_svc"]; !ok {
		t.Fatal("refresh 成功后下一进程应 overlay 新 pair")
	}
}

func TestLegacyMetaWithoutDigestSkipsOverlayAndRefreshes(t *testing.T) {
	tmp := isolateRemote(t)
	cDir := filepath.Join(tmp, "cache")
	_ = os.MkdirAll(cDir, 0700)
	payload := testRegistryJSON("legacy_svc", "9.0.0")
	_ = os.WriteFile(filepath.Join(cDir, "remote_meta.json"), payload, 0644)
	meta, _ := json.Marshal(CacheMeta{LastCheckAt: time.Now().Unix(), Version: "9.0.0", Brand: brandFeishu})
	_ = os.WriteFile(filepath.Join(cDir, "remote_meta.meta.json"), meta, 0644)
	t.Setenv("FEISHU_CLI_META_TTL", "3600")
	hits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(200)
		_, _ = w.Write(testEnvelopeJSON("legacy_refreshed", "9.1.0"))
	}))
	defer ts.Close()
	testMetaURL = ts.URL
	Init()
	if _, ok := mergedServices["legacy_svc"]; ok {
		t.Fatal("无 digest 的旧 meta 不得 overlay")
	}
	waitBackgroundRefresh()
	if hits == 0 {
		t.Fatal("无 digest 的旧 meta 应 refresh")
	}
}

func TestBrandChange_UnchangedDoesNotTrustOldCache(t *testing.T) {
	tmp := isolateRemote(t)
	t.Setenv("FEISHU_CLI_BRAND", brandLark)
	seedCache(t, tmp, "feishu_only_svc", "9.0.0", brandFeishu)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write(testEnvelopeUnchanged())
	}))
	defer ts.Close()
	testMetaURL = ts.URL
	Init()
	if _, ok := mergedServices["feishu_only_svc"]; ok {
		t.Fatal("品牌切换 unchanged 不得 overlay 旧品牌 cache")
	}
	if _, err := os.Stat(filepath.Join(tmp, "cache", "remote_meta.json")); !os.IsNotExist(err) {
		t.Fatal("品牌切换应删除旧 cache JSON")
	}
	reinitKeepingHooks()
	t.Setenv("FEISHU_CLI_META_TTL", "3600")
	Init()
	if _, ok := mergedServices["feishu_only_svc"]; ok {
		t.Fatal("下一进程仍不得信任旧品牌数据")
	}
}

func TestBrandChange_FailureDoesNotTrustOldCache(t *testing.T) {
	tmp := isolateRemote(t)
	t.Setenv("FEISHU_CLI_BRAND", brandLark)
	seedCache(t, tmp, "feishu_only_svc", "9.0.0", brandFeishu)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer ts.Close()
	testMetaURL = ts.URL
	Init()
	if _, ok := mergedServices["feishu_only_svc"]; ok {
		t.Fatal("品牌切换失败不得 overlay 旧品牌 cache")
	}
	reinitKeepingHooks()
	t.Setenv("FEISHU_CLI_META_TTL", "3600")
	Init()
	if _, ok := mergedServices["feishu_only_svc"]; ok {
		t.Fatal("失败后下一进程不得信任旧品牌数据")
	}
}

func TestMetaURLNonLoopbackRejected(t *testing.T) {
	isolateRemote(t)
	testMetaURL = "https://attacker.example/api_definition"
	_, _, err := fetchRemoteMerged("")
	if err == nil {
		t.Fatal("非 loopback 覆盖 URL 应失败")
	}
}

func TestRejectCrossOriginRedirect(t *testing.T) {
	isolateRemote(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://evil.example/meta", http.StatusFound)
	}))
	defer ts.Close()
	testMetaURL = ts.URL
	_, _, err := fetchRemoteMerged("")
	if err == nil {
		t.Fatal("跨 origin 重定向应失败")
	}
}

func TestRejectHTTPSToHTTPRedirect(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1/meta", nil)
	prev, _ := http.NewRequest(http.MethodGet, "https://open.feishu.cn/api/tools/open/api_definition", nil)
	if err := checkMetaRedirect(req, []*http.Request{prev}); err == nil {
		t.Fatal("HTTPS→HTTP 重定向应失败")
	}
}

func TestInvalidRemoteRegistryRejected(t *testing.T) {
	isolateRemote(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"msg":"succeeded","data":{"version":"","services":[{"name":"svc"}]}}`))
	}))
	defer ts.Close()
	testMetaURL = ts.URL
	_, _, err := fetchRemoteMerged("")
	if err == nil {
		t.Fatal("空 version 应拒绝")
	}
}

func TestCacheRootFailureDisablesCache(t *testing.T) {
	ResetForTest()
	t.Setenv("FEISHU_CLI_CONFIG_DIR", "")
	t.Setenv("FEISHU_CLI_REMOTE_META", "on")
	restore := profile.SetHomeFunc(func() (string, error) {
		return "", errors.New("no home")
	})
	t.Cleanup(restore)
	t.Cleanup(func() { ResetForTest() })
	if cacheWritable() {
		t.Fatal("profile root 失败时应禁用 cache，不得回退 /tmp")
	}
	Init()
	if strings.Contains(cacheDir(), os.TempDir()) && cacheDir() != "" {
		t.Fatalf("不得使用共享 /tmp cache，got %q", cacheDir())
	}
}

func TestNetworkError_DoesNotFailInit(t *testing.T) {
	tmp := isolateRemote(t)
	t.Setenv("FEISHU_CLI_META_TTL", "0")
	seedCache(t, tmp, "cached_svc", "9.0.0", brandFeishu)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer ts.Close()
	testMetaURL = ts.URL
	Init()
	if _, ok := mergedServices["cached_svc"]; !ok {
		t.Fatal("网络错误时应保留 cache overlay")
	}
	waitBackgroundRefresh()
}
