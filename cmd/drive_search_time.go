package cmd

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

// drive search 时间窗过滤（对齐官方 shortcuts/drive/drive_search.go 的 *-since / *-until）。
//
//   - --edited-* → my_edit_time、--commented-* → my_comment_time：服务端按小时聚合，非整点输入向外取整并在 stderr 提示；
//   - --opened-* → open_time：服务端单次最多 3 个月。窗口超过 90 天时本次只查最近的一个 90 天切片，
//     stderr 列出全部切片的绝对时间；超过 365 天直接报用法错误（exit 2）；
//   - --created-* → create_time。
//
// 时间值接受相对时间（7d / 1m=30 天 / 1y=365 天）、YYYY-MM-DD[ HH:MM:SS]、RFC3339、unix 秒（13 位视为毫秒）。

const (
	driveSearchSliceDays         = 90
	driveSearchMaxOpenedSpanDays = 365
)

// driveSearchTimeDims 维度顺序固定：flag 前缀 → filter key。
var driveSearchTimeDims = []struct{ flag, key string }{
	{"edited", "my_edit_time"},
	{"commented", "my_comment_time"},
	{"opened", "open_time"},
	{"created", "create_time"},
}

var driveSearchHourAggregated = map[string]bool{"my_edit_time": true, "my_comment_time": true}

var driveSearchRelativeRe = regexp.MustCompile(`^(\d+)([dmy])$`)

// parseDriveSearchTime 解析单个时间值为 unix 秒。
func parseDriveSearchTime(input string, now time.Time) (int64, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return 0, fmt.Errorf("值为空")
	}
	if m := driveSearchRelativeRe.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		days := n
		switch m[2] {
		case "m":
			days = n * 30
		case "y":
			days = n * 365
		}
		return now.Add(-time.Duration(days) * 24 * time.Hour).Unix(), nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t.Unix(), nil
		}
	}
	// 纯数字至少 10 位才当 unix 秒，避免把 20260423 误判；13 位（>=1e12）视为毫秒
	if len(s) >= 10 {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			if n >= 1e12 {
				n /= 1000
			}
			return n, nil
		}
	}
	return 0, fmt.Errorf("应为相对时间（7d/1m/1y）、日期（YYYY-MM-DD[ HH:MM:SS]）、RFC3339 或 unix 秒")
}

// driveSearchTimeInput 是某一维度的原始输入：since/until 为空表示未传。
type driveSearchTimeInput struct{ Since, Until string }

// buildDriveSearchTimeRanges 解析四组时间窗，返回 filter key → {start,end} 与需要打印到 stderr 的提示。
// inputs 的 key 为 flag 前缀（edited/commented/opened/created）。
func buildDriveSearchTimeRanges(inputs map[string]driveSearchTimeInput, now time.Time) (map[string]map[string]int64, []string, error) {
	ranges := map[string]map[string]int64{}
	var notices []string
	for _, d := range driveSearchTimeDims {
		in := inputs[d.flag]
		if in.Since == "" && in.Until == "" {
			continue
		}
		rng := map[string]int64{}
		if in.Since != "" {
			v, err := parseDriveSearchTime(in.Since, now)
			if err != nil {
				return nil, nil, clierr.Usagef("--%s-since %q 无效: %v", d.flag, in.Since, err)
			}
			rng["start"] = v
		}
		if in.Until != "" {
			v, err := parseDriveSearchTime(in.Until, now)
			if err != nil {
				return nil, nil, clierr.Usagef("--%s-until %q 无效: %v", d.flag, in.Until, err)
			}
			rng["end"] = v
		}
		if start, ok := rng["start"]; ok {
			if end, ok := rng["end"]; ok && end <= start {
				return nil, nil, clierr.Usagef("--%s-since 必须早于 --%s-until", d.flag, d.flag)
			}
		}
		if d.key == "open_time" {
			notice, err := clampDriveSearchOpenedWindow(rng, now)
			if err != nil {
				return nil, nil, err
			}
			if notice != "" {
				notices = append(notices, notice)
			}
		}
		if driveSearchHourAggregated[d.key] {
			if v, ok := rng["start"]; ok && v%3600 != 0 {
				snapped := v - v%3600
				notices = append(notices, driveSearchHourSnapNotice(d.flag, "since", v, snapped))
				rng["start"] = snapped
			}
			if v, ok := rng["end"]; ok && v%3600 != 0 {
				snapped := v - v%3600 + 3600
				notices = append(notices, driveSearchHourSnapNotice(d.flag, "until", v, snapped))
				rng["end"] = snapped
			}
		}
		ranges[d.key] = rng
	}
	if len(ranges) == 0 {
		return nil, notices, nil
	}
	return ranges, notices, nil
}

func driveSearchHourSnapNotice(flag, side string, before, after int64) string {
	return fmt.Sprintf("提示：--%s-%s 服务端按小时聚合，已从 %s 取整为 %s", flag, side,
		time.Unix(before, 0).Format("2006-01-02 15:04:05"), time.Unix(after, 0).Format("2006-01-02 15:04:05"))
}

// clampDriveSearchOpenedWindow 处理 open_time 的 3 个月上限：窗口 ≤90 天不处理；(90, 365] 天把 rng 收窄到
// 最近一个 90 天切片并返回列出全部切片的提示；>365 天报用法错误。只传 --opened-until 时不处理。
func clampDriveSearchOpenedWindow(rng map[string]int64, now time.Time) (string, error) {
	since, ok := rng["start"]
	if !ok {
		return "", nil
	}
	until, ok := rng["end"]
	if !ok {
		until = now.Unix()
	}
	span := until - since
	slice := int64(driveSearchSliceDays) * 86400
	if span <= slice {
		return "", nil
	}
	if span > int64(driveSearchMaxOpenedSpanDays)*86400 {
		return "", clierr.Usagef("--opened-* 时间窗跨度 %d 天，超过 %d 天（1 年）上限；请缩小范围或分多次查询", span/86400, driveSearchMaxOpenedSpanDays)
	}
	type win struct{ start, end int64 }
	var slices []win
	for cursor := until; cursor > since; {
		start := cursor - slice
		if start < since {
			start = since
		}
		slices = append(slices, win{start, cursor})
		cursor = start
	}
	fmtT := func(v int64) string { return time.Unix(v, 0).Format(time.RFC3339) }
	var b strings.Builder
	fmt.Fprintf(&b, "提示：--opened-* 时间窗跨度 %d 天，超过服务端单次 3 个月（%d 天）上限，本次只查询最近的切片；共 %d 个切片:\n",
		span/86400, driveSearchSliceDays, len(slices))
	for i, w := range slices {
		label := fmt.Sprintf("[切片 %d/%d]", i+1, len(slices))
		if i == 0 {
			label += "（本次）"
		}
		fmt.Fprintf(&b, "  %s --opened-since %s --opened-until %s\n", label, fmtT(w.start), fmtT(w.end))
	}
	b.WriteString("翻页：在同一切片内用 --page-token 并原样使用该切片的 --opened-since/--opened-until 绝对时间（不要再用 1y 这类相对时间，会随当前时间漂移）；has_more=false 后再换下一个切片，page-token 不能跨切片使用")
	rng["start"], rng["end"] = slices[0].start, slices[0].end
	return b.String(), nil
}
