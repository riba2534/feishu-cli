package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/spf13/cobra"
)

// permAsFlagHelp 是 perm 命令组 --as 的说明：不传时保持历史默认（App/Bot 身份）。
const permAsFlagHelp = "身份: bot(App Token) | user(User Token) | auto(User 优先；未配置回退 Bot；已配置但不可用 fail-closed)。" +
	"不传时保持默认 Bot 身份（显式 --user-access-token 时用该 User Token）"

// addPermIdentityFlags 在 perm 命令组注册身份相关 persistent flag，所有子命令继承。
func addPermIdentityFlags(c *cobra.Command) {
	c.PersistentFlags().String("as", "", permAsFlagHelp)
	c.PersistentFlags().String("user-access-token", "", "User Access Token（显式指定时以该用户身份调用权限 API）")
}

// resolvePermIdentity 解析 perm 子命令的身份：
//   - 显式 --as：bot 强制 App Token；user 强制 User Token（缺失报错）；auto 为 User 优先、
//     未配置回退 Bot、已配置但不可用 fail-closed；
//   - 未传 --as：保持历史默认 Bot，仅显式 --user-access-token 时切换为该 User Token。
//     注意不读 FEISHU_USER_ACCESS_TOKEN 环境变量，避免环境变量静默改变权限操作的身份。
//
// 返回空字符串表示 App/Tenant Token。
func resolvePermIdentity(cmd *cobra.Command) (string, error) {
	return resolveIdentityWithLegacyDefault(cmd, func(c *cobra.Command) (string, error) {
		return resolveFlagUserToken(c), nil
	})
}

// wrapPermError 为权限 API 的典型身份错误追加可执行提示（保留原错误链，HasAPICode 仍可判定）。
func wrapPermError(err error, userAccessToken string) error {
	if err == nil {
		return nil
	}
	if client.HasAPICode(err, 1063002) && userAccessToken == "" {
		return fmt.Errorf("%w\n提示：当前以 Bot（App）身份调用，应用无权访问该文档。若文档属于你本人或你有权限，改用 --as user（需先 feishu-cli auth login）；或先把应用加为文档协作者", err)
	}
	if client.HasAPICode(err, 1063004) {
		if userAccessToken == "" {
			return fmt.Errorf("%w\n提示：当前以 Bot（App）身份调用，应用对该文档没有管理协作者/分享权限。"+
				"若文档属于你本人或你有管理权限，改用 --as user（需先 feishu-cli auth login）；或先把应用加为文档协作者", err)
		}
		return fmt.Errorf("%w\n提示：当前用户对该文档没有管理协作者/分享权限，请联系文档所有者授予「可管理」权限", err)
	}
	return err
}

// wrapPasswordError 为分享密码 API 追加提示。1063002 在密码创建/刷新场景下的常见原因不是身份，
// 而是链接未设为互联网公开（分享密码只对互联网公开链接生效），因此给出 perm public-update 的可执行命令。
func wrapPasswordError(err error, userAccessToken, docToken, docType string) error {
	if err == nil {
		return nil
	}
	if !client.HasAPICode(err, 1063002) {
		return wrapPermError(err, userAccessToken)
	}
	if docType == "" {
		docType = "docx"
	}
	return fmt.Errorf("%w\n提示：分享密码只对「互联网公开」的链接生效。请先（以与本次相同的身份参数）把链接设为互联网公开再设置密码：\n"+
		"  feishu-cli perm public-update %s --doc-type %s --external-access=true --link-share-entity anyone_readable\n"+
		"（需要互联网可编辑时用 anyone_editable）。若链接已是互联网公开仍报此错，再检查当前身份是否有该文档的管理权限", err, docToken, docType)
}
