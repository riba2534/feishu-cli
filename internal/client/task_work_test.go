package client

import (
	"net/http"
	"testing"
)

// TestCompleteTaskSkipsWhenAlreadyCompleted 已完成任务不再 PATCH（回归：此前每次都改写完成时间）
func TestCompleteTaskSkipsWhenAlreadyCompleted(t *testing.T) {
	got := captureAPI(t, func(w http.ResponseWriter, r *http.Request, cap *capturedHTTPRequest) {
		writeJSON(w, http.StatusOK, `{"code":0,"data":{"task":{"guid":"t1","summary":"x","completed_at":"1790000000000"}}}`)
	})
	info, err := CompleteTask("t1", testUserToken)
	if err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}
	if !info.AlreadyCompleted || info.CompletedAt == "" {
		t.Fatalf("应标记 already_completed: %+v", info)
	}
	for _, r := range got() {
		if r.Method == http.MethodPatch {
			t.Fatalf("已完成任务不应 PATCH: %+v", r)
		}
	}
}

// TestCompleteTaskPatchesWhenOpen 未完成任务正常 PATCH completed_at
func TestCompleteTaskPatchesWhenOpen(t *testing.T) {
	got := captureAPI(t, func(w http.ResponseWriter, r *http.Request, cap *capturedHTTPRequest) {
		if r.Method == http.MethodGet {
			writeJSON(w, http.StatusOK, `{"code":0,"data":{"task":{"guid":"t1","summary":"x","completed_at":"0"}}}`)
			return
		}
		writeJSON(w, http.StatusOK, `{"code":0,"data":{"task":{"guid":"t1","summary":"x","completed_at":"1790000000000"}}}`)
	})
	info, err := CompleteTask("t1", testUserToken)
	if err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}
	reqs := got()
	if len(reqs) != 2 || reqs[1].Method != http.MethodPatch || info.AlreadyCompleted {
		t.Fatalf("应先 GET 再 PATCH: %+v / %+v", reqs, info)
	}
}
