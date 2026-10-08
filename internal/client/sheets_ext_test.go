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

// 说明：以下测试全部用 httptest mock 飞书 API，复用 setupTestConfig 注入 base_url；
// 所有调用都显式传 user token（"u-test"），从而跳过 tenant_access_token 自动获取。
// V3 SDK 调用会把 token/sheet_id 等路径参数填充到 URL，因此可直接断言 r.URL.Path。

// ---------- 小工具 ----------

// captureServer 启动一个 httptest 服务器，记录最近一次请求的方法/路径/原始 body，并返回固定 JSON。
func captureServer(t *testing.T, respJSON string, method, path *string, body *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			t.Fatalf("显式 User Token 时不应请求 tenant token，path=%s", r.URL.Path)
		}
		if method != nil {
			*method = r.Method
		}
		if path != nil {
			*path = r.URL.Path
		}
		if body != nil {
			raw, _ := io.ReadAll(r.Body)
			*body = string(raw)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, respJSON)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// ==================== GetFilterView ====================

func TestGetFilterView_HappyPath(t *testing.T) {
	var gotMethod, gotPath string
	srv := captureServer(t,
		`{"code":0,"msg":"ok","data":{"filter_view":{"filter_view_id":"fv1","filter_view_name":"我的视图","range":"sht1!A1:D100"}}}`,
		&gotMethod, &gotPath, nil)
	setupTestConfig(t, srv.URL)

	out, err := GetFilterView(context.Background(), "shtcn1", "sht1", "fv1", "u-test")
	if err != nil {
		t.Fatalf("GetFilterView error: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %s, want GET", gotMethod)
	}
	wantPath := "/open-apis/sheets/v3/spreadsheets/shtcn1/sheets/sht1/filter_views/fv1"
	if gotPath != wantPath {
		t.Errorf("path = %s, want %s", gotPath, wantPath)
	}
	if out.FilterViewID != "fv1" || out.FilterViewName != "我的视图" || out.Range != "sht1!A1:D100" {
		t.Errorf("解析结果不符: %+v", out)
	}
}

func TestGetFilterView_APIErrorCode(t *testing.T) {
	srv := captureServer(t, `{"code":1310214,"msg":"filter view not found","data":{}}`, nil, nil, nil)
	setupTestConfig(t, srv.URL)

	_, err := GetFilterView(context.Background(), "shtcn1", "sht1", "fv_missing", "u-test")
	if err == nil {
		t.Fatal("API code!=0 应返回错误")
	}
	if !strings.Contains(err.Error(), "1310214") {
		t.Errorf("错误应携带 code，got: %v", err)
	}
}

func TestGetFilterView_HTTPNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"code":0}`)
	}))
	defer srv.Close()
	setupTestConfig(t, srv.URL)

	_, err := GetFilterView(context.Background(), "shtcn1", "sht1", "fv1", "u-test")
	if err == nil {
		t.Fatal("HTTP 500 应返回错误")
	}
}

// ==================== UpdateFilterView ====================

func TestUpdateFilterView_HappyPath(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := captureServer(t,
		`{"code":0,"msg":"ok","data":{"filter_view":{"filter_view_id":"fv1","filter_view_name":"新名字","range":"sht1!A1:E50"}}}`,
		&gotMethod, &gotPath, &gotBody)
	setupTestConfig(t, srv.URL)

	out, err := UpdateFilterView(context.Background(), "shtcn1", "sht1", "fv1", "新名字", "sht1!A1:E50", "u-test")
	if err != nil {
		t.Fatalf("UpdateFilterView error: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", gotMethod)
	}
	wantPath := "/open-apis/sheets/v3/spreadsheets/shtcn1/sheets/sht1/filter_views/fv1"
	if gotPath != wantPath {
		t.Errorf("path = %s, want %s", gotPath, wantPath)
	}
	// name / range 均非空，body 应同时携带。
	if !strings.Contains(gotBody, `"filter_view_name":"新名字"`) {
		t.Errorf("body 缺 filter_view_name: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"range":"sht1!A1:E50"`) {
		t.Errorf("body 缺 range: %s", gotBody)
	}
	if out.FilterViewID != "fv1" || out.FilterViewName != "新名字" {
		t.Errorf("解析结果不符: %+v", out)
	}
}

func TestUpdateFilterView_EmptyFieldsOmitted(t *testing.T) {
	var gotBody string
	srv := captureServer(t,
		`{"code":0,"msg":"ok","data":{"filter_view":{"filter_view_id":"fv1"}}}`,
		nil, nil, &gotBody)
	setupTestConfig(t, srv.URL)

	// 仅改 range，name 留空 -> body 不应出现 filter_view_name。
	_, err := UpdateFilterView(context.Background(), "shtcn1", "sht1", "fv1", "", "sht1!A1:B2", "u-test")
	if err != nil {
		t.Fatalf("UpdateFilterView error: %v", err)
	}
	if strings.Contains(gotBody, "filter_view_name") {
		t.Errorf("name 为空不应写入 body: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"range":"sht1!A1:B2"`) {
		t.Errorf("range 非空应写入 body: %s", gotBody)
	}
}

func TestUpdateFilterView_APIErrorCode(t *testing.T) {
	srv := captureServer(t, `{"code":1310215,"msg":"bad range","data":{}}`, nil, nil, nil)
	setupTestConfig(t, srv.URL)

	_, err := UpdateFilterView(context.Background(), "shtcn1", "sht1", "fv1", "x", "y", "u-test")
	if err == nil || !strings.Contains(err.Error(), "1310215") {
		t.Fatalf("API code!=0 应返回携带 code 的错误，got: %v", err)
	}
}

// ==================== CreateFilterViewCondition ====================

func TestCreateFilterViewCondition_HappyPath(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := captureServer(t,
		`{"code":0,"msg":"ok","data":{"condition":{"condition_id":"C","filter_type":"number","compare_type":"greater","expected":["10"]}}}`,
		&gotMethod, &gotPath, &gotBody)
	setupTestConfig(t, srv.URL)

	out, err := CreateFilterViewCondition(context.Background(), "shtcn1", "sht1", "fv1",
		"C", "number", "greater", []string{"10"}, "u-test")
	if err != nil {
		t.Fatalf("CreateFilterViewCondition error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	wantPath := "/open-apis/sheets/v3/spreadsheets/shtcn1/sheets/sht1/filter_views/fv1/conditions"
	if gotPath != wantPath {
		t.Errorf("path = %s, want %s", gotPath, wantPath)
	}
	// 创建时 condition_id 应随 body 提交。
	if !strings.Contains(gotBody, `"condition_id":"C"`) {
		t.Errorf("创建 body 应携带 condition_id: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"filter_type":"number"`) || !strings.Contains(gotBody, `"compare_type":"greater"`) {
		t.Errorf("body 缺 filter_type/compare_type: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"expected":["10"]`) {
		t.Errorf("body 缺 expected: %s", gotBody)
	}
	if out.ConditionID != "C" || out.FilterType != "number" || out.CompareType != "greater" || len(out.Expected) != 1 || out.Expected[0] != "10" {
		t.Errorf("解析结果不符: %+v", out)
	}
}

func TestCreateFilterViewCondition_APIErrorCode(t *testing.T) {
	srv := captureServer(t, `{"code":1310216,"msg":"invalid condition","data":{}}`, nil, nil, nil)
	setupTestConfig(t, srv.URL)

	_, err := CreateFilterViewCondition(context.Background(), "shtcn1", "sht1", "fv1",
		"C", "number", "greater", []string{"10"}, "u-test")
	if err == nil || !strings.Contains(err.Error(), "1310216") {
		t.Fatalf("API code!=0 应返回携带 code 的错误，got: %v", err)
	}
}

// ==================== GetFilterViewCondition ====================

func TestGetFilterViewCondition_HappyPath(t *testing.T) {
	var gotMethod, gotPath string
	srv := captureServer(t,
		`{"code":0,"msg":"ok","data":{"condition":{"condition_id":"C","filter_type":"text","compare_type":"equal","expected":["foo","bar"]}}}`,
		&gotMethod, &gotPath, nil)
	setupTestConfig(t, srv.URL)

	out, err := GetFilterViewCondition(context.Background(), "shtcn1", "sht1", "fv1", "C", "u-test")
	if err != nil {
		t.Fatalf("GetFilterViewCondition error: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %s, want GET", gotMethod)
	}
	wantPath := "/open-apis/sheets/v3/spreadsheets/shtcn1/sheets/sht1/filter_views/fv1/conditions/C"
	if gotPath != wantPath {
		t.Errorf("path = %s, want %s", gotPath, wantPath)
	}
	if out.ConditionID != "C" || out.FilterType != "text" || out.CompareType != "equal" {
		t.Errorf("解析结果不符: %+v", out)
	}
	if len(out.Expected) != 2 || out.Expected[0] != "foo" || out.Expected[1] != "bar" {
		t.Errorf("expected 解析不符: %+v", out.Expected)
	}
}

func TestGetFilterViewCondition_APIErrorCode(t *testing.T) {
	srv := captureServer(t, `{"code":1310217,"msg":"condition not found","data":{}}`, nil, nil, nil)
	setupTestConfig(t, srv.URL)

	_, err := GetFilterViewCondition(context.Background(), "shtcn1", "sht1", "fv1", "C", "u-test")
	if err == nil || !strings.Contains(err.Error(), "1310217") {
		t.Fatalf("API code!=0 应返回携带 code 的错误，got: %v", err)
	}
}

// ==================== UpdateFilterViewCondition ====================

func TestUpdateFilterViewCondition_HappyPath(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := captureServer(t,
		`{"code":0,"msg":"ok","data":{"condition":{"condition_id":"C","filter_type":"number","compare_type":"less","expected":["5"]}}}`,
		&gotMethod, &gotPath, &gotBody)
	setupTestConfig(t, srv.URL)

	out, err := UpdateFilterViewCondition(context.Background(), "shtcn1", "sht1", "fv1", "C",
		"number", "less", []string{"5"}, "u-test")
	if err != nil {
		t.Fatalf("UpdateFilterViewCondition error: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", gotMethod)
	}
	wantPath := "/open-apis/sheets/v3/spreadsheets/shtcn1/sheets/sht1/filter_views/fv1/conditions/C"
	if gotPath != wantPath {
		t.Errorf("path = %s, want %s", gotPath, wantPath)
	}
	// 更新时 condition_id 由 URL 路径携带，body 不应再含 condition_id。
	if strings.Contains(gotBody, "condition_id") {
		t.Errorf("更新 body 不应携带 condition_id（由路径携带）: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"filter_type":"number"`) || !strings.Contains(gotBody, `"compare_type":"less"`) {
		t.Errorf("body 缺 filter_type/compare_type: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"expected":["5"]`) {
		t.Errorf("body 缺 expected: %s", gotBody)
	}
	if out.ConditionID != "C" || out.CompareType != "less" {
		t.Errorf("解析结果不符: %+v", out)
	}
}

func TestUpdateFilterViewCondition_APIErrorCode(t *testing.T) {
	srv := captureServer(t, `{"code":1310218,"msg":"bad","data":{}}`, nil, nil, nil)
	setupTestConfig(t, srv.URL)

	_, err := UpdateFilterViewCondition(context.Background(), "shtcn1", "sht1", "fv1", "C",
		"number", "less", []string{"5"}, "u-test")
	if err == nil || !strings.Contains(err.Error(), "1310218") {
		t.Fatalf("API code!=0 应返回携带 code 的错误，got: %v", err)
	}
}

// ==================== DeleteFilterViewCondition ====================

func TestDeleteFilterViewCondition_HappyPath(t *testing.T) {
	var gotMethod, gotPath string
	srv := captureServer(t, `{"code":0,"msg":"ok"}`, &gotMethod, &gotPath, nil)
	setupTestConfig(t, srv.URL)

	err := DeleteFilterViewCondition(context.Background(), "shtcn1", "sht1", "fv1", "C", "u-test")
	if err != nil {
		t.Fatalf("DeleteFilterViewCondition error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", gotMethod)
	}
	wantPath := "/open-apis/sheets/v3/spreadsheets/shtcn1/sheets/sht1/filter_views/fv1/conditions/C"
	if gotPath != wantPath {
		t.Errorf("path = %s, want %s", gotPath, wantPath)
	}
}

func TestDeleteFilterViewCondition_APIErrorCode(t *testing.T) {
	srv := captureServer(t, `{"code":1310219,"msg":"cannot delete","data":{}}`, nil, nil, nil)
	setupTestConfig(t, srv.URL)

	err := DeleteFilterViewCondition(context.Background(), "shtcn1", "sht1", "fv1", "C", "u-test")
	if err == nil || !strings.Contains(err.Error(), "1310219") {
		t.Fatalf("API code!=0 应返回携带 code 的错误，got: %v", err)
	}
}

// ==================== ListFilterViewConditions ====================

func TestListFilterViewConditions_HappyPath(t *testing.T) {
	var gotMethod, gotPath string
	srv := captureServer(t,
		`{"code":0,"msg":"ok","data":{"items":[`+
			`{"condition_id":"C1","filter_type":"text","compare_type":"equal","expected":["a"]},`+
			`{"condition_id":"C2","filter_type":"number","compare_type":"greater","expected":["1","2"]}]}}`,
		&gotMethod, &gotPath, nil)
	setupTestConfig(t, srv.URL)

	out, err := ListFilterViewConditions(context.Background(), "shtcn1", "sht1", "fv1", "u-test")
	if err != nil {
		t.Fatalf("ListFilterViewConditions error: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %s, want GET", gotMethod)
	}
	wantPath := "/open-apis/sheets/v3/spreadsheets/shtcn1/sheets/sht1/filter_views/fv1/conditions/query"
	if gotPath != wantPath {
		t.Errorf("path = %s, want %s", gotPath, wantPath)
	}
	if len(out) != 2 {
		t.Fatalf("应解析 2 个条件，got %d: %+v", len(out), out)
	}
	if out[0].ConditionID != "C1" || out[0].FilterType != "text" {
		t.Errorf("第 1 个条件解析不符: %+v", out[0])
	}
	if out[1].ConditionID != "C2" || len(out[1].Expected) != 2 || out[1].Expected[1] != "2" {
		t.Errorf("第 2 个条件解析不符: %+v", out[1])
	}
}

func TestListFilterViewConditions_Empty(t *testing.T) {
	srv := captureServer(t, `{"code":0,"msg":"ok","data":{"items":[]}}`, nil, nil, nil)
	setupTestConfig(t, srv.URL)

	out, err := ListFilterViewConditions(context.Background(), "shtcn1", "sht1", "fv1", "u-test")
	if err != nil {
		t.Fatalf("ListFilterViewConditions error: %v", err)
	}
	if out == nil {
		t.Fatal("空列表应返回非 nil 切片")
	}
	if len(out) != 0 {
		t.Errorf("应返回空切片，got %d", len(out))
	}
}

func TestListFilterViewConditions_APIErrorCode(t *testing.T) {
	srv := captureServer(t, `{"code":1310220,"msg":"query failed","data":{}}`, nil, nil, nil)
	setupTestConfig(t, srv.URL)

	_, err := ListFilterViewConditions(context.Background(), "shtcn1", "sht1", "fv1", "u-test")
	if err == nil || !strings.Contains(err.Error(), "1310220") {
		t.Fatalf("API code!=0 应返回携带 code 的错误，got: %v", err)
	}
}

// ==================== GetDropdown (V2) ====================

func TestGetDropdown_HappyPath(t *testing.T) {
	var gotMethod, gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/open-apis/auth/v3/tenant_access_token/internal") {
			t.Fatalf("显式 User Token 时不应请求 tenant token")
		}
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"revision":7,"dataValidations":[{"dataValidationType":"list"}]}}`)
	}))
	defer srv.Close()
	setupTestConfig(t, srv.URL)

	data, err := GetDropdown(context.Background(), "shtcn1", "sht1!A1:A100", "u-test")
	if err != nil {
		t.Fatalf("GetDropdown error: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %s, want GET", gotMethod)
	}
	wantPath := "/open-apis/sheets/v2/spreadsheets/shtcn1/dataValidation"
	if gotPath != wantPath {
		t.Errorf("path = %s, want %s", gotPath, wantPath)
	}
	// query 应携带 range 和 dataValidationType=list。
	if !strings.Contains(gotQuery, "dataValidationType=list") {
		t.Errorf("query 缺 dataValidationType=list: %s", gotQuery)
	}
	if !strings.Contains(gotQuery, "range=sht1") {
		t.Errorf("query 缺 range: %s", gotQuery)
	}
	if data == nil {
		t.Fatal("data 不应为 nil")
	}
	if v, ok := data["revision"]; !ok {
		t.Errorf("data 应解析出 revision 字段: %+v", data)
	} else if f, ok := v.(float64); !ok || f != 7 {
		t.Errorf("revision 解析不符: %v", v)
	}
}

func TestGetDropdown_MissingSheetPrefix(t *testing.T) {
	// range 不含 "!" 前缀时直接报错，且不应发起任何 HTTP 请求。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("range 校验失败时不应发起 HTTP 请求，path=%s", r.URL.Path)
	}))
	defer srv.Close()
	setupTestConfig(t, srv.URL)

	_, err := GetDropdown(context.Background(), "shtcn1", "A1:A100", "u-test")
	if err == nil {
		t.Fatal("range 缺 sheetId 前缀应报错")
	}
	if !strings.Contains(err.Error(), "sheetId") {
		t.Errorf("错误应提示 sheetId 前缀，got: %v", err)
	}
}

func TestGetDropdown_APIErrorCode(t *testing.T) {
	srv := captureServer(t, `{"code":90205,"msg":"range error","data":{}}`, nil, nil, nil)
	setupTestConfig(t, srv.URL)

	_, err := GetDropdown(context.Background(), "shtcn1", "sht1!A1:A100", "u-test")
	if err == nil || !strings.Contains(err.Error(), "90205") {
		t.Fatalf("API code!=0 应返回携带 code 的错误，got: %v", err)
	}
}

// ==================== UpdateDropdown (V2) ====================

func TestUpdateDropdown_HappyPath(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := captureServer(t, `{"code":0,"msg":"ok"}`, &gotMethod, &gotPath, &gotBody)
	setupTestConfig(t, srv.URL)

	err := UpdateDropdown(context.Background(), "shtcn1", "sht1",
		[]string{"sht1!A1:A10"}, []string{"待办", "进行中", "完成"},
		true, nil, false, "u-test")
	if err != nil {
		t.Fatalf("UpdateDropdown error: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", gotMethod)
	}
	wantPath := "/open-apis/sheets/v2/spreadsheets/shtcn1/dataValidation/sht1"
	if gotPath != wantPath {
		t.Errorf("path = %s, want %s", gotPath, wantPath)
	}

	// 校验 body 结构：dataValidationType=list、multipleValues=true、conditionValues 含全部选项。
	var body map[string]any
	if err := json.Unmarshal([]byte(gotBody), &body); err != nil {
		t.Fatalf("body 不是合法 JSON: %v (%s)", err, gotBody)
	}
	if body["dataValidationType"] != "list" {
		t.Errorf("dataValidationType = %v, want list", body["dataValidationType"])
	}
	ranges, _ := body["ranges"].([]any)
	if len(ranges) != 1 || ranges[0] != "sht1!A1:A10" {
		t.Errorf("ranges 不符: %v", body["ranges"])
	}
	dv, _ := body["dataValidation"].(map[string]any)
	if dv == nil {
		t.Fatalf("dataValidation 缺失: %s", gotBody)
	}
	cvs, _ := dv["conditionValues"].([]any)
	if len(cvs) != 3 || cvs[0] != "待办" || cvs[2] != "完成" {
		t.Errorf("conditionValues 不符: %v", dv["conditionValues"])
	}
	opts, _ := dv["options"].(map[string]any)
	if opts == nil || opts["multipleValues"] != true {
		t.Errorf("options.multipleValues 应为 true: %v", dv["options"])
	}
	// 未传 colors / highlight，不应出现上色字段。
	if _, ok := opts["colors"]; ok {
		t.Errorf("未传 colors 不应出现 colors 字段: %v", opts)
	}
	if _, ok := opts["highlightValidData"]; ok {
		t.Errorf("未传 colors/highlight 不应出现 highlightValidData: %v", opts)
	}
}

func TestUpdateDropdown_WithColors(t *testing.T) {
	var gotBody string
	srv := captureServer(t, `{"code":0,"msg":"ok"}`, nil, nil, &gotBody)
	setupTestConfig(t, srv.URL)

	err := UpdateDropdown(context.Background(), "shtcn1", "sht1",
		[]string{"sht1!A1:A10"}, []string{"高", "低"},
		false, []string{"#FF0000", "#00FF00"}, false, "u-test")
	if err != nil {
		t.Fatalf("UpdateDropdown error: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(gotBody), &body); err != nil {
		t.Fatalf("body 不是合法 JSON: %v", err)
	}
	dv, _ := body["dataValidation"].(map[string]any)
	opts, _ := dv["options"].(map[string]any)
	colors, _ := opts["colors"].([]any)
	if len(colors) != 2 || colors[0] != "#FF0000" || colors[1] != "#00FF00" {
		t.Errorf("colors 不符: %v", opts["colors"])
	}
	// 传 colors 时自动视为高亮。
	if opts["highlightValidData"] != true {
		t.Errorf("传 colors 应自动开启 highlightValidData: %v", opts)
	}
}

func TestUpdateDropdown_EmptyRanges(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("校验失败时不应发起 HTTP 请求，path=%s", r.URL.Path)
	}))
	defer srv.Close()
	setupTestConfig(t, srv.URL)

	err := UpdateDropdown(context.Background(), "shtcn1", "sht1",
		nil, []string{"a"}, false, nil, false, "u-test")
	if err == nil || !strings.Contains(err.Error(), "ranges") {
		t.Fatalf("空 ranges 应报错并提示 ranges，got: %v", err)
	}
}

func TestUpdateDropdown_EmptyOptions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("校验失败时不应发起 HTTP 请求，path=%s", r.URL.Path)
	}))
	defer srv.Close()
	setupTestConfig(t, srv.URL)

	err := UpdateDropdown(context.Background(), "shtcn1", "sht1",
		[]string{"sht1!A1:A10"}, nil, false, nil, false, "u-test")
	if err == nil || !strings.Contains(err.Error(), "选项") {
		t.Fatalf("空选项应报错，got: %v", err)
	}
}

func TestUpdateDropdown_ColorsLengthMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("校验失败时不应发起 HTTP 请求，path=%s", r.URL.Path)
	}))
	defer srv.Close()
	setupTestConfig(t, srv.URL)

	// 2 个选项配 1 个颜色，长度不一致应报错。
	err := UpdateDropdown(context.Background(), "shtcn1", "sht1",
		[]string{"sht1!A1:A10"}, []string{"a", "b"}, false, []string{"#FF0000"}, false, "u-test")
	if err == nil || !strings.Contains(err.Error(), "colors") {
		t.Fatalf("colors 长度不一致应报错，got: %v", err)
	}
}

func TestUpdateDropdown_APIErrorCode(t *testing.T) {
	srv := captureServer(t, `{"code":90203,"msg":"invalid","data":{}}`, nil, nil, nil)
	setupTestConfig(t, srv.URL)

	err := UpdateDropdown(context.Background(), "shtcn1", "sht1",
		[]string{"sht1!A1:A10"}, []string{"a"}, false, nil, false, "u-test")
	if err == nil || !strings.Contains(err.Error(), "90203") {
		t.Fatalf("API code!=0 应返回携带 code 的错误，got: %v", err)
	}
}

// ==================== DeleteDropdown (V2) ====================

func TestDeleteDropdown_HappyPath(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := captureServer(t, `{"code":0,"msg":"ok"}`, &gotMethod, &gotPath, &gotBody)
	setupTestConfig(t, srv.URL)

	err := DeleteDropdown(context.Background(), "shtcn1",
		[]string{"sht1!A1:A10", "sht1!B1:B10"}, "u-test")
	if err != nil {
		t.Fatalf("DeleteDropdown error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", gotMethod)
	}
	wantPath := "/open-apis/sheets/v2/spreadsheets/shtcn1/dataValidation"
	if gotPath != wantPath {
		t.Errorf("path = %s, want %s", gotPath, wantPath)
	}
	// body 应携带 dataValidationRanges，每项为 {"range": ...}。
	var body map[string]any
	if err := json.Unmarshal([]byte(gotBody), &body); err != nil {
		t.Fatalf("body 不是合法 JSON: %v (%s)", err, gotBody)
	}
	dvr, _ := body["dataValidationRanges"].([]any)
	if len(dvr) != 2 {
		t.Fatalf("dataValidationRanges 应有 2 项: %v", body["dataValidationRanges"])
	}
	first, _ := dvr[0].(map[string]any)
	if first == nil || first["range"] != "sht1!A1:A10" {
		t.Errorf("第 1 个 range 不符: %v", dvr[0])
	}
}

func TestDeleteDropdown_EmptyRanges(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("校验失败时不应发起 HTTP 请求，path=%s", r.URL.Path)
	}))
	defer srv.Close()
	setupTestConfig(t, srv.URL)

	err := DeleteDropdown(context.Background(), "shtcn1", nil, "u-test")
	if err == nil || !strings.Contains(err.Error(), "ranges") {
		t.Fatalf("空 ranges 应报错，got: %v", err)
	}
}

func TestDeleteDropdown_APIErrorCode(t *testing.T) {
	srv := captureServer(t, `{"code":90204,"msg":"delete failed","data":{}}`, nil, nil, nil)
	setupTestConfig(t, srv.URL)

	err := DeleteDropdown(context.Background(), "shtcn1", []string{"sht1!A1:A10"}, "u-test")
	if err == nil || !strings.Contains(err.Error(), "90204") {
		t.Fatalf("API code!=0 应返回携带 code 的错误，got: %v", err)
	}
}

// TestSheetMediaParentType 覆盖上传图片到电子表格时的 parent_type 选择：
// 原生飞书表格用 sheet_image；导入型 office 表格（IsLocalOfficeToken 判定）用
// office_sheet_file。
func TestSheetMediaParentType(t *testing.T) {
	cases := []struct {
		name  string
		token string
		want  string
	}{
		{"原生表格 token", "shtcnABC123", "sheet_image"},
		{"导入 office 表格 token", "fake_office_abc123", "office_sheet_file"},
		{"仅 fake_office_ 前缀本身", "fake_office_", "office_sheet_file"},
		{"前缀出现在中间不匹配", "shtfake_office_abc", "sheet_image"},
		{"local_office_ 前缀", "local_office_abc123", "office_sheet_file"},
		{"27 字符交织 OFL0X 导入表格", "aaaaOaaaaFaaaaLaaaa0aaaaXaa", "office_sheet_file"},
		{"同长度原生 shtcn 交织 token", "abcdsefghhijkltmnopcqrstnuv", "sheet_image"},
		{"空 token 回落 sheet_image", "", "sheet_image"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sheetMediaParentType(c.token); got != c.want {
				t.Errorf("sheetMediaParentType(%q) = %q, want %q", c.token, got, c.want)
			}
		})
	}
}
