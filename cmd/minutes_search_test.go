package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

// startMinutesSearchServer 记录妙记搜索请求体
func startMinutesSearchServer(t *testing.T) (string, func() map[string]any, *int32) {
	t.Helper()
	var hits int32
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/open-apis/minutes/v1/minutes/search" {
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
			return
		}
		atomic.AddInt32(&hits, 1)
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"items":[],"has_more":false}}`)
	}))
	t.Cleanup(srv.Close)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	content := fmt.Sprintf("app_id: \"test_app_id\"\napp_secret: \"test_app_secret\"\nbase_url: \"%s\"\n", srv.URL)
	if err := os.WriteFile(cfg, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfg, func() map[string]any { return body }, &hits
}

// TestMinutesSearchFlagAlignment 新参数名（对齐官方）+ me 解析 + 旧参数名兼容。
func TestMinutesSearchFlagAlignment(t *testing.T) {
	orig := minutesCurrentUserOpenID
	minutesCurrentUserOpenID = func(*cobra.Command) (string, error) { return "ou_me", nil }
	t.Cleanup(func() { minutesCurrentUserOpenID = orig })

	t.Run("新参数名", func(t *testing.T) {
		isolateMsgTokenTestEnv(t)
		t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
		cfg, body, _ := startMinutesSearchServer(t)
		_, stderr, err := runCLI(t, "minutes", "search", "--keyword", "周会", "--owner-ids", "ou_a,me", "--participant-ids", "me,ou_b,me",
			"--start", "2026-03-01T00:00:00+08:00", "--end", "2026-03-02T00:00:00+08:00", "-o", "json", "--config", cfg)
		if err != nil {
			t.Fatalf("minutes search 失败: %v\n%s", err, stderr)
		}
		b := body()
		if b["query"] != "周会" || b["sorter"] != "create_time_desc" {
			t.Fatalf("body = %v", b)
		}
		filter, _ := b["filter"].(map[string]any)
		if fmt.Sprint(filter["owner_ids"]) != "[ou_a ou_me]" || fmt.Sprint(filter["participant_ids"]) != "[ou_me ou_b]" {
			t.Fatalf("filter = %v", filter)
		}
		ct, _ := filter["create_time"].(map[string]any)
		if ct["start_time"] != "2026-03-01T00:00:00+08:00" || ct["end_time"] != "2026-03-02T00:00:00+08:00" {
			t.Fatalf("create_time = %v", ct)
		}
	})

	t.Run("旧参数名兼容并提示废弃", func(t *testing.T) {
		isolateMsgTokenTestEnv(t)
		t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
		cfg, body, _ := startMinutesSearchServer(t)
		_, stderr, err := runCLI(t, "minutes", "search", "--owner-id", "ou_old", "--start-time", "2026-03-01T00:00:00+08:00", "-o", "json", "--config", cfg)
		if err != nil {
			t.Fatalf("旧参数应可用: %v", err)
		}
		filter, _ := body()["filter"].(map[string]any)
		if fmt.Sprint(filter["owner_ids"]) != "[ou_old]" {
			t.Fatalf("filter = %v", filter)
		}
		if !strings.Contains(stderr, "--owner-ids") || !strings.Contains(stderr, "--start") {
			t.Fatalf("应在 stderr 提示废弃，stderr=%q", stderr)
		}
	})

	t.Run("非法输入用法错误且不联网", func(t *testing.T) {
		isolateMsgTokenTestEnv(t)
		t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
		cfg, _, hits := startMinutesSearchServer(t)
		for _, args := range [][]string{
			{"minutes", "search", "--owner-ids", "u123"},
			{"minutes", "search", "--start", "2026-03-01", "--start-time", "2026-03-02"},
			// 新旧名同时指定不同值：与 --start/--start-time 一致报用法错误，不再静默合并
			{"minutes", "search", "--owner-ids", "ou_new", "--owner-id", "ou_old"},
			{"minutes", "search", "--query", "a", "--keyword", "b"},
			{"minutes", "search"},
		} {
			_, _, err := runCLI(t, append(args, "--config", cfg)...)
			if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
				t.Errorf("%v 应为用法错误，实际 %v", args, err)
			}
		}
		if n := atomic.LoadInt32(hits); n != 0 {
			t.Fatalf("用法错误不应联网，实际 %d 次", n)
		}
	})
}
