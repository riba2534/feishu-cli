package client

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// TestParseOKRWeightOpsValidation 总和大于 1、小数超过 3 位、重复 id、越界、空 id 均拒绝
func TestParseOKRWeightOpsValidation(t *testing.T) {
	bad := map[string]string{
		"总和大于 1":   `[{"id":"a","weight":0.6},{"id":"b","weight":0.5}]`,
		"小数超过 3 位": `[{"id":"a","weight":0.1234}]`,
		"重复 id":    `[{"id":"a","weight":0.1},{"id":"a","weight":0.2}]`,
		"负数":       `[{"id":"a","weight":-0.1}]`,
		"大于 1":     `[{"id":"a","weight":1.5}]`,
		"空 id":     `[{"id":" ","weight":0.1}]`,
		"空数组":      `[]`,
		"非数组":      `{"id":"a"}`,
	}
	for name, raw := range bad {
		if _, err := ParseOKRWeightOps(raw); err == nil {
			t.Fatalf("%s 应报错: %s", name, raw)
		}
	}
	ops, err := ParseOKRWeightOps(`[{"id":"a","weight":0.333},{"id":"b","weight":0.667}]`)
	if err != nil || len(ops) != 2 {
		t.Fatalf("合法输入: %v", err)
	}
}

func weightsOf(t *testing.T, res []OKRWeightResult) map[string]string {
	t.Helper()
	out := map[string]string{}
	var total float64
	for _, r := range res {
		out[r.ID] = r.Weight.String()
		f, _ := r.Weight.Float64()
		total += f
	}
	if total < 0.9999 || total > 1.0001 {
		t.Fatalf("总和应为 1: %+v", res)
	}
	return out
}

// TestNormalizeOKRWeights 指定项保留；其余按原权重比例分摊；原权重全 0 均分；尾差计入最后一项；按 position 排序
func TestNormalizeOKRWeights(t *testing.T) {
	items := []OKRItem{{ID: "c", Position: 3, Weight: 0.2}, {ID: "a", Position: 1, Weight: 0.5}, {ID: "b", Position: 2, Weight: 0.3}}
	res, err := NormalizeOKRWeights(items, []OKRWeightOp{{ID: "a", Weight: 0.4}})
	if err != nil {
		t.Fatal(err)
	}
	if res[0].ID != "a" || res[1].ID != "b" || res[2].ID != "c" {
		t.Fatalf("应按 position 排序: %+v", res)
	}
	w := weightsOf(t, res)
	// 剩余 0.6 按 0.3:0.2 分摊 → 0.36 / 0.24
	if w["a"] != "0.400" || w["b"] != "0.360" || w["c"] != "0.240" {
		t.Fatalf("比例分摊不对: %v", w)
	}

	// 原权重全为 0：剩余 1 均分给 3 项，尾差进最后一项
	zero := []OKRItem{{ID: "x", Position: 1}, {ID: "y", Position: 2}, {ID: "z", Position: 3}}
	w = weightsOf(t, mustNormalize(t, zero, nil))
	if w["x"] != "0.333" || w["y"] != "0.333" || w["z"] != "0.334" {
		t.Fatalf("均分不对: %v", w)
	}

	// 全部指定但总和不足 1：尾差补到最后一项
	w = weightsOf(t, mustNormalize(t, []OKRItem{{ID: "p", Position: 1}, {ID: "q", Position: 2}}, []OKRWeightOp{{ID: "p", Weight: 0.3}, {ID: "q", Weight: 0.3}}))
	if w["p"] != "0.300" || w["q"] != "0.700" {
		t.Fatalf("尾差补齐不对: %v", w)
	}

	// 指定不存在的 id
	if _, err := NormalizeOKRWeights(items, []OKRWeightOp{{ID: "nope", Weight: 0.1}}); err == nil {
		t.Fatal("不存在的 id 应报错")
	}

	// json.Number 序列化为固定三位小数
	b, _ := json.Marshal(OKRWeightRequest(OKRLevelObjective, "1", "", res).Body)
	if !strings.Contains(string(b), `"weight":0.400`) {
		t.Fatalf("权重应按 3 位小数序列化: %s", b)
	}
}

func mustNormalize(t *testing.T, items []OKRItem, ops []OKRWeightOp) []OKRWeightResult {
	t.Helper()
	res, err := NormalizeOKRWeights(items, ops)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// TestOKRReorder 校验 ops 与完整顺序计算
func TestOKRReorder(t *testing.T) {
	for _, raw := range []string{`[]`, `[{"id":"a","position":0}]`, `[{"id":"a","position":1},{"id":"a","position":2}]`, `[{"id":"a","position":1},{"id":"b","position":1}]`} {
		if _, err := ParseOKRReorderOps(raw); err == nil {
			t.Fatalf("%s 应报错", raw)
		}
	}
	items := []OKRItem{{ID: "a", Position: 1}, {ID: "b", Position: 2}, {ID: "c", Position: 3}, {ID: "d", Position: 4}}
	ids, err := BuildOKRReorderedIDs(items, []OKRReorderOp{{ID: "c", Position: 1}})
	if err != nil || strings.Join(ids, ",") != "c,a,b,d" {
		t.Fatalf("把 c 移到第一位: %v %v", ids, err)
	}
	ids, _ = BuildOKRReorderedIDs(items, []OKRReorderOp{{ID: "a", Position: 4}, {ID: "d", Position: 2}})
	if strings.Join(ids, ",") != "b,d,c,a" {
		t.Fatalf("多项移动: %v", ids)
	}
	if _, err := BuildOKRReorderedIDs(items, []OKRReorderOp{{ID: "zz", Position: 1}}); err == nil {
		t.Fatal("不存在的 id 应报错")
	}
}

// TestBatchCreateOKRRollback 中途失败时逆序删除已创建的目标；回滚失败时列出残留
func TestBatchCreateOKRRollback(t *testing.T) {
	old := okrBatchWriter
	t.Cleanup(func() { okrBatchWriter = old })

	var calls []string
	objN := 0
	okrBatchWriter = func(req *OKRWriteRequest, action, token string) (json.RawMessage, error) {
		calls = append(calls, req.Method+" "+req.Path)
		switch {
		case req.Method == "POST" && strings.HasSuffix(req.Path, "/objectives"):
			objN++
			return json.RawMessage(`{"objective_id":"o` + string(rune('0'+objN)) + `"}`), nil
		case req.Method == "POST" && strings.HasSuffix(req.Path, "/key_results"):
			if strings.Contains(req.Path, "/o2/") {
				return nil, errors.New("boom")
			}
			return json.RawMessage(`{"key_result_id":"k1"}`), nil
		case req.Method == "DELETE":
			return json.RawMessage(`{}`), nil
		}
		return nil, errors.New("unexpected")
	}
	objs, err := ParseOKRBatchObjectives(`[{"text":"O1","krs":[{"text":"K1"}]},{"text":"O2","krs":[{"text":"K2"}]}]`)
	if err != nil {
		t.Fatal(err)
	}
	created, err := BatchCreateOKRObjectives("100", "", "open_id", objs, "")
	if err == nil || !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "已回滚删除本次创建的 2 个目标") {
		t.Fatalf("应回滚并保留原错误: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("created: %+v", created)
	}
	n := len(calls)
	if calls[n-2] != "DELETE /open-apis/okr/v2/objectives/o2" || calls[n-1] != "DELETE /open-apis/okr/v2/objectives/o1" {
		t.Fatalf("应逆序删除: %v", calls)
	}

	// 回滚本身失败
	objN = 0
	calls = nil
	okrBatchWriter = func(req *OKRWriteRequest, action, token string) (json.RawMessage, error) {
		if req.Method == "DELETE" {
			return nil, errors.New("deny")
		}
		if strings.HasSuffix(req.Path, "/key_results") {
			return nil, errors.New("boom")
		}
		return json.RawMessage(`{"objective_id":"o1"}`), nil
	}
	_, err = BatchCreateOKRObjectives("100", "", "open_id", objs[:1], "")
	if err == nil || !strings.Contains(err.Error(), "请手动清理: o1") {
		t.Fatalf("回滚失败应列出残留: %v", err)
	}

	// 第一个目标就失败：不回滚，原样返回
	okrBatchWriter = func(req *OKRWriteRequest, action, token string) (json.RawMessage, error) {
		calls = append(calls, req.Method)
		return nil, errors.New("first")
	}
	calls = nil
	_, err = BatchCreateOKRObjectives("100", "", "open_id", objs, "")
	if err == nil || err.Error() != "first" || len(calls) != 1 {
		t.Fatalf("首个失败不应回滚: %v %v", err, calls)
	}
}

// TestParseOKRBatchObjectives 输入校验
func TestParseOKRBatchObjectives(t *testing.T) {
	for _, raw := range []string{`[]`, `[{"text":" "}]`, `[{"text":"o","krs":[{"text":""}]}]`, `[{"text":"o","category_id":"abc"}]`, `[{"text":"o","notes":"  "}]`} {
		if _, err := ParseOKRBatchObjectives(raw); err == nil {
			t.Fatalf("%s 应报错", raw)
		}
	}
}
