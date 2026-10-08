package client

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

// TestUploadBitableAttachmentMultipart >20MB 走 upload_prepare/part/finish，
// parent_type=bitable_file、parent_node=base_token，且不走 upload_all。
func TestUploadBitableAttachmentMultipart(t *testing.T) {
	tmp := t.TempDir() + "/big.bin"
	payload := []byte("0123456789abcdef")
	if err := os.WriteFile(tmp, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	var prepareBody map[string]any
	var parts int
	var sawUploadAll, sawFinish bool
	_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/medias/upload_all"):
			sawUploadAll = true
			_, _ = io.WriteString(w, `{"code":0,"data":{"file_token":"small"}}`)
		case strings.HasSuffix(r.URL.Path, "/medias/upload_prepare"):
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &prepareBody)
			_, _ = io.WriteString(w, `{"code":0,"data":{"upload_id":"up1","block_size":10,"block_num":2}}`)
		case strings.HasSuffix(r.URL.Path, "/medias/upload_part"):
			parts++
			_, _ = io.WriteString(w, `{"code":0}`)
		case strings.HasSuffix(r.URL.Path, "/medias/upload_finish"):
			sawFinish = true
			_, _ = io.WriteString(w, `{"code":0,"data":{"file_token":"boxBig"}}`)
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()
	orig := maxSingleUploadSize
	maxSingleUploadSize = len(payload) - 1
	defer func() { maxSingleUploadSize = orig }()

	token, err := UploadBitableAttachment(tmp, "bascnX", "u-test")
	if err != nil {
		t.Fatalf("multipart upload: %v", err)
	}
	if token != "boxBig" || parts != 2 || !sawFinish || sawUploadAll {
		t.Fatalf("token=%q parts=%d finish=%v uploadAll=%v", token, parts, sawFinish, sawUploadAll)
	}
	if prepareBody["parent_type"] != "bitable_file" || prepareBody["parent_node"] != "bascnX" {
		t.Errorf("prepare body 不对: %v", prepareBody)
	}
}

// TestUploadBitableAttachmentMultipartPartError 分片的业务错误（随 HTTP 400 下发）必须失败，不能静默继续。
func TestUploadBitableAttachmentMultipartPartError(t *testing.T) {
	tmp := t.TempDir() + "/big.bin"
	if err := os.WriteFile(tmp, []byte("0123456789abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/medias/upload_prepare"):
			_, _ = io.WriteString(w, `{"code":0,"data":{"upload_id":"up1","block_size":10,"block_num":2}}`)
		case strings.HasSuffix(r.URL.Path, "/medias/upload_part"):
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"code":1061002,"msg":"params error"}`)
		default:
			t.Errorf("分片失败后不应继续请求 %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	defer cleanup()
	orig := maxSingleUploadSize
	maxSingleUploadSize = 1
	defer func() { maxSingleUploadSize = orig }()

	if _, err := UploadBitableAttachment(tmp, "bascnX", "u-test"); err == nil || !HasAPICode(err, 1061002) {
		t.Fatalf("分片业务错误应透出 code=1061002，got %v", err)
	}
}
