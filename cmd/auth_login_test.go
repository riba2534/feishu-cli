package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/config"
)

// TestFormatUserCode 测试 Device Flow 用户码格式化
func TestFormatUserCode(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"ABCD1234", "ABCD-1234"},
		{"ABCD-1234", "ABCD-1234"},
		{"ABC", "ABC"},
		{"ABCDEFGHIJ", "ABCDEFGHIJ"},
	}
	for _, c := range cases {
		got := formatUserCode(c.input)
		if got != c.want {
			t.Errorf("formatUserCode(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestResolveRequestedScope(t *testing.T) {
	cases := []struct {
		name string
		opts loginScopeOptions
		want string
	}{
		{
			name: "explicit scope adds core scope",
			opts: loginScopeOptions{Scope: "minutes:minutes.basic:read minutes:minute:download"},
			want: "auth:user.id:read minutes:minutes.basic:read minutes:minute:download",
		},
		{
			// 服务端把 "a,b" 当成一个非法 scope：逗号必须被拆开并以空格上线
			name: "comma separated scope is split and deduplicated",
			opts: loginScopeOptions{Scope: "search:docs:read,im:message:readonly, search:docs:read\tvc:note:read"},
			want: "auth:user.id:read search:docs:read im:message:readonly vc:note:read",
		},
		{
			name: "domain all (no recommend)",
			opts: loginScopeOptions{Domains: []string{"search"}},
			want: "auth:user.id:read search:docs:read search:message",
		},
		{
			// --scope 与 --domain 叠加取并集（官方语义），不再互斥
			name: "scope and domain are merged",
			opts: loginScopeOptions{Scope: "vc:note:read,search:message", Domains: []string{"search"}, Recommend: true},
			want: "auth:user.id:read search:docs:read search:message vc:note:read",
		},
		{
			name: "exclude removes scope from domain set",
			opts: loginScopeOptions{Domains: []string{"search"}, Exclude: []string{"search:message"}},
			want: "auth:user.id:read search:docs:read",
		},
		{
			name: "exclude accepts comma separated values",
			opts: loginScopeOptions{Scope: "a:b:c d:e:f g:h:i", Exclude: []string{"a:b:c,g:h:i"}},
			want: "auth:user.id:read d:e:f",
		},
	}

	for _, c := range cases {
		got, err := resolveRequestedScope(c.opts, true)
		if err != nil {
			t.Errorf("%s: resolveRequestedScope() error = %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: resolveRequestedScope(%+v) = %q, want %q", c.name, c.opts, got, c.want)
		}
	}
}

func TestResolveRequestedScopeBatchExcludesSendAsUser(t *testing.T) {
	// 批量申请（--domain）一律剔除 im:message.send_as_user，不论是否 --recommend
	for _, recommend := range []bool{false, true} {
		got, err := resolveRequestedScope(loginScopeOptions{Domains: []string{"chat"}, Recommend: recommend}, true)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(" "+got+" ", " im:message.send_as_user ") {
			t.Fatalf("recommend=%v: 批量申请不应包含 im:message.send_as_user: %s", recommend, got)
		}
		if !strings.Contains(got, "im:message:readonly") {
			t.Fatalf("recommend=%v: chat 域应包含 im:message:readonly: %s", recommend, got)
		}
	}

	// 显式 --scope 可以加回
	got, err := resolveRequestedScope(loginScopeOptions{Domains: []string{"chat"}, Scope: "im:message.send_as_user"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "im:message.send_as_user") {
		t.Fatalf("显式 --scope 应保留 im:message.send_as_user: %s", got)
	}

	// 排除一个已被批量策略剔除的 scope 是合法空操作
	if _, err := resolveRequestedScope(loginScopeOptions{Domains: []string{"chat"}, Recommend: true, Exclude: []string{"im:message.send_as_user"}}, true); err != nil {
		t.Fatalf("--exclude 被批量剔除的 scope 应为空操作: %v", err)
	}
}

func TestResolveRequestedScopeUsageErrors(t *testing.T) {
	cases := []struct {
		name string
		opts loginScopeOptions
		want string
	}{
		{"exclude without scope source", loginScopeOptions{Exclude: []string{"search:message"}}, "--exclude"},
		{"exclude typo", loginScopeOptions{Domains: []string{"search"}, Exclude: []string{"search:mesage"}}, "search:mesage"},
		{"exclude everything", loginScopeOptions{Scope: "a:b:c", Exclude: []string{"a:b:c"}}, "没有剩余"},
		{"unknown domain", loginScopeOptions{Domains: []string{"nope"}}, "未知授权域"},
		{"no scope in non-interactive", loginScopeOptions{}, "--scope"},
	}
	for _, c := range cases {
		_, err := resolveRequestedScope(c.opts, true)
		if err == nil {
			t.Fatalf("%s: 应报错", c.name)
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%s: 错误应包含 %q: %v", c.name, c.want, err)
		}
		if got := exitCodeFor(err); got != 2 {
			t.Fatalf("%s: 用法错误应 exit 2，得到 %d (%v)", c.name, got, err)
		}
	}
}

// --device-code 续轮询拿不到首次的 expires_in：本地上限必须足够长（600s），由服务端 expired_token 决定结束。
func TestRunDeviceFlowResumeUses600sBudget(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var gotExpires, gotInterval int
	var gotDeviceCode string
	orig := pollDeviceTokenFn
	pollDeviceTokenFn = func(appID, appSecret, baseURL, deviceCode string, interval, expiresIn int, onTick func(int, int)) (*auth.TokenStore, error) {
		gotDeviceCode, gotInterval, gotExpires = deviceCode, interval, expiresIn
		return nil, errors.New("stop")
	}
	t.Cleanup(func() { pollDeviceTokenFn = orig })

	cfg := &config.Config{AppID: "cli_app", AppSecret: "sec", BaseURL: "https://open.feishu.cn"}
	err := runDeviceFlow(cfg, true, "dc_resume", false, loginScopeOptions{})
	if err == nil || err.Error() != "stop" {
		t.Fatalf("应透传轮询错误: %v", err)
	}
	if gotDeviceCode != "dc_resume" || gotInterval != 5 || gotExpires != 600 {
		t.Fatalf("续轮询参数 device_code=%q interval=%d expires=%d，期望 600s 上限", gotDeviceCode, gotInterval, gotExpires)
	}

	// --device-code 与任何授权范围参数同用是用法错误
	err = runDeviceFlow(cfg, true, "dc_resume", false, loginScopeOptions{Exclude: []string{"a:b:c"}})
	if err == nil || exitCodeFor(err) != 2 {
		t.Fatalf("--device-code + --exclude 应为用法错误: %v", err)
	}
}
