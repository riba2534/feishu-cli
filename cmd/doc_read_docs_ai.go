package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/converter"
	"github.com/spf13/cobra"
)

// doc read 的 docs_ai 引擎（POST /open-apis/docs_ai/v1/documents/{id}/fetch，对齐官方 docs +fetch）。
//
// 默认的本地引擎（块树 → 本地 Markdown）行为不变；以下任一新 flag 触发 docs_ai 引擎：
// --engine docs_ai、--with-ids、--scope、--detail、--doc-format、--start-block-id、--end-block-id、
// --context-before、--context-after、--max-depth、--revision-id。
// docs_ai 输出的 block id 可直接用于 `doc content-update --block-id / --start-block-id`。

// docsFetchExtraParam 与官方一致：@人引用表、评论旁路数据、HTML5 块数据。
const docsFetchExtraParam = `{"enable_user_cite_reference_map":true,"include_comments":true,"return_html5_block_data":true}`

// docsAIReadFlags 是只属于 docs_ai 引擎的 flag。
var docsAIReadFlags = []string{
	"with-ids", "scope", "detail", "doc-format", "start-block-id", "end-block-id",
	"context-before", "context-after", "max-depth", "revision-id", "output",
}

type docsAIReadOptions struct {
	format        string
	detail        string
	scope         string
	startBlockID  string
	endBlockID    string
	keyword       string
	contextBefore int
	contextAfter  int
	maxDepth      int
	revisionID    int
	output        string
}

// wantsDocsAIRead 判断是否走 docs_ai 引擎。
func wantsDocsAIRead(cmd *cobra.Command) (bool, error) {
	engine, _ := cmd.Flags().GetString("engine")
	engine = strings.ToLower(strings.TrimSpace(engine))
	switch engine {
	case "", "local":
	case "docs_ai", "docs-ai", "docsai":
		return true, nil
	default:
		return false, clierr.Usagef("不支持的 --engine %q，可选 local（默认）或 docs_ai", engine)
	}
	var used []string
	for _, name := range docsAIReadFlags {
		if cmd.Flags().Changed(name) {
			used = append(used, "--"+name)
		}
	}
	if len(used) == 0 {
		return false, nil
	}
	if cmd.Flags().Changed("engine") { // 显式 --engine local 却用了 docs_ai 专属 flag
		return false, clierr.Usagef("%s 只用于 --engine docs_ai", strings.Join(used, ", "))
	}
	return true, nil
}

// buildDocsAIReadOptions 解析并校验 docs_ai 读取参数（离线）。
func buildDocsAIReadOptions(cmd *cobra.Command) (*docsAIReadOptions, error) {
	f := cmd.Flags()
	o := &docsAIReadOptions{}
	withIDs, _ := f.GetBool("with-ids")
	o.format, _ = f.GetString("doc-format")
	o.detail, _ = f.GetString("detail")
	o.scope, _ = f.GetString("scope")
	o.startBlockID, _ = f.GetString("start-block-id")
	o.endBlockID, _ = f.GetString("end-block-id")
	o.contextBefore, _ = f.GetInt("context-before")
	o.contextAfter, _ = f.GetInt("context-after")
	o.maxDepth, _ = f.GetInt("max-depth")
	o.revisionID, _ = f.GetInt("revision-id")
	o.output, _ = f.GetString("output")
	o.format = strings.ToLower(strings.TrimSpace(o.format))
	o.detail = strings.ToLower(strings.TrimSpace(o.detail))
	o.scope = strings.ToLower(strings.TrimSpace(o.scope))
	o.startBlockID = strings.TrimSpace(o.startBlockID)
	o.endBlockID = strings.TrimSpace(o.endBlockID)
	o.output = strings.ToLower(strings.TrimSpace(o.output))

	if o.output != "" && o.output != "json" {
		return nil, clierr.Usagef("不支持的 --output %q，仅支持 json", o.output)
	}
	if withIDs {
		if o.detail != "" && o.detail != "with-ids" && o.detail != "full" {
			return nil, clierr.Usagef("--with-ids 与 --detail %s 冲突", o.detail)
		}
		if o.detail == "" {
			o.detail = "with-ids"
		}
	}
	if o.detail == "" {
		o.detail = "simple"
	}
	switch o.detail {
	case "simple", "with-ids", "full":
	default:
		return nil, clierr.Usagef("不支持的 --detail %q，可选 simple / with-ids / full", o.detail)
	}
	if o.format == "" {
		// block id 只能在 XML 中表达；其余默认 Markdown（更适合阅读）
		if o.detail != "simple" {
			o.format = "xml"
		} else {
			o.format = "markdown"
		}
	}
	if o.format != "xml" && o.format != "markdown" {
		return nil, clierr.Usagef("不支持的 --doc-format %q，可选 xml / markdown", o.format)
	}
	if o.format == "markdown" && o.detail != "simple" {
		return nil, clierr.Usagef("--detail %s（含 --with-ids）只支持 --doc-format xml；Markdown 无法携带 block id", o.detail)
	}

	// 本地引擎的 --outline / --heading / --keyword 映射到 docs_ai 的 scope
	outline, _ := f.GetBool("outline")
	heading, _ := f.GetString("heading")
	keyword, _ := f.GetString("keyword")
	o.keyword = strings.TrimSpace(keyword)
	modeFlags := 0
	for _, on := range []bool{outline, heading != "", o.keyword != ""} {
		if on {
			modeFlags++
		}
	}
	if modeFlags > 1 {
		return nil, clierr.Usagef("--outline / --heading / --keyword 只能选择其中一种")
	}
	implied := ""
	switch {
	case outline:
		implied = "outline"
	case heading != "":
		implied = "section"
	case o.keyword != "":
		implied = "keyword"
	}
	if o.scope != "" && implied != "" && o.scope != implied {
		return nil, clierr.Usagef("--scope %s 与 --outline/--heading/--keyword 隐含的范围 %s 冲突", o.scope, implied)
	}
	if o.scope == "" {
		o.scope = implied
	}
	if o.scope == "" {
		if o.startBlockID != "" || o.endBlockID != "" {
			o.scope = "range"
		} else {
			o.scope = "full"
		}
	}
	if f.Changed("context") && !f.Changed("context-before") && !f.Changed("context-after") && o.scope == "keyword" {
		// 本地 --context 是"行"，docs_ai 是"兄弟块"；显式 --context 时按块数映射，便于沿用旧用法
		c, _ := f.GetInt("context")
		o.contextBefore, o.contextAfter = c, c
	}

	if o.contextBefore < 0 || o.contextAfter < 0 {
		return nil, clierr.Usagef("--context-before / --context-after 必须 >= 0")
	}
	if o.maxDepth < -1 {
		return nil, clierr.Usagef("--max-depth 必须 >= -1（-1 表示不限）")
	}
	switch o.scope {
	case "full", "outline":
	case "range":
		if o.startBlockID == "" && o.endBlockID == "" {
			return nil, clierr.Usagef("--scope range 需要 --start-block-id 或 --end-block-id")
		}
	case "keyword":
		if o.keyword == "" {
			return nil, clierr.Usagef("--scope keyword 需要 --keyword（支持 a|b 多个关键词）")
		}
	case "section":
		if o.startBlockID == "" && heading == "" {
			return nil, clierr.Usagef("--scope section 需要 --start-block-id（标题块 ID）或 --heading")
		}
	default:
		return nil, clierr.Usagef("不支持的 --scope %q，可选 full / outline / range / keyword / section", o.scope)
	}
	return o, nil
}

// buildDocsAIFetchBody 组装 docs_ai fetch 请求体（对齐官方 buildFetchBody）。
func buildDocsAIFetchBody(o *docsAIReadOptions) map[string]any {
	body := map[string]any{
		"format":      o.format,
		"extra_param": docsFetchExtraParam,
	}
	if o.revisionID > 0 {
		body["revision_id"] = o.revisionID
	}
	switch o.detail {
	case "with-ids":
		body["export_option"] = map[string]any{"export_block_id": true}
	case "full":
		body["export_option"] = map[string]any{
			"export_block_id": true, "export_style_attrs": true, "export_cite_extra_data": true,
		}
	default:
		body["export_option"] = map[string]any{
			"export_block_id": false, "export_style_attrs": false, "export_cite_extra_data": false,
		}
	}
	if o.scope != "full" {
		ro := map[string]any{"read_mode": o.scope}
		if o.startBlockID != "" {
			ro["start_block_id"] = o.startBlockID
		}
		if o.endBlockID != "" {
			ro["end_block_id"] = o.endBlockID
		}
		if o.keyword != "" && o.scope == "keyword" {
			ro["keyword"] = o.keyword
		}
		if o.contextBefore > 0 {
			ro["context_before"] = strconv.Itoa(o.contextBefore)
		}
		if o.contextAfter > 0 {
			ro["context_after"] = strconv.Itoa(o.contextAfter)
		}
		if o.maxDepth >= 0 {
			ro["max_depth"] = strconv.Itoa(o.maxDepth)
		}
		body["read_option"] = ro
	}
	return body
}

// runDocReadDocsAI 执行 docs_ai 读取。
func runDocReadDocsAI(cmd *cobra.Command, documentID, userAccessToken string, o *docsAIReadOptions) error {
	if o.scope == "section" && o.startBlockID == "" {
		heading, _ := cmd.Flags().GetString("heading")
		id, err := resolveHeadingBlockID(cmd, documentID, userAccessToken, heading)
		if err != nil {
			return err
		}
		o.startBlockID = id
	}
	data, err := client.FetchDocsAI(documentID, buildDocsAIFetchBody(o), userAccessToken)
	if err != nil {
		return err
	}
	if o.output == "json" {
		return printJSONTo(cmd.OutOrStdout(), data)
	}
	content, _ := client.DocsAIDocumentContent(data)
	fmt.Fprintln(cmd.OutOrStdout(), content)
	for _, w := range client.DocsAIWarnings(data) {
		fmt.Fprintf(cmd.ErrOrStderr(), "⚠ 服务端警告: %s\n", w)
	}
	return nil
}

// resolveHeadingBlockID 按标题子串找到标题块 ID（docs_ai section 模式的锚点）。
func resolveHeadingBlockID(cmd *cobra.Command, documentID, userAccessToken, heading string) (string, error) {
	blocks, err := client.GetAllBlocksWithToken(documentID, userAccessToken)
	if err != nil {
		return "", fmt.Errorf("获取块失败: %w", err)
	}
	var ids, texts []string
	for _, b := range blocks {
		_, text, ok := converter.HeadingInfo(b)
		if !ok || !strings.Contains(text, heading) {
			continue
		}
		ids = append(ids, client.StringVal(b.BlockId))
		texts = append(texts, text)
	}
	if len(ids) == 0 {
		return "", fmt.Errorf("未找到包含 %q 的标题（用 --outline 查看全部标题）", heading)
	}
	if len(ids) > 1 {
		fmt.Fprintf(cmd.ErrOrStderr(), "提示: 有 %d 个标题匹配 %q，已读取第一个；其余: %s\n",
			len(ids), heading, strings.Join(texts[1:], " / "))
	}
	return ids[0], nil
}
