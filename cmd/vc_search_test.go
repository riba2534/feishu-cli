package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/v2/internal/clierr"
)

func writeStubConfig(t *testing.T, baseURL string) string {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfg, []byte(fmt.Sprintf("app_id: \"test_app_id\"\napp_secret: \"test_app_secret\"\nbase_url: \"%s\"\n", baseURL)), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfg
}

// TestVCSearchUsageErrorsExit2 参数校验错误为用法错误（exit 2），且先于身份解析，
// 未登录时也不会被鉴权错误（exit 3）遮住；校验阶段不发任何请求。
func TestVCSearchUsageErrorsExit2(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer srv.Close()
	cfg := writeStubConfig(t, srv.URL)

	var tooManyIDs []string
	for j := 0; j < 51; j++ {
		tooManyIDs = append(tooManyIDs, fmt.Sprintf("ou_%d", j))
	}
	cases := [][]string{
		{}, // 无任何过滤条件
		{"--query", strings.Repeat("长", 51)},
		{"--start", "not-a-time"},
		{"--start", "2026-03-02", "--end", "2026-03-01"},
		{"--query", "周会", "--page-size", "31"},
		{"--organizer-ids", strings.Join(tooManyIDs, ",")},
	}
	for _, extra := range cases {
		args := append([]string{"vc", "search", "--config", cfg}, extra...)
		_, _, err := runCLI(t, args...)
		if err == nil {
			t.Fatalf("%v 应报错", extra)
		}
		if !clierr.HasKind(err, clierr.KindUsage) || exitCodeFor(err) != 2 {
			t.Fatalf("%v 应为用法错误 exit 2，实际 exit %d: %v", extra, exitCodeFor(err), err)
		}
	}
	if hits != 0 {
		t.Fatalf("参数校验阶段不应发请求，实际 %d 次", hits)
	}
}

// TestVCSearchTextStripsHighlight 文本输出去掉 <h></h> 高亮标签；JSON 原样保留。
func TestVCSearchTextStripsHighlight(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"items":[{"id":"6911188411932033028","display_info":"<h>周会</h>复盘","meta_data":{"description":"2026-03-01 <h>周会</h>"}}],"has_more":false}}`)
	}))
	defer srv.Close()
	cfg := writeStubConfig(t, srv.URL)

	stdout, _, err := runCLI(t, "vc", "search", "--query", "周会", "--config", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout, "<h>") || strings.Contains(stdout, "</h>") || !strings.Contains(stdout, "[1] 周会复盘") || !strings.Contains(stdout, "2026-03-01 周会") {
		t.Fatalf("文本输出应去掉高亮标签:\n%s", stdout)
	}
	stdout, _, err = runCLI(t, "vc", "search", "--query", "周会", "-o", "json", "--config", cfg)
	if err != nil || !strings.Contains(stdout, `<h>周会</h>复盘`) {
		t.Fatalf("JSON 应保留服务端原文: err=%v\n%s", err, stdout)
	}
}
