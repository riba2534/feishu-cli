package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplyMessageRequestIncludesThreadAndUUID(t *testing.T) {
	const (
		messageID = "om_parent"
		userToken = "u-test"
		uuid      = "reply-once"
	)
	var gotPath string
	var gotAuth string
	var gotBody map[string]interface{}
	handler := func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("解析请求体失败: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"message_id":"om_reply"}}`)
	}
	_, cleanup := stubFeishuServer(t, handler)
	defer cleanup()

	got, err := ReplyMessage(messageID, "image", `{"image_key":"img_xxx"}`, true, userToken, uuid)
	if err != nil {
		t.Fatalf("ReplyMessage() error = %v", err)
	}
	if got != "om_reply" {
		t.Fatalf("message_id = %q, want om_reply", got)
	}
	if gotPath != "/open-apis/im/v1/messages/"+messageID+"/reply" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer "+userToken {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotBody["msg_type"] != "image" ||
		gotBody["content"] != `{"image_key":"img_xxx"}` ||
		gotBody["reply_in_thread"] != true ||
		gotBody["uuid"] != uuid {
		t.Fatalf("请求体不完整: %#v", gotBody)
	}
}

func TestMessageWriteRejectsSuccessfulResponseWithoutMessageID(t *testing.T) {
	tests := []struct {
		name string
		call func() (string, error)
	}{
		{
			name: "send nil data",
			call: func() (string, error) {
				return SendMessage("chat_id", "oc_xxx", "text", `{"text":"x"}`, "u-test", "")
			},
		},
		{
			name: "reply empty message id",
			call: func() (string, error) {
				return ReplyMessage("om_xxx", "text", `{"text":"x"}`, false, "u-test", "")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.Contains(test.name, "nil") {
					_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":null}`)
					return
				}
				_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"message_id":""}}`)
			}
			_, cleanup := stubFeishuServer(t, handler)
			defer cleanup()

			got, err := test.call()
			if err == nil || got != "" || !strings.Contains(err.Error(), "消息 ID") {
				t.Fatalf("got=%q err=%v, want missing message ID error", got, err)
			}
		})
	}
}

func TestReplyMessageBusinessErrorIsFailure(t *testing.T) {
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":99992402,"msg":"field validation failed","data":{}}`)
	}
	_, cleanup := stubFeishuServer(t, handler)
	defer cleanup()

	got, err := ReplyMessage("om_xxx", "text", `{"text":"x"}`, false, "u-test", "reply-once")
	if err == nil || got != "" || !strings.Contains(err.Error(), "99992402") {
		t.Fatalf("got=%q err=%v, want business error", got, err)
	}
}

func TestUploadIMFileWithOptionsMultipart(t *testing.T) {
	var gotFileType, gotFileName, gotDuration string
	business := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/im/v1/files" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("解析 multipart 失败: %v", err)
		}
		gotFileType = r.FormValue("file_type")
		gotFileName = r.FormValue("file_name")
		gotDuration = r.FormValue("duration")
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("缺少 file part: %v", err)
		}
		_ = file.Close()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"file_key":"file_uploaded"}}`)
	}
	_, cleanup := stubFeishuServer(t, tenantRouteHandler(t, business))
	defer cleanup()

	path := filepath.Join(t.TempDir(), "voice.ogg")
	if err := os.WriteFile(path, []byte("fake-opus"), 0o600); err != nil {
		t.Fatal(err)
	}
	key, err := UploadIMFileWithOptions(path, "voice.ogg", "opus", 1234)
	if err != nil {
		t.Fatalf("UploadIMFileWithOptions() error = %v", err)
	}
	if key != "file_uploaded" ||
		gotFileType != "opus" ||
		gotFileName != "voice.ogg" ||
		gotDuration != "1234" {
		t.Fatalf("key=%q file_type=%q file_name=%q duration=%q", key, gotFileType, gotFileName, gotDuration)
	}
}

func TestUploadIMFileRejectsEmptyFile(t *testing.T) {
	_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("空文件不应发起网络请求")
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer cleanup()

	path := filepath.Join(t.TempDir(), "empty.bin")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := UploadIMFile(path, ""); err == nil || !strings.Contains(err.Error(), "空文件") {
		t.Fatalf("期望空文件错误，实际: %v", err)
	}
}
