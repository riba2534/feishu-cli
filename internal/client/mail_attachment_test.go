package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGetMailAttachmentDownloadURLs_Batch20 每批最多 20 个 attachment_ids，结果合并、failed_ids 透出。
func TestGetMailAttachmentDownloadURLs_Batch20(t *testing.T) {
	var batches [][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/messages/m1/attachments/download_url") {
			http.NotFound(w, r)
			return
		}
		ids := r.URL.Query()["attachment_ids"]
		batches = append(batches, ids)
		var items []string
		for _, id := range ids {
			if id == "bad" {
				continue
			}
			items = append(items, `{"attachment_id":"`+id+`","download_url":"https://dl.example.com/`+id+`"}`)
		}
		failed := `[]`
		for _, id := range ids {
			if id == "bad" {
				failed = `["bad"]`
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"download_urls":[`+strings.Join(items, ",")+`],"failed_ids":`+failed+`}}`)
	}))
	defer srv.Close()
	setupTestConfig(t, srv.URL)

	ids := []string{"bad"}
	for i := 0; i < 22; i++ {
		ids = append(ids, "a"+string(rune('a'+i)))
	}
	urls, failed, err := GetMailAttachmentDownloadURLs("me", "m1", ids, "u-test-token")
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 2 || len(batches[0]) != 20 || len(batches[1]) != 3 {
		t.Errorf("分批错误: %v", batches)
	}
	if len(urls) != 22 || urls["aa"] != "https://dl.example.com/aa" {
		t.Errorf("urls = %v", urls)
	}
	if len(failed) != 1 || failed[0] != "bad" {
		t.Errorf("failed = %v", failed)
	}
}

func TestDownloadPresignedBytes_RejectsUnsafeURL(t *testing.T) {
	for _, u := range []string{"http://dl.example.com/x", "https://127.0.0.1/x", "https://localhost/x", "file:///etc/passwd"} {
		if _, err := DownloadPresignedBytes(u, 1024); err == nil {
			t.Errorf("%s 应被拒绝", u)
		}
	}
}
