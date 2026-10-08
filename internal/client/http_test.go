package client

import (
	"strings"
	"testing"
)

// TestRawUserTokenRequestsUseControlledClient 回归：msg list / chat_p2p / 用户搜索等手写 Bearer 请求
// 过去用 http.DefaultClient（无超时、绕过 host 白名单），base_url 被改成非官方 host 时会把
// User Token 直接发过去。现在统一走受控客户端，未 opt-in 的自定义 host 在发出前即被拒绝。
func TestRawUserTokenRequestsUseControlledClient(t *testing.T) {
	t.Setenv("FEISHU_BASE_URL", "")
	t.Setenv("FEISHU_ALLOW_CUSTOM_BASE_URL", "")
	setupTestConfig(t, "https://open.evil-example.invalid")

	calls := map[string]func() error{
		"BatchGetUsersBasic": func() error { _, err := BatchGetUsersBasic([]string{"ou_x"}, "u-test"); return err },
		"SearchUsers":        func() error { _, err := SearchUsers("x", 10, "", "u-test"); return err },
		"getMessage":         func() error { _, err := getMessageWithUserToken("om_x", "u-test", ""); return err },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			err := call()
			if err == nil || !strings.Contains(err.Error(), "拒绝自定义远端 host") {
				t.Fatalf("应被 host 白名单拒绝（未发出带 User Token 的请求），得到 %v", err)
			}
		})
	}
}
