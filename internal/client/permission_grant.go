package client

import (
	"fmt"
	"net/http"
	"strings"
)

// Bot 创建资源后自动授权当前用户的结果状态（字段形状对齐官方 lark-cli permission_grant）。
const (
	PermissionGrantGranted = "granted"
	PermissionGrantSkipped = "skipped"
	PermissionGrantFailed  = "failed"
	// PermissionGrantPerm 自动授予的权限级别。
	PermissionGrantPerm = "full_access"
)

// PermissionGrantResult 是自动授权的结果，作为创建类命令 JSON 输出的 permission_grant 字段。
type PermissionGrantResult struct {
	Status     string `json:"status"`                 // granted / skipped / failed
	Perm       string `json:"perm"`                   // 固定 full_access
	Message    string `json:"message"`                // 面向用户的结果说明
	UserOpenID string `json:"user_open_id,omitempty"` // 被授权的当前用户 open_id
	MemberType string `json:"member_type,omitempty"`  // 固定 openid（有 open_id 时）
	Hint       string `json:"hint,omitempty"`         // skipped / failed 时的后续操作建议
	LarkCode   int    `json:"lark_code,omitempty"`    // failed 时的飞书业务错误码
}

// PermissionGrantError 是授权接口返回的业务错误。
type PermissionGrantError struct {
	Code  int
	Msg   string
	LogID string
}

func (e *PermissionGrantError) Error() string {
	s := fmt.Sprintf("授予权限失败: code=%d, msg=%s", e.Code, e.Msg)
	if e.LogID != "" {
		s += ", log_id=" + e.LogID
	}
	return s
}

// grantPermType 返回协作者接口的 perm_type：wiki 节点授予「容器」权限（含子页面），其他类型不传。
func grantPermType(resourceType string) string {
	if resourceType == ResourceTypeWiki {
		return "container"
	}
	return ""
}

// GrantFullAccessToOpenID 以 App（Bot）身份把资源的 full_access 授予指定 open_id 用户，不发送通知。
//
// POST /open-apis/drive/v1/permissions/:token/members?type=<resourceType>&need_notification=false
// body: {member_type: openid, member_id, perm: full_access, type: user[, perm_type: container（wiki）]}
// resourceType 取协作者接口的 type 枚举：doc/docx/sheet/bitable/file/folder/wiki/mindnote/slides。
func GrantFullAccessToOpenID(token, resourceType, openID string) error {
	token = strings.TrimSpace(token)
	resourceType = NormalizeResourceType(resourceType)
	openID = strings.TrimSpace(openID)
	if !IsSafeResourceToken(token) {
		return fmt.Errorf("授予权限失败: 资源 token 无效: %q", token)
	}
	if resourceType == "" || openID == "" {
		return fmt.Errorf("授予权限失败: 资源类型与用户 open_id 不能为空")
	}
	body := map[string]any{
		"member_type": "openid",
		"member_id":   openID,
		"perm":        PermissionGrantPerm,
		"type":        "user",
	}
	if permType := grantPermType(resourceType); permType != "" {
		body["perm_type"] = permType
	}
	env, logID, err := callOpenAPIJSON(http.MethodPost, "/open-apis/drive/v1/permissions/:token/members",
		map[string]string{"token": token},
		map[string]string{"type": resourceType, "need_notification": "false"},
		body, "")
	if err != nil {
		return fmt.Errorf("授予权限失败: %w", err)
	}
	if env.Code != 0 {
		return &PermissionGrantError{Code: env.Code, Msg: env.Msg, LogID: logID}
	}
	return nil
}

// PermissionTargetLabel 返回资源类型的中文名称，用于授权结果文案。
func PermissionTargetLabel(resourceType string) string {
	switch NormalizeResourceType(resourceType) {
	case ResourceTypeWiki:
		return "知识库节点"
	case ResourceTypeDoc, ResourceTypeDocx:
		return "文档"
	case ResourceTypeSheet:
		return "电子表格"
	case ResourceTypeBitable:
		return "多维表格"
	case ResourceTypeSlides:
		return "演示文稿"
	case ResourceTypeFile:
		return "文件"
	case ResourceTypeFolder:
		return "文件夹"
	case ResourceTypeMindnote:
		return "思维笔记"
	default:
		return "资源"
	}
}
