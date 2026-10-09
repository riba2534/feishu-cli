package cmd

import (
	"fmt"
	"os"
	"testing"
)

// TestMain 让 cmd 包的全部测试与本机真实环境隔离：家目录指向临时目录（token.json、profile、
// 技能安装目录都不会读写真实的 ~/.feishu-cli、~/.claude），并清掉会影响身份解析的环境变量。
// 否则本机已登录时，读类命令会把真实 User Token 发给测试服务器；token 过期时还会向测试服务器发起
// 刷新——结果随本机登录状态变化，CI（无登录态）复现不了。
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "feishu-cli-cmd-test-home-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建测试家目录失败: %v\n", err)
		os.Exit(1)
	}
	for _, k := range []string{"HOME", "USERPROFILE"} {
		_ = os.Setenv(k, home)
	}
	for _, k := range []string{"FEISHU_USER_ACCESS_TOKEN", "FEISHU_PROFILE", "FEISHU_APP_ID", "FEISHU_APP_SECRET", "FEISHU_CLI_SKILLS_DIR"} {
		_ = os.Unsetenv(k)
	}
	// 只改环境变量、不替换 profile.homeFunc：个别测试会再用 t.Setenv("HOME", ...) 指定自己的家目录
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}
