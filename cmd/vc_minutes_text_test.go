package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFormatMinuteDuration(t *testing.T) {
	cases := map[string]string{
		"3723000": "1h02m03s",
		"60000":   "1m00s",
		"59999":   "59s",
		"0":       "0s",
		"7200000": "2h00m00s",
		"abc":     "abc", // 无法解析原样返回
		"":        "",
	}
	for in, want := range cases {
		if got := formatMinuteDuration(in); got != want {
			t.Errorf("formatMinuteDuration(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestMinutesGetTextShowsReadableDuration 文本输出把毫秒时长转为人类可读；JSON 保持原值。
func TestMinutesGetTextShowsReadableDuration(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"minute":{"token":"`+testMinuteToken+`","title":"周会","duration":"3723000"}}}`)
	}))
	defer srv.Close()
	cfg := writeStubConfig(t, srv.URL)

	stdout, _, err := runCLI(t, "minutes", "get", testMinuteToken, "--config", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "时长:      1h02m03s") || strings.Contains(stdout, "3723000") {
		t.Fatalf("文本时长应为 1h02m03s:\n%s", stdout)
	}
	stdout, _, err = runCLI(t, "minutes", "get", testMinuteToken, "-o", "json", "--config", cfg)
	if err != nil || !strings.Contains(stdout, `"duration": "3723000"`) {
		t.Fatalf("JSON 应保留原始毫秒值: err=%v\n%s", err, stdout)
	}
}
