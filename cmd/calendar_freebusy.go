package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var calendarFreebusyCmd = &cobra.Command{
	Use:   "freebusy",
	Short: "查询忙闲信息",
	Long: `查询一个或多个用户在指定时间段内的忙闲信息（POST /calendar/v4/freebusy/batch）。

参数:
  --start          起始时间，RFC3339 / YYYY-MM-DD / Unix 秒（默认今天 00:00）
  --end            结束时间，同上；日期按当天 23:59:59（默认起始日当天结束）
  --user-id        用户 open_id，可重复或逗号分隔；不传时默认当前登录用户
                   （Bot 身份没有"本人"，必须显式指定）
  --type           输出视图（默认 busy）:
                     busy         每人的忙碌区间（已按时间排序，并合并重叠/相邻区间，区间数≠日程数）
                     raw_busy     每人的原始忙碌条目（未合并，带 rsvp_status）
                     free         每人的空闲时段
                     common_free  所有人的共同空闲时段（多人约会时间）
  --min-duration   free / common_free 只保留不短于该时长的时段（如 30m、1h）

输出（-o json）:
  单个用户且 --type busy：数组 [{start_time, end_time}]（与旧版一致，已排序合并）
  多个用户或其他视图：{"users":[{"user_id":...,"busy"|"raw_busy"|"free":[...]}]}，
  common_free 为 {"common_free":[{start_time,end_time,duration}]}

示例:
  # 查自己今天的忙碌时段
  feishu-cli calendar freebusy

  # 查某人某天
  feishu-cli calendar freebusy --start 2024-01-21 --end 2024-01-21 --user-id ou_xxx

  # 多人共同空闲（至少 30 分钟）
  feishu-cli calendar freebusy --start 2024-01-21T09:00:00+08:00 --end 2024-01-21T18:00:00+08:00 \
    --user-id ou_aaa,ou_bbb --type common_free --min-duration 30m`,
	RunE: func(cmd *cobra.Command, args []string) error {
		startInput, _ := cmd.Flags().GetString("start")
		endInput, _ := cmd.Flags().GetString("end")
		userIDs, _ := cmd.Flags().GetStringSlice("user-id")
		viewType, _ := cmd.Flags().GetString("type")
		minDurStr, _ := cmd.Flags().GetString("min-duration")
		output, _ := cmd.Flags().GetString("output")

		winStart, winEnd, err := parseFreebusyWindow(startInput, endInput, time.Now())
		if err != nil {
			return err
		}
		viewType = strings.TrimSpace(viewType)
		if viewType == "" {
			viewType = "busy"
		}
		if err := validateEnum(viewType, "--type", []string{"busy", "raw_busy", "free", "common_free"}); err != nil {
			return err
		}
		var minDur time.Duration
		if strings.TrimSpace(minDurStr) != "" {
			minDur, err = time.ParseDuration(strings.TrimSpace(minDurStr))
			if err != nil || minDur < 0 {
				return clierr.Usagef("--min-duration %q 无效（示例：30m、1h）", minDurStr)
			}
		}
		ids := dedupeNonEmpty(userIDs)
		for _, id := range ids {
			if !strings.HasPrefix(id, "ou_") {
				return clierr.Usagef("--user-id 需要 open_id（ou_ 开头），得到 %q", id)
			}
		}

		if err := config.Validate(); err != nil {
			return err
		}
		token := resolveOptionalUserTokenWithFallback(cmd)
		if len(ids) == 0 {
			if token == "" {
				return clierr.Usagef("未指定 --user-id：Bot 身份没有\"本人\"日历，请用 --user-id 指定要查询的用户（或先 auth login 以本人身份查询）")
			}
			me, err := resolveCurrentAuthedUserID(cmd, "open_id")
			if err != nil || me == "" {
				return fmt.Errorf("无法确定当前登录用户，请用 --user-id 指定: %v", err)
			}
			ids = []string{me}
		}

		raw, err := client.ListFreebusyBatch(winStart.Format(time.RFC3339), winEnd.Format(time.RFC3339), ids, token)
		if err != nil {
			return err
		}
		merged := make(map[string][]*client.FreebusyInfo, len(ids))
		for _, id := range ids {
			merged[id] = client.MergeFreebusyIntervals(raw[id])
		}

		switch viewType {
		case "busy":
			if output == "json" {
				if len(ids) == 1 {
					return printJSON(merged[ids[0]])
				}
				users := make([]map[string]any, 0, len(ids))
				for _, id := range ids {
					users = append(users, map[string]any{"user_id": id, "busy": merged[id]})
				}
				return printJSON(map[string]any{"users": users})
			}
			for _, id := range ids {
				printFreebusyHeader(ids, id)
				if len(merged[id]) == 0 {
					fmt.Println("该时间段内无忙碌时间")
					continue
				}
				fmt.Printf("忙碌时间段（共 %d 个，已合并重叠区间）:\n\n", len(merged[id]))
				for i, fb := range merged[id] {
					fmt.Printf("[%d] %s ~ %s\n", i+1, fb.StartTime, fb.EndTime)
				}
			}
			return nil

		case "raw_busy":
			if output == "json" {
				users := make([]map[string]any, 0, len(ids))
				for _, id := range ids {
					items := raw[id]
					if items == nil {
						items = []*client.FreebusyRawItem{}
					}
					users = append(users, map[string]any{"user_id": id, "raw_busy": items})
				}
				return printJSON(map[string]any{"users": users})
			}
			for _, id := range ids {
				printFreebusyHeader(ids, id)
				if len(raw[id]) == 0 {
					fmt.Println("该时间段内无日程")
					continue
				}
				for i, it := range raw[id] {
					fmt.Printf("[%d] %s ~ %s  %s\n", i+1, it.StartTime, it.EndTime, it.RSVPStatus)
				}
			}
			return nil

		case "free":
			free := make(map[string][]*client.FreeSlot, len(ids))
			for _, id := range ids {
				free[id] = client.ComputeFreeSlots([][]*client.FreebusyInfo{merged[id]}, winStart, winEnd, minDur)
			}
			if output == "json" {
				users := make([]map[string]any, 0, len(ids))
				for _, id := range ids {
					users = append(users, map[string]any{"user_id": id, "free": free[id]})
				}
				return printJSON(map[string]any{"users": users})
			}
			for _, id := range ids {
				printFreebusyHeader(ids, id)
				printFreeSlots(free[id])
			}
			return nil

		default: // common_free
			all := make([][]*client.FreebusyInfo, 0, len(ids))
			for _, id := range ids {
				all = append(all, merged[id])
			}
			slots := client.ComputeFreeSlots(all, winStart, winEnd, minDur)
			if output == "json" {
				return printJSON(map[string]any{"user_ids": ids, "common_free": slots})
			}
			fmt.Printf("%d 人的共同空闲时段:\n", len(ids))
			printFreeSlots(slots)
			return nil
		}
	},
}

func printFreebusyHeader(ids []string, id string) {
	if len(ids) > 1 {
		fmt.Printf("\n用户 %s\n", id)
	}
}

func printFreeSlots(slots []*client.FreeSlot) {
	if len(slots) == 0 {
		fmt.Println("该时间段内无空闲时段")
		return
	}
	for i, s := range slots {
		fmt.Printf("[%d] %s ~ %s（%s）\n", i+1, s.StartTime, s.EndTime, s.Duration)
	}
}

// parseFreebusyWindow 解析 --start/--end：缺省为今天整天；日期输入的结束端取当天 23:59:59
func parseFreebusyWindow(startInput, endInput string, now time.Time) (time.Time, time.Time, error) {
	startInput, endInput = strings.TrimSpace(startInput), strings.TrimSpace(endInput)
	var start, end time.Time
	var err error
	if startInput == "" {
		y, m, d := now.Date()
		start = time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	} else if start, err = client.ParseTimeInput(startInput, false); err != nil {
		return start, end, clierr.Usagef("--start: %v", err)
	}
	if endInput == "" {
		s := start.In(now.Location())
		end = time.Date(s.Year(), s.Month(), s.Day(), 23, 59, 59, 0, s.Location())
	} else if end, err = client.ParseTimeInput(endInput, true); err != nil {
		return start, end, clierr.Usagef("--end: %v", err)
	}
	if !end.After(start) {
		return start, end, clierr.Usagef("--end 必须晚于 --start")
	}
	return start, end, nil
}

// dedupeNonEmpty 去空白、去空串、去重并保持顺序
func dedupeNonEmpty(items []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(items))
	for _, raw := range items {
		for _, v := range strings.Split(raw, ",") {
			v = strings.TrimSpace(v)
			if v == "" || seen[v] {
				continue
			}
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func init() {
	calendarCmd.AddCommand(calendarFreebusyCmd)
	calendarFreebusyCmd.Flags().String("start", "", "起始时间，RFC3339 / YYYY-MM-DD / Unix 秒（默认今天 00:00）")
	calendarFreebusyCmd.Flags().String("end", "", "结束时间（默认起始日当天 23:59:59）")
	calendarFreebusyCmd.Flags().StringSlice("user-id", nil, "用户 open_id，可重复或逗号分隔（默认当前登录用户；Bot 身份必填）")
	calendarFreebusyCmd.Flags().String("type", "busy", "输出视图：busy | raw_busy | free | common_free")
	calendarFreebusyCmd.Flags().String("min-duration", "", "free/common_free 的最短时长（如 30m、1h）")
	calendarFreebusyCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	calendarFreebusyCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}
