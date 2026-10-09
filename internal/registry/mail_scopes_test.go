package registry

import "testing"

// TestMailDomainRecommendIncludesRuleScopes：mail rule-list/rule-get 需要 mail:user_mailbox.rule:read，
// rule-create/update/delete/reorder 需要 mail:user_mailbox.rule:write；
// auth login --domain mail --recommend 必须能一次申请到，否则收信规则命令登录后仍报缺 scope。
func TestMailDomainRecommendIncludesRuleScopes(t *testing.T) {
	ResetForTest()
	t.Setenv("FEISHU_CLI_REMOTE_META", "off")
	t.Cleanup(ResetForTest)

	want := []string{
		"mail:user_mailbox.rule:read",
		"mail:user_mailbox.rule:write",
		// 既有 mail 推荐 scope 不受影响
		"mail:user_mailbox.message:readonly",
		"mail:user_mailbox.message:send",
		"mail:user_mailbox.folder:read",
	}
	for _, recommend := range []bool{false, true} {
		got := CollectDomainScopes([]string{"mail"}, recommend)
		for _, s := range want {
			if !containsScope(got, s) {
				t.Errorf("--domain mail (recommend=%v) 应包含 %s，实际: %v", recommend, s, got)
			}
		}
	}
	if all := CollectDomainScopes(KnownDomainNames(), true); !containsScope(all, "mail:user_mailbox.rule:write") {
		t.Errorf("--recommend（全部域）应包含 mail:user_mailbox.rule:write")
	}
}
