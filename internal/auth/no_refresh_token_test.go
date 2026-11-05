package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

func TestNoRefreshTokenSettingsURL(t *testing.T) {
	cases := []struct {
		name, baseURL, appID, want string
	}{
		{"飞书", "https://open.feishu.cn", "cli_abc", "https://open.feishu.cn/app/cli_abc/safe"},
		{"Lark", "https://open.larksuite.com", "cli_abc", "https://open.larksuite.com/app/cli_abc/safe"},
		{"自定义代理按飞书", "https://proxy.example.com", "cli_abc", "https://open.feishu.cn/app/cli_abc/safe"},
		{"空 app_id", "https://open.feishu.cn", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NoRefreshTokenSettingsURL(c.baseURL, c.appID); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
	if got := NoRefreshTokenSettingsURL("", "a/b?c"); strings.Contains(got, "?") || strings.Contains(got, "a/b") {
		t.Fatalf("app_id 必须转义: %q", got)
	}
}

func TestNoRefreshTokenFixNamesSettingAndPublish(t *testing.T) {
	fix := NoRefreshTokenFix("https://open.feishu.cn", "cli_abc")
	for _, want := range []string{"刷新 user_access_token", "发布", "https://open.feishu.cn/app/cli_abc/safe"} {
		if !strings.Contains(fix, want) {
			t.Fatalf("fix 缺少 %q: %s", want, fix)
		}
	}
	if strings.Contains(NoRefreshTokenFix("", ""), "http") {
		t.Fatal("app_id 未知时不应拼链接")
	}
	if !strings.Contains(NoRefreshTokenCause, "offline_access") || !strings.Contains(NoRefreshTokenCause, "刷新 user_access_token") {
		t.Fatalf("cause 文案需点明真因: %s", NoRefreshTokenCause)
	}
}

// 已过期 token 的两种形态：从未下发 refresh_token 给独立文案；refresh_token 也过期保持旧文案。退出码都按鉴权（3）。
func TestResolveExpiredTokenDistinguishesMissingRefresh(t *testing.T) {
	cases := []struct {
		name         string
		refreshToken string
		wantContains string
		wantAbsent   string
	}{
		{"从未下发", "", "未下发 refresh_token", "均已失效"},
		{"refresh 也过期", "r-old", "均已失效", "未下发"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
			_ = os.Unsetenv("FEISHU_USER_ACCESS_TOKEN")
			tmpDir := t.TempDir()
			tokenPathFunc = func() (string, error) { return filepath.Join(tmpDir, "token.json"), nil }
			defer func() { tokenPathFunc = originalTokenPath }()
			store := &TokenStore{
				AccessToken:      "u-old",
				RefreshToken:     c.refreshToken,
				ExpiresAt:        time.Now().Add(-time.Hour),
				RefreshExpiresAt: time.Now().Add(-time.Minute),
				AppID:            "cli_abc",
			}
			if err := SaveToken(store); err != nil {
				t.Fatal(err)
			}
			_, err := ResolveUserAccessToken("", "", "cli_abc", "sec", "https://open.feishu.cn")
			if err == nil {
				t.Fatal("期望报错")
			}
			if !strings.Contains(err.Error(), c.wantContains) || strings.Contains(err.Error(), c.wantAbsent) {
				t.Fatalf("文案不符: %v", err)
			}
			if c.refreshToken == "" && !strings.Contains(err.Error(), "https://open.feishu.cn/app/cli_abc/safe") {
				t.Fatalf("应带安全设置链接: %v", err)
			}
			kinds := clierr.Kinds(err)
			if len(kinds) == 0 || kinds[0] != clierr.KindAuth {
				t.Fatalf("应标记为鉴权错误: %v", kinds)
			}

			// 显式传入同一 access_token、以及 auth status --verify 共用的 EnsureFreshLocalToken 走同一分支
			if _, err := refreshIfStaleLocalToken("u-old", "cli_abc", "sec", "https://open.feishu.cn"); c.refreshToken == "" &&
				(err == nil || !strings.Contains(err.Error(), "未下发 refresh_token")) {
				t.Fatalf("显式 token 分支文案不符: %v", err)
			}
			if _, _, err := EnsureFreshLocalToken("cli_abc", "sec", "https://open.feishu.cn", store); c.refreshToken == "" &&
				(err == nil || !strings.Contains(err.Error(), "未下发 refresh_token")) {
				t.Fatalf("EnsureFreshLocalToken 文案不符: %v", err)
			}
		})
	}
}
