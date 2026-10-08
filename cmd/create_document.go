package cmd

import (
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var createDocumentCmd = &cobra.Command{
	Use:   "create",
	Short: "创建新文档",
	Long: `创建新的飞书云文档。

参数:
  --title, -t     文档标题（必填）
  --folder, -f    目标文件夹 token（可选）
  --output, -o    输出格式，可选 json

Bot 身份创建（未传 User Token）时，自动给当前 CLI 登录用户授予 full_access，
JSON 输出 permission_grant（granted/skipped/failed）；以 User 身份创建时不触发。

示例:
  # 创建空白文档
  feishu-cli doc create --title "我的文档"

  # 在指定文件夹创建
  feishu-cli doc create --title "项目文档" --folder FOLDER_TOKEN

  # JSON 格式输出
  feishu-cli doc create --title "测试" --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		title, _ := cmd.Flags().GetString("title")
		folder, _ := cmd.Flags().GetString("folder")
		userAccessToken := resolveOptionalUserToken(cmd)

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
	mustMarkFlagRequired(createDocumentCmd, "title")
}
