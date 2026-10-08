package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// DriveQueryByTokenPath 按 token 识别云空间资源类型的端点（Bot / User 身份均可用）。
//
// 输入任意 token（wiki node_token、docx/sheet/bitable/slides/file/folder 等 obj_token），
// 返回真实 obj_type 与 obj_token；wiki 节点自动解包（is_wiki_token=true）。
// 与 node_by_token 互补：后者只认知识库内资源，前者覆盖普通云空间资源。
const DriveQueryByTokenPath = "/open-apis/drive/v2/files/query_by_token"

// query_by_token 的业务错误码。
const (
	DriveTokenCodeInvalid   = 981002 // token 格式无效
	DriveTokenCodeNotFound  = 981003 // token 不存在（或当前身份不可见）
	DriveTokenCodeForbidden = 981004 // 当前身份无权访问
)

// DriveTokenInfo 是 query_by_token 的识别结果。
type DriveTokenInfo struct {
	ObjToken    string `json:"obj_token"`
	ObjType     string `json:"obj_type"`
	IsWikiToken bool   `json:"is_wiki_token"`
	// Status 描述输入节点本身的状态（不是底层对象）：0 正常，1 在回收站，2 已删除。
	Status int `json:"status"`
}

// DriveTokenQueryError 是 query_by_token 返回的业务错误，携带错误码与中文提示。
type DriveTokenQueryError struct {
	Token string
	Code  int
	Msg   string
	LogID string
}

func (e *DriveTokenQueryError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "识别资源类型失败: code=%d, msg=%s", e.Code, e.Msg)
	if e.LogID != "" {
		fmt.Fprintf(&b, ", log_id=%s", e.LogID)
	}
	if hint := e.Hint(); hint != "" {
		b.WriteString("\n提示: ")
		b.WriteString(hint)
	}
	return b.String()
}

// Hint 按错误码返回中文排障建议。
func (e *DriveTokenQueryError) Hint() string {
	switch e.Code {
	case DriveTokenCodeInvalid:
		return "token 格式无效，请检查是否完整复制了 URL 中的 token。"
	case DriveTokenCodeNotFound:
		return "资源不存在，或当前身份看不到它。请确认 token 正确；他人文档请先 feishu-cli auth login 后用 User 身份访问。"
	case DriveTokenCodeForbidden:
		return "当前身份无权访问该资源。Bot 身份需被添加为文档协作者；也可登录后用 User 身份访问（feishu-cli auth login）。"
	default:
		return ""
	}
}

// QueryDriveToken 调用 GET /open-apis/drive/v2/files/query_by_token 识别 token 的真实类型。
// wiki node_token 会被自动解包为底层 obj_token/obj_type（IsWikiToken=true）。
// userAccessToken 为空时使用 App（Bot）身份。
func QueryDriveToken(token, userAccessToken string) (*DriveTokenInfo, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("识别资源类型失败: token 不能为空")
	}
	env, logID, err := callOpenAPIJSON(http.MethodGet, DriveQueryByTokenPath, nil, map[string]string{"token": token}, nil, userAccessToken)
	if err != nil {
		return nil, fmt.Errorf("识别资源类型失败: %w", err)
	}
	if env.Code != 0 {
		return nil, &DriveTokenQueryError{Token: token, Code: env.Code, Msg: env.Msg, LogID: logID}
	}
	var info DriveTokenInfo
	if len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, &info); err != nil {
			return nil, fmt.Errorf("解析 query_by_token 响应失败: %w", err)
		}
	}
	if info.ObjToken == "" || info.ObjType == "" {
		return nil, fmt.Errorf("识别资源类型失败: query_by_token 返回的数据不完整（obj_type=%q, obj_token=%q）", info.ObjType, info.ObjToken)
	}
	if !IsSafeResourceToken(info.ObjToken) {
		return nil, fmt.Errorf("识别资源类型失败: query_by_token 返回了非法的 obj_token %q", info.ObjToken)
	}
	info.ObjType = NormalizeResourceType(info.ObjType)
	return &info, nil
}
