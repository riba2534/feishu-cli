package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var okrCycleListCmd = &cobra.Command{
	Use:   "list",
	Short: "获取用户的 OKR 周期列表",
	Long: `获取某个用户的 OKR 周期列表（GET /open-apis/okr/v2/cycles，自动分页）。

返回的是**用户周期 ID**，可直接用于 okr cycle detail 和创建目标（v2 /cycles/{id}/...）。
此前版本走 v1 /okr/v1/periods（租户级周期），其 ID 不能用于 cycle detail，属行为变更；
仍需租户级周期（带名称）时加 --tenant。

参数:
  --user-id        周期所属用户（默认当前登录用户；未登录时必须指定）
  --user-id-type   open_id | union_id | user_id（默认 open_id）
  --time-range     只保留与该区间重叠的周期，格式 YYYY-MM--YYYY-MM（如 2026-01--2026-06）
  --tenant         改查租户级周期列表（v1 periods，仅 Bot 身份可用，ID 不能用于 cycle detail）
  --output, -o     输出格式：json

身份（--as，命令组默认 bot）:
  v2 cycles 同时支持 bot 与 user 身份（实测缺 scope 时分别报 99991672 / 99991679）。
  默认 --user-id 取当前登录用户，仅用于确定查询对象，请求本身仍按 --as 选择身份。

权限要求:
  okr:okr.period:readonly（bot 走应用权限，user 需登录时授予同名 scope）

示例:
  # 查自己的周期
  feishu-cli okr cycle list

  # 查某人的周期，只看 2026 上半年
  feishu-cli okr cycle list --user-id ou_xxx --time-range 2026-01--2026-06 -o json

  # 租户级周期（旧行为）
  feishu-cli okr cycle list --tenant`,
	RunE: func(cmd *cobra.Command, args []string) error {
		output, _ := cmd.Flags().GetString("output")
		tenant, _ := cmd.Flags().GetBool("tenant")
		userID, _ := cmd.Flags().GetString("user-id")
		userIDType, _ := cmd.Flags().GetString("user-id-type")
		timeRange, _ := cmd.Flags().GetString("time-range")
		userID = strings.TrimSpace(userID)

		if err := validateUserIDType(userIDType); err != nil {
			return clierr.Usage(err)
		}
		var from, to time.Time
		hasRange := strings.TrimSpace(timeRange) != ""
		if hasRange {
			var err error
			if from, to, err = parseOKRTimeRange(timeRange); err != nil {
				return err
			}
		}
		if tenant && (userID != "" || hasRange) {
			return clierr.Usagef("--tenant 查询租户级周期，不能与 --user-id / --time-range 同用")
		}
		if err := config.Validate(); err != nil {
			return err
		}
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}

		var cycles []*client.OKRCycle
		if tenant {
			cycles, err = client.ListOKRCycles(client.ListOKRCyclesOptions{}, token)
		} else {
			if userID == "" {
				me, idErr := resolveCurrentAuthedUserID(cmd, userIDType)
				if idErr != nil || me == "" {
					return clierr.Usagef("未指定 --user-id，且无法确定当前登录用户（%v）；请用 --user-id 指定要查询的用户", idErr)
				}
				userID = me
			}
			cycles, err = client.ListOKRUserCycles(client.ListOKRUserCyclesOptions{UserID: userID, UserIDType: userIDType}, token)
		}
		if err != nil {
			return err
		}
		if hasRange {
			filtered := make([]*client.OKRCycle, 0, len(cycles))
			for _, c := range cycles {
				if client.OKRCycleOverlaps(c, from, to) {
					filtered = append(filtered, c)
				}
			}
			cycles = filtered
		}
		current := make([]*client.OKRCycle, 0)
		now := time.Now()
		for _, c := range cycles {
			if client.IsCurrentOKRCycle(c, now) {
				current = append(current, c)
			}
		}

		if output == "json" {
			out := map[string]any{
				"cycles": cycles,
				"total":  len(cycles),
			}
			if !tenant {
				out["user_id"] = userID
				out["current_active_cycles"] = current
			}
			return printJSON(out)
		}

		if len(cycles) == 0 {
			fmt.Println("未找到 OKR 周期")
			return nil
		}

		fmt.Printf("共找到 %d 个 OKR 周期\n", len(cycles))
		for idx, c := range cycles {
			name := c.ZhName
			if name == "" {
				name = c.EnName
			}
			if name == "" {
				fmt.Printf("[%d] %s\n", idx+1, c.ID)
			} else {
				fmt.Printf("[%d] %s (%s)\n", idx+1, name, c.ID)
			}
			if c.StartTime != "" || c.EndTime != "" {
				fmt.Printf("    时间: %s ~ %s\n", c.StartTime, c.EndTime)
			}
			if c.CycleStatus != "" {
				fmt.Printf("    状态: %s\n", c.CycleStatus)
			}
		}
		if len(current) > 0 {
			fmt.Println("\n当前进行中的周期:")
			for _, c := range current {
				fmt.Printf("  %s（%s ~ %s）\n", c.ID, c.StartTime, c.EndTime)
			}
		}
		return nil
	},
}

// parseOKRTimeRange 解析 YYYY-MM--YYYY-MM：起始月第一刻 ~ 结束月最后一刻（本地时区）
func parseOKRTimeRange(s string) (time.Time, time.Time, error) {
	parts := strings.SplitN(strings.TrimSpace(s), "--", 2)
	if len(parts) != 2 {
		return time.Time{}, time.Time{}, clierr.Usagef("--time-range 格式应为 YYYY-MM--YYYY-MM，得到 %q", s)
	}
	from, err := time.ParseInLocation("2006-01", strings.TrimSpace(parts[0]), time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, clierr.Usagef("--time-range 起始月 %q 无效", parts[0])
	}
	end, err := time.ParseInLocation("2006-01", strings.TrimSpace(parts[1]), time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, clierr.Usagef("--time-range 结束月 %q 无效", parts[1])
	}
	to := end.AddDate(0, 1, 0).Add(-time.Millisecond)
	if from.After(to) {
		return time.Time{}, time.Time{}, clierr.Usagef("--time-range 起始月晚于结束月")
	}
	return from, to, nil
}

// validateUserIDType 校验 user-id-type 取值
func validateUserIDType(t string) error {
	switch t {
	case "open_id", "union_id", "user_id":
		return nil
	default:
		return fmt.Errorf("不支持的 --user-id-type: %s（可选: open_id / union_id / user_id）", t)
	}
}

func init() {
	okrCycleCmd.AddCommand(okrCycleListCmd)

	okrCycleListCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	okrCycleListCmd.Flags().String("user-id", "", "周期所属用户 ID（默认当前登录用户）")
	okrCycleListCmd.Flags().String("user-id-type", "open_id", "用户 ID 类型：open_id / union_id / user_id")
	okrCycleListCmd.Flags().String("time-range", "", "只保留与该区间重叠的周期，格式 YYYY-MM--YYYY-MM")
	okrCycleListCmd.Flags().Bool("tenant", false, "查询租户级周期（v1 periods，旧行为；ID 不能用于 cycle detail）")
}
