package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/riba2534/feishu-cli/v2/internal/safefile"
	"github.com/spf13/cobra"
)

var driveExportCmd = &cobra.Command{
	Use:   "export",
	Short: "导出云文档为本地文件（有界轮询 + docs_ai markdown + resume）",
	Long: `将 doc/docx/sheet/bitable/slides 或 wiki 文档导出为本地文件。

- markdown 导出走 POST /open-apis/docs_ai/v1/documents/{token}/fetch（format=markdown）
- 其他格式走 export_tasks 异步任务：创建 → 有界轮询（最多 10 次，每次 5s） → 下载
- wiki URL/token 先 node_by_token 再导出底层文档
- 超时未完成时返回 next_command

类型/格式矩阵:
  doc     → docx, pdf
  docx    → docx, pdf, markdown
  sheet   → xlsx, csv
  bitable → xlsx, csv, base
  slides  → pptx, pdf

必填:
  --file-extension  导出格式
  --token 或 --url  源文档

可选:
  --doc-type     裸 token 时必填；wiki 会先解析
  --sub-id       sheet/bitable → csv 时必填
  --only-schema  bitable → base 时只导出 schema
  --file-name    本地文件名
  --output-dir   输出目录（默认当前目录）
  --overwrite    已存在时覆盖
  --as           bot|user|auto（默认 auto：User 优先；未配置回退 Bot；已配置但解析/刷新失败 fail-closed）
  --dry-run      只打印将要发出的请求（不解析/刷新 token）

示例:
  feishu-cli drive export --token docxxx --doc-type docx --file-extension markdown
  feishu-cli drive export --url https://xxx.feishu.cn/wiki/wikcnxxx --file-extension pdf
  feishu-cli drive export --token sldxxx --doc-type slides --file-extension pptx`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		docToken, _ := cmd.Flags().GetString("token")
		docURL, _ := cmd.Flags().GetString("url")
		docType, _ := cmd.Flags().GetString("doc-type")
		fileExtension, _ := cmd.Flags().GetString("file-extension")
		subID, _ := cmd.Flags().GetString("sub-id")
		onlySchema, _ := cmd.Flags().GetBool("only-schema")
		preferredName, _ := cmd.Flags().GetString("file-name")
		outputDir, _ := cmd.Flags().GetString("output-dir")
		overwrite, _ := cmd.Flags().GetBool("overwrite")
		output, _ := cmd.Flags().GetString("output")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		docToken = strings.TrimSpace(docToken)
		docURL = strings.TrimSpace(docURL)
		docType = strings.ToLower(strings.TrimSpace(docType))
		fileExtension = strings.ToLower(strings.TrimSpace(fileExtension))
		preferredName = strings.TrimSpace(preferredName)

		sourceType, sourceToken, resolvedType, err := normalizeDriveExportInput(docURL, docToken, docType)
		if err != nil {
			return err
		}
		if fileExtension == "" {
			return fmt.Errorf("--file-extension 必填，允许: %s", driveExportFileExtensionValues)
		}

		if sourceType == "wiki" && resolvedType == "" {
			switch fileExtension {
			case "docx", "pdf", "xlsx", "csv", "markdown", "base", "pptx":
			default:
				return fmt.Errorf("不支持的 --file-extension %q，允许: %s", fileExtension, driveExportFileExtensionValues)
			}
			if onlySchema && fileExtension != "base" {
				return fmt.Errorf("--only-schema 仅在导出 bitable 为 base 时使用")
			}
			if strings.TrimSpace(subID) != "" && fileExtension != "csv" {
				return fmt.Errorf("--sub-id 仅在 sheet/bitable 导出 csv 时使用")
			}
		} else {
			if err := validateDriveExportFormat(resolvedType, fileExtension, subID, onlySchema); err != nil {
				return err
			}
		}

		if outputDir == "" {
			outputDir = "."
		}
		// 输出目录在任何网络请求（含 token 刷新、wiki 解析、创建导出任务）之前校验，敏感目录直接拒绝
		if err := validateOutputPath(outputDir, ""); err != nil {
			return fmt.Errorf("--output-dir 无效: %w", err)
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}

		if dryRun {
			var steps []dryRunStep
			exportToken := sourceToken
			exportType := resolvedType
			if sourceType == "wiki" {
				steps = append(steps, dryRunStep{
					Method: "GET",
					URL:    client.WikiNodeByTokenPath,
					Desc:   "Resolve wiki node to underlying document token",
					Params: map[string]any{"token": sourceToken},
				})
				exportToken = "obj_token_from_step_0"
				if exportType == "" {
					exportType = "obj_type_from_step_0"
				}
			}
			if fileExtension == "markdown" {
				steps = append(steps, dryRunStep{
					Method: "POST",
					URL:    fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s/fetch", exportToken),
					Desc:   "fetch docx markdown",
					Body:   map[string]any{"format": "markdown"},
				})
			} else {
				body := map[string]any{
					"token":          exportToken,
					"type":           exportType,
					"file_extension": fileExtension,
				}
				if strings.TrimSpace(subID) != "" {
					body["sub_id"] = subID
				}
				if onlySchema {
					body["only_schema"] = true
				}
				steps = append(steps, dryRunStep{
					Method: "POST",
					URL:    "/open-apis/drive/v1/export_tasks",
					Body:   body,
				})
			}
			extra := map[string]any{"output_dir": outputDir}
			if sourceType == "wiki" {
				extra["wiki_token"] = sourceToken
			}
			if preferredName != "" {
				extra["file_name"] = ensureExportFileExtension(sanitizeExportName(preferredName, sourceToken), fileExtension)
			}
			return printDryRunPlan(cmd, "export cloud document", extra, steps)
		}

		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}

		if err := safefile.MkdirAll(outputDir, 0o755); err != nil {
			return fmt.Errorf("创建 --output-dir 失败: %w", err)
		}

		var wikiToken, wikiObjToken, wikiObjType string
		if sourceType == "wiki" {
			fmt.Fprintf(os.Stderr, "解析 wiki 节点: %s\n", sourceToken)
			node, err := client.ResolveWikiNode(sourceToken, token)
			if err != nil {
				return err
			}
			objType := normalizeDriveExportDocType(node.ObjType)
			if !isDriveExportDocType(objType) {
				return fmt.Errorf("wiki 解析为 %q，drive export 仅支持 doc/docx/sheet/bitable/slides", objType)
			}
			if resolvedType != "" && resolvedType != objType {
				return fmt.Errorf("wiki 解析为 %q，但 --doc-type 是 %q", objType, resolvedType)
			}
			resolvedType = objType
			sourceToken = node.ObjToken
			wikiToken = node.NodeToken
			wikiObjToken = node.ObjToken
			wikiObjType = objType
			if err := validateDriveExportFormat(resolvedType, fileExtension, subID, onlySchema); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "已解析为 %s: %s\n", objType, node.ObjToken)
		}

		annotateWiki := func(result map[string]any) map[string]any {
			if wikiToken == "" {
				return result
			}
			result["wiki_token"] = wikiToken
			result["wiki_node"] = map[string]any{"obj_token": wikiObjToken, "obj_type": wikiObjType}
			return result
		}

		if fileExtension == "markdown" {
			fmt.Fprintf(os.Stderr, "Markdown 快捷导出 (docs_ai): %s\n", sourceToken)
			content, err := client.FetchDocxMarkdownContent(sourceToken, token)
			if err != nil {
				return err
			}
			fileName := preferredName
			if fileName == "" {
				title, titleErr := client.FetchDocMetaTitle(sourceToken, resolvedType, token)
				if titleErr != nil || strings.TrimSpace(title) == "" {
					title = sourceToken
				}
				fileName = title
			}
			fileName = ensureExportFileExtension(sanitizeExportName(fileName, sourceToken), fileExtension)
			savedPath, err := writeMarkdownExportFile(outputDir, fileName, []byte(content), overwrite)
			if err != nil {
				return err
			}
			result := annotateWiki(map[string]any{
				"token":          sourceToken,
				"doc_type":       resolvedType,
				"file_extension": fileExtension,
				"file_name":      fileName,
				"saved_path":     savedPath,
				"size_bytes":     len(content),
			})
			if output == "json" {
				return printJSON(result)
			}
			fmt.Printf("导出成功: %s (%d bytes)\n", savedPath, len(content))
			return nil
		}

		ticket, err := client.CreateExportTaskEx(sourceToken, resolvedType, fileExtension, subID, onlySchema, token)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "创建导出任务: %s\n", ticket)

		nextCmd := fmt.Sprintf("feishu-cli drive task-result --scenario export --ticket %s --file-token %s --as %s", quotePOSIXShell(ticket), quotePOSIXShell(sourceToken), resumeIdentity(cmd))
		status, timedOut, err := client.WaitDriveExportWithBound(ticket, sourceToken, token)
		if err != nil {
			return withDrivePollResume(err, nextCmd)
		}

		if timedOut {
			result := annotateWiki(map[string]any{
				"ticket":         ticket,
				"token":          sourceToken,
				"doc_type":       resolvedType,
				"file_extension": fileExtension,
				"ready":          false,
				"timed_out":      true,
				"next_command":   nextCmd,
			})
			if status != nil {
				result["job_status"] = status.JobStatus
				result["job_status_label"] = status.StatusLabel()
			}
			if preferredName != "" {
				result["file_name"] = ensureExportFileExtension(sanitizeExportName(preferredName, sourceToken), fileExtension)
			}
			if output == "json" {
				_ = printJSON(result)
			} else {
				fmt.Fprintf(os.Stderr, "导出任务仍在进行中，继续: %s\n", nextCmd)
			}
			return nil
		}

		fileName := preferredName
		if fileName == "" {
			fileName = status.FileName
		}
		fileName = ensureExportFileExtension(sanitizeExportName(fileName, sourceToken), fileExtension)
		savedPath := filepath.Join(outputDir, fileName)
		if _, err := os.Stat(savedPath); err == nil && !overwrite {
			return clierr.Usagef("文件已存在: %s（使用 --overwrite 覆盖）", savedPath)
		}
		if err := client.DownloadExportFile(status.FileToken, savedPath, token); err != nil {
			nextCmd := driveExportDownloadRetryCommand(status.FileToken, outputDir, fileName, token)
			return fmt.Errorf("下载导出文件失败: %w\n可重试: %s", err, nextCmd)
		}

		stat, _ := os.Stat(savedPath)
		size := int64(0)
		if stat != nil {
			size = stat.Size()
		}
		result := annotateWiki(map[string]any{
			"ticket":         ticket,
			"token":          sourceToken,
			"doc_type":       resolvedType,
			"file_extension": fileExtension,
			"file_name":      fileName,
			"saved_path":     savedPath,
			"size_bytes":     size,
		})
		if output == "json" {
			return printJSON(result)
		}
		fmt.Printf("导出成功: %s (%d bytes)\n", savedPath, size)
		return nil
	},
}

func normalizeDriveExportInput(rawURL, token, docType string) (sourceType, sourceToken, resolvedType string, err error) {
	if token == "" && rawURL == "" {
		return "", "", "", fmt.Errorf("必须提供 --url 或 --token")
	}
	if token != "" && rawURL != "" {
		return "", "", "", fmt.Errorf("--url 与 --token 互斥")
	}
	raw := token
	if rawURL != "" {
		raw = rawURL
	}
	if strings.Contains(raw, "://") {
		// 只按 URL 路径前缀推断类型（query 中的 /wiki/ 等字样不会劫持解析）
		ref, parseErr := client.ParseResourceURL(raw)
		if parseErr != nil {
			return "", "", "", fmt.Errorf("--url 解析失败: %w", parseErr)
		}
		parsedType := ref.Type
		sourceType = normalizeDriveExportDocType(parsedType)
		sourceToken = ref.Token
		if sourceType == "wiki" {
			if docType == "wiki" {
				return "wiki", sourceToken, "", nil
			}
			if docType != "" && !isDriveExportDocType(docType) && docType != "wiki" {
				return "", "", "", fmt.Errorf("--doc-type %q 与 URL 类型 wiki 冲突", docType)
			}
			if docType != "" && docType != "wiki" {
				return "wiki", sourceToken, normalizeDriveExportDocType(docType), nil
			}
			return "wiki", sourceToken, "", nil
		}
		if !isDriveExportDocType(sourceType) {
			return "", "", "", fmt.Errorf("URL 类型 %q 不受 drive export 支持", parsedType)
		}
		if docType == "wiki" {
			return "", "", "", fmt.Errorf("--doc-type wiki 与 URL 类型 %q 冲突", sourceType)
		}
		if docType != "" && normalizeDriveExportDocType(docType) != sourceType {
			return "", "", "", fmt.Errorf("--doc-type %q 与 URL 类型 %q 冲突", docType, sourceType)
		}
		return sourceType, sourceToken, sourceType, nil
	}
	if rawURL != "" {
		return "", "", "", fmt.Errorf("不支持的 --url %q，请使用飞书文档 URL", rawURL)
	}
	if !client.IsSafeResourceToken(strings.TrimSpace(token)) {
		return "", "", "", fmt.Errorf("--token 不是有效的 token（只允许字母、数字、_ 和 -）: %q", token)
	}
	token = strings.TrimSpace(token)
	if docType == "" {
		return "", "", "", fmt.Errorf("裸 token 必须提供 --doc-type（允许: %s）", driveExportInputDocTypeValues)
	}
	if docType == "wiki" {
		return "wiki", token, "", nil
	}
	if !isDriveExportDocType(docType) {
		return "", "", "", fmt.Errorf("不支持的 --doc-type %q，允许: %s", docType, driveExportInputDocTypeValues)
	}
	return normalizeDriveExportDocType(docType), token, normalizeDriveExportDocType(docType), nil
}

func sanitizeExportName(title, fallback string) string {
	name := strings.TrimSpace(title)
	if name == "" {
		name = fallback
	}
	return safeOutputPath(name, "")
}

// driveExportDownloadRetryCommand 构造导出文件下载失败后的重试命令。
// 身份取本次实际使用的身份（token 非空为 user，否则 bot），不原样回显 auto：导出产物归创建任务的
// 身份所有，auto 在重试时可能因登录态变化解析成另一身份而无权下载（与 file delete / wiki 删除的续查命令一致）。
// 同时带上本次的文件名（export-download 默认以 file_token 命名、不带扩展名），参数按 POSIX shell 转义。
func driveExportDownloadRetryCommand(fileToken, outputDir, fileName, token string) string {
	identity := "bot"
	if token != "" {
		identity = "user"
	}
	return fmt.Sprintf("feishu-cli drive export-download --file-token %s --output-dir %s --file-name %s --as %s",
		quotePOSIXShell(fileToken), quotePOSIXShell(outputDir), quotePOSIXShell(fileName), identity)
}

var driveExportDownloadCmd = &cobra.Command{
	Use:   "export-download",
	Short: "下载已完成的导出任务文件（配合 drive export 超时后 resume）",
	Long: `通过 file_token 下载已经完成的导出任务产物。

必填:
  --file-token  导出任务生成的 file_token（由 drive export 或 drive task-result 返回）

可选:
  --file-name   保存文件名（默认由 file_token 自动构造）
  --output-dir  输出目录（默认当前目录）
  --overwrite   已存在时覆盖
  --as          bot|user|auto（默认 auto）

示例:
  feishu-cli drive export-download --file-token boxxxx --output-dir ./exports`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken, _ := cmd.Flags().GetString("file-token")
		fileName, _ := cmd.Flags().GetString("file-name")
		outputDir, _ := cmd.Flags().GetString("output-dir")
		overwrite, _ := cmd.Flags().GetBool("overwrite")
		output, _ := cmd.Flags().GetString("output")

		if fileToken == "" {
			return fmt.Errorf("--file-token 必填")
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}
		if outputDir == "" {
			outputDir = "."
		}
		if fileName == "" {
			fileName = safeOutputPath(fileToken, "")
		}
		savedPath := filepath.Join(outputDir, fileName)
		// 输出目录与最终文件在任何网络请求（含 token 刷新）之前校验，敏感目录直接拒绝
		if err := validateOutputPath(outputDir, ""); err != nil {
			return fmt.Errorf("--output-dir 无效: %w", err)
		}
		if err := validateOutputPath(savedPath, ""); err != nil {
			return err
		}
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		if err := safefile.MkdirAll(outputDir, 0o755); err != nil {
			return fmt.Errorf("创建 --output-dir 失败: %w", err)
		}
		if _, err := os.Stat(savedPath); err == nil && !overwrite {
			return clierr.Usagef("文件已存在: %s（使用 --overwrite 覆盖）", savedPath)
		}

		if err := client.DownloadExportFile(fileToken, savedPath, token); err != nil {
			return err
		}

		stat, _ := os.Stat(savedPath)
		size := int64(0)
		if stat != nil {
			size = stat.Size()
		}
		result := map[string]any{
			"file_token": fileToken,
			"saved_path": savedPath,
			"size_bytes": size,
		}
		if output == "json" {
			return printJSON(result)
		}
		fmt.Printf("下载成功: %s (%d bytes)\n", savedPath, size)
		return nil
	},
}

func init() {
	driveCmd.AddCommand(driveExportCmd)
	driveExportCmd.Flags().String("token", "", "源文档 token（与 --url 二选一）")
	driveExportCmd.Flags().String("url", "", "源文档 URL（与 --token 二选一；wiki URL 会先解析）")
	driveExportCmd.Flags().String("doc-type", "", "源文档类型: doc/docx/sheet/bitable/slides/wiki")
	driveExportCmd.Flags().String("file-extension", "", "导出格式: docx/pdf/xlsx/csv/markdown/base/pptx（必填）")
	driveExportCmd.Flags().String("sub-id", "", "子表/工作表 ID（sheet/bitable 导出 csv 时必填）")
	driveExportCmd.Flags().Bool("only-schema", false, "bitable 导出 base 时只导出 schema")
	driveExportCmd.Flags().String("file-name", "", "本地文件名")
	driveExportCmd.Flags().String("output-dir", ".", "输出目录")
	driveExportCmd.Flags().Bool("overwrite", false, "已存在时覆盖")
	driveExportCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求")
	addAsFlag(driveExportCmd)
	driveExportCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	driveExportCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(driveExportCmd, "file-extension")

	driveCmd.AddCommand(driveExportDownloadCmd)
	driveExportDownloadCmd.Flags().String("file-token", "", "导出文件 token（必填）")
	driveExportDownloadCmd.Flags().String("file-name", "", "保存文件名")
	driveExportDownloadCmd.Flags().String("output-dir", ".", "输出目录")
	driveExportDownloadCmd.Flags().Bool("overwrite", false, "已存在时覆盖")
	addAsFlag(driveExportDownloadCmd)
	driveExportDownloadCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	driveExportDownloadCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(driveExportDownloadCmd, "file-token")
}
