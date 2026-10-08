package client

import (
	"fmt"
	"net/http"
	"testing"
)

func TestListDriveFileHistory(t *testing.T) {
	var q string
	_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		q = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"has_more":true,"items":[
			{"version":"11","name":"a.pdf","edit_time":"1700000000000","edit_user_id":"ou_x","size":"14","type":1,"tag":2},
			{"version":"10","name":"a.pdf","edit_time":1690000000000,"size":10,"type":4,"is_deleted":true}]}}`)
	})
	defer cleanup()
	page, err := ListDriveFileHistory("boxcn1", 2, "1800000000000", "u-x")
	if err != nil {
		t.Fatal(err)
	}
	if q != "last_edit_time=1800000000000&only_tag=true&page_size=2" {
		t.Fatalf("query = %s", q)
	}
	if len(page.Versions) != 2 || page.Versions[0].SizeBytes != 14 || page.Versions[1].ActionType != "revert" || !page.Versions[1].IsDeleted {
		t.Fatalf("page = %+v", page)
	}
	if page.NextCursor != "1690000000000" {
		t.Fatalf("next cursor = %q", page.NextCursor)
	}
}
