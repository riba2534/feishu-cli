package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// >20MB 覆盖走 upload_prepare（携带 file_token）→ upload_part → upload_finish，file_token 不变。
func TestOverwriteDriveFileFromPath_MultipartCarriesFileToken(t *testing.T) {
	old := maxSingleUploadSize
	maxSingleUploadSize = 4
	defer func() { maxSingleUploadSize = old }()

	var mu sync.Mutex
	var prepareBody map[string]any
	var parts int
	var finished bool
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/open-apis/drive/v1/files/upload_prepare":
			b, _ := io.ReadAll(r.Body)
			mu.Lock()
			_ = json.Unmarshal(b, &prepareBody)
			mu.Unlock()
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"upload_id":"up1","block_size":4,"block_num":3}}`)
		case "/open-apis/drive/v1/files/upload_part":
			mu.Lock()
			parts++
			mu.Unlock()
			_, _ = fmt.Fprint(w, `{"code":0,"data":{}}`)
		case "/open-apis/drive/v1/files/upload_finish":
			finished = true
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"file_token":"boxcn_keep","version":"12"}}`)
		default:
			http.NotFound(w, r)
		}
	}
	_, cleanup := stubFeishuServer(t, handler)
	defer cleanup()

	p := filepath.Join(t.TempDir(), "big.bin")
	if err := os.WriteFile(p, []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := OverwriteDriveFileFromPath(p, "fld_parent", "big.bin", "boxcn_keep", "u-x")
	if err != nil {
		t.Fatalf("分片覆盖失败: %v", err)
	}
	if res.FileToken != "boxcn_keep" || res.Version != "12" {
		t.Fatalf("res = %+v", res)
	}
	if prepareBody["file_token"] != "boxcn_keep" || prepareBody["parent_node"] != "fld_parent" {
		t.Fatalf("upload_prepare body 缺 file_token: %+v", prepareBody)
	}
	if parts != 3 || !finished {
		t.Fatalf("parts=%d finished=%v", parts, finished)
	}
}

// 业务码随 HTTP 400 下发时保留业务码（HasAPICode 可用于分级）。
func TestOverwriteDriveFileFromPath_HTTP400BusinessCode(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"code":1061002,"msg":"params error","log_id":"lg1"}`)
	}
	_, cleanup := stubFeishuServer(t, handler)
	defer cleanup()

	p := filepath.Join(t.TempDir(), "a.txt")
	_ = os.WriteFile(p, []byte("x"), 0o644)
	_, err := OverwriteDriveFileFromPath(p, "fld", "a.txt", "boxcn_keep", "u-x")
	if err == nil || !HasAPICode(err, 1061002) || !strings.Contains(err.Error(), "lg1") {
		t.Fatalf("want APIError 1061002 with log_id, got %v", err)
	}
}
