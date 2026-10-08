package cmd

import (
	"net/http"
	"strings"
	"testing"
)

// TestOKRCycleListDefaultsToCurrentUserV2 默认查当前登录用户的 v2 用户周期（此前走 v1 租户周期，ID 与 cycle detail 不通）
func TestOKRCycleListDefaultsToCurrentUserV2(t *testing.T) {
	rec := setupWorkCmdTest(t, "u-test", func(w http.ResponseWriter, r *http.Request, body string) {
		switch r.URL.Path {
		case "/open-apis/authen/v1/user_info":
			_, _ = w.Write([]byte(`{"code":0,"data":{"open_id":"ou_me"}}`))
		case "/open-apis/okr/v2/cycles":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":"c1","start_time":"1767196800000","end_time":"1782835199000","cycle_status":1}],"has_more":false}}`))
		default:
			http.Error(w, `{"code":404}`, http.StatusNotFound)
		}
	})
	out, err := runWorkCmd(t, okrCycleListCmd, nil, map[string]string{"output": "json"})
	if err != nil {
		t.Fatalf("okr cycle list: %v", err)
	}
	if !strings.Contains(out, `"id": "c1"`) || !strings.Contains(out, `"user_id": "ou_me"`) {
		t.Fatalf("输出错误: %s", out)
	}
	var cycleReq *workReq
	for _, q := range rec.apiReqs() {
		if q.Path == "/open-apis/okr/v2/cycles" {
			q := q
			cycleReq = &q
		}
		if strings.Contains(q.Path, "/okr/v1/periods") {
			t.Fatalf("默认不应再查 v1 periods: %+v", q)
		}
	}
	if cycleReq == nil || !strings.Contains(cycleReq.Query, "user_id=ou_me") || cycleReq.Auth != "Bearer t-bot" {
		t.Fatalf("应以默认 bot 身份查询当前用户的 v2 周期: %+v", cycleReq)
	}
}

// TestOKRCycleListTenantKeepsV1 --tenant 保留旧的 v1 租户周期查询
func TestOKRCycleListTenantKeepsV1(t *testing.T) {
	rec := setupWorkCmdTest(t, "", func(w http.ResponseWriter, r *http.Request, body string) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":"p1","zh_name":"2026 上半年","status":1}],"has_more":false}}`))
	})
	out, err := runWorkCmd(t, okrCycleListCmd, nil, map[string]string{"tenant": "true"})
	if err != nil {
		t.Fatalf("--tenant: %v", err)
	}
	if !strings.Contains(out, "2026 上半年") || rec.apiReqs()[0].Path != "/open-apis/okr/v1/periods" {
		t.Fatalf("--tenant 应走 v1: %s %+v", out, rec.apiReqs())
	}
}
