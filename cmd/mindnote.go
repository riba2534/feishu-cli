package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// mindnote：已有思维笔记的节点读取与写入（对齐官方 lark-cli mindnotes nodes list/create）。
// 这条链路不新建思维笔记；mindnote_id 是思维笔记文档 token，wiki 链接需解包且底层类型必须是 mindnote。

var mindnoteHighlights = []string{"red", "yellow", "pink", "blue", "cyan", "olive", "grey"}

var mindnoteUserIDTypes = []string{"open_id", "union_id", "user_id"}

var mindnoteCmd = &cobra.Command{
	Use:   "mindnote",
	Short: "思维笔记节点操作（读取 / 新增 / 更新节点）",
	Long: `操作已有思维笔记（Mindnote）里的节点。只支持读取节点和新增/更新节点，不新建思维笔记。

子命令组:
  nodes list     获取节点列表（node_id / parent_id / texts / notes / images / finish / highlight）
  nodes create   新增子节点（传 parent_id），或按 node_id 更新已有节点

思维笔记参数（位置参数或 --mindnote-id，二选一）:
  - 思维笔记 token，或 https://xxx.feishu.cn/mindnotes/<token> 链接
  - https://xxx.feishu.cn/wiki/<node_token>：自动解包，底层类型必须是 mindnote，否则报错（退出码 2）
  - 不要把 wiki 链接里的 node_token 当裸 token 传（服务端会报 3410003 resource not found）

示例:
  feishu-cli mindnote nodes list https://xxx.feishu.cn/mindnotes/bmncnxxx
  feishu-cli mindnote nodes create bmncnxxx --client-token "$(uuidgen)" \
    --data '{"nodes":[{"parent_id":"node_parent123","texts":[{"element_type":"text","text":{"content":"子节点"}}]}]}'`,
}

var mindnoteNodesCmd = &cobra.Command{
	Use:   "nodes",
	Short: "思维笔记节点（list / create）",
	Long: `思维笔记节点读取与写入。

子命令:
  list     获取节点列表（默认 User 身份）
  create   新增或更新节点（默认 auto 身份，支持 --dry-run）`,
}

var mindnoteNodesListCmd = &cobra.Command{
	Use:   "list [mindnote_token|url]",
	Short: "获取思维笔记节点列表",
	Long: `获取思维笔记的全部节点（GET /open-apis/mindnote/v1/mindnotes/{mindnote_id}/nodes）。

默认输出缩进的节点树（文本 + node_id，完成/高亮/备注/图片作为附注）；-o json 输出接口原始 data。
返回字段：nodes[].node_id、parent_id、texts、notes、images、finish、highlight。

身份：--as user（默认，与官方元数据 accessTokens=[user] 一致）| auto | bot。User 身份需 auth login 时带
mindnote:node:read；--as bot 时网关按应用身份 scope mindnote:node:read 校验（应用未开通报 99991672）。

示例:
  feishu-cli mindnote nodes list bmncnxxx
  feishu-cli mindnote nodes list --mindnote-id https://xxx.feishu.cn/wiki/wikcnxxx -o json`,
	Args: cobra.MaximumNArgs(1),
	RunE: runMindnoteNodesList,
}

var mindnoteNodesCreateCmd = &cobra.Command{
	Use:   "create [mindnote_token|url]",
	Short: "新增或更新思维笔记节点",
	Long: `在已有思维笔记里新增子节点或更新已有节点（POST /open-apis/mindnote/v1/mindnotes/{mindnote_id}/nodes）。

--data 是 JSON 请求体（内联、@文件 或 - 读 stdin）：
  {"client_token":"<uuid>","nodes":[{"parent_id":"node_parent123","texts":[{"element_type":"text","text":{"content":"子节点"}}]}]}
  - nodes 必填且非空；nodes[].parent_id 表示在该节点下新增子节点，nodes[].node_id 指向已有节点表示更新它
  - texts / notes 是富文本元素数组（element_type: text / link / user / doc），images 是 [{"token":"图片 token"}]
  - highlight: red / yellow / pink / blue / cyan / olive / grey；finish: 完成状态

幂等：client_token 用于防止重试时重复创建/更新。CLI 不会自动生成（与官方一致）——每次生成新值无法让重试幂等；
需要可安全重试时自己生成并在重试时复用：--client-token "$(uuidgen)"，或写进 --data。
两处都给且不一致时报用法错误；都没给时在 stderr 提示。

写入前先 nodes list 确认 parent_id / node_id。--dry-run 只打印将发出的请求（不联网、不解析身份）。
身份：--as auto（默认，User 优先、未配置回退 Bot）| user | bot；需要 mindnote:node:create。

示例:
  feishu-cli mindnote nodes create bmncnxxx --client-token 9f1c2d3e-0000-4000-8000-000000000001 \
    --data '{"nodes":[{"parent_id":"node_parent123","texts":[{"element_type":"text","text":{"content":"子节点"}}],"highlight":"yellow"}]}'
  feishu-cli mindnote nodes create bmncnxxx --data @nodes.json --dry-run
  cat nodes.json | feishu-cli mindnote nodes create https://xxx.feishu.cn/wiki/wikcnxxx --data - --as user`,
	Args: cobra.MaximumNArgs(1),
	RunE: runMindnoteNodesCreate,
}

func init() {
	rootCmd.AddCommand(mindnoteCmd)
	mindnoteCmd.AddCommand(mindnoteNodesCmd)
	mindnoteNodesCmd.AddCommand(mindnoteNodesListCmd, mindnoteNodesCreateCmd)

	for _, c := range []*cobra.Command{mindnoteNodesListCmd, mindnoteNodesCreateCmd} {
		c.Flags().String("mindnote-id", "", "思维笔记 token 或 /mindnotes/、/wiki/ 链接（与位置参数二选一）")
		c.Flags().String("user-id-type", "", "用户 ID 类型：open_id / union_id / user_id（不传由服务端按 open_id 处理）")
		c.Flags().StringP("output", "o", "", "输出格式（json 输出接口原始 data）")
		c.Flags().String("user-access-token", "", "User Access Token（--as user/auto 时优先使用）")
	}
	mindnoteNodesListCmd.Flags().String("as", "user", "身份: user(User Token，默认) | bot(App Token) | auto(User 优先；未配置回退 Bot；已配置但解析/刷新失败 fail-closed)")

	addAsFlag(mindnoteNodesCreateCmd)
	mindnoteNodesCreateCmd.Flags().String("data", "", "JSON 请求体：内联 JSON、@文件路径 或 -（stdin）（必填）")
	mindnoteNodesCreateCmd.Flags().String("client-token", "", "幂等 token（写入请求体 client_token；建议 UUID，重试时复用同一个值）")
	mindnoteNodesCreateCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求，不联网、不解析身份")
}

// mindnoteInput 取思维笔记参数：位置参数与 --mindnote-id 二选一（相同值视为一个）。
func mindnoteInput(cmd *cobra.Command, args []string) (string, error) {
	flagVal := strings.TrimSpace(flagString(cmd, "mindnote-id"))
	argVal := ""
	if len(args) > 0 {
		argVal = strings.TrimSpace(args[0])
	}
	switch {
	case argVal != "" && flagVal != "" && argVal != flagVal:
		return "", clierr.Usagef("位置参数 %q 与 --mindnote-id %q 不一致，只需提供其中一个", argVal, flagVal)
	case argVal != "":
		return argVal, nil
	case flagVal != "":
		return flagVal, nil
	}
	return "", clierr.Usagef("缺少思维笔记：传 <mindnote_token|url> 位置参数或 --mindnote-id")
}

func mindnoteArgOptions(userAccessToken string) resourceArgOptions {
	return resourceArgOptions{
		ArgName:         "<mindnote_token|url>",
		DefaultType:     client.ResourceTypeMindnote,
		Allowed:         []string{client.ResourceTypeMindnote},
		ResolveWiki:     true,
		UserAccessToken: userAccessToken,
	}
}

// parseMindnoteArgOffline 离线解析（不联网）：格式错误、非 mindnote 链接按用法错误返回。
// isWiki=true 时 token 是 wiki node_token，执行时需 node_by_token 解包。
func parseMindnoteArgOffline(raw string) (token string, isWiki bool, err error) {
	res, err := parseResourceArg(raw, mindnoteArgOptions(""))
	if err != nil {
		return "", false, clierr.Usage(err)
	}
	if res.InputType == client.ResourceTypeWiki {
		return res.InputToken, true, nil
	}
	return res.Token, false, nil
}

// resolveMindnoteArg 把 <mindnote_token | /mindnotes/ URL | /wiki/ URL> 解析为思维笔记 token。
// wiki 链接用 node_by_token 解包，底层类型不是 mindnote 时返回用法错误（退出码 2）。
func resolveMindnoteArg(raw, userAccessToken string) (string, error) {
	token, isWiki, err := parseMindnoteArgOffline(raw)
	if err != nil || !isWiki {
		return token, err
	}
	node, err := client.ResolveWikiNode(token, userAccessToken)
	if err != nil {
		return "", err
	}
	objType := client.NormalizeResourceType(node.ObjType)
	if objType != client.ResourceTypeMindnote {
		return "", clierr.Usagef("wiki 节点 %s 的底层文档类型是 %q，不是思维笔记（mindnote）；该命令只接受思维笔记", token, objType)
	}
	noteWikiResolved(&resolvedResource{InputToken: token, Type: objType, Token: node.ObjToken, WikiNode: node})
	return node.ObjToken, nil
}

// mindnoteCommonFlags 校验 --user-id-type 与 -o（离线）。
func mindnoteCommonFlags(cmd *cobra.Command) (userIDType, output string, err error) {
	userIDType = strings.TrimSpace(flagString(cmd, "user-id-type"))
	if userIDType != "" {
		if err := validateEnum(userIDType, "--user-id-type", mindnoteUserIDTypes); err != nil {
			return "", "", err
		}
	}
	output = strings.ToLower(strings.TrimSpace(flagString(cmd, "output")))
	if output != "" && output != "json" {
		return "", "", clierr.Usagef("不支持的 --output %q，仅支持 json", output)
	}
	return userIDType, output, nil
}

// decorateMindnoteError 为思维笔记典型错误追加指引，保留原错误链。
func decorateMindnoteError(err error) error {
	if client.HasAPICode(err, 3410003) {
		return fmt.Errorf("%w\n提示：思维笔记不存在或当前身份无权访问。确认传的是思维笔记文档 token（不是节点 ID）；"+
			"wiki 链接请传完整 /wiki/ URL 让 CLI 解包，或先 `feishu-cli drive inspect --url <链接>` 确认底层类型是 mindnote", err)
	}
	return err
}

func runMindnoteNodesList(cmd *cobra.Command, args []string) error {
	raw, err := mindnoteInput(cmd, args)
	if err != nil {
		return err
	}
	userIDType, output, err := mindnoteCommonFlags(cmd)
	if err != nil {
		return err
	}
	if err := validateIdentityAs(cmd); err != nil {
		return err
	}
	if _, _, err := parseMindnoteArgOffline(raw); err != nil {
		return err
	}
	if err := config.Validate(); err != nil {
		return err
	}
	userAccessToken, err := resolveIdentityToken(cmd)
	if err != nil {
		return err
	}
	mindnoteID, err := resolveMindnoteArg(raw, userAccessToken)
	if err != nil {
		return err
	}
	data, err := client.ListMindnoteNodes(mindnoteID, userIDType, userAccessToken)
	if err != nil {
		return decorateMindnoteError(err)
	}
	if output == "json" {
		return printJSONTo(cmd.OutOrStdout(), data)
	}
	renderMindnoteNodes(cmd.OutOrStdout(), mindnoteID, data)
	return nil
}

func runMindnoteNodesCreate(cmd *cobra.Command, args []string) error {
	raw, err := mindnoteInput(cmd, args)
	if err != nil {
		return err
	}
	userIDType, output, err := mindnoteCommonFlags(cmd)
	if err != nil {
		return err
	}
	if err := validateIdentityAs(cmd); err != nil {
		return err
	}
	body, err := buildMindnoteCreateBody(cmd)
	if err != nil {
		return err
	}
	if _, ok := body["client_token"]; !ok {
		fmt.Fprintln(cmd.ErrOrStderr(), "提示: 未提供 client_token，重试可能重复创建节点；需要安全重试时传 --client-token <uuid> 并在重试时复用")
	}

	if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
		// dry-run 不联网、不解析身份：wiki 链接只离线解析，用占位符表示解包后的 token
		token, isWiki, err := parseMindnoteArgOffline(raw)
		if err != nil {
			return err
		}
		var steps []map[string]any
		path := client.MindnoteNodesPath(token)
		if isWiki {
			steps = append(steps, map[string]any{
				"method": "GET",
				"path":   client.WikiNodeByTokenPath,
				"params": map[string]any{"token": token},
				"desc":   "解析 wiki 节点为底层思维笔记（obj_type 必须为 mindnote）",
			})
			// 占位符不做 URL 转义，便于阅读
			path = strings.Replace(client.MindnoteNodesPath("MINDNOTE"), "MINDNOTE", "<resolved_mindnote_token>", 1)
		}
		step := map[string]any{
			"method": "POST",
			"path":   path,
			"body":   body,
		}
		if userIDType != "" {
			step["params"] = map[string]any{"user_id_type": userIDType}
		}
		steps = append(steps, step)
		return printJSONTo(cmd.OutOrStdout(), map[string]any{"dry_run": true, "steps": steps})
	}

	if _, _, err := parseMindnoteArgOffline(raw); err != nil {
		return err
	}
	if err := config.Validate(); err != nil {
		return err
	}
	userAccessToken, err := resolveIdentityToken(cmd)
	if err != nil {
		return err
	}
	mindnoteID, err := resolveMindnoteArg(raw, userAccessToken)
	if err != nil {
		return err
	}
	data, err := client.CreateMindnoteNodes(mindnoteID, userIDType, body, userAccessToken)
	if err != nil {
		return decorateMindnoteError(err)
	}
	if output == "json" {
		return printJSONTo(cmd.OutOrStdout(), data)
	}
	w := cmd.OutOrStdout()
	ids, _ := data["ids"].([]any)
	fmt.Fprintf(w, "已提交思维笔记 %s 的 %d 个节点\n", mindnoteID, len(body["nodes"].([]any)))
	if len(ids) > 0 {
		fmt.Fprintln(w, "节点 ID:")
		for _, id := range ids {
			fmt.Fprintf(w, "  %s\n", mindnoteScalarText(id))
		}
	}
	if ct, _ := data["client_token"].(string); ct != "" {
		fmt.Fprintf(w, "client_token: %s\n", ct)
	}
	return nil
}

// buildMindnoteCreateBody 读取并校验 --data / --client-token（离线）。
func buildMindnoteCreateBody(cmd *cobra.Command) (map[string]any, error) {
	if strings.TrimSpace(flagString(cmd, "data")) == "" {
		return nil, clierr.Usagef("缺少 --data：传 JSON 请求体（内联、@文件 或 - 读 stdin），至少包含非空的 nodes 数组")
	}
	rawData, err := readSlidesFlagInput(cmd, "data")
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(rawData)))
	dec.UseNumber()
	var parsed any
	if err := dec.Decode(&parsed); err != nil {
		if err == io.EOF {
			return nil, clierr.Usagef("--data 内容为空：至少包含非空的 nodes 数组")
		}
		return nil, clierr.Usagef("--data 不是合法 JSON: %v", err)
	}
	if err := ensureNoTrailingJSON(dec, "--data"); err != nil {
		return nil, clierr.Usage(err)
	}
	body, ok := parsed.(map[string]any)
	if !ok {
		return nil, clierr.Usagef("--data 必须是 JSON 对象，例如 {\"nodes\":[...]}")
	}
	nodes, ok := body["nodes"].([]any)
	if !ok || len(nodes) == 0 {
		return nil, clierr.Usagef("--data.nodes 必须是非空数组（新增子节点传 parent_id，更新已有节点传 node_id）")
	}
	for i, n := range nodes {
		node, ok := n.(map[string]any)
		if !ok {
			return nil, clierr.Usagef("--data.nodes[%d] 必须是 JSON 对象", i)
		}
		if h, exists := node["highlight"]; exists && h != nil {
			hs, _ := h.(string)
			if err := validateEnum(hs, fmt.Sprintf("--data.nodes[%d].highlight", i), mindnoteHighlights); err != nil {
				return nil, err
			}
		}
	}
	if v, exists := body["client_token"]; exists {
		if s, ok := v.(string); !ok || strings.TrimSpace(s) == "" {
			return nil, clierr.Usagef("--data.client_token 必须是非空字符串")
		}
	}
	if ct := strings.TrimSpace(flagString(cmd, "client-token")); ct != "" {
		if existing, ok := body["client_token"].(string); ok && existing != ct {
			return nil, clierr.Usagef("--client-token %q 与 --data.client_token %q 不一致，只需提供其中一个", ct, existing)
		}
		body["client_token"] = ct
	}
	return body, nil
}

// renderMindnoteNodes 按 parent_id 把节点渲染成缩进树；父节点不在列表中的视为根。
func renderMindnoteNodes(w io.Writer, mindnoteID string, data map[string]any) {
	nodes, _ := data["nodes"].([]any)
	fmt.Fprintf(w, "思维笔记 %s：%d 个节点\n", mindnoteID, len(nodes))
	if len(nodes) == 0 {
		return
	}
	byID := map[string]map[string]any{}
	var order []map[string]any
	for _, n := range nodes {
		node, ok := n.(map[string]any)
		if !ok {
			continue
		}
		order = append(order, node)
		if id := mindnoteScalarText(node["node_id"]); id != "" {
			byID[id] = node
		}
	}
	children := map[string][]map[string]any{}
	var roots []map[string]any
	for _, node := range order {
		parent := mindnoteScalarText(node["parent_id"])
		if _, ok := byID[parent]; parent != "" && ok && parent != mindnoteScalarText(node["node_id"]) {
			children[parent] = append(children[parent], node)
			continue
		}
		roots = append(roots, node)
	}
	visited := map[string]bool{}
	var walk func(node map[string]any, depth int)
	walk = func(node map[string]any, depth int) {
		id := mindnoteScalarText(node["node_id"])
		if id != "" {
			if visited[id] {
				return // 防御异常数据中的环
			}
			visited[id] = true
		}
		indent := strings.Repeat("  ", depth)
		text := mindnoteRichText(node["texts"])
		if text == "" {
			text = "（空节点）"
		}
		line := fmt.Sprintf("%s- %s  [%s]", indent, text, id)
		var marks []string
		if finish, _ := node["finish"].(bool); finish {
			marks = append(marks, "已完成")
		}
		if h, _ := node["highlight"].(string); h != "" {
			marks = append(marks, "高亮:"+h)
		}
		if imgs, _ := node["images"].([]any); len(imgs) > 0 {
			marks = append(marks, fmt.Sprintf("图片×%d", len(imgs)))
		}
		if len(marks) > 0 {
			line += "  (" + strings.Join(marks, ", ") + ")"
		}
		fmt.Fprintln(w, line)
		if note := mindnoteRichText(node["notes"]); note != "" {
			fmt.Fprintf(w, "%s    备注: %s\n", indent, note)
		}
		for _, child := range children[id] {
			walk(child, depth+1)
		}
	}
	for _, root := range roots {
		walk(root, 0)
	}
}

// mindnoteRichText 把 texts / notes 富文本元素数组拼成纯文本（尽力而为，未知结构回退到常见字段）。
func mindnoteRichText(v any) string {
	elems, _ := v.([]any)
	var b strings.Builder
	for _, e := range elems {
		el, ok := e.(map[string]any)
		if !ok {
			continue
		}
		etype, _ := el["element_type"].(string)
		switch etype {
		case "user":
			if s := mindnoteFirstField(el["mention_user"], "name", "user_name", "user_id", "id"); s != "" {
				b.WriteString("@" + s)
			}
		case "doc":
			b.WriteString(mindnoteFirstField(el["mention_doc"], "title", "name", "url", "token"))
		case "link":
			b.WriteString(mindnoteFirstField(el["link"], "text", "content", "title", "url"))
		default:
			b.WriteString(mindnoteFirstField(el["text"], "content", "text"))
		}
	}
	return strings.TrimSpace(strings.ReplaceAll(b.String(), "\n", " "))
}

func mindnoteFirstField(v any, keys ...string) string {
	m, ok := v.(map[string]any)
	if !ok {
		if s, ok := v.(string); ok {
			return s
		}
		return ""
	}
	for _, k := range keys {
		// 富文本片段之间的空格有意义：判空时去空白，返回原文
		if s, ok := m[k].(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
		if s := mindnoteScalarText(m[k]); s != "" {
			return s
		}
	}
	return ""
}

func mindnoteScalarText(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case json.Number:
		return x.String()
	case nil:
		return ""
	case map[string]any, []any:
		b, _ := json.Marshal(x)
		return string(b)
	default:
		return fmt.Sprint(x)
	}
}
