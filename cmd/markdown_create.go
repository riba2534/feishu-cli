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

var markdownCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "在 Drive 创建一个原生 Markdown (.md) 文件",
	Long: `把一段 Markdown 内容（或本地 .md 文件）作为普通 Drive 文件上传，保留原始 Markdown 格式。

底层调用 ` + "`POST /open-apis/drive/v1/files/upload_all`" + `；>20MB 自动走 upload_prepare/part/finish。
--wiki-token 时 parent_type=wiki。--as bot|user|auto（默认 auto）。

必填:
  --content / --content-file / --file  三选一（content 与 file 互斥）
  --name   使用 --content 时必填，且必须以 .md 结尾

可选:
  --folder-token   目标 Drive 文件夹（默认根目录；与 --wiki-token 互斥）
  --wiki-token     目标 wiki 节点
  --dry-run        只打印将要发出的请求
  --user-access-token  覆盖登录态

以 Bot 身份创建时，自动给当前 CLI 登录用户授予文件 full_access（JSON 输出 permission_grant）。

示例:
  feishu-cli markdown create --name plan.md --content "# Plan"
  feishu-cli markdown create --file ./local.md --folder-token fldxxx
  feishu-cli markdown create --name draft.md --content "# wiki" --wiki-token wikcnxxx --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		name, _ := cmd.Flags().GetString("name")
		content, _ := cmd.Flags().GetString("content")
		contentFile, _ := cmd.Flags().GetString("content-file")
		contentFileAlias, _ := cmd.Flags().GetString("file")
		folderToken, _ := cmd.Flags().GetString("folder-token")
		wikiToken, _ := cmd.Flags().GetString("wiki-token")
		output, _ := cmd.Flags().GetString("output")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		contentChanged := cmd.Flags().Changed("content")
		fileChanged := cmd.Flags().Changed("content-file") || cmd.Flags().Changed("file")

		var err error
		contentFile, err = resolveMarkdownFileFlag(contentFile, contentFileAlias)
		if err != nil {
			return err
		}
		if contentChanged && fileChanged {
			return clierr.Usagef("--content 与 --content-file/--file 不能同时使用")
		}
		if !contentChanged && !fileChanged {
			return clierr.Usagef("请提供 --content 或 --content-file")
		}
		folderToken = strings.TrimSpace(folderToken)
		wikiToken = strings.TrimSpace(wikiToken)
		if cmd.Flags().Changed("folder-token") && folderToken == "" {
			return clierr.Usagef("--folder-token 不能为空；省略该 flag 以上传到 Drive 根目录")
		}
		if cmd.Flags().Changed("wiki-token") && wikiToken == "" {
			return clierr.Usagef("--wiki-token 不能为空")
		}
		if folderToken != "" && wikiToken != "" {
			return clierr.Usagef("--folder-token 与 --wiki-token 互斥")
		}

		fileName, err := markdownCreateSpecName(name, contentFile)
		if err != nil {
			return err
		}

		var size int64
		if contentChanged {
			size = int64(len(content))
		} else {
			stat, err := os.Stat(contentFile)
			if err != nil {
				return clierr.Usagef("读取本地文件失败: %w", err)
			}
			if stat.IsDir() {
				return clierr.Usagef("--content-file 必须指向文件，不是目录")
			}
			if err := validateMarkdownFileName(fileName, "--name"); err != nil {
				return err
			}
			size = stat.Size()
		}
		if size == 0 {
			return clierr.Usagef("Markdown 内容为空，不支持创建空 .md 文件")
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}

		spec := client.MarkdownUploadSpec{
			FileName:    fileName,
			FolderToken: folderToken,
			WikiToken:   wikiToken,
		}
		parentType, parentNode := "explorer", folderToken
		if wikiToken != "" {
			parentType, parentNode = "wiki", wikiToken
		}

		if dryRun {
			multipart := markdownNeedsMultipart(size)
			steps := markdownUploadDryRunSteps(spec, size, multipart, contentFile)
			steps = append(steps, dryRunStep{
				Method: "POST",
				URL:    "/open-apis/drive/v1/metas/batch_query",
				Desc:   "Fetch the created Markdown file's real access URL",
				Body: map[string]any{
					"request_docs": []map[string]any{{
						"doc_token": "<file_token from upload response>",
						"doc_type":  "file",
					}},
					"with_url": true,
				},
			})
			return printDryRunPlan(cmd, "upload markdown file", map[string]any{
				"parent_type": parentType,
				"parent_node": parentNode,
				"size":        size,
			}, steps)
		}

		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		var result client.MarkdownUploadResult
		if fileChanged {
			result, err = client.UploadMarkdownFile(spec, contentFile, token)
		} else {
			result, err = client.UploadMarkdownContent(spec, []byte(content), token)
		}
		if err != nil {
			return err
		}

		out := map[string]any{
			"file_token": result.FileToken,
			"file_name":  fileName,
			"size_bytes": size,
		}
		if u, metaErr := client.FetchDocMetaURL(result.FileToken, "file", token); metaErr == nil && strings.TrimSpace(u) != "" {
			out["url"] = u
		} else if metaErr != nil {
			fmt.Fprintf(os.Stderr, "warning: 创建后查询 URL 失败: %v\n", metaErr)
		}

		// --as bot（或 auto 未登录）创建时，自动给当前 CLI 登录用户授予文件 full_access
		grant := autoGrantCurrentUser(token, result.FileToken, client.ResourceTypeFile)
		withPermissionGrant(out, grant)

		if output == "json" {
			return printJSON(out)
		}
		fmt.Printf("Markdown 文件创建成功!\n")
		fmt.Printf("  file_name:  %s\n", fileName)
		fmt.Printf("  file_token: %s\n", result.FileToken)
		fmt.Printf("  size:       %d bytes\n", size)
		if u, ok := out["url"].(string); ok && u != "" {
			fmt.Printf("  url:        %s\n", u)
		}
		printPermissionGrantText(os.Stdout, grant)
		return nil
	},
}

func markdownUploadDryRunSteps(spec client.MarkdownUploadSpec, size int64, multipart bool, filePath string) []dryRunStep {
	parentType, parentNode := "explorer", spec.FolderToken
	if spec.WikiToken != "" {
		parentType, parentNode = "wiki", spec.WikiToken
	}
	fileField := "<markdown content>"
	if filePath != "" {
		fileField = "@" + filePath
	}
	if !multipart {
		body := map[string]any{
			"file_name":   spec.FileName,
			"parent_type": parentType,
			"parent_node": parentNode,
			"size":        size,
			"file":        fileField,
		}
		if spec.FileToken != "" {
			body["file_token"] = spec.FileToken
		}
		return []dryRunStep{{
			Method: "POST",
			URL:    "/open-apis/drive/v1/files/upload_all",
			Body:   body,
		}}
	}
	prepare := map[string]any{
		"file_name":   spec.FileName,
		"parent_type": parentType,
		"parent_node": parentNode,
		"size":        size,
	}
	if spec.FileToken != "" {
		prepare["file_token"] = spec.FileToken
	}
	return []dryRunStep{
		{Method: "POST", URL: "/open-apis/drive/v1/files/upload_prepare", Desc: "Initialize multipart upload", Body: prepare},
		{Method: "POST", URL: "/open-apis/drive/v1/files/upload_part", Desc: "Upload file parts (repeated)", Body: map[string]any{
			"upload_id": "<upload_id>", "seq": "<chunk_index>", "size": "<chunk_size>", "file": "<chunk_binary>",
		}},
		{Method: "POST", URL: "/open-apis/drive/v1/files/upload_finish", Desc: "Finalize upload", Body: map[string]any{
			"upload_id": "<upload_id>", "block_num": "<block_num>",
		}},
	}
}

func init() {
	markdownCmd.AddCommand(markdownCreateCmd)
	markdownCreateCmd.Flags().String("name", "", "远端文件名（必须 .md 结尾；--content 搭配时必填）")
	markdownCreateCmd.Flags().String("content", "", "Markdown 字符串内容（与 --content-file 二选一）")
	markdownCreateCmd.Flags().String("content-file", "", "本地 .md 文件路径（与 --content 二选一）")
	markdownCreateCmd.Flags().String("file", "", "本地 .md 文件路径，兼容别名（等价于 --content-file）")
	markdownCreateCmd.Flags().String("folder-token", "", "目标文件夹 token（默认 Drive 根目录；与 --wiki-token 互斥）")
	markdownCreateCmd.Flags().String("wiki-token", "", "目标 wiki 节点 token（与 --folder-token 互斥）")
	markdownCreateCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求")
	markdownCreateCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	markdownCreateCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
}
