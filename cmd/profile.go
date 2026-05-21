package cmd

import (
	"github.com/spf13/cobra"
)

// profileCmd 是 profile 多配置管理的顶层命令。
// 子命令：add / list / remove / rename / use / migrate / current
var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "管理多个 profile（如 work / personal）",
	Long: `管理 feishu-cli 多配置（profile），用于在多个飞书账号 / 应用之间快速切换。

每个 profile 是 ~/.feishu-cli/profiles/<name>/ 下的一个独立目录，包含：
  - config.yaml      profile 自己的 app_id / app_secret / base_url 等配置
  - token.json       该 profile 的 User Access Token / Refresh Token
  - user_profile.json 已登录用户信息缓存

通过 ~/.feishu-cli/active-profile 指针文件指向"当前 profile"，
所有 feishu-cli 命令（auth login / doc import / msg send 等）会自动
从 active profile 读 config + token，不需要任何额外参数。

向后兼容：
  - 没有任何 profile 时（profiles/ 目录不存在），仍然走旧布局
    ~/.feishu-cli/config.yaml 和 token.json，原有用户无感升级。
  - 第一次执行 'profile add' 不会自动迁移旧文件——用 'profile migrate'
    把现有 config.yaml + token.json 拷到 profiles/default/。

环境变量：
  FEISHU_PROFILE=<name>   临时强制使用指定 profile（不修改指针文件）

示例:
  feishu-cli profile add work --app-id cli_xxx --app-secret xxx --use
  feishu-cli profile list
  feishu-cli profile use personal
  feishu-cli profile use -                # 切回上一个 profile
  feishu-cli profile rename old new
  feishu-cli profile remove temp --force
  feishu-cli profile current
  feishu-cli profile migrate              # 旧布局 → profiles/default/
  FEISHU_PROFILE=work feishu-cli msg send ...`,
}

func init() {
	rootCmd.AddCommand(profileCmd)
}
