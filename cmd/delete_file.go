package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var deleteFileCmd = &cobra.Command{
	Use:   "delete <file_token>",
	Short: "删除文件或文件夹",
	Long: `删除云空间中的文件或文件夹（移入回收站）。

警告: 删除操作需谨慎！

参数:
  file_token    文件或文件夹的 Token
  --type        文件类型（必填）

删除以异步模式提交（async=true）：服务端返回 task_id 时（如文件夹）默认轮询 task_check 直到完成，
失败状态（failed / fail）会报错；轮询窗口内未完成时输出续查命令。--wait=false 只提交不轮询。

文件类型:
  doc       旧版文档
  docx      新版文档
  sheet     电子表格
  bitable   多维表格
  mindnote  思维笔记
  file      普通文件
  folder    文件夹
  slides    幻灯片
  （wiki 节点请用 feishu-cli wiki delete）

示例:
  # 删除文档
  feishu-cli file delete doccnXXX --type docx

  # 删除文件夹并等待异步任务完成
  feishu-cli file delete fldcnXXX --type folder --force

  # 只提交删除，不轮询
  feishu-cli file delete fldcnXXX --type folder --force --wait=false -o json

  # 预览请求
  feishu-cli file delete fldcnXXX --type folder --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		fileToken := strings.TrimSpace(args[0])
		fileType, _ := cmd.Flags().GetString("type")
		force, _ := cmd.Flags().GetBool("force")
		wait, _ := cmd.Flags().GetBool("wait")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		output, _ := cmd.Flags().GetString("output")
		fileType = strings.ToLower(strings.TrimSpace(fileType))

		if err := validateResourceIdentifier(fileToken, "<file_token>"); err != nil {
			return err
		}
		if fileType == "wiki" {
			return clierr.Usagef("file delete 不支持 wiki 节点，请改用 feishu-cli wiki delete")
		}

		if dryRun {
			return printJSON(map[string]any{
				"dry_run": true,
				"requests": []map[string]any{
					{"method": "DELETE", "path": "/open-apis/drive/v1/files/" + fileToken, "params": map[string]any{"type": fileType, "async": true}},
					{"method": "GET", "path": "/open-apis/drive/v1/files/task_check", "params": map[string]any{"task_id": "<task_id>"}, "desc": "返回 task_id 时轮询"},
				},
			})
		}

		userAccessToken := resolveOptionalUserToken(cmd)

		// 危险操作确认（--force / --yes 跳过；非交互且未确认时报错退出码 10）
		if !force {
			if err := confirmDangerousAction(cmd, fmt.Sprintf("确定要删除文件 %s (%s) 吗？", fileToken, fileType)); err != nil {
				return err
			}
		}

		taskID, err := client.DeleteDriveFileAsync(fileToken, fileType, userAccessToken)
		if err != nil {
			return err
		}

		result := map[string]any{
			"file_token": fileToken,
			"type":       fileType,
		}
		if taskID == "" {
			result["deleted"] = true
			result["ready"] = true
			return printDeleteFileResult(result, output)
		}

		result["task_id"] = taskID
		identity := "bot"
		if userAccessToken != "" {
			identity = "user"
		}
		nextCmd := fmt.Sprintf("feishu-cli drive task-result --scenario task_check --task-id %s --as %s", quotePOSIXShell(taskID), identity)
		if !wait {
			result["ready"] = false
			result["next_command"] = nextCmd
			return printDeleteFileResult(result, output)
		}

		status, timedOut, err := client.WaitDriveTaskCheckWithBound(taskID, userAccessToken)
		if err != nil {
			if status != nil && status.Failed() {
				return deleteTaskFailedError(err, identity)
			}
			return withDrivePollResume(err, nextCmd)
		}
		if timedOut {
			result["ready"] = false
			result["timed_out"] = true
			result["next_command"] = nextCmd
			if status != nil {
				result["status"] = status.Status
			}
			return printDeleteFileResult(result, output)
		}
		result["ready"] = true
		result["deleted"] = true
		result["status"] = status.Status
		return printDeleteFileResult(result, output)
	},
}

func printDeleteFileResult(result map[string]any, output string) error {
	if output == "json" {
		return printJSON(result)
	}
	if result["deleted"] == true {
		fmt.Printf("删除成功！\n")
	} else {
		fmt.Printf("删除操作已提交！\n")
	}
	fmt.Printf("  文件 Token: %v\n", result["file_token"])
	fmt.Printf("  文件类型:   %v\n", result["type"])
	if id, ok := result["task_id"]; ok {
		fmt.Printf("  任务 ID:    %v\n", id)
	}
	if next, ok := result["next_command"]; ok {
		fmt.Printf("  续查:       %v\n", next)
	}
	return nil
}

func init() {
	fileCmd.AddCommand(deleteFileCmd)
	deleteFileCmd.Flags().String("type", "", "文件类型（必填）")
	deleteFileCmd.Flags().BoolP("force", "f", false, "跳过确认直接删除")
	deleteFileCmd.Flags().Bool("wait", true, "返回 task_id 时轮询直到完成（--wait=false 只提交）")
	deleteFileCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求")
	deleteFileCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	deleteFileCmd.Flags().String("user-access-token", "", "User Access Token（可选，使用用户身份访问文件）")
	mustMarkFlagRequired(deleteFileCmd, "type")
}

// deleteTaskFailedError 为异步删除任务失败补充原因提示。
//
// 异步删除（async=true）失败时 task_check 只返回 status=fail，不带原因；改为异步之前，
// 同步删除会直接返回"无权限"等具体错误码。实测最常见的原因是身份不对：
// Bot 无权删除用户自己创建的文件（如 User 身份 sheet create 建的表格）。
func deleteTaskFailedError(err error, identity string) error {
	if identity == "bot" {
		return fmt.Errorf("%w\n提示：服务端未返回失败原因，最常见的是当前 Bot 身份无权删除该文件"+
			"（Bot 只能删除自己创建或拥有管理权限的文件）。用户自有文件请加 --user-access-token 以用户身份删除，"+
			"或用 feishu-cli perm list <token> --doc-type <type> --as user 确认 Bot 是否有管理权限", err)
	}
	return fmt.Errorf("%w\n提示：服务端未返回失败原因，常见原因是当前用户不是所有者且无管理权限，"+
		"或文件已被删除；可用 feishu-cli drive inspect --url <token> 核对文件状态", err)
}
