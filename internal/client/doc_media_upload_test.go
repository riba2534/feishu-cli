package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestUploadDocMediaMultipart 超过单次上限的文档素材走 upload_prepare/part/finish，
// prepare 显式携带 parent_type/parent_node/extra(drive_route_token)，分片按服务端计划切分。
func TestUploadDocMediaMultipart(t *testing.T) {
	payload := strings.Repeat("A", 10) + strings.Repeat("B", 10) + "C"
	orig := maxSingleUploadSize
	maxSingleUploadSize = len(payload) - 1
	defer func() { maxSingleUploadSize = orig }()

	var mu sync.Mutex
	var prepare map[string]any
	var parts []string
	partAttempts := 0
	var finish map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case "/open-apis/drive/v1/medias/upload_prepare":
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &prepare)
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"upload_id":"up1","block_size":10,"block_num":3}}`)
		case "/open-apis/drive/v1/medias/upload_part":
			partAttempts++
			if partAttempts == 2 { // 第二片首次限流，验证分片级重试
				w.WriteHeader(http.StatusTooManyRequests)
				fmt.Fprint(w, `{"code":99991400,"msg":"request trigger frequency limit"}`)
				return
			}
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("解析分片表单失败: %v", err)
			}
			f, _, _ := r.FormFile("file")
			data, _ := io.ReadAll(f)
			parts = append(parts, r.FormValue("seq")+":"+r.FormValue("size")+":"+string(data))
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{}}`)
		case "/open-apis/drive/v1/medias/upload_finish":
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &finish)
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"file_token":"boxTok"}}`)
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	setupTestConfig(t, server.URL)

	path := filepath.Join(t.TempDir(), "big.bin")
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	token, err := UploadDocMedia(path, "docx_file", "blkFile", "", "docXYZ", "")
	if err != nil {
		t.Fatalf("UploadDocMedia 失败: %v", err)
	}
	if token != "boxTok" {
		t.Fatalf("token = %q", token)
	}
	if prepare["parent_type"] != "docx_file" || prepare["parent_node"] != "blkFile" || prepare["file_name"] != "big.bin" ||
		prepare["extra"] != `{"drive_route_token":"docXYZ"}` || prepare["size"] != float64(len(payload)) {
		t.Fatalf("prepare 请求体异常: %#v", prepare)
	}
	want := []string{"0:10:AAAAAAAAAA", "1:10:BBBBBBBBBB", "2:1:C"}
	if strings.Join(parts, "|") != strings.Join(want, "|") {
		t.Fatalf("分片 = %v，期望 %v", parts, want)
	}
	if finish["upload_id"] != "up1" || finish["block_num"] != float64(3) {
		t.Fatalf("finish 请求体异常: %#v", finish)
	}
}

func TestUploadDocMediaRejectsEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.bin")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := UploadDocMedia(path, "docx_file", "blk", "", "doc", ""); err == nil || !strings.Contains(err.Error(), "为空") {
		t.Fatalf("空文件应报错，得到 %v", err)
	}
}
