package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

func TestNormalizeMinuteTokenInput(t *testing.T) {
	ok := map[string]string{
		"obcnabc123": "obcnabc123",
		"https://example.feishu.cn/minutes/obcnabc123":                    "obcnabc123",
		"https://example.larkoffice.com/minutes/obsgh0000000000000000000": "obsgh0000000000000000000",
		"https://example.larksuite.com/minutes/obcnabc123?from=share":     "obcnabc123",
	}
	for in, want := range ok {
		got, err := normalizeMinuteTokenInput(in)
		if err != nil || got != want {
			t.Errorf("normalizeMinuteTokenInput(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{
		"https://evil.example.com/minutes/obcnabc123", // 主机不在白名单
		"https://example.feishu.cn/docx/obcnabc123",   // 不是妙记链接
		"obcn-abc/123", // token 含非法字符
		"ab",           // 过短
	} {
		_, err := normalizeMinuteTokenInput(in)
		if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
			t.Errorf("%q 应为用法错误，实际 %v", in, err)
		}
	}
}

func TestParseMinuteTokenListDedupesURLAndToken(t *testing.T) {
	got, err := parseMinuteTokenList("https://example.feishu.cn/minutes/obcnabc123, obcnabc123,obcndef456", "minute-tokens")
	if err != nil || strings.Join(got, ",") != "obcnabc123,obcndef456" {
		t.Fatalf("got %v, %v", got, err)
	}
}

// TestMinutesGetAcceptsMinuteURL minutes get 接受妙记链接（以前报"minute_token 含非法字符"exit 1）。
func TestMinutesGetAcceptsMinuteURL(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
	cfg, _ := startMinutesServer(t, false)
	stdout, stderr, err := runCLI(t, "minutes", "get", "https://example.larkoffice.com/minutes/"+testMinuteToken+"?from=im", "-o", "json", "--config", cfg)
	if err != nil {
		t.Fatalf("妙记链接应可用: %v\nstderr=%s", err, stderr)
	}
	var out struct {
		Minute struct {
			Minute struct {
				Token string `json:"token"`
			} `json:"minute"`
		} `json:"minute"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil || out.Minute.Minute.Token != testMinuteToken {
		t.Fatalf("输出 = %s, err=%v", stdout, err)
	}
}

// TestMinuteInputUsageErrorsExit2 非法 minute_token / 链接以及批量参数错误均为用法错误（exit 2），
// 且先于身份解析——未登录时不会被鉴权错误（exit 3）遮住。
func TestMinuteInputUsageErrorsExit2(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	cfg := writeStubConfig(t, "http://127.0.0.1:9") // 校验阶段不应联网
	var many []string
	for i := 0; i < 51; i++ {
		many = append(many, fmt.Sprintf("obcnabc%03d", i))
	}
	cases := [][]string{
		{"minutes", "get", "https://evil.example.com/minutes/obcnabc123"},
		{"minutes", "get", "obcn-bad/token"},
		{"minutes", "download", "--minute-tokens", "https://example.feishu.cn/docx/obcnabc123"},
		{"minutes", "download", "--minute-tokens", strings.Join(many, ",")},
		{"minutes", "apply-permission", "--minute-token", "bad!", "--perm", "view"},
		{"minutes", "apply-permission", "--minute-token", "obcnabc123", "--perm", "admin"},
		{"vc", "notes", "--minute-tokens", "bad!"},
		{"vc", "notes", "--meeting-ids", "1", "--minute-tokens", "obcnabc123"},
		{"vc", "notes"},
		{"vc", "recording", "--meeting-ids", "1", "--calendar-event-ids", "e1"},
		{"vc", "recording"},
	}
	for _, args := range cases {
		_, _, err := runCLI(t, append(args, "--config", cfg)...)
		if err == nil || exitCodeFor(err) != 2 {
			code := -1
			if err != nil {
				code = exitCodeFor(err)
			}
			t.Errorf("%v 应为用法错误 exit 2，实际 exit %d: %v", args, code, err)
		}
	}
}
