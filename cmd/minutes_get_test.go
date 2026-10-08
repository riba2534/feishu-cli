package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

const testMinuteToken = "obcnxxxxxx"

// startMinutesServer 假妙记服务：minute 基础信息 + artifacts（含 22KB 级逐字稿的缩小版）。
// noPerm=true 时 minute 接口返回 HTTP 403 + 2091005（真实服务端形态）。
func startMinutesServer(t *testing.T, noPerm bool) (cfgPath string, artifactHits *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/open-apis/minutes/v1/minutes/" + testMinuteToken:
			if noPerm {
				w.WriteHeader(http.StatusForbidden)
				_, _ = fmt.Fprint(w, `{"code":2091005,"msg":"permission deny","error":{"log_id":"20260101000000ABC"}}`)
				return
			}
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"minute":{"token":"`+testMinuteToken+`","title":"周会/复盘","duration":"60000"}}}`)
		case "/open-apis/minutes/v1/minutes/" + testMinuteToken + "/artifacts":
			atomic.AddInt32(&hits, 1)
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"summary":"摘要内容","minute_todos":[{"content":"todo1"}],`+
				`"minute_chapters":[{"title":"c1"}],"keywords":["k1","k2"],"transcript":"说话人1 00:00\n逐字稿正文"}}`)
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	cfgPath = filepath.Join(t.TempDir(), "config.yaml")
	content := fmt.Sprintf("app_id: \"test_app_id\"\napp_secret: \"test_app_secret\"\nbase_url: \"%s\"\n", srv.URL)
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfgPath, &hits
}

// TestMinutesGetSelectiveArtifacts #16：只返回选中的 AI 产物，不内联逐字稿。
func TestMinutesGetSelectiveArtifacts(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
	cfg, _ := startMinutesServer(t, false)
	stdout, stderr, err := runCLI(t, "minutes", "get", testMinuteToken, "--summary", "--keyword", "-o", "json", "--config", cfg)
	if err != nil {
		t.Fatalf("minutes get 失败: %v\nstderr=%s", err, stderr)
	}
	var out struct {
		Artifacts map[string]any `json:"artifacts"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("输出不是 JSON: %v\n%s", err, stdout)
	}
	if out.Artifacts["summary"] != "摘要内容" {
		t.Fatalf("summary = %v", out.Artifacts["summary"])
	}
	if _, ok := out.Artifacts["keywords"]; !ok {
		t.Fatalf("缺少 keywords: %v", out.Artifacts)
	}
	for _, k := range []string{"minute_todos", "minute_chapters", "transcript", "transcript_file"} {
		if _, ok := out.Artifacts[k]; ok {
			t.Fatalf("未选择的 %s 不应出现: %v", k, out.Artifacts)
		}
	}
	if strings.Contains(stdout, "逐字稿正文") {
		t.Fatalf("逐字稿不应内联到输出")
	}
}

// TestMinutesGetWithArtifactsWritesTranscriptFile 旧 --with-artifacts 等价于全选，逐字稿写文件、输出只给路径。
func TestMinutesGetWithArtifactsWritesTranscriptFile(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
	cfg, _ := startMinutesServer(t, false)
	dir := t.TempDir()
	stdout, stderr, err := runCLI(t, "minutes", "get", testMinuteToken, "--with-artifacts", "--output-dir", dir, "-o", "json", "--config", cfg)
	if err != nil {
		t.Fatalf("minutes get 失败: %v\nstderr=%s", err, stderr)
	}
	var out struct {
		Artifacts map[string]any `json:"artifacts"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("输出不是 JSON: %v\n%s", err, stdout)
	}
	for _, k := range []string{"summary", "minute_todos", "minute_chapters", "keywords", "transcript_file"} {
		if _, ok := out.Artifacts[k]; !ok {
			t.Fatalf("--with-artifacts 应包含 %s: %v", k, out.Artifacts)
		}
	}
	if _, ok := out.Artifacts["transcript"]; ok || strings.Contains(stdout, "逐字稿正文") {
		t.Fatalf("逐字稿不应内联: %s", stdout)
	}
	path, _ := out.Artifacts["transcript_file"].(string)
	if !strings.HasPrefix(path, dir) || !strings.HasSuffix(path, filepath.Join("artifact-周会_复盘-"+testMinuteToken, "transcript.txt")) {
		t.Fatalf("transcript_file = %q", path)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "说话人1 00:00\n逐字稿正文" {
		t.Fatalf("逐字稿文件内容 = %q, %v", b, err)
	}
}

// TestMinutesGetNoArtifactsByDefault 不选择 AI 产物时不请求 artifacts。
func TestMinutesGetNoArtifactsByDefault(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
	cfg, hits := startMinutesServer(t, false)
	if _, stderr, err := runCLI(t, "minutes", "get", testMinuteToken, "--config", cfg); err != nil {
		t.Fatalf("minutes get 失败: %v\nstderr=%s", err, stderr)
	}
	if n := atomic.LoadInt32(hits); n != 0 {
		t.Fatalf("未选择 AI 产物不应请求 artifacts，实际 %d 次", n)
	}
}

// TestMinutesGetNoPermissionHint 2091005（随 HTTP 403 下发）提示用 minutes apply-permission。
func TestMinutesGetNoPermissionHint(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
	cfg, _ := startMinutesServer(t, true)
	_, _, err := runCLI(t, "minutes", "get", testMinuteToken, "--config", cfg)
	if err == nil || !strings.Contains(err.Error(), "minutes apply-permission --minute-token "+testMinuteToken+" --perm view") {
		t.Fatalf("应提示 apply-permission，实际: %v", err)
	}
	if !clierr.HasKind(err, clierr.KindAuth) {
		t.Fatalf("无权限应为鉴权/权限错误（exit 3），kinds=%v", clierr.Kinds(err))
	}
}
