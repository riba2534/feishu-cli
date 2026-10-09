package cmd

import (
	"net/http"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/v2/internal/clierr"
)

// TestSheetInsertCols_Conversion insert-cols 与 insert-rows 同口径：--range "C:D" → startIndex 2, endIndex 4。
func TestSheetInsertCols_Conversion(t *testing.T) {
	m := newSheetMockServer(t)
	if _, err := runSheetCmdForTest(t, sheetInsertColsCmd, []string{"shtTok", "s1"}, map[string]string{"range": "C:D", "inherit-style": "before"}); err != nil {
		t.Fatal(err)
	}
	reqs := m.requests()
	if len(reqs) != 1 || !strings.HasSuffix(reqs[0].Path, "/insert_dimension_range") {
		t.Fatalf("请求 = %+v", reqs)
	}
	body := decodeBody(t, reqs[0].Body)
	assertDim(t, body["dimension"].(map[string]any), "COLUMNS", 2, 4)
	if body["inheritStyle"] != "BEFORE" {
		t.Errorf("inheritStyle = %v", body["inheritStyle"])
	}
}

// TestSheetUpdateAndMoveDimension_Commands 新增的 update-dimension / move-dimension / update-sheet / freeze 请求体。
func TestSheetUpdateAndMoveDimension_Commands(t *testing.T) {
	m := newSheetMockServer(t)
	if _, err := runSheetCmdForTest(t, sheetUpdateDimensionCmd, []string{"shtTok", "s1"}, map[string]string{"range": "B:D", "hidden": "false", "size": "120"}); err != nil {
		t.Fatal(err)
	}
	req := m.requests()[0]
	body := decodeBody(t, req.Body)
	assertDim(t, body["dimension"].(map[string]any), "COLUMNS", 2, 4)
	props := body["dimensionProperties"].(map[string]any)
	if props["visible"] != true || props["fixedSize"] != float64(120) {
		t.Errorf("dimensionProperties = %v", props)
	}

	if _, err := runSheetCmdForTest(t, sheetMoveDimensionCmd, []string{"shtTok", "s1"}, map[string]string{"range": "2:3", "target": "5"}); err != nil {
		t.Fatal(err)
	}
	req = m.requests()[1]
	if !strings.HasSuffix(req.Path, "/sheets/s1/move_dimension") {
		t.Fatalf("path = %s", req.Path)
	}
	mv := decodeBody(t, req.Body)
	src := mv["source"].(map[string]any)
	if src["start_index"] != float64(1) || src["end_index"] != float64(2) || mv["destination_index"] != float64(4) {
		t.Errorf("move body = %v", mv)
	}
	if _, err := runSheetCmdForTest(t, sheetMoveDimensionCmd, []string{"shtTok", "s1"}, map[string]string{"range": "2:3", "target": "E"}); err == nil {
		t.Error("--target 维度与 --range 不一致应报错")
	}

	m.respond = func(w http.ResponseWriter, r *http.Request, body []byte) bool {
		_, _ = w.Write([]byte(`{"code":0,"msg":"ok","data":{"replies":[]}}`))
		return true
	}
	if _, err := runSheetCmdForTest(t, sheetFreezeCmd, []string{"shtTok", "s1"}, map[string]string{"rows": "1", "cols": "0"}); err != nil {
		t.Fatal(err)
	}
	reqs := m.requests()
	raw := string(reqs[len(reqs)-1].Body)
	if !strings.Contains(raw, `"frozenRowCount":1`) || !strings.Contains(raw, `"frozenColCount":0`) {
		t.Errorf("freeze body = %s", raw)
	}
	if _, err := runSheetCmdForTest(t, sheetUpdateSheetCmd, []string{"shtTok", "s1"}, map[string]string{"title": "汇总", "hidden": "false", "index": "0"}); err != nil {
		t.Fatal(err)
	}
	reqs = m.requests()
	raw = string(reqs[len(reqs)-1].Body)
	for _, want := range []string{`"title":"汇总"`, `"hidden":false`, `"index":0`} {
		if !strings.Contains(raw, want) {
			t.Errorf("update-sheet body 缺少 %s: %s", want, raw)
		}
	}
	if _, err := runSheetCmdForTest(t, sheetUpdateSheetCmd, []string{"shtTok", "s1"}, nil); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Errorf("update-sheet 无属性应返回用法错误，得到 %v", err)
	}
}
