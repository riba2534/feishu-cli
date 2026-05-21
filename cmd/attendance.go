package cmd

import "github.com/spf13/cobra"

var attendanceCmd = &cobra.Command{
	Use:     "attendance",
	Aliases: []string{"att"},
	Short:   "考勤打卡操作命令",
	Long: `考勤打卡操作命令，对接飞书考勤 OpenAPI。

子命令:
  user-task query    查询用户考勤打卡记录（user_tasks.query）
  user-stats query   查询用户考勤统计数据（user_stats_datas.query）

身份要求:
  全部命令走 tenant_access_token（应用身份），无需 ` + "`auth login`" + `；
  larksuite/oapi-sdk-go v3.5.3 中相关接口仅支持 Tenant token。

Scope（在飞书开放平台「应用权限管理」页面授予应用）:
  attendance:task:readonly  打卡 / 统计查询（推荐）
  attendance:task           打卡读写

日期格式:
  接受 YYYY-MM-DD 或 YYYYMMDD（飞书 API 内部统一用 yyyyMMdd 整数）。

示例:
  # 查询本人最近一周打卡
  feishu-cli attendance user-task query \
      --employee-type open_id \
      --user-ids ou_xxxxxxxxx \
      --start 2026-05-01 --end 2026-05-18

  # 查询本月日度统计
  feishu-cli attendance user-stats query \
      --employee-type open_id \
      --user-ids ou_xxxxxxxxx \
      --current-user-id ou_xxxxxxxxx \
      --stats-type daily --start 2026-05-01 --end 2026-05-18

  # JSON 输出（适合 AI Agent 解析）
  feishu-cli attendance user-task query --user-ids ou_xxx --start 2026-05-01 --end 2026-05-18 -o json

注意:
  - 考勤 API 涉及员工隐私，需企业管理员在飞书后台为应用开通 attendance:task* scope。
  - user_ids 单次最多 50 个（user-task）/ 200 个（user-stats）。
  - user-stats 起止日期跨度不超过 31 天。`,
}

func init() {
	rootCmd.AddCommand(attendanceCmd)
}
