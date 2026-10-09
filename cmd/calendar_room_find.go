package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

const roomFindWorkers = 10

var calendarRoomFindCmd = &cobra.Command{
	Use:   "room-find",
	Short: "查找指定时段可用的会议室",
	Long: `调用飞书 freebusy/room_find 接口，按城市/建筑/楼层/容量等约束，
为一个或多个时段并发查询可用会议室候选。AI Agent 排会议时选会议室常用。

参数:
  --slot               待查时段，格式 start~end（RFC3339），可重复传入或逗号分隔多段
  --attendee-ids       参与者 ID 列表，逗号分隔（ou_xxx / oc_xxx），可省略
  --city               城市约束（例如 "北京"）
  --building           建筑约束
  --floor              楼层约束（例如 "F2"）
  --room-name          会议室名称约束，逗号分隔可指定多个（例如 "01,02,03"）
  --min-capacity       最小容量
  --max-capacity       最大容量
  --timezone           时区（例：Asia/Shanghai）
  --event-rrule        周期性规则（rrule 字符串）

权限:
  calendar:calendar.free_busy:read（User Token 或 App Token 均可）

示例:
  # 查 2024-01-22 9-10 点的可用会议室
  feishu-cli calendar room-find \
    --slot 2024-01-22T09:00:00+08:00~2024-01-22T10:00:00+08:00

  # 多个时段 + 容量/建筑约束
  feishu-cli calendar room-find \
    --slot 2024-01-22T09:00:00+08:00~2024-01-22T10:00:00+08:00 \
    --slot 2024-01-22T14:00:00+08:00~2024-01-22T15:00:00+08:00 \
    --building "飞书大厦" --min-capacity 6 --max-capacity 20

  # 配合参与者，便于服务端筛选离参与者近的会议室
  feishu-cli calendar room-find \
    --slot 2024-01-22T09:00:00+08:00~2024-01-22T10:00:00+08:00 \
    --attendee-ids ou_aaa,ou_bbb -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserTokenWithFallback(cmd)

		// 解析 --slot 多值（StringSliceArray 支持重复传入 + 逗号分隔）
		slotInputs, _ := cmd.Flags().GetStringSlice("slot")
		slots, err := parseRoomFindSlots(slotInputs)
		if err != nil {
			return err
		}

		// 解析参与者
		attendeeIDs, _ := cmd.Flags().GetString("attendee-ids")
		userIDs, chatIDs, err := client.SplitAttendeeIDs(attendeeIDs)
		if err != nil {
			return err
		}

		city, _ := cmd.Flags().GetString("city")
		building, _ := cmd.Flags().GetString("building")
		floor, _ := cmd.Flags().GetString("floor")
		roomName, _ := cmd.Flags().GetString("room-name")
		minCap, _ := cmd.Flags().GetInt("min-capacity")
		maxCap, _ := cmd.Flags().GetInt("max-capacity")
		timezone, _ := cmd.Flags().GetString("timezone")
		eventRrule, _ := cmd.Flags().GetString("event-rrule")
		output, _ := cmd.Flags().GetString("output")

		if minCap < 0 || maxCap < 0 {
			return fmt.Errorf("--min-capacity 和 --max-capacity 必须 >= 0")
		}
		if minCap > 0 && maxCap > 0 && minCap > maxCap {
			return fmt.Errorf("--min-capacity 必须 <= --max-capacity")
		}

		baseReq := &client.RoomFindRequest{
			City:            strings.TrimSpace(city),
			Building:        strings.TrimSpace(building),
			Floor:           strings.TrimSpace(floor),
			RoomName:        normalizeCommaSeparated(roomName),
			MinCapacity:     minCap,
			MaxCapacity:     maxCap,
			AttendeeUserIDs: userIDs,
			AttendeeChatIDs: chatIDs,
			Timezone:        strings.TrimSpace(timezone),
			EventRrule:      strings.TrimSpace(eventRrule),
		}

		result, err := client.FindMeetingRoomBatch(baseReq, slots, roomFindWorkers, token)
		if err != nil {
			return err
		}

		if output == "json" {
			return printJSON(result)
		}

		if result == nil || len(result.TimeSlots) == 0 {
			fmt.Println("未找到可用会议室")
			return nil
		}

		for _, slot := range result.TimeSlots {
			fmt.Printf("%s ~ %s\n", slot.Start, slot.End)
			if len(slot.MeetingRooms) == 0 {
				fmt.Println("  （无可用会议室）")
				fmt.Println()
				continue
			}
			for i, room := range slot.MeetingRooms {
				fmt.Printf("  [%d] %s (id=%s, capacity=%d)\n", i+1, room.RoomName, room.RoomID, room.Capacity)
				if room.ReserveUntilTime != "" {
					fmt.Printf("      可预订至: %s\n", room.ReserveUntilTime)
				}
			}
			fmt.Println()
		}

		return nil
	},
}

// parseRoomFindSlots 解析 --slot 列表（每项 start~end，RFC3339）
func parseRoomFindSlots(raws []string) ([]client.RoomFindSlot, error) {
	if len(raws) == 0 {
		return nil, fmt.Errorf("至少需要指定一个 --slot")
	}
	var out []client.RoomFindSlot
	for _, raw := range raws {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parts := strings.Split(raw, "~")
		if len(parts) != 2 {
			return nil, fmt.Errorf("无效的 --slot 格式 %q，应为 start~end", raw)
		}
		startT, err := time.Parse(time.RFC3339, strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, fmt.Errorf("--slot 起始时间 %q 解析失败: %w（RFC3339 格式）", parts[0], err)
		}
		endT, err := time.Parse(time.RFC3339, strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("--slot 结束时间 %q 解析失败: %w（RFC3339 格式）", parts[1], err)
		}
		if !endT.After(startT) {
			return nil, fmt.Errorf("--slot %q 的结束时间必须晚于起始时间", raw)
		}
		out = append(out, client.RoomFindSlot{
			Start: startT.Format(time.RFC3339),
			End:   endT.Format(time.RFC3339),
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("至少需要指定一个 --slot")
	}
	return out, nil
}

// normalizeCommaSeparated 去除每段前后空格，过滤空段
func normalizeCommaSeparated(raw string) string {
	parts := strings.Split(raw, ",")
	var cleaned []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			cleaned = append(cleaned, p)
		}
	}
	return strings.Join(cleaned, ",")
}

func init() {
	calendarCmd.AddCommand(calendarRoomFindCmd)
	calendarRoomFindCmd.Flags().StringSlice("slot", nil, "待查时段 start~end（RFC3339），可重复传入或逗号分隔多段（必填）")
	calendarRoomFindCmd.Flags().String("attendee-ids", "", "参与者 ID 列表，逗号分隔（ou_xxx / oc_xxx，可省略）")
	calendarRoomFindCmd.Flags().String("city", "", "城市约束")
	calendarRoomFindCmd.Flags().String("building", "", "建筑约束")
	calendarRoomFindCmd.Flags().String("floor", "", "楼层约束（例：F2）")
	calendarRoomFindCmd.Flags().String("room-name", "", "会议室名称约束（多个用逗号分隔）")
	calendarRoomFindCmd.Flags().Int("min-capacity", 0, "最小容量")
	calendarRoomFindCmd.Flags().Int("max-capacity", 0, "最大容量")
	calendarRoomFindCmd.Flags().String("timezone", "", "时区（例：Asia/Shanghai）")
	calendarRoomFindCmd.Flags().String("event-rrule", "", "周期性规则 rrule 字符串")
	calendarRoomFindCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	calendarRoomFindCmd.Flags().String("user-access-token", "", "User Access Token（可选，默认 App Token）")

	mustMarkFlagRequired(calendarRoomFindCmd, "slot")
}
