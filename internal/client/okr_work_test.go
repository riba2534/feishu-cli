package client

import (
	"net/http"
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
	if reqs[0].Auth != "Bearer t-fake" {
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
