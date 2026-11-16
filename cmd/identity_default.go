package cmd

import (
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// 默认身份 default_as（config.yaml 的 default_as / 环境变量 FEISHU_DEFAULT_AS），opt-in：
//
//   - auto（默认）：不改变任何命令的既有默认身份；
//   - user：支持本人身份的命令在未显式传 --as 时一律按 User 身份解析——User Token 来自
//     --user-access-token > FEISHU_USER_ACCESS_TOKEN > token.json（过期自动刷新）> config.yaml，
//     拿不到可用 Token（未登录、刷新失败、未绑定当前 App）时报错（退出码 3），不回落 Bot。
//
// 显式 --as bot|user|auto 始终优先于 default_as；只支持 Bot 的命令不读取 default_as。
// 各 resolve* helper（cmd/utils.go）在入口统一调用 defaultAsUserApplies 判定。

// ignoreDefaultAsAnnotation 标记不受 default_as=user 影响、保持自身默认身份的命令（含子命令）。
// 用于 User 身份能力明显弱于 Bot 的命令（例如 user info 的 User 身份只返回姓名类字段）。
const ignoreDefaultAsAnnotation = "feishu-cli/ignore-default-as"

// configuredDefaultAs 返回规范化后的 default_as；非法值按 auto 处理（config.Validate 会先以用法错误拦下）。
func configuredDefaultAs() string {
	v, err := config.NormalizeDefaultAs(config.Get().DefaultAs)
	if err != nil {
		return config.DefaultAsAuto
	}
	return v
}

// defaultAsUserApplies 报告本次调用是否应按 default_as=user 解析身份：
// 配置为 user、命令未显式传 --as、命令（及其父命令）未声明 ignoreDefaultAsAnnotation。
func defaultAsUserApplies(cmd *cobra.Command) bool {
	if configuredDefaultAs() != config.DefaultAsUser {
		return false
	}
	if cmd == nil {
		return true
	}
	if f := cmd.Flags().Lookup("as"); f != nil && f.Changed {
		return false
	}
	for c := cmd; c != nil; c = c.Parent() {
		if c.Annotations[ignoreDefaultAsAnnotation] == "1" {
			return false
		}
	}
	return true
}

// requireDefaultAsUserToken 按 default_as=user 解析 User Token；失败时 fail-closed，返回带修复提示的鉴权错误。
func requireDefaultAsUserToken(cmd *cobra.Command) (string, error) {
	token, err := resolveRequiredUserToken(cmd)
	if err != nil {
		return "", defaultAsUserError(err)
	}
	if config.Get().Debug {
		fmt.Fprintf(os.Stderr, "[Debug] default_as=user（%s）：使用 User Access Token\n", defaultAsSource())
	}
	return token, nil
}

// defaultAsUserError 包装 default_as=user 下拿不到 User Token 的错误：保留原错误链
// （网络错误仍按退出码 4 处理），其余标记为鉴权类（退出码 3）。
func defaultAsUserError(err error) error {
	return clierr.Auth(fmt.Errorf("已设置默认身份 default_as=user（%s），本命令需以本人身份执行，"+
		"但没有可用的 User Access Token；为避免以 Bot 身份执行，已停止：%w\n"+
		"修复：运行 `feishu-cli auth login` 登录或重新登录（过期、刷新失败时同样适用）；"+
		"确需以 Bot 身份执行本次命令，可追加 --as bot（命令支持 --as 时），或临时设置 %s=auto",
		defaultAsSource(), err, config.EnvDefaultAs))
}

// defaultAsSource 返回 default_as 的来源描述，用于提示与调试日志。
func defaultAsSource() string {
	if os.Getenv(config.EnvDefaultAs) != "" {
		return "来自环境变量 " + config.EnvDefaultAs
	}
	return "来自 config.yaml"
}

// effectiveIdentityLabel 返回不解析 token 的身份标签（dry-run 预览、续查命令提示用）：
// default_as=user 生效（未显式传 --as）时返回 "user"；否则返回 fallback（调用方传入自己原本的标签）。
func effectiveIdentityLabel(cmd *cobra.Command, fallback string) string {
	if defaultAsUserApplies(cmd) {
		return config.DefaultAsUser
	}
	return fallback
}

// defaultAsUserPreviewToken 是 default_as=user 时 --dry-run 预览使用的占位值，只用于在预览里展示 identity=user，
// 不会被发往服务端（dry-run 分支在任何网络请求之前返回）。
const defaultAsUserPreviewToken = "<default_as=user: auth login 登录态>"

// resolveOptionalUserTokenOrPreview 供在 --dry-run 判断之前就需要身份信息的写类命令使用：
//   - --dry-run：只做静态判断（显式 token 原样返回；default_as=user 返回占位值），不读 token.json、不刷新、不联网；
//   - 否则：等同 resolveOptionalUserToken。
func resolveOptionalUserTokenOrPreview(cmd *cobra.Command) (string, error) {
	if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
		if token := resolveExplicitUserToken(cmd); token != "" {
			return token, nil
		}
		if defaultAsUserApplies(cmd) {
			return defaultAsUserPreviewToken, nil
		}
		return "", nil
	}
	return resolveOptionalUserToken(cmd)
}
