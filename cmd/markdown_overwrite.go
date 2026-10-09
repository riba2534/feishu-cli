package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/safefile"
	"github.com/spf13/cobra"
)

var markdownOverwriteCmd = &cobra.Command{
	Use:   "overwrite",
	Short: "覆盖 Drive 中已存在的 Markdown (.md) 文件",
	Long: `把新的 Markdown 内容写到一个已存在的 .md 文件，file_token 保持不变。

底层调 ` + "`POST /open-apis/drive/v1/files/upload_all`" + `，带 file_token。>20MB 走分片。
未传 --name 时通过 metas/batch_query 读取现有文件名。--as bot|user|auto（默认 auto）。

必填:
  --file-token     目标 .md 文件 token
  --content / --content-file / --file  二选一

可选:
  --name           覆盖后文件名（必须 .md 结尾；缺省读取远端现有名）
  --dry-run        只打印将要发出的请求
  --user-access-token  覆盖登录态

示例:
  feishu-cli markdown overwrite --file-token boxcnxxx --name existing.md --content "新内容"
  feishu-cli markdown overwrite --file-token boxcnxxx --file ./new.md`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken, _ := cmd.Flags().GetString("file-token")
		name, _ := cmd.Flags().GetString("name")
		content, _ := cmd.Flags().GetString("content")
		contentFile, _ := cmd.Flags().GetString("content-file")
		contentFileAlias, _ := cmd.Flags().GetString("file")
		output, _ := cmd.Flags().GetString("output")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		contentChanged := cmd.Flags().Changed("content")
		fileChanged := cmd.Flags().Changed("content-file") || cmd.Flags().Changed("file")

		var err error
		contentFile, err = resolveMarkdownFileFlag(contentFile, contentFileAlias)
		if err != nil {
			return err
		}
		fileToken = strings.TrimSpace(fileToken)
		if fileToken == "" {
			return clierr.Usagef("--file-token 必填")
		}
		if contentChanged && fileChanged {
			return clierr.Usagef("--content 与 --content-file/--file 不能同时使用")
		}
		if !contentChanged && !fileChanged {
			return clierr.Usagef("请提供 --content 或 --content-file")
		}

		// 不传 --name 时一律读远端现有名（含 --content-file 场景）。
		// 刻意不用 filepath.Base(contentFile)：帮助承诺「缺省读取远端现有名」，
		// 若拿本地文件名顶替，`overwrite --file ./new.md` 会把远端文件静默改名为 new.md。
		fileName := strings.TrimSpace(name)
		if fileName != "" {
			if err := validateMarkdownFileName(fileName, "--name"); err != nil {
				return err
			}
		}

		var size int64
		if contentChanged {
			size = int64(len(content))
		} else {
			// 敏感目录、不存在、是目录、无权限读取均为用法错误（先于任何网络请求）
			stat, err := safefile.StatInputFile(contentFile)
			if err != nil {
				return fmt.Errorf("--content-file 无效: %w", err)
			}
			size = stat.Size()
		}
		if size == 0 {
			return clierr.Usagef("Markdown 内容为空，不支持把 .md 覆盖为空文件")
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}

		uploadSpec := client.MarkdownUploadSpec{FileToken: fileToken, FileName: fileName}
		if dryRun {
			multipart := markdownNeedsMultipart(size)
			var steps []dryRunStep
			if fileName == "" {
				steps = append(steps, dryRunStep{
					Method: "POST",
					URL:    "/open-apis/drive/v1/metas/batch_query",
					Desc:   "Read current file metadata to preserve the existing file name",
					Body: map[string]any{
						"request_docs": []map[string]any{{
							"doc_token": fileToken,
							"doc_type":  "file",
						}},
					},
				})
				uploadSpec.FileName = "<existing_remote_name>"
			}
			steps = append(steps, markdownUploadDryRunSteps(uploadSpec, size, multipart, contentFile)...)
			return printDryRunPlan(cmd, "overwrite markdown file", map[string]any{
				"file_token": fileToken,
				"size":       size,
			}, steps)
		}

		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		if fileName == "" {
			remoteName, err := client.FetchMarkdownFileName(fileToken, token)
			if err != nil {
				return err
			}
			fileName = strings.TrimSpace(remoteName)
			if fileName == "" {
				// 不能退化成 fileToken+".md"：那会把远端文件静默重命名，
				// 与「覆盖内容、保留原名」的命令语义相悖。取不到原名就 fail-closed。
				return fmt.Errorf("无法读取 file_token=%s 的现有文件名，拒绝以 %s.md 静默重命名远端文件；请显式指定 --name <原文件名.md>",
					fileToken, fileToken)
			}
			uploadSpec.FileName = fileName
		}

		var result client.MarkdownUploadResult
		if fileChanged {
			result, err = client.UploadMarkdownFile(uploadSpec, contentFile, token)
		} else {
			result, err = client.UploadMarkdownContent(uploadSpec, []byte(content), token)
		}
		if err != nil {
			return err
		}

		out := map[string]any{
			"file_token": result.FileToken,
			"file_name":  fileName,
			"version":    result.Version,
			"size_bytes": size,
		}
		if output == "json" {
			return printJSON(out)
		}
		fmt.Printf("Markdown 文件覆盖成功!\n")
		fmt.Printf("  file_name:  %s\n", fileName)
		fmt.Printf("  file_token: %s\n", result.FileToken)
		if result.Version != "" {
			fmt.Printf("  version:    %s\n", result.Version)
		}
		fmt.Printf("  size:       %d bytes\n", size)
		return nil
	},
}

func init() {
	markdownCmd.AddCommand(markdownOverwriteCmd)
	markdownOverwriteCmd.Flags().String("file-token", "", "目标 .md 文件 token（必填）")
	markdownOverwriteCmd.Flags().String("name", "", "覆盖后文件名（必须 .md 结尾；缺省读取远端现有名）")
	markdownOverwriteCmd.Flags().String("content", "", "新 Markdown 内容（与 --content-file 二选一）")
	markdownOverwriteCmd.Flags().String("content-file", "", "本地 .md 文件路径（与 --content 二选一）")
	markdownOverwriteCmd.Flags().String("file", "", "本地 .md 文件路径，兼容别名（等价于 --content-file）")
	markdownOverwriteCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求")
	markdownOverwriteCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	markdownOverwriteCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(markdownOverwriteCmd, "file-token")
}
