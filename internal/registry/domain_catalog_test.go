package registry

import (
	"strings"
	"testing"
)

func containsScope(list []string, scope string) bool {
	for _, s := range list {
		if s == scope {
			return true
		}
	}
	return false
}

// TestDomainCatalogCoversCommandScopes 回归防护：本项目实际命令需要的 User scope 必须能通过
// auth login --domain <x>（含 --recommend）申请到。官方 lark-cli 远程 scopes.json 有 okr/apps/markdown
// 等域，本项目原先 --domain okr / --domain apps 报"未知授权域"，而对应命令需要这些 scope。
// 只依赖内嵌目录（不受运行时 overlay 缓存影响），保证全新安装也可用。
func TestDomainCatalogCoversCommandScopes(t *testing.T) {
	ResetForTest()
	t.Setenv("FEISHU_CLI_REMOTE_META", "off")
	t.Cleanup(ResetForTest)

	cases := map[string][]string{
		"okr": {
			"okr:okr.period:readonly", "okr:okr.content:readonly", "okr:okr.progress:readonly",
			"okr:okr.progress:writeonly", "okr:okr.progress:delete", "okr:okr.progress.file:upload",
		},
		"apps":     {"spark:app:read", "spark:app:write"},
		"markdown": {"drive:drive.metadata:readonly", "drive:file:download", "drive:file:upload"},
		"task":     {"task:attachment:write"},
		"drive":    {"docs:secure_label:readonly", "docs:secure_label:write_only", "docs:permission.member:apply"},
		"wiki":     {"wiki:space:write_only"},
		"minutes":  {"minutes:permission:apply"},
		"chat":     {"im:feed.flag:read", "im:feed.flag:write"},
		"im":       {"im:feed.flag:read", "im:feed.flag:write"},
	}
	known := KnownDomainNames()
	for domain, want := range cases {
		if !containsScope(known, domain) {
			t.Errorf("KnownDomainNames 应包含 %q: %v", domain, known)
		}
		if _, err := ParseDomains([]string{domain}); err != nil {
			t.Errorf("ParseDomains(%q) 不应报错: %v", domain, err)
		}
		for _, recommend := range []bool{false, true} {
			got := CollectDomainScopes([]string{domain}, recommend)
			for _, s := range want {
				if !containsScope(got, s) {
					t.Errorf("--domain %s (recommend=%v) 应包含 %s，实际: %v", domain, recommend, s, got)
				}
			}
		}
	}

	// --domain all：同样覆盖上述全部 scope
	all := CollectDomainScopes(known, true)
	for domain, want := range cases {
		for _, s := range want {
			if !containsScope(all, s) {
				t.Errorf("--recommend（全部域）应包含 %s（来自 %s 域）", s, domain)
			}
		}
	}
}

// TestBatchExcludesSendAsUser：批量申请一律剔除 im:message.send_as_user（不只是 --recommend），
// 但它仍属于 chat 域的合法范围（--exclude 校验用 DomainScopeUniverse）。
func TestBatchExcludesSendAsUser(t *testing.T) {
	ResetForTest()
	t.Setenv("FEISHU_CLI_REMOTE_META", "off")
	t.Cleanup(ResetForTest)

	for _, recommend := range []bool{false, true} {
		for _, domains := range [][]string{{"chat"}, KnownDomainNames()} {
			got := CollectDomainScopes(domains, recommend)
			if containsScope(got, "im:message.send_as_user") {
				t.Fatalf("domains=%s recommend=%v 不应包含 im:message.send_as_user", strings.Join(domains, ","), recommend)
			}
		}
	}
	if !containsScope(DomainScopeUniverse([]string{"chat"}), "im:message.send_as_user") {
		t.Fatal("DomainScopeUniverse(chat) 应包含 im:message.send_as_user，供 --exclude 校验")
	}
	if !IsBatchExcludedScope("im:message.send_as_user") || IsBatchExcludedScope("im:message") {
		t.Fatal("IsBatchExcludedScope 判定错误")
	}
}
