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

// buildDocsAICreateBody 组装 docs_ai 建文档请求体（离线，可在任何网络请求前完成全部校验）。
func buildDocsAICreateBody(cmd *cobra.Command) (map[string]any, error) {
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
		return nil, clierr.Usagef("不支持的 --doc-format %q，可选 markdown / xml", format)
	}
	if output != "" && output != "json" {
		return nil, clierr.Usagef("不支持的 --output %q，仅支持 json", output)
	}
	if f.Changed("title") && strings.TrimSpace(title) == "" {
		return nil, clierr.Usagef("--title 不能为空")
	}
	if f.Changed("content") && f.Changed("content-file") {
		return nil, clierr.Usagef("--content 与 --content-file 只能使用其中一个")
	}
	if folder != "" && parentToken != "" {
		return nil, clierr.Usagef("--folder 与 --parent-token 只能使用其中一个")
	}
	if parentToken == "" {
		parentToken = folder
	}
	if parentToken != "" && parentPosition != "" {
		return nil, clierr.Usagef("--parent-token/--folder 与 --parent-position 互斥")
	}
	if contentFile != "" {
		data, err := loadJSONInput("", contentFile, "content", "content-file", "文档内容")
		if err != nil {
			return nil, err
		}
		content = data
	} else if content != "" {
		content = strings.ReplaceAll(content, "\\n", "\n")
	}
	if strings.TrimSpace(content) == "" && strings.TrimSpace(title) == "" {
		return nil, clierr.Usagef("--content/--content-file 为空且未提供 --title；请提供内容或标题")
	}
	if format == "markdown" && content != "" {
		if err := validateNoLocalResources(false, content); err != nil {
			return nil, clierr.Usagef("doc create --content 暂不支持本地图片/附件；请改用网络图片 URL，或用 'feishu-cli doc import' 导入带本地图片的 Markdown")
		}
		converted, conv, err := convertLocalDialectForDocsAI(content)
		if err != nil {
			return nil, err
		}
		if s := conv.summary(); s != "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "提示: 已将 doc export 本地方言转换为 docs_ai 写法（%s）\n", s)
		}
		content = converted
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
	if parentToken != "" {
		body["parent_token"] = parentToken
	}
	if parentPosition != "" {
		body["parent_position"] = parentPosition
	}
	return body, nil
}

// runDocCreateDocsAI 服务端带内容建文档（docs_ai），含异步任务轮询与 Bot 自动授权。
func runDocCreateDocsAI(cmd *cobra.Command, userAccessToken string) error {
	body, err := buildDocsAICreateBody(cmd)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")

	data, createErr := client.CreateDocsAIDocument(body, userAccessToken)
	if data == nil && createErr != nil {
		return createErr
	}
	doc, _ := data["document"].(map[string]any)
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

	out := map[string]any{
		"engine":      "docs_ai",
		"document_id": documentID,
		"url":         link,
		"revision_id": doc["revision_id"],
		"document":    doc,
	}
	for _, k := range []string{"result", "warnings", "log_id", "new_blocks"} {
		if v, ok := data[k]; ok {
			out[k] = v
		}
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
	}
	if createErr != nil {
		return createErr
	}
	if documentID == "" {
		return fmt.Errorf("创建文档成功但响应缺少 document_id")
	}
	return nil
}
