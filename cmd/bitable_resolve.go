package cmd

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/riba2534/feishu-cli/v2/internal/output"
	"github.com/spf13/cobra"
)

// ==================== bitable resolve：把多维表格相关链接解析成可用坐标 ====================
// 对齐官方 base +url-resolve（shortcuts/base/base_resolve.go）：
//   - /base/<token>?table=&view=&record=  base_token 离线解析；?table= 是"当前选中的顶层块"，
//     可能是数据表、仪表盘、工作流、文件夹或文档，需 POST /bases/:token/blocks/list 判型
//   - /wiki/<node_token>                  node_by_token 换出 obj_token，校验 obj_type=bitable
//   - /record/<share_token>               GET /base/v3/record_share/:token/meta 换出 base/table/record
//   - /share/base/form/<share_token>      表单分享 token（form detail/submit 使用）
//   - 视图/仪表盘分享、工作区、新增记录、BaseApp 链接：CLI 无法解析，明确报错

const (
	bitableNextStepBlockList = "用 `feishu-cli bitable block list --base-token <base_token>` 列出数据表、仪表盘、工作流等顶层块"
	bitableNextStepRecords   = "用 `feishu-cli bitable record list --base-token <base_token> --table-id <table_id>` 读取记录"
)

// bitableURLKind 链接分类（只看 URL path 前缀）。
func bitableURLKind(u *url.URL) string {
	path := u.Path
	if path == "" {
		path = "/"
	}
	has := func(prefix string) bool { return bitableURLSegment(path, prefix) != "" }
	switch {
	case has("/app/"):
		return "baseapp_url"
	case has("/base/workspace/"):
		return "workspace_url"
	case has("/base/add/"):
		return "add_record_url"
	case has("/base/"), has("/bitable/"):
		return "base_url"
	case has("/wiki/"):
		return "wiki_url"
	case has("/record/"):
		return "record_share_url"
	case has("/share/base/form/"):
		return "form_share_url"
	case has("/share/base/view/"):
		return "view_share_url"
	case has("/share/base/dashboard/"):
		return "dashboard_share_url"
	case has("/share/base/"):
		// form share update 返回的分享链接形如 /share/base/<shr_token>（实测 form detail 可用）
		return "form_share_url"
	}
	return ""
}

// bitableFormShareToken 取表单分享 token：/share/base/form/<token> 或 /share/base/<token>。
func bitableFormShareToken(path string) string {
	if t := bitableURLSegment(path, "/share/base/form/"); t != "" {
		return t
	}
	return bitableURLSegment(path, "/share/base/")
}

// classifyBaseShareURL 给 --base-token 的报错用：识别出"不是 base 本体"的多维表格相关链接，返回中文类别名。
func classifyBaseShareURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	switch bitableURLKind(u) {
	case "record_share_url":
		return "记录分享"
	case "form_share_url":
		return "表单分享"
	case "view_share_url":
		return "视图分享"
	case "dashboard_share_url":
		return "仪表盘分享"
	case "workspace_url":
		return "工作区"
	case "add_record_url":
		return "新增记录"
	case "baseapp_url":
		return "BaseApp"
	}
	return ""
}

// bitableURLSegment 返回 path 以 prefix 开头时紧随其后的第一个路径段。
func bitableURLSegment(path, prefix string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := path[len(prefix):]
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	return strings.TrimSpace(rest)
}

// parseBitableResolveURL 校验协议、域名白名单与 token 形状（复用统一资源 URL 的安全规则）。
func parseBitableResolveURL(raw string) (*url.URL, string, error) {
	raw = strings.TrimSpace(raw)
	if !client.LooksLikeURL(raw) {
		return nil, "", clierr.Usagef("--url 只接受完整链接（https://...）；已知 base_token 时直接用 --base-token")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, "", clierr.Usagef("--url 不是合法链接: %q", raw)
	}
	if u.User != nil {
		return nil, "", clierr.Usagef("--url 不能包含用户信息 (userinfo)")
	}
	host := strings.ToLower(u.Hostname())
	switch strings.ToLower(u.Scheme) {
	case "https":
		if !client.IsFeishuResourceHost(host) && !bitableIsLoopbackHost(host) && !config.AllowCustomBaseURL() {
			return nil, "", clierr.Usagef("不支持的域名 %q：仅接受飞书/Lark 文档域名（*.feishu.cn、*.larksuite.com、*.larkoffice.com）", host)
		}
	case "http":
		if !bitableIsLoopbackHost(host) {
			return nil, "", clierr.Usagef("链接必须使用 https 协议: %q", raw)
		}
	default:
		return nil, "", clierr.Usagef("不支持的 URL 协议 %q，仅支持 https", u.Scheme)
	}
	kind := bitableURLKind(u)
	return u, kind, nil
}

func bitableIsLoopbackHost(host string) bool {
	h := strings.Trim(host, "[]")
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

// checkResolvedToken 校验从链接中取出的 token 只含安全字符（拼进 API 路径前）。
func checkResolvedToken(name, token string) error {
	if !client.IsSafeResourceToken(token) {
		return clierr.Usagef("链接中的 %s %q 格式无效（只允许字母、数字、_ 和 -）", name, token)
	}
	return nil
}

var bitableResolveCmd = &cobra.Command{
	Use:   "resolve",
	Short: "把多维表格 / 知识库 / 记录分享链接解析成 base_token、table_id 等坐标",
	Long: `把链接解析成可直接用于其他 bitable 命令的坐标（对齐官方 base +url-resolve）。

支持的链接:
  /base/<base_token>?table=<block_id>&view=<view_id>&record=<record_id>
      base_token 离线解析；URL 里的 table= 是"当前选中的顶层块"，可能是数据表、仪表盘、
      工作流、文件夹或文档——会调用 blocks/list 判型（block_type），只有 table 才输出 table_id
  /wiki/<node_token>        知识库中的多维表格：node_by_token 换出底层 base_token（校验 obj_type=bitable）
  /record/<share_token>     记录分享链接：换出 base_token / table_id / record_id
  /share/base/form/<token>、/share/base/<token>
                            表单分享链接：输出 share_token（配合 form detail / form submit）

视图分享、仪表盘分享、工作区、新增记录、BaseApp 链接无法解析，会明确报错。

示例:
  feishu-cli bitable resolve --url "https://example.feishu.cn/base/bascnxxx?table=tblxxx&view=vewxxx"
  feishu-cli bitable resolve --url "https://example.feishu.cn/wiki/wikcnxxx"
  feishu-cli bitable resolve --url "https://example.feishu.cn/record/xxxx" --jq .record_id`,
	RunE: func(cmd *cobra.Command, args []string) error {
		raw, _ := cmd.Flags().GetString("url")
		if strings.TrimSpace(raw) == "" {
			return clierr.Usagef("--url 必填")
		}
		u, kind, err := parseBitableResolveURL(raw)
		if err != nil {
			return err
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}
		if err := config.Validate(); err != nil {
			return err
		}
		o, err := output.ParseOptions(cmd)
		if err != nil {
			return err
		}
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		switch kind {
		case "base_url":
			out, err := resolveBitableBaseURL(cmd, u, dryRun)
			if err != nil {
				return err
			}
			return output.Render(o, out)
		case "wiki_url":
			nodeToken := bitableURLSegment(u.Path, "/wiki/")
			if err := checkResolvedToken("wiki node_token", nodeToken); err != nil {
				return err
			}
			if dryRun {
				return output.Render(o, map[string]any{
					"input_type": "wiki_url",
					"api": []map[string]any{
						{"method": "GET", "path": client.WikiNodeByTokenPath, "params": map[string]any{"token": nodeToken}},
						{"method": "POST", "path": "/open-apis/base/v3/bases/<obj_token>/blocks/list", "note": "仅当 URL 带 table= 时调用"},
					},
				})
			}
			token, err := resolveIdentityToken(cmd)
			if err != nil {
				return err
			}
			node, err := resolveBitableWikiNode(nodeToken, token)
			if err != nil {
				return err
			}
			out := map[string]any{
				"input_type":      "wiki_url",
				"resource_type":   "bitable",
				"wiki_node_token": nodeToken,
				"base_token":      node.ObjToken,
				"title":           node.Title,
			}
			enrichBitableSelection(out, u, node.ObjToken, token)
			return output.Render(o, out)
		case "record_share_url":
			shareToken := bitableURLSegment(u.Path, "/record/")
			if err := checkResolvedToken("record share token", shareToken); err != nil {
				return err
			}
			path := client.BaseV3Path("record_share", shareToken, "meta")
			if dryRun {
				return output.Render(o, map[string]any{"input_type": "record_share_url", "api": []map[string]any{{"method": "GET", "path": path}}})
			}
			token, err := resolveIdentityToken(cmd)
			if err != nil {
				return err
			}
			data, err := client.BaseV3Call("GET", path, nil, nil, token)
			if err != nil {
				return err
			}
			baseToken := bitableStr(data, "base_token")
			tableID := bitableStr(data, "table_id")
			recordID := bitableStr(data, "record_id")
			out := map[string]any{
				"input_type":         "record_share_url",
				"resource_type":      "bitable",
				"record_share_token": bitableFirstNonEmpty(bitableStr(data, "record_share_token"), shareToken),
				"base_token":         baseToken,
				"table_id":           tableID,
				"record_id":          recordID,
				"hint": map[string]any{
					"next_step": fmt.Sprintf("用 `feishu-cli bitable record get --base-token %s --table-id %s --record-id %s` 读取该记录", baseToken, tableID, recordID),
				},
			}
			return output.Render(o, out)
		case "form_share_url":
			shareToken := bitableFormShareToken(u.Path)
			if err := checkResolvedToken("form share token", shareToken); err != nil {
				return err
			}
			return output.Render(o, map[string]any{
				"input_type":    "form_share_url",
				"resource_type": "bitable_form",
				"share_token":   shareToken,
				"hint": map[string]any{
					"next_step": fmt.Sprintf("用 `feishu-cli bitable form detail --share-token %s` 查看表单，或 form submit 提交", shareToken),
				},
			})
		case "view_share_url", "dashboard_share_url", "workspace_url", "add_record_url", "baseapp_url":
			return clierr.Usagef("这是多维表格%s链接，CLI 无法解析；请在浏览器中打开，或提供多维表格本身的链接（/base/ 或 /wiki/）", classifyBaseShareURL(raw))
		default:
			return clierr.Usagef("不是受支持的多维表格链接；支持 /base/、/wiki/、/record/、/share/base/form/ 链接")
		}
	},
}

// resolveBitableBaseURL 解析 /base/ 链接：base_token 离线取得；带 table= 时调 blocks/list 判型。
func resolveBitableBaseURL(cmd *cobra.Command, u *url.URL, dryRun bool) (map[string]any, error) {
	baseToken := bitableURLSegment(u.Path, "/base/")
	if baseToken == "" {
		baseToken = bitableURLSegment(u.Path, "/bitable/")
	}
	if err := checkResolvedToken("base_token", baseToken); err != nil {
		return nil, err
	}
	out := map[string]any{
		"input_type":    "base_url",
		"resource_type": "bitable",
		"base_token":    baseToken,
	}
	selected := strings.TrimSpace(u.Query().Get("table"))
	if dryRun {
		if selected != "" {
			out["api"] = []map[string]any{{"method": "POST", "path": client.BaseV3Path("bases", baseToken, "blocks", "list"), "body": map[string]any{}}}
			out["block_id"] = selected
		} else {
			out["resolution"] = "local"
		}
		return out, nil
	}
	token := ""
	if selected != "" {
		var err error
		token, err = resolveIdentityToken(cmd)
		if err != nil {
			return nil, err
		}
	}
	enrichBitableSelection(out, u, baseToken, token)
	return out, nil
}

// enrichBitableSelection 处理 URL 里的 table=/view=/record= 选择：table= 是顶层块 ID，
// 通过 blocks/list 判型后才决定输出 table_id / dashboard_id / workflow_id。
func enrichBitableSelection(out map[string]any, u *url.URL, baseToken, token string) {
	q := u.Query()
	selected := strings.TrimSpace(q.Get("table"))
	if selected == "" {
		out["hint"] = map[string]any{"next_step": bitableNextStepBlockList}
		return
	}
	out["block_id"] = selected
	out["selection_source"] = "url_query"
	block, found, err := findBitableBlock(baseToken, selected, token)
	if err != nil || !found {
		next := "用 `feishu-cli bitable block list` 按 block_id 确认它是数据表、仪表盘、工作流、文件夹还是文档"
		if err != nil {
			next = fmt.Sprintf("读取顶层块列表失败（%v）；%s", err, next)
		}
		out["hint"] = map[string]any{"next_step": next}
		return
	}
	blockType := bitableStr(block, "type")
	out["block_type"] = blockType
	if name := bitableStr(block, "name"); name != "" {
		out["block_name"] = name
	}
	switch blockType {
	case "table":
		out["table_id"] = selected
		if v := strings.TrimSpace(q.Get("view")); v != "" {
			out["view_id"] = v
		}
		if r := strings.TrimSpace(q.Get("record")); r != "" {
			out["record_id"] = r
		}
		out["hint"] = map[string]any{"next_step": bitableNextStepRecords}
	case "dashboard":
		out["dashboard_id"] = selected
		out["hint"] = map[string]any{"next_step": "用 `feishu-cli bitable dashboard get --dashboard-id <dashboard_id>` 查看该仪表盘"}
	case "workflow":
		out["workflow_id"] = selected
		out["hint"] = map[string]any{"next_step": "用 `feishu-cli bitable workflow get --workflow-id <workflow_id>` 查看该工作流"}
	case "folder":
		out["hint"] = map[string]any{"next_step": fmt.Sprintf("用 `feishu-cli bitable block list --base-token %s --parent-id %s` 列出该文件夹下的块", baseToken, selected)}
	case "docx":
		if docx := bitableStr(block, "docx_token"); docx != "" {
			out["docx_token"] = docx
			out["hint"] = map[string]any{"next_step": fmt.Sprintf("用 `feishu-cli doc read %s` 读取该文档", docx)}
		}
	default:
		out["hint"] = map[string]any{"next_step": bitableNextStepBlockList}
	}
}

// findBitableBlock 在 blocks/list 结果中按 ID 查找顶层块。
func findBitableBlock(baseToken, blockID, token string) (map[string]any, bool, error) {
	data, err := client.BaseV3Call("POST", client.BaseV3Path("bases", baseToken, "blocks", "list"), nil, map[string]any{}, token)
	if err != nil {
		return nil, false, err
	}
	for _, item := range bitableAnySlice(data["blocks"]) {
		if m, ok := item.(map[string]any); ok && bitableStr(m, "id") == blockID {
			return m, true, nil
		}
	}
	return nil, false, nil
}

func bitableFirstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// ==================== bitable block list ====================

var bitableBlockTypes = []string{"folder", "table", "docx", "dashboard", "workflow"}

var bitableBlockCmd = &cobra.Command{
	Use:   "block",
	Short: "多维表格顶层块（数据表/仪表盘/工作流/文件夹/文档）",
}

var bitableBlockListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出多维表格的顶层块（数据表、仪表盘、工作流、文件夹、文档）",
	Long: `POST /open-apis/base/v3/bases/{base_token}/blocks/list

列出多维表格直接管理的顶层块。table/dashboard/workflow 块返回的 id 即对应命令的
--table-id / --dashboard-id / --workflow-id；docx 块返回 docx_token；folder 块的 id
可作为 --parent-id 只列该文件夹的直接子块。服务端一次返回全量列表（无分页参数）。

URL 中的 ?table=<id> 是"当前选中的顶层块"，不一定是数据表——用本命令或 bitable resolve 判型。

可选:
  --type        只保留某类块: folder|table|docx|dashboard|workflow
  --parent-id   文件夹块 ID，只列其直接子块

示例:
  feishu-cli bitable block list --base-token bascnxxx
  feishu-cli bitable block list --base-token bascnxxx --type dashboard --jq '.blocks[] | {id, name}'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		blockType, _ := cmd.Flags().GetString("type")
		blockType = strings.TrimSpace(blockType)
		if blockType != "" {
			if err := validateEnum(blockType, "type", bitableBlockTypes); err != nil {
				return clierr.Usage(err)
			}
		}
		body := map[string]any{}
		if parentID, _ := cmd.Flags().GetString("parent-id"); strings.TrimSpace(parentID) != "" {
			body["parent_id"] = strings.TrimSpace(parentID)
		}
		if err := config.Validate(); err != nil {
			return err
		}
		baseToken, err := resolveBaseToken(cmd)
		if err != nil {
			return err
		}
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		data, err := client.BaseV3Call("POST", client.BaseV3Path("bases", baseToken, "blocks", "list"), nil, body, token)
		if err != nil {
			return err
		}
		filterBitableBlocks(data, blockType)
		return renderBitableResult(cmd, data)
	},
}

// filterBitableBlocks 按 --type 过滤 blocks 并同步 total。
func filterBitableBlocks(data map[string]any, blockType string) {
	if blockType == "" || data == nil {
		return
	}
	blocks := bitableAnySlice(data["blocks"])
	filtered := make([]any, 0, len(blocks))
	for _, b := range blocks {
		if m, ok := b.(map[string]any); ok && m["type"] == blockType {
			filtered = append(filtered, b)
		}
	}
	data["blocks"] = filtered
	data["total"] = len(filtered)
}

func init() {
	bitableCmd.AddCommand(bitableResolveCmd)
	bitableResolveCmd.Flags().String("url", "", "多维表格 / 知识库 / 记录分享 / 表单分享链接（必填）")
	bitableResolveCmd.Flags().String("user-access-token", "", "User Access Token")
	output.AddFormatFlags(bitableResolveCmd)
	output.AddDryRunFlag(bitableResolveCmd)
	mustMarkFlagRequired(bitableResolveCmd, "url")

	bitableCmd.AddCommand(bitableBlockCmd)
	bitableBlockCmd.AddCommand(bitableBlockListCmd)
	addBitableCommonFlags(bitableBlockListCmd)
	bitableBlockListCmd.Flags().String("type", "", "只保留某类块: folder|table|docx|dashboard|workflow")
	bitableBlockListCmd.Flags().String("parent-id", "", "文件夹块 ID（只列其直接子块）")
}
