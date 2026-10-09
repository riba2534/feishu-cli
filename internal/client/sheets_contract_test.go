package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSheetsWriteCells_ConvertBooleanToText 验证 WriteCells 写入布尔值转为 "TRUE"/"FALSE" 字符串
func TestSheetsWriteCells_ConvertBooleanToText(t *testing.T) {
	var gotBody map[string]any
	var gotMethod, gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"updatedRange":"Sheet1!A1:B1","updatedRows":1,"updatedColumns":2,"updatedCells":2}}`)
	}))
	defer srv.Close()
	setupTestConfig(t, srv.URL)

	inputValues := [][]any{
		{true, false, "文本", 123},
	}

	res, err := WriteCells(context.Background(), "shtcn_test", "Sheet1!A1:D1", inputValues, "u-test-token")
	if err != nil {
		t.Fatalf("WriteCells error: %v", err)
	}

	if gotMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", gotMethod)
	}
	if gotPath != "/open-apis/sheets/v2/spreadsheets/shtcn_test/values" {
		t.Errorf("path = %s, want /open-apis/sheets/v2/spreadsheets/shtcn_test/values", gotPath)
	}

	valueRange, ok := gotBody["valueRange"].(map[string]any)
	if !ok {
		t.Fatalf("valueRange missing: %v", gotBody)
	}
	values, ok := valueRange["values"].([]any)
	if !ok || len(values) != 1 {
		t.Fatalf("values format error: %v", valueRange)
	}
	row, ok := values[0].([]any)
	if !ok || len(row) != 4 {
		t.Fatalf("row format error: %v", values[0])
	}

	// 关键断言：sheets v2 写入 API 不接受 JSON Boolean（实测 code=90204 invalid cell type），
	// 必须转成 "TRUE" / "FALSE" 字符串（与官方 stringifyCellValue 一致）
	if s, ok := row[0].(string); !ok || s != "TRUE" {
		t.Errorf("row[0] = %v (type %T), want \"TRUE\" (type string)", row[0], row[0])
	}
	if s, ok := row[1].(string); !ok || s != "FALSE" {
		t.Errorf("row[1] = %v (type %T), want \"FALSE\" (type string)", row[1], row[1])
	}
	if s, ok := row[2].(string); !ok || s != "文本" {
		t.Errorf("row[2] = %v, want '文本'", row[2])
	}

	if res.Range != "Sheet1!A1:B1" {
		t.Errorf("res.Range = %s, want Sheet1!A1:B1", res.Range)
	}
}

// TestSheetsAppendCells_ConvertBooleanToText 验证 AppendCells 追加布尔值转为 "TRUE"/"FALSE" 字符串
func TestSheetsAppendCells_ConvertBooleanToText(t *testing.T) {
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"tableRange":"Sheet1!A1:B2","updates":{"updatedRange":"Sheet1!A2:B2"}}}`)
	}))
	defer srv.Close()
	setupTestConfig(t, srv.URL)

	inputValues := [][]any{
		{false, true},
	}

	_, err := AppendCells(context.Background(), "shtcn_test", "Sheet1!A1:B1", inputValues, "OVERWRITE", "u-test-token")
	if err != nil {
		t.Fatalf("AppendCells error: %v", err)
	}

	valueRange, ok := gotBody["valueRange"].(map[string]any)
	if !ok {
		t.Fatalf("valueRange missing: %v", gotBody)
	}
	values, ok := valueRange["values"].([]any)
	if !ok || len(values) != 1 {
		t.Fatalf("values format error: %v", valueRange)
	}
	row, ok := values[0].([]any)
	if !ok || len(row) != 2 {
		t.Fatalf("row format error: %v", values[0])
	}

	if s, ok := row[0].(string); !ok || s != "FALSE" {
		t.Errorf("row[0] = %v (type %T), want \"FALSE\"", row[0], row[0])
	}
	if s, ok := row[1].(string); !ok || s != "TRUE" {
		t.Errorf("row[1] = %v (type %T), want \"TRUE\"", row[1], row[1])
	}
}

// TestSheetsProtect_ParseProtectIDAndValidate 验证保护范围 create/delete 的请求契约与 protectId 解析层级。
// 实测 sheets v2 protected_dimension / protected_range_batch_del 端点在线可用，
// 且 protectId 位于 addProtectedDimension[i] 顶层（不在嵌套 dimension 内）。
func TestSheetsProtect_ParseProtectIDAndValidate(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "protected_range_batch_del") {
			_, _ = io.WriteString(w, `{"code":0,"msg":"success","data":{"delProtectIds":["7678691418626969209"]}}`)
			return
		}
		// protectId 与 dimension 平级，验证解析不会误取嵌套层级
		_, _ = io.WriteString(w, `{"code":0,"msg":"success","data":{"addProtectedDimension":[{"dimension":{"sheetId":"sht1","majorDimension":"ROWS","startIndex":0,"endIndex":5},"protectId":"7678691418626969209"}]}}`)
	}))
	defer srv.Close()
	setupTestConfig(t, srv.URL)

	ids, err := CreateProtectedRange(context.Background(), "shtcn_test", []*ProtectedRange{
		{
			SheetID:  "sht1",
			LockInfo: "lock",
			Dimension: &Dimension{
				SheetID:        "sht1",
				MajorDimension: "ROWS",
				StartIndex:     1,
				EndIndex:       5,
			},
		},
	}, "", "u-test-token")
	if err != nil {
		t.Fatalf("CreateProtectedRange error: %v", err)
	}
	if len(ids) != 1 || ids[0] != "7678691418626969209" {
		t.Errorf("protectIDs = %v, want [7678691418626969209]", ids)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("create method = %s, want POST", gotMethod)
	}
	if gotPath != "/open-apis/sheets/v2/spreadsheets/shtcn_test/protected_dimension" {
		t.Errorf("create path = %s", gotPath)
	}
	if _, ok := gotBody["addProtectedDimension"].([]any); !ok {
		t.Errorf("请求体应含 addProtectedDimension 数组: %v", gotBody)
	}

	// dimension 缺失时前置报错，不得发出无效请求
	if _, err := CreateProtectedRange(context.Background(), "shtcn_test", []*ProtectedRange{
		{SheetID: "sht1"},
	}, "", "u-test-token"); err == nil {
		t.Error("dimension 为 nil 时应报错")
	}

	if err := DeleteProtectedRange(context.Background(), "shtcn_test", []string{"7678691418626969209"}, "u-test-token"); err != nil {
		t.Fatalf("DeleteProtectedRange error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("delete method = %s, want DELETE", gotMethod)
	}
	if gotPath != "/open-apis/sheets/v2/spreadsheets/shtcn_test/protected_range_batch_del" {
		t.Errorf("delete path = %s", gotPath)
	}

	// 空 protectIds 前置报错
	if err := DeleteProtectedRange(context.Background(), "shtcn_test", nil, "u-test-token"); err == nil {
		t.Error("protectIds 为空时应报错")
	}
}
