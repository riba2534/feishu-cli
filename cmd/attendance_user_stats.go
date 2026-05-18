package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var attendanceUserStatsCmd = &cobra.Command{
	Use:     "user-stats",
	Aliases: []string{"user-stats-data", "stats"},
	Short:   "考勤统计数据",
	Long: `考勤统计数据相关命令（对应 OpenAPI 资源 user_stats_data）。

子命令:
  query  查询用户考勤统计数据（日度 / 月度）

示例:
  feishu-cli attendance user-stats query \
      --employee-type open_id --user-ids ou_xxx --current-user-id ou_xxx \
      --stats-type daily --start 2026-05-01 --end 2026-05-18`,
}

var attendanceUserStatsQueryCmd = &cobra.Command{
	Use:   "query",
	Short: "查询用户考勤统计数据",
	Long: `查询日度或月度的考勤统计数据。

对应 OpenAPI: POST /open-apis/attendance/v1/user_stats_datas/query
权限要求: tenant_access_token；应用需获得 attendance:task:readonly 权限
（larksuite/oapi-sdk-go v3.5.3 该接口仅支持 tenant token）

参数:
  --employee-type      用户 ID 类型 employee_id|open_id|user_id|employee_no（默认 employee_id）
  --stats-type         统计类型：daily（日度）/ month（月度），默认 daily
  --user-ids           查询的用户 ID 列表，逗号分隔，最多 200 个（必填）
  --current-user-id    发起请求的用户 ID（新系统用户必填，同【查询统计设置】的 user_id）
  --start              起始日期（必填，YYYY-MM-DD 或 YYYYMMDD）
  --end                结束日期（必填，YYYY-MM-DD 或 YYYYMMDD，跨度 ≤ 31 天）
  --locale             语言：zh / en / ja
  --need-history       是否返回历史数据（默认 false）
  --current-group-only 仅展示当前考勤组（默认 false）
  --output, -o         输出格式：text（默认）/ json

示例:
  # 查本人 5 月日度统计
  feishu-cli attendance user-stats query \
      --employee-type open_id \
      --user-ids ou_xxxxxxxx --current-user-id ou_xxxxxxxx \
      --stats-type daily --start 2026-05-01 --end 2026-05-31

  # 查月度统计 + JSON
  feishu-cli attendance user-stats query \
      --employee-type open_id --user-ids ou_xxx --current-user-id ou_xxx \
      --stats-type month --start 2026-05-01 --end 2026-05-31 -o json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		employeeType, _ := cmd.Flags().GetString("employee-type")
		statsType, _ := cmd.Flags().GetString("stats-type")
		userIDsRaw, _ := cmd.Flags().GetString("user-ids")
		currentUserID, _ := cmd.Flags().GetString("current-user-id")
		startStr, _ := cmd.Flags().GetString("start")
		endStr, _ := cmd.Flags().GetString("end")
		locale, _ := cmd.Flags().GetString("locale")
		needHistory, _ := cmd.Flags().GetBool("need-history")
		currentGroupOnly, _ := cmd.Flags().GetBool("current-group-only")
		output, _ := cmd.Flags().GetString("output")

		if err := validateEnum(statsType, "stats-type", []string{"daily", "month"}); err != nil {
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
		if len(userIDs) == 0 {
			return fmt.Errorf("--user-ids 不能为空")
		}
		if len(userIDs) > 200 {
			return fmt.Errorf("--user-ids 单次最多 200 个，当前 %d 个", len(userIDs))
		}

		startDate, err := client.ParseAttendanceDate(startStr)
		if err != nil {
			return fmt.Errorf("--start: %w", err)
		}
		endDate, err := client.ParseAttendanceDate(endStr)
		if err != nil {
			return fmt.Errorf("--end: %w", err)
		}
		if startDate > endDate {
			return fmt.Errorf("--start (%d) 不能晚于 --end (%d)", startDate, endDate)
		}
		// OpenAPI 限制：单次查询跨度 ≤ 31 天
		tStart, _ := time.Parse("20060102", strconv.Itoa(startDate))
		tEnd, _ := time.Parse("20060102", strconv.Itoa(endDate))
		if tEnd.Sub(tStart) > 31*24*time.Hour {
			return fmt.Errorf("--start 到 --end 跨度不能超过 31 天（当前 %.0f 天，请缩短范围或多次查询）", tEnd.Sub(tStart).Hours()/24)
		}

		result, err := client.QueryAttendanceUserStats(
			employeeType,
			statsType,
			startDate,
			endDate,
			userIDs,
			currentUserID,
			locale,
			needHistory,
			currentGroupOnly,
		)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(result)
		}

		printAttendanceUserStatsResult(result)
		return nil
	},
}

// printAttendanceUserStatsResult 人类可读输出。
func printAttendanceUserStatsResult(r *client.AttendanceQueryUserStatsResult) {
	if r == nil || len(r.UserDatas) == 0 {
		fmt.Println("未查询到统计数据")
		if r != nil && len(r.InvalidUserList) > 0 {
			fmt.Printf("⚠ 无权限用户 (%d): %s\n", len(r.InvalidUserList), strings.Join(r.InvalidUserList, ", "))
		}
		return
	}

	fmt.Printf("共 %d 个用户的统计数据:\n\n", len(r.UserDatas))
	for i, u := range r.UserDatas {
		name := u.Name
		if name == "" {
			name = "(未返回姓名)"
		}
		fmt.Printf("[%d] %s (%s)\n", i+1, name, u.UserID)
		for _, cell := range u.Datas {
			title := cell.Title
			if title == "" {
				title = cell.Code
			}
			fmt.Printf("    %-20s = %s\n", title, cell.Value)
		}
		fmt.Println()
	}

	if len(r.InvalidUserList) > 0 {
		fmt.Printf("⚠ 无权限用户 (%d): %s\n", len(r.InvalidUserList), strings.Join(r.InvalidUserList, ", "))
	}
}

func init() {
	attendanceCmd.AddCommand(attendanceUserStatsCmd)
	attendanceUserStatsCmd.AddCommand(attendanceUserStatsQueryCmd)

	attendanceUserStatsQueryCmd.Flags().String("employee-type", "employee_id", "用户 ID 类型：employee_id | open_id | user_id | employee_no")
	attendanceUserStatsQueryCmd.Flags().String("stats-type", "daily", "统计类型：daily（日度） | month（月度）")
	attendanceUserStatsQueryCmd.Flags().String("user-ids", "", "查询的用户 ID 列表（逗号分隔，最多 200 个，必填）")
	attendanceUserStatsQueryCmd.Flags().String("current-user-id", "", "发起请求的用户 ID（新系统用户必填，对应【查询统计设置】user_id）")
	attendanceUserStatsQueryCmd.Flags().String("start", "", "起始日期（YYYY-MM-DD 或 YYYYMMDD，必填）")
	attendanceUserStatsQueryCmd.Flags().String("end", "", "结束日期（YYYY-MM-DD 或 YYYYMMDD，必填；跨度 ≤ 31 天）")
	attendanceUserStatsQueryCmd.Flags().String("locale", "", "语言：zh / en / ja")
	attendanceUserStatsQueryCmd.Flags().Bool("need-history", false, "是否返回历史数据")
	attendanceUserStatsQueryCmd.Flags().Bool("current-group-only", false, "仅展示当前考勤组")
	attendanceUserStatsQueryCmd.Flags().StringP("output", "o", "text", "输出格式：text | json")

	mustMarkFlagRequired(attendanceUserStatsQueryCmd, "user-ids", "start", "end")
}
