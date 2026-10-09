package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var driveTaskScenarios = []string{"import", "export", "task_check", "wiki_move", "wiki_move_to_drive", "wiki_delete_space", "wiki_delete_node"}

var driveTaskResultCmd = &cobra.Command{
	Use:   "task-result",
	Short: "通用异步任务查询（import / export / task_check / wiki_move / wiki_move_to_drive / wiki_delete_space / wiki_delete_node）",
	Long: `统一查询异步任务状态，用于 drive import / export / move / delete 与 wiki 移动、移出到云盘、
删除空间、删除节点等异步任务的 resume。

必填:
  --scenario     任务场景:
                   import              drive import 导入任务（--ticket）
                   export              drive export 导出任务（--ticket + --file-token）
                   task_check          drive move/delete 文件夹异步任务（--task-id）
                   wiki_move           wiki move-docs（云盘文档移入知识库）异步任务（--task-id）
                   wiki_move_to_drive  wiki move-to-drive（知识库节点移出到云盘）异步任务（--task-id）
                   wiki_delete_space   wiki delete-space 异步任务（--task-id）
                   wiki_delete_node    wiki delete 节点删除异步任务（--task-id）

对应入参:
  --ticket       import/export 场景必填
  --file-token   export 场景必填（原始文档 token）
  --task-id      task_check 与所有 wiki_* 场景必填（异步任务 ID）

状态:
  输出 ready / failed / pending；task_check 的失败终态同时识别 "failed" 与删除任务返回的 "fail"。

权限与身份:
  - User / Bot 身份（--as bot|user|auto，默认 auto: User 优先，回退 Bot）

示例:
  feishu-cli drive task-result --scenario export --ticket abcxxx --file-token docxxx
  feishu-cli drive task-result --scenario import --ticket abcxxx
  feishu-cli drive task-result --scenario task_check --task-id xxx
  feishu-cli drive task-result --scenario wiki_move_to_drive --task-id xxx --as user
  feishu-cli drive task-result --scenario wiki_delete_space --task-id xxx --as user
  feishu-cli drive task-result --scenario wiki_delete_node --task-id xxx --as bot`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		scenario, _ := cmd.Flags().GetString("scenario")
		ticket, _ := cmd.Flags().GetString("ticket")
		fileToken, _ := cmd.Flags().GetString("file-token")
		taskID, _ := cmd.Flags().GetString("task-id")
		output, _ := cmd.Flags().GetString("output")

		// 1. 先行完成所有本地参数校验（参数非法时零网络、零 token 刷新）
		output = strings.ToLower(strings.TrimSpace(output))
		if output != "" && output != "json" {
			return fmt.Errorf("不支持的 --output %q，仅支持 json（或留空使用默认格式）", output)
		}

		if err := validateEnum(scenario, "--scenario", driveTaskScenarios); err != nil {
			return err
		}

		switch scenario {
		case "import":
			if ticket == "" {
				return fmt.Errorf("--ticket 在 import 场景必填")
			}
			if err := validateResourceIdentifier(ticket, "--ticket"); err != nil {
				return err
			}
		case "export":
			if ticket == "" {
				return fmt.Errorf("--ticket 在 export 场景必填")
			}
			if err := validateResourceIdentifier(ticket, "--ticket"); err != nil {
				return err
			}
			if fileToken == "" {
				return fmt.Errorf("--file-token 在 export 场景必填（原始文档 token）")
			}
			if err := validateResourceIdentifier(fileToken, "--file-token"); err != nil {
				return err
			}
		case "task_check", "wiki_move", "wiki_move_to_drive", "wiki_delete_space", "wiki_delete_node":
			if taskID == "" {
				return fmt.Errorf("--task-id 在 %s 场景必填", scenario)
			}
			if err := validateResourceIdentifier(taskID, "--task-id"); err != nil {
				return err
			}
		}

		// 2. 本地参数全部合法后，才解析身份与可能触发刷新的 token
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}

		var result map[string]any

		switch scenario {
		case "import":
			status, err := client.GetDriveImportStatus(ticket, token)
			if err != nil {
				return err
			}
			result = map[string]any{
				"scenario":      "import",
				"ticket":        ticket,
				"ready":         status.Ready(),
				"pending":       status.Pending(),
				"failed":        status.Failed(),
				"job_status":    status.JobStatus,
				"job_error_msg": status.JobErrorMsg,
				"doc_token":     status.DocToken,
				"doc_url":       status.DocURL,
				"type":          status.Type,
			}
			// 导入完成且以 Bot 身份查询时，给当前 CLI 登录用户授予新文档 full_access（对齐 drive import）
			if status.Ready() && status.DocToken != "" {
				withPermissionGrant(result, autoGrantCurrentUser(token, status.DocToken, status.Type))
			}
		case "export":
			status, err := client.GetDriveExportStatus(ticket, fileToken, token)
			if err != nil {
				return err
			}
			result = map[string]any{
				"scenario":         "export",
				"ticket":           ticket,
				"ready":            status.Ready(),
				"pending":          status.Pending(),
				"failed":           status.Failed(),
				"job_status":       status.JobStatus,
				"job_status_label": status.StatusLabel(),
				"job_error_msg":    status.JobErrorMsg,
				"file_token":       status.FileToken,
				"file_name":        status.FileName,
				"file_size":        status.FileSize,
				"doc_type":         status.DocType,
				"file_extension":   status.FileExtension,
			}
		case "task_check":
			status, err := client.GetDriveTaskCheck(taskID, token)
			if err != nil {
				return err
			}
			result = map[string]any{
				"scenario": "task_check",
				"task_id":  taskID,
				"status":   status.Status,
				"ready":    status.Ready(),
				"failed":   status.Failed(),
				"pending":  status.Pending(),
			}
		case "wiki_move":
			status, err := client.GetWikiMoveTask(taskID, token)
			if err != nil {
				return err
			}
			code := 1
			if p := status.Primary(); p != nil {
				code = p.Status
			}
			result = map[string]any{
				"scenario":   "wiki_move",
				"task_id":    status.TaskID,
				"ready":      status.Ready(),
				"failed":     status.Failed(),
				"pending":    !status.Ready() && !status.Failed(),
				"status":     code,
				"status_msg": status.StatusLabel(),
			}
			if len(status.MoveResults) > 0 {
				result["move_results"] = status.MoveResults
				if node := status.MoveResults[0].Node; node != nil {
					result["node"] = node
					if tok, ok := node["node_token"].(string); ok && tok != "" {
						result["wiki_token"] = tok
					}
				}
			}
		case "wiki_move_to_drive":
			status, err := client.GetMoveWikiToDriveTask(taskID, token)
			if err != nil {
				return err
			}
			result = map[string]any{
				"scenario":   "wiki_move_to_drive",
				"task_id":    taskIDOr(status.TaskID, taskID),
				"ready":      status.Ready(),
				"failed":     status.Failed(),
				"pending":    !status.Ready() && !status.Failed(),
				"status":     status.Status,
				"status_msg": status.StatusMsg,
				"obj_token":  status.ObjToken,
				"obj_type":   status.ObjType,
				"url":        status.URL,
			}
		case "wiki_delete_space":
			status, err := client.GetWikiDeleteSpaceTask(taskID, token)
			if err != nil {
				return err
			}
			st := status.Status
			if strings.TrimSpace(st) == "" {
				st = "processing"
			}
			result = map[string]any{
				"scenario":   "wiki_delete_space",
				"task_id":    taskIDOr(status.TaskID, taskID),
				"ready":      status.Ready(),
				"failed":     status.Failed(),
				"pending":    !status.Ready() && !status.Failed(),
				"status":     st,
				"status_msg": status.StatusMsg,
			}
		case "wiki_delete_node":
			status, err := client.GetWikiDeleteNodeTask(taskID, token)
			if err != nil {
				return err
			}
			result = map[string]any{
				"scenario":   "wiki_delete_node",
				"task_id":    taskID,
				"ready":      status.Ready(),
				"failed":     status.Failed(),
				"pending":    !status.Ready() && !status.Failed(),
				"status":     status.Status,
				"status_msg": status.StatusMsg,
			}
		}

		if output == "json" {
			return printJSON(result)
		}

		fmt.Printf("scenario: %s\n", scenario)
		for k, v := range result {
			if k == "scenario" {
				continue
			}
			fmt.Printf("  %s: %v\n", k, v)
		}
		return nil
	},
}

// taskIDOr 服务端未回显 task_id 时使用请求里的 task_id。
func taskIDOr(got, fallback string) string {
	if strings.TrimSpace(got) != "" {
		return got
	}
	return fallback
}

func init() {
	driveCmd.AddCommand(driveTaskResultCmd)
	driveTaskResultCmd.Flags().String("scenario", "", "任务场景: import/export/task_check/wiki_move/wiki_move_to_drive/wiki_delete_space/wiki_delete_node（必填）")
	driveTaskResultCmd.Flags().String("as", "auto", "操作身份：bot|user|auto（默认 auto: User 优先，回退 Bot）")
	driveTaskResultCmd.Flags().String("ticket", "", "异步任务 ticket（import/export 必填）")
	driveTaskResultCmd.Flags().String("file-token", "", "原始文档 token（export 必填）")
	driveTaskResultCmd.Flags().String("task-id", "", "异步任务 ID（task_check 与 wiki_* 场景必填）")
	driveTaskResultCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	driveTaskResultCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(driveTaskResultCmd, "scenario")
}
