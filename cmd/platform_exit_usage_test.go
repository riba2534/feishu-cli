package cmd

import (
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

// 平台命令组（auth/config/search/user/skills/update）的参数错误：退出码 2，且在缺凭据时也先报参数错误
// （本地校验先于 config.Validate 与身份解析）。错误文本保持原样。
func TestPlatformCommandsUsageExitWithoutCredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FEISHU_PROFILE", "")
	t.Setenv("FEISHU_APP_ID", "")
	t.Setenv("FEISHU_APP_SECRET", "")
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
	useRepoSkills(t)

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"search", "messages", "x", "--page-size", "99"}, "每页数量必须在 1–50 之间"},
		{[]string{"search", "messages", "x", "--start-time", "notadate"}, "无法解析时间"},
		{[]string{"search", "messages", "x", "--chat-type", "weird"}, "不支持的会话类型"},
		{[]string{"search", "messages", "x", "--format", "bad"}, "不支持的 --format"},
		{[]string{"search", "messages", "x", "--jq", "..[["}, "jq 表达式解析失败"},
		{[]string{"search", "messages", "x", "--as", "weird"}, "--as 仅支持 bot|user|auto"},
		{[]string{"search", "messages", "x", "--enrich", "--card-content-type", "bad"}, "无效的 --card-content-type"},
		{[]string{"search", "docs", "x", "--docs-types", "bad"}, "不支持的 --docs-types"},
		{[]string{"user", "info", "ou_x", "--as", "weird"}, "--as 仅支持 bot|user|auto"},
		{[]string{"user", "search"}, "至少需要指定 --email"},
		{[]string{"user", "search", "--query", "x", "--page-size", "99"}, "--page-size 范围 1-30"},
		{[]string{"user", "search", "--email", "a@example.com", "--query", "x"}, "互斥"},
		{[]string{"auth", "token", "--as", "weird"}, "--as 仅支持 user|bot|auto"},
		{[]string{"auth", "token", "--as", "bot", "--user-access-token", "u-x"}, "不能同时使用 --as bot"},
		{[]string{"auth", "login", "--exclude", "a"}, "--exclude 需要与"},
		{[]string{"auth", "login", "--device-code", "dc", "--scope", "x"}, "--device-code 模式下不能"},
		// StringSlice flag 在 runCLI 复位后不会清空，--domain 放在 login 用例最后
		{[]string{"auth", "login", "--domain", "nope", "--no-wait", "--json"}, "未知授权域"},
		{[]string{"config", "get", "nope_key"}, "未知的配置项"},
		{[]string{"skills", "read", "nope_skill"}, "未知技能"},
		{[]string{"skills", "list", "nope_skill"}, "未知技能"},
		{[]string{"skills", "read", "feishu-cli-docs/SKILL.md", "SKILL.md"}, "路径只能给一次"},
		{[]string{"update", "--check", "--dry-run"}, "--check 与 --dry-run 不能同时使用"},
		{[]string{"update", "--target", home + "/nope/feishu-cli", "--dry-run"}, "不存在或无法解析"},
	}
	for _, c := range cases {
		_, _, err := runCLI(t, c.args...)
		if err == nil {
			t.Fatalf("%v: 期望报错", c.args)
		}
		if code := exitCodeFor(err); code != clierr.ExitUsage {
			t.Fatalf("%v: exit=%d, want 2 (%v)", c.args, code, err)
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%v: 错误文本变化: %v", c.args, err)
		}
	}

	// 参数合法但缺凭据：仍是鉴权错误（退出码 3），没有被参数校验前移吞掉
	_, _, err := runCLI(t, "search", "docs", "x")
	if err == nil || exitCodeFor(err) != clierr.ExitAuth {
		t.Fatalf("合法参数 + 缺凭据应 exit 3: %v", err)
	}
	_, _, err = runCLI(t, "auth", "token", "--as", "bot")
	if err == nil || exitCodeFor(err) != clierr.ExitAuth {
		t.Fatalf("auth token 缺凭据应 exit 3: %v", err)
	}
}
