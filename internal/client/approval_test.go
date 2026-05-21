package client

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

func TestParseEmbeddedJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{
			name:  "json object",
			input: `{"name":"leave"}`,
			want:  map[string]any{"name": "leave"},
		},
		{
			name:  "json array",
			input: `[{"id":"widget_1"}]`,
			want:  []any{map[string]any{"id": "widget_1"}},
		},
		{
			name:  "plain string fallback",
			input: `not-json`,
			want:  "not-json",
		},
		{
			name:  "empty string",
			input: `   `,
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseEmbeddedJSON(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseEmbeddedJSON(%q) = %#v, want %#v", tt.input, got, tt.want)
			}
		})
	}
}

func TestApprovalTaskStringUnmarshal(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "string",
			input: `"RUNNING"`,
			want:  "RUNNING",
		},
		{
			name:  "number",
			input: `42`,
			want:  "42",
		},
		{
			name:  "boolean",
			input: `true`,
			want:  "true",
		},
		{
			name:  "null",
			input: `null`,
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got approvalTaskString
			if err := json.Unmarshal([]byte(tt.input), &got); err != nil {
				t.Fatalf("json.Unmarshal(%s) error = %v", tt.input, err)
			}
			if got.String() != tt.want {
				t.Fatalf("json.Unmarshal(%s) = %q, want %q", tt.input, got.String(), tt.want)
			}
		})
	}
}

func TestParseApprovalDefinitionResponse(t *testing.T) {
	body := []byte(`{
		"code": 0,
		"msg": "success",
		"data": {
			"approval_name": "请假",
			"status": "ACTIVE",
			"form": "{\"widgets\":[{\"id\":\"widget_1\"}]}",
			"node_list": [
				{
					"name": "直属上级",
					"node_id": "node_1",
					"custom_node_id": "custom_1",
					"node_type": "AND",
					"need_approver": true,
					"approver_chosen_multi": false,
					"require_signature": true
				}
			],
			"viewers": [
				{
					"type": "USER",
					"id": "ou_viewer",
					"user_id": "ou_viewer"
				}
			],
			"approval_admin_ids": ["ou_admin"],
			"form_widget_relation": "{\"widget_1\":[\"widget_2\"]}"
		}
	}`)

	result, err := parseApprovalDefinitionResponse(body, "approval_123")
	if err != nil {
		t.Fatalf("parseApprovalDefinitionResponse() error = %v", err)
	}

	if result.ApprovalCode != "approval_123" {
		t.Fatalf("ApprovalCode = %q, want %q", result.ApprovalCode, "approval_123")
	}
	if result.ApprovalName != "请假" {
		t.Fatalf("ApprovalName = %q, want %q", result.ApprovalName, "请假")
	}
	if result.Status != "ACTIVE" {
		t.Fatalf("Status = %q, want %q", result.Status, "ACTIVE")
	}

	wantForm := map[string]any{
		"widgets": []any{
			map[string]any{"id": "widget_1"},
		},
	}
	if !reflect.DeepEqual(result.Form, wantForm) {
		t.Fatalf("Form = %#v, want %#v", result.Form, wantForm)
	}

	if len(result.NodeList) != 1 || result.NodeList[0].NodeID != "node_1" || !result.NodeList[0].NeedApprover || !result.NodeList[0].RequireSignature {
		t.Fatalf("NodeList = %#v, want one populated node", result.NodeList)
	}
	if len(result.Viewers) != 1 || result.Viewers[0].UserID != "ou_viewer" {
		t.Fatalf("Viewers = %#v, want one viewer", result.Viewers)
	}
	if !reflect.DeepEqual(result.ApprovalAdminIDs, []string{"ou_admin"}) {
		t.Fatalf("ApprovalAdminIDs = %#v, want %#v", result.ApprovalAdminIDs, []string{"ou_admin"})
	}

	wantRelation := map[string]any{
		"widget_1": []any{"widget_2"},
	}
	if !reflect.DeepEqual(result.FormWidgetRelation, wantRelation) {
		t.Fatalf("FormWidgetRelation = %#v, want %#v", result.FormWidgetRelation, wantRelation)
	}
}

func TestParseApprovalDefinitionResponseError(t *testing.T) {
	body := []byte(`{"code": 99991663, "msg": "invalid approval code"}`)

	_, err := parseApprovalDefinitionResponse(body, "approval_123")
	if err == nil {
		t.Fatal("parseApprovalDefinitionResponse() error = nil, want non-nil")
	}
	if got := err.Error(); got != "获取审批定义失败: code=99991663, msg=invalid approval code" {
		t.Fatalf("parseApprovalDefinitionResponse() error = %q, want %q", got, "获取审批定义失败: code=99991663, msg=invalid approval code")
	}
}

func TestParseApprovalTaskQueryResponseHandlesNumericProcessStatus(t *testing.T) {
	body := []byte(`{
		"code": 0,
		"msg": "success",
		"data": {
			"page_token": "next_page",
			"has_more": true,
			"count": {
				"total": 1,
				"has_more": false
			},
			"tasks": [
				{
					"topic": 1,
					"user_id": "ou_user",
					"title": "审批标题",
					"status": "PENDING",
					"process_status": 12,
					"definition_name": "文档权限申请",
					"task_id": "task_id",
					"process_id": "process_id",
					"initiator_names": ["A"],
					"urls": {
						"pc": "https://pc"
					}
				}
			]
		}
	}`)

	result, err := parseApprovalTaskQueryResponse(body)
	if err != nil {
		t.Fatalf("parseApprovalTaskQueryResponse() error = %v", err)
	}

	if !result.HasMore {
		t.Fatalf("HasMore = %v, want true", result.HasMore)
	}
	if result.PageToken != "next_page" {
		t.Fatalf("PageToken = %q, want %q", result.PageToken, "next_page")
	}
	if result.Count == nil || result.Count.Total != 1 {
		t.Fatalf("Count = %#v, want total 1", result.Count)
	}
	if len(result.Tasks) != 1 {
		t.Fatalf("len(Tasks) = %d, want 1", len(result.Tasks))
	}
	if result.Tasks[0].Topic != "1" {
		t.Fatalf("Topic = %q, want %q", result.Tasks[0].Topic, "1")
	}
	if result.Tasks[0].ProcessStatus != "12" {
		t.Fatalf("ProcessStatus = %q, want %q", result.Tasks[0].ProcessStatus, "12")
	}
	if result.Tasks[0].PCURL != "https://pc" {
		t.Fatalf("PCURL = %q, want %q", result.Tasks[0].PCURL, "https://pc")
	}
}

func TestQueryApprovalTasksUsesOfficialUATQueryAndUserToken(t *testing.T) {
	const userToken = "u-test"
	var gotPath, gotAuth, gotUserID string
	_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotUserID = r.URL.Query().Get("user_id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":"ok","data":{"tasks":[],"has_more":false}}`))
	})
	defer cleanup()

	_, err := QueryApprovalTasksRaw(ApprovalTaskQueryOptions{
		UserID:     "ou_test",
		Topic:      "1",
		UserIDType: "open_id",
	}, userToken)
	if err != nil {
		t.Fatalf("QueryApprovalTasksRaw() error = %v", err)
	}
	if gotPath != "/open-apis/approval/v4/tasks/uat_query" {
		t.Fatalf("path = %q, want uat_query", gotPath)
	}
	if gotAuth != "Bearer "+userToken {
		t.Fatalf("Authorization = %q, want user token", gotAuth)
	}
	if gotUserID != "ou_test" {
		t.Fatalf("user_id = %q, want ou_test", gotUserID)
	}
}

func TestQueryApprovalTasksRejectsMissingUserToken(t *testing.T) {
	_, err := QueryApprovalTasksRaw(ApprovalTaskQueryOptions{UserID: "ou_test", Topic: "1"}, "")
	if err == nil {
		t.Fatal("expected missing user token error")
	}
}

func TestGetApprovalInstanceUsesOfficialUATGetAndUserToken(t *testing.T) {
	const userToken = "u-test"
	var gotPath, gotAuth, gotInstanceCode, gotUserIDType string
	_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotInstanceCode = r.URL.Query().Get("instance_code")
		gotUserIDType = r.URL.Query().Get("user_id_type")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":"ok","data":{"instance_code":"instance_1","status":"PENDING"}}`))
	})
	defer cleanup()

	data, err := GetApprovalInstance(GetApprovalInstanceOptions{
		InstanceCode: "instance_1",
		UserIDType:   "open_id",
	}, userToken)
	if err != nil {
		t.Fatalf("GetApprovalInstance() error = %v", err)
	}
	if gotPath != "/open-apis/approval/v4/instances/uat_get" {
		t.Fatalf("path = %q, want uat_get", gotPath)
	}
	if gotAuth != "Bearer "+userToken {
		t.Fatalf("Authorization = %q, want user token", gotAuth)
	}
	if gotInstanceCode != "instance_1" {
		t.Fatalf("instance_code = %q, want instance_1", gotInstanceCode)
	}
	if gotUserIDType != "open_id" {
		t.Fatalf("user_id_type = %q, want open_id", gotUserIDType)
	}
	if data["status"] != "PENDING" {
		t.Fatalf("status = %#v, want PENDING", data["status"])
	}
}

func TestGetApprovalInstanceRejectsMissingUserToken(t *testing.T) {
	_, err := GetApprovalInstanceRaw(GetApprovalInstanceOptions{InstanceCode: "instance_1"}, "")
	if err == nil {
		t.Fatal("expected missing user token error")
	}
}
