package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var attendanceUserTaskCmd = &cobra.Command{
	Use:     "user-task",
	Aliases: []string{"user-tasks", "task"},
	Short:   "考勤打卡记录",
	Long: `考勤打卡记录相关命令（对应 OpenAPI 资源 user_tasks）。

子命令:
  query  查询用户考勤打卡记录

示例:
  # 查本人（User Token，无需 --user-ids）
  feishu-cli attendance user-task query \
      --employee-type employee_no \
      --start 2026-05-01 --end 2026-05-18

  # 按 employee_id 查指定员工（--employee-type 只支持 employee_id / employee_no，不支持 open_id）
  feishu-cli attendance user-task query \
      --employee-type employee_id --user-ids 2847xxxx \
      --start 2026-05-01 --end 2026-05-18`,
}

var attendanceUserTaskQueryCmd = &cobra.Command{
	Use:   "query",
	Short: "查询用户考勤打卡记录",
	Long: `查询指定用户在某段日期内的考勤打卡记录（上下班实际打卡结果）。

对应 OpenAPI: POST /open-apis/attendance/v1/user_tasks/query
权限要求: attendance:task:readonly 或 attendance:task
支持 User Access Token 与 Tenant Access Token。

参数:
  --employee-type        用户 ID 类型：employee_id | employee_no（默认 employee_id）
  --user-ids             用户 ID 列表，逗号分隔，最多 50 个（留空则按 employee_no 查询本人）
  --start                查询起始工作日（必填，YYYY-MM-DD 或 YYYYMMDD）
  --end                  查询结束工作日（必填，YYYY-MM-DD 或 YYYYMMDD）
  --need-overtime        是否包含加班班段打卡（默认 false）
  --ignore-invalid-users 忽略无效/无权限用户（默认 true）
  --include-terminated   包含离职员工数据（默认 false）
  --user-access-token    User Access Token（覆盖登录态）
  --output, -o           输出格式：text（默认）/ json

示例:
  # 查询本人 5/1-5/18 打卡（无需指定 --user-ids）
  feishu-cli attendance user-task query \
      --start 2026-05-01 --end 2026-05-18

  # 按工号查询指定员工
  feishu-cli attendance user-task query \
      --employee-type employee_no --user-ids 10001,10002 \
      --start 2026-05-01 --end 2026-05-18

  # 多人 + 加班 + JSON
  feishu-cli attendance user-task query \
      --employee-type employee_id \
      --user-ids 2847xxxx,2848xxxx \
      --start 20260501 --end 20260518 \
      --need-overtime -o json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		employeeType, _ := cmd.Flags().GetString("employee-type")
		userIDsRaw, _ := cmd.Flags().GetString("user-ids")
		startStr, _ := cmd.Flags().GetString("start")
		endStr, _ := cmd.Flags().GetString("end")
		needOvertime, _ := cmd.Flags().GetBool("need-overtime")
		ignoreInvalid, _ := cmd.Flags().GetBool("ignore-invalid-users")
		includeTerminated, _ := cmd.Flags().GetBool("include-terminated")
		output, _ := cmd.Flags().GetString("output")

		if err := validateEnum(employeeType, "employee-type", []string{"employee_id", "employee_no"}); err != nil {
			return err
		}

		userIDs := splitAndTrim(userIDsRaw)
		// 局部去重（不改公共 helper splitAndTrim，避免影响其他模块）
		seen := make(map[string]bool)
		unique := make([]string, 0, len(userIDs))
		for _, id := range userIDs {
			if !seen[id] {
				seen[id] = true
				unique = append(unique, id)
			}
		}
		userIDs = unique

		// 官方本人自查路径：未传 user-ids 时使用 employee_no + 空 user_ids
		if len(userIDs) == 0 {
			if !cmd.Flags().Changed("employee-type") {
				employeeType = "employee_no"
			} else if employeeType != "employee_no" {
				return fmt.Errorf("--employee-type 为 %s 时必须指定 --user-ids（如需查询本人请使用 --employee-type employee_no 且无需指定 --user-ids）", employeeType)
			}
		}

		if len(userIDs) > 50 {
			return fmt.Errorf("--user-ids 单次最多 50 个，当前 %d 个", len(userIDs))
		}

		dateFrom, err := client.ParseAttendanceDate(startStr)
		if err != nil {
			return fmt.Errorf("--start: %w", err)
		}
		dateTo, err := client.ParseAttendanceDate(endStr)
		if err != nil {
			return fmt.Errorf("--end: %w", err)
		}
		if dateFrom > dateTo {
			return fmt.Errorf("--start (%d) 不能晚于 --end (%d)", dateFrom, dateTo)
		}

		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}

		result, err := client.QueryAttendanceUserTasks(
			employeeType,
			userIDs,
			dateFrom,
			dateTo,
			needOvertime,
			ignoreInvalid,
			includeTerminated,
			token,
		)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(result)
		}

		printAttendanceUserTaskResult(result)
		return nil
	},
}

// printAttendanceUserTaskResult 人类可读输出。
func printAttendanceUserTaskResult(r *client.AttendanceQueryUserTaskResult) {
	if r == nil || len(r.UserTaskResults) == 0 {
		fmt.Println("未查询到打卡记录")
		if r != nil {
			printAttendanceInvalidLists(r.InvalidUserIDs, r.UnauthorizedUserIDs)
		}
		return
	}

	fmt.Printf("共 %d 条打卡任务:\n\n", len(r.UserTaskResults))
	for i, t := range r.UserTaskResults {
		name := t.EmployeeName
		if name == "" {
			name = "(未返回姓名)"
		}
		fmt.Printf("[%d] %s (%s)  日期: %s\n", i+1, name, t.UserID, client.FormatAttendanceDate(t.Day))
		if t.GroupID != "" {
			fmt.Printf("    考勤组: %s   班次: %s   打卡记录 ID: %s\n", t.GroupID, t.ShiftID, t.ResultID)
		}
		for j, rec := range t.Records {
			label := "上下班"
			if rec.TaskShiftType == 1 {
				label = "加班"
			}
			fmt.Printf("    [%d] (%s)\n", j+1, label)
			if rec.CheckInShiftTime != "" || rec.CheckInResult != "" {
				fmt.Printf("        上班: %s  结果: %s%s\n",
					emptyDash(rec.CheckInShiftTime),
					emptyDash(rec.CheckInResult),
					supplement(rec.CheckInResultSupplement))
			}
			if rec.CheckOutShiftTime != "" || rec.CheckOutResult != "" {
				fmt.Printf("        下班: %s  结果: %s%s\n",
					emptyDash(rec.CheckOutShiftTime),
					emptyDash(rec.CheckOutResult),
					supplement(rec.CheckOutResultSupplement))
			}
		}
		fmt.Println()
	}

	printAttendanceInvalidLists(r.InvalidUserIDs, r.UnauthorizedUserIDs)
}

func printAttendanceInvalidLists(invalid, unauthorized []string) {
	if len(invalid) > 0 {
		fmt.Printf("⚠ 无效用户 ID (%d): %s\n", len(invalid), strings.Join(invalid, ", "))
	}
	if len(unauthorized) > 0 {
		fmt.Printf("⚠ 无权限用户 ID (%d): %s\n", len(unauthorized), strings.Join(unauthorized, ", "))
	}
}

func emptyDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func supplement(s string) string {
	if s == "" {
		return ""
	}
	return "  (" + s + ")"
}

func init() {
	attendanceCmd.AddCommand(attendanceUserTaskCmd)
	attendanceUserTaskCmd.AddCommand(attendanceUserTaskQueryCmd)

	attendanceUserTaskQueryCmd.Flags().String("employee-type", "employee_id", "用户 ID 类型：employee_id | employee_no（默认 employee_id）")
	attendanceUserTaskQueryCmd.Flags().String("user-ids", "", "用户 ID 列表（逗号分隔，最多 50 个；留空则按 employee_no 自查本人）")
	attendanceUserTaskQueryCmd.Flags().String("start", "", "查询起始工作日（YYYY-MM-DD 或 YYYYMMDD，必填）")
	attendanceUserTaskQueryCmd.Flags().String("end", "", "查询结束工作日（YYYY-MM-DD 或 YYYYMMDD，必填）")
	attendanceUserTaskQueryCmd.Flags().Bool("need-overtime", false, "是否包含加班班段打卡结果")
	attendanceUserTaskQueryCmd.Flags().Bool("ignore-invalid-users", true, "忽略无效或无权限用户，仅返回有效数据")
	attendanceUserTaskQueryCmd.Flags().Bool("include-terminated", false, "包含离职员工数据")
	attendanceUserTaskQueryCmd.Flags().String("as", "auto", "身份选择: bot | user | auto（默认 auto）")
	attendanceUserTaskQueryCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	attendanceUserTaskQueryCmd.Flags().StringP("output", "o", "text", "输出格式：text | json")

	mustMarkFlagRequired(attendanceUserTaskQueryCmd, "start", "end")
}
