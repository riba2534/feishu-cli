package client

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestBuildCancelApprovalInstanceBody(t *testing.T) {
	body, userIDType, err := buildCancelApprovalInstanceBody(CancelApprovalInstanceOptions{
		InstanceCode: "instance_1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if userIDType != "" {
		t.Fatalf("userIDType = %q, want empty", userIDType)
	}
	assertApprovalBodyField(t, body, "instance_code", "instance_1")
}

func TestBuildCCApprovalInstanceBody(t *testing.T) {
	body, userIDType, err := buildCCApprovalInstanceBody(CCApprovalInstanceOptions{
		InstanceCode: "instance_1",
		CCUserIDs:    []string{"on_a", "on_b"},
		Comment:      "请知悉",
		UserIDType:   "union_id",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if userIDType != "union_id" {
		t.Fatalf("userIDType = %q, want union_id", userIDType)
	}
	assertApprovalBodyField(t, body, "instance_code", "instance_1")
	assertApprovalBodyField(t, body, "comment", "请知悉")
	gotIDs, ok := body["cc_user_ids"].([]string)
	if !ok || len(gotIDs) != 2 || gotIDs[0] != "on_a" || gotIDs[1] != "on_b" {
		t.Fatalf("cc_user_ids = %#v, want [on_a on_b]", body["cc_user_ids"])
	}
}

func TestBuildApprovalTaskActionBody(t *testing.T) {
	body, userIDType, err := buildApprovalTaskActionBody(ApprovalTaskActionOptions{
		InstanceCode: "instance_1",
		TaskID:       "task_1",
		Comment:      "同意",
		Form:         `[{"id":"widget_1","type":"input","value":"ok"}]`,
	}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if userIDType != "" {
		t.Fatalf("userIDType = %q, want empty", userIDType)
	}
	assertApprovalBodyField(t, body, "instance_code", "instance_1")
	assertApprovalBodyField(t, body, "task_id", "task_1")
	assertApprovalBodyField(t, body, "comment", "同意")
	assertApprovalBodyField(t, body, "form", `[{"id":"widget_1","type":"input","value":"ok"}]`)
}

func TestBuildApprovalTaskActionBodyRejectOmitsForm(t *testing.T) {
	body, _, err := buildApprovalTaskActionBody(ApprovalTaskActionOptions{
		InstanceCode: "instance_1",
		TaskID:       "task_1",
		Comment:      "拒绝",
		Form:         `[{"id":"ignored"}]`,
	}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := body["form"]; ok {
		t.Fatalf("reject body should not contain form, got %#v", body)
	}
}

func TestBuildTransferApprovalTaskBody(t *testing.T) {
	body, userIDType, err := buildTransferApprovalTaskBody(TransferApprovalTaskOptions{
		InstanceCode:   "instance_1",
		TaskID:         "task_1",
		TransferUserID: "on_target",
		Comment:        "请代审",
		UserIDType:     "union_id",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if userIDType != "union_id" {
		t.Fatalf("userIDType = %q, want union_id", userIDType)
	}
	assertApprovalBodyField(t, body, "instance_code", "instance_1")
	assertApprovalBodyField(t, body, "task_id", "task_1")
	assertApprovalBodyField(t, body, "transfer_user_id", "on_target")
	assertApprovalBodyField(t, body, "comment", "请代审")
}

func TestApprovalWriteUsesOfficialUATPathsAndUserToken(t *testing.T) {
	const userToken = "u-test"
	tests := []struct {
		name     string
		call     func() error
		wantPath string
	}{
		{
			name: "cancel",
			call: func() error {
				return CancelApprovalInstance(CancelApprovalInstanceOptions{
					InstanceCode: "instance_1",
				}, userToken)
			},
			wantPath: "/open-apis/approval/v4/instances/uat_cancel",
		},
		{
			name: "cc",
			call: func() error {
				return CCApprovalInstance(CCApprovalInstanceOptions{
					InstanceCode: "instance_1",
					CCUserIDs:    []string{"ou_a"},
				}, userToken)
			},
			wantPath: "/open-apis/approval/v4/instances/uat_cc",
		},
		{
			name: "approve",
			call: func() error {
				return ApproveApprovalTask(ApprovalTaskActionOptions{
					InstanceCode: "instance_1",
					TaskID:       "task_1",
				}, userToken)
			},
			wantPath: "/open-apis/approval/v4/tasks/uat_approval",
		},
		{
			name: "reject",
			call: func() error {
				return RejectApprovalTask(ApprovalTaskActionOptions{
					InstanceCode: "instance_1",
					TaskID:       "task_1",
				}, userToken)
			},
			wantPath: "/open-apis/approval/v4/tasks/uat_reject",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath, gotAuth, gotUserIDType string
			var gotBody map[string]any
			_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotAuth = r.Header.Get("Authorization")
				gotUserIDType = r.URL.Query().Get("user_id_type")
				if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
					t.Fatalf("decode request body: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"code":0,"msg":"ok","data":{}}`))
			})
			defer cleanup()

			if err := tt.call(); err != nil {
				t.Fatalf("call error: %v", err)
			}
			if gotPath != tt.wantPath {
				t.Fatalf("path = %q, want %q", gotPath, tt.wantPath)
			}
			if gotAuth != "Bearer "+userToken {
				t.Fatalf("Authorization = %q, want user token", gotAuth)
			}
			if tt.name == "cc" && gotUserIDType != "open_id" {
				t.Fatalf("user_id_type = %q, want open_id", gotUserIDType)
			}
			if tt.name != "cc" && gotUserIDType != "" {
				t.Fatalf("user_id_type = %q, want empty", gotUserIDType)
			}
			assertApprovalBodyField(t, gotBody, "instance_code", "instance_1")
		})
	}
}

func TestTransferApprovalTaskUsesOfficialUATTransferAndUserToken(t *testing.T) {
	const userToken = "u-test"
	var gotPath, gotAuth, gotUserIDType string
	var gotBody map[string]any
	_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotUserIDType = r.URL.Query().Get("user_id_type")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":"ok","data":{}}`))
	})
	defer cleanup()

	err := TransferApprovalTask(TransferApprovalTaskOptions{
		InstanceCode:   "instance_1",
		TaskID:         "task_1",
		TransferUserID: "ou_target",
		UserIDType:     "open_id",
	}, userToken)
	if err != nil {
		t.Fatalf("TransferApprovalTask() error = %v", err)
	}
	if gotPath != "/open-apis/approval/v4/tasks/uat_transfer" {
		t.Fatalf("path = %q, want uat_transfer", gotPath)
	}
	if gotAuth != "Bearer "+userToken {
		t.Fatalf("Authorization = %q, want user token", gotAuth)
	}
	if gotUserIDType != "open_id" {
		t.Fatalf("user_id_type = %q, want open_id", gotUserIDType)
	}
	assertApprovalBodyField(t, gotBody, "instance_code", "instance_1")
	assertApprovalBodyField(t, gotBody, "task_id", "task_1")
	assertApprovalBodyField(t, gotBody, "transfer_user_id", "ou_target")
}

func TestTransferApprovalTaskRejectsMissingUserToken(t *testing.T) {
	err := TransferApprovalTask(TransferApprovalTaskOptions{
		InstanceCode:   "instance_1",
		TaskID:         "task_1",
		TransferUserID: "ou_target",
	}, "")
	if err == nil {
		t.Fatal("expected missing user token error")
	}
}

func assertApprovalBodyField(t *testing.T, body map[string]any, key, want string) {
	t.Helper()
	if got, ok := body[key]; !ok || got != want {
		t.Fatalf("body[%q] = %#v ok=%v, want %q", key, got, ok, want)
	}
}
