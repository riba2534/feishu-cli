package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
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
  未传 --content 时仍走原有的本地空文档创建；带图表/本地图片的完整 Markdown 建议用 doc import。

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
  feishu-cli doc create --title "周报" --content-file weekly.md -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		title, _ := cmd.Flags().GetString("title")
		folder, _ := cmd.Flags().GetString("folder")
		userAccessToken := resolveOptionalUserToken(cmd)

		if cmd.Flags().Changed("content") || cmd.Flags().Changed("content-file") ||
			cmd.Flags().Changed("parent-token") || cmd.Flags().Changed("parent-position") || cmd.Flags().Changed("doc-format") {
			return runDocCreateDocsAI(cmd, userAccessToken)
		}
		if strings.TrimSpace(title) == "" {
			return clierr.Usagef("--title 不能为空（或用 --content/--content-file 由服务端按内容创建）")
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
}
