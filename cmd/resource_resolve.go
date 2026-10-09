package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
)

// resourceArgOptions 控制 resolveResourceArg / parseResourceArg 的解析行为。
type resourceArgOptions struct {
	// ArgName 报错时展示的参数名，如 "<document_id>"、"--url"、"--doc-token"。
	ArgName string
	// ExplicitType 用户显式声明的类型（--type / --doc-type 等），可空。
	// 与 URL 路径推断的类型冲突时报错；唯一例外：URL 是 wiki 且 ResolveWiki=true 时，
	// 非 wiki 的 ExplicitType 表示「期望的底层文档类型」，解包后校验。
	ExplicitType string
	// DefaultType 裸 token 且未显式声明类型时采用的类型；为空表示裸 token 必须显式声明类型。
	DefaultType string
	// Allowed 最终资源类型（wiki 解包后）的白名单；为空表示不限制。
	Allowed []string
	// ResolveWiki 为 true 时，wiki 输入通过 node_by_token 换出底层 obj_token / obj_type。
	ResolveWiki bool
	// DetectBareType 为 true 时，未声明类型（且无 DefaultType）的裸 token 通过
	// query_by_token 自动识别类型（会发起网络请求；wiki node_token 识别为 wiki）。
	DetectBareType bool
	// UserAccessToken wiki 解析使用的身份；为空表示 Bot（App）身份。
	UserAccessToken string
}

// resolvedResource 是资源参数的解析结果。
type resolvedResource struct {
	Input      string           // 原始输入（已 trim）
	FromURL    bool             // 输入是否为 URL
	InputType  string           // URL 路径推断或显式声明的类型（wiki 解包前）
	InputToken string           // URL 中的 token 或裸 token（wiki 解包前）
	Type       string           // 最终类型：wiki 解包后为 obj_type
	Token      string           // 最终 token：wiki 解包后为 obj_token
	WikiNode   *client.WikiNode // 非 nil 表示输入是 wiki 节点且已解包
	// ExpectedObjType 是 wiki 输入时用户声明的期望底层类型（来自 ExplicitType），解包后校验。
	ExpectedObjType string
	// DetectedBy 非空表示类型由服务端识别（目前只有 "query_by_token"）。
	DetectedBy string
	// TokenStatus 是 query_by_token 返回的输入节点状态：0 正常，1 回收站，2 已删除。
	TokenStatus int
}

func (o resourceArgOptions) argName() string {
	if strings.TrimSpace(o.ArgName) == "" {
		return "参数"
	}
	return o.ArgName
}

// parseResourceArg 离线解析资源参数（裸 token 或飞书/Lark URL），不发起任何网络请求，
// 可在 --dry-run 之前调用。wiki 输入在 ResolveWiki=true 时 Type/Token 先保持 wiki 原值，
// 需再调用 resolveWikiInResource 解包。
func parseResourceArg(raw string, opts resourceArgOptions) (*resolvedResource, error) {
	input := strings.TrimSpace(raw)
	name := opts.argName()
	if input == "" {
		return nil, fmt.Errorf("%s 不能为空", name)
	}
	explicit := client.NormalizeResourceType(opts.ExplicitType)
	res := &resolvedResource{Input: input}

	if client.LooksLikeURL(input) {
		ref, err := client.ParseResourceURL(input)
		if err != nil {
			return nil, fmt.Errorf("%s 解析失败: %w", name, err)
		}
		res.FromURL = true
		res.InputType = ref.Type
		res.InputToken = ref.Token
		if explicit != "" && explicit != ref.Type {
			if ref.Type == client.ResourceTypeWiki && opts.ResolveWiki {
				// wiki URL + 非 wiki 类型：视为对底层文档类型的断言，解包后校验
				res.ExpectedObjType = explicit
			} else {
				return nil, fmt.Errorf("显式指定的类型 %q 与 URL 路径推断的类型 %q 冲突；请去掉类型参数或改成一致的值", explicit, ref.Type)
			}
		}
	} else {
		if strings.ContainsAny(input, "/?#\\ \t\r\n") {
			return nil, fmt.Errorf("%s 必须是裸 token 或完整的飞书/Lark URL，不接受部分路径: %q", name, input)
		}
		if !client.IsSafeResourceToken(input) {
			return nil, fmt.Errorf("%s 不是有效的 token（只允许字母、数字、_ 和 -，长度 1-128）: %q", name, input)
		}
		res.InputToken = input
		res.InputType = explicit
		if res.InputType == "" {
			res.InputType = client.NormalizeResourceType(opts.DefaultType)
		}
		if res.InputType == "" {
			if opts.DetectBareType {
				// 类型留空，由 detectResourceType 通过 query_by_token 识别
				return res, nil
			}
			return nil, fmt.Errorf("%s 是裸 token 时必须显式指定资源类型", name)
		}
	}

	res.Type = res.InputType
	res.Token = res.InputToken
	// 非 wiki 或不解包的输入，此时即可校验类型白名单
	if !(res.InputType == client.ResourceTypeWiki && opts.ResolveWiki) {
		if err := checkResourceTypeAllowed(name, res.Type, opts.Allowed); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// resolveWikiInResource 对 wiki 输入调用 node_by_token 换出底层 obj_token/obj_type，
// 并校验 ExpectedObjType 与 Allowed。非 wiki 输入原样返回。
func resolveWikiInResource(res *resolvedResource, opts resourceArgOptions) error {
	if res == nil || res.InputType != client.ResourceTypeWiki || !opts.ResolveWiki {
		return nil
	}
	node, err := client.ResolveWikiNode(res.InputToken, opts.UserAccessToken)
	if err != nil {
		return err
	}
	objType := client.NormalizeResourceType(node.ObjType)
	if res.ExpectedObjType != "" && res.ExpectedObjType != objType {
		return fmt.Errorf("wiki 节点 %s 的底层文档类型是 %q，与显式指定的类型 %q 不一致", res.InputToken, objType, res.ExpectedObjType)
	}
	if err := checkResourceTypeAllowed(opts.argName(), objType, opts.Allowed); err != nil {
		return fmt.Errorf("wiki 节点 %s 解析为 %s: %w", res.InputToken, objType, err)
	}
	res.WikiNode = node
	res.Type = objType
	res.Token = node.ObjToken
	return nil
}

// detectResourceType 对类型未知的裸 token 调用 query_by_token 识别真实类型。
// wiki node_token 识别为 wiki（保留原 token，后续由 resolveWikiInResource 解包取得 space_id 等信息）；
// 其他资源直接采用服务端返回的 obj_type/obj_token。类型已知时不发请求。
func detectResourceType(res *resolvedResource, opts resourceArgOptions) error {
	if res == nil || res.InputType != "" || !opts.DetectBareType {
		return nil
	}
	info, err := client.QueryDriveToken(res.InputToken, opts.UserAccessToken)
	if err != nil {
		return fmt.Errorf("%w\n无法自动识别 %s 的类型时，请显式指定资源类型", err, opts.argName())
	}
	res.DetectedBy = "query_by_token"
	res.TokenStatus = info.Status
	if info.IsWikiToken {
		res.InputType = client.ResourceTypeWiki
	} else {
		res.InputType = info.ObjType
		res.InputToken = info.ObjToken
	}
	res.Type, res.Token = res.InputType, res.InputToken
	if !(res.InputType == client.ResourceTypeWiki && opts.ResolveWiki) {
		return checkResourceTypeAllowed(opts.argName(), res.Type, opts.Allowed)
	}
	return nil
}

// resolveResourceArg 解析资源参数：离线解析 → （可选）query_by_token 识别裸 token 类型 →
// （可选）node_by_token 解包 wiki。是 doc/slides/drive 等命令接收「裸 token 或 URL」的统一入口。
func resolveResourceArg(raw string, opts resourceArgOptions) (*resolvedResource, error) {
	res, err := parseResourceArg(raw, opts)
	if err != nil {
		return nil, err
	}
	if err := detectResourceType(res, opts); err != nil {
		return nil, err
	}
	if err := resolveWikiInResource(res, opts); err != nil {
		return nil, err
	}
	return res, nil
}

func checkResourceTypeAllowed(name, t string, allowed []string) error {
	if len(allowed) == 0 {
		return nil
	}
	for _, a := range allowed {
		if t == a {
			return nil
		}
	}
	return fmt.Errorf("%s 的资源类型是 %q，但该命令仅支持 %s", name, t, strings.Join(allowed, "/"))
}

// resolveDocxArg 解析 docx 文档参数：docx token、/docx/ URL、/wiki/ URL（自动换出底层 docx 的 obj_token）。
// 返回可直接用于 docx/docs_ai 接口的 document_id；wiki 解包时在 stderr 提示。
func resolveDocxArg(raw, argName, userAccessToken string) (string, error) {
	res, err := resolveResourceArg(raw, resourceArgOptions{
		ArgName:         argName,
		DefaultType:     client.ResourceTypeDocx,
		Allowed:         []string{client.ResourceTypeDocx},
		ResolveWiki:     true,
		UserAccessToken: userAccessToken,
	})
	if err != nil {
		return "", err
	}
	noteWikiResolved(res)
	return res.Token, nil
}

// noteWikiResolved 在 stderr 提示 wiki 已解包为底层文档（stdout 保持干净，便于管道/JSON）。
func noteWikiResolved(res *resolvedResource) {
	if res == nil || res.WikiNode == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "已将 wiki 节点 %s 解析为 %s: %s\n", res.InputToken, res.Type, res.Token)
}

// wikiLookupInputTypes 是 node_by_token 能识别的输入类型（wiki 节点本身或挂载在知识库中的文档）。
var wikiLookupInputTypes = []string{
	client.ResourceTypeWiki, client.ResourceTypeDocx, client.ResourceTypeDoc, client.ResourceTypeSheet,
	client.ResourceTypeBitable, client.ResourceTypeMindnote, client.ResourceTypeSlides, client.ResourceTypeFile,
}

// extractWikiLookupToken 从裸 token 或飞书 URL 中取出可交给 node_by_token 的 token。
// 接受 /wiki/ URL，也接受挂载在知识库中的文档 URL（/docx/、/sheets/ 等，服务端按 obj_token 识别）。
func extractWikiLookupToken(input string) (string, error) {
	res, err := parseResourceArg(input, resourceArgOptions{
		ArgName:     "<node_token>",
		DefaultType: client.ResourceTypeWiki,
		Allowed:     wikiLookupInputTypes,
	})
	if err != nil {
		return "", err
	}
	return res.InputToken, nil
}
