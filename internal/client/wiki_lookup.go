package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// WikiNodeByTokenPath 是统一的知识库节点解析端点。
//
// 与旧的 get_node 不同，node_by_token 由服务端自动识别 token 类型：
// 既接受 wiki node_token，也接受挂载在知识库里的文档 obj_token（无需 obj_type），
// 并返回区分度更高的错误码。旧端点对 obj_token 一律报 131005 not found。
// 官方 lark-cli 已全量迁移（#2665/#2676/#2682/#2689/#2696/#2699）。
const WikiNodeByTokenPath = "/open-apis/wiki/v2/spaces/node_by_token"

// node_by_token 的业务错误码。
const (
	WikiCodeForbidden        = 131006 // 当前身份无权读取该节点
	WikiCodeNodeNotFound     = 131012 // 节点已删除或不存在
	WikiCodeInvalidToken     = 131013 // token 无效（完整长度但不存在）
	WikiCodeNotInWiki        = 131014 // 文档存在但不在知识库中
	WikiCodeInvalidTokenSize = 131016 // token 长度非法（通常是被截断）
)

// WikiLookupError 是 node_by_token 返回的业务错误，携带错误码与中文排障提示。
// Error() 保持本仓统一的 "code=<N>, msg=..." 形态，HasAPICode 可直接识别。
type WikiLookupError struct {
	Token string
	Code  int
	Msg   string
	LogID string
}

func (e *WikiLookupError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "解析知识库节点失败: code=%d, msg=%s", e.Code, e.Msg)
	if e.LogID != "" {
		fmt.Fprintf(&b, ", log_id=%s", e.LogID)
	}
	if hint := e.Hint(); hint != "" {
		b.WriteString("\n提示: ")
		b.WriteString(hint)
	}
	return b.String()
}

// Hint 返回按错误码分类的中文排障建议；未知错误码返回空串。
func (e *WikiLookupError) Hint() string {
	switch e.Code {
	case WikiCodeNodeNotFound:
		return "知识库节点已被删除或不存在。不要用同一个 token 重试，请重新获取当前有效的知识库链接。"
	case WikiCodeInvalidToken, WikiCodeInvalidTokenSize:
		return "token 无效（131016 通常表示 token 被截断）。重试、切换身份或重新授权都无法修复；请检查 URL/token 是否完整，提供有效的 wiki node_token、完整的文档 obj_token 或带类型的文档 URL。"
	case WikiCodeNotInWiki:
		return "该文档不在知识库中（不是 wiki 节点）。普通云文档请直接使用其 docx/sheets/base 等 URL 或 token，无需 wiki 解析。"
	case WikiCodeForbidden:
		return "当前身份无权读取该知识库节点。Bot 身份需被添加为知识空间成员或文档协作者；也可登录后用 User 身份访问（feishu-cli auth login）。"
	default:
		return ""
	}
}

// NotFound 节点已删除或不存在。
func (e *WikiLookupError) NotFound() bool { return e.Code == WikiCodeNodeNotFound }

// InvalidToken token 形状或取值非法（131013/131016）。
func (e *WikiLookupError) InvalidToken() bool {
	return e.Code == WikiCodeInvalidToken || e.Code == WikiCodeInvalidTokenSize
}

// NotInWiki 文档存在但未挂载在知识库。
func (e *WikiLookupError) NotInWiki() bool { return e.Code == WikiCodeNotInWiki }

// Retryable node_by_token 的已知业务错误码都是终态，换 token / 权限才可能成功。
func (e *WikiLookupError) Retryable() bool {
	switch e.Code {
	case WikiCodeNodeNotFound, WikiCodeInvalidToken, WikiCodeInvalidTokenSize, WikiCodeNotInWiki, WikiCodeForbidden:
		return false
	}
	return IsRetryableError(e)
}

// wikiNodeByTokenNode 对应 node_by_token 响应 data.node。
type wikiNodeByTokenNode struct {
	SpaceID         string `json:"space_id"`
	NodeToken       string `json:"node_token"`
	ObjToken        string `json:"obj_token"`
	ObjType         string `json:"obj_type"`
	ParentNodeToken string `json:"parent_node_token"`
	NodeType        string `json:"node_type"`
	OriginNodeToken string `json:"origin_node_token"`
	OriginSpaceID   string `json:"origin_space_id"`
	Title           string `json:"title"`
	HasChild        bool   `json:"has_child"`
	Creator         string `json:"creator"`
	Owner           string `json:"owner"`
	NodeCreateTime  string `json:"node_create_time"`
	ObjCreateTime   string `json:"obj_create_time"`
	ObjEditTime     string `json:"obj_edit_time"`
}

// ResolveWikiNode 通过 node_by_token 解析知识库节点。
//
// token 可以是 wiki node_token，也可以是挂载在知识库中的文档 obj_token（服务端自动识别）。
// 返回的 NodeToken 一律取自响应：传入 obj_token 时它与输入不同，后续 move/update 等
// 节点级操作必须使用这里的 NodeToken。userAccessToken 为空时使用 App 身份。
func ResolveWikiNode(token, userAccessToken string) (*WikiNode, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("解析知识库节点失败: token 不能为空")
	}
	env, logID, err := callOpenAPIJSON(http.MethodGet, WikiNodeByTokenPath, nil, map[string]string{"token": token}, nil, userAccessToken)
	if err != nil {
		return nil, fmt.Errorf("解析知识库节点失败: %w", err)
	}
	if env.Code != 0 {
		return nil, &WikiLookupError{Token: token, Code: env.Code, Msg: env.Msg, LogID: logID}
	}
	var data struct {
		Node *wikiNodeByTokenNode `json:"node"`
	}
	if len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, &data); err != nil {
			return nil, fmt.Errorf("解析 node_by_token 响应失败: %w", err)
		}
	}
	if data.Node == nil {
		return nil, fmt.Errorf("解析知识库节点失败: node_by_token 未返回节点信息（token=%s）", token)
	}
	node := data.Node
	if node.ObjToken == "" || node.ObjType == "" {
		return nil, fmt.Errorf("解析知识库节点失败: node_by_token 返回的节点数据不完整（obj_type=%q, obj_token=%q）", node.ObjType, node.ObjToken)
	}
	nodeToken := node.NodeToken
	if nodeToken == "" {
		// 理论上不会发生。输入不是 obj_token 时它只能是 node_token，可以安全回填；
		// 输入就是 obj_token 时绝不能把它当作 node_token 回填（旧 get_node 实现的 bug）。
		if token == node.ObjToken {
			return nil, fmt.Errorf("解析知识库节点失败: node_by_token 未返回 node_token（obj_token=%s）", node.ObjToken)
		}
		nodeToken = token
	}
	return &WikiNode{
		SpaceID:         node.SpaceID,
		NodeToken:       nodeToken,
		ObjToken:        node.ObjToken,
		ObjType:         node.ObjType,
		ParentNodeToken: node.ParentNodeToken,
		NodeType:        node.NodeType,
		OriginNodeToken: node.OriginNodeToken,
		OriginSpaceID:   node.OriginSpaceID,
		Title:           node.Title,
		HasChild:        node.HasChild,
		Creator:         node.Creator,
		Owner:           node.Owner,
		NodeCreateTime:  node.NodeCreateTime,
		ObjCreateTime:   node.ObjCreateTime,
		ObjEditTime:     node.ObjEditTime,
	}, nil
}
