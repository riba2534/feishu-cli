package cmd

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var markdownPatchCmd = &cobra.Command{
	Use:   "patch",
	Short: "对 Drive 原生 Markdown 做查找替换后覆盖",
	Long: `下载当前 Markdown，在本地做 literal 或 RE2 替换，命中后再 overwrite。

流程:
  1. GET /open-apis/drive/v1/medias/{token}/preview_download?preview_type=16
  2. 本地替换；match_count=0 时不写回
  3. POST files/upload_all 或分片覆盖，保留 file_token

必填:
  --file-token  目标文件 token
  --pattern     查找文本或正则
  --content     替换内容（可为 empty string）

可选:
  --regex       将 --pattern 解释为 RE2
  --name        覆盖时使用的文件名（必须 .md 结尾；缺省读取远端现有名，读不到时拒绝写回，与 overwrite 一致）
  --as          bot|user|auto（默认 auto）
  --dry-run     只打印将要发出的请求

示例:
  feishu-cli markdown patch --file-token boxcnxxx --pattern "TODO" --content "DONE"
  feishu-cli markdown patch --file-token boxcnxxx --regex --pattern "v[0-9]+" --content "v2"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken, _ := cmd.Flags().GetString("file-token")
		pattern, _ := cmd.Flags().GetString("pattern")
		useRegex, _ := cmd.Flags().GetBool("regex")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		output, _ := cmd.Flags().GetString("output")
		contentSet := cmd.Flags().Changed("content")
		content, _ := cmd.Flags().GetString("content")
		name, _ := cmd.Flags().GetString("name")

		fileToken = strings.TrimSpace(fileToken)
		if fileToken == "" {
			return clierr.Usagef("--file-token 必填")
		}
		if !cmd.Flags().Changed("pattern") || pattern == "" {
			return clierr.Usagef("--pattern 必填且不能为空")
		}
		if !contentSet {
			return clierr.Usagef("--content 必填")
		}
		if useRegex {
			if _, err := regexp.Compile(pattern); err != nil {
				return clierr.Usagef("无效的 --pattern 正则: %w", err)
			}
		}
		fileName := strings.TrimSpace(name)
		if cmd.Flags().Changed("name") {
			if err := validateMarkdownFileName(fileName, "--name"); err != nil {
				return err
			}
		}

		mode := "literal"
		if useRegex {
			mode = "regex"
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}

		if dryRun {
			sizeThreshold := formatByteSize(20 * 1024 * 1024)
			planName := fileName
			steps := []dryRunStep{{
				Method: "GET",
				URL:    markdownPreviewDownloadPath(fileToken),
				Desc:   "Download the current Markdown source file preview artifact",
				Params: markdownPreviewParams(""),
			}}
			if planName == "" {
				planName = "<existing_remote_name>"
				steps = append(steps, dryRunStep{
					Method: "POST",
					URL:    "/open-apis/drive/v1/metas/batch_query",
					Desc:   "Read current file metadata to preserve the existing file name before overwrite (fail-closed when unavailable; pass --name)",
					Body: map[string]any{
						"request_docs": []map[string]any{{
							"doc_token": fileToken,
							"doc_type":  "file",
						}},
					},
				})
			}
			steps = append(steps,
				dryRunStep{
					Method: "POST",
					URL:    "/open-apis/drive/v1/files/upload_all",
					Desc:   "If patched Markdown is at most " + sizeThreshold + ", overwrite with upload_all",
					Body: map[string]any{
						"file_name":   planName,
						"parent_type": "explorer",
						"parent_node": "",
						"size":        "<updated_size_bytes>",
						"file":        "<patched_markdown_content>",
						"file_token":  fileToken,
					},
				},
				dryRunStep{
					Method: "POST",
					URL:    "/open-apis/drive/v1/files/upload_prepare",
					Desc:   "If patched Markdown exceeds " + sizeThreshold + ", initialize multipart overwrite",
					Body: map[string]any{
						"file_name":   planName,
						"parent_type": "explorer",
						"parent_node": "",
						"size":        "<updated_size_bytes>",
						"file_token":  fileToken,
					},
				},
			)
			return printDryRunPlan(cmd, "Download the current Markdown file, apply the replacement locally, and overwrite only when matches are found", map[string]any{
				"mode": mode,
			}, steps)
		}

		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		payload, _, err := client.FetchMarkdownSource(fileToken, "", token)
		if err != nil {
			return err
		}
		original := string(payload)
		patched, matchCount, err := applyMarkdownPatch(original, pattern, content, useRegex)
		if err != nil {
			return err
		}

		out := map[string]any{
			"updated":           false,
			"mode":              mode,
			"match_count":       matchCount,
			"version":           "",
			"size_bytes_before": len(payload),
			"size_bytes_after":  len(payload),
		}
		if matchCount == 0 {
			if output == "json" {
				return printJSON(out)
			}
			fmt.Println("updated: false")
			fmt.Printf("mode: %s\n", mode)
			fmt.Printf("match_count: %d\n", matchCount)
			return nil
		}

		patchedPayload := []byte(patched)
		if len(patchedPayload) == 0 {
			return clierr.Usagef("Markdown 内容为空，不支持把 .md 覆盖为空文件")
		}

		if fileName == "" {
			remoteName, err := client.FetchMarkdownFileName(fileToken, token)
			if err != nil {
				return err
			}
			fileName = strings.TrimSpace(remoteName)
			if fileName == "" {
				// 与 overwrite 一致：不能退化成 fileToken+".md" 静默重命名远端文件，取不到原名就 fail-closed
				return fmt.Errorf("无法读取 file_token=%s 的现有文件名，拒绝以 %s.md 静默重命名远端文件；请显式指定 --name <原文件名.md>",
					fileToken, fileToken)
			}
		}

		result, err := client.UploadMarkdownContent(client.MarkdownUploadSpec{
			FileToken: fileToken,
			FileName:  fileName,
		}, patchedPayload, token)
		if err != nil {
			return err
		}

		out["updated"] = true
		out["version"] = result.Version
		out["size_bytes_after"] = len(patchedPayload)
		if output == "json" {
			return printJSON(out)
		}
		fmt.Println("updated: true")
		fmt.Printf("mode: %s\n", mode)
		fmt.Printf("match_count: %d\n", matchCount)
		if result.Version != "" {
			fmt.Printf("version: %s\n", result.Version)
		}
		fmt.Printf("size_bytes_before: %d\n", len(payload))
		fmt.Printf("size_bytes_after: %d\n", len(patchedPayload))
		return nil
	},
}

func applyMarkdownPatch(original, pattern, replacement string, useRegex bool) (string, int, error) {
	if !useRegex {
		return strings.ReplaceAll(original, pattern, replacement), strings.Count(original, pattern), nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", 0, clierr.Usagef("无效的 --pattern 正则: %w", err)
	}
	return re.ReplaceAllString(original, replacement), len(re.FindAllStringIndex(original, -1)), nil
}

func init() {
	markdownCmd.AddCommand(markdownPatchCmd)
	markdownPatchCmd.Flags().String("file-token", "", "目标 Markdown 文件 token（必填）")
	markdownPatchCmd.Flags().String("pattern", "", "查找文本或 RE2 正则（必填）")
	markdownPatchCmd.Flags().String("content", "", "替换内容（必填，允许空字符串）")
	markdownPatchCmd.Flags().Bool("regex", false, "将 --pattern 解释为 RE2 正则")
	markdownPatchCmd.Flags().String("name", "", "覆盖时使用的文件名（必须 .md 结尾；缺省读取远端现有名）")
	markdownPatchCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求")
	markdownPatchCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	markdownPatchCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(markdownPatchCmd, "file-token")
}
