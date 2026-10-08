package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/client"
)

func TestParseTablePutStartCell(t *testing.T) {
	for in, want := range map[string][2]int{"": {0, 0}, "A1": {0, 0}, "c5": {2, 4}, "$AA$10": {26, 9}} {
		c, r, err := parseTablePutStartCell(in)
		if err != nil || c != want[0] || r != want[1] {
			t.Errorf("parseTablePutStartCell(%q) = (%d,%d,%v), want %v", in, c, r, err, want)
		}
	}
	for _, bad := range []string{"A1:B2", "A", "3", "Sheet1!A1", "A0"} {
		if _, _, err := parseTablePutStartCell(bad); err == nil {
			t.Errorf("parseTablePutStartCell(%q) 应报错", bad)
		}
	}
}

func TestLastNonEmptyRow(t *testing.T) {
	cr := &client.CellRange{Values: [][]any{{"a", nil}, {nil, nil}, {nil, json.Number("1")}, {nil, ""}, {nil, nil}}}
	if got := lastNonEmptyRow(cr, 100); got != 103 {
		t.Errorf("lastNonEmptyRow = %d, want 103（中间空行不能截断）", got)
	}
	if got := lastNonEmptyRow(&client.CellRange{Values: [][]any{{nil}, {""}}}, 0); got != 0 {
		t.Errorf("全空应返回 0，得到 %d", got)
	}
}

// TestSheetTablePut_AppendStartCellAndGrow 追加模式写到已有数据之后（默认不写表头），--start-cell 决定列，
// 网格不足时先追加行；日期时间列用带时间的 formatter 且序列号保留时间。
func TestSheetTablePut_AppendStartCellAndGrow(t *testing.T) {
	m := newSheetMockServer(t)
	m.sheets = `[{"sheet_id":"s1","title":"Sheet1","index":0,"grid_properties":{"row_count":4,"column_count":5}}]`
	m.respond = func(w http.ResponseWriter, r *http.Request, body []byte) bool {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/values/") {
			// C1:D4 中第 3 行有数据、第 2 行为空 → 最后数据行为 3
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"valueRange":{"range":"s1!C1:D4","values":[["h1","h2"],[null,null],["x",1],[null,null]]}}}`)
			return true
		}
		return false
	}
	payload := `{"sheets":[{"columns":["name","ts"],"data":[["a","2024-01-15T08:30:00"],["b",""]],"dtypes":{"ts":"datetime64[ns]"}}]}`
	out, err := runSheetCmdForTest(t, sheetTablePutCmd, []string{"shtTok", "s1"},
		map[string]string{"sheets": payload, "mode": "append", "start-cell": "C9", "output": "json"})
	if err != nil {
		t.Fatal(err)
	}

	var (
		readPath   string
		addDims    []map[string]any
		styles     []string
		writeRange string
		writeBody  string
	)
	for _, r := range m.requests() {
		switch {
		case r.Method == http.MethodGet:
			readPath, _ = url.PathUnescape(r.RawPath)
		case strings.HasSuffix(r.Path, "/dimension_range"):
			addDims = append(addDims, decodeBody(t, r.Body))
		case strings.HasSuffix(r.Path, "/style"):
			styles = append(styles, string(r.Body))
		case strings.HasSuffix(r.Path, "/values/batch_update"):
			writeBody = string(r.Body)
			var b struct {
				VR []struct {
					Range string `json:"range"`
				} `json:"value_ranges"`
			}
			_ = json.Unmarshal(r.Body, &b)
			writeRange = b.VR[0].Range
		}
	}
	if !strings.HasSuffix(readPath, "/values/s1!C1:D4") {
		t.Errorf("追加定位应读取目标列的整个网格，得到 %s", readPath)
	}
	if writeRange != "s1!C4:D5" {
		t.Errorf("追加写入范围 = %q, want s1!C4:D5（接在第 3 行之后、默认不写表头）", writeRange)
	}
	if len(addDims) != 1 || addDims[0]["dimension"].(map[string]any)["length"] != float64(1) {
		t.Errorf("网格 4 行不足 5 行时应追加 1 行，得到 %v", addDims)
	}
	if !strings.Contains(writeBody, `"value":"45306.354166666664"`) || strings.Contains(writeBody, `"name"`) {
		t.Errorf("写入内容错误（时间丢失或重复表头）: %s", writeBody)
	}
	joined := strings.Join(styles, "\n")
	if !strings.Contains(joined, `yyyy/MM/dd HH:mm:ss`) || !strings.Contains(joined, `s1!D4:D5`) {
		t.Errorf("日期时间列 formatter 错误: %s", joined)
	}
	if !strings.Contains(out, `"mode": "append"`) || !strings.Contains(out, `"range": "s1!C4:D5"`) {
		t.Errorf("输出 = %s", out)
	}
}

// TestSheetFilterCreate_SendsColAndCondition filter create 必须带 col + condition（旧实现只传 range，实测必然 99992402）。
func TestSheetFilterCreate_SendsColAndCondition(t *testing.T) {
	m := newSheetMockServer(t)
	if _, err := runSheetCmdForTest(t, sheetFilterCreateCmd, []string{"shtTok", "s1", "Sheet1!A1:C10"},
		map[string]string{"col": "b", "filter-type": "number", "compare-type": "less", "expected": `["6"]`}); err != nil {
		t.Fatal(err)
	}
	reqs := m.requests()
	if len(reqs) != 1 || reqs[0].Method != http.MethodPost || !strings.HasSuffix(reqs[0].Path, "/sheets/s1/filter") {
		t.Fatalf("请求 = %+v", reqs)
	}
	body := decodeBody(t, reqs[0].Body)
	cond, _ := body["condition"].(map[string]any)
	if body["range"] != "s1!A1:C10" || body["col"] != "B" || cond["filter_type"] != "number" || cond["compare_type"] != "less" {
		t.Errorf("filter create body = %v", body)
	}

	if _, err := runSheetCmdForTest(t, sheetFilterUpdateCmd, []string{"shtTok", "s1"},
		map[string]string{"col": "C", "filter-type": "hiddenValue", "expected": `["x"]`}); err != nil {
		t.Fatal(err)
	}
	reqs = m.requests()
	if last := reqs[len(reqs)-1]; last.Method != http.MethodPut || !strings.HasSuffix(last.Path, "/sheets/s1/filter") {
		t.Errorf("filter update 请求 = %s %s", last.Method, last.Path)
	}
	if _, err := runSheetCmdForTest(t, sheetFilterCreateCmd, []string{"shtTok", "s1", "A1:C10"}, nil); err == nil {
		t.Error("缺少 --col/--filter-type 应报错")
	}
}
