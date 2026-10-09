package cmd

import (
	"fmt"
	"os"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var driveMoveAllowedTypes = []string{"file", "docx", "doc", "sheet", "bitable", "mindnote", "folder", "slides"}

var driveMoveCmd = &cobra.Command{
	Use:   "move",
	Short: "移动文件/文件夹（folder 移动时轮询异步任务）",
	Long: `移动文件或文件夹到新位置。

- 文件移动：同步返回
- 文件夹移动：异步任务，自动轮询 task_check（最多 30×2s），超时返回 task_id 可用 drive task-result 继续
- 省略 --folder-token 时先 GET /open-apis/drive/explorer/v2/root_folder/meta 取真实根目录 token

必填:
  --file-token     要移动的文件/文件夹 token
  --type           类型: file / docx / doc / sheet / bitable / mindnote / folder / slides

可选:
  --folder-token   目标文件夹 token（默认真实根目录）
  --as             bot|user|auto（默认 auto：User 优先；未配置回退 Bot；已配置但解析/刷新失败 fail-closed）
  --dry-run        只打印将要发出的请求（不解析/刷新 token）
  --user-access-token  覆盖登录态

示例:
  feishu-cli drive move --file-token boxxxx --type docx --folder-token fldxxx
  feishu-cli drive move --file-token fldxxx --type folder`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken, _ := cmd.Flags().GetString("file-token")
		folderToken, _ := cmd.Flags().GetString("folder-token")
		fileType, _ := cmd.Flags().GetString("type")
		output, _ := cmd.Flags().GetString("output")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		if fileToken == "" {
			return fmt.Errorf("--file-token 必填")
		}
		if err := validateEnum(fileType, "--type", driveMoveAllowedTypes); err != nil {
			return err
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}

		if dryRun {
			var steps []dryRunStep
			dest := folderToken
			if dest == "" {
				steps = append(steps, dryRunStep{
					Method: "GET",
					URL:    "/open-apis/drive/explorer/v2/root_folder/meta",
					Desc:   "Resolve the caller's real Drive root folder token",
				})
				dest = "<root_folder_token>"
			}
			steps = append(steps, dryRunStep{
				Method: "POST",
				URL:    "/open-apis/drive/v1/files/" + fileToken + "/move",
				Desc:   "Move file/folder",
				Body: map[string]any{
					"type":         fileType,
					"folder_token": dest,
				},
			})
			if fileType == "folder" {
				steps = append(steps, dryRunStep{
					Method: "GET",
					URL:    "/open-apis/drive/v1/files/task_check",
					Desc:   "Poll async task status (for folder move)",
					Params: map[string]any{"task_id": "<task_id>"},
				})
			}
			return printDryRunPlan(cmd, "Move file or folder in Drive", map[string]any{
				"file_token": fileToken,
				"type":       fileType,
			}, steps)
		}

		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}

		if folderToken == "" {
			fmt.Fprintf(os.Stderr, "未指定目标文件夹，获取根目录 token...\n")
			rootToken, err := client.GetRootFolderToken(token)
			if err != nil {
				return err
			}
			if rootToken == "" {
				return fmt.Errorf("获取根目录 token 失败：返回为空")
			}
			folderToken = rootToken
		}

		taskID, err := client.MoveFileWithToken(fileToken, folderToken, fileType, token)
		if err != nil {
			return err
		}

		result := map[string]any{
			"file_token":   fileToken,
			"type":         fileType,
			"folder_token": folderToken,
		}

		if taskID == "" {
			result["ready"] = true
			if output == "json" {
				return printJSON(result)
			}
			fmt.Println("文件移动成功")
			return nil
		}

		result["task_id"] = taskID
		fmt.Fprintf(os.Stderr, "文件夹移动任务: %s，开始轮询...\n", taskID)

		nextCmd := fmt.Sprintf("feishu-cli drive task-result --scenario task_check --task-id %s --as %s", quotePOSIXShell(taskID), resumeIdentity(cmd))
		status, timedOut, err := client.WaitDriveTaskCheckWithBound(taskID, token)
		if err != nil {
			return withDrivePollResume(err, nextCmd)
		}

		if timedOut {
			result["ready"] = false
			result["timed_out"] = true
			result["next_command"] = nextCmd
			if status != nil {
				result["status"] = status.Status
			}
			if output == "json" {
				_ = printJSON(result)
			} else {
				fmt.Fprintf(os.Stderr, "移动仍在进行中，继续: %s\n", nextCmd)
			}
			return nil
		}

		result["ready"] = true
		result["status"] = status.Status
		if output == "json" {
			return printJSON(result)
		}
		fmt.Printf("文件夹移动成功 (task_id=%s, status=%s)\n", taskID, status.Status)
		return nil
	},
}

func init() {
	driveCmd.AddCommand(driveMoveCmd)
	driveMoveCmd.Flags().String("file-token", "", "要移动的文件/文件夹 token（必填）")
	driveMoveCmd.Flags().String("type", "", "类型: file/docx/doc/sheet/bitable/mindnote/folder/slides（必填）")
	driveMoveCmd.Flags().String("folder-token", "", "目标文件夹 token（默认真实根目录）")
	driveMoveCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求")
	addAsFlag(driveMoveCmd)
	driveMoveCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	driveMoveCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(driveMoveCmd, "file-token", "type")
}
