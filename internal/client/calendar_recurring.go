package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ============================================================================
// 重复日程（主日程 / 实例 / 例外）的分类、范围校验与批量处理。
//
// 语义对齐官方 lark-calendar-recurring.md，并经自建重复日程实测：
//   - event_id 形如 {uid}_{originalTime}：originalTime=0 是主日程（带 recurrence）或普通日程，
//     >0 是某一次实例；实例被单独修改/删除后落库成为例外（is_exception=true，ID 形状不变）。
//   - DELETE 主日程 ID：删除整条序列，但**不级联**已单独修改过的例外（例外仍在日历上）。
//   - DELETE 实例 ID：只删这一次（服务端把它物化为 status=cancelled 的例外）。
//   - PATCH 主日程：作用于整条序列，但不会改到已存在的例外；PATCH 实例 ID：只改这一次。
// ============================================================================

// RecurringKind 日程在重复序列中的角色
type RecurringKind string

const (
	RecurringKindNormal    RecurringKind = "normal"    // 普通（非重复）日程
	RecurringKindMaster    RecurringKind = "master"    // 重复日程主体（{uid}_0 且有 recurrence）
	RecurringKindException RecurringKind = "exception" // 已单独修改/删除过的实例
	RecurringKindInstance  RecurringKind = "instance"  // 未单独修改过的实例（{uid}_{ts>0}）
)

// --apply-to 取值
const (
	ApplyToSingle           = "single"
	ApplyToAll              = "all"
	ApplyToThisAndFollowing = "this-and-following"
)

// ApplyToValues 有序的 --apply-to 合法值
var ApplyToValues = []string{ApplyToSingle, ApplyToAll, ApplyToThisAndFollowing}

// 例外数量上限：防止异常序列把成千上万条例外一次性载入内存
const recurringExceptionMaxCount = 5000

// ParseInstanceOriginalTime 从 {uid}_{originalTime} 中取出原始时间戳（秒）。
// 后缀为 0、非数字或缺失时返回 (0,false)。
func ParseInstanceOriginalTime(eventID string) (int64, bool) {
	idx := strings.LastIndex(eventID, "_")
	if idx <= 0 || idx == len(eventID)-1 {
		return 0, false
	}
	n, err := strconv.ParseInt(eventID[idx+1:], 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// HasStandardEventIDShape 报告 event_id 是否形如 {uid}_{digits}
func HasStandardEventIDShape(eventID string) bool {
	idx := strings.LastIndex(eventID, "_")
	if idx <= 0 || idx == len(eventID)-1 {
		return false
	}
	_, err := strconv.ParseInt(eventID[idx+1:], 10, 64)
	return err == nil
}

// MasterEventID 把实例/例外 ID 改写为主日程 ID（{uid}_0）
func MasterEventID(eventID string) string {
	idx := strings.LastIndex(eventID, "_")
	if idx <= 0 {
		return eventID
	}
	return eventID[:idx] + "_0"
}

// ClassifyEvent 根据 GET 结果判断日程在重复序列中的角色
func ClassifyEvent(ev *CalendarEvent) RecurringKind {
	if ev == nil {
		return RecurringKindNormal
	}
	if ev.IsException {
		return RecurringKindException
	}
	if strings.TrimSpace(ev.Recurrence) != "" {
		return RecurringKindMaster
	}
	if _, ok := ParseInstanceOriginalTime(ev.EventID); ok {
		return RecurringKindInstance
	}
	if ev.RecurringID != "" && ev.RecurringID != ev.EventID {
		return RecurringKindInstance
	}
	return RecurringKindNormal
}

// RecurringKindLabel 中文描述
func RecurringKindLabel(k RecurringKind) string {
	switch k {
	case RecurringKindMaster:
		return "重复日程主体"
	case RecurringKindException:
		return "重复日程例外（单独修改过的某一次）"
	case RecurringKindInstance:
		return "重复日程的某一次实例"
	default:
		return "普通日程"
	}
}

// ValidateApplyTo 校验 --apply-to 与日程类型是否匹配，返回生效范围。
//
// requested 为空表示调用方未指定：保持服务端原生语义（兼容旧行为），返回 ""，由调用方说明影响范围。
//   - 普通日程：只接受 single（或不传）
//   - 主日程：只接受 all（single / this-and-following 需要传具体实例 ID）
//   - 实例：single / all / this-and-following 均可
//   - 例外：single / all（例外不能作为截断点）
func ValidateApplyTo(kind RecurringKind, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		if kind == RecurringKindNormal {
			return ApplyToSingle, nil
		}
		return "", nil
	}
	valid := false
	for _, v := range ApplyToValues {
		if v == requested {
			valid = true
		}
	}
	if !valid {
		return "", fmt.Errorf("--apply-to 仅支持 %s，得到 %q", strings.Join(ApplyToValues, " | "), requested)
	}
	switch kind {
	case RecurringKindNormal:
		if requested != ApplyToSingle {
			return "", fmt.Errorf("--apply-to=%s 不适用于普通（非重复）日程；去掉 --apply-to 或改为 single", requested)
		}
	case RecurringKindMaster:
		if requested != ApplyToAll {
			return "", fmt.Errorf("--apply-to=%s 不适用于重复日程主体（{uid}_0）；主体只接受 all。"+
				"要操作某一次或从某一次起截断，请传该次实例的 event_id（形如 {uid}_{原始时间戳}，可从 calendar agenda / event-search 获取）", requested)
		}
	case RecurringKindException:
		if requested == ApplyToThisAndFollowing {
			return "", fmt.Errorf("--apply-to=this-and-following 不适用于例外日程（已单独修改过的那一次）；" +
				"可用 single 或 all，或改传一个未修改过的实例 event_id 作为截断点")
		}
	}
	return requested, nil
}

// TruncateRecurrenceUntil 把 RRULE 的 UNTIL 改写为 until（UTC，YYYYMMDDThhmmssZ）；
// 没有 UNTIL 则追加；COUNT 与 UNTIL 互斥，一并删除。保留可能的 "RRULE:" 前缀。
func TruncateRecurrenceUntil(rrule string, until time.Time) string {
	body := strings.TrimSpace(rrule)
	prefix := ""
	if strings.HasPrefix(body, "RRULE:") {
		prefix = "RRULE:"
		body = strings.TrimPrefix(body, "RRULE:")
	}
	newUntil := "UNTIL=" + until.UTC().Format("20060102T150405Z")
	var kept []string
	replaced := false
	for _, part := range strings.Split(body, ";") {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		up := strings.ToUpper(p)
		if strings.HasPrefix(up, "UNTIL=") {
			if !replaced {
				kept = append(kept, newUntil)
				replaced = true
			}
			continue
		}
		if strings.HasPrefix(up, "COUNT=") {
			continue
		}
		kept = append(kept, p)
	}
	if !replaced {
		kept = append(kept, newUntil)
	}
	return prefix + strings.Join(kept, ";")
}

// InheritRRuleForFollowing 新序列继承原 RRULE：保留 UNTIL（绝对截止），去掉 COUNT
// （COUNT 以原序列首次为锚点，挪到新起点会算错次数）。
func InheritRRuleForFollowing(rrule string) string {
	body := strings.TrimSpace(rrule)
	prefix := ""
	if strings.HasPrefix(body, "RRULE:") {
		prefix = "RRULE:"
		body = strings.TrimPrefix(body, "RRULE:")
	}
	var kept []string
	for _, part := range strings.Split(body, ";") {
		p := strings.TrimSpace(part)
		if p == "" || strings.HasPrefix(strings.ToUpper(p), "COUNT=") {
			continue
		}
		kept = append(kept, p)
	}
	if len(kept) == 0 {
		return ""
	}
	return prefix + strings.Join(kept, ";")
}

// eventLocation 返回日程声明的时区（start 优先），缺失/无法识别时用本地时区
func eventLocation(ev *CalendarEvent) *time.Location {
	tz := ""
	if ev != nil {
		if ev.rawStart != nil {
			tz = StringVal(ev.rawStart.Timezone)
		}
		if tz == "" && ev.rawEnd != nil {
			tz = StringVal(ev.rawEnd.Timezone)
		}
	}
	if tz == "" {
		return time.Local
	}
	if loc, err := time.LoadLocation(tz); err == nil {
		return loc
	}
	return time.Local
}

// PivotDayCutoff 返回截断点所在日（按日程时区）零点前 1 秒，用作 UNTIL：
// 原序列在截断日之前结束，截断日及之后的实例全部移出。
func PivotDayCutoff(pivotUnix int64, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	t := time.Unix(pivotUnix, 0).In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc).Add(-time.Second)
}

// masterStartUnix 主日程开始日零点（日程时区）；全天日程按 UTC 日期
func masterStartUnix(master *CalendarEvent) (int64, error) {
	if master == nil || master.rawStart == nil {
		return 0, fmt.Errorf("重复日程主体缺少开始时间")
	}
	loc := eventLocation(master)
	if ts := StringVal(master.rawStart.Timestamp); ts != "" {
		n, err := strconv.ParseInt(ts, 10, 64)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("重复日程主体开始时间无效: %q", ts)
		}
		t := time.Unix(n, 0).In(loc)
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc).Unix(), nil
	}
	if date := StringVal(master.rawStart.Date); date != "" {
		t, err := time.ParseInLocation("2006-01-02", date, time.UTC)
		if err != nil {
			return 0, fmt.Errorf("重复日程主体开始日期无效: %q", date)
		}
		return t.Unix(), nil
	}
	return 0, fmt.Errorf("重复日程主体开始时间既无 timestamp 也无 date")
}

const recurringDefaultHorizon = 5 * 365 * 24 * time.Hour

// recurringEndFromRRule 估算序列最后一次的结束上界：UNTIL（+1 天余量）> COUNT×周期 > 现在+5 年
func recurringEndFromRRule(rrule string, startSec int64, now time.Time) int64 {
	upper := now.Add(recurringDefaultHorizon).Unix()
	rule := strings.TrimPrefix(strings.TrimSpace(rrule), "RRULE:")
	if rule == "" {
		return upper
	}
	kv := map[string]string{}
	for _, p := range strings.Split(rule, ";") {
		if eq := strings.IndexByte(p, '='); eq > 0 {
			kv[strings.ToUpper(strings.TrimSpace(p[:eq]))] = strings.TrimSpace(p[eq+1:])
		}
	}
	if until, ok := kv["UNTIL"]; ok {
		for _, layout := range []string{"20060102T150405Z", "20060102"} {
			if t, err := time.ParseInLocation(layout, until, time.UTC); err == nil {
				return t.Add(24 * time.Hour).Unix()
			}
		}
	}
	if countStr, ok := kv["COUNT"]; ok {
		if n, err := strconv.Atoi(countStr); err == nil && n > 0 {
			interval := 1
			if v, err := strconv.Atoi(kv["INTERVAL"]); err == nil && v > 0 {
				interval = v
			}
			var step time.Duration
			switch strings.ToUpper(kv["FREQ"]) {
			case "DAILY":
				step = 24 * time.Hour
			case "WEEKLY":
				step = 7 * 24 * time.Hour
			case "MONTHLY":
				step = 31 * 24 * time.Hour
			case "YEARLY":
				step = 366 * 24 * time.Hour
			}
			if step > 0 {
				h := time.Unix(startSec, 0).Add(step * time.Duration(interval) * time.Duration(n+1)).Unix()
				if h < upper {
					return h
				}
			}
		}
	}
	return upper
}

// RecurringException 例外日程（event_id + 实例开始时间）
type RecurringException struct {
	EventID   string `json:"event_id"`
	StartUnix int64  `json:"-"`
	Status    string `json:"status,omitempty"`
}

// recurringInstanceItem /instances 返回的单个实例
type recurringInstanceItem struct {
	EventID     string `json:"event_id"`
	IsException bool   `json:"is_exception"`
	Status      string `json:"status"`
	StartTime   *struct {
		Timestamp string `json:"timestamp"`
		Date      string `json:"date"`
	} `json:"start_time"`
}

func (it *recurringInstanceItem) startUnix() int64 {
	var sec int64
	if it.StartTime != nil {
		if it.StartTime.Timestamp != "" {
			sec, _ = strconv.ParseInt(it.StartTime.Timestamp, 10, 64)
		} else if it.StartTime.Date != "" {
			if t, err := time.ParseInLocation("2006-01-02", it.StartTime.Date, time.UTC); err == nil {
				sec = t.Unix()
			}
		}
	}
	if sec == 0 {
		sec, _ = ParseInstanceOriginalTime(it.EventID)
	}
	return sec
}

// scanRecurringInstances 分段（≤1 年，接口上限 2 年）分页扫描主日程的 /instances，逐条回调
func scanRecurringInstances(calendarID, masterID string, start, end int64, userAccessToken string, visit func(*recurringInstanceItem) error) error {
	if end <= start {
		return nil
	}
	cli, err := GetClient()
	if err != nil {
		return err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	const oneYear int64 = 365 * 24 * 60 * 60
	for chunkStart := start; chunkStart < end; {
		chunkEnd := chunkStart + oneYear
		if chunkEnd > end {
			chunkEnd = end
		}
		pageToken := ""
		for page := 0; ; page++ {
			q := url.Values{}
			q.Set("start_time", strconv.FormatInt(chunkStart, 10))
			q.Set("end_time", strconv.FormatInt(chunkEnd, 10))
			q.Set("page_size", "500")
			if pageToken != "" {
				q.Set("page_token", pageToken)
			}
			var body []byte
			err := withCalendarRateLimitRetry(func() error {
				resp, err := cli.Get(Context(), calendarEventPath(calendarID, masterID)+"/instances?"+q.Encode(), nil, tokenType, opts...)
				if err != nil {
					return fmt.Errorf("查询重复日程实例失败: %w", err)
				}
				if err := CheckAPIResponse("查询重复日程实例", resp); err != nil {
					return err
				}
				body = resp.RawBody
				return nil
			})
			if err != nil {
				return err
			}
			var apiResp struct {
				Data struct {
					Items     []*recurringInstanceItem `json:"items"`
					HasMore   bool                     `json:"has_more"`
					PageToken string                   `json:"page_token"`
				} `json:"data"`
			}
			if err := json.Unmarshal(body, &apiResp); err != nil {
				return fmt.Errorf("解析重复日程实例失败: %w", err)
			}
			for _, item := range apiResp.Data.Items {
				if item == nil || item.EventID == "" {
					continue
				}
				if err := visit(item); err != nil {
					return err
				}
			}
			if !apiResp.Data.HasMore || apiResp.Data.PageToken == "" || apiResp.Data.PageToken == pageToken || page >= 200 {
				break
			}
			pageToken = apiResp.Data.PageToken
		}
		chunkStart = chunkEnd + 1
	}
	return nil
}

// ListRecurringExceptions 扫描主日程的 /instances，返回窗口内的例外日程（按开始时间升序）。
// includeCancelled 为 true 时连同 cancelled 占位一起返回（全量清理场景要把占位一并销毁）。按 event_id 去重。
func ListRecurringExceptions(calendarID, masterID string, start, end int64, includeCancelled bool, userAccessToken string) ([]*RecurringException, error) {
	seen := map[string]bool{}
	var out []*RecurringException
	err := scanRecurringInstances(calendarID, masterID, start, end, userAccessToken, func(item *recurringInstanceItem) error {
		if !item.IsException || seen[item.EventID] {
			return nil
		}
		if item.Status == "cancelled" && !includeCancelled {
			return nil
		}
		seen[item.EventID] = true
		out = append(out, &RecurringException{EventID: item.EventID, StartUnix: item.startUnix(), Status: item.Status})
		if len(out) > recurringExceptionMaxCount {
			return fmt.Errorf("该重复日程的例外超过 %d 个，拒绝一次性全部处理；请缩小范围或分批清理", recurringExceptionMaxCount)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartUnix < out[j].StartUnix })
	return out, nil
}

// lastOccurrenceStart 返回序列（按原规则展开）最后一次实例的开始时间；无实例返回 0
func lastOccurrenceStart(calendarID string, master *CalendarEvent, userAccessToken string) (int64, error) {
	start, err := masterStartUnix(master)
	if err != nil {
		return 0, err
	}
	end := recurringEndFromRRule(master.Recurrence, start, time.Now())
	var last int64
	err = scanRecurringInstances(calendarID, master.EventID, start, end, userAccessToken, func(item *recurringInstanceItem) error {
		if ot, ok := ParseInstanceOriginalTime(item.EventID); ok && ot > last {
			last = ot
		}
		return nil
	})
	return last, err
}

// followingSeriesRRule 计算新序列的重复规则：原规则带 COUNT 时换算成原序列最后一次的 UNTIL，
// 保证新旧两段合起来的次数与原来一致（官方直接丢弃 COUNT，会让新序列变成无限重复）。
func followingSeriesRRule(calendarID string, master *CalendarEvent, userAccessToken string) (string, error) {
	rule := InheritRRuleForFollowing(master.Recurrence)
	if !strings.Contains(strings.ToUpper(master.Recurrence), "COUNT=") {
		return rule, nil
	}
	last, err := lastOccurrenceStart(calendarID, master, userAccessToken)
	if err != nil {
		return "", err
	}
	if last == 0 {
		return rule, nil
	}
	return TruncateRecurrenceUntil(rule, time.Unix(last, 0)), nil
}

// RecurringBatchFailure 批量处理例外时的单条失败
type RecurringBatchFailure struct {
	EventID string `json:"event_id"`
	Error   string `json:"error"`
}

// RecurringBatchSummary 例外批量处理汇总
type RecurringBatchSummary struct {
	Total     int                     `json:"total"`
	Succeeded int                     `json:"succeeded"`
	Failed    int                     `json:"failed"`
	Failures  []RecurringBatchFailure `json:"failures,omitempty"`
}

// runExceptionBatch 逐个处理例外：单条失败记录后继续，不中断整批（失败可凭 ID 重试）
func runExceptionBatch(items []*RecurringException, do func(ex *RecurringException) error) *RecurringBatchSummary {
	sum := &RecurringBatchSummary{Total: len(items)}
	for _, ex := range items {
		if err := do(ex); err != nil {
			sum.Failed++
			sum.Failures = append(sum.Failures, RecurringBatchFailure{EventID: ex.EventID, Error: err.Error()})
			continue
		}
		sum.Succeeded++
	}
	return sum
}

// ensureRecurringMaster 返回主日程：current 本身是主日程时直接复用，否则按 {uid}_0 读取
func ensureRecurringMaster(calendarID string, current *CalendarEvent, eventID, userAccessToken string) (*CalendarEvent, error) {
	if current != nil && strings.TrimSpace(current.Recurrence) != "" && !current.IsException {
		return current, nil
	}
	masterID := MasterEventID(eventID)
	if current != nil && current.RecurringID != "" {
		masterID = current.RecurringID
	}
	master, err := GetEvent(calendarID, masterID, userAccessToken)
	if err != nil {
		return nil, fmt.Errorf("读取重复日程主体 %s 失败: %w", masterID, err)
	}
	if strings.TrimSpace(master.Recurrence) == "" {
		return nil, fmt.Errorf("日程 %s 没有重复规则，--apply-to all / this-and-following 只适用于重复日程", masterID)
	}
	if master.EventID == "" {
		master.EventID = masterID
	}
	return master, nil
}

// RecurringDeleteResult 删除重复日程的结果
type RecurringDeleteResult struct {
	CalendarID          string                 `json:"calendar_id"`
	EventID             string                 `json:"event_id"`
	Kind                RecurringKind          `json:"kind"`
	ApplyTo             string                 `json:"apply_to,omitempty"`
	Scope               string                 `json:"scope"`
	MasterEventID       string                 `json:"master_event_id,omitempty"`
	RecurrenceTruncated string                 `json:"recurrence_truncated,omitempty"`
	Exceptions          *RecurringBatchSummary `json:"exceptions,omitempty"`
}

// DeleteEventScoped 按 --apply-to 删除日程（current 为事先 GET 的结果，scope 已经 ValidateApplyTo 校验）。
//
//   - "" / single：直接删除传入的 ID（服务端原生语义）
//   - all：先销毁全部例外，再删除主日程（例外清理不通知参与人）
//   - this-and-following：销毁截断点及之后的例外，再把主日程 RRULE 截断到截断日前
func DeleteEventScoped(calendarID, eventID string, current *CalendarEvent, scope string, notify bool, progress func(string), userAccessToken string) (*RecurringDeleteResult, error) {
	if progress == nil {
		progress = func(string) {}
	}
	kind := ClassifyEvent(current)
	res := &RecurringDeleteResult{CalendarID: calendarID, EventID: eventID, Kind: kind, ApplyTo: scope}
	notifyPtr := &notify
	noNotify := false

	switch scope {
	case "", ApplyToSingle:
		if err := DeleteEventWithOptions(calendarID, eventID, DeleteEventOptions{NeedNotification: notifyPtr}, userAccessToken); err != nil {
			return nil, err
		}
		res.Scope = DescribeDeleteScope(kind, scope)
		return res, nil

	case ApplyToAll:
		master, err := ensureRecurringMaster(calendarID, current, eventID, userAccessToken)
		if err != nil {
			return nil, err
		}
		res.MasterEventID = master.EventID
		start, err := masterStartUnix(master)
		if err != nil {
			return nil, err
		}
		end := recurringEndFromRRule(master.Recurrence, start, time.Now())
		exceptions, err := ListRecurringExceptions(calendarID, master.EventID, start, end, true, userAccessToken)
		if err != nil {
			return nil, err
		}
		if len(exceptions) > 0 {
			progress(fmt.Sprintf("删除 %d 个例外日程后再删除重复日程主体", len(exceptions)))
			res.Exceptions = runExceptionBatch(exceptions, func(ex *RecurringException) error {
				return DeleteEventWithOptions(calendarID, ex.EventID, DeleteEventOptions{NeedNotification: &noNotify, DeleteException: true}, userAccessToken)
			})
		}
		if err := DeleteEventWithOptions(calendarID, master.EventID, DeleteEventOptions{NeedNotification: notifyPtr}, userAccessToken); err != nil {
			return res, fmt.Errorf("删除重复日程主体 %s 失败: %w", master.EventID, err)
		}
		res.Scope = DescribeDeleteScope(kind, scope)
		return res, nil

	case ApplyToThisAndFollowing:
		pivot, ok := ParseInstanceOriginalTime(eventID)
		if !ok {
			return nil, fmt.Errorf("--apply-to=this-and-following 需要实例 event_id（形如 {uid}_{原始时间戳}），得到 %q", eventID)
		}
		master, err := ensureRecurringMaster(calendarID, current, eventID, userAccessToken)
		if err != nil {
			return nil, err
		}
		res.MasterEventID = master.EventID
		loc := eventLocation(master)
		cutoff := PivotDayCutoff(pivot, loc)
		end := recurringEndFromRRule(master.Recurrence, cutoff.Unix(), time.Now())
		all, err := ListRecurringExceptions(calendarID, master.EventID, cutoff.Unix()+1, end, true, userAccessToken)
		if err != nil {
			return nil, err
		}
		var future []*RecurringException
		for _, ex := range all {
			if ex.StartUnix >= pivot {
				future = append(future, ex)
			}
		}
		if len(future) > 0 {
			progress(fmt.Sprintf("删除截断点及之后的 %d 个例外日程", len(future)))
			res.Exceptions = runExceptionBatch(future, func(ex *RecurringException) error {
				return DeleteEventWithOptions(calendarID, ex.EventID, DeleteEventOptions{NeedNotification: &noNotify, DeleteException: true}, userAccessToken)
			})
		}
		newRule := TruncateRecurrenceUntil(master.Recurrence, cutoff)
		progress(fmt.Sprintf("截断重复规则：%s", newRule))
		if _, err := UpdateEvent(&UpdateEventParams{
			CalendarID:       calendarID,
			EventID:          master.EventID,
			Recurrence:       newRule,
			NeedNotification: notifyPtr,
		}, userAccessToken); err != nil {
			return res, fmt.Errorf("截断重复日程主体 %s 失败（截断点之后的例外可能已删除）: %w", master.EventID, err)
		}
		res.RecurrenceTruncated = newRule
		res.Scope = DescribeDeleteScope(kind, scope)
		return res, nil
	}
	return nil, fmt.Errorf("不支持的 --apply-to: %q", scope)
}

// DescribeDeleteScope 用中文描述删除的实际影响范围（输出/提示用）
func DescribeDeleteScope(kind RecurringKind, scope string) string {
	switch scope {
	case ApplyToAll:
		return "整个重复序列（主体及全部例外）"
	case ApplyToThisAndFollowing:
		return "从该次起的后续所有实例（原序列截断到前一天，之后的例外一并删除）"
	case ApplyToSingle:
		if kind == RecurringKindNormal {
			return "该日程"
		}
		return "仅这一次"
	}
	switch kind {
	case RecurringKindMaster:
		return "整个重复序列（不含已单独修改过的例外；一并删除请加 --apply-to all）"
	case RecurringKindInstance, RecurringKindException:
		return "仅这一次（删除整个序列请加 --apply-to all）"
	}
	return "该日程"
}

// TransferEvent 转让日程组织者（POST /calendars/{calendar_id}/events/{event_id}/transfer）。
// 重复日程会转让整个序列（接口不支持只转让某一次）。
func TransferEvent(calendarID, eventID, toUserID string, removeOriginalOrganizer bool, userAccessToken string) error {
	cli, err := GetClient()
	if err != nil {
		return err
	}
	body := map[string]any{
		"to_user_id":                     toUserID,
		"need_remove_original_organizer": removeOriginalOrganizer,
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Post(Context(), calendarEventPath(calendarID, eventID)+"/transfer?user_id_type=open_id", body, tokenType, opts...)
	if err != nil {
		return fmt.Errorf("转让日程失败: %w", err)
	}
	if err := CheckAPIResponse("转让日程", resp); err != nil {
		if apiErr, ok := AsAPIError(err); ok {
			if hint := calendarTransferHint(apiErr.Code); hint != "" {
				return fmt.Errorf("%w\n提示：%s", err, hint)
			}
		}
		return err
	}
	return nil
}

func calendarTransferHint(code int) string {
	switch code {
	case 191002:
		return "当前身份对该日历没有编辑权限（需 WRITER/OWNER），这不是 scope 问题，重新登录无效；请用日程当前组织者的身份（--as）执行"
	case 191004:
		return "只有主日历或共享日历上的日程可以转让；会议室、邮箱、导入的外部日历不支持"
	case 193109:
		return "协作规则不允许邀请该接收人，请换一个接收人"
	case 193110:
		return "转让必须在组织者自己的日历上执行；日程在共享日历上时请显式传该日历的 calendar_id"
	case 193111:
		return "不支持跨租户转让，请改为把对方加为参与人"
	}
	return ""
}

// RecurringUpdateResult 更新重复日程的结果
type RecurringUpdateResult struct {
	CalendarID          string                 `json:"calendar_id"`
	EventID             string                 `json:"event_id"`
	Kind                RecurringKind          `json:"kind"`
	ApplyTo             string                 `json:"apply_to,omitempty"`
	Scope               string                 `json:"scope"`
	Event               *CalendarEvent         `json:"event,omitempty"`
	MasterEventID       string                 `json:"master_event_id,omitempty"`
	RecurrenceTruncated string                 `json:"recurrence_truncated,omitempty"`
	FollowEvent         *CalendarEvent         `json:"follow_event,omitempty"`
	Exceptions          *RecurringBatchSummary `json:"exceptions,omitempty"`
	// ExceptionsDeleted 为 true 表示 --apply-to all 改了时间，原例外占位已无意义而被删除
	ExceptionsDeleted bool `json:"exceptions_deleted,omitempty"`
}

// DescribeUpdateScope 用中文描述更新的实际影响范围
func DescribeUpdateScope(kind RecurringKind, scope string) string {
	switch scope {
	case ApplyToAll:
		return "整个重复序列（主体及全部例外）"
	case ApplyToThisAndFollowing:
		return "从该次起的后续所有实例（原序列截断，后续以新序列承载修改）"
	case ApplyToSingle:
		if kind == RecurringKindNormal {
			return "该日程"
		}
		return "仅这一次"
	}
	switch kind {
	case RecurringKindMaster:
		return "整个重复序列（已单独修改过的例外不受影响；一并修改请加 --apply-to all）"
	case RecurringKindInstance, RecurringKindException:
		return "仅这一次（修改整个序列请加 --apply-to all）"
	}
	return "该日程"
}

// eventTimeChanged 报告 params 的起止时间是否与日程当前时间不同（未传时间视为未改）
func eventTimeChanged(params *UpdateEventParams, ev *CalendarEvent) bool {
	if params.StartTime == "" && params.EndTime == "" {
		return false
	}
	if ev == nil || ev.rawStart == nil || ev.rawEnd == nil {
		return true
	}
	cmp := func(input string, ti string) bool {
		if input == "" {
			return true
		}
		ts, err := parseTimeToTimestamp(input)
		if err != nil {
			return false
		}
		return ts == ti
	}
	return !(cmp(params.StartTime, StringVal(ev.rawStart.Timestamp)) && cmp(params.EndTime, StringVal(ev.rawEnd.Timestamp)))
}

// UpdateEventScoped 按 --apply-to 更新日程（scope 已经 ValidateApplyTo 校验）。
//
//   - "" / single：直接 PATCH 传入的 ID（主日程=整条序列但不改例外；实例=只改这一次）
//   - all：改了时间 → 先删除全部例外（原占位已无意义）再 PATCH 主日程；
//     未改时间 → 把本次显式传入的字段逐个 PATCH 到每个例外，再 PATCH 主日程
//   - this-and-following：删除截断点及之后的例外 → 截断主日程 RRULE →
//     以截断点那次的时间（或显式传入的时间）新建一条继承原设置的新序列并同步参与人
func UpdateEventScoped(calendarID, eventID string, current *CalendarEvent, scope string, params UpdateEventParams, progress func(string), userAccessToken string) (*RecurringUpdateResult, error) {
	if progress == nil {
		progress = func(string) {}
	}
	kind := ClassifyEvent(current)
	res := &RecurringUpdateResult{CalendarID: calendarID, EventID: eventID, Kind: kind, ApplyTo: scope}
	noNotify := false

	switch scope {
	case "", ApplyToSingle:
		p := params
		p.CalendarID, p.EventID = calendarID, eventID
		ev, err := UpdateEvent(&p, userAccessToken)
		if err != nil {
			return nil, err
		}
		res.Event = ev
		res.Scope = DescribeUpdateScope(kind, scope)
		return res, nil

	case ApplyToAll:
		master, err := ensureRecurringMaster(calendarID, current, eventID, userAccessToken)
		if err != nil {
			return nil, err
		}
		res.MasterEventID = master.EventID
		start, err := masterStartUnix(master)
		if err != nil {
			return nil, err
		}
		end := recurringEndFromRRule(master.Recurrence, start, time.Now())
		timeChanged := eventTimeChanged(&params, master)
		exceptions, err := ListRecurringExceptions(calendarID, master.EventID, start, end, timeChanged, userAccessToken)
		if err != nil {
			return nil, err
		}
		if len(exceptions) > 0 {
			if timeChanged {
				progress(fmt.Sprintf("时间已变更：删除 %d 个例外日程后再更新重复日程主体", len(exceptions)))
				res.ExceptionsDeleted = true
				res.Exceptions = runExceptionBatch(exceptions, func(ex *RecurringException) error {
					return DeleteEventWithOptions(calendarID, ex.EventID, DeleteEventOptions{NeedNotification: &noNotify, DeleteException: true}, userAccessToken)
				})
			} else {
				// 例外只同步本次显式传入的非时间字段；例外不能带重复规则
				exParams := params
				exParams.StartTime, exParams.EndTime, exParams.Recurrence = "", "", ""
				exParams.NeedNotification = &noNotify
				if exParams.HasFields() {
					progress(fmt.Sprintf("同步修改 %d 个例外日程", len(exceptions)))
					res.Exceptions = runExceptionBatch(exceptions, func(ex *RecurringException) error {
						p := exParams
						p.CalendarID, p.EventID = calendarID, ex.EventID
						_, err := UpdateEvent(&p, userAccessToken)
						return err
					})
				}
			}
		}
		p := params
		p.CalendarID, p.EventID = calendarID, master.EventID
		if !timeChanged {
			// 时间未变（脚本回传了原时间）时不重复下发，避免误判
			p.StartTime, p.EndTime = "", ""
		}
		if !p.HasFields() {
			res.Event = master
		} else {
			ev, err := UpdateEvent(&p, userAccessToken)
			if err != nil {
				return res, fmt.Errorf("更新重复日程主体 %s 失败: %w", master.EventID, err)
			}
			res.Event = ev
		}
		res.Scope = DescribeUpdateScope(kind, scope)
		return res, nil

	case ApplyToThisAndFollowing:
		pivot, ok := ParseInstanceOriginalTime(eventID)
		if !ok {
			return nil, fmt.Errorf("--apply-to=this-and-following 需要实例 event_id（形如 {uid}_{原始时间戳}），得到 %q", eventID)
		}
		master, err := ensureRecurringMaster(calendarID, current, eventID, userAccessToken)
		if err != nil {
			return nil, err
		}
		res.MasterEventID = master.EventID
		followRule := params.Recurrence
		if followRule == "" {
			// 必须在删例外、截断主体之前计算：截断后就数不出原序列的最后一次了
			if followRule, err = followingSeriesRRule(calendarID, master, userAccessToken); err != nil {
				return nil, fmt.Errorf("计算后续序列的重复规则失败（尚未修改任何数据）: %w", err)
			}
		}
		loc := eventLocation(master)
		cutoff := PivotDayCutoff(pivot, loc)
		end := recurringEndFromRRule(master.Recurrence, cutoff.Unix(), time.Now())
		all, err := ListRecurringExceptions(calendarID, master.EventID, cutoff.Unix()+1, end, true, userAccessToken)
		if err != nil {
			return nil, err
		}
		var future []*RecurringException
		for _, ex := range all {
			if ex.StartUnix >= pivot {
				future = append(future, ex)
			}
		}
		if len(future) > 0 {
			progress(fmt.Sprintf("删除截断点及之后的 %d 个例外日程", len(future)))
			res.Exceptions = runExceptionBatch(future, func(ex *RecurringException) error {
				return DeleteEventWithOptions(calendarID, ex.EventID, DeleteEventOptions{NeedNotification: &noNotify, DeleteException: true}, userAccessToken)
			})
		}
		newRule := TruncateRecurrenceUntil(master.Recurrence, cutoff)
		progress(fmt.Sprintf("截断原序列重复规则：%s", newRule))
		if _, err := UpdateEvent(&UpdateEventParams{
			CalendarID:       calendarID,
			EventID:          master.EventID,
			Recurrence:       newRule,
			NeedNotification: params.NeedNotification,
		}, userAccessToken); err != nil {
			return res, fmt.Errorf("截断重复日程主体 %s 失败（截断点之后的例外可能已删除）: %w", master.EventID, err)
		}
		res.RecurrenceTruncated = newRule

		followParams := params
		followParams.Recurrence = followRule
		follow, err := createFollowingSeries(calendarID, master, current, pivot, followParams, userAccessToken)
		if err != nil {
			return res, fmt.Errorf("原序列已截断，但新建后续序列失败，请用 create-event 手动补建: %w", err)
		}
		progress(fmt.Sprintf("已新建后续序列 event_id=%s", follow.EventID))
		res.FollowEvent = follow
		res.Scope = DescribeUpdateScope(kind, scope)
		return res, nil
	}
	return nil, fmt.Errorf("不支持的 --apply-to: %q", scope)
}

// createFollowingSeries 以截断点为起点新建一条继承主日程设置的序列：
// 标题/描述/地点/可见性/忙闲/参与人权限/提醒/视频会议/颜色继承主日程，显式传入的字段优先；
// RRULE 继承原规则（去 COUNT）；参与人从主日程复制。
func createFollowingSeries(calendarID string, master, pivot *CalendarEvent, pivotUnix int64, params UpdateEventParams, userAccessToken string) (*CalendarEvent, error) {
	body := map[string]any{"summary": master.Summary}
	if params.Summary != "" {
		body["summary"] = params.Summary
	}
	if params.Description != "" {
		body["description"] = params.Description
	} else if master.Description != "" {
		body["description"] = master.Description
	}
	if params.Location != "" {
		body["location"] = map[string]string{"name": params.Location}
	} else if master.Location != "" {
		body["location"] = map[string]string{"name": master.Location}
	}
	if master.Visibility != "" {
		body["visibility"] = master.Visibility
	}
	if master.FreeBusyStatus != "" {
		body["free_busy_status"] = master.FreeBusyStatus
	}
	if master.AttendeeAbility != "" {
		body["attendee_ability"] = master.AttendeeAbility
	}
	if len(master.Reminders) > 0 {
		body["reminders"] = master.Reminders
	}
	if master.Vchat != nil && master.Vchat.VcType != "" {
		vc := map[string]string{"vc_type": master.Vchat.VcType}
		if master.Vchat.VcType == "third_party" {
			vc["meeting_url"] = master.Vchat.MeetingURL
			vc["description"] = master.Vchat.Description
			vc["icon_type"] = master.Vchat.IconType
		}
		body["vchat"] = vc
	}
	if master.Color != 0 {
		body["color"] = master.Color
	}

	tz := ""
	if master.rawStart != nil {
		tz = StringVal(master.rawStart.Timezone)
	}
	var startTs, endTs string
	if params.StartTime != "" && params.EndTime != "" {
		var err error
		if startTs, err = parseTimeToTimestamp(params.StartTime); err != nil {
			return nil, fmt.Errorf("解析开始时间失败: %w", err)
		}
		if endTs, err = parseTimeToTimestamp(params.EndTime); err != nil {
			return nil, fmt.Errorf("解析结束时间失败: %w", err)
		}
	} else if pivot != nil && pivot.rawStart != nil && pivot.rawEnd != nil &&
		StringVal(pivot.rawStart.Timestamp) != "" && StringVal(pivot.rawEnd.Timestamp) != "" {
		startTs, endTs = StringVal(pivot.rawStart.Timestamp), StringVal(pivot.rawEnd.Timestamp)
	} else {
		startTs, endTs = strconv.FormatInt(pivotUnix, 10), strconv.FormatInt(pivotUnix+3600, 10)
	}
	startMap := map[string]string{"timestamp": startTs}
	endMap := map[string]string{"timestamp": endTs}
	if tz != "" && tz != "UTC" {
		startMap["timezone"] = tz
		endMap["timezone"] = tz
	}
	body["start_time"] = startMap
	body["end_time"] = endMap
	rule := params.Recurrence
	if rule == "" {
		rule = InheritRRuleForFollowing(master.Recurrence)
	}
	if rule != "" {
		body["recurrence"] = rule
	}

	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	resp, err := cli.Post(Context(), fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events", url.PathEscape(calendarID)), body, tokenType, opts...)
	if err != nil {
		return nil, fmt.Errorf("创建日程失败: %w", err)
	}
	if err := CheckAPIResponse("创建日程", resp); err != nil {
		return nil, err
	}
	var apiResp struct {
		Data struct {
			Event *calendarEventWire `json:"event"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &apiResp); err != nil || apiResp.Data.Event == nil {
		return nil, fmt.Errorf("创建日程成功但未返回日程信息")
	}
	follow := convertEvent(&apiResp.Data.Event.CalendarEvent)

	// 参与人：从主日程复制（日程创建接口不收内联参与人）
	var inherited []*EventAttendee
	pageToken := ""
	for page := 0; page < 50; page++ {
		items, next, more, err := ListEventAttendees(calendarID, master.EventID, 100, pageToken, userAccessToken)
		if err != nil {
			return follow, fmt.Errorf("新序列 %s 已创建，但读取原序列参与人失败: %w", follow.EventID, err)
		}
		for _, a := range items {
			switch a.Type {
			case "user":
				if a.UserID != "" {
					inherited = append(inherited, &EventAttendee{Type: "user", UserID: a.UserID})
				}
			case "chat":
				if a.ChatID != "" {
					inherited = append(inherited, &EventAttendee{Type: "chat", ChatID: a.ChatID})
				}
			case "resource":
				if a.RoomID != "" {
					inherited = append(inherited, &EventAttendee{Type: "resource", RoomID: a.RoomID})
				}
			case "third_party":
				if a.ThirdPartyEmail != "" {
					inherited = append(inherited, &EventAttendee{Type: "third_party", ThirdPartyEmail: a.ThirdPartyEmail})
				}
			}
		}
		if !more || next == "" || next == pageToken {
			break
		}
		pageToken = next
	}
	if len(inherited) > 0 {
		if err := AddEventAttendees(calendarID, follow.EventID, inherited, userAccessToken); err != nil {
			return follow, fmt.Errorf("新序列 %s 已创建，但同步参与人失败: %w", follow.EventID, err)
		}
	}
	return follow, nil
}

// calendarRateLimitCodes 日历侧限流/并发冲突业务码（官方 calendar_recurring.go 同口径）+ 网关限流
var calendarRateLimitCodes = []int{190004, 190005, 190010, 99991400}

// calendarRetrySleep 测试可替换
var calendarRetrySleep = func(d time.Duration) bool {
	select {
	case <-Context().Done():
		return false
	case <-time.After(d):
		return true
	}
}

// withCalendarRateLimitRetry 对日历限流类错误做有限次退避重试（1s/2s/4s），其余错误立即返回。
// 只包装幂等或可安全重放的请求（GET、share_info、按 ID 删除/修改）。
func withCalendarRateLimitRetry(fn func() error) error {
	wait := time.Second
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}
		limited := false
		for _, code := range calendarRateLimitCodes {
			if HasAPICode(err, code) {
				limited = true
				break
			}
		}
		if !limited || attempt == 3 {
			return err
		}
		if !calendarRetrySleep(wait) {
			return err
		}
		wait *= 2
	}
	return err
}
