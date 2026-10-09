package cmd

import (
	"encoding/xml"
	"fmt"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

// buildDocsAICreateBody 组装 docs_ai 建文档请求体（离线，可在任何网络请求前完成全部校验），
// 同时返回内容中待上传的本地图片/附件（建文档成功后上传并绑定）。
func buildDocsAICreateBody(cmd *cobra.Command) (map[string]any, []*localDocResource, error) {
	f := cmd.Flags()
	title, _ := f.GetString("title")
	folder, _ := f.GetString("folder")
	content, _ := f.GetString("content")
	contentFile, _ := f.GetString("content-file")
	format, _ := f.GetString("doc-format")
	parentToken, _ := f.GetString("parent-token")
	parentPosition, _ := f.GetString("parent-position")
	output, _ := f.GetString("output")

	format = strings.ToLower(strings.TrimSpace(format))
	if format != "markdown" && format != "xml" {
		return nil, nil, clierr.Usagef("不支持的 --doc-format %q，可选 markdown / xml", format)
	}
	if output != "" && output != "json" {
		return nil, nil, clierr.Usagef("不支持的 --output %q，仅支持 json", output)
	}
	if f.Changed("title") && strings.TrimSpace(title) == "" {
		return nil, nil, clierr.Usagef("--title 不能为空")
	}
	if f.Changed("content") && f.Changed("content-file") {
		return nil, nil, clierr.Usagef("--content 与 --content-file 只能使用其中一个")
	}
	if folder != "" && parentToken != "" {
		return nil, nil, clierr.Usagef("--folder 与 --parent-token 只能使用其中一个")
	}
	if parentToken == "" {
		parentToken = folder
	}
	if parentToken != "" && parentPosition != "" {
		return nil, nil, clierr.Usagef("--parent-token/--folder 与 --parent-position 互斥")
	}
	if contentFile != "" {
		data, err := loadJSONInput("", contentFile, "content", "content-file", "文档内容")
		if err != nil {
			return nil, nil, err
		}
		content = data
	} else if content != "" {
		content = strings.ReplaceAll(content, "\\n", "\n")
	}
	if strings.TrimSpace(content) == "" && strings.TrimSpace(title) == "" {
		return nil, nil, clierr.Usagef("--content/--content-file 为空且未提供 --title；请提供内容或标题")
	}
	var refMap map[string]any
	if f.Changed("reference-map") {
		if strings.TrimSpace(content) == "" {
			return nil, nil, clierr.Usagef("--reference-map 需要与 --content/--content-file 一起使用")
		}
		raw, _ := f.GetString("reference-map")
		m, err := readReferenceMapFlag(raw, true)
		if err != nil {
			return nil, nil, err
		}
		refMap = m
	}
	if format == "markdown" && content != "" {
		converted, conv, err := convertLocalDialectForDocsAI(content)
		if err != nil {
			return nil, nil, err
		}
		if s := conv.summary(); s != "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "提示: 已将 doc export 本地方言转换为 docs_ai 写法（%s）\n", s)
		}
		content = converted
	}
	// 本地图片/附件 → 占位标签；html5-block / 画板本地文件 → 内联（严格校验，对齐官方 docs +create）
	var resources []*localDocResource
	if content != "" {
		in, err := prepareDocsAIWriteInput(content, refMap, docsAIWriteOptions{Format: format, SourceFile: contentFile, Strict: true})
		if err != nil {
			return nil, nil, err
		}
		content, refMap, resources = in.Content, in.ReferenceMap, in.Resources
	}
	// --title 以 <title> 前置，优先于内容中的标题（与官方一致）
	if t := strings.TrimSpace(title); t != "" {
		var b strings.Builder
		_ = xml.EscapeText(&b, []byte(t))
		tag := "<title>" + b.String() + "</title>"
		if content == "" {
			content = tag
		} else {
			content = tag + "\n" + content
		}
	}
	body := map[string]any{"format": format, "content": content}
	if len(refMap) > 0 {
		body["reference_map"] = refMap
	}
	if parentToken != "" {
		body["parent_token"] = parentToken
	}
	if parentPosition != "" {
		body["parent_position"] = parentPosition
	}
	return body, resources, nil
}

// runDocCreateDocsAI 服务端带内容建文档（docs_ai），含异步任务轮询与 Bot 自动授权。
func runDocCreateDocsAI(cmd *cobra.Command, userAccessToken string) error {
	body, resources, err := buildDocsAICreateBody(cmd)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")
	if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
		return printDocCreateDryRun(cmd, body, resources, userAccessToken)
	}

	data, createErr := client.CreateDocsAIDocument(body, userAccessToken)
	if data == nil && createErr != nil {
		return createErr
	}
	doc, _ := data["document"].(map[string]any)
	if doc == nil {
		doc = map[string]any{}
	}
	documentID, _ := doc["document_id"].(string)
	link, _ := doc["url"].(string)
	if documentID != "" && strings.TrimSpace(link) == "" {
		link = client.BuildResourceURL(client.ResourceTypeDocx, documentID)
		doc["url"] = link
	}

	var grant *client.PermissionGrantResult
	if documentID != "" {
		grant = autoGrantCurrentUser(userAccessToken, documentID, client.ResourceTypeDocx)
	}

	// 本地图片/附件：文档建成（含 partial_success）后以占位块为父节点上传并绑定，失败项清理占位块
	res := &contentUpdateParams{documentID: documentID, userToken: userAccessToken, output: output,
		stdout: cmd.OutOrStdout(), stderr: cmd.ErrOrStderr(), resources: resources}
	var resErr error
	if len(resources) > 0 && documentID != "" {
		if rerr, ok := client.AsDocsAIResultError(createErr); createErr == nil || (ok && rerr.Result == "partial_success") {
			if !finalizeLocalDocResources(res, data, resources) {
				resErr = res.reportLocalResources(nil, resources, false)
			}
		}
	}

	out := map[string]any{
		"engine":      "docs_ai",
		"document_id": documentID,
		"url":         link,
		"revision_id": doc["revision_id"],
		"document":    doc,
	}
	for _, k := range []string{"result", "warnings", "log_id", "new_blocks", "local_resource_failures"} {
		if v, ok := data[k]; ok {
			out[k] = v
		}
	}
	if len(resources) > 0 {
		out["local_resources"] = resources
	}
	if output == "json" {
		if err := printJSON(withPermissionGrant(out, grant)); err != nil {
			return err
		}
	} else if documentID != "" {
		fmt.Println("文档创建成功！（docs_ai）")
		fmt.Printf("  文档 ID: %s\n", documentID)
		fmt.Printf("  版本: %v\n", doc["revision_id"])
		fmt.Printf("  链接: %s\n", link)
		printPermissionGrantText(os.Stdout, grant)
		for _, w := range client.DocsAIWarnings(data) {
			fmt.Fprintf(cmd.ErrOrStderr(), "⚠ 服务端警告: %s\n", w)
		}
		res.printLocalResourceLines()
	}
	if createErr != nil {
		return createErr
	}
	if documentID == "" {
		return fmt.Errorf("创建文档成功但响应缺少 document_id")
	}
	return resErr
}
