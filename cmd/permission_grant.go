package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
)

// permissionGrantStderr 是自动授权告警的输出位置（测试可替换）。
var permissionGrantStderr io.Writer = os.Stderr

// currentCLIUserOpenIDFunc 解析当前 CLI 登录用户的 open_id（测试可替换）。
var currentCLIUserOpenIDFunc = currentCLIUserOpenID

// grantFullAccessFunc 执行授权调用（测试可替换）。
var grantFullAccessFunc = client.GrantFullAccessToOpenID

// currentCLIUserOpenID 返回当前 CLI 登录用户的 open_id；拿不到时返回空串与原因。
//
// 不强制登录：只复用已有的 User Token 来源（token.json / 环境变量 / config），未配置时直接跳过。
// 优先命中 user_profile.json 缓存（与 token 指纹匹配），缓存失效时回源 /authen/v1/user_info 一次并写回缓存。
// open_id 按应用隔离：ResolveUserAccessToken 会校验 token.json 绑定的 app_id 与当前应用一致。
func currentCLIUserOpenID() (string, string) {
	cfg := config.Get()
	userToken, err := auth.ResolveUserAccessToken("", cfg.UserAccessToken, cfg.AppID, cfg.AppSecret, cfg.BaseURL)
	if err != nil {
		if auth.IsNoUserTokenConfigured(err) {
			return "", "未找到当前 CLI 登录用户（未登录）"
		}
		return "", "已配置的 User Token 无法使用: " + compactGrantError(err)
	}
	openID, err := resolveCurrentUserOpenID(userToken)
	if err != nil {
		return "", "获取当前用户 open_id 失败: " + compactGrantError(err)
	}
	return openID, ""
}

func compactGrantError(err error) string {
	if err == nil {
		return ""
	}
	return strings.Join(strings.Fields(err.Error()), " ")
}

// autoGrantCurrentUser 在资源以 Bot 身份创建后，自动把 full_access 授予当前 CLI 登录用户，
// 避免出现「Bot 建的文档用户自己打不开」。对齐官方 lark-cli AutoGrantCurrentUserDrivePermission。
//
//   - usedUserToken 非空表示操作以 User 身份执行（资源本就属于用户），不触发，返回 nil；
//   - 调用方须在 --dry-run 分支之后调用，dry-run 不触发；
//   - 授权失败或无登录用户只在 stderr 告警，绝不让主操作失败；
//   - 返回值写入 JSON 输出的 permission_grant 字段。
func autoGrantCurrentUser(usedUserToken, token, resourceType string) *client.PermissionGrantResult {
	if strings.TrimSpace(usedUserToken) != "" {
		return nil
	}
	token = strings.TrimSpace(token)
	resourceType = client.NormalizeResourceType(resourceType)
	label := client.PermissionTargetLabel(resourceType)
	if token == "" || resourceType == "" {
		res := &client.PermissionGrantResult{
			Status:  client.PermissionGrantSkipped,
			Perm:    client.PermissionGrantPerm,
			Message: "操作未返回可授权的资源 token 或类型，未为当前用户授予 full_access；可继续以 Bot 身份使用，或稍后手动授权",
			Hint:    "可用 feishu-cli perm add <token> --doc-type <type> --member-type openid --member-id <open_id> --perm full_access 手动授权",
		}
		fmt.Fprintf(permissionGrantStderr, "⚠️  %s\n", res.Message)
		return res
	}

	openID, reason := currentCLIUserOpenIDFunc()
	if openID == "" {
		res := &client.PermissionGrantResult{
			Status:  client.PermissionGrantSkipped,
			Perm:    client.PermissionGrantPerm,
			Message: fmt.Sprintf("%s以 Bot 身份创建，但%s，未自动授予 full_access；可继续以 Bot 身份使用，或稍后手动授权", label, reason),
			Hint: fmt.Sprintf("执行 feishu-cli auth login 后重新创建，或手动授权: feishu-cli perm add %s --doc-type %s --member-type email --member-id <邮箱> --perm full_access",
				token, resourceType),
		}
		fmt.Fprintf(permissionGrantStderr, "⚠️  %s\n", res.Message)
		return res
	}

	if err := grantFullAccessFunc(token, resourceType, openID); err != nil {
		errMsg := compactGrantError(err)
		res := &client.PermissionGrantResult{
			Status:     client.PermissionGrantFailed,
			Perm:       client.PermissionGrantPerm,
			UserOpenID: openID,
			MemberType: "openid",
			Message:    fmt.Sprintf("%s已创建，但为当前用户授予 full_access 失败: %s", label, errMsg),
			Hint: fmt.Sprintf("应用可能缺少协作者管理权限（如 docs:permission.member:create），或资源限制了权限变更；可稍后重试，或手动授权: feishu-cli perm add %s --doc-type %s --member-type openid --member-id %s --perm full_access",
				token, resourceType, openID),
		}
		var grantErr *client.PermissionGrantError
		if errors.As(err, &grantErr) {
			res.LarkCode = grantErr.Code
		}
		fmt.Fprintf(permissionGrantStderr, "⚠️  %s\n", res.Message)
		return res
	}

	return &client.PermissionGrantResult{
		Status:     client.PermissionGrantGranted,
		Perm:       client.PermissionGrantPerm,
		UserOpenID: openID,
		MemberType: "openid",
		Message:    fmt.Sprintf("已为当前 CLI 用户授予新建%s的 full_access 权限", label),
	}
}

// printPermissionGrantText 在文本输出模式下追加一行授权结果（skipped/failed 的详情已在 stderr 告警）。
func printPermissionGrantText(w io.Writer, grant *client.PermissionGrantResult) {
	if grant == nil {
		return
	}
	switch grant.Status {
	case client.PermissionGrantGranted:
		fmt.Fprintf(w, "  当前用户权限: 已授予 full_access（%s）\n", grant.UserOpenID)
	case client.PermissionGrantSkipped:
		fmt.Fprintf(w, "  当前用户权限: 未授予（已跳过，见上方提示）\n")
	case client.PermissionGrantFailed:
		fmt.Fprintf(w, "  当前用户权限: 授予失败（见上方提示）\n")
	}
}

// withPermissionGrant 把授权结果写入 JSON 输出 map（grant 为 nil 时不写）。
func withPermissionGrant(out map[string]any, grant *client.PermissionGrantResult) map[string]any {
	if grant != nil {
		out["permission_grant"] = grant
	}
	return out
}
