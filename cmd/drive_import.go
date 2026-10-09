package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/safefile"
	"github.com/spf13/cobra"
)

var driveImportCmd = &cobra.Command{
	Use:   "import",
	Short: "导入本地文件为云文档（分块上传 + 有界轮询 + resume）",
	Long: `导入本地文件为云文档（docx/sheet/bitable/slides）。

流程:
  1. 上传本地文件到临时媒体（>20MB 自动走 medias/upload_prepare/part/finish）
  2. 创建 import_tasks 任务（始终携带 point.mount_type=1）
  3. 有界轮询（最多 30 次，每次 2s）
  4. 超时时返回 next_command 可用 drive task-result 继续

官方格式/大小矩阵:
  - .docx/.doc → docx，上限 600MB
  - .pptx → slides，上限 500MB
  - .xlsx → sheet/bitable，上限 800MB
  - .csv → sheet 20MB / bitable 100MB
  - .txt/.md/.mark/.markdown/.html/.xls/.base → 20MB
  - .base 仅 bitable；.pptx 仅 slides

upload_all 省略 parent_node；upload_prepare 显式 parent_node=""。
--folder-token 若解析为 wiki 节点会被拒绝。省略时 point.mount_key 为空（根目录）。

必填:
  --file        本地文件路径
  --type        目标文档类型: docx / sheet / bitable / slides

可选:
  --folder-token   目标 Drive 文件夹 token 或文件夹 URL
  --name           导入后的文件名（默认本地文件名去扩展名）
  --target-token   已有 bitable token（仅 --type bitable）
  --as             bot|user|auto（默认 auto：User 优先；未配置回退 Bot；已配置但解析/刷新失败 fail-closed）
  --dry-run        只打印将要发出的请求（不解析/刷新 token）

以 Bot 身份导入成功后，自动给当前 CLI 登录用户授予新文档 full_access（JSON 输出 permission_grant）。

示例:
  feishu-cli drive import --file report.docx --type docx
  feishu-cli drive import --file data.xlsx --type sheet --folder-token fldxxx
  feishu-cli drive import --file deck.pptx --type slides --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		filePath, _ := cmd.Flags().GetString("file")
		targetType, _ := cmd.Flags().GetString("type")
		folderToken, _ := cmd.Flags().GetString("folder-token")
		name, _ := cmd.Flags().GetString("name")
		targetToken, _ := cmd.Flags().GetString("target-token")
		output, _ := cmd.Flags().GetString("output")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		filePath = strings.TrimSpace(filePath)
		targetType = strings.ToLower(strings.TrimSpace(targetType))
		folderToken = strings.TrimSpace(folderToken)
		targetToken = strings.TrimSpace(targetToken)

		if filePath == "" {
			return fmt.Errorf("--file 必填")
		}
		// --folder-token 也接受文件夹 URL（/drive/folder/、/drive/shr/ 等），只按路径前缀解析
		if folderToken != "" {
			res, err := parseResourceArg(folderToken, resourceArgOptions{
				ArgName:     "--folder-token",
				DefaultType: client.ResourceTypeFolder,
				Allowed:     []string{client.ResourceTypeFolder},
			})
			if err != nil {
				return err
			}
			folderToken = res.Token
		}
		if err := validateDriveImportSpec(filePath, targetType, folderToken, targetToken, ""); err != nil {
			return err
		}

		// 敏感目录、不存在、是目录、无权限读取均为用法错误（先于任何网络请求）
		stat, err := safefile.StatInputFile(filePath)
		if err != nil {
			return fmt.Errorf("--file 无效: %w", err)
		}
		if !stat.Mode().IsRegular() {
			return clierr.Usagef("--file 必须指向普通文件")
		}

		ext := driveImportFileExtension(filePath, "")
		if err := validateDriveImportFileSize(ext, targetType, stat.Size()); err != nil {
			return err
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}

		fileName := driveImportDefaultFileName(filePath, name)
		uploadedName := driveImportSourceFileName(filePath, "")
		extra := fmt.Sprintf(`{"obj_type":"%s","file_extension":"%s"}`, targetType, ext)

		if dryRun {
			var steps []dryRunStep
			if folderToken != "" {
				steps = append(steps, dryRunStep{
					Method: "GET",
					URL:    client.WikiNodeByTokenPath,
					Desc:   "Validate whether --folder-token is a wiki node",
					Params: map[string]any{"token": folderToken},
				})
			}
			if client.DriveNeedsMultipart(stat.Size()) {
				steps = append(steps,
					dryRunStep{Method: "POST", URL: "/open-apis/drive/v1/medias/upload_prepare", Desc: "Initialize multipart upload", Body: map[string]any{
						"file_name": uploadedName, "parent_type": "ccm_import_open", "parent_node": "", "size": stat.Size(), "extra": extra,
					}},
					dryRunStep{Method: "POST", URL: "/open-apis/drive/v1/medias/upload_part", Desc: "Upload file parts (repeated)", Body: map[string]any{
						"upload_id": "<upload_id>", "seq": "<chunk_index>", "size": "<chunk_size>", "file": "<chunk_binary>",
					}},
					dryRunStep{Method: "POST", URL: "/open-apis/drive/v1/medias/upload_finish", Desc: "Finalize multipart upload", Body: map[string]any{
						"upload_id": "<upload_id>", "block_num": "<block_num>",
					}},
				)
			} else {
				steps = append(steps, dryRunStep{
					Method: "POST",
					URL:    "/open-apis/drive/v1/medias/upload_all",
					Desc:   "Upload file to get file_token (parent_node omitted)",
					Body: map[string]any{
						"file_name": uploadedName, "parent_type": "ccm_import_open", "size": stat.Size(), "extra": extra, "file": "@" + filePath,
					},
				})
			}
			taskBody := map[string]any{
				"file_extension": ext,
				"file_token":     "<file_token>",
				"type":           targetType,
				"file_name":      fileName,
				"point": map[string]any{
					"mount_type": 1,
					"mount_key":  folderToken,
				},
			}
			if targetType == "bitable" && targetToken != "" {
				taskBody["token"] = targetToken
			}
			steps = append(steps,
				dryRunStep{Method: "POST", URL: "/open-apis/drive/v1/import_tasks", Desc: "Create import task", Body: taskBody},
				dryRunStep{Method: "GET", URL: "/open-apis/drive/v1/import_tasks/:ticket", Desc: "Poll import task result", Params: map[string]any{"ticket": "<ticket>"}},
			)
			return printDryRunPlan(cmd, "Upload file -> create import task -> poll status", map[string]any{
				"type": targetType,
				"size": stat.Size(),
			}, steps)
		}

		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}

		if err := rejectDriveImportWikiFolderToken(folderToken, token); err != nil {
			return err
		}

		fmt.Fprintf(os.Stderr, "上传临时媒体: %s (%s)\n", filepath.Base(filePath), formatByteSize(stat.Size()))
		fileToken, err := client.UploadMediaForImport(filePath, uploadedName, targetType, ext, token)
		if err != nil {
			return fmt.Errorf("上传源文件失败: %w", err)
		}
		fmt.Fprintf(os.Stderr, "上传成功，file_token: %s\n", fileToken)

		ticket, err := client.CreateImportTaskEx(fileToken, ext, fileName, targetType, folderToken, targetToken, token)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "创建导入任务: %s\n", ticket)

		nextCmd := fmt.Sprintf("feishu-cli drive task-result --scenario import --ticket %s --as %s", quotePOSIXShell(ticket), resumeIdentity(cmd))
		status, timedOut, err := client.WaitDriveImportWithBound(ticket, token)
		if err != nil {
			return withDrivePollResume(err, nextCmd)
		}

		if timedOut {
			result := map[string]any{
				"ticket":       ticket,
				"file_token":   fileToken,
				"type":         targetType,
				"ready":        false,
				"timed_out":    true,
				"next_command": nextCmd,
			}
			if status != nil {
				result["job_status"] = status.JobStatus
			}
			if output == "json" {
				_ = printJSON(result)
			} else {
				fmt.Fprintf(os.Stderr, "导入任务仍在进行中，继续: %s\n", nextCmd)
			}
			return nil
		}

		resultType := targetType
		if status != nil && status.Type != "" {
			resultType = status.Type
		}
		result := map[string]any{
			"ticket":     ticket,
			"file_token": fileToken,
			"type":       resultType,
			"doc_token":  status.DocToken,
			"token":      status.DocToken,
			"doc_url":    status.DocURL,
		}
		if status.DocURL != "" {
			result["url"] = status.DocURL
		}
		// --as bot（或 auto 未登录）导入时，自动给当前 CLI 登录用户授予新文档 full_access
		grant := autoGrantCurrentUser(token, status.DocToken, resultType)
		withPermissionGrant(result, grant)

		if output == "json" {
			return printJSON(result)
		}

		fmt.Printf("导入成功!\n")
		fmt.Printf("  类型:      %s\n", resultType)
		fmt.Printf("  doc_token: %s\n", status.DocToken)
		if status.DocURL != "" {
			fmt.Printf("  URL:       %s\n", status.DocURL)
		}
		printPermissionGrantText(os.Stdout, grant)
		return nil
	},
}

// rejectDriveImportWikiFolderToken 拒绝把 wiki 节点当作导入挂载目录。
// node_by_token 对 Drive 文件夹 token 返回 131013/131014 等错误，任何查询失败都视为「不是 wiki 节点」放行，
// 只有成功解析出节点时才报错（与官方 drive_import_common.go 一致）。
func rejectDriveImportWikiFolderToken(folderToken, userToken string) error {
	folderToken = strings.TrimSpace(folderToken)
	if folderToken == "" {
		return nil
	}
	node, err := client.ResolveWikiNode(folderToken, userToken)
	if err != nil {
		return nil
	}
	if node == nil || (node.ObjToken == "" && node.NodeToken == "") {
		return nil
	}
	return fmt.Errorf("--folder-token 只支持 Drive 文件夹 token，但提供的 token 解析为 wiki 节点；请改用 Drive 文件夹或省略 --folder-token")
}

func init() {
	driveCmd.AddCommand(driveImportCmd)
	driveImportCmd.Flags().String("file", "", "本地文件路径（必填）")
	driveImportCmd.Flags().String("type", "", "目标文档类型: docx/sheet/bitable/slides（必填）")
	driveImportCmd.Flags().String("folder-token", "", "目标文件夹 token（省略表示根目录）")
	driveImportCmd.Flags().String("name", "", "导入后的文件名（默认本地文件名去扩展名）")
	driveImportCmd.Flags().String("target-token", "", "已有 bitable token（仅 --type bitable）")
	driveImportCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求")
	addAsFlag(driveImportCmd)
	driveImportCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	driveImportCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(driveImportCmd, "file", "type")
}
