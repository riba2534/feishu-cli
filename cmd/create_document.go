package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var createDocumentCmd = &cobra.Command{
	Use:   "create",
	Short: "创建新文档",
	Long: `创建新的飞书云文档。

参数:
  --title, -t     文档标题（本地创建必填；带 --content 时可省略，由内容中的 <title>/首个一级标题决定）
  --folder, -f    目标文件夹 token（可选）
  --output, -o    输出格式，可选 json

服务端建文档（docs_ai，带初始内容，与官方 docs +create 同协议）:
  --content / --content-file   初始内容（默认 Markdown；--doc-format xml 时为 XML）
  --doc-format                 markdown（默认）| xml
  --parent-token               父文件夹 token 或知识库节点 token（与 --folder 二选一）
  --parent-position            父位置（如 my_library），与 --parent-token/--folder 互斥
  内容较大时服务端转为异步任务，CLI 自动轮询直至完成（最长 10 分钟，只重试查询、不重复创建）；
  超时或 expired 时提示分批：先创建部分内容，再用 doc content-update --mode append 追加。
  Markdown 中 doc export 产生的本地方言会先转换为 docs_ai 写法（规则同 doc content-update）。
  未传 --content 时仍走原有的本地空文档创建；带 Mermaid 代码块等需本地转换的完整 Markdown 建议用 doc import。

  本地资源（建文档成功后自动上传并绑定，失败项清理占位块并非零退出）:
    ![说明](@./a.png)、![说明](./a.png)（相对 --content-file 所在目录）、
    <img path="@./a.png" width="600"/>、<source path="@./report.pdf" name="报告.pdf"/>
    <img> 的 width/height 按图片真实像素归一化，给出的显示宽度换算为 scale（对齐官方）。
    网络图片 <img href="https://..."/>、![说明](https://...) 不经 CLI，原样交给服务端下载。
  本地 HTML / 画板源文件（markdown 与 xml 均可，围栏代码块内不处理）:
    <html5-block path="@./widget.html"/>             读取本地单文件 HTML 写入 HTML 块
    <whiteboard type="mermaid" path="@./flow.mmd"/>  读取本地源文件生成画板（svg|mermaid|plantuml）
  路径以 @ 开头、相对当前目录；--doc-format xml 且用 --content-file 时，当前目录不存在则回退到该文件所在目录。
  --reference-map              结构化 reference_map（内联 JSON、@file 或 - 读 stdin），需与 --content 一起使用
  --dry-run                    只打印将发出的请求与本地资源上传计划，不联网、不创建文档（不带内容时预览空白文档创建）

Bot 身份创建（未传 User Token）时，自动给当前 CLI 登录用户授予 full_access，
JSON 输出 permission_grant（granted/skipped/failed）；以 User 身份创建时不触发。

示例:
  # 创建空白文档
  feishu-cli doc create --title "我的文档"

  # 在指定文件夹创建
  feishu-cli doc create --title "项目文档" --folder FOLDER_TOKEN

  # JSON 格式输出
  feishu-cli doc create --title "测试" --output json

  # 服务端带内容创建（Markdown）
  feishu-cli doc create --title "周报" --content-file weekly.md -o json

  # 带本地图片、附件、HTML 组件与 Mermaid 画板创建（XML），先预览
  feishu-cli doc create --doc-format xml --content-file report.xml --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		title, _ := cmd.Flags().GetString("title")
		folder, _ := cmd.Flags().GetString("folder")
		userAccessToken := resolveOptionalUserToken(cmd)

		if cmd.Flags().Changed("content") || cmd.Flags().Changed("content-file") ||
			cmd.Flags().Changed("parent-token") || cmd.Flags().Changed("parent-position") || cmd.Flags().Changed("doc-format") ||
			cmd.Flags().Changed("reference-map") {
			return runDocCreateDocsAI(cmd, userAccessToken)
		}
		if strings.TrimSpace(title) == "" {
			return clierr.Usagef("--title 不能为空（或用 --content/--content-file 由服务端按内容创建）")
		}
		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			body := map[string]any{"title": title}
			if folder != "" {
				body["folder_token"] = folder
			}
			steps := []dryRunStep{{Method: "POST", URL: "/open-apis/docx/v1/documents", Desc: "创建空白文档", Body: body}}
			if strings.TrimSpace(userAccessToken) == "" {
				steps = append(steps, dryRunStep{Method: "POST", URL: "/open-apis/drive/v1/permissions/<created_document_id>/members",
					Desc: "条件：Bot 身份创建成功后，为当前 CLI 登录用户授予 full_access", Params: map[string]any{"type": "docx"}})
			}
			return printDryRunPlan(cmd, "doc create 预览：未发送任何请求", nil, steps)
		}

		doc, err := client.CreateDocument(title, folder, userAccessToken)
		if err != nil {
			return err
		}

		documentID := ""
		docTitle := ""
		var revisionID int
		if doc.DocumentId != nil {
			documentID = *doc.DocumentId
		}
		if doc.Title != nil {
			docTitle = *doc.Title
		}
		if doc.RevisionId != nil {
			revisionID = *doc.RevisionId
		}

		// Bot 身份创建时自动给当前 CLI 登录用户授予 full_access（User 身份创建不触发）
		grant := autoGrantCurrentUser(userAccessToken, documentID, client.ResourceTypeDocx)

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			if err := printJSON(withPermissionGrant(map[string]any{
				"document_id": documentID,
				"title":       docTitle,
				"revision_id": revisionID,
				"url":         client.BuildResourceURL(client.ResourceTypeDocx, documentID),
			}, grant)); err != nil {
				return err
			}
		} else {
			fmt.Printf("文档创建成功！\n")
			fmt.Printf("  文档 ID: %s\n", documentID)
			fmt.Printf("  标题: %s\n", docTitle)
			fmt.Printf("  版本: %d\n", revisionID)
			if link := client.BuildResourceURL(client.ResourceTypeDocx, documentID); link != "" {
				fmt.Printf("  链接: %s\n", link)
			}
			printPermissionGrantText(os.Stdout, grant)
		}

		return nil
	},
}

func init() {
	docCmd.AddCommand(createDocumentCmd)
	createDocumentCmd.Flags().StringP("title", "t", "", "文档标题（必填）")
	createDocumentCmd.Flags().StringP("folder", "f", "", "目标文件夹 token")
	createDocumentCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	createDocumentCmd.Flags().String("user-access-token", "", "User Access Token（可选，使用用户身份创建文档）")
	createDocumentCmd.Flags().String("content", "", "docs_ai: 初始内容（默认 Markdown）")
	createDocumentCmd.Flags().String("content-file", "", "docs_ai: 从文件读取初始内容")
	createDocumentCmd.Flags().String("doc-format", "markdown", "docs_ai: 内容格式 markdown | xml")
	createDocumentCmd.Flags().String("parent-token", "", "docs_ai: 父文件夹 token 或知识库节点 token")
	createDocumentCmd.Flags().String("parent-position", "", "docs_ai: 父位置（如 my_library）")
	createDocumentCmd.Flags().String("reference-map", "", "docs_ai: 结构化 reference_map JSON 对象（内联 JSON、@file 或 - 读 stdin），需与 --content 一起使用")
	createDocumentCmd.Flags().Bool("dry-run", false, "只打印将发出的请求（含本地资源上传计划），不联网、不创建文档")
}
