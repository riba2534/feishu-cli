package client

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// ============================================================================
// OKR 权重 / 排序 / 指标 / 评论 get-update-delete / 目标批量创建（对齐官方 okr +weight / +reorder /
// +indicator-update / +comment-get|patch|delete / +batch-create）。
// 纯函数（解析、归一化、排序）不联网，dry-run 与执行共用。
// ============================================================================

// OKRLevel 权重、排序、指标操作的层级
const (
	OKRLevelObjective = "objective"
	OKRLevelKeyResult = "key-result"
)

// OKRItem 参与权重 / 排序计算的目标或关键结果（只取 ID、位置、权重）
type OKRItem struct {
	ID       string
	Position int32
	Weight   float64
}

// ---------------------------------------------------------------- 权重

// OKRWeightOp 一条权重指定
type OKRWeightOp struct {
	ID     string  `json:"id"`
	Weight float64 `json:"weight"`
}

// OKRWeightResult 归一化后的权重（json.Number 固定 3 位小数，避免浮点误差）
type OKRWeightResult struct {
	ID     string      `json:"id"`
	Weight json.Number `json:"weight"`
}

const okrWeightScale = 1000 // 三位小数定点

// ParseOKRWeightOps 解析并校验 --weights：每项 0..1、最多 3 位小数、ID 不重复、总和 ≤ 1。
func ParseOKRWeightOps(raw string) ([]OKRWeightOp, error) {
	var ops []OKRWeightOp
	if err := json.Unmarshal([]byte(raw), &ops); err != nil {
		return nil, fmt.Errorf("--weights 必须是 JSON 数组，如 [{\"id\":\"7xxx\",\"weight\":0.4}]: %v", err)
	}
	if len(ops) == 0 {
		return nil, fmt.Errorf("--weights 至少需要一项")
	}
	seen := map[string]bool{}
	var sum int64
	for i, op := range ops {
		if strings.TrimSpace(op.ID) == "" {
			return nil, fmt.Errorf("weights[%d].id 不能为空", i)
		}
		if op.Weight < 0 || op.Weight > 1 || math.IsNaN(op.Weight) {
			return nil, fmt.Errorf("weights[%d].weight 必须在 0 到 1 之间", i)
		}
		if math.Round(op.Weight*okrWeightScale)/okrWeightScale != op.Weight {
			return nil, fmt.Errorf("weights[%d].weight 最多 3 位小数", i)
		}
		if seen[op.ID] {
			return nil, fmt.Errorf("--weights 中 id %q 重复", op.ID)
		}
		seen[op.ID] = true
		sum += int64(math.Round(op.Weight * okrWeightScale))
	}
	if sum > okrWeightScale {
		return nil, fmt.Errorf("--weights 权重总和必须 ≤ 1，当前 %.3f", float64(sum)/okrWeightScale)
	}
	return ops, nil
}

func formatOKRWeight(fp int64) json.Number {
	return json.Number(fmt.Sprintf("%d.%03d", fp/okrWeightScale, fp%okrWeightScale))
}

// NormalizeOKRWeights 把指定的权重原样保留，剩余权重按未指定项的原权重比例分摊（原权重全为 0 时均分），
// 定点运算保证总和恰为 1（尾差计入最后一项）。结果按 position 排序（接口要求完整列表）。
func NormalizeOKRWeights(items []OKRItem, ops []OKRWeightOp) ([]OKRWeightResult, error) {
	exists := map[string]bool{}
	for _, it := range items {
		exists[it.ID] = true
	}
	specified := map[string]int64{}
	var specifiedSum int64
	for _, op := range ops {
		if !exists[op.ID] {
			return nil, fmt.Errorf("id %q 不在当前列表中", op.ID)
		}
		fp := int64(math.Round(op.Weight * okrWeightScale))
		specified[op.ID] = fp
		specifiedSum += fp
	}
	remaining := int64(okrWeightScale) - specifiedSum
	if remaining < 0 {
		return nil, fmt.Errorf("权重总和超过 1")
	}

	sorted := append([]OKRItem(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Position < sorted[j].Position })

	weights := map[string]int64{}
	var unspecified []OKRItem
	var origSum int64
	for _, it := range sorted {
		if fp, ok := specified[it.ID]; ok {
			weights[it.ID] = fp
			continue
		}
		unspecified = append(unspecified, it)
		w := it.Weight
		if w < 0 {
			w = 0
		}
		origSum += int64(math.Round(w * okrWeightScale))
	}
	switch {
	case len(unspecified) == 0:
		// 全部指定但总和不足 1：尾差补到最后一项
		if remaining > 0 && len(sorted) > 0 {
			weights[sorted[len(sorted)-1].ID] += remaining
		}
	case origSum == 0:
		per := remaining / int64(len(unspecified))
		for i, it := range unspecified {
			fp := per
			if i == len(unspecified)-1 {
				fp = remaining - per*int64(len(unspecified)-1)
			}
			weights[it.ID] = fp
		}
	default:
		var distributed int64
		for i, it := range unspecified {
			if i == len(unspecified)-1 {
				weights[it.ID] = remaining - distributed
				break
			}
			w := it.Weight
			if w < 0 {
				w = 0
			}
			fp := int64(float64(remaining) * float64(int64(math.Round(w*okrWeightScale))) / float64(origSum))
			weights[it.ID] = fp
			distributed += fp
		}
	}

	out := make([]OKRWeightResult, 0, len(sorted))
	var total int64
	for _, it := range sorted {
		fp := weights[it.ID]
		total += fp
		out = append(out, OKRWeightResult{ID: it.ID, Weight: formatOKRWeight(fp)})
	}
	if total != okrWeightScale {
		return nil, fmt.Errorf("权重归一化异常：总和 %.3f，应为 1", float64(total)/okrWeightScale)
	}
	return out, nil
}

// OKRWeightRequest 构造权重更新请求（PUT 完整列表）
func OKRWeightRequest(level, cycleID, objectiveID string, weights []OKRWeightResult) *OKRWriteRequest {
	if level == OKRLevelObjective {
		list := make([]any, 0, len(weights))
		for _, w := range weights {
			list = append(list, map[string]any{"objective_id": w.ID, "weight": w.Weight})
		}
		return &OKRWriteRequest{Method: "PUT", Path: "/open-apis/okr/v2/cycles/" + url.PathEscape(cycleID) + "/objectives_weight", Body: map[string]any{"objective_weights": list}}
	}
	list := make([]any, 0, len(weights))
	for _, w := range weights {
		list = append(list, map[string]any{"key_result_id": w.ID, "weight": w.Weight})
	}
	return &OKRWriteRequest{Method: "PUT", Path: "/open-apis/okr/v2/objectives/" + url.PathEscape(objectiveID) + "/key_results_weight", Body: map[string]any{"key_result_weights": list}}
}

// ---------------------------------------------------------------- 排序

// OKRReorderOp 把 id 移到 position（1 起）
type OKRReorderOp struct {
	ID       string `json:"id"`
	Position int32  `json:"position"`
}

// ParseOKRReorderOps 解析并校验 --ops：position 为正整数，id 与 position 均不重复。
func ParseOKRReorderOps(raw string) ([]OKRReorderOp, error) {
	var ops []OKRReorderOp
	if err := json.Unmarshal([]byte(raw), &ops); err != nil {
		return nil, fmt.Errorf("--ops 必须是 JSON 数组，如 [{\"id\":\"7xxx\",\"position\":1}]: %v", err)
	}
	if len(ops) == 0 {
		return nil, fmt.Errorf("--ops 至少需要一项")
	}
	seenID, seenPos := map[string]bool{}, map[int32]bool{}
	for i, op := range ops {
		if strings.TrimSpace(op.ID) == "" {
			return nil, fmt.Errorf("ops[%d].id 不能为空", i)
		}
		if op.Position <= 0 {
			return nil, fmt.Errorf("ops[%d].position 必须是正整数（1 表示第一位）", i)
		}
		if seenID[op.ID] {
			return nil, fmt.Errorf("--ops 中 id %q 重复", op.ID)
		}
		if seenPos[op.Position] {
			return nil, fmt.Errorf("--ops 中 position %d 重复", op.Position)
		}
		seenID[op.ID], seenPos[op.Position] = true, true
	}
	return ops, nil
}

// BuildOKRReorderedIDs 把指定项放到目标位置，其余项按原顺序依次填入空位，返回完整 ID 顺序。
func BuildOKRReorderedIDs(items []OKRItem, ops []OKRReorderOp) ([]string, error) {
	sorted := append([]OKRItem(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Position < sorted[j].Position })
	exists := map[string]bool{}
	for _, it := range sorted {
		exists[it.ID] = true
	}
	posToID := map[int]string{}
	used := map[string]bool{}
	for _, op := range ops {
		if !exists[op.ID] {
			return nil, fmt.Errorf("id %q 不在当前列表中", op.ID)
		}
		posToID[int(op.Position)] = op.ID
		used[op.ID] = true
	}
	pos := 1
	for _, it := range sorted {
		if used[it.ID] {
			continue
		}
		for {
			if _, taken := posToID[pos]; !taken {
				break
			}
			pos++
		}
		posToID[pos] = it.ID
	}
	positions := make([]int, 0, len(posToID))
	for p := range posToID {
		positions = append(positions, p)
	}
	sort.Ints(positions)
	out := make([]string, 0, len(positions))
	for _, p := range positions {
		out = append(out, posToID[p])
	}
	return out, nil
}

// OKRReorderRequest 构造排序请求（PUT 完整 ID 列表）
func OKRReorderRequest(level, cycleID, objectiveID string, ids []string) *OKRWriteRequest {
	if level == OKRLevelObjective {
		return &OKRWriteRequest{Method: "PUT", Path: "/open-apis/okr/v2/cycles/" + url.PathEscape(cycleID) + "/objectives_position", Body: map[string]any{"objective_ids": ids}}
	}
	return &OKRWriteRequest{Method: "PUT", Path: "/open-apis/okr/v2/objectives/" + url.PathEscape(objectiveID) + "/key_results_position", Body: map[string]any{"key_result_ids": ids}}
}

// ListOKRLevelItems 读取周期下的目标或目标下的关键结果（权重 / 排序计算用）
func ListOKRLevelItems(level, cycleID, objectiveID, userAccessToken string) ([]OKRItem, error) {
	cli, err := GetClient()
	if err != nil {
		return nil, err
	}
	tokenType, opts := resolveTokenOpts(userAccessToken)
	var items []OKRItem
	if level == OKRLevelObjective {
		raw, err := listOKRCycleObjectives(cli, tokenType, opts, cycleID)
		if err != nil {
			return nil, err
		}
		for _, o := range raw {
			items = append(items, okrItem(o.ID, o.Position, o.Weight))
		}
		return items, nil
	}
	krs, err := listOKRObjectiveKeyResults(cli, tokenType, opts, objectiveID)
	if err != nil {
		return nil, err
	}
	for _, k := range krs {
		items = append(items, okrItem(k.ID, k.Position, k.Weight))
	}
	return items, nil
}

func okrItem(id string, pos *int32, weight *float64) OKRItem {
	it := OKRItem{ID: id}
	if pos != nil {
		it.Position = *pos
	}
	if weight != nil {
		it.Weight = *weight
	}
	return it
}

// ---------------------------------------------------------------- 指标

// ParseOKRIndicatorValue 指标当前值：数字，范围 ±99999999999
func ParseOKRIndicatorValue(raw string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < -99999999999 || v > 99999999999 {
		return 0, fmt.Errorf("--value 必须是 -99999999999 到 99999999999 之间的数字")
	}
	return v, nil
}

// OKRIndicatorListPath 目标 / 关键结果的指标查询路径
func OKRIndicatorListPath(level, id string) string {
	if level == OKRLevelObjective {
		return "/open-apis/okr/v2/objectives/" + url.PathEscape(id) + "/indicators"
	}
	return "/open-apis/okr/v2/key_results/" + url.PathEscape(id) + "/indicators"
}

// GetOKRIndicatorID 查询目标 / 关键结果绑定的指标 ID
func GetOKRIndicatorID(level, id, userAccessToken string) (string, error) {
	data, err := DoOKRWrite(&OKRWriteRequest{Method: "GET", Path: OKRIndicatorListPath(level, id), Query: map[string]any{"page_size": 100}}, "查询 OKR 指标", userAccessToken)
	if err != nil {
		return "", err
	}
	var resp struct {
		Indicator struct {
			ID string `json:"id"`
		} `json:"indicator"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("解析 OKR 指标响应失败: %w", err)
	}
	if resp.Indicator.ID == "" {
		return "", fmt.Errorf("该%s未配置指标（indicator 为空），请先在 OKR 中为其设置量化指标", map[string]string{OKRLevelObjective: "目标", OKRLevelKeyResult: "关键结果"}[level])
	}
	return resp.Indicator.ID, nil
}

// OKRIndicatorPatchRequest 更新指标当前值
func OKRIndicatorPatchRequest(indicatorID string, value float64) *OKRWriteRequest {
	return &OKRWriteRequest{Method: "PATCH", Path: "/open-apis/okr/v2/indicators/" + url.PathEscape(indicatorID), Body: map[string]any{"current_value": value}}
}

// ---------------------------------------------------------------- 评论

// OKRCommentPath 单条评论路径
func OKRCommentPath(commentID string) string {
	return "/open-apis/okr/v2/comments/" + url.PathEscape(commentID)
}

// ---------------------------------------------------------------- 批量创建

// OKRBatchKR 批量创建中的关键结果
type OKRBatchKR struct {
	Text string `json:"text"`
}

// OKRBatchObjective 批量创建中的目标
type OKRBatchObjective struct {
	Text       string       `json:"text"`
	Notes      string       `json:"notes,omitempty"`
	CategoryID string       `json:"category_id,omitempty"`
	KRs        []OKRBatchKR `json:"krs,omitempty"`
}

// ParseOKRBatchObjectives 解析 --input：[{"text":"目标","notes":"备注","category_id":"…","krs":[{"text":"KR"}]}]
func ParseOKRBatchObjectives(raw string) ([]OKRBatchObjective, error) {
	var objs []OKRBatchObjective
	if err := json.Unmarshal([]byte(raw), &objs); err != nil {
		return nil, fmt.Errorf("--input 必须是 JSON 数组，如 [{\"text\":\"目标\",\"krs\":[{\"text\":\"KR\"}]}]: %v", err)
	}
	if len(objs) == 0 {
		return nil, fmt.Errorf("--input 至少需要一个目标")
	}
	for i, o := range objs {
		if strings.TrimSpace(o.Text) == "" {
			return nil, fmt.Errorf("objective[%d].text 不能为空", i)
		}
		if o.Notes != "" && strings.TrimSpace(o.Notes) == "" {
			return nil, fmt.Errorf("objective[%d].notes 不能是空白", i)
		}
		if o.CategoryID != "" {
			if n, err := strconv.ParseInt(o.CategoryID, 10, 64); err != nil || n <= 0 {
				return nil, fmt.Errorf("objective[%d].category_id 必须是正整数 ID", i)
			}
		}
		for j, kr := range o.KRs {
			if strings.TrimSpace(kr.Text) == "" {
				return nil, fmt.Errorf("objective[%d].krs[%d].text 不能为空", i, j)
			}
		}
	}
	return objs, nil
}

// OKRBatchCreated 批量创建结果
type OKRBatchCreated struct {
	ObjectiveID  string   `json:"objective_id"`
	KeyResultIDs []string `json:"key_result_ids"`
}

// OKRBatchPlan 把批量输入展开为逐条创建请求（dry-run 展示用；KR 的 objective_id 待创建后回填）
func OKRBatchPlan(cycleID, defaultCategoryID, userIDType string, objs []OKRBatchObjective) ([]*OKRWriteRequest, error) {
	var reqs []*OKRWriteRequest
	for _, o := range objs {
		req, err := buildBatchObjectiveRequest(cycleID, defaultCategoryID, userIDType, o)
		if err != nil {
			return nil, err
		}
		reqs = append(reqs, req)
		for _, kr := range o.KRs {
			krReq, err := BuildOKRCreateKeyResult("<objective_id>", OKRV2TextContent(kr.Text), userIDType)
			if err != nil {
				return nil, err
			}
			krReq.Path = "/open-apis/okr/v2/objectives/<objective_id>/key_results" // 占位符不做 URL 转义，便于阅读
			reqs = append(reqs, krReq)
		}
	}
	return reqs, nil
}

func buildBatchObjectiveRequest(cycleID, defaultCategoryID, userIDType string, o OKRBatchObjective) (*OKRWriteRequest, error) {
	var notes map[string]any
	if o.Notes != "" {
		notes = OKRV2TextContent(o.Notes)
	}
	category := o.CategoryID
	if category == "" {
		category = defaultCategoryID
	}
	return BuildOKRCreateObjective(cycleID, OKRV2TextContent(o.Text), notes, category, userIDType)
}

// okrBatchWriter 批量创建的写操作（测试可替换）
var okrBatchWriter = DoOKRWrite

// BatchCreateOKRObjectives 逐个创建目标及其关键结果；任一步失败时逆序删除本次已创建的目标
// （关键结果随目标一并删除），返回的错误说明回滚结果与需要手动清理的残留。
func BatchCreateOKRObjectives(cycleID, defaultCategoryID, userIDType string, objs []OKRBatchObjective, userAccessToken string) ([]OKRBatchCreated, error) {
	var created []OKRBatchCreated
	fail := func(cause error) ([]OKRBatchCreated, error) {
		return created, okrBatchRollback(cause, created, userAccessToken)
	}
	for _, o := range objs {
		req, err := buildBatchObjectiveRequest(cycleID, defaultCategoryID, userIDType, o)
		if err != nil {
			return fail(err)
		}
		data, err := okrBatchWriter(req, "创建 OKR 目标", userAccessToken)
		if err != nil {
			return fail(err)
		}
		var objResp struct {
			ObjectiveID string `json:"objective_id"`
		}
		if err := json.Unmarshal(data, &objResp); err != nil || objResp.ObjectiveID == "" {
			return fail(fmt.Errorf("创建目标的响应缺少 objective_id"))
		}
		cur := OKRBatchCreated{ObjectiveID: objResp.ObjectiveID, KeyResultIDs: []string{}}
		created = append(created, cur)
		for _, kr := range o.KRs {
			krReq, err := BuildOKRCreateKeyResult(objResp.ObjectiveID, OKRV2TextContent(kr.Text), userIDType)
			if err != nil {
				return fail(err)
			}
			data, err := okrBatchWriter(krReq, "创建 OKR 关键结果", userAccessToken)
			if err != nil {
				return fail(err)
			}
			var krResp struct {
				KeyResultID string `json:"key_result_id"`
			}
			if err := json.Unmarshal(data, &krResp); err != nil || krResp.KeyResultID == "" {
				return fail(fmt.Errorf("创建关键结果的响应缺少 key_result_id"))
			}
			created[len(created)-1].KeyResultIDs = append(created[len(created)-1].KeyResultIDs, krResp.KeyResultID)
		}
	}
	return created, nil
}

// okrBatchRollback 逆序删除已创建的目标，汇总回滚结果；原始错误用 %w 保留（退出码按原错误分类）
func okrBatchRollback(cause error, created []OKRBatchCreated, userAccessToken string) error {
	if len(created) == 0 {
		return cause
	}
	var failed []string
	for i := len(created) - 1; i >= 0; i-- {
		id := created[i].ObjectiveID
		req := &OKRWriteRequest{Method: "DELETE", Path: "/open-apis/okr/v2/objectives/" + url.PathEscape(id)}
		if _, err := okrBatchWriter(req, "回滚删除 OKR 目标", userAccessToken); err != nil {
			failed = append(failed, fmt.Sprintf("%s（%v）", id, err))
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("%w\n批量创建失败，回滚时有 %d 个目标未能删除，请手动清理: %s", cause, len(failed), strings.Join(failed, "; "))
	}
	return fmt.Errorf("%w\n批量创建失败，已回滚删除本次创建的 %d 个目标（关键结果随目标删除）", cause, len(created))
}
