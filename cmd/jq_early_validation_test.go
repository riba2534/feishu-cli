package cmd

import (
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

// --jq 非法表达式在读配置与凭据之前报用法错误（exit 2），覆盖所有注册了 gojq --jq 的命令组。
func TestJQValidatedBeforeCredentials(t *testing.T) {
	withCmdHome(t)
	for _, args := range [][]string{
		{"apps", "list", "--jq", "bad("},
		{"bitable", "table", "list", "--base-token", "bascnx", "--jq", "bad("},
		{"api", "GET", "/open-apis/x", "--jq", "bad("},
	} {
		_, _, err := runCLI(t, args...)
		if exitCodeFor(err) != clierr.ExitUsage {
			t.Fatalf("%v 应为用法错误 exit 2，got %v", args, err)
		}
	}
	// event consume 的 --jq 是点路径语法，不受 gojq 校验影响
	if _, _, err := runCLI(t, "event", "consume", "im.message.receive_v1", "--jq", ".event", "--dry-run"); err != nil {
		t.Fatalf("event consume 点路径 --jq 不应被拒绝: %v", err)
	}
}
