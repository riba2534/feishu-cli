// 部分实现改编自 larksuite/cli（MIT License, Copyright (c) 2026 Lark Technologies Pte. Ltd.）

package cmd

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

// docs_ai 写入（doc create / doc content-update）的 --dry-run 计划：不联网、不解析身份，
// 打印将发出的请求、待上传的本地资源与绑定计划（对齐官方 dryRunCreateV2 / dryRunUpdateV2）。

// localResourcesDryRunSteps 生成本地资源的上传、绑定与条件清理步骤。docKey 为文档 ID 或占位符。
func localResourcesDryRunSteps(docKey string, resources []*localDocResource) []dryRunStep {
	if len(resources) == 0 {
		return nil
	}
	extra, _ := jsonMarshalNoEscape(map[string]string{"drive_route_token": docKey})
	enc := dryRunPathSegment(docKey)
	var steps []dryRunStep
	for _, r := range resources {
		if r.RemoteURL != "" {
			steps = append(steps, dryRunStep{Method: "GET", URL: r.URL,
				Desc: fmt.Sprintf("下载远程图片 #%d（文档写入成功后；受控客户端，≤20MiB，最多 5 次重定向；已隐去 query）", r.Occurrence)})
		}
		parentType := "docx_image"
		if r.Kind == "file" {
			parentType = "docx_file"
		}
		body := map[string]any{
			"file_name":   r.FileName,
			"parent_type": parentType,
			"parent_node": dryRunResourceBlockID(r),
			"size":        r.Size,
			"extra":       extra,
		}
		label := fmt.Sprintf("上传本地%s #%d（%s）", resourceKindLabel(r.Kind), r.Occurrence, r.Path)
		if r.RemoteURL != "" {
			body["file_name"], body["size"] = fmt.Sprintf("<remote_image_%d_filename>", r.Occurrence), "<downloaded_size>"
			label = fmt.Sprintf("上传远程图片 #%d", r.Occurrence)
		}
		if client.DriveNeedsMultipart(r.Size) {
			steps = append(steps,
				dryRunStep{Method: "POST", URL: "/open-apis/drive/v1/medias/upload_prepare", Desc: label + "：初始化分片上传", Body: body},
				dryRunStep{Method: "POST", URL: "/open-apis/drive/v1/medias/upload_part", Desc: label + "：上传分片（重复）",
					Body: map[string]any{"upload_id": "<upload_id>", "seq": "<chunk_index>", "size": "<chunk_size>", "file": "<本地文件分片>"}},
				dryRunStep{Method: "POST", URL: "/open-apis/drive/v1/medias/upload_finish", Desc: label + "：完成分片上传",
					Body: map[string]any{"upload_id": "<upload_id>", "block_num": "<block_num>"}},
			)
			continue
		}
		body["file"] = "<本地文件内容>"
		steps = append(steps, dryRunStep{Method: "POST", URL: "/open-apis/drive/v1/medias/upload_all", Desc: label, Body: body})
	}
	for start := 0; start < len(resources); start += localResourceBindBatch {
		end := min(start+localResourceBindBatch, len(resources))
		var reqs []any
		for _, r := range resources[start:end] {
			token := fmt.Sprintf("<uploaded_file_token_%d>", r.Occurrence)
			if r.Kind == "file" {
				reqs = append(reqs, map[string]any{"block_id": dryRunResourceBlockID(r), "replace_file": map[string]any{"token": token}})
				continue
			}
			reqs = append(reqs, map[string]any{"block_id": dryRunResourceBlockID(r), "replace_image": r.replaceImageRequest(token)})
		}
		steps = append(steps, dryRunStep{
			Method: "PATCH",
			URL:    fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks/batch_update", enc),
			Desc:   fmt.Sprintf("绑定已上传的本地资源（第 %d 批，每批最多 %d 个）", start/localResourceBindBatch+1, localResourceBindBatch),
			Params: map[string]any{"client_token": fmt.Sprintf("<client_token_%d>", start/localResourceBindBatch+1)},
			Body:   map[string]any{"requests": reqs},
		})
	}
	steps = append(steps, dryRunStep{
		Method: "PUT",
		URL:    "/open-apis/docs_ai/v1/documents/" + enc,
		Desc:   "条件：有资源上传/绑定失败时删除其占位块（附件删除外层视图块），成功项保留",
		Body:   map[string]any{"format": "markdown", "command": "block_delete", "block_id": "<failed_placeholder_block_ids>", "revision_id": -1},
	})
	return steps
}

// dryRunPathSegment 转义路径段；<占位符> 原样保留便于阅读。
func dryRunPathSegment(s string) string {
	if strings.HasPrefix(s, "<") && strings.HasSuffix(s, ">") {
		return s
	}
	return url.PathEscape(s)
}

func dryRunResourceBlockID(r *localDocResource) string {
	return fmt.Sprintf("<local_%s_%d_block_id>", r.Kind, r.Occurrence)
}

func resourceKindLabel(kind string) string {
	if kind == "file" {
		return "附件"
	}
	return "图片"
}

// localResourcesDryRunSummary 是 dry-run 输出中的本地资源清单。
func localResourcesDryRunSummary(resources []*localDocResource) []map[string]any {
	var out []map[string]any
	for _, r := range resources {
		item := map[string]any{"occurrence": r.Occurrence, "kind": r.Kind, "path": r.Path, "file_name": r.FileName, "size": r.Size}
		if r.RemoteURL != "" {
			item = map[string]any{"occurrence": r.Occurrence, "kind": r.Kind, "url": r.URL, "source": "remote"}
			for _, a := range r.requested {
				item["requested_"+a.Name] = a.Value
			}
		}
		if r.Width > 0 {
			item["width"] = r.Width
		}
		if r.Height > 0 {
			item["height"] = r.Height
		}
		if r.HasScale {
			item["scale"] = r.Scale
		}
		out = append(out, item)
	}
	return out
}

// printDocCreateDryRun 打印 doc create（docs_ai）的请求计划。
func printDocCreateDryRun(cmd *cobra.Command, body map[string]any, resources []*localDocResource, userAccessToken string) error {
	if _, ok := body["extra_param"]; !ok {
		body["extra_param"] = `{"open_create_async":true}`
	}
	steps := []dryRunStep{
		{Method: "POST", URL: "/open-apis/docs_ai/v1/documents", Desc: "docs_ai 创建文档", Body: body},
		{Method: "GET", URL: "/open-apis/docs_ai/v1/async_tasks/<task_id>", Desc: "条件：创建返回异步任务时轮询结果（只重试查询，不重复创建）"},
	}
	if strings.TrimSpace(userAccessToken) == "" {
		steps = append(steps, dryRunStep{Method: "POST", URL: "/open-apis/drive/v1/permissions/<created_document_id>/members",
			Desc: "条件：Bot 身份创建成功后，为当前 CLI 登录用户授予 full_access", Params: map[string]any{"type": "docx"}})
	}
	steps = append(steps, localResourcesDryRunSteps("<created_document_id>", resources)...)
	extra := map[string]any{}
	if len(resources) > 0 {
		extra["local_resources"] = localResourcesDryRunSummary(resources)
	}
	return printDryRunPlan(cmd, "doc create（docs_ai）预览：未发送任何请求", extra, steps)
}

// printContentUpdateDryRun 打印 doc content-update 的请求计划（定位类模式的块 ID 以占位符表示）。
func printContentUpdateDryRun(cmd *cobra.Command, p *contentUpdateParams, rawDoc string) error {
	ref, err := parseResourceArg(rawDoc, resourceArgOptions{ArgName: "<document_id|url>", DefaultType: client.ResourceTypeDocx, Allowed: []string{client.ResourceTypeDocx, client.ResourceTypeWiki}})
	if err != nil {
		return err
	}
	docID := ref.Token
	var steps []dryRunStep
	if ref.InputType == client.ResourceTypeWiki {
		docID = "<wiki 解析后的 docx token>"
		steps = append(steps, dryRunStep{Method: "GET", URL: client.WikiNodeByTokenPath, Desc: "解析知识库节点为底层 docx 文档",
			Params: map[string]any{"token": ref.InputToken}})
	}
	p.documentID = docID
	enc := dryRunPathSegment(docID)
	childrenStep := dryRunStep{Method: "GET", URL: fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks/%s/children", enc, enc),
		Desc: "读取文档顶层块，按选择器定位目标块"}
	put := func(desc string, body map[string]any) dryRunStep {
		injectRevisionID(body, p.revisionID)
		p.injectReferenceMap(body)
		return dryRunStep{Method: "PUT", URL: "/open-apis/docs_ai/v1/documents/" + enc, Desc: desc, Body: body}
	}
	withContent := func(command string) map[string]any {
		body := p.newBody(command)
		body["content"] = p.content
		return body
	}
	textLevel := isPlainTextSelector(p.selEllipsis) && (p.mode == "replace_all" || p.mode == "replace_range" || p.mode == "delete_range")

	switch {
	case p.mode == "append":
		body := withContent("block_insert_after")
		body["block_id"] = "-1"
		steps = append(steps, put("追加到文档末尾", body))
	case p.mode == "overwrite":
		steps = append(steps, put("覆盖整篇文档", withContent("overwrite")))
	case p.mode == "str_replace" || textLevel:
		pattern, desc := p.pattern, "文本级替换唯一命中"
		if p.mode != "str_replace" {
			pattern = strings.TrimSpace(p.selEllipsis)
			if p.mode == "replace_all" {
				desc = "文本级替换：每处命中一次（pattern 会扩展为全文唯一的上下文窗口）"
			}
		}
		steps = append(steps, dryRunStep{Method: "POST", URL: fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s/fetch", enc),
			Desc: "读取文档当前序列化内容，定位命中并构造唯一替换窗口"})
		body := p.newBody("str_replace")
		body["pattern"] = pattern
		body["content"] = p.content
		if p.mode == "delete_range" {
			body["content"] = ""
		}
		steps = append(steps, put(desc, body))
	case p.mode == "block_move_after" || p.mode == "block_copy_insert_after":
		body := p.newBody(p.mode)
		body["block_id"] = p.blockID
		body["src_block_ids"] = p.srcBlockIDs
		steps = append(steps, put("移动/复制块", body))
	case p.mode == "replace_range" || p.mode == "delete_range" || p.mode == "replace_all":
		command := "block_replace"
		if p.mode == "delete_range" {
			command = "block_delete"
		}
		body := p.newBody(command)
		if command == "block_replace" {
			body["content"] = p.content
		}
		desc := "块级替换"
		if command == "block_delete" {
			desc = "块级删除"
		}
		switch {
		case p.blockID != "":
			body["block_id"] = p.blockID
		case p.startBlockID != "":
			body["start_block_id"], body["end_block_id"] = p.startBlockID, p.endBlockID
		default:
			steps = append(steps, childrenStep)
			body["start_block_id"], body["end_block_id"] = "<定位到的起始块>", "<定位到的结束块>"
			if p.mode == "replace_all" {
				desc = "块级替换：每个命中范围一次（倒序执行）"
			}
		}
		steps = append(steps, put(desc, body))
	case p.mode == "insert_before" || p.mode == "insert_after":
		body := withContent("block_insert_after")
		switch {
		case p.mode == "insert_after" && p.blockID != "":
			body["block_id"] = p.blockID
		case p.mode == "insert_before" && p.blockID != "":
			steps = append(steps,
				dryRunStep{Method: "GET", URL: fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks/%s", enc, url.PathEscape(p.blockID)), Desc: "读取锚点块的父块"},
				dryRunStep{Method: "GET", URL: fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks/<parent_block_id>/children", enc), Desc: "读取兄弟块，取锚点前一个块"})
			body["block_id"] = "<锚点块的前一个兄弟块，首块时为 0>"
		default:
			steps = append(steps, childrenStep)
			body["block_id"] = "<定位到的锚点块>"
		}
		steps = append(steps, put("插入内容", body))
	}
	steps = append(steps, localResourcesDryRunSteps(docID, p.resources)...)
	extra := map[string]any{"mode": p.mode}
	if len(p.resources) > 0 {
		extra["local_resources"] = localResourcesDryRunSummary(p.resources)
	}
	return printDryRunPlan(cmd, "doc content-update 预览：未发送任何请求", extra, steps)
}
