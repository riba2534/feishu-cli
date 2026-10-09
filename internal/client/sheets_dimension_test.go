package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// sheetsRecorder 记录 httptest 收到的请求（方法、路径、query、原始 body）。
type sheetsRecorder struct {
	mu   sync.Mutex
	reqs []recordedReq
}

type recordedReq struct {
	Method, Path, Query string
	Body                []byte
}

func (r *sheetsRecorder) add(req *http.Request) []byte {
	raw, _ := io.ReadAll(req.Body)
	r.mu.Lock()
	r.reqs = append(r.reqs, recordedReq{Method: req.Method, Path: req.URL.Path, Query: req.URL.RawQuery, Body: raw})
	r.mu.Unlock()
	return raw
}

func (r *sheetsRecorder) last() recordedReq {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reqs[len(r.reqs)-1]
}

func newSheetsServer(t *testing.T, rec *sheetsRecorder, respond func(w http.ResponseWriter, r *http.Request, body []byte)) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "tenant_access_token") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
			return
		}
		body := rec.add(r)
		w.Header().Set("Content-Type", "application/json")
		if respond != nil {
			respond(w, r, body)
			return
		}
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{}}`)
	}))
	t.Cleanup(srv.Close)
	setupTestConfig(t, srv.URL)
}

func dimensionOf(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("解析请求体失败: %v (%s)", err, body)
	}
	d, _ := m["dimension"].(map[string]any)
	return d
}

// TestDeleteDimension_SendsAPIIndexes 删除接口按 1 起始、两端包含发送；0 起始在本地拦截（实测接口返回 90202）。
func TestDeleteDimension_SendsAPIIndexes(t *testing.T) {
	rec := &sheetsRecorder{}
	newSheetsServer(t, rec, nil)

	if err := DeleteDimension(context.Background(), "shtTok", "s1", "ROWS", 3, 5, "u-test"); err != nil {
		t.Fatal(err)
	}
	req := rec.last()
	if req.Method != http.MethodDelete || req.Path != "/open-apis/sheets/v2/spreadsheets/shtTok/dimension_range" {
		t.Fatalf("请求 = %s %s", req.Method, req.Path)
	}
	d := dimensionOf(t, req.Body)
	if d["startIndex"] != float64(3) || d["endIndex"] != float64(5) || d["majorDimension"] != "ROWS" || d["sheetId"] != "s1" {
		t.Errorf("dimension = %v", d)
	}
	n := len(rec.reqs)
	if err := DeleteDimension(context.Background(), "shtTok", "s1", "ROWS", 0, 1, "u-test"); err == nil {
		t.Error("startIndex=0 应在本地报错")
	}
	if err := DeleteDimension(context.Background(), "shtTok", "s1", "ROWS", 5, 3, "u-test"); err == nil {
		t.Error("endIndex<startIndex 应在本地报错")
	}
	if len(rec.reqs) != n {
		t.Error("非法索引不应发出请求")
	}
}

// TestCreateProtectedRange_UsersAndUserIDType editors 改为文档要求的 users + user_id_type。
func TestCreateProtectedRange_UsersAndUserIDType(t *testing.T) {
	rec := &sheetsRecorder{}
	newSheetsServer(t, rec, func(w http.ResponseWriter, r *http.Request, body []byte) {
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"addProtectedDimension":[{"protectId":"p1"}]}}`)
	})
	ids, err := CreateProtectedRange(context.Background(), "shtTok", []*ProtectedRange{{
		SheetID:   "s1",
		Dimension: &Dimension{MajorDimension: "COLUMNS", StartIndex: 1, EndIndex: 3},
		Users:     []string{"ou_xxx"},
		LockInfo:  "lock",
	}}, "", "u-test")
	if err != nil || len(ids) != 1 || ids[0] != "p1" {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
	req := rec.last()
	if req.Query != "user_id_type=open_id" {
		t.Errorf("带 users 时必须传 user_id_type（默认 open_id），query=%q", req.Query)
	}
	var body struct {
		Add []map[string]any `json:"addProtectedDimension"`
	}
	_ = json.Unmarshal(req.Body, &body)
	item := body.Add[0]
	if _, has := item["editors"]; has {
		t.Errorf("不应再发送已废弃且结构错误的 editors: %v", item)
	}
	users, _ := item["users"].([]any)
	if len(users) != 1 || users[0] != "ou_xxx" {
		t.Errorf("users = %v", item["users"])
	}
	dim := item["dimension"].(map[string]any)
	if dim["startIndex"] != float64(1) || dim["endIndex"] != float64(3) || dim["majorDimension"] != "COLUMNS" {
		t.Errorf("dimension = %v", dim)
	}

	// 无 users 时不带 user_id_type
	if _, err := CreateProtectedRange(context.Background(), "shtTok", []*ProtectedRange{{
		SheetID: "s1", Dimension: &Dimension{MajorDimension: "ROWS", StartIndex: 1, EndIndex: 1},
	}}, "union_id", "u-test"); err != nil {
		t.Fatal(err)
	}
	if q := rec.last().Query; q != "" {
		t.Errorf("无 users 时不应带 query，得到 %q", q)
	}
}

// TestWriteCells_PreservesNumberLiterals v2 写入请求体保留数字原始字面量。
func TestWriteCells_PreservesNumberLiterals(t *testing.T) {
	rec := &sheetsRecorder{}
	newSheetsServer(t, rec, func(w http.ResponseWriter, r *http.Request, body []byte) {
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"updatedRange":"s1!A1:C1"}}`)
	})
	values, err := DecodeSheetValues([]byte(`[[1000000, 1234567890123456789, 0.000001]]`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WriteCells(context.Background(), "shtTok", "s1!A1:C1", values, "u-test"); err != nil {
		t.Fatal(err)
	}
	raw := string(rec.last().Body)
	if !strings.Contains(raw, "[[1000000,1234567890123456789,0.000001]]") {
		t.Errorf("请求体数字被改写: %s", raw)
	}
}

// TestReadCells_KeepsNumberPrecision 读取结果数字保留为 json.Number。
func TestReadCells_KeepsNumberPrecision(t *testing.T) {
	rec := &sheetsRecorder{}
	newSheetsServer(t, rec, func(w http.ResponseWriter, r *http.Request, body []byte) {
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"valueRange":{"range":"s1!A1:B1","values":[[12345678901234567890123,1e21]]}}}`)
	})
	cr, err := ReadCells(context.Background(), "shtTok", "s1!A1:B1", "", "", "u-test")
	if err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(cr.Values)
	if string(out) != "[[12345678901234567890123,1e21]]" {
		t.Errorf("读取数字被改写: %s", out)
	}
}

// TestV2APICall_BusinessCodeOnHTTP400 业务错误随 HTTP 400 下发时也要解析出业务码与 log_id。
func TestV2APICall_BusinessCodeOnHTTP400(t *testing.T) {
	rec := &sheetsRecorder{}
	newSheetsServer(t, rec, func(w http.ResponseWriter, r *http.Request, body []byte) {
		w.Header().Set("X-Tt-Logid", "202601010000000000000000000000AAAA")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"code":90215,"msg":"not found sheetId"}`)
	})
	_, err := ReadCells(context.Background(), "shtTok", "Sheet1!A1:B1", "", "", "u-test")
	if err == nil {
		t.Fatal("应返回错误")
	}
	apiErr, ok := AsAPIError(err)
	if !ok || apiErr.Code != 90215 {
		t.Fatalf("应解析为业务错误 90215，得到 %T %v", err, err)
	}
	if !HasAPICode(err, 90215) {
		t.Error("HasAPICode 应命中 90215")
	}
	if strings.Contains(err.Error(), "HTTP 400") {
		t.Errorf("错误不应退化为 HTTP 状态码: %v", err)
	}
}

// TestQuerySheets_ResourceType list-sheets 带出子表类型。
func TestQuerySheets_ResourceType(t *testing.T) {
	rec := &sheetsRecorder{}
	newSheetsServer(t, rec, func(w http.ResponseWriter, r *http.Request, body []byte) {
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"sheets":[{"sheet_id":"s1","title":"Sheet1","index":0,"resource_type":"sheet"},{"sheet_id":"b1","title":"多维表","index":1,"resource_type":"bitable"}]}}`)
	})
	sheets, err := QuerySheets(context.Background(), "shtTok", "u-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets) != 2 || sheets[0].ResourceType != "sheet" || sheets[1].ResourceType != "bitable" {
		t.Errorf("resource_type 解析错误: %+v %+v", sheets[0], sheets[1])
	}
}
