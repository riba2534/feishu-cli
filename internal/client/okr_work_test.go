package client

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestListOKRUserCyclesUsesV2 cycle list 改走 v2 /okr/v2/cycles（用户周期 ID 才能用于 cycle detail）
func TestListOKRUserCyclesUsesV2(t *testing.T) {
	got := captureAPI(t, func(w http.ResponseWriter, r *http.Request, cap *capturedHTTPRequest) {
		if r.URL.Query().Get("page_token") == "" {
			writeJSON(w, http.StatusOK, `{"code":0,"data":{"items":[{"id":"c1","tenant_cycle_id":"p1","owner":{"owner_type":"user","user_id":"ou_a"},
				"start_time":"1767196800000","end_time":"1782835199000","cycle_status":1}],"has_more":true,"page_token":"n2"}}`)
			return
		}
		writeJSON(w, http.StatusOK, `{"code":0,"data":{"items":[{"id":"c2","start_time":"1735660800000","end_time":"1751299199000","cycle_status":3}],"has_more":false}}`)
	})
	cycles, err := ListOKRUserCycles(ListOKRUserCyclesOptions{UserID: "ou_a"}, "")
	if err != nil {
		t.Fatalf("ListOKRUserCycles: %v", err)
	}
	reqs := got()
	if len(reqs) != 2 || reqs[0].Path != "/open-apis/okr/v2/cycles" || reqs[0].Query.Get("user_id") != "ou_a" ||
		reqs[0].Query.Get("user_id_type") != "open_id" || reqs[1].Query.Get("page_token") != "n2" {
		t.Fatalf("请求错误: %+v", reqs)
	}
	if !strings.HasPrefix(reqs[0].Auth, "Bearer t-") {
		t.Fatalf("未传 User Token 时应走 Tenant Token: %s", reqs[0].Auth)
	}
	if len(cycles) != 2 || cycles[0].ID != "c1" || cycles[0].TenantCycleID != "p1" || cycles[0].CycleStatus != "normal" || cycles[1].CycleStatus != "hidden" {
		t.Fatalf("解析错误: %+v %+v", cycles[0], cycles[1])
	}
	inH1 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if !OKRCycleOverlaps(cycles[0], inH1, inH1.AddDate(0, 1, 0)) || OKRCycleOverlaps(cycles[1], inH1, inH1.AddDate(0, 1, 0)) {
		t.Fatal("时间范围过滤错误")
	}
	if !IsCurrentOKRCycle(cycles[0], inH1) || IsCurrentOKRCycle(cycles[1], time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("当前周期判断错误（hidden 状态不算进行中）")
	}
}

// TestOKRBusinessErrorOn400 OKR 缺 scope 等业务错误随 400 下发时按业务码返回
func TestOKRBusinessErrorOn400(t *testing.T) {
	captureAPI(t, func(w http.ResponseWriter, r *http.Request, cap *capturedHTTPRequest) {
		writeJSON(w, http.StatusBadRequest, `{"code":99991672,"msg":"Access denied"}`)
	})
	_, err := ListOKRCycles(ListOKRCyclesOptions{}, "")
	if apiErr, ok := AsAPIError(err); !ok || apiErr.Code != 99991672 {
		t.Fatalf("期望 APIError 99991672，得到 %v", err)
	}
}

func TestOKRV2ContentAndBuilders(t *testing.T) {
	c := OKRV2TextContent("第一行\n第二行")
	blocks := c["blocks"].([]any)
	first := blocks[0].(map[string]any)
	if len(blocks) != 2 || first["block_element_type"] != "paragraph" {
		t.Fatalf("v2 ContentBlock 应使用 snake_case 键: %+v", c)
	}
	if _, err := ParseOKRV2Content("a", `{"blocks":[]}`, "content"); err == nil {
		t.Fatal("--content 与 --content-json 同时传应报错")
	}
	if _, err := ParseOKRV2Content("", `{"x":1}`, "content"); err == nil {
		t.Fatal("缺 blocks 的 JSON 应报错")
	}
	req, err := BuildOKRCreateObjective("c1", c, nil, "", "")
	if err != nil || req.Path != "/open-apis/okr/v2/cycles/c1/objectives" || req.Query["user_id_type"] != "open_id" {
		t.Fatalf("create objective: %+v %v", req, err)
	}
	if _, err := BuildOKRCreateObjective("", c, nil, "", ""); err == nil {
		t.Fatal("缺 cycle_id 应报错")
	}
	for _, s := range []float64{-0.1, 1.1, 0.75} {
		s := s
		if _, err := BuildOKRPatch("objective", "o1", OKRPatchFields{Score: &s}, ""); err == nil {
			t.Fatalf("score %v 应报错", s)
		}
	}
	ok := 0.5
	req, err = BuildOKRPatch("key-result", "k1", OKRPatchFields{Score: &ok}, "")
	if err != nil || req.Method != "PATCH" || req.Path != "/open-apis/okr/v2/key_results/k1" || req.Body["score"] != 0.5 {
		t.Fatalf("patch kr: %+v %v", req, err)
	}
	if _, err := BuildOKRPatch("key-result", "k1", OKRPatchFields{Notes: c}, ""); err == nil {
		t.Fatal("关键结果不支持 notes")
	}
	sec := int64(1798732799)
	if _, err := BuildOKRPatch("objective", "o1", OKRPatchFields{Deadline: &sec}, ""); err == nil {
		t.Fatal("秒级时间戳应报错")
	}
	if _, err := BuildOKRCommentCreate(OKRCommentCreate{TargetType: "objective", TargetID: "o1", Content: c}, ""); err == nil {
		t.Fatal("目标评论缺选区应报错")
	}
	req, err = BuildOKRCommentCreate(OKRCommentCreate{TargetType: "objective", TargetID: "o1", Content: OKRV2TextContent("评论"), SelectAll: true}, "")
	if err != nil || req.Body["selected_text"] != "**" {
		t.Fatalf("select-all 应生成通配选区: %+v %v", req, err)
	}
	// --content-json 传入的 ContentBlock 同样按结构取文本（含 mention），不能得到空选区
	raw, err := ParseOKRV2Content("", `{"blocks":[{"block_element_type":"paragraph","paragraph":{"elements":[`+
		`{"paragraph_element_type":"textRun","text_run":{"text":"看下"}},`+
		`{"paragraph_element_type":"mention","mention":{"user_id":"ou_x"}}]}},`+
		`{"block_element_type":"paragraph","paragraph":{"elements":[{"paragraph_element_type":"textRun","text_run":{"text":"口径"}}]}}]}`, "content")
	if err != nil {
		t.Fatal(err)
	}
	req, err = BuildOKRCommentCreate(OKRCommentCreate{TargetType: "objective", TargetID: "o1", Content: raw, SelectAll: true}, "")
	if want := strings.Repeat("*", len([]rune("看下 @{ou_x} 口径"))); err != nil || req.Body["selected_text"] != want {
		t.Fatalf("--content-json + select-all 应按结构生成通配选区 %q: %+v %v", want, req.Body["selected_text"], err)
	}
	if _, err := BuildOKRCommentCreate(OKRCommentCreate{TargetType: "cycle", TargetID: "c1", Content: c, SelectedText: "x"}, ""); err == nil {
		t.Fatal("周期评论不支持选区")
	}
}

func TestDoOKRWriteSendsRequest(t *testing.T) {
	got := captureAPI(t, func(w http.ResponseWriter, r *http.Request, cap *capturedHTTPRequest) {
		writeJSON(w, http.StatusOK, `{"code":0,"data":{"objective_id":"o9"}}`)
	})
	req, _ := BuildOKRCreateObjective("c1", OKRV2TextContent("x"), nil, "cat1", "")
	data, err := DoOKRWrite(req, "创建 OKR 目标", testUserToken)
	if err != nil || !strings.Contains(string(data), "o9") {
		t.Fatalf("DoOKRWrite: %s %v", data, err)
	}
	r := got()[0]
	if r.Method != http.MethodPost || r.Path != "/open-apis/okr/v2/cycles/c1/objectives" || r.Query.Get("user_id_type") != "open_id" ||
		!strings.Contains(string(r.Body), `"category_id":"cat1"`) {
		t.Fatalf("请求错误: %+v %s", r, r.Body)
	}
}
