package cmd

import (
	"strings"
	"testing"
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
