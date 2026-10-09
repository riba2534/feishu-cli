package client

import (
	"fmt"
	"net/http"
	"testing"
)

func TestGetDriveQuota_ParsesNumbersAndStrings(t *testing.T) {
	var gotPath string
	_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"biz_infos":[{"name":"ccm","used":"100"},{"name":"all","used":300}],
			"is_tenant_quota_exceeded":true,"user_quota":{"limit":"1000","usage":"300","type":1}}}`)
	})
	defer cleanup()
	q, err := GetDriveQuota("u123", "u-x")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/open-apis/drive/v2/quota_details/u123" {
		t.Fatalf("path = %s", gotPath)
	}
	if q.Total != 1000 || q.Used != 300 || q.Unlimited || !q.IsTenantQuotaExceeded || len(q.BizInfos) != 2 || q.BizInfos[0].Used != 100 {
		t.Fatalf("q = %+v", q)
	}
}

func TestGetDriveQuota_UnlimitedAndRequiresUser(t *testing.T) {
	_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"biz_infos":[{"name":"all","used":5}],"user_quota":{"limit":9223372036854775807,"usage":5}}}`)
	})
	defer cleanup()
	q, err := GetDriveQuota("u1", "u-x")
	if err != nil || !q.Unlimited || q.Used != 5 {
		t.Fatalf("q=%+v err=%v", q, err)
	}
	if _, err := GetDriveQuota("u1", ""); err == nil {
		t.Fatal("缺 User Token 应报错")
	}
}
